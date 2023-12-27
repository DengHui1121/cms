package mod

import (
	"fmt"
	"main/alert"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baliance.com/gooxml"
	"baliance.com/gooxml/color"
	"baliance.com/gooxml/common"
	"baliance.com/gooxml/document"
	"github.com/wcharczuk/go-chart"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// * 返回给前端的文件导出状况
type OutputJob struct {
	JobSet      JobSet
	OutputFiles []*OutputFile `json:"output_file"`
}
type JobSet struct {
	FileType string `json:"file_type"` //"1"data "2"alert
	Limit
	FilePath string   `json:"file_path"` //存储路径
	PointIDs []string `json:"point_ids"`
}
type OutputFile struct {
	FileType   string         `json:"file_type"` //"1"data "2"alert
	FWP        []FanWithPoint `json:"-"`
	FileName   string         `json:"file_name"`
	FileStatus bool           `json:"file_status"`
}
type FanWithPoint struct {
	FanID      string   `json:"-"`
	IDtoOutput []string `json:"-"` //excel:data docx:point
}

func (opj *OutputJob) New() {
	opj.OutputFiles = make([]*OutputFile, 0)
}

func (opj *OutputJob) OutputData(db *gorm.DB) error {
	//find data id
	var err error
	task := new(OutputFile)
	task.FileType = opj.JobSet.FileType

	var fwp FanWithPoint
	fwp.FanID = opj.JobSet.Machine

	stime, _ := StrtoTime("2006-01-02 15:04:05", opj.JobSet.Starttime)
	etime, _ := StrtoTime("2006-01-02 15:04:05", opj.JobSet.Endtime)

	err = db.Table("data_"+opj.JobSet.Machine).Where("time_set BETWEEN ? AND ?", stime, etime).Where("rpm BETWEEN ? AND ?", opj.JobSet.MinRpm, opj.JobSet.MaxRpm).
		Joins(fmt.Sprintf("left join point on point.uuid = %s", "data_"+opj.JobSet.Machine+".point_uuid")).
		Where("point.id IN ?", opj.JobSet.PointIDs).
		Pluck("data_"+opj.JobSet.Machine+".id", &fwp.IDtoOutput).Error
	if err != nil {
		return err
	}
	task.FWP = append(task.FWP, fwp)
	task.FileName, err = OutputDataExcel(db, opj.JobSet.FilePath, fwp.FanID, fwp.IDtoOutput)
	if err != nil {
		return err
	}
	task.FileStatus = true
	opj.OutputFiles = append(opj.OutputFiles, task)
	return nil
}
func (opj *OutputJob) OutputAlert(db *gorm.DB) error {
	var err error
	task := new(OutputFile)
	task.FileType = opj.JobSet.FileType

	stime, _ := StrtoTime("2006-01-02 15:04:05", opj.JobSet.Starttime)
	etime, _ := StrtoTime("2006-01-02 15:04:05", opj.JobSet.Endtime)

	var fwp FanWithPoint
	fwp.FanID = opj.JobSet.Machine
	err = db.Table("alert").Where("time_set BETWEEN ? AND ?", stime, etime).Where("rpm BETWEEN ? AND ?", opj.JobSet.MinRpm, opj.JobSet.MaxRpm).
		Joins("left join point on point.uuid = alert.point_uuid").
		Where("point.id IN ?", opj.JobSet.PointIDs).
		Pluck("alert.id", &fwp.IDtoOutput).Error
	if err != nil {
		return err
	}
	task.FWP = append(task.FWP, fwp)
	task.FileName, err = OutputAlertExcel(db, opj.JobSet.FilePath, fwp.FanID, fwp.IDtoOutput)
	if err != nil {
		return err
	}
	task.FileStatus = true
	opj.OutputFiles = append(opj.OutputFiles, task)
	return nil
}

// *机组自动诊断报告
func (opj *OutputJob) OutputLog(db *gorm.DB) error {
	var err error
	//每个风场 多个风机 加载到多个outputfile中
	task := new(OutputFile)
	var fids []string
	db.Table("windfarm").
		Joins("right join machine on machine.windfarm_uuid = windfarm.uuid").
		Where("windfarm.id = ?", opj.JobSet.Windfarm).
		Select("machine.id").
		Scan(&fids)
	task.FWP = make([]FanWithPoint, len(fids))
	for k, v := range fids {
		task.FWP[k].FanID = v
		db.Table("machine").
			Where("machine.id = ?", task.FWP[k].FanID).
			Joins("right join part on part.machine_uuid = machine.uuid").
			Joins("right join point on point.part_uuid = part.uuid").
			Select("point.id AS idto_output").
			Scan(&task.FWP[k].IDtoOutput)
	}
	task.FileName, err = OutputLogDocx(db, opj.JobSet.FilePath, *task, opj.JobSet)
	if err != nil {
		_, ierr := os.Stat(task.FileName)
		if os.IsExist(ierr) {
			os.Remove(task.FileName)
		}
		task.FileStatus = false
		opj.OutputFiles = append(opj.OutputFiles, task)
		return err
	}
	task.FileStatus = true
	opj.OutputFiles = append(opj.OutputFiles, task)
	return err
}
func (opj *OutputJob) OutputReport(db *gorm.DB) error {
	var err error
	//每个风场 多个风机 加载到多个outputfile中
	task := new(OutputFile)
	var fids []string
	db.Table("windfarm").
		Joins("right join machine on machine.windfarm_uuid = windfarm.uuid").
		Where("windfarm.id = ?", opj.JobSet.Windfarm).
		Select("machine.id").
		Scan(&fids)
	task.FWP = make([]FanWithPoint, len(fids))
	for k, v := range fids {
		task.FWP[k].FanID = v
		db.Table("machine").
			Where("machine.id = ?", task.FWP[k].FanID).
			Joins("right join part on part.machine_uuid = machine.uuid").
			Joins("right join point on point.part_uuid = part.uuid").
			Select("point.id AS idto_output").
			Scan(&task.FWP[k].IDtoOutput)
	}
	task.FileName, err = OutputSuggestDocx(db, opj.JobSet.FilePath, *task, opj.JobSet)
	if err != nil {
		_, ierr := os.Stat(task.FileName)
		if os.IsExist(ierr) {
			os.Remove(task.FileName)
		}
		task.FileStatus = false
		opj.OutputFiles = append(opj.OutputFiles, task)
		return err
	}
	task.FileStatus = true
	opj.OutputFiles = append(opj.OutputFiles, task)
	return err
}

//*查找范围内数据

// *根据data_id查询所有的结果
// 风场，机组，测点
// 数据类型
// 门限值，报警状态
// result
type ExcelIndex struct {
	Column int
	CName  string
	EName  string
}

type DataExcel struct {
	//基础
	Windfield string `json:"windfield"`
	Fan       string `json:"fan"`
	Part      string `json:"part"`
	Point     string `json:"point"`
	//data表
	ID            uint   `json:"id,string"`
	PointID       uint   `json:"point_id,string"`
	Time          string `json:"time" gorm:"-"` //采样时间
	TimeSet       int64  `json:"-"`
	Measuredefine string `json:"define"`
	Status        uint8  `json:"status,string"` //数据状态
	StatusName    string `json:"status_name"`   //数据状态
	Band_value1   string `json:"bv1"`           //预留：频带值1。格式：最小值 最大值
	Band_value2   string `json:"bv2"`           //预留：频带值2
	Band_value3   string `json:"bv3"`           //预留：频带值3
	Band_value4   string `json:"bv4"`           //预留：频带值4
	Band_value5   string `json:"bv5"`           //预留：频带值5
	Band_value6   string `json:"bv6"`           //预留：频带值6
	//result表
	Result
	AlertVersion string //alert_version
	PartType     string //part type
	//报警band
	BandMessage string           `json:"bandmessage"`
	BandStds    []DataExcel_Band `gorm:"-"`
}
type DataExcel_Band struct {
	Range    string
	FloorStd float32
	UpperStd float32
	Property string
}

func OutputDataExcel(db *gorm.DB, fpath string, fid string, dataid []string) (fname string, err error) {
	var eimap []ExcelIndex = []ExcelIndex{
		{CName: "时间", EName: "time"},
		{CName: "风场", EName: "windfield"},
		{CName: "机组", EName: "fan"},
		{CName: "部件", EName: "part"},
		{CName: "测点", EName: "point"},
		{CName: "数据类型", EName: "define"},
		{CName: "状态", EName: "status_name"},
		{CName: "门限值", EName: "bandmessage"},
		{CName: "有效值", EName: "rmsvalue"},
		{CName: "峭度指标", EName: "indexkur"},
		{CName: "脉冲指标", EName: "indexi"},
		{CName: "波形指标", EName: "indexk"},
		{CName: "裕度指标", EName: "indexl"},
		{CName: "歪度指标", EName: "indexsk"},
		{CName: "峰值指标", EName: "indexc"},
		{CName: "方根赋值", EName: "indexxr"},
		{CName: "最大值", EName: "indexmax"},
		{CName: "最小值", EName: "indexmin"},
		{CName: "均值", EName: "indexmean"},
		{CName: "平均赋值", EName: "indexeven"},
		{CName: "频带1", EName: "bv1"}, {CName: "频带1有效值", EName: "brms1"},
		{CName: "频带2", EName: "bv2"}, {CName: "频带2有效值", EName: "brms2"},
		{CName: "频带3", EName: "bv3"}, {CName: "频带3有效值", EName: "brms3"},
		{CName: "频带4", EName: "bv4"}, {CName: "频带4有效值", EName: "brms4"},
		{CName: "频带5", EName: "bv5"}, {CName: "频带5有效值", EName: "brms5"},
		{CName: "频带6", EName: "bv6"}, {CName: "频带6有效值", EName: "brms6"},
	}
	//开始查询
	//基本信息 数据
	dtable := "data_" + fid
	dtablei := "data_" + fid + ".id"
	var o []DataExcel
	//join 拼表
	db.Table(dtable).
		Where(dtablei+" IN ?", dataid).
		Order(dtable+".time_set").
		Joins(fmt.Sprintf("left join point on point.uuid = %s", dtable+".point_uuid")).
		Joins("left join part on part.uuid = point.part_uuid").
		Joins("left join machine on machine.uuid = part.machine_uuid").
		Joins("left join windfarm on windfarm.uuid = machine.windfarm_uuid").
		// Joins("left join band on band.version = machine.alert_version AND band.type = part.type AND band.value = data_11.measuredefine").
		Select(dtable+".*", "point.id AS point_id", "point.name AS point", "part.name AS part", "part.type AS part_type", "machine.desc AS fan", "machine.alert_version", "windfarm.name AS windfield").
		// Select("band.range", "band.floor", "band.upper").
		Limit(100000). //excel限制行数
		Scan(&o)

	f := excelize.NewFile()
	streamWriter, err := f.NewStreamWriter("Sheet1")
	if err != nil {
		return
	}
	row := make([]interface{}, len(eimap))
	for k := range eimap {
		row[k] = eimap[k].CName
	}
	if err = streamWriter.SetRow("A1", row); err != nil {
		return
	}
	for rowID := 2; rowID < len(o)+2; rowID++ {
		row := make([]interface{}, len(eimap))
		db.Table("band").Where("value=?", o[rowID-2].Measuredefine).Find(&o[rowID-2].BandStds)
		ppmids, _, ppmuuid, _ := PointtoFactory(db, o[rowID-2].PointID)
		var bds []alert.Band
		bds, err = AlertSearch(db, ppmids, ppmuuid[0], o[rowID-2].Measuredefine)
		if err != nil {
			return
		}
		if len(bds) != 0 {
			for _, v := range bds {
				o[rowID-2].BandMessage = o[rowID-2].BandMessage + fmt.Sprintf("对特征值:%s;频带:%s; B/C:%v; C/D:%v\n", v.Property, v.Range, v.FloorStd, v.UpperStd)
			}
		}
		o[rowID-2].BandMessage = strings.Trim(o[rowID-2].BandMessage, "\n")
		switch o[rowID-2].Status {
		case 2:
			o[rowID-2].StatusName = "注意"
		case 3:
			o[rowID-2].StatusName = "报警"
		default:
			o[rowID-2].StatusName = "正常"
		}
		var omap map[string]interface{}
		o[rowID-2].Time = TimetoStr(o[rowID-2].TimeSet).Format("2006-01-02 15:04:05")
		MaptoStruct(o[rowID-2], &omap)

		for colID := 0; colID < len(eimap); colID++ {
			row[colID] = omap[eimap[colID].EName]
		}

		cell, _ := excelize.CoordinatesToCellName(1, rowID)
		if err = streamWriter.SetRow(cell, row); err != nil {
			return
		}
	}
	if err = streamWriter.Flush(); err != nil {
		return
	}
	if fpath == "" {
		fpath = "./output/excel"
	}
	_, err = os.Stat(fpath)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(fpath, os.ModePerm); err != nil {
			return
		}
	}
	if !filepath.IsAbs(fpath) {
		fpath, err = filepath.Abs(fpath)
		if err != nil {
			return
		}
	}
	fpath = filepath.ToSlash(fpath)
	fname = fpath + "/data_" + time.Now().Format("20060102150405") + ".xlsx"
	if err = f.SaveAs(fname); err != nil {
		return
	}
	return
}

