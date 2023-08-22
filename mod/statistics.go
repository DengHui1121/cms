package mod

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//

type FaultCount struct {
	PartName string `json:"part_name"`
	Counts   string `json:"counts"`
	Key      string `json:"key"`
}

//TODO 一个种类 多个部件
var partindex = map[string]string{
	"bearing":   "主轴承",
	"gear":      "齿轮箱",
	"generator": "发电机",
	"cabin":     "机舱",
	// "tower":     "塔筒",
	// "blade":     "叶片",
}
var partchindex = map[string]string{
	"主轴承": "bearing",
	"齿轮箱": "gear",
	"发电机": "generator",
	"机舱":  "cabin",
	// "塔筒":  "tower",
	// "叶片":  "blade",
}

var levelindex = map[string]string{
	"0": "无数据",
	"1": "正常",
	"2": "注意",
	"3": "报警",
}

//风机月统计故障次数结果查询。当前时间往前一年的数据
func MonthFaultCounts(db *gorm.DB, fid string, keyword string) (fcs []FaultCount, err error) {
	fcs = []FaultCount{
		{PartName: "bearing", Counts: "0", Key: "1"},
		{PartName: "gear", Counts: "0", Key: "2"},
		{PartName: "generator", Counts: "0", Key: "3"},
		{PartName: "cabin", Counts: "0", Key: "4"},
		// {PartName: "blade", Counts: "0", Key: "5"},
		// {PartName: "tower", Counts: "0", Key: "6"},
	}
	stime := time.Now().AddDate(-1, 0, 0).Unix()
	etime := time.Now().Unix()
	var table string
	var idindex string
	var uuid string
	switch keyword {
	case "fan":
		table = "machine_month_report"
		idindex = "machine_uuid"
		db.Table("machine").Where("id=?", fid).Pluck("uuid", &uuid)
	case "windfield":
		table = "windfarm_month_report"
		idindex = "windfarm_uuid"
		db.Table("windfarm").Where("id=?", fid).Pluck("uuid", &uuid)
	}
	var rmap map[string]interface{}
	var count int64
	if db.Table(table).Where(idindex+" =?", uuid).Where("time_set < ? AND time_set >= ?", etime, stime).Count(&count); count == 0 {
		for k := range fcs {
			fcs[k].PartName = partindex[fcs[k].PartName]
		}
		return
	}
	err = db.Table(table).Where(idindex+" =?", uuid).
		Where("time_set < ? AND time_set >= ?", etime, stime).
		Select("SUM(gear_alert_count) AS gear",
			"SUM(bearing_alert_count) AS bearing",
			"SUM(generator_alert_count) AS generator",
			"SUM(cabin_alert_count) AS cabin",
			"SUM(tower_alert_count) AS tower",
			"SUM(blade_alert_count) AS blade").
		Find(&rmap).Error
	if err != nil {
		for k := range fcs {
			fcs[k].Counts = "Err"
		}
	}
	for k := range fcs {
		if v, ok := rmap[fcs[k].PartName].(string); ok {
			fcs[k].Counts = v
		}
		fcs[k].PartName = partindex[fcs[k].PartName]
	}
	return
}

type FaultBar struct {
	Title string `json:"title"`
	Key   string `json:"key"`
	Data  []Bar  `json:"data"`
}
type Bar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Key   string `json:"key"`
}

