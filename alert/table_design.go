package alert

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// 从alertstd标准文件读取
type Alert struct {
	Version string
	Band    []Band
}

type Model struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

//频带报警的标准表 写到数据库
type Band_2 struct {
	Model     `gorm:"embedded" json:"-"`
	ID        uint   `json:"-"`
	UUID      string `gorm:"type:char(36);unique_index" json:"-"`
	PartUUID  string `json:"-"`
	PointUUID string `json:"-"`
	// Version  string    `json:"-"`
	Name     string    `json:"part_type" toml:"type" gorm:"column:type"` //TODO 报警列表显示part_name
	Type     string    `gorm:"-"`                                        // TODO
	Value    string    `json:"value"`
	Property string    `json:"property"`
	Range    string    `json:"range"`
	FloorStd float32   `json:"floor" gorm:"-"`
	UpperStd float32   `json:"upper" gorm:"-"`
	Floor    BandStage `gorm:"embedded;embeddedPrefix:floor_"`
	Upper    BandStage `gorm:"embedded;embeddedPrefix:upper_"`
}
type Band struct {
	Model     `gorm:"embedded" json:"-"`
	ID        uint   `json:"id,string"`
	UUID      string `gorm:"type:char(36);unique_index" json:"-"`
	PartUUID  string `json:"-"`
	PointUUID string `json:"-"`
	// Version  string    `json:"-"`
	PartType string    `json:"part_type" gorm:"-"` //TODO 报警列表显示part_name
	Value    string    `json:"value"`
	Property string    `json:"property"`
	Range    string    `json:"range" gorm:"column:band_range"`
	FloorStd float32   `json:"floor" gorm:"-"`
	UpperStd float32   `json:"upper" gorm:"-"`
	RPMFloor float32   `json:"rpm_floor" gorm:"rpm_floor" toml:"rpm_floor"`
	RPMUpper float32   `json:"rpm_upper" gorm:"rpm_upper" toml:"rpm_upper"`
	Floor    BandStage `gorm:"embedded;embeddedPrefix:floor_" json:"floor_stage"`
	Upper    BandStage `gorm:"embedded;embeddedPrefix:upper_" json:"upper_stage"`
}
type BandStage struct {
	Level   uint8   `json:"level"`
	Std     float32 `json:"std"`
	Desc    string  `json:"desc"`
	Suggest string  `json:"suggest"`
}

func (u *Band) BeforeCreate(tx *gorm.DB) error {
	var err error
	u.UUID = uuid.NewString()
	if err != nil {
		return err
	}
	return nil
}
func (u *Band) AfterFind(tx *gorm.DB) error {
	u.FloorStd = u.Floor.Std
	u.UpperStd = u.Upper.Std
	if u.PointUUID != "" {
		tx.Table("part").Joins("join point on part.uuid = point.part_uuid").
			Where("point.uuid = ?", u.PointUUID).
			Pluck("part.name", &u.PartType)
	}
	if u.PartUUID != "" {
		tx.Table("part").Where("uuid = ?", u.PartUUID).Pluck("name", &u.PartType)
	}
	return nil
}

//报警频带报警 详细信息 返回给前端
type BandAlert struct {
	ID         uint    `json:"id" gorm:"primarykey" ` //对应alert表的id
	AlertID    uint    `json:"alert_id,string" gorm:"-"`
	AlertUUID  string  `json:"-" gorm:"unique_index"`
	FileName   string  `json:"file_name" gorm:"-"` //相关文件
	Alarmvalue float32 `json:"alarmvalue,string"`  //报警值
	Range      string  `json:"range" `             //报警频带
	Limit      float32 `json:"limit,string"`       //报警限值
}

func (u *BandAlert) AfterFind(tx *gorm.DB) error {
	err := tx.Table("alert").Where("uuid=?", u.AlertUUID).Pluck("id", &u.AlertID).Error
	if err != nil {
		u.AlertID = 0
		return err
	}
	return nil
}

//故障树信息\
type BasicTree struct {
	Version string `json:"-"`
	Name    string `json:"-"`
	Type    string `json:"-"`
}
type Tree struct {
	// Model `gorm:"embedded" json:"-"`
	// ID       uint            `json:"id,string"`
	Layer    int                    `json:"-"` //最大层数
	Version  string                 `json:"-"`
	DataTime int64                  `json:"-"` //报警数据的时间戳
	Rpm      float32                `json:"-"`
	Name     string                 `json:"-"`
	Type     string                 `json:"-"`
	Index    string                 `json:"-"` //报警策略，即特征值
	Stages   []Stage                `json:"stages"`
	Nodes    []Node                 `json:"nodes"` // Nodes   []Node
	NodesMap map[int][]*Node        `json:"-"`     //[层数]节点
	ValueMap map[string]interface{} `json:"-"`
	Desc     string                 `json:"-"`
	Suggest  string                 `json:"-"`
}

type Node struct {
	ID        int       `json:"-"`
	Name      string    `json:"name"` //与前端界面上的节点名称
	Leaves    []int     `json:"-"`    //该节点的枝节点，每一分支最底层节点的枝节点为[-1]
	Layer     int       `json:"-"`
	Calculate Calculate `json:"-"`
	TrueValue []float32 `json:"true_value"`
	Result    bool      `json:"result"`
	Message   bool      `json:"message"` //若为true只是显示信息，不参与标红
	Children  []Node    `json:"children"`
}
type Stage struct {
	Name      string      `json:"name"`
	Calculate []Calculate `json:"-"`
	TrueValue []float32   `json:"true_value"`
	Result    bool        `json:"result"`
	Desc      string      `json:"-"`
	Suggest   string      `json:"-"`
}

type Calculate struct {
	Value1    float32
	Goal1     string //properties 对应幅值
	Cal       string
	Value2    float32
	Goal2     string //rpm
	Method    string
	Standard  float32
	Lower     float32
	LowerGoal string
	Upper     float32
	UpperGoal string
}

//报警故障树 写到数据库
type TreeAlert struct {
	ID           uint   `json:"id,string" gorm:"primarykey"`
	AlertID      uint   `json:"alert_id,string" gorm:"-"`
	AlertUUID    string `json:"-" gorm:"unique_index"`
	TreeName     string `json:"tree_name"`          //有阶段显示阶段。故障树名
	FileTreeName string `json:"file_tree_name"`     //相关文件
	FileName     string `json:"file_name" gorm:"-"` //相关数据
	Tree         Tree   `json:"tree" gorm:"-"`
	TreeJson     []byte `json:"-"`
}

func (u *TreeAlert) AfterFind(tx *gorm.DB) error {
	err := tx.Table("alert").Where("uuid=?", u.AlertUUID).Pluck("id", &u.AlertID).Error
	if err != nil {
		u.AlertID = 0
		return err
	}
	return nil
}

//人工报警详细内容
type ManualAlert struct {
	ID        uint   `json:"id,string" gorm:"primarykey"`
	AlertID   uint   `json:"alert_id,string" gorm:"-"`
	AlertUUID string `json:"-" gorm:"unique_index"`
	FileName  string `json:"file_name" gorm:"-"` //相关数据
	//图片
}

func (u *ManualAlert) AfterFind(tx *gorm.DB) error {
	err := tx.Table("alert").Where("uuid=?", u.AlertUUID).Pluck("id", &u.AlertID).Error
	if err != nil {
		u.AlertID = 0
		return err
	}
	return nil
}