// *导出报警结果
type AlertExcel struct {
	ID      uint `gorm:"primarykey" json:"-"`
	PointID uint `json:"point_id,string"`
	//基础
	Windfield string `json:"windfield"`
	Fan       string `json:"fan"`
	Part      string `json:"part"`
	Point     string `json:"point"`
	//alert表
	Time      string `json:"time"` //时间
	TimeSet   int64  `json:"-"`    //格式转换 //^ 数据的时间
	Level     uint8  `gorm:"type:tinyint" json:"level"`
	Type      string `json:"type"`     //报警类型 故障树、频道幅值···
	Strategy  string `json:"strategy"` //策略描述 如有效值报警
	Desc      string `json:"desc"`     //报警描述
	Source    uint8  //0：自动 1：人工
	SourceStr string `json:"source"`    //0：自动 1：人工
	Code      string `json:"code"`      //预留 告警类型代码
	Faulttype int    `json:"faulttype"` //预留 故障标识
	Suggest   string `json:"suggest"`
}

func OutputAlertExcel(db *gorm.DB, fpath string, fid string, dataid []string) (fname string, err error) {
	//查询信息index
	imap := []ExcelIndex{
		{CName: "时间", EName: "time"},
		{CName: "风场", EName: "windfield"},
		{CName: "机组", EName: "fan"},
		{CName: "部件", EName: "part"},
		{CName: "测点", EName: "point"},
		{CName: "报警类型", EName: "type"},
		{CName: "策略描述", EName: "strategy"},
		{CName: "报警描述", EName: "desc"},
		{CName: "报警来源", EName: "source"},
		{CName: "处理建议", EName: "suggest"},
		{CName: "告警类型代码", EName: "code"},
		{CName: "故障标识", EName: "faulttype"},
	}
	//开始查询
	//基本信息 数据
	var o []AlertExcel
	db.Table("alert").
		Where("alert.id IN ?", dataid).
		Joins("left join point on point.uuid = alert.point_uuid").
		Joins("left join part on part.uuid = point.part_uuid").
		Joins("left join machine on machine.uuid = part.machine_uuid").
		Joins("left join windfarm on windfarm.uuid = machine.windfarm_uuid").
		Select("alert.*", "point.id AS point_id", "point.name AS point", "part.name AS part", "part.type AS part_type", "machine.desc AS fan", "machine.alert_version", "windfarm.name AS windfield").
		Order("alert.time_set").
		Limit(100000). //excel限制行数
		Scan(&o)
	f := excelize.NewFile()
	streamWriter, err := f.NewStreamWriter("Sheet1")
	if err != nil {
		return
	}
	row := make([]interface{}, len(imap))
	for k := range imap {
		row[k] = imap[k].CName
	}
	if err = streamWriter.SetRow("A1", row); err != nil {
		return
	}

	for rowID := 2; rowID < len(o)+2; rowID++ {
		o[rowID-2].Time = TimetoStr(o[rowID-2].TimeSet).Format("2006-01-02 15:04:05")
		if o[rowID-2].Source == 1 {
			o[rowID-2].SourceStr = "人工"
		} else {
			o[rowID-2].SourceStr = "自动"
		}
		row := make([]interface{}, len(imap))
		var omap map[string]interface{}
		MaptoStruct(o[rowID-2], &omap)
		for colID := 0; colID < len(imap); colID++ {
			row[colID] = omap[imap[colID].EName]
		}
		cell, _ := excelize.CoordinatesToCellName(1, rowID)
		if err = streamWriter.SetRow(cell, row); err != nil {
			return
		}
	}
	if err = streamWriter.Flush(); err != nil {
		return
	}
	if fpath == "" {
		fpath = "./output/excel"
	}
	_, err = os.Stat(fpath)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(fpath, os.ModePerm); err != nil {
			return
		}
	}
	if !filepath.IsAbs(fpath) {
		fpath, err = filepath.Abs(fpath)
		if err != nil {
			return
		}
	}
	fpath = filepath.ToSlash(fpath)
	fname = fpath + "/alert_" + time.Now().Format("20060102150405") + ".xlsx"
	if err = f.SaveAs(fname); err != nil {
		return
	}
	return
}

