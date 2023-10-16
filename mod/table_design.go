package mod

import (
	"fmt"
	"main/alert"
	"strconv"
	"time"

	"github.com/mozillazg/go-pinyin"

	"github.com/google/uuid"
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
	Model_Equip `gorm:"embedded" `
	ID          uint    `gorm:"primarykey" json:"id,string"`
	UUID        string  `gorm:"unique_index" json:"-"`
	FactoryID   uint    `gorm:"-" json:"company_id,string"`
	FactoryUUID string  `gorm:"comment:公司uuid" json:"-"`
	Name        string  `gorm:"not null; comment:风场名" json:"desc"`            //前端显示为风场编码
	Desc        string  `gorm:"not null; comment:风场描述" json:"windfield_name"` //前端显示为风场名称
	Province    string  `gorm:"not null; comment:省" json:"province"`
	City        string  `gorm:"not null; comment:市" json:"city"`
	District    string  `gorm:"not null; comment:地区" json:"district"`
	Longitude   float32 `gorm:"not null; comment:经度" json:"longitude,string"`
	Latitude    float32 `gorm:"not null; comment:纬度" json:"latitude,string"`

	Status   uint8     `gorm:"type:tinyint;default:0" json:"status,string"`
	Machines []Machine `json:"children" gorm:"foreignKey:WindfarmUUID;references:UUID"`
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
	UUID         string `gorm:"unique_index" json:"-"`
	WindfarmID   uint   `gorm:"-" json:"windfield_id,string"`
	WindfarmUUID string `json:"-"`
	Name         string `gorm:"not null; comment:风机名" json:"desc" toml:"name"`
	Type         string `gorm:"not null; comment:风机类型" json:"model"`
	// PointVersion    string  `json:"point_version,omitempty"`
	// PropertyVersion string  `json:"property_version,omitempty"`
	// AlertVersion    string  `json:"alert_version,omitempty"`
	FanVersion      string  `gorm:"comment:风机标准" json:"version"` //风机标准
	TreeVersion     string  `gorm:"comment:设备树版本" json:"tree_version"`
	Unit            string  `json:"unit" toml:"unit"`
	Desc            string  `gorm:"comment:风机描述" json:"fan_name"`
	BuiltTime       string  `gorm:"comment:投运时间" json:"time"`
	Status          uint8   `gorm:"type:tinyint;default:0;comment:状态" json:"status,string"`
	Genfactory      string  `gorm:"comment:发电机厂家" json:"genfactory"`     //发电机厂家
	Gentype         string  `gorm:"comment:发电机型号" json:"gentype"`        //发电机型号
	Gearfactory     string  `gorm:"comment:齿轮箱厂家" json:"gearfactory"`    //齿轮箱厂家
	Geartype        string  `gorm:"comment:齿轮箱型号" json:"geartype"`       //齿轮箱型号
	Mainshaffactory string  `gorm:"comment:主轴厂家" json:"mainshaffactory"` //主轴厂家
	Mainshaftype    string  `gorm:"comment:主轴型号" json:"mainshaftype"`    //主轴型号
	Bladefactory    string  `gorm:"comment:叶片厂家" json:"bladefactory"`    //叶片厂家
	Bladetype       string  `gorm:"comment:叶片型号" json:"bladetype"`       //叶片型号
	Health          float64 `gorm:"-" json:"health"`                     //全生命周期
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
	UUID        string       `gorm:"unique_index" json:"-"`
	MachineID   uint         `gorm:"-" json:"fan_id,string"`
	MachineUUID string       `json:"-" `
	Name        string       `gorm:"not null" json:"part_name" toml:"name"`
	Type        string       `gorm:"not null" json:"part_type" toml:"type"`
	Module      string       `gorm:"default:CMS" json:"module" ` //TODO 所属模块：CMS BMS（叶片） TMS（塔架）
	Points      []Point      `json:"measuring_point" gorm:"foreignKey:PartUUID;references:UUID" `
	Properties  []Property   `json:"characteristic" gorm:"foreignKey:PartUUID;references:UUID"`
	Bands       []alert.Band `json:"band"  gorm:"foreignKey:PartUUID;references:UUID"`
	Status      uint8        `gorm:"type:tinyint;default:0" json:"status,string"`
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
	UUID         string       `gorm:"unique_index" json:"-"`
	PartID       uint         `gorm:"-" json:"part_id,string" `
	PartUUID     string       `json:"-"`
	Name         string       `gorm:"not null" json:"point_name"`
	TreeVersion  string       `gorm:"tree_version" json:"tree_version" toml:"treeversion"`
	Status       uint8        `gorm:"type:tinyint;default:0" json:"status,string"`
	Data         []Data       `json:"data,omitempty" gorm:"foreignKey:PointUUID;references:UUID"`
	Direction    string       `json:"direction"`                                //TODO 前端需要增加相关字段显示
	LastDataTime time.Time    `json:"-" gorm:"default:2000-01-01 00:00:00.000"` //最后更新数据的时间
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
	ID            uint    `gorm:"primarykey" json:"id,string"`
	PointID       uint    `gorm:"-" json:"point_id,string"`
	PointUUID     string  `json:"-"`
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
	FaultTag      int     `json:"faulttag"`                                    //故障标签
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
	Rmsvalue     float32 `json:"rmsvalue"`     //有效值
	Indexkur     float32 `json:"indexkur"`     //峭度指标
	Indexi       float32 `json:"indexi"`       //脉冲指标
	Indexk       float32 `json:"indexk"`       //波形指标
	Indexl       float32 `json:"indexl"`       //裕度指标
	Indexsk      float32 `json:"indexsk"`      //歪度指标
	Indexc       float32 `json:"indexc"`       //峰值指标
	Indexxr      float32 `json:"indexxr"`      //方根赋值
	Indexmax     float32 `json:"indexmax"`     //最大值
	Indexmin     float32 `json:"indexmin"`     //最小值
	Indexmean    float32 `json:"indexmean"`    //均值
	Indexeven    float32 `json:"indexeven"`    //平均赋值
	Indexp       float32 `json:"indexp"`       //峰值
	Indexpp      float32 `json:"indexpp"`      // 峰峰值
	MeanFre      float32 `json:"meanfre"`      //均值频率
	SquareFre    float32 `json:"squarefre"`    //频谱均方根值
	GravFre      float32 `json:"gravfre"`      //频谱重心
	SecGravFre   float32 `json:"secgravfre"`   //二阶重心
	GravRatio    float32 `json:"gravratio"`    //重心比
	StandDeviate float32 `json:"standdeviate"` //标准偏差频率
	Brms1        float32 `json:"brms1"`        //预留：频带值1的有效值
	Brms2        float32 `json:"brms2"`        //预留：频带值2的有效值
	Brms3        float32 `json:"brms3"`        //预留：频带值3的有效值
	Brms4        float32 `json:"brms4"`        //预留：频带值4的有效值
	Brms5        float32 `json:"brms5"`        //预留：频带值5的有效值
	Brms6        float32 `json:"brms6"`        //预留：频带值6的有效值
}

