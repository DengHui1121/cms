package mod

import (
	"errors"
	"fmt"
	"main/alert"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//频带报警。

var ResultIndex map[string]string = map[string]string{
	"有效值":  "rmsvalue",
	"峭度指标": "indexkur",
	"脉冲指标": "indexi",
	"波形指标": "indexk",
	"裕度指标": "indexl",
	"歪度指标": "indexsk",
	"峰值指标": "indexc",
	"方根赋值": "indexxr",
	"最大值":  "indexmax",
	"最小值":  "indexmin",
	"均值":   "indexmean",
	"平均赋值": "indexeven",
}

// TODO 2023/12/14 新增将故障说明插入到故障标签中，以便于后续的数据标签查询、以及删除
func BandAlertSet_2(db *gorm.DB, pdata Data, ppmwcid []string, fid string) (level uint, ids []int, err error) {
	var bband []alert.Band
	tagIds := make([]int, 0)
	if bband, err = AlertSearch(db, ppmwcid, pdata.PointUUID, pdata.Measuredefine); err != nil {
		return
	}
	var maxlevel uint = 1
	err = db.Transaction(func(tx *gorm.DB) error {
		for k, v := range bband {
			//判断转速
			if v.RPMFloor <= pdata.Rpm {
				if v.RPMUpper == 0 || (v.RPMUpper > pdata.Rpm) {
					if v.Property == "有效值" && v.Range != "" {
						var a Alert
						var b alert.BandAlert
						kk := strconv.Itoa(k + 1)
						var brms float32
						tx.Table("data_"+fid).Where("id=?", pdata.ID).Select("brms" + kk).Scan(&brms)
						a.DataUUID = pdata.UUID
						a.PointUUID = pdata.PointUUID
						a.TimeSet = pdata.TimeSet
						a.Location = v.PartType
						//判断有效值是否在范围内
						if brms >= v.Floor.Std {
							a.Rpm = pdata.Rpm
							a.Strategy = v.Property
							a.Type = "频带幅值"
							a.Level = v.Floor.Level
							a.Desc = v.Floor.Desc
							tagIds = append(tagIds, CheckTagExist(tx, pdata.PointUUID, a.Desc).Id)
							a.Suggest = v.Floor.Suggest
							b.Limit = v.FloorStd
							if brms >= v.Upper.Std {
								a.Level = v.Upper.Level
								a.Desc = v.Upper.Desc
								a.Suggest = v.Upper.Suggest
								b.Limit = v.UpperStd
							}
							err := tx.Table("alert").Omit(clause.Associations).Create(&a).Error
							if err != nil {
								return err
							}
							b.AlertID = a.ID
							b.AlertUUID = a.UUID
							b.Alarmvalue = brms
							b.Range = v.Range
							err = tx.Table("band_alert").Create(&b).Error
							if err != nil {
								return err
							}
							if a.Level > uint8(maxlevel) {
								maxlevel = uint(a.Level)
							}
						}
					} else {
						var a Alert
						var b alert.BandAlert
						var brms float32
						tx.Table("data_"+fid).Where("id=?", pdata.ID).Select(ResultIndex[v.Property]).Scan(&brms)
						a.DataUUID = pdata.UUID
						a.PointUUID = pdata.PointUUID
						a.TimeSet = pdata.TimeSet
						a.Location = v.PartType
						//判断有效值是否在范围内
						if brms >= v.Floor.Std {
							a.Rpm = pdata.Rpm
							a.Strategy = v.Property
							a.Type = "频带幅值"
							a.Level = v.Floor.Level
							a.Desc = v.Floor.Desc
							tagIds = append(tagIds, CheckTagExist(tx, pdata.PointUUID, a.Desc).Id)
							a.Suggest = v.Floor.Suggest
							b.Limit = v.FloorStd
							if brms >= v.Upper.Std {
								a.Level = v.Upper.Level
								a.Desc = v.Upper.Desc
								a.Suggest = v.Upper.Suggest
								b.Limit = v.UpperStd
							}
							err := tx.Table("alert").Omit(clause.Associations).Create(&a).Error
							if err != nil {
								return err
							}
							b.AlertID = a.ID
							b.AlertUUID = a.UUID
							b.Alarmvalue = brms
							b.Range = v.Range
							err = tx.Table("band_alert").Create(&b).Error
							if err != nil {
								return err
							}
							if a.Level > uint8(maxlevel) {
								maxlevel = uint(a.Level)
							}
						}
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return maxlevel, nil, err
	}
	return maxlevel, tagIds, nil
}

// TODO 目前至更新了测点的status。再根据测点最大状态值更新风机和风场的状态
// * 先查询是否为该测点的最新数据，是则更新测点状态
func StatusUpdate(db *gorm.DB, times int64, ppmwcid []string, level uint8) error {
	//* 是否为最新
	var point Point
	db.Table("point").Where("id=?", ppmwcid[0]).Select("uuid", "last_data_time").First(&point)
	if times >= point.LastDataTime.Unix() {
		if !TimetoStr(times).Before(time.Now().AddDate(0, 0, -3)) {
			err := db.Transaction(func(tx *gorm.DB) error {
				//报警状态实时更新
				if err := tx.Table("point").Where("id=?", ppmwcid[0]).Clauses(clause.Locking{Strength: "UPDATE"}).Update("status", level).Error; err != nil {
					return err
				}
				_, _, err := StatusCheck(ppmwcid[0], "part", "point", tx)
				if err != nil {
					return err
				}
				_, _, err = StatusCheck(ppmwcid[1], "machine", "part", tx)
				if err != nil {
					return err
				}
				if _, _, err = StatusCheck(ppmwcid[2], "windfarm", "machine", tx); err != nil {
					return err
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// 报警查询的时间转换
func AlertsTimeSet(a interface{}) (*[]Alert, error) {
	value, ok := a.(*[]Alert)
	if !ok {
		return nil, errors.New("it's not ok for type []Alert")
	}
	vv := *value
	for k, v := range vv {
		vv[k].Time = TimetoStr(v.TimeSet).String()
	}
	return &vv, nil
}

// 频带报警详细信息查询至alert.BandAlert
func BandAlertDetail(db *gorm.DB, id string) (balert alert.BandAlert, err error) {
	var a Alert
	if err = db.Table("alert").Preload("BandAlert").Last(&a, id).Error; err != nil {
		return balert, err
	}
	balert = a.BandAlert
	ppmwcid, _, _, err := PointtoFactory(db, a.PointID)
	if err != nil {
		return
	}
	fid := ppmwcid[2]
	if err = db.Table("data_"+fid).Select("filepath").Where("uuid=?", a.DataUUID).Scan(&balert.FileName).Error; err != nil {
		return balert, err
	}
	return balert, nil
}
func ManualAlertDetail(db *gorm.DB, id string) (balert alert.ManualAlert, err error) {
	var a Alert
	if err = db.Table("alert").Preload("ManualAlert").Last(&a, id).Error; err != nil {
		return balert, err
	}
	balert = a.ManualAlert
	ppmwcid, _, _, err := PointtoFactory(db, a.PointID)
	if err != nil {
		return
	}
	fid := ppmwcid[2]
	if err = db.Table("data_"+fid).Select("filepath").Where("uuid=?", a.DataUUID).Scan(&balert.FileName).Error; err != nil {
		return balert, err
	}
	return balert, nil
}

// 检索报警alert表的信息。
func AlertsSearch(db *gorm.DB, m Limit, c echo.Context) (ff []Alert, count int64, err error) {
	//* 先筛选时间条件。
	var stamp1 int64
	var stamp2 int64
	if m.Starttime == "" {
		m.Starttime = "1999-01-01 00:00:00"
	}
	stamp1, err = StrtoTime("2006-01-02 15:04:05", m.Starttime)
	if err != nil {
		return nil, 0, err
	}
	if m.Endtime == "" {
		m.Endtime = "9999-01-01 00:00:00"
	}
	stamp2, err = StrtoTime("2006-01-02 15:04:05", m.Endtime)
	if err != nil {
		return nil, 0, err
	}
	//* 按插入时间 倒序排序
	sub := db.Table("alert").
		Where("time_set BETWEEN ? AND ?", stamp1, stamp2)

	//* 筛选alert字段信息
	if m.Location != "" {
		sub = sub.Where("location=?", m.Location)
	}
	if m.Level != 0 {
		sub = sub.Where("level=?", m.Level)
	}
	if m.Strategy != "" {
		sub = sub.Where("strategy=?", m.Strategy)
	}
	if m.Type != "" {
		sub = sub.Where("type=?", m.Type)
	}
	//TODO debug
	if m.Source != nil {
		sub = sub.Where("source=?", &m.Source)
	}
	//* 筛选设备基本信息
	if m.Machine != "" || m.Windfarm != "" || m.Factory != "" {
		var temppid []string
		if m.Machine != "" {
			temppid = UppertoPoint(db, "machine", m.Machine)
		} else if m.Windfarm != "" {
			temppid = UppertoPoint(db, "windfarm", m.Windfarm)
		} else if m.Factory != "" {
			temppid = UppertoPoint(db, "factory", m.Factory)
		}
		sub = sub.Where("point_uuid IN ?", temppid)
	}
	if m.MaxRpm == 0 {
		m.MaxRpm = 999999
	}
	sub = sub.Where("rpm<=? AND rpm>=?", m.MaxRpm, m.MinRpm).
		Order("time_set desc")

	sub.Where("deleted_at IS NULL").Count(&count)

	//* 映射到结构体
	err = sub.Scopes(Paginate(c.Request())).Find(&ff).Error
	if err != nil {
		return nil, 0, err
	}
	//* 需要查询联表的信息填入
	for k := range ff {
		_, pmwname, _, err := PointtoFactory(db, ff[k].PointID)
		if err != nil {
			return ff, 0, err
		}
		ff[k].Machine = pmwname[2]
		ff[k].Windfarm = pmwname[3]
		ff[k].Factory = pmwname[4]
		ff[k].Time = TimetoStr(ff[k].TimeSet).Format("2006-01-02 15:04:05")
	}
	return ff, count, nil
}

// 下级对应上级下所有下级状态check → 改变上级状态为最高。(上一级，当前级)
// 对应风场下所有风机check 风场的状态修改
func StatusCheck(id interface{}, upper string, current string, db *gorm.DB) (cs []uint, uid string, err error) {
	uTable, upointer := ModelCheck(upper)
	cTable, _ := ModelCheck(current)
	if err = db.Table(cTable).
		Joins(fmt.Sprintf("left join %s on %s.uuid = %s.%s_uuid", upper, upper, cTable, upper)).
		Select(fmt.Sprintf("%s.id", upper)).
		Where(fmt.Sprintf("%s.id=?", cTable), id).Scan(&uid).Error; err != nil {
		return cs, uid, err
	}

	if err = db.Table(uTable).
		Joins(fmt.Sprintf("join %s on %s.uuid = %s.%s_uuid", cTable, uTable, cTable, uTable)).
		Where(uTable+".id=?", uid).Select(cTable + ".status").Scan(&cs).Error; err != nil {
		return cs, uid, err
	}
	_, max := MaxStatus(cs)
	if db.Migrator().HasColumn(upointer, "status") {
		if err = db.Table(uTable).Where("id=?", uid).Clauses(clause.Locking{Strength: "UPDATE"}).Update("status", max).Error; err != nil {
			return cs, uid, err
		}
	}
	return cs, uid, nil
}

// 对应模块
func ModelCheck(desc string) (table string, dst interface{}) {
	switch desc {
	case "company", "factory":
		table = "factory"
		dst = new(Factory)
	case "windfield", "windfarm":
		table = "windfarm"
		dst = new(Windfarm)
	case "fan", "machine":
		table = "machine"
		dst = new(Machine)
	case "measuringPoint", "point":
		table = "point"
		dst = new(Point)
	case "part":
		table = "part"
		dst = new(Part)
	case "":
		table = ""
		dst = nil
	}
	return table, dst
}

// 获取最大状态值
func MaxStatus(l []uint) (key int, max uint) {
	key = 0
	for k := range l {
		if l[key] <= l[k] {
			key = k
			max = l[k]
		}
	}
	return key, max
}