// 数据趋势报告
func OutputLogDocx(db *gorm.DB, fpath string, outputfile OutputFile, jobset JobSet) (fname string, err error) {
	if fpath == "" {
		fpath = "./output/doc"
	}
	_, err = os.Stat(fpath)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(fpath, os.ModePerm); err != nil {
			return
		}
	}
	if !filepath.IsAbs(fpath) {
		fpath, err = filepath.Abs(fpath)
		if err != nil {
			return
		}
	}
	fpath = filepath.ToSlash(fpath)
	fname = fpath + "/currentreport_" + time.Now().Format("20060102150405") + ".docx"
	// 调取报告模板，替换基本信息
	// read and parse the template docx

	var wname string
	db.Table("windfarm").Where("id=?", jobset.Windfarm).Select("name").Find(&wname)
	stime, _ := StrtoTime("2006-01-02 15:04:05", jobset.Starttime)
	etime, _ := StrtoTime("2006-01-02 15:04:05", jobset.Endtime)
	// doc, err := docx.Open("./template/currentreport_template.docx")
	// if err != nil {
	// 	return
	// }
	// replaceMap := docx.PlaceholderMap{
	// 	"windfield": wname,
	// 	"stime":     jobset.Starttime,
	// 	"etime":     jobset.Endtime,
	// 	"min":       jobset.MinRpm,
	// 	"max":       jobset.MaxRpm,
	// }
	// // replace the keys with values from replaceMap
	// err = doc.ReplaceAll(replaceMap)
	// if err != nil {
	// 	return
	// }
	// err = doc.WriteToFile(fname)
	// if err != nil {
	// 	return
	// }
	//使用unioffice读取
	gooxml.DisableLogging()
	// docw, err := document.Open(fname)
	docw := document.New()
	style := docw.Styles.AddStyle("CustomStyle", 1, true)
	style.RunProperties().SetFontFamily("宋体")
	style.RunProperties().SetFontFamily("Times New Roman")
	style.RunProperties().SetCharacterSpacing(0.5)
	style.RunProperties().SetColor(color.Black)
	style.RunProperties().SetSize(12.0)

	p := docw.AddParagraph()
	run := p.AddRun()
	run.AddText(fmt.Sprintf("风场：%s  机组诊断报告  %s", wname, time.Now().Format("2006-01-02 15:04:05")))
	docw.AddParagraph().AddRun().AddText("=========================================================")
	docw.AddParagraph().AddRun().AddText(fmt.Sprintf("时间范围：%s ~ %s", jobset.Starttime, jobset.Endtime))
	docw.AddParagraph().AddRun().AddText(fmt.Sprintf("转速范围：%v ~ %vRPM", jobset.MinRpm, jobset.MaxRpm))

	//循环 每个风机添加风机信息
	for _, fv := range outputfile.FWP {
		// 添加风机名
		var fanname string
		err = db.Table("machine").Where("id=?", fv.FanID).Pluck("desc", &fanname).Error
		if err != nil {
			return
		}
		// 设置字体
		docw.AddParagraph().AddRun().AddText("=====================================================")
		docw.AddParagraph().AddRun().AddText("风机：" + fanname)
		// err = docw.SaveToFile(fname)
		// if err != nil {
		// 	return
		// }
		//每个测点填写模板
		for _, pv := range fv.IDtoOutput {
			var define []string
			db.Table("data_"+fv.FanID).
				Joins(fmt.Sprintf("left join point on point.uuid=%s.point_uuid", "data_"+fv.FanID)).
				Where("point.id=?", pv).Distinct("measuredefine").Pluck("measuredefine", &define)
			if len(define) == 0 {
				define = append(define, "无数据")
			}
			for _, d := range define {
				// docw, err = document.Open(fname)
				// if err != nil {
				// 	return
				// }
				err = WritePointInfo(db, docw, pv, d, stime, etime)
				if err != nil {
					return
				}
				//诊断描述为该测点最新报警状况。
				var descword string
				sub := db.Table("alert").
					Joins("left join point on point.uuid=alert.point_uuid").
					Where("point.id=?", pv).
					Where("time_set BETWEEN ? AND ?", stime, etime).
					Order("time_set desc").
					Select("alert.*")
				var acount int64
				db.Table("(?) as tree", sub).Count(&acount)
				var apoint Alert
				if acount == 0 {
					descword = "运行正常。"
				} else {
					db.Table("(?) as tree", sub).
						Select("level", "desc", "suggest").First(&apoint)
					descword = fmt.Sprintf("%v级报警。%s", apoint.Level, apoint.Desc)
				}
				p = docw.AddParagraph()
				run = p.AddRun()
				run.AddText("诊断结果：" + descword)
				p = docw.AddParagraph()
				run = p.AddRun()
				run.AddText("处理建议：" + apoint.Suggest)
				var pic *os.File
				pic, err = CurrentTemplate(db, pv, fv.FanID, d)
				if err != nil {
					return
				}
				if pic == nil {
					p := docw.AddParagraph()
					run := p.AddRun()
					run.AddText("无历史数据。")
				} else {
					err = WritePic(pic, docw)
					if err != nil {
						return
					}
				}
				//进行筛选得到趋势图 图片加入unioffice
				docw.AddParagraph().AddRun()
				if pic != nil {
					defer os.Remove(pic.Name())
				}
			}
		}

	}
	err = docw.SaveToFile(fname)
	if err != nil {
		return
	}
	// write out a new file
	return
}

