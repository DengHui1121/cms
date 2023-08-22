package mod

import (
	"main/alert"
	"strconv"

	"gorm.io/gorm"
)

func (src *Factory) BeforeDelete(tx *gorm.DB) (err error) {
	tx.Preload("Windfarms").Last(&src)
	if len(src.Windfarms) != 0 {
		for k := range src.Windfarms {
			err = tx.Table("windfarm").Unscoped().Delete(&Windfarm{}, src.Windfarms[k].ID).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (src *Windfarm) BeforeDelete(tx *gorm.DB) (err error) {
	tx.Preload("Machines").Last(&src)
	if len(src.Machines) != 0 {
		for _, v := range src.Machines {
			err = tx.Table("machine").Unscoped().Delete(&v, v.ID).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func ChecktoDropTable(db *gorm.DB, name string) error {
	var err error
	if db.Migrator().HasTable(name) {
		err = db.Migrator().DropTable(name)
	}
	return err
}
func (src *Machine) BeforeDelete(tx *gorm.DB) (err error) {
	err = tx.Preload("Parts").Last(src).Error
	if err != nil {
		return
	}
	if len(src.Parts) != 0 {
		for _, v := range src.Parts {
			err = tx.Table("part").Unscoped().Delete(&v, v.ID).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (src *Part) BeforeDelete(tx *gorm.DB) (err error) {
	err = tx.Preload("Points").Preload("Properties").Preload("Bands").Last(src).Error
	if err != nil {
		return err
	}
	if len(src.Properties) != 0 {
		for _, v := range src.Properties {
			err = tx.Table("property").Unscoped().Delete(&v, v.ID).Error
			if err != nil {
				return err
			}
		}
	}

	if len(src.Bands) != 0 {
		for _, v := range src.Bands {
			err = tx.Table("band").Unscoped().Delete(&v, v.ID).Error
			if err != nil {
				return err
			}
		}
	}
	if len(src.Points) != 0 {
		for _, v := range src.Points {
			err = tx.Table("point").Unscoped().Delete(&v, v.ID).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (src *Point) BeforeDelete(tx *gorm.DB) (err error) {
	if src.ID != 0 {
		tx.Preload("Bands").Preload("Properties").Last(&src)
		if len(src.Properties) != 0 {
			for _, v := range src.Properties {
				err = tx.Table("property").Unscoped().Delete(&v, v.ID).Error
				if err != nil {
					return err
				}
			}
		}
		if len(src.Bands) != 0 {
			for _, v := range src.Bands {
				err = tx.Table("band").Unscoped().Delete(&v, v.ID).Error
				if err != nil {
					return err
				}
			}
		}
		ppmwcid, _, _, err := PointtoFactory(tx, src.ID)
		fid := ppmwcid[2]
		if err != nil {
			return err
		}
		dname := "data_" + fid
		wname := "wave_" + fid
		var dataid []Data
		tx.Table(dname).Where("point_uuid = ?", src.UUID).Select("id", "uuid", "point_uuid").Find(&dataid)
		if len(dataid) != 0 {
			for _, v := range dataid {
				err = tx.Table(dname).Unscoped().Delete(&v, v.ID).Error
				if err != nil {
					return err
				}
				err = tx.Table(wname).Unscoped().Where("data_uuid=?", v.UUID).Delete(&v).Error
				if err != nil {
					return err
				}
			}
		}
		rpmname := "data_rpm_" + fid
		rpmwname := "wave_rpm_" + fid

		var rpmdataid []Data
		tx.Table(rpmname).Where("point_uuid = ?", src.UUID).Select("id", "uuid", "point_uuid").Find(&rpmdataid)
		if len(rpmdataid) != 0 {
			for _, v := range dataid {
				err = tx.Table(rpmname).Unscoped().Delete(&v, v.ID).Error
				if err != nil {
					return err
				}
				err = tx.Table(rpmwname).Unscoped().Where("data_uuid=?", v.UUID).Delete(&v).Error
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (src *Alert) BeforeDelete(tx *gorm.DB) (err error) {
	tx.Table("band_alert").Where("alert_uuid =?", src.UUID).Unscoped().Delete(&alert.BandAlert{})
	tx.Table("tree_alert").Where("alert_uuid =?", src.UUID).Unscoped().Delete(&alert.TreeAlert{})
	tx.Table("manual_alert").Where("alert_uuid =?", src.UUID).Unscoped().Delete(&alert.ManualAlert{})
	//计数
	err = RollbackReportAfterDelet(tx, *src)
	if err != nil {
		return err
	}
	return nil
}
func (src *Alert) AfterCreate(tx *gorm.DB) (err error) {
	defer func() {
		if src.Source == 0 {
			Alertmessage <- src
		}
	}()
	err = UpdateReportAfterAlert(tx, *src)
	if err != nil {
		return err
	}
	return nil
}

func (src *Data) BeforeDelete(tx *gorm.DB) (err error) {
	ppmwcid, _, _, err := PointtoFactory(tx, src.PointID)
	fid := ppmwcid[2]
	if err != nil {
		return err
	}
	var c int64
	tx.Table("data_"+fid).Where("id=?", src.ID).Count(&c)
	if c != 0 {
		tx.Table("data_"+fid).Where("id=?", src.ID).First(src)
	} else {
		return nil
	}
	if src.TimeSet != 0 {
		if src.PointUUID != "" {
			var alertid []Alert
			tx.Table("alert").Where("data_uuid = ?", src.UUID).Find(&alertid)
			if len(alertid) != 0 {
				for _, v := range alertid {
					err = tx.Table("alert").Where("id = ?", v.ID).Unscoped().Delete(&v).Error
					if err != nil {
						return err
					}
				}
			} else {
				var atemp Alert
				atemp.Level = 1
				atemp.PointID = src.PointID
				atemp.TimeSet = src.TimeSet
				// atemp.PartType = parttype
				err = RollbackReportAfterDelet(tx, atemp)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

//按风机分数据表
func (src *Machine) AfterCreate(tx *gorm.DB) (err error) {
	tblpre := strconv.FormatUint(uint64(src.ID), 10)
	tx.Table("data_" + tblpre).AutoMigrate(&Data_Update{})
	tx.Table("data_rpm_" + tblpre).AutoMigrate(&Data_Update{})
	tx.Table("wave_" + tblpre).AutoMigrate(&Wave_Update{})
	tx.Table("wave_rpm_" + tblpre).AutoMigrate(&Wave_Update{})
	return nil
}