//故障统计饼图
func MonthFaultLevel(db *gorm.DB, fid string, keyword string) (fcs []FaultBar, err error) {
	fcs = make([]FaultBar, 0)
	var table string
	switch keyword {
	case "company":
		table = "factory"
	case "windfield":
		table = "windfarm"
	}
	sub := db.Table("factory").
		Joins("right join windfarm on factory.uuid = windfarm.factory_uuid").
		Joins("right join machine on windfarm.uuid = machine.windfarm_uuid").
		Where(table+".id=?", fid)

	//风机状态比例饼图
	var fanpercent FaultBar
	var ex []map[string]interface{}
	db.Table("(?) as tree", sub.Select("machine.id AS machine_id,machine.status AS machine_status")).Group("machine_status").
		Select("machine_status,COUNT(*)").Scan(&ex)
	fanpercent.Title = "风机状态比例饼图"
	fanpercent.Key = "1"
	fanpercent.Data = []Bar{
		{Name: "无数据", Value: "0"},
		{Name: "正常", Value: "0"},
		{Name: "注意", Value: "0"},
		{Name: "报警", Value: "0"},
	}
	for _, v := range ex {
		var s string
		switch v["machine_status"].(int64) {
		case 0:
			s = "无数据"
		case 1:
			s = "正常"
		case 2:
			s = "注意"
		case 3:
			s = "报警"
		}
		fanpercent.Data[v["machine_status"].(int64)] = Bar{Name: s, Value: fmt.Sprint(v["COUNT(*)"].(int64))}
	}
	fcs = append(fcs, fanpercent)
	//注意状态 大部件比例饼图
	var pindex = map[string]int{
		"主轴承": 0,
		"齿轮箱": 1,
		"发电机": 2,
		"机舱":  3,
		// "叶片":  4,
		// "塔筒":  5,
	}
	var warnpercent FaultBar
	ex = make([]map[string]interface{}, 0)
	sub2 := sub.Joins("right join part on machine.uuid = part.machine_uuid").
		Select("part.type AS part_type,part.status AS part_status")
	db.Table("(?) as tree", sub2).Where("part_status=?", 2).
		Group("part_type").
		Select("part_type,COUNT(*)").Scan(&ex)
	warnpercent.Title = "注意状态的部件比例饼图"
	warnpercent.Key = "2"
	warnpercent.Data = []Bar{
		{Name: "主轴承", Value: "0", Key: "1"}, //0
		{Name: "齿轮箱", Value: "0", Key: "2"}, //1
		{Name: "发电机", Value: "0", Key: "3"}, //2
		{Name: "机舱", Value: "0", Key: "4"},  //3
		// {Name: "叶片", Value: "0", Key: "5"},  //4
		// {Name: "塔筒", Value: "0", Key: "6"},  //5
	}
	for _, v := range ex {
		warnpercent.Data[pindex[v["part_type"].(string)]].Value = fmt.Sprint(v["COUNT(*)"].(int64))
	}
	fcs = append(fcs, warnpercent)

	//报警状态 大部件比例饼图
	var alertpercent FaultBar
	ex = make([]map[string]interface{}, 0)
	db.Table("(?) as tree", sub2).Where("part_status=?", 3).
		Group("part_type").
		Select("part_type,COUNT(*)").Scan(&ex)
	alertpercent.Title = "报警状态的部件比例饼图"
	alertpercent.Key = "3"
	alertpercent.Data = []Bar{
		{Name: "主轴承", Value: "0", Key: "1"}, //0
		{Name: "齿轮箱", Value: "0", Key: "2"}, //1
		{Name: "发电机", Value: "0", Key: "3"}, //2
		{Name: "机舱", Value: "0", Key: "4"},  //3
		// {Name: "叶片", Value: "0", Key: "5"},  //4
		// {Name: "塔筒", Value: "0", Key: "6"},  //5
	}
	for _, v := range ex {
		alertpercent.Data[pindex[v["part_type"].(string)]].Value = fmt.Sprint(v["COUNT(*)"].(int64))
	}
	fcs = append(fcs, alertpercent)
	return
}