func CurrentTemplate(db *gorm.DB, pid string, fid string, define string) (pic *os.File, err error) {
	//画图
	var historydata []Data
	db.Table("data_"+fid).
		Joins(fmt.Sprintf("left join point on point.uuid=%s.point_uuid", "data_"+fid)).
		Where("point.id=?", pid).
		Where("measuredefine = ?", define).Order("time_set").
		Select("data_"+fid+".id", "data_"+fid+".uuid", "data_"+fid+".time_set", "data_"+fid+".rmsvalue").Find(&historydata)
	if len(historydata) != 0 {
		var historyt []time.Time
		var historyr []float32
		for k := range historydata {
			historyt = append(historyt, TimetoStr(historydata[k].TimeSet))
			historyr = append(historyr, historydata[k].Rmsvalue)
		}
		pic, err = DrawLineChart_time(historyt, historyr)
		if err != nil {
			return
		}
		defer pic.Close()
	} else {
		return nil, nil
	}
	return
}

// 时间序列趋势图
func DrawLineChart_time(X_axis []time.Time, Y_axis []float32) (*os.File, error) {
	//Setting the Width and Height of the Image
	const (
		lineChartWidth  = 595
		lineChartHeight = 210
		lineChartDpi    = 72
	)
	ymin := float64(Y_axis[0])
	ymax := float64(Y_axis[0])

	//transfer float32 to float64
	var Y_axis_64 []float64
	for _, f32_Y := range Y_axis {
		Y_axis_64 = append(Y_axis_64, float64(f32_Y))
		if float64(f32_Y) < ymin {
			ymin = float64(f32_Y)
		}
		if float64(f32_Y) > ymax {
			ymax = float64(f32_Y)
		}
	}
	var delta float64 = 0
	if ymin == ymax {
		delta = 0.1
	}
	if len(X_axis) == 1 {
		xtime2, _ := time.ParseDuration("1h")
		X_axis = append(X_axis, X_axis[0].Add(xtime2))
		Y_axis_64 = append(Y_axis_64, Y_axis_64[0])
	}
	//Draw picture
	graph := chart.Chart{
		Width:  lineChartWidth,
		Height: lineChartHeight,
		DPI:    lineChartDpi,

		Series: []chart.Series{
			&chart.TimeSeries{
				XValues: X_axis,
				YValues: Y_axis_64,
			},
		},
		Title: "RMS Value Trend Chart",
		TitleStyle: chart.Style{
			Show:     true,
			FontSize: 10,
		},
		XAxis: chart.XAxis{
			Style: chart.Style{
				Show: true,
			},
			Name: "Time/ms",
			NameStyle: chart.Style{
				Show: true,
			},
			ValueFormatter: chart.TimeValueFormatterWithFormat("2006-01-02 15:04:05"),
		},
		YAxis: chart.YAxis{
			Style: chart.Style{
				Show: true,
			},
			Name: "RMS",
			NameStyle: chart.Style{
				Show: false,
			},
			AxisType: chart.YAxisSecondary,
			Range: &chart.ContinuousRange{
				Min: ymin - (ymax-ymin)/5 - delta,
				Max: ymax + (ymax-ymin)/5 + delta,
			},
		},
	}

	if _, err := os.Stat("./output/temp"); err != nil {
		if !os.IsExist(err) {
			if err = os.MkdirAll("./output/temp", os.ModePerm); err != nil {
				return nil, err
			}
		}
	}
	fpath, err := filepath.Abs("./output/temp")
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(fpath, fmt.Sprintf("%v", time.Now().Unix()))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	err = graph.Render(chart.PNG, f)
	if err != nil {
		return nil, err
	}
	return f, err
}

