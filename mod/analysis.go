package mod

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	
	"gorm.io/gorm"
)

//* 算法索引 变量
var ao map[string]AnalysisOption = map[string]AnalysisOption{
	"envelop": {
		Value:        1,
		Label:        "上包络算法",
		RpmAvailable: false,
		DataUrl:      "1",
	},
	"filter": {
		Value:        2,
		Label:        "带通滤波",
		RpmAvailable: false,
		DataUrl:      "2",
	},
	"resample": {
		Value:        3,
		Label:        "重采样",
		RpmAvailable: false,
		DataUrl:      "3",
	},
	"spectrum": {
		Value:        4,
		Label:        "频谱分析",
		RpmAvailable: false,
		DataUrl:      "0",
	},
	"order": {
		Value:        5,
		Label:        "阶次分析",
		RpmAvailable: false,
		DataUrl:      "5",
	},
}

func GetAnalysisOption() []AnalysisOption {
	var aoo []AnalysisOption
	for _, v := range ao {
		aoo = append(aoo, v)
	}
	return aoo
}

//! 全部改为float32的类型 因此写的数据为4+4+count*4

//* 前四个字节的信息
type ShmData struct {
	Cmd   float32
	Count float32
}

//* 传输的数据
type FData struct {
	Value []float32
}

//* 共享内存信息  名称、尺寸
type ShmInfo struct {
	Name  string
	Count float32
	Size  int32
}

// //* 功能码有三种：初始化、写共享内存和读共享内存
// const (
// 	SInit  float32 = 1 //初始化
// 	SWrite float32 = 2 //写
// 	SRead  float32 = 3 //读
// )

// var SInitByte, _ = Encode(SInit)
// var SWriteByte, _ = Encode(SWrite)
// var SReadByte, _ = Encode(SRead)

// func (sd *ShmData) LengthGet() float32 {
// 	return 4 + 4 + sd.Count*4
// }

// func ReadData(si ShmInfo) (t FData, err error) {
// 	var sd ShmData = ShmData{Cmd: SWrite, Count: si.Count}
// 	s, err := ShmInit(sd, si)
// 	if err != nil {
// 		return t, err
// 	}
// 	if err := t.Read(s, si.Count); err != nil {
// 		return t, err
// 	}
// 	if len(t.Value) == 0 {
// 		return t, errors.New("共享内存传输错误")
// 	}
// 	return t, nil
// }

// //开辟共享内存块并写入初始化信息（初始化功能码和数据条数上限
// func ShmInit(sd ShmData, si ShmInfo) (*shm.Memory, error) {
// 	s, err := shm.Open(si.Name, si.Size)
// 	sd.Cmd = SWrite
// 	sd.Count = si.Count
// 	if err != nil {
// 		return nil, err
// 	}
// 	buf, err := Encode(sd)
// 	if err != nil {
// 		return nil, err
// 	}
// 	_, err = s.WriteAt(buf, 0)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return s, nil
// }
// func (fd *FData) Read(s *shm.Memory, maxc float32) error {
// 	for {
// 		prefix := make([]byte, 8)
// 		s.ReadAt(prefix, 0)
// 		if bytes.Equal(prefix[:4], SReadByte) {
// 			var sd ShmData
// 			if err := Decode(prefix, &sd); err != nil {
// 				return err
// 			}
// 			if sd.Count != 0 {
// 				databuf := make([]byte, int(sd.Count)*4)
// 				s.ReadAt(databuf, 8)
// 				v := make([]float32, int(sd.Count))
// 				if err := Decode(databuf, &v); err != nil {
// 					return err
// 				}
// 				fd.Value = append(fd.Value, v...)
// 				s.WriteAt(SWriteByte, 0)
// 			}
// 			if sd.Count < maxc {
// 				break
// 			}
// 		}
// 	}
// 	s.Close()
// 	return nil
// }

// func (fd *FData) Write(name string, senddata []float32) {
// 	sinit, _ := shm.Open(name, 8)
// 	var prefix ShmData
// 	for {
// 		initbuf := make([]byte, 8)
// 		sinit.ReadAt(initbuf, 0)
// 		if bytes.Equal(initbuf[:4], SWriteByte) {
// 			Decode(initbuf, &prefix)