//各部位的三个故障等级统计饼图
//确认风场id 、月统计图搜索风场月度统计、数量统计计算、返回
func MonthPartFault(db *gorm.DB, id string, keyword string) (fcs []FaultBar, err error) {
	fcs = []FaultBar{
		{Title: "bearing", Key: "1"},
		{Title: "gear", Key: "2"},
		{Title: "generator", Key: "3"},
		{Title: "cabin", Key: "4"},
		// {Title: "blade", Key: "5"},
		// {Title: "tower", Key: "6"},
	}
	var wuuids []string
	switch keyword {
	case "company":
		err = db.Table("factory").
			Joins("join windfarm on factory.uuid = windfarm.factory_uuid").
			Where("factory.id=?", id).
			Pluck("windfarm.uuid", &wuuids).Error

	case "windfield":
		err = db.Table("windfarm").Where("id=?", id).Pluck("uuid", &wuuids).Error
	}
	if err != nil {
		return
	}
	stime := time.Now().AddDate(-1, 0, 0).Unix()
	etime := time.Now().Unix()
	//一年内
	sub := db.Table("windfarm_month_report").
		Where("windfarm_uuid IN ?", wuuids).
		Where("time_set < ? AND time_set >= ?", etime, stime)
	var count int64
	sub.Count(&count)
	//各级计算
	for i := 1; i <= 3; i++ {
		var rmap map[string]interface{}
		err = db.Table("(?) as tree", sub).
			Select(
				fmt.Sprintf("SUM(bearing_alert_count_%v) AS bearing", i),
				fmt.Sprintf("SUM(gear_alert_count_%v) AS gear", i),
				fmt.Sprintf("SUM(generator_alert_count_%v) AS generator", i),
				fmt.Sprintf("SUM(cabin_alert_count_%v) AS cabin", i),
				fmt.Sprintf("SUM(tower_alert_count_%v) AS tower", i)).
			Find(&rmap).Error
		if err != nil {
			return
		}
		for k := range fcs {
			if v, ok := rmap[fcs[k].Title].(string); ok {
				fcs[k].Data = append(fcs[k].Data, Bar{
					Name:  levelindex[fmt.Sprint(i)],
					Value: v,
				})
			} else {
				fcs[k].Data = append(fcs[k].Data, Bar{
					Name:  levelindex[fmt.Sprint(i)],
					Value: "0",
				})
			}
		}
	}
	for k := range fcs {
		fcs[k].Title = partindex[fcs[k].Title]
	}
	return
}

type FaultCurrent struct {
	Legend string    `json:"legend"`
	X      []string  `json:"x" gorm:"-"`
	Y      []float32 `json:"y" gorm:"-"`
}

type PartTrendSum struct {
	Year      string
	Month     string
	Gear      uint32
	Bearing   uint32
	Generator uint32
	Cabin     uint32
	// Blade     uint32
	// Tower     uint32
}

func FaultTrend(db *gorm.DB, id string, keytype string, keyword string) (fcs []FaultCurrent, err error) {
	var wuuids []string

	fcs = []FaultCurrent{
		{Legend: "主轴承"},
		{Legend: "齿轮箱"},
		{Legend: "发电机"},
		{Legend: "机舱"},
		// {Legend: "叶片"},
		// {Legend: "塔筒"},
	}
	switch keyword {
	case "company":
		err = db.Table("factory").
			Joins("join windfarm on factory.uuid = windfarm.factory_uuid").
			Where("factory.id=?", id).
			Pluck("windfarm.uuid", &wuuids).Error
		if err != nil {
			return
		}
	case "windfield":
		db.Table("windfarm").Where("id=?", id).Pluck("uuid", &wuuids)
	}
	var mrs []PartTrendSum
	var mrsmap map[string][]PartTrendSum = make(map[string][]PartTrendSum)
	//按月、年统计总和
	switch keytype {
	case "month":
		timenow := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
		stime := timenow.AddDate(-1, 0, 0)
		etime := time.Now()
		//月份填入
		for k := range fcs {
			for i := 1; i < 13; i++ {
				fcs[k].X = append(fcs[k].X, stime.AddDate(0, i, 0).Format("2006.1"))
			}
			fcs[k].Y = make([]float32, 12)
		}
		err = db.Table("windfarm_month_report").
			Where("windfarm_uuid IN ?", wuuids).
			Where("time_set <= ? AND time_set > ?", etime.Unix(), stime.Unix()).
			Select("year", "month", "bearing_alert_count AS bearing",
				"gear_alert_count AS gear",
				"generator_alert_count AS generator",
				"cabin_alert_count AS cabin",
				"blade_alert_count AS blade",
				"tower_alert_count AS tower").
			Scan(&mrs).Error
		for k, v := range mrs {
			mrsmap[fmt.Sprint(v.Year)+"."+fmt.Sprint(v.Month)] = append(mrsmap[fmt.Sprint(v.Year)+"."+fmt.Sprint(v.Month)], mrs[k])
		}
	case "year":
		stime := time.Now().Year() - 3 //TODO
		etime := time.Now().Year()
		//年份
		for k := range fcs {
			for i := 1; i < 4; i++ {
				fcs[k].X = append(fcs[k].X, fmt.Sprint(stime+i))
			}
			fcs[k].Y = make([]float32, 3)
		}
		err = db.Table("windfarm_month_report").
			Where("windfarm_uuid IN ?", wuuids).
			Where("year <= ? AND year > ?", etime, stime).
			Group("year").
			Select("year",
				"SUM(bearing_alert_count) AS bearing",
				"SUM(gear_alert_count) AS gear",
				"SUM(generator_alert_count) AS generator",
				"SUM(cabin_alert_count) AS cabin",
				"SUM(blade_alert_count) AS blade",
				"SUM(tower_alert_count) AS tower").
			Scan(&mrs).Error
		for k, v := range mrs {
			mrsmap[fmt.Sprint(v.Year)] = append(mrsmap[fmt.Sprint(v.Year)], mrs[k])
		}
	}
	if err != nil {
		return
	}
	for kk := range fcs[0].X {
		if p, ok := mrsmap[fcs[0].X[kk]]; ok {
			for k := range p {
				fcs[0].Y[kk] += float32(p[k].Bearing)
				fcs[1].Y[kk] += float32(p[k].Gear)
				fcs[2].Y[kk] += float32(p[k].Generator)
				fcs[3].Y[kk] += float32(p[k].Cabin)
				// fcs[4].Y[kk] += float32(p[k].Blade)
				// fcs[5].Y[kk] += float32(p[k].Tower)
			}

		}
	}
	return fcs, err
}