// //TODO  dubug人工报警报告
func OutputSuggestDocx(db *gorm.DB, fpath string, outputfile OutputFile, jobset JobSet) (fname string, err error) {
	if fpath == "" {
		fpath = "./output/doc"
	}
	_, err = os.Stat(fpath)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(fpath, os.ModePerm); err != nil {
			return
		}
	}
	if !filepath.IsAbs(fpath) {
		fpath, err = filepath.Abs(fpath)
		if err != nil {
			return
		}
	}
	fpath = filepath.ToSlash(fpath)
	fname = fpath + "/suggestreport_" + time.Now().Format("20060102150405") + ".docx"
	var wname string
	db.Table("windfarm").Where("id=?", jobset.Windfarm).Select("name").Find(&wname)
	stime, _ := StrtoTime("2006-01-02 15:04:05", jobset.Starttime)
	etime, _ := StrtoTime("2006-01-02 15:04:05", jobset.Endtime)
	// doc, err := docx.Open("./template/alertreport_template.docx")
	// if err != nil {
	// 	return err
	// }
	// stime, err := time.ParseInLocation("2006-01-02 15:04:05", rlimit.Starttime, time.Local)
	// if err != nil {
	// 	return err
	// }
	// rlimit.Starttime = stime.Format("2006.01.02 15:04")
	// etime, err := time.ParseInLocation("2006-01-02 15:04:05", rlimit.Endtime, time.Local)
	// if err != nil {
	// 	return err
	// }
	// rlimit.Endtime = etime.Format("2006.01.02 15:04")

	// replaceMap := docx.PlaceholderMap{
	// 	"windfield": rlimit.Windfield,
	// 	"stime":     rlimit.Starttime,
	// 	"etime":     rlimit.Endtime,
	// }
	// // replace the keys with values from replaceMap
	// err = doc.ReplaceAll(replaceMap)
	// if err != nil {
	// 	return
	// }
	// err = doc.WriteToFile(outputfilename)
	// if err != nil {
	// 	return
	// }
	//使用unioffice读取
	gooxml.DisableLogging()
	docw := document.New()
	style := docw.Styles.AddStyle("CustomStyle", 1, true)
	style.RunProperties().SetFontFamily("宋体")
	style.RunProperties().SetFontFamily("Times New Roman")
	style.RunProperties().SetColor(color.Black)
	style.RunProperties().SetSize(12.0)
	style.RunProperties().SetCharacterSpacing(0.5)
	p := docw.AddParagraph()
	run := p.AddRun()
	run.AddText(fmt.Sprintf("风场：%s  人工诊断报告  %s", wname, time.Now().Format("2006-01-02 15:04:05")))
	docw.AddParagraph().AddRun().AddText("=========================================================")
	docw.AddParagraph().AddRun().AddText(fmt.Sprintf("时间范围：%s ~ %s", jobset.Starttime, jobset.Endtime))
	docw.AddParagraph().AddRun().AddText(fmt.Sprintf("转速范围：%v ~ %vRPM", jobset.MinRpm, jobset.MaxRpm))

	//循环 每个风机添加风机信息
	for _, fv := range outputfile.FWP {
		// 添加风机名
		var alertcount int64
		db.Table("alert").Joins("left join point on point.uuid = alert.point_uuid").
			Where("point.id IN ?", fv.IDtoOutput).
			Where("source = ?", 0).
			Count(&alertcount)
		if alertcount > 0 {
			var fanname string
			var fanunit string
			err = db.Table("machine").Where("id=?", fv.FanID).Pluck("desc", &fanname).Error
			if err != nil {
				continue
			}
			err = db.Table("machine").Where("id=?", fv.FanID).Pluck("unit", &fanunit).Error
			if err != nil {
				continue
			}
			docw.AddParagraph().AddRun().AddText("=========================================================")
			docw.AddParagraph().AddRun().AddText("风机：" + fanname)

			//每个测点填写模板
			for _, pv := range fv.IDtoOutput {
				//查找所有人工报警
				var aid []Alert
				db.Table("alert").
					Joins("left join point on point.uuid = alert.point_uuid").
					Where("point.id=? AND alert.source=?", pv, 0).
					Where("alert.time_set BETWEEN ? AND ?", stime, etime).
					Preload("ManualAlert").
					Find(&aid)
				//每条人工报警进行说明
				for _, a := range aid {
					var d Data
					db.Table("data_"+fv.FanID).Where("uuid=?", a.DataUUID).First(&d)
					//测点基本信息
					err = WritePointInfo(db, docw, pv, d.Measuredefine, stime, etime)
					if err != nil {
						return
					}
					//波形图
					//频谱图
					var pico, picr *os.File
					pico, picr, err = AlertTemplate(db, d, fanunit)
					if err != nil {
						return
					}
					err = WritePic(pico, docw)
					if err != nil {
						return
					}
					err = WritePic(picr, docw)
					if err != nil {
						return
					}
					//人工报警
					p := docw.AddParagraph()
					run := p.AddRun()
					run.AddText("诊断结论：" + a.Desc)
					p = docw.AddParagraph()
					run = p.AddRun()
					run.AddText("处理建议：" + a.Suggest)
					docw.AddParagraph().AddRun()
					if pico != nil {
						defer os.Remove(pico.Name())
					}
					if picr != nil {
						defer os.Remove(picr.Name())
					}
					//维护图片 写到临时文件 暂不加

				}
			}
		}

	}
	err = docw.SaveToFile(fname)
	if err != nil {
		return
	}
	return
}