// 			sinit.WriteAt(SWriteByte, 0)
// 			maxc := prefix.Count
// 			s, _ := shm.Open(name, int32(prefix.LengthGet()+100))
// 			var offset = 0
// 			var limit int
// 			for {
// 				prefixbuf := make([]byte, 8)
// 				s.ReadAt(prefixbuf, 0)
// 				if bytes.Equal(prefixbuf[:4], SWriteByte) {
// 					var writeprefix ShmData
// 					var b bytes.Buffer
// 					if offset >= len(senddata) {
// 						writeprefix = ShmData{Cmd: SRead, Count: 0}
// 						p, _ := Encode(writeprefix)
// 						b.Write(p)
// 						s.WriteAt(b.Bytes(), 0)
// 						break
// 					}
// 					if offset+int(maxc) <= len(senddata) {
// 						writeprefix = ShmData{Cmd: SRead, Count: maxc}
// 						limit = offset + int(maxc)
// 					} else {
// 						writeprefix = ShmData{Cmd: SRead, Count: float32(len(senddata) - offset)}
// 						limit = len(senddata)
// 					}
// 					p, _ := Encode(writeprefix)
// 					b.Write(p)
// 					dtosend := senddata[offset:limit]
// 					d, _ := Encode(dtosend)
// 					b.Write(d)
// 					s.WriteAt(b.Bytes(), 0)
// 					offset = offset + int(len(dtosend))
// 				}
// 			}
// 			break
// 		}
// 	}
// }

//* 原始波形图
func (plot *AnalysetoPlot) OringinPlot(db *gorm.DB, fid string, iid string) (freq int, err error) {
	var dd Data
	dtable := "data_" + fid
	// 获取data和result基础信息
	err = db.Table(dtable).Preload("Wave").Last(&dd, iid).Error
	if err != nil {
		return 0, err
	}
	dd.Time = TimetoStr(dd.TimeSet).Format("2006-01-02 15:04:05")
	//时图
	var originy []float32
	freq = dd.SampleFreq
	origin := strings.Trim(string(dd.Wave.DataFloat), " ")
	onum := strings.Split(origin, " ")
	for _, v := range onum {
		temp, _ := strconv.ParseFloat(v, 32)
		originy = append(originy, float32(temp))
	}
	var ostep float64 = 1000 / float64(freq)
	originx := XGenerate(ostep, len(onum))
	var s SinglePlot = SinglePlot{Legend: "原波形图", Xaxis: originx, Yaxis: originy}
	plot.Plots = append(plot.Plots, s)
	return freq, nil
}

//传递数据图
// func (plot *AnalysetoPlot) Plot(xstep float64, si ShmInfo, legend string) (err error) {
// 	var analysedata SinglePlot
// 	fdata, err := ReadData(si)
// 	if err != nil {
// 		return err
// 	}
// 	x := XGenerate(xstep, len(fdata.Value))
// 	analysedata.Xaxis = x
// 	analysedata.Yaxis = fdata.Value
// 	analysedata.Legend = legend
// 	plot.Plots = append(plot.Plots, analysedata)
// 	return nil
// }
//*普通算法画图
func (plot *AnalysetoPlot) Plot(db *gorm.DB, xstep float64, tempid uint, legend string) (err error) {
	var analysedata SinglePlot
	dtable := "temp"
	var tempd Temp
	ch1 := make(chan bool, 1)
	ch2 := make(chan bool, 1)
	defer close(ch1)
	defer close(ch2)
	go func() {
		for {
			err = db.Table(dtable).Select("complete").Last(&tempd, tempid).Error
			if err != nil {
				return
			}
			if tempd.Complete {
				db.Table(dtable).Last(&tempd, tempid)
				ch1 <- true
				return
			}
			if <-ch2 {
				return
			}
		}
		
	}()
	select {
	//TODO 补充超时
	case <-time.After(4 * time.Second):
		db.Table("temp").Unscoped().Delete(&Temp{}, tempid)
		err = errors.New("算法分析超时")
		ch2 <- true
		return
	case <-ch1:
		if err != nil {
			return err
		}
		origin := strings.Trim(string(tempd.Data), " ")
		onum := strings.Split(origin, " ")
		
		for _, v := range onum {
			temp, _ := strconv.ParseFloat(v, 32)
			analysedata.Yaxis = append(analysedata.Yaxis, float32(temp))
		}
		x := XGenerate(xstep, len(analysedata.Yaxis))
		analysedata.Xaxis = x
		analysedata.Legend = legend
		plot.Plots = append(plot.Plots, analysedata)
		db.Table("temp").Unscoped().Delete(&Temp{}, tempid)
		return
	}
}
func (plot *AnalysetoPlot) Plot_2(db *gorm.DB, xstep float64, yData []float32, legend string) (err error) {
	var analysedata SinglePlot
	analysedata.Yaxis = yData
	x := XGenerate(xstep, len(analysedata.Yaxis))
	analysedata.Xaxis = x
	analysedata.Legend = legend
	plot.Plots = append(plot.Plots, analysedata)
	return
	
}

