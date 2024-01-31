package mod

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

func MaptoStruct(mm interface{}, m interface{}) {
	mtemp, _ := json.Marshal(mm)
	json.Unmarshal(mtemp, m)
}

// 转时间戳
func StrtoTime(format string, timestr string) (t int64, err error) {
	time, err := time.ParseInLocation(format, timestr, time.Local)
	t = time.Unix()
	return t, err
}

// 时间戳转换成字符串
func TimetoStrFormat(format string, t int64) string {
	return time.Unix(t, 0).Format(format)
}

func GetCurrentTime() string {
	return time.Now().Local().Format("2006-01-02 15:04:05")
}

func GetCurrentTimeHHMM() string {
	return time.Now().Local().Format("2006-01")
}

func TimetoStr(t int64) time.Time {
	return time.Unix(t, 0)
}

// ^ 根据测点id获取测点、风机、风场、公司id和name
func PointtoFactory(db *gorm.DB, pid interface{}) (pmwid []string, pmwname []string, pmwuuid []string, err error) {
	type iduuid struct {
		ID   string `gorm:"column:id"`
		UUID string `gorm:"column:uuid"`
		Name string `gorm:"column:name"`
	}
	var temp iduuid
	if err = db.Table("point").Where("id =? ", pid).Scan(&temp).Error; err != nil {
		return
	}
	pmwid = append(pmwid, temp.ID)
	pmwuuid = append(pmwuuid, temp.UUID)
	pmwname = append(pmwname, temp.Name)

	if err = db.Table("point").Joins("left join part on part.uuid = point.part_uuid").
		Where("point.id =? ", temp.ID).
		Select("part.id as id", "part.uuid as uuid", "part.name as name").Scan(&temp).Error; err != nil {
		return
	}
	pmwid = append(pmwid, temp.ID)
	pmwuuid = append(pmwuuid, temp.UUID)
	pmwname = append(pmwname, temp.Name)

	if err = db.Table("part").Joins("left join machine on machine.uuid = part.machine_uuid").
		Where("part.id =? ", temp.ID).
		Select("machine.id as id", "machine.uuid as uuid", "machine.desc as name").
		Scan(&temp).Error; err != nil {
		return
	}
	pmwid = append(pmwid, temp.ID)
	pmwuuid = append(pmwuuid, temp.UUID)
	pmwname = append(pmwname, temp.Name)

	if err = db.Table("machine").Joins("left join windfarm on windfarm.uuid = machine.windfarm_uuid").
		Where("machine.id =? ", temp.ID).
		Select("windfarm.id as id", "windfarm.uuid as uuid", "windfarm.name as name").Scan(&temp).Error; err != nil {
		return
	}
	pmwid = append(pmwid, temp.ID)
	pmwuuid = append(pmwuuid, temp.UUID)
	pmwname = append(pmwname, temp.Name)

	if err = db.Table("windfarm").Joins("left join factory on factory.uuid = windfarm.factory_uuid").
		Where("windfarm.id =? ", temp.ID).
		Select("factory.id as id", "factory.uuid as uuid", "factory.name as name").Scan(&temp).Error; err != nil {
		return
	}
	pmwid = append(pmwid, temp.ID)
	pmwuuid = append(pmwuuid, temp.UUID)
	pmwname = append(pmwname, temp.Name)

	return
}

// ^ 获得upper包含的测点的uuid!!
func UppertoPoint(db *gorm.DB, upper string, id string) []string {
	var pointuuids []string
	db.Table("factory").
		Joins("join windfarm on factory.uuid = windfarm.factory_uuid").
		Joins("join machine on windfarm.uuid = machine.windfarm_uuid").
		Joins("join part on machine.uuid = part.machine_uuid").
		Joins("join point on part.uuid = point.part_uuid").
		Where(upper+".id = ?", id).
		Pluck("point.uuid", &pointuuids)
	return pointuuids
	//var relationMap map[int]string = map[int]string{
	//	4: "factory",
	//	3: "windfarm",
	//	2: "machine",
	//	1: "part",
	//	0: "point",
	//}
	//var key int
	//for k := range relationMap {
	//	if relationMap[k] == upper {
	//		key = k
	//	}
	//}
	//for i := key; i > 0; i-- {
	//	var tempid []uint
	//	db.Table(relationMap[i-1]).Where(relationMap[i]+"_id IN ?", pid).Select("id").Scan(&tempid)
	//	pid = tempid
	//}
	//return pid
}

// 检查tag是否存在，存在返回tag的id，不存在插入数据库后返回tag的id
func CheckTagExist(tx *gorm.DB, pointUUID, desc string) (tag FaultTagSecond) {
	// 开始判断DESC在fault_tag中是否存在，如果存在，拼接id字符串，如果不存在，则加入到fault_tag，在拼接id字符串
	// 首先根据测点找到部件的类型
	var partType string
	// 查询部件类型
	tx.Table("point").Select("part.type").Joins("left join part on part.uuid = point.part_uuid").Where("point.uuid = ?", pointUUID).Find(&partType)
	// 首先根据部件类型寻找一级标签的id传入tag中
	tx.Model(&FaultTagFirst{}).Select("id").Where("name = '自动报警' and type = ?", partType).Find(&tag.FaultTagFirstID)
	// 然后根据一级标签的id和二级标签的name寻找二级标签
	tag.Name = desc
	tag.Source = true
	tx.Model(&tag).Where("upper = ? AND name = ?", tag.FaultTagFirstID, desc).FirstOrCreate(&tag)
	return
}

func IntArrayToString(db *gorm.DB, arr []int) string {
	arr = removeDuplicates(arr)
	strArr := make([]string, len(arr))

	for i, v := range arr {
		// 从二级标签表获取tag
		var tag FaultTagSecond
		db.Table("fault_tag_second").Where("id = ?", v).Find(&tag)
		strArr[i] = fmt.Sprintf("%d-%d", tag.FaultTagFirstID, v)
	}
	result := strings.Join(strArr, ",")
	return result
}

func removeDuplicates(nums []int) []int {
	uniqueMap := make(map[int]bool)
	var result []int

	for _, num := range nums {
		if !uniqueMap[num] {
			uniqueMap[num] = true
			result = append(result, num)
		}
	}

	return result
}