type LevelTrendSum struct {
	Year   string
	Month  string
	Level1 uint
	Level  uint
	Level2 uint
	Level3 uint
}

func FaultPartTrend(db *gorm.DB, id string, keytype string, keyword string) (fcs []FaultCurrent, err error) {
	var wuuids []string
	switch keyword {
	case "company":
		err = db.Table("factory").
			Joins("join windfarm on factory.uuid = windfarm.factory_uuid").
			Where("factory.id=?", id).
			Pluck("windfarm.uuid", &wuuids).Error
		if err != nil {
			return
		}
	case "windfield":
		db.Table("windfarm").Where("id=?", id).Pluck("uuid", &wuuids)
	}
	var key string
	switch keytype {
	case "1":
		key = "bearing"
	case "2":
		key = "gear"
	case "3":
		key = "generator"
	case "4":
		key = "cabin"
	// case "5":
	// 	key = "tower"
	// case "6":
	// 	key = "blade"
	default:
		key = "bearing"
	}
	var mrs []LevelTrendSum
	var mrsmap map[string][]LevelTrendSum = make(map[string][]LevelTrendSum)
	fcs = []FaultCurrent{
		{Legend: "注意"},
		{Legend: "报警"},
	}
	//月份填入
	timenow := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.Local)
	stime := timenow.AddDate(-1, 0, 0) //TODO
	etime := time.Now()
	for k := range fcs {
		for i := 1; i < 13; i++ {
			fcs[k].X = append(fcs[k].X, stime.AddDate(0, i, 0).Format("2006.1"))
		}
		fcs[k].Y = make([]float32, 12)
	}
	err = db.Table("windfarm_month_report").
		Where("windfarm_uuid IN ?", wuuids).
		Where("time_set < ? AND time_set >= ?", etime.Unix(), stime.Unix()).
		Select("year", "month",
			fmt.Sprintf("%s_alert_count as level", key),
			fmt.Sprintf("%s_alert_count_1 as level1", key),
			fmt.Sprintf("%s_alert_count_2 as level2", key),
			fmt.Sprintf("%s_alert_count_3 as level3", key)).
		Scan(&mrs).Error
	for k, v := range mrs {
		mrsmap[v.Year+"."+v.Month] = append(mrsmap[v.Year+"."+v.Month], mrs[k])
	}
	for kk := range fcs[0].X {
		if p, ok := mrsmap[fcs[0].X[kk]]; ok {
			for k := range p {
				fcs[0].Y[kk] += float32(p[k].Level2)
				fcs[1].Y[kk] += float32(p[k].Level3)
			}
		}
	}
	return
}