//*阶次图画图
func (plot *AnalysetoPlot) JPlot(db *gorm.DB, tempid uint, legend string) (err error) {
	var analysedata SinglePlot
	dtable := "temp"
	var tempd Temp
	ch1 := make(chan bool, 1)
	ch2 := make(chan bool, 1)
	defer close(ch1)
	defer close(ch2)
	go func() {
		for {
			err = db.Table(dtable).Select("complete").Last(&tempd, tempid).Error
			
			if err != nil {
				return
			}
			if tempd.Complete {
				db.Table(dtable).Last(&tempd, tempid)
				ch1 <- true
				return
			}
			if <-ch2 {
				return
			}
		}
		
	}()
	select {
	//TODO 补充超时
	case <-time.After(8 * time.Second):
		db.Table("temp").Unscoped().Delete(&Temp{}, tempid)
		err = errors.New("算法分析超时")
		ch2 <- true
		return
	case <-ch1:
		if err != nil {
			return err
		}
		origin := strings.Trim(string(tempd.Data), " ")
		onum := strings.Split(origin, " ")
		
		for _, v := range onum {
			temp, _ := strconv.ParseFloat(v, 32)
			analysedata.Yaxis = append(analysedata.Yaxis, float32(temp))
		}
		xstep := analysedata.Yaxis[len(analysedata.Yaxis)-1]
		x := XGenerate(float64(xstep), len(analysedata.Yaxis)-1)
		analysedata.Xaxis = x
		analysedata.Yaxis = analysedata.Yaxis[:len(analysedata.Yaxis)-1]
		analysedata.Legend = legend
		plot.Plots = append(plot.Plots, analysedata)
		db.Table("temp").Unscoped().Delete(&Temp{}, tempid)
		return
	}
}
func (m *AnalysetoPlot) AnalyseHandler(dbconfig *GormConfig, exepath string, atype string, fid string,
	dataidstr string, s ShmInfo, arg []string) (err error) {
	db, err := dbconfig.GormOpen()
	if err != nil {
		return err
	}
	var temp Temp
	db.Table("temp").Create(&temp)
	switch atype {
	case "envelop":
		var freq int
		freq, err = m.OringinPlot(db, fid, dataidstr)
		if err != nil {
			break
		}
		var xstep float64 = 1000 / float64(freq)
		carg := "2"
		if len(arg) != 1 {
			err = errors.New("输入参数错误")
			break
		}
		for k := range arg {
			carg = carg + " " + arg[k]
		}
		
		dbconfig.DataAnalysis(exepath, "wave_"+fid, "data_"+fid, dataidstr, carg, fmt.Sprintf("%v", temp.ID))
		
		err = m.Plot(db, xstep, temp.ID, "上包络曲线")
		if err != nil {
			break
		}
	case "filter":
		var freq int
		freq, err = m.OringinPlot(db, fid, dataidstr)
		if err != nil {
			break
		}
		var xstep float64 = 1000 / float64(freq)
		
		carg := "3" + " " + strconv.Itoa(freq)
		//参数校验
		if len(arg) != 3 {
			err = errors.New("输入参数错误")
			break
		}
		for i := 1; i < 3; i++ {
			var ai int
			ai, err = strconv.Atoi(arg[i])
			if ai > freq/2 {
				err = errors.New("输入参数需小于原波形采样频率/2=" + fmt.Sprint(freq/2))
				break
			}
		}
		if err != nil {
			break
		}
		if arg[1] == "0" {
			arg[1] = strconv.Itoa(freq / 2)
		}
		
		for k := range arg {
			carg = carg + " " + arg[k]
		}
		dbconfig.DataAnalysis(exepath, "wave_"+fid, "data_"+fid, dataidstr, carg, fmt.Sprintf("%v", temp.ID))
		err = m.Plot(db, xstep, temp.ID, "带通滤波曲线")
		if err != nil {
			break
		}
	case "resample":
		var freq, refreq int
		freq, err = m.OringinPlot(db, fid, dataidstr)
		if err != nil {
			break
		}
		carg := "4" + " " + strconv.Itoa(freq)
		if len(arg) != 1 {
			err = errors.New("输入参数错误")
			break
		}
		for k := range arg {
			carg = carg + " " + arg[k]
		}
		refreq, err = strconv.Atoi(arg[0])
		if err != nil {
			break
		}
		var xstep float64 = 1000 / float64(refreq)
		
		dbconfig.DataAnalysis(exepath, "wave_"+fid, "data_"+fid, dataidstr, carg, fmt.Sprintf("%v", temp.ID))
		err = m.Plot(db, xstep, temp.ID, "重采样曲线")
		if err != nil {
			break
		}
	
	case "spectrum":
		var freq int
		db.Table("data_"+fid).Select("sample_freq").Where("id=?", dataidstr).Scan(&freq)
		var d Wave
		db.Table("wave_"+fid).Select("data").Where("data_id=?", dataidstr).Last(&d)
		dlen := len(strings.Split(strings.Trim(string(d.DataFloat), " "), " "))
		//参数校验
		carg := "1"
		if len(arg) != 3 {
			err = errors.New("输入参数错误")
			break
		}
		var wlen, overlop int
		wlen, err = strconv.Atoi(arg[1])
		if err != nil {
			break
		}
		if wlen > dlen || wlen == 0 {
			err = errors.New("窗长度输入参数错误")
			break
		}
		overlop, err = strconv.Atoi(arg[2])
		if err != nil {
			break
		}
		if wlen <= overlop {
			err = errors.New("重复点数应小于窗长度")
			break
		}
		
		for k := range arg {
			carg = carg + " " + arg[k]
		}
		//窗长度的1/2 向上取整
		//采样频率/（窗长度/2）
		var xstep float64 = float64(freq) / math.Ceil(float64(wlen)/2)
		
		dbconfig.DataAnalysis(exepath, "wave_"+fid, "data_"+fid, dataidstr, carg, fmt.Sprintf("%v", temp.ID))
		err = m.Plot(db, xstep, temp.ID, "自分析频谱")
		if err != nil {
			break
		}
	
	case "order":
		//根据起始时间获取转速id
		var data Data
		err = db.Table("data_"+fid).Where("id=?", dataidstr).
			Select("id", "point_id", "time_set", "sample_freq").Last(&data).Error
		if err != nil {
			break
		}
		//起始时间、风场id、测点id相同
		var rpmdata Data
		err = db.Table("rpmdata_"+fid).Where("point_id=?", data.PointID).
			Where("time_set=?", data.TimeSet).
			Select("id", "point_id", "time_set", "sample_freq").
			Last(&rpmdata).Error
		if err != nil {
			err = errors.New("未找到相同起始时间的转速数据")
			break
		}
		
		arg := make([]string, 3)
		arg[0] = fmt.Sprintf("%v", rpmdata.ID)
		arg[1] = fmt.Sprintf("%v", data.SampleFreq)
		arg[2] = fmt.Sprintf("%v", rpmdata.SampleFreq)
		carg := "6"
		for k := range arg {
			carg = carg + " " + arg[k]
		}
		//调用算法计算
		dbconfig.DataAnalysis(exepath, "wave_"+fid, "data_"+fid, dataidstr, carg, fmt.Sprintf("%v", temp.ID), "rpmdata_"+fid)
		err = m.JPlot(db, temp.ID, "阶次分析")
		if err != nil {
			break
		}
	}
	return err
}