// 返回模板、波形、频谱
func AlertTemplate(db *gorm.DB, d Data, fanunit string) (pic1, pic2 *os.File, err error) {
	var x_origin, y_origin, x_result, y_result []float32
	ids, _, _, err := PointtoFactory(db, d.PointID)
	err = db.Table("data_"+ids[2]).Preload("Wave", func(db *gorm.DB) *gorm.DB {
		return db.Table("wave_" + ids[2])
	}).Last(&d, d.ID).Error
	//时频图
	y_origin = make([]float32, len(d.Wave.DataFloat)/4)
	y_result = make([]float32, len(d.Wave.SpectrumFloat)/4)
	err = Decode(d.Wave.DataFloat, &y_origin)
	if err != nil {
		return
	}
	err = Decode(d.Wave.SpectrumFloat, &y_result)
	if err != nil {
		return
	}
	onum := len(y_origin)
	rnum := len(y_result)
	var ostep float64 = 1000 / float64(d.SampleFreq)
	var rstep float64 = float64(d.SampleFreq) / float64(onum)
	x_origin = XGenerate(ostep, onum)
	x_result = XGenerate(rstep, rnum)
	//画图 原始
	pic1, err = DrawLineChart(x_origin, y_origin, "Time/ms", "", "Waveform Diagram (unit:"+fanunit+")")
	if err != nil {
		os.Remove(pic1.Name())
		return
	}
	defer pic1.Close()
	//画图 频谱

	pic2, err = DrawLineChart(x_result, y_result, "Frequency/HZ", "", "Spectrogram")
	if err != nil {
		os.Remove(pic2.Name())
		return
	}
	defer pic2.Close()
	return
}

