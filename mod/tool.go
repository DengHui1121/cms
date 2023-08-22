package mod

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

func MaptoStruct(mm interface{}, m interface{}) {
	mtemp, _ := json.Marshal(mm)
	json.Unmarshal(mtemp, m)
}

//转时间戳
func StrtoTime(format string, timestr string) (t int64, err error) {
	time, err := time.ParseInLocation(format, timestr, time.Local)
	t = time.Unix()
	return t, err
}
func TimetoStr(t int64) time.Time {
	return time.Unix(t, 0)
}

//^ 根据测点id获取测点、风机、风场、公司id和name
func PointtoFactory(db *gorm.DB, pid interface{}) (pmwid []string, pmwname []string, pmwuuid []string, err error) {
	type iduuid struct {
		ID   string
		UUID string
		Name string
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

//^ 获得upper包含的测点的uuid!!
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
	// var relationMap map[int]string = map[int]string{
	// 	4: "factory",
	// 	3: "windfarm",
	// 	2: "machine",
	// 	1: "part",
	// 	0: "point",
	// }
	// var key int
	// for k := range relationMap {
	// 	if relationMap[k] == upper {
	// 		key = k
	// 	}
	// }
	// for i := key; i > 0; i-- {
	// 	var tempid []uint
	// 	db.Table(relationMap[i-1]).Where(relationMap[i]+"_id IN ?", pid).Select("id").Scan(&tempid)
	// 	pid = tempid
	// }
	// return pid
}