// func (m *AnalysetoPlot) AnalyseHandler(dbconfig *GormConfig, exepath string, atype string, fid string,
// 	dataidstr string, s ShmInfo, arg []string) (err error) {
// 	db, err := dbconfig.GormOpen()
// 	if err != nil {
// 		return err
// 	}
// 	s.Size = 4 + 4 + 4*int32(s.Count) + 100
// 	stemp, _ := shm.Create(s.Name, s.Size)
// 	//TODO check linux shm key
// 	fmt.Printf("name:%v,size:%v,key:%v \n", s.Name, s.Size, stemp.Key)
// 	ch := make(chan struct{}, 1)
// 	go func(key int) {
// 		switch atype {
// 		case "envelop":
// 			var freq int
// 			freq, err = m.OringinPlot(db, fid, dataidstr)
// 			if err != nil {
// 				break
// 			}
// 			var xstep float64 = 1000 / float64(freq)
// 			var wg sync.WaitGroup
// 			wg.Add(1)
// 			carg := "2"
// 			if len(arg) != 1 {
// 				err = errors.New("输入参数错误")
// 				break
// 			}
// 			for k := range arg {
// 				carg = carg + " " + arg[k]
// 			}
// 			go func() {
// 				err = m.Plot(xstep, s, "上包络曲线")
// 				wg.Done()
// 			}()
// 			// dbconfig.DataAnalysis(exepath, "data_"+fid, "result_"+fid, dataidstr, carg, s.Name)
// 			dbconfig.DataAnalysis(exepath, "data_"+fid, "result_"+fid, dataidstr, carg, fmt.Sprintf("%v", key))
//
// 			wg.Wait()
// 		case "filter":
// 			var freq int
// 			freq, err = m.OringinPlot(db, fid, dataidstr)
// 			if err != nil {
// 				break
// 			}
// 			var xstep float64 = 1000 / float64(freq)
//
// 			carg := "3" + " " + strconv.Itoa(freq)
// 			//参数校验
// 			if len(arg) != 3 {
// 				err = errors.New("输入参数错误")
// 				break
// 			}
// 			for i := 1; i < 3; i++ {
// 				var ai int
// 				ai, err = strconv.Atoi(arg[i])
// 				if ai > freq/2 {
// 					err = errors.New("输入参数需小于原波形采样频率/2=" + fmt.Sprint(freq/2))
// 					break
// 				}
// 			}
// 			if err != nil {
// 				break
// 			}
// 			if arg[1] == "0" {
// 				arg[1] = strconv.Itoa(freq / 2)
// 			}
// 			var wg sync.WaitGroup
// 			wg.Add(1)
// 			go func() {
// 				err = m.Plot(xstep, s, "带通滤波曲线")
// 				wg.Done()
// 			}()
// 			for k := range arg {
// 				carg = carg + " " + arg[k]
// 			}
// 			dbconfig.DataAnalysis(exepath, "data_"+fid, "result_"+fid, dataidstr, carg, fmt.Sprintf("%v", key))
// 			wg.Wait()
// 		case "resample":
// 			var freq, refreq int
// 			freq, err = m.OringinPlot(db, fid, dataidstr)
// 			if err != nil {
// 				break
// 			}
// 			carg := "4" + " " + strconv.Itoa(freq)
// 			if len(arg) != 1 {
// 				err = errors.New("输入参数错误")
// 				break
// 			}
// 			for k := range arg {
// 				carg = carg + " " + arg[k]
// 			}
// 			refreq, err = strconv.Atoi(arg[0])
// 			if err != nil {
// 				break
// 			}
// 			var xstep float64 = 1000 / float64(refreq)
// 			var wg sync.WaitGroup
// 			wg.Add(1)
//
// 			go func() {
// 				err = m.Plot(xstep, s, "重采样曲线")
// 				wg.Done()
// 			}()
// 			dbconfig.DataAnalysis(exepath, "data_"+fid, "result_"+fid, dataidstr, carg, fmt.Sprintf("%v", key))
// 			wg.Wait()
// 		case "spectrum":
// 			var freq int
// 			db.Table("data_"+fid).Select("sample_freq").Where("id=?", dataidstr).Scan(&freq)
// 			var d Data
// 			db.Table("data_"+fid).Select("data").Where("id=?", dataidstr).Last(&d)
// 			dlen := len(strings.Split(strings.Trim(string(d.Data), " "), " "))
// 			//参数校验
// 			carg := "1"
// 			if len(arg) != 3 {
// 				err = errors.New("输入参数错误")
// 				break
// 			}
// 			var wlen, overlop int
// 			wlen, err = strconv.Atoi(arg[1])
// 			if err != nil {
// 				break
// 			}
// 			if wlen > dlen || wlen == 0 {
// 				err = errors.New("窗长度输入参数错误")
// 				break
// 			}
// 			overlop, err = strconv.Atoi(arg[2])
// 			if err != nil {
// 				break
// 			}
// 			if wlen <= overlop {
// 				err = errors.New("重复点数应小于窗长度")
// 				break
// 			}
//
// 			for k := range arg {
// 				carg = carg + " " + arg[k]
// 			}
// 			//窗长度的1/2 向上取整
// 			//采样频率/（窗长度/2）
// 			var xstep float64 = float64(freq) / math.Ceil(float64(wlen)/2)
// 			var wg sync.WaitGroup
// 			wg.Add(1)
// 			go func() {
// 				err = m.Plot(xstep, s, "自分析频谱")
// 				wg.Done()
// 			}()
// 			dbconfig.DataAnalysis(exepath, "data_"+fid, "result_"+fid, dataidstr, carg, fmt.Sprintf("%v", key))
// 			wg.Wait()
//
// 		// TODO 阶次分析测试  需要数据测试
// 		case "order":
// 			//根据起始时间获取转速id
// 			var data Data
// 			err := db.Table("data_"+fid).Where("id=?", dataidstr).
// 				Select("id", "point_id", "time_set", "sample_freq").Last(&data).Error
// 			if err != nil {
// 				break
// 			}
// 			//起始时间、风场id、测点id相同
// 			var rpmdata Data
// 			err = db.Table("rpmdata_"+fid).Where("point_id=?", data.PointID).
// 				Where("time_set=?", data.TimeSet).
// 				Select("id", "point_id", "time_set", "sample_freq").
// 				Last(&rpmdata).Error
// 			if err != nil {
// 				err = errors.New("未找到相同起始时间的转速数据")
// 				break
// 			}
//
// 			arg := make([]string, 3)
// 			arg[0] = fmt.Sprintf("%v", rpmdata.ID)
// 			arg[1] = fmt.Sprintf("%v", data.SampleFreq)
// 			arg[2] = fmt.Sprintf("%v", rpmdata.SampleFreq)
// 			carg := "6"
// 			for k := range arg {
// 				carg = carg + " " + arg[k]
// 			}
//
// 			//调用算法计算
// 			var wg sync.WaitGroup
// 			wg.Add(1)
//
// 			go func() {
// 				err = m.JPlot(s, "阶次分析")
// 				wg.Done()
// 			}()
// 			dbconfig.DataAnalysis(exepath, "data_"+fid, "result_"+fid, dataidstr, carg, fmt.Sprintf("%v", key), "rpmdata_"+fid)
// 			wg.Wait()
// 		}
// 		ch <- struct{}{}
// 	}(stemp.Key)
//
// 	select {
// 	case <-ch:
// 		stemp.Close()
// 		return err
// 	//TODO 补充超时
// 	case <-time.After(5 * time.Second):
// 		stemp.Close()
// 		return errors.New("共享内存传输超时")
// 	}
// }