func DrawLineChart(X_axis, Y_axis []float32, Xname, Yname string, title string) (*os.File, error) {
	//Setting the Width and Height of the Image
	const (
		lineChartWidth  = 595
		lineChartHeight = 220
		lineChartDpi    = 72
	)
	ymin := float64(Y_axis[0])
	ymax := float64(Y_axis[0])

	//transfer float32 to float64
	var Y_axis_64 []float64
	var X_axis_64 []float64
	for _, f32_Y := range Y_axis {
		Y_axis_64 = append(Y_axis_64, float64(f32_Y))
		if float64(f32_Y) < ymin {
			ymin = float64(f32_Y)
		}
		if float64(f32_Y) > ymax {
			ymax = float64(f32_Y)
		}
	}

	//transfer float32 to float64
	for _, f32_Y := range Y_axis {
		Y_axis_64 = append(Y_axis_64, float64(f32_Y))
		if float64(f32_Y) < ymin {
			ymin = float64(f32_Y)
		}
		if float64(f32_Y) > ymax {
			ymax = float64(f32_Y)
		}
	}
	for _, f32_X := range X_axis {
		X_axis_64 = append(X_axis_64, float64(f32_X))
	}
	var delta float64 = 0
	if ymax == ymin {
		delta = 0.1
	}
	//Draw picture
	graph := chart.Chart{
		Width:  lineChartWidth,
		Height: lineChartHeight,
		DPI:    lineChartDpi,
		Title:  title,
		TitleStyle: chart.Style{
			Show:     true,
			FontSize: 10,
		},

		Background: chart.Style{
			Padding: chart.Box{
				Top: 20,
			},
		},

		Series: []chart.Series{
			chart.ContinuousSeries{
				XValues: X_axis_64,
				YValues: Y_axis_64,
			},
		},
		XAxis: chart.XAxis{
			Style: chart.Style{
				Show: true,
			},
			Name: Xname,
			NameStyle: chart.Style{
				Show: true,
			},
		},
		YAxis: chart.YAxis{
			Style: chart.Style{
				Show: true,
			},
			Name: Yname,
			NameStyle: chart.Style{
				Show: true,
			},
			AxisType: chart.YAxisSecondary,
			Range: &chart.ContinuousRange{
				Min: ymin - (ymax-ymin)/5 - delta,
				Max: ymax + (ymax-ymin)/5 + delta,
			},
		},
	}
	if _, err := os.Stat("./output/temp"); err != nil {
		if !os.IsExist(err) {
			if err = os.MkdirAll("./output/temp", os.ModePerm); err != nil {
				return nil, err
			}
		}
	}
	fpath, err := filepath.Abs("./output/temp")
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(fpath, fmt.Sprintf("%v", time.Now().Unix()))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	err = graph.Render(chart.PNG, f)
	if err != nil {
		return nil, err
	}
	return f, err
}

