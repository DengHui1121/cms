package alert

import (
	"errors"
	"io"
	"strconv"
	"time"

	"github.com/BurntSushi/toml"
	"gorm.io/gorm"
)

func (a *Alert) AlertGet(file io.Reader, db *gorm.DB, opt string) error {
	_, err := toml.NewDecoder(file).Decode(a)
	if err != nil {
		return err
	}
	//查询重复
	if a.Version == "" {
		return errors.New("标准配置文件错误")
	}
	if opt == "check" {
		if len(a.Band) != 0 {
			var count int64
			db.Table("band").Where("deleted_at IS NULL").Select("version").Where("version=?", a.Version).Count(&count)
			if count != 0 {
				return errors.New("repeat")
			}
		}
	}
	// 对频带报警配置的导入
	if len(a.Band) != 0 {
		err = db.Transaction(func(tx *gorm.DB) error {
			back := a.Version + "_" + strconv.Itoa(int(time.Now().Local().Unix()))
			err = tx.Table("band").Where("version=?", a.Version).Update("version", back).Error
			if err != nil {
				return err
			}
			for _, v := range a.Band {
				// v.Version = a.Version
				if err = tx.Table("band").Create(&v).Error; err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	return nil
}