// func (plot *AnalysetoPlot) JPlot(si ShmInfo, legend string) (err error) {
// 	var analysedata SinglePlot
// 	fdata, err := ReadData(si)
// 	if err != nil {
// 		return err
// 	}

// 	xstep := fdata.Value[len(fdata.Value)-1]
// 	x := XGenerate(float64(xstep), len(fdata.Value)-1)
// 	analysedata.Xaxis = x
// 	analysedata.Yaxis = fdata.Value[:len(fdata.Value)-1]
// 	analysedata.Legend = legend
// 	plot.Plots = append(plot.Plots, analysedata)
// 	return nil
// }

//*数据服务 端口可配置，默认3005
//keyfunc 为目标算法序号，与服务的url相关

type Index00 struct {
	Window     string `json:"window,omitempty"` //“hanning” / “rectangular”
	WLen       int    `json:"w_len,omitempty"`
	Overlop    int    `json:"overlop"`
	Freq       int    `json:"fs,omitempty"`
	DownFr     int    `json:"down_fr"`
	UpFr       int    `json:"up_fr,omitempty"`
	Extension  string `json:"extension,omitempty"`
	ResampleFs int    `json:"resample_fs,omitempty"`
	Mode       string `json:"mode,omitempty"`
}
type DataPost00 struct {
	Datafloat []float32 `json:"data,omitempty"`
	Rpmfloat  []float32 `json:"rpm,omitempty"`
	Index00
	Backfloat []float32 `json:"result"`
	Result
	RMS     float32 `json:"rms"`
	Code    int     `json:"code"`
	Message string  `json:"message"`
}