func WritePointInfo(db *gorm.DB, docw *document.Document, pid string, define string, stime, etime int64) (err error) {
	// 测点
	var rpoint Point
	err = db.Table("point").Where("id=?", pid).Find(&rpoint).Error
	if err != nil {
		return
	}
	if rpoint.Direction == "" {
		rpoint.Direction = "缺失"
	}
	var status string
	switch rpoint.Status {
	case 0:
		status = "无数据"
	case 1:
		status = "正常  "
	case 2:
		status = "注意  "
	case 3:
		status = "报警  "
	}
	p := docw.AddParagraph()
	run := p.AddRun()
	run.Properties().SetFontFamily("宋体")
	run.AddText("所选测点：" + rpoint.Name)
	p = docw.AddParagraph()
	run = p.AddRun()
	run.Properties().SetFontFamily("宋体")
	run.AddText(fmt.Sprintf("方向：%s", rpoint.Direction))
	run.AddTab()
	run.AddTab()
	run.AddTab()
	run.AddTab()
	run.AddText(fmt.Sprintf("当前状态：%s", status))
	run.AddTab()
	run.AddTab()
	run.AddTab()
	run.AddText(fmt.Sprintf("参数：%s", define))
	// //诊断描述为该测点最新报警状况。
	return
}

func WritePic(pic *os.File, docw *document.Document) (err error) {
	// 测点
	i, err := common.ImageFromFile(pic.Name())
	if err != nil {
		return err
	}
	i.Size.X = 410
	i.Size.Y = 144
	iref, err := docw.AddImage(i)
	if err != nil {
		return err
	}
	para := docw.AddParagraph()
	_, err = para.AddRun().AddDrawingInline(iref)
	if err != nil {
		return err
	}
	return
}

// func WriteAlertPic(pic []*os.File, docw *document.Document) (err error) {
// 	para := docw.AddParagraph()
// 	for _, v := range pic {
// 		i, err := common.ImageFromFile(v.Name())
// 		if err != nil {
// 			return err
// 		}
// 		i.Size.X = 200
// 		iref, err := docw.AddImage(i)
// 		if err != nil {
// 			return err
// 		}
// 		_, err = para.AddRun().AddDrawingInline(iref)
// 		if err != nil {
// 			return err
// 		}
// 	}
// 	return
// }