// wave的uuid与data的uuid相同 一对一
type Wave struct {
	ID            uint   `gorm:"primarykey" json:"id,string"`
	DataUUID      string `json:"-"`
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
	Factory          string            `json:"company" gorm:"-"`   //公司名
	Windfarm         string            `json:"windfield" gorm:"-"` //风场名
	Machine          string            `json:"fa" gorm:"-"`        //风机名
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
	Code             string            `gorm:"comment:报警类型代码" json:"code"` //预留 告警类型代码
	Faulttype        int               `json:"faulttype"`                  //预留 故障标识
	Source           uint8             `json:"source"`                     //0：自动 1：人工
	Suggest          string            `json:"suggest"`                    //TODO 增加处理建议 可编辑 显示在右下角
	Progress         string            `json:"progress" gorm:"-"`
	CheckTime        string            `json:"checkTime" gorm:"-"`
	RepairTime       string            `json:"repairTime" gorm:"-"`
	PartName         string            `json:"partName" gorm:"-"`
	File             string            `json:"file" gorm:"-"`
	Handle           uint8             `gorm:"type:tinyint" json:"handle"`    //0为红色表示未处理，1为绿色表示已处理。
	Broadcast        uint8             `gorm:"type:tinyint" json:"broadcast"` //是否通知给了前端 0/1
	BroadcastMessage string            `gorm:"-" json:"message"`              //是否通知给了前端 0/1

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
	ID            uint `gorm:"primarykey" json:"id,string"`
	PointID       uint `json:"point_id,string"`
	PointUUID     string
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
	Result        `json:"result" gorm:"embedded"`
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
	ChannelAlarmType   int     `json:"ChannelAlarmType"`   //通道报警类型
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

func (factoryData FactoryUpdateData) InsertFactoryData(db *gorm.DB, farmIdStr, turbineIdStr string) (data Data, err error) {

	farmId, err := strconv.Atoi(farmIdStr)
	if err != nil {
		return
	}

	turbineId, err := strconv.Atoi(turbineIdStr)
	if err != nil {
		return
	}

	farm := Windfarm{
		ID: uint(farmId),
	}
	machine := Machine{
		ID: uint(turbineId),
	}

	if err = db.Table("windfarm").Where("id = ? ", farmId).Find(&farm).Error; err != nil {
		return
	}

	if err = db.Table("machine").Where("id = ? ", turbineId).Find(&machine).
		Error; err != nil {
		return
	}
	// data和alert相同长度，alert需要data的uuid，首先插入data到数据库中
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
	factoryName := ""
	tx.Table("factory").Where("id = ?", farm.FactoryID).Pluck("name", factoryName)
	// alert := Alert{
	// 	Windfarm: farm.Name,
	// 	Machine:  machine.Name,
	// 	Factory:  factoryName,
	// 	Rpm:      float32(rotorSpeed),
	// }

	channelContentList := factoryData.CMSData.ChannelContentList
	//alarmList := factoryData.AlarmInfo
	signalType, waveType := "", ""
	for i := 0; i < len(channelContentList); i++ {
		switch channelContentList[i].SignalType {
		case 0:
			signalType = "加速度"
		case 1:
			signalType = "速度"
		case 2:
			signalType = "位移"

		}

		switch channelContentList[i].WaveType {
		case 0:
			waveType = "TIMEWAVE"
		case 1:
			waveType = "LONGTIMEWAVE"
		case 2:
			waveType = "TACH"
		}

		//查询测点uuid
		var point Point
		if err = db.Table("point").Where("uuid = ?", channelContentList[i].ECSChannel).Find(&point).Error; err != nil {
			return
		}
		data.Length = strconv.Itoa(channelContentList[i].WaveLength)
		data.SampleFreq = int(channelContentList[i].SampleRate)
		data.Datatype = waveType
		data.Measuredefine = signalType
		data.TimeSet, _ = StrtoTime(time.DateTime, channelContentList[i].AcquisitionTime)
		data.Time = channelContentList[i].AcquisitionTime
		data.Result = channelContentList[i].Eigenvalue
		data.FaultTag = channelContentList[i].ChannelAlarmType
		data.PointUUID = point.UUID
		//插入data数据表
		if err = tx.Table("data_" + turbineIdStr).Create(&data).Error; err != nil {
			tx.Rollback()
			return
		}

		//根据WaveDefDescription的值对原始波形数据进行处理。
		var wave = Wave{
			DataFloat: []byte(channelContentList[i].WaveData),
			DataUUID:  data.UUID,
		}

		/*//将data数据存入wave表之后，再进行数据分析
		switch channelContentList[i].WaveDefDescription {
		case 0:
			//时序波形数据
		case 1:
			//http://localhost:3006/data/trans/0
			//频谱波形数据

		case 3:
			//http://localhost:3006/data/trans/1
			//包络波形数据

		}*/

		if err = tx.Table("wave_" + turbineIdStr).Create(&wave).Error; err != nil {
			tx.Rollback()
			return
		}
		// 处理报警表单，数据上传包含正常数据或者是异常数据
		// 异常数据才进行报警表单插入
		// alert.DataID = data.ID
		// alert.DataUUID = data.UUID
		// alert.PointUUID = data.PointUUID
		// alert.Time = alarmList[i].AlarmUpdateTime
		// alert.TimeSet, _ = StrtoTime(time.DateTime, alarmList[i].AlarmUpdateTime)
		// alert.Level = uint8(alarmList[i].AlarmDegree)
		// alert.Location = alarmList[i].ComponentName
		// if err = tx.Table("alert").Create(&alert).Error; err != nil {
		// 	tx.Rollback()
		// 	return
		// }

	}
	tx.Commit()
	return
}

// 故障反馈记录
type Fault struct {
	ID          uint   `json:"id"`
	FarmUUID    string `gorm:"farm_uuid;comment:风场uuid" json:"farmuuid"`
	FanType     string `gorm:"fan_type;comment:风机类型" json:"fantype"`
	EquipmentId string `gorm:"equipment_id;comment:设备id" json:"equipmentid"`
	FanDesc     string `gorm:"fan_desc;comment:风机描述" json:"fandesc"`
	Source      string `gorm:"source;comment:报警来源" json:"source"`
	Time        string `gorm:"-" json:"time"`
	TimeSet     string `gorm:"time_set;comment:时间戳" json:"timeset"`
	Confirm     int    `gorm:"confirm;comment:报警确认" json:"confirm"`
	FaultName   string `gorm:"fault_name;comment:报警名称" json:"faultname"`
	Mttr        string `gorm:"mttr;comment:维修更换部件名" json:"mttr"`
	Mrrt        string `gomr:"mrrt;comment:维修更换部件时间" json:"mrrt"`
}

type Algorith struct {
	Name     string  `json:"name"`
	Counts   int64   `json:"counts"`
	Accuracy float32 `json:"accuracy"`
}
