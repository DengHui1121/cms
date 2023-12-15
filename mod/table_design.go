package mod

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	"gorm.io/gorm/clause"
	"main/alert"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mozillazg/go-pinyin"
	"gorm.io/gorm"
)

type User struct {
	ID       uint   `gorm:"primarykey" json:"id,string"`
	Username string `json:"username"`
	Password string `json:"password"`
	Level    uint8  `json:"level"` //1：元账号 2：系统账号 3：访客账号
}
type PublicUser struct {
	*User              // 匿名嵌套
	Password *struct{} `json:"password,omitempty"`
}

type Model struct {
	CreatedAt time.Time      `json:"-"`
	UpdatedAt time.Time      `json:"-"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	UUID      string         `gorm:"type:char(36);unique_index" json:"-"`
}

func (u *Model) BeforeCreate(tx *gorm.DB) error {
	var err error
	if u.UUID == "" {
		u.UUID = uuid.NewString()
	}
	if err != nil {
		return err
	}
	return nil
}

type Model_Equip struct {
	CreatedAt time.Time      `json:"-"`
	UpdatedAt time.Time      `json:"-"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func InitialLetter(cn string) (firstLetters string) {
	a := pinyin.NewArgs()
	a.Style = pinyin.Normal
	a.Fallback = func(r rune, a pinyin.Args) []string {
		return []string{string(r)}
	}
	ils := pinyin.Pinyin(cn, a)
	for _, py := range ils {
		firstLetters += string(py[0][0])
	}
	return
}

type Factory struct {
	Model_Equip `gorm:"embedded"`
	ID          uint       `gorm:"primarykey" json:"id,string"`
	UUID        string     `gorm:"unique_index" json:"-"`
	Name        string     `gorm:"not null" json:"company_name"`
	Windfarms   []Windfarm `json:"children" gorm:"foreignKey:FactoryUUID;references:UUID"`
}

func (u *Factory) BeforeCreate(tx *gorm.DB) error {
	u.UUID = InitialLetter(u.Name)
	var c int64
	tx.Table("factory").Where("uuid=?", u.UUID).Count(&c)
	if c != 0 {
		u.UUID = u.UUID + "#" + fmt.Sprint(time.Now().Unix())
	}
	return nil
}

type Windfarm struct {
	Model_Equip       `gorm:"embedded" `
	ID                uint      `gorm:"primarykey" json:"id,string"`
	UUID              string    `gorm:"unique_index" json:"-"`
	FactoryID         uint      `gorm:"-" json:"company_id,string"`
	FactoryUUID       string    `gorm:"comment:公司uuid" json:"-"`
	Name              string    `gorm:"not null; comment:风场名" json:"desc"`            //前端显示为风场编码
	Desc              string    `gorm:"not null; comment:风场描述" json:"windfield_name"` //前端显示为风场名称
	Province          string    `gorm:"not null; comment:省" json:"province"`
	City              string    `gorm:"not null; comment:市" json:"city"`
	District          string    `gorm:"not null; comment:地区" json:"district"`
	Longitude         float32   `gorm:"not null; comment:经度" json:"longitude,string"`
	Latitude          float32   `gorm:"not null; comment:纬度" json:"latitude,string"`
	InstalledCapacity float32   `gorm:"-" json:"installedCapacity"`
	Status            uint8     `gorm:"type:tinyint;default:0" json:"status,string"`
	MachineCounts     int       `gorm:"-" json:"machineCounts"`
	Machines          []Machine `json:"children" gorm:"foreignKey:WindfarmUUID;references:UUID"`
	//一年内故障次数
	TotalAlertCount int `json:"total_alert_count"  gorm:"-"`
}

func (u *Windfarm) BeforeCreate(tx *gorm.DB) error {
	u.UUID = u.FactoryUUID + "_" + InitialLetter(u.Name)
	var c int64
	tx.Table("windfarm").Where("uuid=?", u.UUID).Count(&c)
	if c != 0 {
		u.UUID = u.UUID + "#" + fmt.Sprint(time.Now().Unix())
	}
	return nil
}
func (u *Windfarm) AfterFind(tx *gorm.DB) error {
	err := tx.Table("factory").Where("uuid=?", u.FactoryUUID).Pluck("id", &u.FactoryID).Error
	if err != nil {
		u.FactoryID = 0
		return err
	}

	var statuss []uint
	tx.Table("windfarm").Joins("join machine on machine.windfarm_uuid = windfarm.uuid").Where("windfarm.id=?", u.ID).Select("machine.status").Scan(&statuss)
	_, max := MaxStatus(statuss)
	if u.Status != uint8(max) {
		tx.Table("windfarm").Where("id=?", u.ID).Update("status", max)
		u.Status = uint8(max)
	}
	//一年内故障次数
	stime := time.Now().AddDate(-1, 0, 0)
	etime := time.Now()
	sub := tx.Table("windfarm").Where("windfarm.uuid=?", u.UUID).
		Joins("right join windfarm_month_report on windfarm.uuid =windfarm_month_report.windfarm_uuid").
		Where("windfarm_month_report.time_set < ? AND windfarm_month_report.time_set >= ?", etime.Unix(), stime.Unix()).
		Select("windfarm.uuid AS windfarm_uuid", "windfarm_month_report.total_alert_count AS total_alert_count")
	var count int64
	sub.Count(&count)
	if count != 0 {
		tx.Table("(?) as tree", sub).
			Group("windfarm_uuid").
			Select("SUM(total_alert_count) AS total_alert_count").
			Scan(&u.TotalAlertCount)
	} else {
		u.TotalAlertCount = 0
	}
	// if u.Desc == "" {
	// 	u.Desc = u.Name
	// 	var desccount int64
	// 	tx.Table("windfarm").Where("'desc'=?", u.Desc).Count(&desccount)
	// 	if desccount == 0 {
	// 		tx.Table("windfarm").Where("windfarm.uuid=?", u.UUID).Update("desc", u.Desc)
	// 	}
	// }
	return nil
}

// fan_name和desc互换。fan_name:业主定义风机名，desc：数据导入的索引
type Machine struct {
	Model        `gorm:"embedded"`
	ID           uint   `gorm:"primarykey" json:"id,string"`
	UUID         string `gorm:"unique_index" json:"uuid"`
	WindfarmID   uint   `gorm:"-" json:"windfield_id,string"`
	WindfarmUUID string `json:"-"`
	Name         string `gorm:"not null; comment:风机名" json:"desc" toml:"name"`
	Type         int    `gorm:"not null; comment:风机类型" json:"model"`
	// PointVersion    string  `json:"point_version,omitempty"`
	// PropertyVersion string  `json:"property_version,omitempty"`
	// AlertVersion    string  `json:"alert_version,omitempty"`
	FanVersion      string  `gorm:"comment:风机标准" json:"version"` //风机标准
	TreeVersion     string  `gorm:"comment:设备树版本" json:"tree_version"`
	Unit            string  `json:"unit" toml:"unit"`
	Desc            string  `gorm:"comment:风机描述" json:"fan_name"`
	BuiltTime       string  `gorm:"comment:投运时间" json:"time"`
	Capacity        float32 `gorm:"column:capacity; comment:容量" json:"capacity,string"`
	OverinsuredTime string  `gorm:"column:overinsuredTime;comment:过保时间" json:"overinsuredTime"`
	Status          uint8   `gorm:"type:tinyint;default:0;comment:状态" json:"status,string"`
	Genfactory      string  `gorm:"comment:发电机厂家" json:"genfactory" toml:"genfactory"`    //TODO 发电机厂家
	Gentype         string  `gorm:"comment:发电机型号" json:"gentype" toml:"gentype"`          //TODO 发电机型号
	Gearfactory     string  `gorm:"comment:齿轮箱厂家" json:"gbxfactory" toml:"gbxfactory"`    //TODO 齿轮箱厂家
	Geartype        string  `gorm:"comment:齿轮箱型号" json:"gbxtype" toml:"gbxtype"`          //TODO 齿轮箱型号
	Mainshaffactory string  `gorm:"comment:主轴厂家" json:"mbrfactory" toml:"mbrfactory"`     //TODO 主轴厂家
	Mainshaftype    string  `gorm:"comment:主轴型号" json:"mbrtype" toml:"mbrtype"`           //TODO 主轴型号
	Bladefactory    string  `gorm:"comment:叶片厂家" json:"bladefactory" toml:"bladefactory"` //TODO 叶片厂家
	Bladetype       string  `gorm:"comment:叶片型号" json:"bladetype" toml:"bladetype"`       //TODO 叶片型号
	Health          float64 `gorm:"-" json:"health"`                                      //全生命周期
	Tag             int     `gorm:"column:tag;comment:故障标签" json:"tag"`
	//一年内故障次数
	TotalAlertCount int    `json:"total_alert_count"  gorm:"-"`
	Parts           []Part `json:"children" gorm:"foreignKey:MachineUUID;references:UUID"`
	//用于批量新建
	FanFront     string `gorm:"-" json:"fan_front,omitempty"`
	StartNum     int    `json:"start_num,omitempty" gorm:"-"`
	DescFront    string `gorm:"-" json:"desc_front,omitempty"`
	DescStartNum int    `json:"desc_start_num,omitempty" gorm:"-"`
	EndNum       int    `json:"end_num,omitempty" gorm:"-"`
	//故障使能开关
	BandAlertSet bool `gorm:"band_alert_set" json:"band_alert_bool"`
	TreeAlertSet bool `gorm:"tree_alert_set" json:"tree_alert_bool"`
}

type MachineStd struct {
	Model   `gorm:"embedded"`
	ID      uint   `gorm:"primarykey" json:"id,string"`
	Version string `gorm:"version" json:"version"`
	Desc    string `gorm:"desc" json:"desc"`
	Set     []byte `gorm:"set" json:"-"`
}

func (u *Machine) BeforeCreate(tx *gorm.DB) error {
	u.UUID = u.WindfarmUUID + "_" + InitialLetter(u.Name)
	var c int64
	tx.Table("machine").Where("uuid=?", u.UUID).Count(&c)
	if c != 0 {
		u.UUID = u.UUID + "#" + fmt.Sprint(time.Now().Unix())
	}
	return nil
}

// * 一个月以内无数据为无数据
func (u *Machine) AfterFind(tx *gorm.DB) error {
	err := tx.Table("windfarm").Where("uuid=?", u.WindfarmUUID).Pluck("id", &u.WindfarmID).Error
	if err != nil {
		u.WindfarmID = 0
		return err
	}
	// var statuss []uint
	// tx.Table("machine").Joins("join part on machine.uuid = part.machine_uuid").Where("machine.id=?", u.ID).Select("part.status").Scan(&statuss)
	// _, max := MaxStatus(statuss)
	// if u.Status != uint8(max) {
	// 	tx.Table("machine").Where("id=?", u.ID).Update("status", max)
	// 	u.Status = uint8(max)
	// }
	var subfv string
	tx.Table("machine_std").
		Where("id=?", u.FanVersion).
		Pluck("version", &subfv)
	if subfv != "" {
		u.FanVersion = subfv
	}

	//一年内故障次数
	stime := time.Now().AddDate(-1, 0, 0)
	etime := time.Now()
	sub := tx.Table("machine").Where("machine.uuid=?", u.UUID).
		Joins("right join machine_month_report on machine.uuid =machine_month_report.machine_uuid").
		Where("machine_month_report.time_set < ? AND machine_month_report.time_set >= ?", etime.Unix(), stime.Unix()).
		Select("machine.uuid AS machine_uuid", "machine_month_report.total_alert_count AS total_alert_count")
	var count int64
	sub.Count(&count)
	if count != 0 {
		tx.Table("(?) as tree", sub).
			Group("machine_uuid").
			Select("SUM(total_alert_count) AS total_alert_count").
			Scan(&u.TotalAlertCount)
	} else {
		u.TotalAlertCount = 0
	}
	return nil
}

type Part_2 struct {
	Model_Equip `gorm:"embedded"`
	ID          uint       `gorm:"primarykey" json:"id,string"`
	UUID        string     `gorm:"unique_index" json:"-"`
	MachineID   uint       `gorm:"-" json:"fan_id,string"`
	MachineUUID string     `json:"-" `
	Name        string     `gorm:"not null" json:"part_name" toml:"name"`
	Type        string     `gorm:"not null" json:"part_type" toml:"type"`
	Module      string     `gorm:"default:CMS" json:"module" ` //TODO 所属模块：CMS BMS（叶片） TMS（塔架）
	Properties  []Property `json:"characteristic" gorm:"foreignKey:PartUUID;references:UUID" `
	Points      []Point    `json:"measuring_point" gorm:"foreignKey:PartUUID;references:UUID" `
	Status      uint8      `gorm:"type:tinyint;default:0" json:"status,string"`
}

type Part struct {
	Model_Equip `gorm:"embedded"`
	ID          uint         `gorm:"primarykey" json:"id,string"`
	UUID        string       `gorm:"unique_index" json:"uuid"`
	MachineID   uint         `gorm:"-" json:"fan_id,string"`
	MachineUUID string       `json:"-" `
	Name        string       `gorm:"not null; comment: 部件名" json:"part_name" toml:"name"`
	Type        string       `gorm:"not null; comment: 部件类型" json:"part_type" toml:"type"`
	TypeEN      string       `gorm:"not null; comment: 部件类型(英文)" json:"part_type_en" toml:"nameEn"`
	Tag         int          `gorm:"column:tag;comment:故障标签" json:"tag"`
	Module      string       `gorm:"default:CMS; comment: 所属模块CMS BMS(叶片)TMS(塔架)" json:"module" ` //TODO 所属模块：CMS BMS（叶片） TMS（塔架）
	Points      []Point      `json:"measuring_point" gorm:"foreignKey:PartUUID;references:UUID" `
	Properties  []Property   `json:"characteristic" gorm:"foreignKey:PartUUID;references:UUID"`
	Bands       []alert.Band `json:"band"  gorm:"foreignKey:PartUUID;references:UUID"`
	Status      uint8        `gorm:"type:tinyint;default:0;comment:状态" json:"status,string"`
}

func (u *Part) BeforeCreate(tx *gorm.DB) error {
	u.UUID = u.MachineUUID + "_" + InitialLetter(u.Name)
	var c int64
	tx.Table("part").Where("uuid=?", u.UUID).Count(&c)
	if c != 0 {
		u.UUID = u.UUID + "#" + fmt.Sprint(time.Now().Unix())
	}
	return nil
}
func (u *Part) AfterFind(tx *gorm.DB) error {
	err := tx.Table("machine").Where("uuid=?", u.MachineUUID).Pluck("id", &u.MachineID).Error
	if err != nil {
		u.MachineID = 0
		return err
	}
	var statuss []uint
	tx.Table("part").Joins("join point on part.uuid = point.part_uuid").Where("part.id=?", u.ID).Select("point.status").Scan(&statuss)
	_, max := MaxStatus(statuss)
	if u.Status != uint8(max) {
		tx.Table("part").Where("id=?", u.ID).Update("status", max)
		u.Status = uint8(max)
	}
	return nil
}

type PropertyStd struct {
	Model    `gorm:"embedded"`
	ID       uint    `gorm:"primarykey" json:"id,string"`
	Version  string  `json:"-"`
	PartType string  `gorm:"column:part_type" json:"part_type"` //已改：部件名！不是部件类型
	Name     string  `json:"properties"`
	NameEn   string  `json:"name_en"` //TODO 英文名 用于故障树索引
	Formula  string  `json:"formula"`
	Value    float32 `json:"value"`
}
type Property_2 struct {
	Model_Equip `gorm:"embedded"`
	ID          uint    `gorm:"primarykey" json:"characteristic_id,string"`
	UUID        string  `gorm:"unique_index" json:"-"`
	PartID      uint    `gorm:"-" json:"part_id,string"`
	PartUUID    string  `json:"-"`
	Name        string  `json:"characteristic_name"`
	NameEn      string  `json:"name_en"`
	Value       float32 `json:"value,string"`
	Formula     string  `json:"formula"`
	Remark      string  `json:"remark"`
}
type Property struct {
	Model_Equip `gorm:"embedded"`
	ID          uint   `gorm:"primarykey" json:"characteristic_id,string"`
	UUID        string `gorm:"unique_index" json:"-"`
	// PartID      uint    `gorm:"-" json:"part_id,string"`
	PartUUID  string  `json:"-"`
	PointUUID string  `json:"-"`
	Name      string  `json:"characteristic_name"`
	NameEn    string  `json:"name_en"`
	Value     float32 `json:"value,string"`
	Formula   string  `json:"formula"`
	Remark    string  `json:"remark"`
}

func (u *Property) BeforeCreate(tx *gorm.DB) error {
	u.UUID = uuid.NewString()
	return nil
}

// func (u *Property) AfterFind(tx *gorm.DB) error {
// 	err := tx.Table("part").Where("uuid=?", u.PartUUID).Pluck("id", &u.PartID).Error
// 	if err != nil {
// 		u.PartID = 0
// 		return err
// 	}
// 	return nil
// }

type Point struct {
	Model_Equip  `gorm:"embedded"`
	ID           uint         `gorm:"primarykey" json:"point_id,string"`
	UUID         string       `gorm:"unique_index" json:"point_UUID"`
	PartID       uint         `gorm:"-" json:"part_id,string" `
	PartUUID     string       `json:"-"`
	Name         string       `gorm:"not null; comment:测点名称" json:"point_name"`
	TreeVersion  string       `gorm:"tree_version; comment:故障树版本" json:"tree_version" toml:"treeversion"`
	Status       uint8        `gorm:"type:tinyint;default:0; comment:状态" json:"status,string"`
	Data         []Data       `json:"data,omitempty" gorm:"foreignKey:PointUUID;references:UUID"`
	Direction    string       `json:"direction"`                                //TODO 前端需要增加相关字段显示
	LastDataTime time.Time    `json:"-" gorm:"default:2000-01-01 00:00:00.000"` //最后更新数据的时间
	Location     string       `json:"point_nameEn" gorm:"column:location; comment:测点英文位置" toml:"nameEn"`
	Properties   []Property   `json:"characteristic" gorm:"foreignKey:PointUUID;references:UUID" `
	Bands        []alert.Band `json:"band" gorm:"foreignKey:PointUUID;references:UUID"`
}

func (u *Point) BeforeCreate(tx *gorm.DB) error {
	u.UUID = u.PartUUID + "_" + InitialLetter(u.Name)
	var c int64
	tx.Table("point").Where("uuid=?", u.UUID).Count(&c)
	if c != 0 {
		u.UUID = u.UUID + "#" + fmt.Sprint(time.Now().Unix())
	}
	return nil
}
func (u *Point) AfterFind(tx *gorm.DB) error {
	err := tx.Table("part").Where("uuid=?", u.PartUUID).Pluck("id", &u.PartID).Error
	if err != nil {
		u.PartID = 0
		return err
	}
	return nil
}

// 取消测点标注
type PointStd struct {
	Model     `gorm:"embedded"`
	ID        uint   `gorm:"primarykey" json:"id,string"`
	Version   string `json:"-"`
	PartType  string `gorm:"column:part_type" json:"part_type"` //已改：部件名！不是部件类型
	Name      string `json:"point_name"`
	Direction string `json:"direction"` //TODO 前端需要增加相关字段显示
}

// * 故障统计数据
type FanPartLevelAlertReport struct {
	//有故障后实时更新
	AlertCount_1 uint32 `gorm:"type:int unsigned;default:0"` //等级1 正常
	AlertCount   uint32 `gorm:"type:int unsigned;default:0"` //齿轮箱报警数（=2+3）
	AlertCount_2 uint32 `gorm:"type:int unsigned;default:0"` //等级2 注意
	AlertCount_3 uint32 `gorm:"type:int unsigned;default:0"` //等级3 报警
}

// TODO 风场月统计
type WindfarmMonthReport struct {
	Model               `gorm:"embedded"`
	ID                  uint `gorm:"primarykey" json:"id,string"`
	WindfarmUUID        string
	WindfarmID          uint                    `json:"windfield_id,string" gorm:"-"`
	DateTime            time.Time               `gorm:"type:date"`
	TimeSet             int64                   //当前月的时间戳
	Year                uint                    `gorm:"type:smallint unsigned"`
	Month               uint                    `gorm:"type:smallint unsigned"`
	TotalAlertCount     uint32                  `gorm:"type:int unsigned"` //总报警数
	LevelAlertGear      FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:gear_"`
	LevelAlertBearing   FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:bearing_"`
	LevelAlertGenerator FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:generator_"`
	LevelAlertCabin     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:cabin_"`
	LevelAlertTower     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:tower_"`
	LevelAlertBlade     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:blade_"`
}

// TODO 风场日统计
type WindfarmDailyReport struct {
	Model               `gorm:"embedded"`
	ID                  uint `gorm:"primarykey" json:"id,string"`
	WindfarmUUID        string
	WindfarmID          uint                    `json:"windfield_id,string" gorm:"-"`
	DateTime            time.Time               `gorm:"type:date"`
	TimeSet             int64                   //当前月的时间戳
	TotalAlertCount     uint32                  `gorm:"type:int unsigned"` //总报警数
	LevelAlertGear      FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:gear_"`
	LevelAlertBearing   FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:bearing_"`
	LevelAlertGenerator FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:generator_"`
	LevelAlertCabin     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:cabin_"`
	LevelAlertTower     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:tower_"`
	LevelAlertBlade     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:blade_"`
}

// TODO 风机月统计
type MachineMonthReport struct {
	Model               `gorm:"embedded"`
	ID                  uint `gorm:"primarykey" json:"id,string"`
	MachineUUID         string
	WindfarmID          uint                    `json:"windfield_id,string" gorm:"-"`
	MachineID           uint                    `json:"fan_id,string"  gorm:"-"`
	DateTime            time.Time               `gorm:"type:date"`
	TimeSet             int64                   //当前月的时间戳
	Year                uint                    `gorm:"type:smallint unsigned"`
	Month               uint                    `gorm:"type:smallint unsigned"`
	TotalAlertCount     uint32                  `gorm:"type:int unsigned;default:0"` //总报警数
	LevelAlertGear      FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:gear_"`
	LevelAlertBearing   FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:bearing_"`
	LevelAlertGenerator FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:generator_"`
	LevelAlertCabin     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:cabin_"`
	LevelAlertTower     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:tower_"`
	LevelAlertBlade     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:blade_"`
}

// TODO 风机日统计
type MachineDailyReport struct {
	Model               `gorm:"embedded"`
	ID                  uint `gorm:"primarykey" json:"id,string"`
	MachineUUID         string
	WindfarmID          uint                    `json:"windfield_id,string" gorm:"-"`
	MachineID           uint                    `json:"fan_id,string" gorm:"-"`
	DateTime            time.Time               `gorm:"type:date"`
	TimeSet             int64                   //当前日的时间戳
	TotalAlertCount     uint32                  `gorm:"type:int unsigned"` //总报警数
	LevelAlertGear      FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:gear_"`
	LevelAlertBearing   FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:bearing_"`
	LevelAlertGenerator FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:generator_"`
	LevelAlertCabin     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:cabin_"`
	LevelAlertTower     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:tower_"`
	LevelAlertBlade     FanPartLevelAlertReport `gorm:"embedded;embeddedPrefix:blade_"`
}

type Data struct {
	Model         `gorm:"embedded"`
	ID            uint    `gorm:"primarykey" json:"id"`
	PointID       uint    `gorm:"-" json:"-"`
	PointUUID     string  `json:"pointUUID"`
	Datatag       int8    `gorm:"type:tinyint;default:1" json:"datatag"`       //不压缩 赋值为1
	Length        string  `json:"length"`                                      //长度
	SampleFreq    int     `json:"sample_freq"`                                 //采样频率
	Datatype      string  `json:"datatype"`                                    //波形类型。TIMEWAVE/LONGTIMEWAVE/TACH
	Measuredefine string  `json:"define"`                                      //测量参数描述
	Filepath      string  `json:"file_name"`                                   //数据文件名
	Rpm           float32 `json:"rpm"`                                         //转速
	Time          string  `json:"time" gorm:"-"`                               //采样时间
	TimeSet       int64   `json:"-"`                                           //格式转换
	Wavesave      string  `json:"wavesave"`                                    //备用字段 无需赋值
	Status        uint8   `gorm:"type:tinyint;default:1" json:"status,string"` //数据状态 //TODO 数据导入分析后修改
	BandValue1    string  `json:"bv1"`                                         //预留：频带值1。格式：最小值 最大值
	BandValue2    string  `json:"bv2"`                                         //预留：频带值2
	BandValue3    string  `json:"bv3"`                                         //预留：频带值3
	BandValue4    string  `json:"bv4"`                                         //预留：频带值4
	BandValue5    string  `json:"bv5"`                                         //预留：频带值5
	BandValue6    string  `json:"bv6"`                                         //预留：频带值6
	Power         float32 `json:"power"`                                       //功率
	WindSpeed     float32 `json:"windspeed"`                                   //风速
	Yew           float32 `json:"yew"`                                         //偏航
	Pitch1        float32 `json:"pitch1"`                                      //浆角1
	Pitch2        float32 `json:"pitch2"`                                      //浆角2
	Pitch3        float32 `json:"pitch3"`                                      //浆角3
	Tag           string  `json:"tag"`                                         //故障标签
	Result        `json:"result" gorm:"embedded"`
	Wave          Wave    `json:"-" gorm:"foreignKey:DataUUID;references:UUID"`
	Alert         []Alert `json:"-" gorm:"foreignKey:DataUUID;references:UUID"`
}

func (u *Data) AfterFind(tx *gorm.DB) error {
	err := tx.Table("point").Where("uuid=?", u.PointUUID).Pluck("id", &u.PointID).Error
	if err != nil {
		u.PointID = 0
		return err
	}
	return nil
}

type Result struct {
	Rmsvalue  float32 `json:"rmsvalue" gorm:"column:rmsvalue;comment:有效值"`      //有效值
	Indexkur  float32 `json:"indexkur" gorm:"column:indexkur;comment:峭度指标"`     //峭度指标
	Indexi    float32 `json:"indexi" gorm:"column:indexi;comment:脉冲指标"`         //脉冲指标
	Indexk    float32 `json:"indexk" gorm:"column:indexk;comment:波形指标"`         //波形指标
	Indexl    float32 `json:"indexl" gorm:"column:indexl;comment:裕度指标"`         //裕度指标
	Indexsk   float32 `json:"indexsk" gorm:"column:indexsk;comment:歪度指标"`       //歪度指标
	Indexc    float32 `json:"indexc" gorm:"column:indexc;comment:峰值指标"`         //峰值指标
	Indexxr   float32 `json:"indexxr" gorm:"column:indexxr;comment:方根赋值指标"`     //方根赋值指标`          //方根赋值
	Indexmax  float32 `json:"indexmax" gorm:"column:indexmax;comment:最大值指标"`    //最大值
	Indexmin  float32 `json:"indexmin" gorm:"column:indexmin;comment:最小值指标"`    //最小值`                                      //最小值
	Indexmean float32 `json:"indexmean" gorm:"column:indexmean;comment:平均值指标"`  //平均值`                                     //均值
	Indexeven float32 `json:"indexeven" gorm:"column:indexeven;comment:平均赋值指标"` //平均赋值指标`                                       //平均赋值
	Indexp    float32 `json:"indexp" gorm:"column:indexp;comment:峰值"`           //峰值`                                           //峰值
	Indexpp   float32 `json:"indexpp" gorm:"column:indexpp;comment:峰峰值"`        //峰峰值
	Brms1     float32 `json:"brms1"`                                            //预留：频带值1的有效值
	Brms2     float32 `json:"brms2"`                                            //预留：频带值2的有效值
	Brms3     float32 `json:"brms3"`                                            //预留：频带值3的有效值
	Brms4     float32 `json:"brms4"`                                            //预留：频带值4的有效值
	Brms5     float32 `json:"brms5"`                                            //预留：频带值5的有效值
	Brms6     float32 `json:"brms6"`                                            //预留：频带值6的有效值
	TypiFeature
}

// wave的uuid与data的uuid相同 一对一
type Wave struct {
	ID            uint   `gorm:"primarykey" json:"id,string"`
	DataUUID      string `json:"-"`
	DataString    string `json:"-" gorm:"-"`
	File          []byte `json:"file" gorm:"-"` //原文件
	DataFloat     []byte `json:"data"`          //采样幅值 时序图
	SpectrumFloat []byte `json:"spectrum"`      //频谱幅值 频谱图
	//包络
	EnvelopSet            string `json:"envelop_set "`      //包络设置
	SpectrumEnvelopeFloat []byte `json:"spectrum_envelope"` //包络频谱
}

type Alert struct {
	Model            `gorm:"embedded"`
	ID               uint              `gorm:"primarykey" json:"id,string"`
	DataID           uint              `json:"data_id,string" gorm:"-"`
	DataUUID         string            `gorm:"type:char(36);" json:"-"`
	PointID          uint              `json:"point_id,string" gorm:"-"` //使用pid而不是wid，避免数据删除后无法定位到测点
	PointUUID        string            `gorm:"unique_index" json:"-"`
	Point            string            `json:"point" gorm:"-"`
	Factory          string            `json:"company" gorm:"-"`   //公司名
	Windfarm         string            `json:"windfield" gorm:"-"` //风场名
	Machine          string            `json:"fan" gorm:"-"`       //风机名
	Location         string            `json:"location"`           //部件
	PartType         string            `json:"-" gorm:"-"`
	Time             string            `json:"time" gorm:"-"` //时间
	Level            uint8             `gorm:"type:tinyint;comment:报警等级" json:"level"`
	Type             string            `gorm:"comment:报警类型" json:"type" `     //报警类型 故障树、频道幅值···// TODO 可自定义增加
	Strategy         string            `gorm:"comment:报警策略" json:"strategy" ` //策略描述 如有效值报警
	Desc             string            `gorm:"comment:报警描述" json:"desc"`      //报警描述
	TimeSet          int64             `gorm:"comment:报警描述" json:"-"`         //格式转换 //^ 数据的时间
	Rpm              float32           `gorm:"rpm;comment:报警描述" json:"rpm" `
	BandAlert        alert.BandAlert   `json:"-" gorm:"foreignKey:AlertUUID;references:UUID"`
	TreeAlert        alert.TreeAlert   `json:"-" gorm:"foreignKey:AlertUUID;references:UUID"`
	ManualAlert      alert.ManualAlert `json:"-" gorm:"foreignKey:AlertUUID;references:UUID"`
	Code             string            `gorm:"comment:报警类型代码" json:"code"`                                                 //预留 告警类型代码
	Faulttype        int               `json:"faulttype" gorm:"column:faulttype; comment:故障标识"`                            //预留 故障标识
	Source           uint8             `json:"source" gorm:"column:source; comment:报警来源"`                                  //0：自动 1：人工
	Confirm          int               `json:"confirm" gorm:"column:confirm; default:1; comment:报警确认,0:无故障 1:与报警一致 2:待观察"` //TODO 0:无故障 1:与报警一致 2:待观察
	Suggest          string            `json:"suggest" gorm:"column:suggest; comment:处理建议"`                                //TODO 增加处理建议 可编辑 显示在右下角
	CheckTime        string            `json:"checkTime" gorm:"column:check_time; comment:检查时间"`                           //TODO 增加处理时间
	Handle           uint8             `gorm:"type:tinyint" json:"handle"`                                                 //0为红色表示未处理，1为绿色表示已处理。
	Broadcast        uint8             `gorm:"type:tinyint" json:"broadcast"`                                              //是否通知给了前端 0/1
	BroadcastMessage string            `gorm:"-" json:"message"`                                                           //是否通知给了前端 0/1

}

func (u *Alert) AfterFind(tx *gorm.DB) error {
	if u.PointUUID != "" {
		err := tx.Table("point").Where("uuid=?", u.PointUUID).Pluck("id", &u.PointID).Error
		if err != nil {
			u.PointID = 0
			return err
		}
		ids, _, _, err := PointtoFactory(tx, u.PointID)
		if err != nil {
			u.DataID = 0
		}
		if tx.Migrator().HasTable("data_" + ids[2]) {
			err = tx.Table("data_"+ids[2]).Where("uuid=?", u.DataUUID).Pluck("id", &u.DataID).Error
			if err != nil {
				u.DataID = 0
			}
		}
	}
	return nil
}

type AlertInfo struct {
	SearchBox string   `json:"search_box"` //下拉菜单字段
	Options   []string `json:"options"`    //下拉菜单选项
}

// TODO
type Datainfo struct {
	ID            uint    `gorm:"primarykey" json:"id,string"`
	PointID       uint    `json:"point_id,string"`
	PointUUID     string  `json:"point_uuid"`
	Time          string  `json:"time"`
	TimeSet       int64   `json:"-"`
	Measuredefine string  `json:"define"`
	Rpm           float32 `json:"rpm"`
	Status        uint8   `gorm:"type:tinyint;default:1" json:"status,string"`
}

func (u *Datainfo) AfterFind(tx *gorm.DB) error {
	err := tx.Table("point").Where("uuid=?", u.PointUUID).Pluck("id", &u.PointID).Error
	if err != nil {
		u.PointID = 0
		return err
	}
	return nil
}

type PointInfo struct {
	FactoryName  string `json:"company_name"`
	WindfarmName string `json:"windfield_name"`
	MachineName  string `json:"fan_name"`
	PartName     string `json:"part_name"`
	PointName    string `json:"point_name"`
}

type ReturnData struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// 筛选条件
type LimitCondition struct {
	MinRpm        float32 `json:"min_rpm" query:"min_rpm"`
	MaxRpm        float32 `json:"max_rpm" query:"max_rpm"`
	Starttime     string  `json:"start_time" query:"start_time"`       //开始时间
	Endtime       string  `json:"end_time" query:"end_time"`           //结束时间
	Datatype      string  `json:"datatype query:"datatype"`            //数据类型
	Measuredefine string  `json:"measuredefine" query:"measuredefine"` //测量方法
	Tag           string  `json:"tag" query:"tag"`
	Freq          string  `json:"freq" query:"freq"`
}

type Limit struct {
	LimitCondition
	Factory  string `json:"company" query:"company"`     //公司
	Windfarm string `json:"windfield" query:"windfield"` //风场
	Machine  string `json:"fan" query:"fan"`             //风机
	Location string `json:"location" query:"location"`   //部件
	Level    uint8  `json:"level,string" query:"level"`  //级别
	Type     string `json:"type" query:"type"`           //告警类型
	Strategy string `json:"strategy" query:"strategy"`   //策略描述
	Source   *int   `json:"source,omitempty"`            //来源
}

type SinglePlot struct {
	Legend string    `json:"legend"` //图例
	Xaxis  []float32 `json:"x"`
	Yaxis  []float32 `json:"y"`
	Xunit  string    `json:"x_unit"`
	Yunit  string    `json:"y_unit"`
}

// 单测点分析绘图
type AnalysetoPlot struct {
	Plots []SinglePlot `json:"analyse"`
}

// 单测点数据绘图
type DatatoPlot struct {
	Originplot  SinglePlot  `json:"origin"`
	Resultplot  SinglePlot  `json:"result"`
	Currentplot CurrentPlot `json:"current"`
	Data        Data        `json:"data"`
}

// 对比图
type MultiDatatoPlot struct {
	Currentplot []CurrentPlot `json:"current" query:"current"`
}

type CurrentPlot struct {
	PointId string    `json:"point_id"`
	Limit   Limit     `json:"limit" gorm:"-"`
	Legend  string    `json:"legend"`
	IDaxis  []string  `json:"id," gorm:"-"`
	Xaxis   []string  `json:"x" gorm:"-"`
	Yaxis   []float32 `json:"y" gorm:"-"`
	Xunit   string    `json:"x_unit"`
	Yunit   string    `json:"y_unit"`
}

type Temp struct {
	Model    `gorm:"embedded" json:"-"`
	ID       uint `gorm:"primarykey" json:"id,string"`
	Data     []byte
	Complete bool `gorm:"default:0"`
}

//* 用于数据库迁移的结构体

type Data_Update struct {
	Model         `gorm:"embedded" `
	ID            uint    `gorm:"primary" json:"id,string"`
	PointID       uint    `gorm:"-" json:"point_id,string"`
	PointUUID     string  `gorm:"unique_index" json:"-"`
	Datatag       int8    `gorm:"type:tinyint;default:1" json:"datatag"`       //不压缩 赋值为1
	Length        string  `json:"length"`                                      //长度
	SampleFreq    int     `json:"sample_freq"`                                 //采样频率
	Datatype      string  `json:"datatype"`                                    //波形类型。TIMEWAVE/LONGTIMEWAVE/TACH
	Measuredefine string  `json:"define"`                                      //测量参数描述
	Filepath      string  `json:"file_name"`                                   //数据文件名
	Rpm           float32 `json:"rpm"`                                         //转速
	Time          string  `json:"time" gorm:"-"`                               //采样时间
	TimeSet       int64   `json:"-"`                                           //格式转换
	Wavesave      string  `json:"wavesave"`                                    //备用字段 无需赋值
	Status        uint8   `gorm:"type:tinyint;default:1" json:"status,string"` //数据状态 //TODO 数据导入分析后修改
	BandValue1    string  `json:"bv1"`                                         //预留：频带值1。格式：最小值,最大值
	BandValue2    string  `json:"bv2"`                                         //预留：频带值2
	BandValue3    string  `json:"bv3"`                                         //预留：频带值3
	BandValue4    string  `json:"bv4"`                                         //预留：频带值4
	BandValue5    string  `json:"bv5"`                                         //预留：频带值5
	BandValue6    string  `json:"bv6"`                                         //预留：频带值6
	Power         float32 `json:"power"`
	WindSpeed     float32 `json:"windspped"`
	Yew           float32 `json:"yew"`
	Pitch1        float32 `json:"pitch1"`
	Pitch2        float32 `json:"pitch2"`
	Pitch3        float32 `json:"pitch3"`
	Tag           string  `json:"tag"`
	Result        `json:"result" gorm:"embedded"`
	TypiFeature
}
type Wave_Update struct {
	ID            uint   `gorm:"primarykey" json:"id,string"`
	DataUUID      string `gorm:"type:char(36);unique_index" json:"-"`
	File          []byte `json:"file" gorm:"-"`
	DataFloat     []byte `json:"-" gorm:"data"`     //采样幅值 时序图
	SpectrumFloat []byte `json:"-" gorm:"spectrum"` //频谱幅值 频谱图
	//包络
	EnvelopSet            string `json:"-"`                           //包络设置
	SpectrumEnvelopeFloat []byte `json:"-" gorm:"spectrumE_envelope"` //包络频谱
}

type WorkCondition struct {
	WindSp1s    string `json:"WindSp1S"`    //风速1s
	ActivePower string `json:"ActivePower"` //功率
	RotorSpeed  string `json:"RotorSpeed"`  //电机转速
	PitchAngle  string `json:"PitchAngle"`  //桨叶角度
	YawState    string `json:"YawState"`    //偏航
}
type SubdivisionAlarm struct {
}
type Alarm struct {
	ComponentName        string             `json:"ComponentName"`        //ComponentName 部件名称
	AlarmType            int64              `json:"AlarmType"`            //AlarmType 报警类型
	AlarmUpdateTime      string             `json:"AlarmUpdateTime"`      //AlarmUpdateTime 报警时间
	SubdivisionAlarmList []SubdivisionAlarm `json:"SubdivisionAlarmList"` //细分故障列表
	AlarmDegree          int64              `json:"AlarmDegree"`          //AlarmDegree 故障报警程度
}

type ChannelContent struct {
	ECSChannel         int     `json:"ECSChannel"`         //通道编号
	WaveLength         int     `json:"WaveLength"`         //波形长度
	SignalType         int     `json:"SignalType"`         //信号类型
	WaveType           int     `json:"WaveType"`           //波形类型
	UpperFreq          float32 `json:"UpperFreq"`          //波形上限频率
	LowerFreq          float32 `json:"LowerFreq"`          //波形下限频率
	ComponentName      string  `json:"ComponentName"`      //检测部件简称
	LocationSection    string  `json:"LocationSection"`    //检测部件测点位置
	SampleRate         float32 `json:"SampleRate"`         //采样率
	WaveDefDescription int     `json:"WaveDefDescription"` //波形定义描述
	AcquisitionTime    string  `json:"AcquisitionTime"`    //波形采样时间
	WaveData           string  `json:"WaveData"`           //波形数据
	ChannelAlarmType   string  `json:"ChannelAlarmType"`   //通道报警类型
	Eigenvalue         Result  `json:"Eigenvalue"`         //通道特征值
}

type CMSData struct {
	DeviceId           string           `json:"DeviceID"`           //设备编号，编号确定后不能更改
	DeviceType         string           `json:"DeviceType"`         //设备名称，可以修改
	DeviceIP           string           `json:"DeviceIP"`           //设备通信地址/IP地址
	DeviceStatus       string           `json:"DeviceStatus"`       //设备状态
	StopLevel          int64            `json:"StopLevel"`          //CMS设备给的报警等级
	AlarmInfo          []Alarm          `json:"AlarmInfo"`          //报警信息
	ChannelContentList []ChannelContent `json:"ChannelContentList"` //通道信息
}
type FactoryUpdateData struct {
	TurbineName   string                 `json:"TurbineName"` //风机名,按风机名称排序
	StopLevel     string                 `json:"StopLevel"`   //预警停机等级
	DeviceTime    string                 `json:"DeviceTime"`  //系统时间/设备时间
	WorkCondition `json:"WorkCondition"` //运行工况
	CMSData       `json:"CMSData"`       //CMS数据
}

var faultMap = map[int]string{
	0: "有限制门限报警",
	1: "峰值门限报警",
}

// @Title InsertFactoryData
// @Description alarminfolist 为部件报警、 channelalarm为测点报警
// @Author MuXi 2023-12-15 14:45:23
// @Param db 实例数据库
// @Param farmIdStr 风场id
// @Param turbineIdStr 风机id
// @Return data
// @Return err
// FIXME 2023/12/15 需要将数据写入数据库在算法调用前完成， 算法调用产生异常，直接抛出
func (factoryData *FactoryUpdateData) InsertFactoryData(db *gorm.DB, farmIdStr, turbineIdStr string) (data Data, err error) {
	farmId, err := strconv.Atoi(farmIdStr)
	if err != nil {
		err = errors.New("farmId, 转换错误")
		return
	}

	turbineId, err := strconv.Atoi(turbineIdStr)
	if err != nil {
		err = errors.New("turbineId, 转换错误")
		return
	}

	var farm Windfarm
	var machine Machine
	if err = db.Table("windfarm").Where("id = ? ", farmId).Find(&farm).Error; err != nil {
		err = errors.New("查询风场信息错误")
		return
	}

	if err = db.Table("machine").Where("id = ? ", turbineId).Find(&machine).Error; err != nil {
		err = errors.New("查询风机信息错误")
		return
	}

	tx := db.Begin()
	//处理数据表单
	power, _ := strconv.ParseFloat(factoryData.ActivePower, 32)
	windSpeed, _ := strconv.ParseFloat(factoryData.WindSp1s, 32)
	yew, _ := strconv.ParseFloat(factoryData.YawState, 32)
	rotorSpeed, _ := strconv.ParseFloat(factoryData.RotorSpeed, 32)
	pitch, _ := strconv.ParseFloat(factoryData.PitchAngle, 32)

	data = Data{
		Power:     float32(power),
		Yew:       float32(yew),
		Pitch1:    float32(pitch),
		WindSpeed: float32(windSpeed),
		Rpm:       float32(rotorSpeed),
	}

	alarmInfoList := factoryData.AlarmInfo
	// 部件报警信息不为空，
	for _, alarmInfo := range alarmInfoList {
		//部件报警信息不为空时，更新对应的部件status， 是否需要插入到报警表中？
		var machineUUID string
		tx.Table("machine").Select("uuid").Where("id = ?", turbineId).Find(&machineUUID)
		if err = tx.Table("part").Where("type_en = ? and machine_uuid = ?", alarmInfo.ComponentName, machineUUID).
			Update("status", alarmInfo.AlarmDegree).Error; err != nil {
			err = errors.New("更新部件状态错误")
			tx.Rollback()
			return
		}
	}

	// 测点信息
	channelContentList := factoryData.CMSData.ChannelContentList
	for _, channelContent := range channelContentList {
		//接收通道数据
		data.Length = strconv.Itoa(channelContent.WaveLength)
		switch channelContent.SignalType {
		case 0:
			data.Measuredefine = "加速度"
		case 1:
			data.Measuredefine = "速度"
		case 2:
			data.Measuredefine = "位移"
		}

		switch channelContent.WaveType {
		case 0:
			data.Datatype = "TIMEWAVE"
		case 1:
			data.Datatype = "LONGTIMEWAVE"
		case 2:
			data.Datatype = "TACH"
		}
		//componetName、locationSection、同时查询出测点uuid 填入data中
		var point Point
		if err = tx.Table("point").Joins("LEFT JOIN part on point.part_uuid = part.uuid").
			Where("part.type_en = ? AND point.location = ?", channelContent.ComponentName, channelContent.LocationSection).
			Find(&point).Error; err != nil {
			err = errors.New("查询测点信息错误")
			tx.Rollback()
			return
		}
		data.PointUUID = point.UUID
		data.PointID = point.ID
		data.Time = channelContent.AcquisitionTime
		data.TimeSet, _ = StrtoTime("2006-01-02 15:04:05", channelContent.AcquisitionTime)
		data.SampleFreq = int(channelContent.SampleRate)

		data.Result = channelContent.Eigenvalue
		err = tx.Table("data_" + turbineIdStr).Omit("Wave").Create(&data).Error

		//波形数据不等于空时, 通信协议规定，数据为base64加密，首先进行解密，在进行数据操作。
		if channelContent.WaveData != "" {
			encodingString := channelContent.WaveData
			decodedBytes, _ := base64.StdEncoding.DecodeString(encodingString)
			data.Wave.DataFloat = decodedBytes
			data.Wave.DataUUID = data.UUID
		}

		err = data.DataAnalysis_2(db, "localhost:3006", turbineIdStr)
		if err != nil {
			err = errors.New("数据分析失败")
		}
		// 防止前步插入数据失败
		if data.ID == 0 {
			err = tx.Table("data_" + turbineIdStr).Create(&data).Error
			if err != nil {
				err = errors.New("数据插入失败")
				return
			}
		} else {
			err = tx.Table("data_" + turbineIdStr).Save(&data).Error
		}
		// 波形数据不等于空时，插入波形数据
		if len(data.Wave.DataFloat) != 0 || len(data.Wave.SpectrumFloat) != 0 || len(data.Wave.SpectrumEnvelopeFloat) != 0 {
			data.Wave.DataUUID = data.UUID
			tx.Table("wave_"+turbineIdStr).Where("data_uuid=?", data.UUID).Select("id").Scan(&data.Wave)
			if data.Wave.ID == 0 {
				err = tx.Table("wave_" + turbineIdStr).Create(&data.Wave).Error
				if err != nil {
					err = errors.New("数据插入失败")
					return
				}
			} else {
				err = tx.Table("wave_" + turbineIdStr).Omit("created_at").Clauses(clause.Locking{Strength: "UPDATE"}).Save(&data.Wave).Error
				if err != nil {
					err = errors.New("数据更新失败")
					return
				}
			}
		}
		//更新 风机最新数据时间
		var ptime time.Time
		err = tx.Table("point").Where("id=?", data.PointID).Pluck("last_data_time", &ptime).Error
		if err != nil {
			err = errors.New("查询最新数据时间失败")
			return
		}
		if ptime.Unix() < data.TimeSet {
			err = tx.Table("point").Where("id=?", data.PointID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("last_data_time", data.Time).Error
			if err != nil {
				err = errors.New("更新最新数据时间失败")
				return
			}
		}
		if ptime.Unix() < data.TimeSet {
			err = tx.Table("point").Where("id=?", data.PointID).Update("last_data_time", data.Time).Error
			if err != nil {
				err = errors.New("更新最新数据时间失败")
				tx.Rollback()
				return
			}
		}
		//通道报警类型不为空的话, 通道报警个数不一定为一个，需要分割字符串后进行比对
		if channelContent.ChannelAlarmType != "" {
			splitStr := strings.Split(channelContent.ChannelAlarmType, ",")
			for _, valueStr := range splitStr {
				if valueStr != "" {
					value, _ := strconv.Atoi(valueStr)
					faultName := faultMap[value]
					aler := Alert{
						DataUUID:  data.UUID,
						PointUUID: data.PointUUID,
						Location:  point.Name,
						Level:     3,
						Strategy:  "通道报警",
						Desc:      faultName,
						TimeSet:   data.TimeSet,
						Rpm:       data.Rpm,
						Source:    0,
						Suggest:   "检修",
					}
					id := CheckTagExist(tx, point.UUID, faultName)
					tx.Table("data_"+turbineIdStr).Where("uuid =?", data.UUID).Update("alert_id", id)
					data.Alert = append(data.Alert, aler)
				}
			}
		}
		//构建算法请求体
		requestBody := AlgorithmReqBody{
			WindfarmName: farm.Name,
			MachineName:  machine.Name,
			PointName:    point.Name,
			Data:         data.Wave.DataString,
			SampleTime:   time.Unix(data.TimeSet, 0).Format("2006_01_02_15:04"),
			SampleRate:   strconv.Itoa(data.SampleFreq) + "Hz",
			Rpm:          strconv.FormatFloat(float64(data.Rpm), 'f', 6, 64) + "rpm",
		}
		//根据测点uuid查询相关算法
		var algorithms []Algorithm
		if err = tx.Table("algorithm").Where("point_uuid = ? AND enabled = true", point.UUID).Find(&algorithms).Error; err != nil {
			err = errors.New("查询算法信息错误")
			tx.Rollback()
			return
		}
		//使用resty包发送算法请求，根据算法类型使用不同的响应体接收返回值，存入对应的数据库
		client := resty.New()
		for _, algorithm := range algorithms {
			switch algorithm.Type {
			case "A":
				var responseBody AlgorithmRepBodyA
				resp, err2 := client.R().SetHeader("Content-Type", "application/json").SetBody(requestBody).SetResult(&responseBody).Post(algorithm.Url)
				if err2 != nil {
					err = errors.New("算法请求失败。err:" + err.Error())
					return
				} else {
					if resp.StatusCode() != 200 {
						err = errors.New("算法请求失败。err:" + resp.Status())
						return
					}
					if responseBody.Success == "True" && responseBody.Error == "0" {
						//将responseBody中的TypiFeatureSource转换成responseBody.TypiFeature
						for index, value := range responseBody.TypiFeatureSource {
							switch index {
							case 0:
								responseBody.TypiFeature.MeanFre = float64(value)
							case 1:
								responseBody.TypiFeature.SquareFre = float64(value)
							case 2:
								responseBody.TypiFeature.GravFre = float64(value)
							case 3:
								responseBody.TypiFeature.SecGravFre = float64(value)
							case 4:
								responseBody.TypiFeature.GravRatio = float64(value)
							case 5:
								responseBody.TypiFeature.StandDeviate = float64(value)
							}
						}
						data.TypiFeature = responseBody.TypiFeature
						if err = tx.Table("data_" + turbineIdStr).Save(&data).Error; err != nil {
							err = errors.New("数据更新失败")
							tx.Rollback()
							return
						}
						// 将结果转换后插入到数据库中
						algorithmResultA := AlgorithmResultA{
							DataUUID:       data.UUID,
							AlgorithmID:    algorithm.Id,
							FTendencyFloat: responseBody.FTendency.Translate(),
							TTendencyFloat: responseBody.TTendency.Translate(),
							TypiFeature:    responseBody.TypiFeature,
							CreateTime:     GetCurrentTime(),
							UpdateTime:     GetCurrentTime(),
						}
						//插入结果表GetCurrentTime
						if err = tx.Table("algorithm_result_a").Create(&algorithmResultA).Error; err != nil {
							err = errors.New("插入算法结果失败")
							tx.Rollback()
							return
						}
					} else if responseBody.Success == "False" && responseBody.Error == "0" {
						err = errors.New("算法运行异常")
						tx.Rollback()
						return
					} else {
						switch responseBody.Error {
						case "1":
							err = errors.New("风场名错误")
						case "2":
							err = errors.New("风机号错误")
						case "3":
							err = errors.New("测点名错误")
						case "4":
							err = errors.New("数据长度错误")
						case "5":
							err = errors.New("采样频率错误")
						case "6":
							err = errors.New("风速错误 ")
						}
					}
				}
			case "B":
				var responseBody AlgorithmRepBodyB
				resp, err3 := client.R().SetHeader("Content-Type", "application/json").SetBody(requestBody).SetResult(&responseBody).Post(algorithm.Url)
				if err3 != nil {
					err = errors.New("算法请求失败。err:" + err.Error())
					return
				}
				if resp.StatusCode() != 200 {
					err = errors.New("算法请求失败。err:" + resp.Status())
					return
				}

				if responseBody.Success == "True" && responseBody.Error == "0" {
					//故障诊断结果和概率插入报警表
					algorithmResultB := AlgorithmResultB{
						DataUUID:    data.UUID,
						AlgorithmID: algorithm.Id,
						DataDTO:     responseBody.Data.Translate(),
						CreateTime:  GetCurrentTime(),
						UpdateTime:  GetCurrentTime(),
					}
					//插入结果表
					if err = tx.Table("algorithm_result_b").Create(&algorithmResultB).Error; err != nil {
						err = errors.New("故障诊断结果插入失败")
						tx.Rollback()
						return
					}
					if responseBody.Data.FaultName != "" {
						//插入报警表
						aler := Alert{
							DataUUID:  data.UUID,
							PointUUID: data.PointUUID,
							Location:  point.Name,
							Level:     3,
							Strategy:  "预警算法",
							Desc:      responseBody.Data.FaultName,
							TimeSet:   data.TimeSet,
							Rpm:       data.Rpm,
							Source:    2,
							Suggest:   "检修",
						}
						id := CheckTagExist(tx, point.UUID, responseBody.Data.FaultName)
						tx.Table("data_"+turbineIdStr).Where("uuid =?", data.UUID).Update("tag", id)
						data.Alert = append(data.Alert, aler)
					}

				} else if responseBody.Success == "False" && responseBody.Error == "0" {
					err = errors.New("算法运行异常")
					tx.Rollback()
					return
				} else {
					switch responseBody.Error {
					case "1":
						err = errors.New("风场名错误")
					case "2":
						err = errors.New("风机号错误")
					case "3":
						err = errors.New("测点名错误")
					case "4":
						err = errors.New("数据长度错误")
					case "5":
						err = errors.New("采样频率错误")
					case "6":
						err = errors.New("风速错误 ")
					}
				}
			}
		}

	}

	if len(data.Alert) > 0 {
		if err = tx.Table("alert").Create(&data.Alert).Error; err != nil {
			tx.Rollback()
			err = errors.New("数据通道报警插入失败")
			return
		}
	}
	tx.Commit()
	return
}

type AlgorithmStatistic struct {
	Name     string  `json:"name" gorm:"name"`         //预警算法名
	Counts   int64   `json:"counts" gorm:"counts"`     //预警次数
	Accuracy float32 `json:"accuracy" gorm:"accuracy"` //准确率
}

type Algorithm struct {
	Id         int64  `json:"id" gorm:"id"`
	Name       string `json:"name" gorm:"name; comment:预警算法名"`
	Url        string `json:"url" gorm:"column:url; comment:预警算法url"`
	PointUUID  string `json:"point_uuid" gorm:"point_uuid"`
	Type       string `json:"type" gorm:"column:type;comment:算法类型"`
	Enabled    bool   `json:"enabled" gorm:"column:enabled;comment:是否启用;default:1"`
	BuiltTime  string `json:"builtTime" gorm:"column:built_time; comment:投运时间"`
	CreateTime string `json:"createTime" gorm:"column:create_time; comment:创建时间"`
	UpdateTime string `json:"updateTime" gorm:"column:update_time; comment:更新时间"`
	IsDel      bool   `json:"isDel" gorm:"column:is_del;default:0"`
}

func (Algorithm) TableName() string {
	return "algorithm"
}

func (f *FTendencyString) Translate() (res FTendencyFloat) {
	res.FLevel1, _ = strconv.ParseFloat(f.FLevel1, 64)
	res.FLevel2, _ = strconv.ParseFloat(f.FLevel2, 64)
	res.FScore, _ = strconv.ParseFloat(f.FScore, 64)
	return
}
func (f *TTendencyString) Translate() (res TTendencyFloat) {
	res.TLevel1, _ = strconv.ParseFloat(f.TLevel1, 64)
	res.TLevel2, _ = strconv.ParseFloat(f.TLevel2, 64)
	res.TScore, _ = strconv.ParseFloat(f.TScore, 64)
	return
}

type AlgorithmResultB struct {
	Id          int64  `json:"id" gorm:"id"`
	DataUUID    string `json:"dataUUID" gorm:"data_uuid"`
	AlgorithmID int64  `json:"algorithmId" gorm:"algorithm_id"`
	DataDTO
	CreateTime string `json:"createTime" gorm:"column:create_time; comment:创建时间"`
	UpdateTime string `json:"updateTime" gorm:"column:update_time; comment:更新时间"`
	IsDel      bool   `json:"isDel" gorm:"is_del"`
}

func (*AlgorithmResultB) TableName() string {
	return "algorithm_result_b"
}

type AlgorithmVo struct {
	List  []Algorithm `json:"list"`
	Total int64       `json:"total"`
}

// 算法调用请求体
type AlgorithmReqBody struct {
	WindfarmName string `json:"风场名称" gorm:"column:windfarmName"`
	MachineName  string `json:"风机号" gorm:"column:machineName"`
	PointName    string `json:"测点" gorm:"column:pointName"`
	Data         string `json:"数据"`
	SampleTime   string `json:"时间"`
	SampleRate   string `json:"采样频率"`
	Rpm          string `json:"转速"`
}

func (b *AlgorithmReqBody) ToString() {
	fmt.Println(b.MachineName)
	fmt.Println(b.WindfarmName)
	fmt.Println(b.PointName)
	fmt.Println(b.SampleTime)
	fmt.Println(b.SampleRate)
	fmt.Println(b.Rpm)
	fmt.Println(b.Data)
}

type DataRes struct {
	FaultName string `json:"fault_name" gorm:"column:fault_name"`
	XYZString
	Probability string `json:"probability" gorm:"column:probability"`
}
type DataDTO struct {
	FaultName string `json:"fault_name" gorm:"column:fault_name"`
	XYZFloat
	Probability string `json:"probability" gorm:"column:probability"`
}

type XYZString struct {
	X string `json:"x"`
	Y string `json:"y"`
	Z string `json:"z"`
}
type XYZFloat struct {
	X float64 `json:"x" gorm:"column:x"`
	Y float64 `json:"y" gorm:"column:y"`
	Z float64 `json:"z" gorm:"column:z"`
}

type TimePlot struct {
	TLev1  float64   `json:"lev1"`
	TLev2  float64   `json:"lev2"`
	TScore []float64 `json:"score"`
	XAxis  []string  `json:"x_axis"`
}
type FrequencyPlot struct {
	FLev1  float64   `json:"lev1"`
	FLev2  float64   `json:"lev2"`
	FScore []float64 `json:"score"`
	XAxis  []string  `json:"x_axis"`
}

type TypiFeaturePlot struct {
	MeanFre      []float64 `json:"meanfre" `      //均值频率
	SquareFre    []float64 `json:"squarefre"`     //频谱均方根值
	GravFre      []float64 `json:"gravfre" `      //频谱重心
	SecGravFre   []float64 `json:"secgravfre"`    //二阶重心
	GravRatio    []float64 `json:"gravratio" `    //重心比
	StandDeviate []float64 `json:"standdeviate" ` //标准偏差`
}

type EigenValuePlot struct {
	TypiFeature TypiFeaturePlot `json:"typiFeature"`
	XAxis       []string        `json:"x_axis"`
}

// A类算法画图
type AlgorithmPlotA struct {
	TimePlot       TimePlot       `json:"time"`
	FrequencyPlot  FrequencyPlot  `json:"frequency"`
	EigenValuePlot EigenValuePlot `json:"eigenValue"`
}

type TDimension struct {
	X []float64 `json:"x"`
	Y []float64 `json:"y"`
	Z []float64 `json:"z"`
}

// B类算法画图
type AlgorithmPlotB struct {
	Coordinates TDimension `json:"tdimension"`
	FaultName   []string   `json:"faultName"`
}

type AlgorithmResultA struct {
	Id          int64  `json:"id" gorm:"id"`
	DataUUID    string `json:"dataUUID" gorm:"data_uuid"`
	AlgorithmID int64  `json:"algorithmId" gorm:"algorithm_id"`
	FTendencyFloat
	TTendencyFloat
	TypiFeature
	CreateTime string `json:"createTime" gorm:"column:create_time; comment:创建时间"`
	UpdateTime string `json:"updateTime" gorm:"column:update_time; comment:更新时间"`
	IsDel      bool   `json:"isDel" gorm:"is_del"`
}

func (*AlgorithmResultA) TableName() string {
	return "algorithm_result_a"
}

// 频域残差值
type FTendencyFloat struct {
	FLevel1 float64 `json:"F_lev1" gorm:"column:f_lev1"`
	FLevel2 float64 `json:"F_lev2" gorm:"column:f_lev2"`
	FScore  float64 `json:"F_score" gorm:"column:f_score"`
}

// 时域残差值
type TTendencyFloat struct {
	TLevel1 float64 `json:"T_lev1" gorm:"column:t_lev1"`
	TLevel2 float64 `json:"T_lev2" gorm:"column:t_lev2"`
	TScore  float64 `json:"T_score" gorm:"column:t_score"`
}

// 敏感特征值
type TypiFeature struct {
	MeanFre      float64 `json:"meanfre" gorm:"column:mean_fre"`           //均值频率
	SquareFre    float64 `json:"squarefre" gorm:"column:square_fre"`       //频谱均方根值
	GravFre      float64 `json:"gravfre" gorm:"column:grav_fre"`           //频谱重心
	SecGravFre   float64 `json:"secgravfre" gorm:"column:sec_grav_fre"`    //二阶重心
	GravRatio    float64 `json:"gravratio" gorm:"column:grav_ratio"`       //重心比
	StandDeviate float64 `json:"standdeviate" gorm:"column:stand_deviate"` //标准偏差`                       //标准偏差频率
}

// A类算法调用响应体
type AlgorithmRepBodyA struct {
	Success           string          `json:"success"`
	FTendency         FTendencyString `json:"F_tendency"`
	TTendency         TTendencyString `json:"T_tendency"`
	TypiFeatureSource []float32       `json:"typi_feature"`
	TypiFeature       TypiFeature     `json:"-"`
	Error             string          `json:"error"`
}

type AlgorithmRepBodyB struct {
	Success string  `json:"success"`
	Data    DataRes `json:"data"`
	Error   string  `json:"error"`
}

func (d *DataRes) Translate() (res DataDTO) {
	res.FaultName = d.FaultName
	res.X, _ = strconv.ParseFloat(d.X, 64)
	res.Y, _ = strconv.ParseFloat(d.Y, 64)
	res.Z, _ = strconv.ParseFloat(d.Z, 64)
	res.Probability = d.Probability
	return
}

// 频域残差值
type FTendencyString struct {
	FLevel1 string `json:"lev_F1" gorm:"column:f_lev1"`
	FLevel2 string `json:"lev_F2" gorm:"column:f_lev2"`
	FScore  string `json:"scoreF" gorm:"column:f_score"`
}

// 时域残差值
type TTendencyString struct {
	TLevel1 string `json:"lev_T1" gorm:"column:t_lev1"`
	TLevel2 string `json:"lev_T2" gorm:"column:t_lev2"`
	TScore  string `json:"scoreT" gorm:"column:t_score"`
}

type Parsing struct {
	Id         int64  `json:"id" gorm:"primarykey"`
	Name       string `json:"name" gorm:"name; comment:解析方式名"`
	DataInfo   string `json:"dataInfo" gorm:"column:data_info; comment:数据信息信息格式"`
	Separator  string `json:"separator" gorm:"column:separator; comment:分隔符"`
	Length     int    `json:"length" gorm:"column:length; comment:长度"`
	Type       int    `json:"type" gorm:"column:type; comment:类型"`
	CreateTime string `json:"createTime" gorm:"column:create_time; comment:创建时间"`
	UpdateTime string `json:"updateTime" gorm:"column:update_time; comment:更新时间"`
	IsDel      bool   `json:"isDel" gorm:"is_del; default:0"`
}

func (*Parsing) TableName() string {
	return "parsing"
}

type ParsingRESP struct {
	List  []Parsing `json:"list"`
	Total int64     `json:"total"`
}

type FaultTag struct {
	Id     int    `json:"id" gorm:"primarykey"`
	Num    int    `json:"num" gorm:"column:num; comment:序号"`
	Type   string `json:"type" gorm:"column:type; comment:类型"`
	Name   string `json:"name" gorm:"column:name; comment:故障"`
	Source bool   `json:"source" gorm:"column:source; comment:来源, 0表示故障反馈, 1表示报警说明"`
	IsDel  bool   `json:"isDel" gorm:"is_del"`
}

func (*FaultTag) TableName() string {
	return "fault_tag"
}

type FaultTagVo struct {
	List  []FaultTag `json:"list"`
	Total int64      `json:"total"`
}

// 上传数据解析数据， 匹配info所用到的字段
type DataInfo struct {
	Windfarm   string //风场名
	Machine    string //风机名
	Point      string //测点名
	Length     string //数据长度
	SampleRate string //采样频率
	DataType   string //测量类型
	Parameter  string //测量参数
	Rpm        string //转速
	Time       string //时间
	Other      string //其他参数
}

type FaultBack struct {
	Id             int64   `json:"id" gorm:"primarykey"`
	FaultStartTime string  `json:"faultStartTime" gorm:"-"`
	FaultEndTime   string  `json:"faultEndTime" gorm:"-"`
	StartTimeSet   int64   `json:"-" gorm:"column:start_time_set; comment:开始时间戳"`
	EndTimeSet     int64   `json:"-" gorm:"column:end_time_set; comment:结束时间戳"`
	Source         int     `json:"source" gorm:"column:source; comment:来源; default:2"`
	MachineUUID    string  `json:"machineUUID" gorm:"column:machine_uuid; comment:风机uuid"`
	PartUUID       string  `json:"partUUID" gorm:"column:part_uuid; comment:部件uuid"`
	PointUUID      string  `json:"pointUUID" gorm:"column:point_uuid; comment:测点uuid"`
	Tag            string  `json:"tag" gorm:"column:tag; comment:故障标签"`
	Progress       float64 `json:"progress" gorm:"column:progress; comment:进度"`
	Suggest        string  `json:"suggest" gorm:"column:suggest; comment:建议"`
	CheckTime      string  `json:"checkTime" gorm:"column:check_time; comment:检查时间"`
	RepairTime     string  `json:"repairTime" gorm:"column:repair_time; comment:维修时间"`
	RepairPart     string  `json:"repairPart" gorm:"column:repair_part; comment:维修部件"`
	FileId         int     `json:"fileId" gorm:"column:file_id"`
	File           string  `json:"file" gorm:"-"`
	Status         int     `json:"status" gorm:"column:status; comment:状态"`
	CreateTime     string  `json:"createTime" gorm:"column:create_time"`
	UpdateTime     string  `json:"updateTime" gorm:"column:update_time"`
	IsDel          bool    `json:"isDel" gorm:"is_del"`
}

type FaultBackUpdate struct {
	Id         int64   `json:"id" gorm:"primarykey"`
	CheckTime  string  `json:"checkTime" gorm:"column:check_time; comment:检查时间"`
	RepairTime string  `json:"repairTime" gorm:"column:repair_time; comment:维修时间"`
	RepairPart string  `json:"repairPart" gorm:"column:repair_part; comment:维修部件"`
	FileId     int     `json:"fileId" gorm:"column:file_id"`
	Progress   float64 `json:"progress" gorm:"column:progress; comment:进度"`
	Suggest    string  `json:"suggest" gorm:"column:suggest; comment:建议"`
	UpdateTime string  `json:"updateTime" gorm:"column:update_time"`
}

func (*FaultBack) TableName() string {
	return "fault_back"
}

// 故障记录详情，包含报警表和故障反馈
type FaultBackInfo struct {
	Id             int64   `json:"id" gorm:"primarykey"`
	FaultTime      string  `json:"faultTime" gorm:"-"`
	FaultStartTime string  `json:"faultStartTime" gorm:"-"`
	FaultEndTime   string  `json:"faultEndTime" gorm:"-"`
	TimeSet        int64   `json:"-" gorm:"column:start_time_set"`
	EndTimeSet     int64   `json:"-" gorm:"column:end_time_set"`
	Source         int     `json:"source" gorm:"column:source"`
	MachineUUID    string  `json:"machineUUID" gorm:"column:machine_uuid"`
	PartUUID       string  `json:"partUUID" gorm:"column:part_uuid"`
	Tag            string  `json:"tag" gorm:"column:tag"`
	Progress       float64 `json:"progress" gorm:"column:progress; comment:进度"`
	Suggest        string  `json:"suggest" gorm:"column:suggest; comment:建议"`
	CheckTime      string  `json:"checkTime" gorm:"column:check_time; comment:检查时间"`
	RepairTime     string  `json:"repairTime" gorm:"column:repair_time; comment:维修时间"`
	RepairPart     string  `json:"repairPart" gorm:"column:repair_part; comment:维修部件"`
	File           string  `json:"file" gorm:"column:fileName"`
	FileDir        string  `json:"fileDir" gorm:"column:fileDir"`
	Level          int     `json:"level" gorm:"column:status; comment:状态"`
}

type FaultBackRESP struct {
	Id           int    `json:"id" gorm:"column:id"`
	FaultTime    string `json:"faultTime" gorm:"-"`                    // 时间
	StartTimeSet int64  `json:"-" gorm:"column:timeSet"`               // 时间戳
	EndTimeSet   int64  `json:"-" gorm:"column:endTimeSet"`            // 时间戳
	Source       int    `json:"source" gorm:"column:source"`           // 来源 0：自动报警； 1：手动报警； 2：故障反馈
	TurbineName  string `json:"turbineName" gorm:"column:turbineName"` //风机名称
	Level        int    `json:"level" gorm:"column:level"`             // 故障等级
	Location     string `json:"location" gorm:"column:location"`       // 位置 自动报警、手动报警展示测点名称，故障反馈展示部件名称
	Desc         string `json:"desc" gorm:"column:desc"`               //自动报警、手动报警展示报警描述，故障反馈展示故障标签
}

type FaultBackVo struct {
	List  []FaultBackRESP `json:"list"`
	Total int64           `json:"total"`
}

type File struct {
	Id         int64  `json:"id" gorm:"primarykey"`
	Name       string `json:"name" gorm:"column:name; comment:文件名"`
	MD5Name    string `json:"md5Name" gorm:"column:md5_name"`
	Dir        string `json:"dir" gorm:"column:dir; comment:文件路径"`
	CreateTime string `json:"createTime" gorm:"column:create_time"`
	UpdateTime string `json:"updateTime" gorm:"column:update_time"`
	IsDel      bool   `json:"isDel" gorm:"column:is_del; default:0"`
}

func (*File) TableName() string {
	return "file"
}

// 测点趋势图
type Models struct {
	Id     int64  `json:"id" gorm:"primarykey"`
	Name   string `json:"name" gorm:"column:name; comment:模型名"`
	NameEn string `json:"nameEn" gorm:"column:nameEn; comment:模型英文名"`
	IsDel  bool   `json:"isDel" gorm:"column:is_del; default:0"`
}
type ModelsVo struct {
	List  []Models `json:"list"`
	Total int64    `json:"total"`
}

func (*Models) TableName() string {
	return "model"
}