//* 出现故障后，风机、风场日更新、月更新
func UpdateReportAfterAlert(db *gorm.DB, alert Alert) (err error) {
	if alert.ID != 0 {
		db.Table("alert").First(&alert, alert.ID)
	}
	ppmfid, _, ppmfuuid, err := PointtoFactory(db, alert.PointID)
	if err != nil {
		return
	}
	db.Table("part").Where("id = ? ", ppmfid[1]).Pluck("type", &alert.PartType)
	datetime := TimetoStr(alert.TimeSet)
	datetime = time.Date(datetime.Year(), datetime.Month(), datetime.Day(), 0, 0, 0, 0, time.Local)
	monthtime := time.Date(datetime.Year(), datetime.Month(), 1, 0, 0, 0, 0, time.Local)

	//machine daily
	var mday MachineDailyReport
	mday.MachineUUID = ppmfuuid[2]
	mday.DateTime = datetime
	mday.TimeSet = datetime.Unix()
	db.Table("machine_daily_report").Where("machine_uuid=? AND time_set=?", mday.MachineUUID, mday.TimeSet).
		Clauses(clause.Locking{Strength: "SHARE"}).
		FirstOrCreate(&mday)
	err = UpdateReportCount(db, alert.PartType, alert.Level, "machine_daily_report", fmt.Sprint(mday.ID))
	if err != nil {
		return
	}
	//machine monthly
	var mmonth MachineMonthReport
	mmonth.MachineUUID = ppmfuuid[2]
	mmonth.DateTime = monthtime
	mmonth.TimeSet = monthtime.Unix()
	mmonth.Month = uint(datetime.Month())
	mmonth.Year = uint(datetime.Year())
	db.Table("machine_month_report").Where("machine_uuid=? AND time_set=?", mmonth.MachineUUID, mmonth.TimeSet).
		Clauses(clause.Locking{Strength: "SHARE"}).
		FirstOrCreate(&mmonth)
	err = UpdateReportCount(db, alert.PartType, alert.Level, "machine_month_report", fmt.Sprint(mmonth.ID))
	if err != nil {
		return
	}
	//windfarm daily
	var wday WindfarmDailyReport
	wday.DateTime = datetime
	wday.TimeSet = datetime.Unix()
	wday.WindfarmUUID = ppmfuuid[3]
	db.Table("windfarm_daily_report").Where("windfarm_uuid=? AND time_set=?", wday.WindfarmUUID, wday.TimeSet).
		Clauses(clause.Locking{Strength: "SHARE"}).
		FirstOrCreate(&wday)
	err = UpdateReportCount(db, alert.PartType, alert.Level, "windfarm_daily_report", fmt.Sprint(wday.ID))
	if err != nil {
		return
	}
	//windfarm monthly
	var wmonth WindfarmMonthReport
	wmonth.DateTime = monthtime
	wmonth.TimeSet = monthtime.Unix()
	wmonth.WindfarmUUID = ppmfuuid[3]
	wmonth.Month = uint(datetime.Month())
	wmonth.Year = uint(datetime.Year())
	db.Table("windfarm_month_report").Where("windfarm_uuid=? AND time_set=?", wmonth.WindfarmUUID, wmonth.TimeSet).
		Clauses(clause.Locking{Strength: "SHARE"}).
		FirstOrCreate(&wmonth)
	err = UpdateReportCount(db, alert.PartType, alert.Level, "windfarm_month_report", fmt.Sprint(wmonth.ID))
	if err != nil {
		return
	}
	return
}
func UpdateReportCount(db *gorm.DB, alerttype string, alertlevel uint8, table string, id string) (err error) {
	if p, ok := partchindex[alerttype]; ok {
		if alertlevel != 1 {
			err = db.Table(table).Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).
				Updates(map[string]interface{}{
					fmt.Sprintf("%s_alert_count_%v", p, alertlevel): gorm.Expr(fmt.Sprintf("%s_alert_count_%v + ?", p, alertlevel), 1),
					fmt.Sprintf("%s_alert_count", p):                gorm.Expr(fmt.Sprintf("%s_alert_count + ?", p), 1),
					"total_alert_count":                             gorm.Expr("total_alert_count + ?", 1),
				}).Error
		} else {
			err = db.Table(table).Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).
				Updates(map[string]interface{}{
					fmt.Sprintf("%s_alert_count_%v", p, alertlevel): gorm.Expr(fmt.Sprintf("%s_alert_count_%v + ?", p, alertlevel), 1)}).Error
		}
		if err != nil {
			return
		}
	}
	return
}

