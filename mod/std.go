package mod

import (
	"encoding/json"
	"errors"
	"main/alert"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//测点和特征值的标准文件读取
type Parts struct {
	Version string
	Part    []Part
}

func (pointinfo *Parts) PointGet(src multipart.File, db *gorm.DB, opt string) error {
	var err error
	_, err = toml.NewDecoder(src).Decode(&pointinfo)
	if err != nil {
		return err
	}
	//重复
	if len(pointinfo.Part) != 0 {
		if opt == "check" {
			var count int64
			db.Table("point_std").Select("version").Where("deleted_at IS NULL AND version=?", pointinfo.Version).Count(&count)
			if count != 0 {
				return errors.New("repeat")
			}

		}
		err = db.Transaction(func(tx *gorm.DB) error {
			back := pointinfo.Version + "_" + strconv.Itoa(int(time.Now().Local().Unix()))
			err = tx.Table("point_std").Where("version=?", pointinfo.Version).Clauses(clause.Locking{Strength: "UPDATE"}).Update("version", back).Error

			for _, v := range pointinfo.Part {
				for _, vv := range v.Points {
					var pstd PointStd
					pstd.PartType = v.Name
					pstd.Name = vv.Name
					pstd.Version = pointinfo.Version
					pstd.Direction = vv.Direction
					if err = tx.Table("point_std").Create(&pstd).Error; err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
	return nil
}

func (propertyinfo *Parts) PropertyGet(src multipart.File, db *gorm.DB, opt string) error {
	var err error
	_, err = toml.NewDecoder(src).Decode(&propertyinfo)
	if err != nil {
		return err
	}
	if len(propertyinfo.Part) != 0 {
		if opt == "check" {
			var count int64
			db.Table("property_std").Select("version").Where("deleted_at IS NULL AND version=?", propertyinfo.Version).Count(&count)
			if count != 0 {
				return errors.New("repeat")
			}
		}
		err = db.Transaction(func(tx *gorm.DB) error {
			back := propertyinfo.Version + "_" + strconv.Itoa(int(time.Now().Local().Unix()))
			err = tx.Table("property_std").Where("version=?", propertyinfo.Version).Clauses(clause.Locking{Strength: "UPDATE"}).Update("version", back).Error
			for _, v := range propertyinfo.Part {
				for _, vv := range v.Properties {
					var ppstd PropertyStd
					ppstd.PartType = v.Name
					ppstd.Name = vv.Name
					//! 英文名统一存为大写，便于比较
					ppstd.NameEn = strings.ToUpper(vv.NameEn)
					ppstd.Formula = vv.Formula
					ppstd.Value = vv.Value
					ppstd.Version = propertyinfo.Version
					if err = tx.Table("property_std").Create(&ppstd).Error; err != nil {
						return err
					}
				}
			}
			return nil
		})
	}
	return nil
}

func (fanstd *MachineStd) Get(src multipart.File, db *gorm.DB, opt string) error {
	var err error
	var fan Machine
	_, err = toml.NewDecoder(src).Decode(&fan)
	if err != nil {
		return err
	}
	if fan.Parts == nil {
		fan.Parts = make([]Part, 0)
	} else {
		for k := range fan.Parts {
			if fan.Parts[k].Points == nil {
				fan.Parts[k].Points = make([]Point, 0)
			} else {
				for kk := range fan.Parts[k].Points {
					if fan.Parts[k].Points[kk].Properties == nil {
						fan.Parts[k].Points[kk].Properties = make([]Property, 0)
					}
					if fan.Parts[k].Points[kk].Bands == nil {
						fan.Parts[k].Points[kk].Bands = make([]alert.Band, 0)
					} else {
						for bk, bv := range fan.Parts[k].Points[kk].Bands {
							fan.Parts[k].Points[kk].Bands[bk].FloorStd = bv.Floor.Std
							fan.Parts[k].Points[kk].Bands[bk].UpperStd = bv.Upper.Std
						}
					}
				}
			}
			if fan.Parts[k].Properties == nil {
				fan.Parts[k].Properties = make([]Property, 0)
			}
			if fan.Parts[k].Bands == nil {
				fan.Parts[k].Bands = make([]alert.Band, 0)
			} else {
				for bk, bv := range fan.Parts[k].Bands {
					fan.Parts[k].Bands[bk].FloorStd = bv.Floor.Std
					fan.Parts[k].Bands[bk].UpperStd = bv.Upper.Std
				}
			}
		}
	}

	fanstd.Version = fan.FanVersion
	fanstd.Desc = fan.Desc
	fanstd.Set, err = json.Marshal(fan)
	if err != nil {
		return err
	}
	if opt == "check" {
		var count int64
		db.Table("machine_std").Select("version").Where("version=?", fanstd.Version).Count(&count)
		if count != 0 {
			return errors.New("repeat")
		} else {
			err = db.Table("machine_std").Create(fanstd).Error
			if err != nil {
				return err
			}
			return nil
		}
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		back := fanstd.Version + "_" + strconv.Itoa(int(time.Now().Local().Unix()))
		oldversion := fanstd.Version
		fanstd.Version = back
		err = tx.Table("machine_std").Where("version=?", oldversion).Create(fanstd).Error
		if err != nil {
			return err
		}
		return nil
	})
	return nil
}