func (m *AnalysetoPlot) AnalyseHandler_2(db *gorm.DB, ipport string, keyfunc string, fid string, dataid string, arg ...string) (err error) {
	var postData *DataPost00
	var data Data
	var url string
	err = db.Table("data_"+fid).Preload("Wave", func(db *gorm.DB) *gorm.DB {
		return db.Table("wave_" + fid)
	}).Last(&data, dataid).Error
	if err != nil {
		return err
	}
	ourl := "http://" + ipport + "/api/v1/data/trans/"
	var originy []float32 = make([]float32, len(data.Wave.DataFloat)/4)
	err = Decode(data.Wave.DataFloat, &originy)
	if err != nil {
		return err
	}
	switch keyfunc {
	case "spectrum": //0
		//窗函数
		//ourl : http://localhost:3006/api/v1/data/trans/0
		url = ourl + ao[keyfunc].DataUrl
		if len(arg) != 3 {
			err = errors.New("输入参数错误")
			break
		}
		dlen := len(originy)
		var wlen, overlop int
		wlen, err = strconv.Atoi(arg[1])
		if err != nil {
			break
		}
		if wlen > dlen || wlen == 0 {
			err = errors.New("窗长度输入参数错误，应小于该数据长度：" + fmt.Sprint(dlen))
			break
		}
		overlop, err = strconv.Atoi(arg[2])
		if err != nil {
			break
		}
		if wlen <= overlop {
			err = errors.New("重复点数应小于窗长度")
			break
		}
		var window string
		if arg[1] == "1" {
			window = "hanning"
		} else {
			window = "rectangular"
			
		}
		postData = &DataPost00{
			Datafloat: originy,
			Index00: Index00{
				Window:  window,
				WLen:    wlen,
				Overlop: overlop,
			},
		}
		postData.Analysis(url)
		
		var xstep float64 = float64(data.SampleFreq) / math.Ceil(float64(wlen)/2)
		err = m.Plot_2(db, xstep, postData.Backfloat, "自分析频谱")
		if err != nil {
			break
		}
	
	case "envelop":
		//包络提取
		url = ourl + ao[keyfunc].DataUrl
		if len(arg) != 1 {
			err = errors.New("输入参数错误")
			break
		}
		postData = &DataPost00{
			Datafloat: originy,
		}
		if arg[0] == "1" {
			postData.Mode = "hilbert" //1
		} else {
			postData.Mode = "localpeak" //0
		}
		postData.Analysis(url)
		var xstep float64 = 1000 / float64(data.SampleFreq)
		err = m.Plot_2(db, xstep, postData.Datafloat, "原波形图")
		if err != nil {
			break
		}
		err = m.Plot_2(db, xstep, postData.Backfloat, "上包络曲线")
		if err != nil {
			break
		}
	
	case "filter":
		//滤波
		url = ourl + ao[keyfunc].DataUrl
		postData = &DataPost00{
			Datafloat: originy,
			Index00: Index00{
				Freq: data.SampleFreq,
			},
		}
		if len(arg) != 3 {
			err = errors.New("输入参数错误")
			return err
		}
		var ai int
		ai, err = strconv.Atoi(arg[0])
		if ai > data.SampleFreq/2 {
			err = errors.New("输入参数需小于原波形采样频率/2=" + fmt.Sprint(data.SampleFreq/2))
			return err
		}
		if err != nil {
			return err
		}
		postData.DownFr = ai
		ai, err = strconv.Atoi(arg[1])
		if ai > data.SampleFreq/2 {
			err = errors.New("输入参数需小于原波形采样频率/2=" + fmt.Sprint(data.SampleFreq/2))
			return err
		}
		if err != nil {
			return err
		}
		if ai == 0 {
			ai = data.SampleFreq / 2
		}
		postData.UpFr = ai
		
		if arg[2] == "1" {
			postData.Extension = "1" //1
		} else {
			postData.Extension = "0" //0
		}
		postData.Analysis(url)
		var xstep float64 = 1000 / float64(data.SampleFreq)
		err = m.Plot_2(db, xstep, postData.Datafloat, "原波形图")
		if err != nil {
			break
		}
		err = m.Plot_2(db, xstep, postData.Backfloat, "带通滤波曲线")
		if err != nil {
			break
		}
	
	case "resample":
		//重采样
		url = ourl + ao[keyfunc].DataUrl
		
		postData = &DataPost00{
			Datafloat: originy,
			Index00: Index00{
				Freq: data.SampleFreq,
			},
		}
		if len(arg) != 1 {
			err = errors.New("输入参数错误")
			return err
		}
		var ai int
		ai, err = strconv.Atoi(arg[0])
		if err != nil {
			return err
		}
		postData.ResampleFs = ai
		postData.Analysis(url)
		var xstep float64 = 1000 / float64(data.SampleFreq)
		err = m.Plot_2(db, xstep, postData.Datafloat, "原波形图")
		if err != nil {
			break
		}
		err = m.Plot_2(db, 1000/float64(ai), postData.Backfloat, "重采样曲线")
		if err != nil {
			break
		}
	
	case "order":
		//阶次谱
		url = ourl + ao[keyfunc].DataUrl
		postData = &DataPost00{
			Datafloat: originy,
			Index00: Index00{
				Freq: data.SampleFreq,
			},
		}
		//起始时间、风场id、测点id相同
		var rpmdata Data
		err = db.Table("data_rpm_"+fid).
			Joins(fmt.Sprintf("join %s on %s = %s", "wave_rpm_"+fid, "wave_rpm_"+fid+".data_uuid", "data_rpm_"+fid+".uuid")).
			Where("data_rpm_"+fid+".point_id=?", data.PointID).Where("data_rpm_"+fid+".time_set=?", data.TimeSet).
			Select("wave_rpm_" + fid + ".data").
			Last(&rpmdata).Error
		if err != nil {
			err = errors.New("未找到相同起始时间的转速数据")
			return err
		}
		var originrpm []float32 = make([]float32, len(rpmdata.Wave.DataFloat)/4)
		err := Decode(data.Wave.DataFloat, &originrpm)
		if err != nil {
			return err
		}
		postData.Rpmfloat = originrpm
		postData.Analysis(url)
		var xstep = 1000 / float64(data.SampleFreq)
		err = m.Plot_2(db, xstep, postData.Datafloat, "原波形图")
		if err != nil {
			break
		}
		xstep = float64(postData.Backfloat[len(postData.Backfloat)-1])
		err = m.Plot_2(db, xstep, postData.Backfloat[:len(postData.Backfloat)-1], "阶次分析图")
		if err != nil {
			break
		}
		
	}
	return
}

func (postData *DataPost00) Analysis(url string) error {
	postBody, err := json.Marshal(*postData)
	if err != nil {
		return err
	}
	
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postBody))
	if err != nil {
		return err
	}
	
	// 读取响应内容
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(postData)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	return nil
}

type AnalysisOption struct {
	Value        int    `json:"value"`
	Label        string `json:"label"`
	RpmAvailable bool   `json:"rpm_available"`
	DataUrl      string `json:"-"`
}