//* 删除故障后，风机、风场日更新、月更新
func RollbackReportAfterDelet(db *gorm.DB, alert Alert) (err error) {
	ppmfid, _, ppmfuuid, err := PointtoFactory(db, alert.PointID)
	if err != nil {
		return
	}

	db.Table("part").Where("id = ? ", ppmfid[1]).Pluck("type", &alert.PartType)
	datetime := TimetoStr(alert.TimeSet)
	datetime = time.Date(datetime.Year(), datetime.Month(), datetime.Day(), 0, 0, 0, 0, time.Local)
	monthtime := time.Date(datetime.Year(), datetime.Month(), 1, 0, 0, 0, 0, time.Local)
	//machine daily
	var mday MachineDailyReport
	mday.MachineUUID = ppmfuuid[2]
	mday.DateTime = datetime
	mday.TimeSet = datetime.Unix()
	db.Table("machine_daily_report").
		Where("machine_uuid=? AND time_set=?", mday.MachineUUID, mday.TimeSet).
		Pluck("id", &mday.ID)
	err = RollbackReportCount(db, alert.PartType, alert.Level, "machine_daily_report", fmt.Sprint(mday.ID))
	if err != nil {
		return
	}
	//machine monthly
	var mmonth MachineMonthReport
	mmonth.MachineUUID = ppmfuuid[2]
	mmonth.DateTime = monthtime
	mmonth.TimeSet = monthtime.Unix()
	db.Table("machine_month_report").Where("machine_uuid=? AND time_set=?", mmonth.MachineUUID, mmonth.TimeSet).
		Pluck("id", &mmonth.ID)
	err = RollbackReportCount(db, alert.PartType, alert.Level, "machine_month_report", fmt.Sprint(mmonth.ID))
	if err != nil {
		return
	}
	//windfarm daily
	var wday WindfarmDailyReport
	wday.DateTime = datetime
	wday.TimeSet = datetime.Unix()
	wday.WindfarmUUID = ppmfuuid[3]
	db.Table("windfarm_daily_report").Where("windfarm_uuid=? AND time_set=?", wday.WindfarmUUID, wday.TimeSet).
		Pluck("id", &wday.ID)
	err = RollbackReportCount(db, alert.PartType, alert.Level, "windfarm_daily_report", fmt.Sprint(wday.ID))
	if err != nil {
		return
	}
	//windfarm monthly
	var wmonth WindfarmMonthReport
	wmonth.DateTime = monthtime
	wmonth.TimeSet = monthtime.Unix()
	wmonth.WindfarmUUID = ppmfuuid[3]
	db.Table("windfarm_month_report").Where("windfarm_uuid=? AND time_set=?", wmonth.WindfarmUUID, wmonth.TimeSet).
		Pluck("id", &wmonth.ID)
	err = RollbackReportCount(db, alert.PartType, alert.Level, "windfarm_month_report", fmt.Sprint(wmonth.ID))
	if err != nil {
		return
	}
	return
}
func RollbackReportCount(db *gorm.DB, alerttype string, alertlevel uint8, table string, id string) (err error) {
	if p, ok := partchindex[alerttype]; ok {
		if alertlevel != 1 {
			err = db.Table(table).Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).
				Where(fmt.Sprintf("%s_alert_count_%v", p, alertlevel) + ">= 1").
				Where(fmt.Sprintf("%s_alert_count", p) + ">= 1").
				Where("total_alert_count >= 1").
				Updates(map[string]interface{}{
					fmt.Sprintf("%s_alert_count_%v", p, alertlevel): gorm.Expr(fmt.Sprintf("%s_alert_count_%v - ?", p, alertlevel), 1),
					fmt.Sprintf("%s_alert_count", p):                gorm.Expr(fmt.Sprintf("%s_alert_count - ?", p), 1),
					"total_alert_count":                             gorm.Expr("total_alert_count - ?", 1),
				}).Error
		} else {
			err = db.Table(table).Where("id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).
				Where(fmt.Sprintf("%s_alert_count_%v", p, alertlevel) + ">= 1").
				Updates(map[string]interface{}{
					fmt.Sprintf("%s_alert_count_%v", p, alertlevel): gorm.Expr(fmt.Sprintf("%s_alert_count_%v - ?", p, alertlevel), 1)}).Error
		}
		if err != nil {
			return
		}
	}
	return
}
