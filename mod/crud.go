//^ handler 涉及较复杂CRUD的函数

package mod

import (
	"fmt"
	"io"
	"main/alert"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var Alertmessage chan *Alert = make(chan *Alert, 200) //收报警信息的id
var DataLimit *LimitCondition = new(LimitCondition)

// ^ 文件读取
func FileGet(file io.Reader, dst interface{}) error {
	_, err := toml.NewDecoder(file).Decode(dst)
	if err != nil {
		return err
	}
	return nil
}

//^ 风机特征值和测点导入事务。

// ^ 读取风机toml文件并索引到标准文件。
func MachineFileUpdate(src io.Reader, db *gorm.DB) (m Machine, err error) {
	err = FileGet(src, &m)
	if err != nil {
		return m, err
	}
	return m, nil
}

// 风机更新
func MachineUpdate(db *gorm.DB, p Machine) error {
	db.Transaction(func(tx *gorm.DB) error {
		//部件层更新
		if len(p.Parts) != 0 {
			var parttemp []uint
			for k := range p.Parts {
				err := tx.Table("machine").Select("uuid").Where("id=?", p.ID).First(&p).Error
				if err != nil {
					return err
				}
				p.Parts[k].ID = CheckExist(tx, "part", "machine_uuid", p.UUID, "name", p.Parts[k].Name)
				if p.Parts[k].ID == 0 {
					//没有部件则创建
					p.Parts[k].MachineUUID = p.UUID
					tx.Table("part").Create(&p.Parts[k])
					parttemp = append(parttemp, p.Parts[k].ID)
				} else {
					//有部件则更新，记录部件的id
					parttemp = append(parttemp, p.Parts[k].ID)
					tx.Table("part").Omit("status", "machine_uuid").Omit(clause.Associations).Clauses(clause.Locking{Strength: "UPDATE"}).Updates(&p.Parts[k])
					tx.Table("part").Select("uuid").Where("id=?", p.Parts[k].ID).First(&p.Parts[k])
					//更新测点（测点名、方向、故障树版本）
					//测点 测点下特征值和频带标准
					var pointtemp []uint
					var propertytemp []uint
					var bandtemp []uint
					for pk := range p.Parts[k].Points {
						//uuid 关联
						p.Parts[k].Points[pk].ID = CheckExist(tx, "point", "part_uuid", p.Parts[k].UUID, "name", p.Parts[k].Points[pk].Name)
						if p.Parts[k].Points[pk].ID != 0 {
							tx.Table("point").
								Omit("status", "part_uuid").Omit(clause.Associations).
								Select("name", "direction", "tree_version").Clauses(clause.Locking{Strength: "UPDATE"}).
								Updates(&p.Parts[k].Points[pk])
							pointtemp = append(pointtemp, p.Parts[k].Points[pk].ID)
						} else {
							p.Parts[k].Points[pk].PartUUID = p.Parts[k].UUID
							tx.Table("point").Omit(clause.Associations).Create(&p.Parts[k].Points[pk])
							pointtemp = append(pointtemp, p.Parts[k].Points[pk].ID)
						}
						tx.Table("point").Omit(clause.Associations).First(&p.Parts[k].Points[pk], p.Parts[k].Points[pk].ID)

						//更新测点下特征值
						var pointpropertytemp []uint
						for ppk := range p.Parts[k].Points[pk].Properties {
							err := tx.Table("property").Where("point_uuid=?", p.Parts[k].Points[pk].UUID).
								Where("name=? AND name_en=?", p.Parts[k].Points[pk].Properties[ppk].Name, p.Parts[k].Points[pk].Properties[ppk].NameEn).Pluck("id", &p.Parts[k].Points[pk].Properties[ppk].ID).Error
							if err != nil {
								return err
							}
							if p.Parts[k].Points[pk].Properties[ppk].ID != 0 {
								tx.Table("property").Omit(clause.Associations).
									Select("name", "value", "name_en", "formula", "remark").Clauses(clause.Locking{Strength: "UPDATE"}).Updates(&p.Parts[k].Points[pk].Properties[ppk])
								pointpropertytemp = append(pointpropertytemp, p.Parts[k].Points[pk].Properties[ppk].ID)
							} else {
								p.Parts[k].Points[pk].Properties[ppk].PointUUID = p.Parts[k].Points[pk].UUID
								tx.Table("property").Omit(clause.Associations).Create(&p.Parts[k].Points[pk].Properties[ppk])
								pointpropertytemp = append(pointpropertytemp, p.Parts[k].Points[pk].Properties[ppk].ID)
							}
						}
						var pointpropertydeleteid []Property
						tx.Table("property").Where("point_uuid=? AND id NOT IN?", p.Parts[k].Points[pk].UUID, pointpropertytemp).Find(&pointpropertydeleteid)
						if len(pointpropertydeleteid) != 0 {
							tx.Table("property").Unscoped().Delete(&pointpropertydeleteid)
						}
						//更新测点下频带标准
						var pointbandtemp []uint
						for bk := range p.Parts[k].Points[pk].Bands {
							if p.Parts[k].Points[pk].Bands[bk].Range == "" {
								err = tx.Table("band").
									Where("point_uuid=?", p.Parts[k].Points[pk].UUID).
									Where("value=? ", p.Parts[k].Points[pk].Bands[bk].Value).
									Where("property=?", p.Parts[k].Points[pk].Bands[bk].Property).
									Pluck("id", &p.Parts[k].Points[pk].Bands[bk].ID).
									Error
							} else {
								err = tx.Table("band").
									Where("point_uuid=?", p.Parts[k].Points[pk].UUID).
									Where("value=? ", p.Parts[k].Points[pk].Bands[bk].Value).
									Where("property=?", p.Parts[k].Points[pk].Bands[bk].Property).
									Where("band_range=?", p.Parts[k].Points[pk].Bands[bk].Range).
									Pluck("id", &p.Parts[k].Points[pk].Bands[bk].ID).
									Error
							}
							if err != nil {
								return err
							}
							if p.Parts[k].Points[pk].Bands[bk].ID != 0 {
								tx.Table("band").Omit(clause.Associations).
									Select("value", "property", "band_range", "floor_level", "floor_std", "floor_desc", "floor_suggest", "upper_level", "upper_std", "upper_desc", "upper_suggest", "rpm_floor", "rpm_upper").
									Clauses(clause.Locking{Strength: "UPDATE"}).Updates(&p.Parts[k].Points[pk].Bands[bk])
								pointbandtemp = append(pointbandtemp, p.Parts[k].Points[pk].Bands[bk].ID)
							} else {
								p.Parts[k].Points[pk].Bands[bk].PointUUID = p.Parts[k].Points[pk].UUID
								tx.Table("band").Omit(clause.Associations).Create(&p.Parts[k].Points[pk].Bands[bk])
								pointbandtemp = append(pointbandtemp, p.Parts[k].Points[pk].Bands[bk].ID)
							}
						}
						var pointbanddeleteid []alert.Band
						tx.Table("band").Where("point_uuid=? AND id NOT IN?", p.Parts[k].Points[pk].UUID, pointbandtemp).Find(&pointbanddeleteid)
						if len(pointbanddeleteid) != 0 {
							tx.Table("band").Unscoped().Delete(&pointbanddeleteid)
						}
					}
					var pointdeleteid []Point
					tx.Table("point").Where("part_uuid=? AND id NOT IN?", p.Parts[k].UUID, pointtemp).Find(&pointdeleteid)
					if len(pointdeleteid) != 0 {
						tx.Table("point").Unscoped().Delete(&pointdeleteid)
					}
					//更新特征值
					for ppk := range p.Parts[k].Properties {
						err := tx.Table("property").Where("point_uuid=?", p.Parts[k].UUID).
							Where("name=? AND name_en=?", p.Parts[k].Properties[ppk].Name, p.Parts[k].Properties[ppk].NameEn).Pluck("id", &p.Parts[k].Properties[ppk].ID).Error
						if err != nil {
							return err
						}
						if p.Parts[k].Properties[ppk].ID != 0 {
							tx.Table("property").Omit(clause.Associations).
								Select("name", "value", "name_en", "formula", "remark").Clauses(clause.Locking{Strength: "UPDATE"}).Updates(&p.Parts[k].Properties[ppk])
							propertytemp = append(propertytemp, p.Parts[k].Properties[ppk].ID)
						} else {
							p.Parts[k].Properties[ppk].PartUUID = p.Parts[k].UUID
							tx.Table("property").Omit(clause.Associations).Create(&p.Parts[k].Properties[ppk])
							propertytemp = append(propertytemp, p.Parts[k].Properties[ppk].ID)
						}
					}
					var prodeleteid []Property
					tx.Table("property").Where("part_uuid=? AND id NOT IN?", p.Parts[k].UUID, propertytemp).Find(&prodeleteid)
					if len(prodeleteid) != 0 {
						tx.Table("property").Unscoped().Delete(&prodeleteid)
					}
					//更新频带
					for bk := range p.Parts[k].Bands {
						if p.Parts[k].Bands[bk].Range == "" {
							err = tx.Table("band").Where("point_uuid=?", p.Parts[k].UUID).
								Where("value=? ", p.Parts[k].Bands[bk].Value).
								Where("property=? ", p.Parts[k].Bands[bk].Property).
								Pluck("id", &p.Parts[k].Bands[bk].ID).Error
						} else {
							err = tx.Table("band").Where("point_uuid=?", p.Parts[k].UUID).
								Where("value=? ", p.Parts[k].Bands[bk].Value).
								Where("property=? ", p.Parts[k].Bands[bk].Property).
								Where("band_range=?", p.Parts[k].Bands[bk].Range).
								Pluck("id", &p.Parts[k].Bands[bk].ID).Error
						}
						if err != nil {
							return err
						}
						if p.Parts[k].Bands[bk].ID != 0 {
							tx.Table("band").Omit(clause.Associations).
								Select("value", "property", "band_range", "floor_level", "floor_std", "floor_desc", "floor_suggest", "upper_level", "upper_std", "upper_desc", "upper_suggest", "rpm_floor", "rpm_upper").
								Clauses(clause.Locking{Strength: "UPDATE"}).Updates(&p.Parts[k].Bands[bk])
							bandtemp = append(bandtemp, p.Parts[k].Bands[bk].ID)
						} else {
							p.Parts[k].Bands[bk].PartUUID = p.Parts[k].UUID
							tx.Table("band").Omit(clause.Associations).Create(&p.Parts[k].Bands[bk])
							bandtemp = append(bandtemp, p.Parts[k].Bands[bk].ID)
						}
					}
					var banddeleteid []alert.Band
					tx.Table("band").Where("part_uuid=? AND id NOT IN?", p.Parts[k].UUID, bandtemp).Find(&banddeleteid)
					if len(banddeleteid) != 0 {
						tx.Table("property").Unscoped().Delete(&banddeleteid)
					}
				}
			}
			if len(parttemp) != 0 {
				var deleteid []Part
				tx.Table("part").Where("machine_uuid=? AND id NOT IN?", p.UUID, parttemp).Find(&deleteid)
				if len(deleteid) != 0 {
					tx.Table("part").Unscoped().Delete(&deleteid)
				}
			}
			if err := tx.Table("machine").Select("windfarm.name name").Joins("left join windfarm on windfarm.uuid = machine.windfarm_uuid").Where("machine.id = ?").Error; err != nil {
				return err
			}
		} else {
			var deleteid []Part
			tx.Table("part").Where("machine_uuid=?", p.UUID).Select("id").Find(&deleteid)
			if len(deleteid) != 0 {
				tx.Table("part").Unscoped().Delete(&deleteid)
			}
		}
		db.Table("machine").
			Select("name", "type", "fan_version", "tree_version", "desc", "built_time", "band_alert_set",
				"tree_alert_set", "unit", "genfactory", "gentype", "gbxfactory", "gbxtype", "mbrfactory", "mbrtype", "bladefactory", "bladetype").
			Clauses(clause.Locking{Strength: "UPDATE"}).Updates(&p)
		return nil
	})
	return nil
}

func CheckExist(db *gorm.DB, table string, uplevel, uplevelid string, indexlevel, index string) (id uint) {
	err := db.Table(table).Where(uplevel+"=?", uplevelid).Where(indexlevel+"=?", index).Pluck("id", &id).Error
	if err != nil {
		return
	}
	return
}

// ^ x坐标生成
func XGenerate(step float64, length int) (x []float32) {
	x = make([]float32, 0)
	for i := 0; i < length; i++ {
		if step > 0.01 {
			aa, _ := strconv.ParseFloat(strconv.FormatFloat(float64(i)*step, 'f', 2, 32), 32)
			x = append(x, float32(aa))
		} else {
			aa, _ := strconv.ParseFloat(strconv.FormatFloat(float64(i)*step, 'f', 6, 32), 32)
			x = append(x, float32(aa))
		}
	}
	return x
}

// ^ 单测点绘图坐标
func (plot *DatatoPlot) Plot(db *gorm.DB, tableprefix string, fid string, iid string) (err error) {
	var dd Data
	dtable := "data_" + tableprefix + fid
	rtable := "wave_" + tableprefix + fid
	// 获取data和result基础信息
	err = db.Table(dtable).Preload("Wave", func(db *gorm.DB) *gorm.DB {
		return db.Table(rtable)
	}).Last(&dd, iid).Error
	if err != nil {
		return err
	}
	dd.Time = time.Unix(dd.TimeSet, 0).Format("2006-01-02 15:04:05")
	freq := dd.SampleFreq

	//时频图
	originy := make([]float32, len(dd.Wave.DataFloat)/4)
	resulty := make([]float32, len(dd.Wave.SpectrumFloat)/4)
	err = Decode(dd.Wave.DataFloat, &originy)
	if err != nil {
		return err
	}
	onum := len(originy)
	err = Decode(dd.Wave.SpectrumFloat, &resulty)
	if err != nil {
		return err
	}
	rnum := len(resulty)

	var ostep float64 = 1000 / float64(freq)
	var rstep float64 = float64(freq) / float64(onum)

	originx := XGenerate(ostep, onum)
	resultx := XGenerate(rstep, rnum)

	plot.Originplot.Xaxis = originx
	plot.Originplot.Yaxis = originy
	plot.Originplot.Xunit = "ms"
	db.Table("machine").Where("id=?", fid).Pluck("unit", &plot.Originplot.Yunit)

	plot.Resultplot.Xaxis = resultx
	plot.Resultplot.Yaxis = resulty
	plot.Resultplot.Xunit = "hz"
	db.Table("machine").Where("id=?", fid).Pluck("unit", &plot.Resultplot.Yunit)
	plot.Data = dd
	return err
}

// ^ 趋势图绘图坐标
// TODO: 限制点数.选择点的前后一年内10000条的历史数据
func (plot *DatatoPlot) CPlot(db *gorm.DB, tableprefix string, fid string, iid string, ctype string) (err error) {
	dtable := "data_" + tableprefix + fid
	var data Data
	db.Table(dtable).Where("id=?", iid).Select("point_uuid", "time_set", "measuredefine").First(&data)
	stime, _ := StrtoTime(DataLimit.Starttime, "2006-01-02 15:04:05")
	etime, _ := StrtoTime(DataLimit.Endtime, "2006-01-02 15:04:05")
	// stime := time.Unix(data.TimeSet, 0).AddDate(0, -6, 0).Unix()
	// etime := time.Unix(data.TimeSet, 0).AddDate(0, 6, 0).Unix()
	// var historydata []Data
	// err = db.Table("(?) as d", sub).Find(&historydata).Error
	var historydata []Data
	var hd1, hd2 []Data
	var hd1f, hd2f []float32
	// var count int64
	// sub := db.Table(dtable).Where("time_set <? and time_set >= ?", etime, stime).Where("measuredefine=?", data.Measuredefine).Count(&count)

	//按全局的查询条件限制趋势的时间范围
	sub2 := db.Table(dtable).Where("point_uuid=?", data.PointUUID).Order("time_set").
		Where("time_set > ? and time_set<=?", data.TimeSet, etime).Where("measuredefine=?", data.Measuredefine)
	sub1 := db.Table(dtable).Where("point_uuid=?", data.PointUUID).Order("time_set desc").
		Where("time_set <= ? and time_set>=?", data.TimeSet, stime).Where("measuredefine=?", data.Measuredefine)
	db.Table("(?) as u", sub1).Order("time_set").Limit(10000).Offset(0).Select("time_set", "id").Find(&hd1).Select(ctype).Find(&hd1f)
	db.Table("(?) as u", sub2).Order("time_set").Limit(10000).Offset(0).Select("time_set", "id").Find(&hd2).Select(ctype).Find(&hd2f)

	historydata = append(hd1, hd2...)
	if err != nil {
		return err
	}
	historyt := make([]string, 0)
	historyd := make([]string, 0)
	historyr := append(hd1f, hd2f...)
	if len(historydata) != 0 {
		for _, v := range historydata {
			historyd = append(historyd, fmt.Sprint(v.ID))
			t := TimetoStr(v.TimeSet)
			historyt = append(historyt, t.Format("2006-01-02 15:04:05"))
			if err != nil {
				return err
			}
		}
	}
	plot.Currentplot.IDaxis = historyd //添加了id序列。便于趋势图选点
	plot.Currentplot.Xaxis = historyt
	plot.Currentplot.Yaxis = historyr
	plot.Currentplot.PointId = fmt.Sprint(data.PointID)
	return err
}

// TODO Debug
func (plot *MultiDatatoPlot) Plot(db *gorm.DB, ctype string) (err error) {
	for k, v := range plot.Currentplot {
		ppmwcid, pmwname, _, err := PointtoFactory(db, v.PointId)
		if err != nil {

			return err
		}
		plot.Currentplot[k].Legend = pmwname[0] + "_" + pmwname[2] + "_" + pmwname[3]
		fid := ppmwcid[2]

		stamp1, err := time.ParseInLocation("2006-01-02 15:04:05", v.Limit.Starttime, time.Local)
		if err != nil {
			return err
		}
		stamp2, err := time.ParseInLocation("2006-01-02 15:04:05", v.Limit.Endtime, time.Local)
		if err != nil {
			return err
		}

		var tempd []Data
		var point Point
		db.Table("point").Where("id=?", v.PointId).Select("uuid").First(&point)
		sub := db.Table("data_"+fid).
			Where("point_uuid=?", point.UUID).
			Where("time_set BETWEEN ? AND ?", stamp1.Unix(), stamp2.Unix()).
			Where("rpm BETWEEN ? AND ?", v.Limit.MinRpm, v.Limit.MaxRpm).
			Order("time_set")
		err = db.Table("(?) as d", sub).Select("id", "time_set").Find(&tempd).Error
		if err != nil {
			return err
		}
		var idgroup = make([]string, 0)
		var rgroup = make([]float32, 0)
		var tgroup = make([]string, 0)
		for _, v := range tempd {
			tgroup = append(tgroup, TimetoStr(v.TimeSet).Format("2006-01-02 15:04:05"))
			idgroup = append(idgroup, fmt.Sprint(v.ID))
			var tempr float32
			err = db.Table("(?) as d", sub).Where("id=?", v.ID).Select(ctype).Scan(&tempr).Error
			if err != nil {
				return err
			}
			rgroup = append(rgroup, tempr)
		}
		plot.Currentplot[k].Xaxis = tgroup
		plot.Currentplot[k].Yaxis = rgroup
		plot.Currentplot[k].IDaxis = idgroup
	}
	return nil
}

// 最新一百条数据
func (plot *MultiDatatoPlot) FanStaticPlot(db *gorm.DB, ctype string, fid string) (err error) {
	// monthStart := time.Now().Format("2006-01")
	// monthEnd := time.Now().AddDate(0, -2, 0).Format("2006-01")
	for k, v := range plot.Currentplot {
		var tempd []Data
		var point Point
		db.Table("point").Where("id=?", v.PointId).Select("uuid").First(&point)
		var idgroup []string
		//* 最近三个月的数据
		sub := db.Table("data_"+fid).
			Where("point_uuid=?", point.UUID).
			Order("time_set desc").Limit(100)

		err = db.Table("(?) as d", sub).Order("time_set").Select("id", "time_set", ctype).Find(&tempd).Error
		if err != nil {
			return err
		}
		idgroup = make([]string, 0)
		var rgroup []float32 = make([]float32, 0)
		var tgroup []string = make([]string, 0)
		for _, v := range tempd {
			tgroup = append(tgroup, TimetoStr(v.TimeSet).Format("2006-01-02 15:04:05"))
			idgroup = append(idgroup, fmt.Sprint(v.ID))
			switch ctype {
			case "rmsvalue":
				rgroup = append(rgroup, v.Rmsvalue)
			case "indexkur":
				rgroup = append(rgroup, v.Indexkur)
			case "indexi":
				rgroup = append(rgroup, v.Indexi)
			case "indexk":
				rgroup = append(rgroup, v.Indexk)
			case "indexl":
				rgroup = append(rgroup, v.Indexl)
			case "indexc":
				rgroup = append(rgroup, v.Indexc)
			case "indexxr":
				rgroup = append(rgroup, v.Indexxr)
			case "indexmax":
				rgroup = append(rgroup, v.Indexmax)
			case "indexmin":
				rgroup = append(rgroup, v.Indexmin)
			case "indexmean":
				rgroup = append(rgroup, v.Indexmean)
			case "indexeven":
				rgroup = append(rgroup, v.Indexeven)
			}

		}
		plot.Currentplot[k].Xaxis = tgroup
		plot.Currentplot[k].Yaxis = rgroup
		plot.Currentplot[k].IDaxis = idgroup
	}
	return nil
}

// ^ 导入数据
// 将上传文件的pdata传参，根据测点、时间戳、文件名检查是否存在，不存在则写入数据库；
// 存在则填充数据id、uuid, time到pdata中
// FIXME 解决离线数据上传, 导致创建时间为0000-00-00 00:00:00
func CheckData(db *gorm.DB, pdata *Data) error {
	pid := strconv.FormatUint(uint64(pdata.PointID), 10)
	ppmwcid, _, _, err := PointtoFactory(db, pid)
	if err != nil {
		return err
	}
	fid := ppmwcid[2]
	var table string
	if strings.ToUpper(pdata.Datatype) == "TACH" {
		table = "data_rpm_" + fid
	} else {
		table = "data_" + fid
	}
	var count int64
	sub := db.Table(table).Where("point_uuid=?", pdata.PointUUID).
		Where("time_set=? AND filepath=?", pdata.TimeSet, pdata.Filepath).
		Count(&count)
	if count != 0 {
		err = sub.Select("id", "uuid", "created_at", "updated_at").Scan(&pdata).Error
	}
	if err != nil {
		return err
	}
	return nil
}

func InsertData(ddb *gorm.DB, db *gorm.DB, ipport string, pdata Data) error {
	// var edata Data
	pid := strconv.FormatUint(uint64(pdata.PointID), 10)
	ppmwcid, _, _, err := PointtoFactory(db, pid)
	if err != nil {
		return err
	}
	fid := ppmwcid[2]
	pdata.Status = 1 //默认为1.正常

	//写入数据库
	if strings.ToUpper(pdata.Datatype) == "TACH" {
		err = db.Transaction(func(tx *gorm.DB) error {
			if pdata.ID == 0 {
				err = tx.Table("data_rpm_" + fid).Omit("created_at").Omit(clause.Associations).Create(&pdata).Error
				if err != nil {
					return err
				}
			} else {
				err = tx.Table("data_rpm_"+fid).Omit("created_at").Omit(clause.Associations).Where("id=?", pdata.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Save(&pdata).Error
			}
			if len(pdata.Wave.DataFloat) != 0 || len(pdata.Wave.SpectrumFloat) != 0 || len(pdata.Wave.SpectrumEnvelopeFloat) != 0 {
				pdata.Wave.DataUUID = pdata.UUID
				tx.Table("wave_rpm_"+fid).Where("data_uuid=?", pdata.UUID).Select("id").Scan(&pdata.Wave)
				if pdata.Wave.ID == 0 {
					err = tx.Table("wave_rpm_" + fid).Create(&pdata.Wave).Error
					if err != nil {
						return err
					}
				} else {
					err = tx.Table("wave_rpm_"+fid).Omit("created_at").Where("id=?", pdata.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Save(&pdata.Wave).Error
					if err != nil {
						return err
					}
				}
			}
			//更新 风机最新数据时间
			err = tx.Table("point").Where("id=?", pdata.PointID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("last_data_time", pdata.Time).Error
			if err != nil {
				return err
			}
			return nil
		})
		return err
	} else {
		var bband []alert.Band
		//从标准表搜寻标准
		if bband, err = AlertSearch(db, ppmwcid, pdata.PointUUID, pdata.Measuredefine); err != nil {
			return err
		}
		//从标准表将频带写进数据库
		BandUpdate(db, &pdata, bband)
		// 数据服务 调用exe计算result并获得数据结构，存至数据库。出现错误重复循环三次。
		err = pdata.DataAnalysis_2(db, ipport, fid)
		//循环尝试三次
		for i := 0; i < 3; i++ {
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				err = pdata.DataAnalysis_2(db, ipport, fid)
			}
		}
		if err != nil {
			return err
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			if pdata.ID == 0 {
				err = tx.Table("data_" + fid).Omit(clause.Associations).Create(&pdata).Error
				if err != nil {
					return err
				}
			} else {
				err = tx.Table("data_" + fid).Omit(clause.Associations).Clauses(clause.Locking{Strength: "UPDATE"}).Save(&pdata).Error
			}
			if len(pdata.Wave.DataFloat) != 0 || len(pdata.Wave.SpectrumFloat) != 0 || len(pdata.Wave.SpectrumEnvelopeFloat) != 0 {
				pdata.Wave.DataUUID = pdata.UUID
				tx.Table("wave_"+fid).Where("data_uuid=?", pdata.UUID).Select("id").Scan(&pdata.Wave)
				if pdata.Wave.ID == 0 {
					err = tx.Table("wave_" + fid).Create(&pdata.Wave).Error
					if err != nil {
						return err
					}
				} else {
					err = tx.Table("wave_" + fid).Clauses(clause.Locking{Strength: "UPDATE"}).Save(&pdata.Wave).Error
					if err != nil {
						return err
					}
				}
			}
			//更新 风机最新数据时间
			var ptime time.Time
			err = tx.Table("point").Where("id=?", pdata.PointID).Pluck("last_data_time", &ptime).Error
			if err != nil {
				return err
			}
			if ptime.Unix() < pdata.TimeSet {
				err = tx.Table("point").Where("id=?", pdata.PointID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("last_data_time", pdata.Time).Error
				if err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err != nil {
		return err
	}

	//新开协程, 执行频带幅值、故障树报警
	go func() {
		//TODO DEBUG 报警 报警go进程，不耽误数据导入？
		err = DataAlert_2(ddb, pdata, fid, ipport)
		if err != nil {
			modlog.Error("频带报警出错。err:" + err.Error())
		}

	}()

	return nil
}

//报警，也要更新日报、月报
//func DataAlert(db *gorm.DB, pdata Data, ppmwcid []string, bband []alert.Band) (err error) {
//	var maxlevel uint8 = 1
//	// 频带幅值报警并更新至alert中
//	mlevel, err := BandAlertSet(db, pdata, ppmwcid[2], bband)
//	if err != nil {
//		return err
//	}
//	if mlevel > maxlevel {
//		maxlevel = mlevel
//	}
//	//故障树报警并更新至alert中
//	mlevel, err = TreeAlert(db, dataidstr, pid, exepath)
//	if err == nil {
//		if mlevel > maxlevel {
//			maxlevel = mlevel
//		}
//	} else {
//		if err != nil && err.Error() != "empty tree version" {
//			return err
//		}
//	}
//	// 报警后的风机状态字段更新
//	if err = StatusUpdate(db, pdata.TimeSet, ppmwcid, maxlevel); err != nil {
//		return err
//	}
//	return
//}

func DataAlert_2(db *gorm.DB, pdata Data, fid string, ipport string) (err error) {
	//ppmwcid: point part machine windfield company id
	ppmwcid, _, _, err := PointtoFactory(db, pdata.PointID)
	if err != nil {
		return err
	}
	// 查询风机报警使能，然后执行不同的报警策略
	var machine Machine
	err = db.Table("machine").Where("id=?", fid).First(&machine).Error
	if err != nil {
		modlog.Error("风机查找错误。err:" + err.Error())
	}
	//每种自动报警的最大状态
	var levels []uint = []uint{1}
	tagIdsOfAlert := make([]int, 0)
	// TODO 频带自动报警
	if machine.BandAlertSet {
		a, tagIds, err := BandAlertSet_2(db, pdata, ppmwcid, fid)
		if err != nil {
			return err
		}
		levels = append(levels, a)
		tagIdsOfAlert = append(tagIdsOfAlert, tagIds...)
	}
	// TODO 故障树自动报警
	if machine.TreeAlertSet {
		mlevel, err, tagIds := TreeAlert(db, pdata, ipport)
		if err != nil {
			return err
		}
		levels = append(levels, uint(mlevel))
		tagIdsOfAlert = append(tagIdsOfAlert, tagIds...)
	}
	tagStr := IntArrayToString(db, tagIdsOfAlert)
	_, maxlevel := MaxStatus(levels)
	//* 报警后的风机状态字段更新
	err = db.Transaction(func(tx *gorm.DB) error {
		// 首先更新数据状态，以及tag
		db.Table("data_"+fid).Where("id=?", pdata.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Updates(&Data{Status: uint8(maxlevel), Tag: tagStr})
		// 更新上级状态
		if err = StatusUpdate(tx, pdata.TimeSet, ppmwcid, uint8(maxlevel)); err != nil {
			return err
		}
		//* 数据正常时的计数
		if maxlevel == 1 {
			var parttype string
			db.Table("part").Where("id=?", ppmwcid[1]).Pluck("type", &parttype)
			var atemp Alert
			atemp.Level = 1
			atemp.PointID = pdata.PointID
			atemp.TimeSet = pdata.TimeSet
			atemp.Location = parttype
			err = UpdateReportAfterAlert(tx, atemp)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return
}

// ^ 查询数据
func FindDataHistory(db *gorm.DB, c echo.Context, datatable string, m Limit, fid string, id string) (f interface{}, err error) {
	var d []Datainfo
	if m.Starttime == "" {
		m.Starttime = "1900-01-01 00:00:00"
	}
	stamp1, err := StrtoTime("2006-01-02 15:04:05", m.Starttime)
	if err != nil {
		return f, err
	}
	if m.Endtime == "" {
		m.Endtime = "9999-01-01 00:00:00"
	}
	stamp2, err := StrtoTime("2006-01-02 15:04:05", m.Endtime)
	if err != nil {
		return f, err
	}
	if m.MaxRpm == 0 {
		m.MaxRpm = 999999
	}
	DataLimit = &m.LimitCondition

	var point Point
	db.Table("point").Where("id=?", id).Select("uuid").First(&point)
	sub := db.Table(datatable+fid).Where("point_uuid=?", point.UUID).
		Where("time_set BETWEEN ? AND ?", stamp1, stamp2).
		Where("rpm BETWEEN ? AND ?", m.MinRpm, m.MaxRpm)
	if m.Freq != "" {
		sub = sub.Where("sample_freq =?", m.Freq)
	}

	if m.Datatype != "" {
		sub.Where("datatype =?", m.Datatype)
	}
	if m.Measuredefine != "" {
		sub.Where("measuredefine =?", m.Measuredefine)
	}
	// 新增数据标签模糊查询, tag字段
	if m.Tag != "" {
		sub.Where(fmt.Sprintf(" `tag` LIKE '%%%s%%'", m.Tag))
	}
	sub = sub.Order("time_set DESC")
	var count int64
	sub.Count(&count)
	err = sub.Scopes(Paginate(c.Request())).Find(&d).Error
	if err != nil {
		return f, err
	}
	for k := range d {
		d[k].Time = TimetoStr(d[k].TimeSet).Format("2006-01-02 15:04:05")
	}

	type returnpage struct {
		Count    int64      `json:"count,string"`
		Children []Datainfo `json:"children"`
	}
	var r returnpage
	r.Count = count
	r.Children = d
	f = r
	return r, err
}
