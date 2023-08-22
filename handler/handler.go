package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"main/alert"
	"main/mod"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func ErrCheck(c echo.Context, returnData mod.ReturnData, err error, info string) {
	returnData.Code = http.StatusBadRequest
	returnData.Message = info + "。err:" + err.Error()
	returnData.Data = nil
	mainlog.Error(returnData.Message)
	c.JSON(200, returnData)
}
func WarnCheck(c echo.Context, returnData mod.ReturnData, err error, info string) {
	returnData.Code = http.StatusBadRequest
	returnData.Message = info + "。err:" + err.Error()
	returnData.Data = nil
	mainlog.Warn(returnData.Message)
	c.JSON(200, returnData)
}
func ErrNil(c echo.Context, returnData mod.ReturnData, d interface{}, info string) {
	returnData.Code = http.StatusOK
	returnData.Data = d
	returnData.Message = info
	c.JSON(200, returnData)
}

//*登录
func Login(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var mm mod.User
	err = c.Bind(&mm)
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由body解析失败")
		return err
	}
	if mm.Username == "visitor" {
		ErrNil(c, returnData, mod.User{Username: "visitor", Level: 3}, "登录成功")
		return nil
	} else {
		var existuser mod.User
		ra := db.Table("user").Where("username =?", mm.Username).
			Select("id", "username", "password", "level").
			Scan(&existuser).RowsAffected
		if ra == 0 {
			err = errors.New("wrong username")
			ErrNil(c, returnData, err, "账号名错误")
			return err
		}
		if existuser.Password != mm.Password {
			err = errors.New("wrong password")
			ErrNil(c, returnData, err, "密码错误")
			return err
		}
		ErrNil(c, returnData, mod.PublicUser{User: &existuser}, "登录成功")
		return err
	}
}

//*修改账号
func UserOption(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var mm mod.User
	err = c.Bind(&mm)
	opt := c.Param("type")
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由body解析失败")
		return err
	}
	var f interface{}
	var existuser mod.User

	switch opt {
	case "info":
		if existuser.Password != mm.Password {
			err = db.Table("user").Where("id=?", mm.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("password", mm.Password).Error
		} else {
			err = errors.New("password wrong")
			ErrCheck(c, returnData, err, "密码与原密码相同")
			return err
		}
	case "add":
		if mm.Level == 0 || mm.Password == "" || mm.Username == "" {
			err = errors.New("missing information")
			ErrCheck(c, returnData, nil, "账号信息不完整")
			return err
		}
		ra := db.Table("user").Where("username =?", mm.Username).
			Select("id", "username", "password", "level").
			Scan(&existuser).RowsAffected
		if ra == 0 {
			err = db.Table("user").Create(&mm).Error
		} else {
			err = errors.New("username exists")
			ErrCheck(c, returnData, err, "已有该账号名")
			return err
		}

	case "delete":

		err = db.Table("user").Unscoped().Delete(&mod.User{}, mm.ID).Error
	case "list":
		l := c.QueryParam("level")
		var userlist []mod.User
		var publicuserlist []mod.PublicUser

		err = db.Table("user").Where("level > ?", l).
			Select("id", "username", "level").
			Scan(&userlist).Error
		for k := range userlist {
			publicuserlist = append(publicuserlist, mod.PublicUser{User: &userlist[k]})
		}
		f = publicuserlist
	case "logout":
		c.QueryParam("id")
		// err = db.Table("user").Where("id=?", id).Updates(map[string]interface{}{
		// 	"status": gorm.Expr("status-?", 1)}).Error
	}
	if err != nil {
		ErrCheck(c, returnData, err, "操作失败")
		return err
	}
	ErrNil(c, returnData, f, "操作成功")
	return err
}

//* 标准文件读取。相同版本号的直接覆盖。
func StdFileUpload(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	i := c.Param("type")
	opt := c.Param("option")
	file, err := c.FormFile("stdfile")
	if err != nil {
		ErrCheck(c, returnData, err, "原文件上传失败")
		return err
	}
	fn := strings.Split(file.Filename, ".")
	if fn[1] != "toml" {
		err = errors.New("unsupported file type")
		ErrCheck(c, returnData, err, "不支持该文件类型")
		return err
	}
	src, err := file.Open()
	if err != nil {
		ErrCheck(c, returnData, err, "原文件打开失败")
		return err
	}
	defer src.Close()
	switch i {
	case "band", "alert":
		var a alert.Alert
		err = a.AlertGet(src, db, opt)
		if err != nil && err.Error() == "repeat" {
			ErrNil(c, returnData, true, "文件版本重复")
			return err
		}
	case "measuringPoint":
		var pointinfo mod.Parts
		err = pointinfo.PointGet(src, db, opt)
		if err != nil && err.Error() == "repeat" {
			ErrNil(c, returnData, true, "文件版本重复")
			return err
		}
	case "characteristic":
		var propertyinfo mod.Parts
		err = propertyinfo.PropertyGet(src, db, opt)
		if err != nil && err.Error() == "repeat" {
			ErrNil(c, returnData, true, "文件版本重复")
			return err
		}
	case "fan":
		fanstd := new(mod.MachineStd)
		err = fanstd.Get(src, db, opt)
		if err != nil && err.Error() == "repeat" {
			ErrNil(c, returnData, true, "文件版本重复")
			return err
		}
	}
	if err != nil {
		ErrCheck(c, returnData, err, file.Filename+" 文件读取失败")
		return err
	}
	ErrNil(c, returnData, false, "文件读取成功")
	return err
}

func StdUpdate(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var stdinfo mod.MachineStd
	c.Bind(&stdinfo)
	err = db.Table("machine_std").Where("id=?", stdinfo.ID).
		Clauses(clause.Locking{Strength: "UPDATE"}).Updates(stdinfo).Error
	if err != nil {
		ErrCheck(c, returnData, err, "update fail")
	}
	ErrNil(c, returnData, nil, "update success")
	return nil
}

//* api/v1/structure
func FindAll(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var f []mod.Factory
	err = db.Table("factory").Preload("Windfarms.Machines.Parts.Points").Find(&f).Error
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
		return err
	}

	for _, v := range f {
		for _, wv := range v.Windfarms {
			for _, mv := range wv.Machines {
				for _, pv := range mv.Parts {
					for _, ptv := range pv.Points {
						var newdata mod.Data
						db.Table(fmt.Sprint("data_", mv.ID)).Where("point_uuid=?", ptv.UUID).Order("time_set desc").
							Select("id", "status", "time_set").Limit(1).Scan(&newdata)
						if newdata.ID != 0 {
							ptv.LastDataTime = mod.TimetoStr(newdata.TimeSet)
							db.Table("point").Where("id=?", ptv.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("last_data_time", ptv.LastDataTime)
						} else {
							ptv.LastDataTime = time.Date(2000, 01, 01, 00, 00, 00, 00, time.Local)
							db.Table("point").Where("id=?", ptv.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("last_data_time", ptv.LastDataTime)
						}
						tn := time.Now().AddDate(0, 0, -3)
						if ptv.LastDataTime.Before(tn) {
							db.Table("point").Where("id=?", ptv.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("status", 0)
							ptv.Status = 0
						} else {
							db.Table("point").Where("id=?", ptv.ID).Clauses(clause.Locking{Strength: "UPDATE"}).Update("status", newdata.Status)
							ptv.Status = newdata.Status
						}
						mod.StatusCheck(ptv.ID, "part", "point", db)
					}

				}

			}
		}
	}
	err = db.Table("factory").Preload("Windfarms.Machines.Parts.Points").Find(&f).Error
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
		return err
	}
	ErrNil(c, returnData, f, "成功查询")
	return err
}

//* api/v1/xx?id=
func FindTree(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	id := c.QueryParam("id")
	i := c.Param("type")
	var f interface{}
	switch i {
	case "company":
		if id != "" {
			var ff mod.Factory
			err = db.Table("factory").Omit("created_at", "updated_at").
				// Where("id=? AND branch_id=?", iid, bid).
				Last(&ff, id).Error
			f = ff
		} else {
			var ff []mod.Factory
			err = db.Table("factory").Omit("created_at", "updated_at").Find(&ff).Error
			f = ff
		}

	case "windFields":
		var ff mod.Factory
		err = db.Table("factory").Omit("created_at", "updated_at").Preload(clause.Associations).
			Find(&ff, id).Error
		f = ff.Windfarms
	case "windField":
		var ff mod.Windfarm
		err = db.Table("windfarm").Omit("created_at", "updated_at").
			Last(&ff, id).Error
		var Longitudestr, Latitudestr string
		if ff.Longitude == float32(int32(ff.Longitude)) {
			if ff.Longitude == 0 {
				Longitudestr = ""
			} else {
				Longitudestr = fmt.Sprintf("%.1f", ff.Longitude)
			}
		} else {
			Longitudestr = fmt.Sprint(ff.Longitude)
		}
		if ff.Latitude == float32(int32(ff.Latitude)) {
			if ff.Latitude == 0 {
				Latitudestr = ""
			} else {
				Latitudestr = fmt.Sprintf("%.1f", ff.Latitude)
			}
		} else {
			Latitudestr = fmt.Sprint(ff.Latitude)
		}
		t := struct {
			*mod.Windfarm
			Longitudestr string `json:"longitude"`
			Latitudestr  string `json:"latitude"`
		}{
			&ff,
			Longitudestr,
			Latitudestr,
		}
		f = t
	case "fans":
		var ff mod.Windfarm
		err = db.Table("windfarm").Omit("created_at", "updated_at").Preload(clause.Associations).
			Find(&ff, id).Error
		f = ff.Machines
	case "fan":
		var ff mod.Machine
		err = db.Table("machine").Omit("created_at", "updated_at").Preload("Parts.Properties").Preload("Parts.Bands").Preload("Parts.Points.Properties").Preload("Parts.Points.Bands").
			Last(&ff, id).Error
		bt, err := time.ParseInLocation("2006-01-02", ff.BuiltTime, time.Local)
		if err != nil {
			f = ff
			break
		}
		nt := time.Now()
		gap := nt.Sub(bt).Hours()
		ff.Health = 1 - gap/24/365/20
		f = ff
	case "parts":
		var ff mod.Machine
		err = db.Table("machine").Omit("created_at", "updated_at").
			Preload("Parts.Properties").Preload("Parts.Bands").
			Preload("Parts.Points.Properties").Preload("Parts.Bands").
			Where("id=?", id).
			Last(&ff).Error
		f = ff
	case "search":
		var ppmwcid []string
		ppmwcid, _, _, err = mod.PointtoFactory(db, id)
		if err != nil {
			ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
			return err
		}
		fid := ppmwcid[2]
		var ai mod.AlertInfo
		ai.SearchBox = c.QueryParam("search_box")
		var temp []string = make([]string, 0)
		err = db.Table("data_" + fid).Group(ai.SearchBox).Select(ai.SearchBox).Scan(&temp).Error
		if err != nil {
			break
		}
		ai.Options = temp
		f = ai.Options
	case "history":
		//历史数据
		var m mod.Limit
		var ppmwcid []string
		ppmwcid, _, _, err = mod.PointtoFactory(db, id)
		if err != nil {
			break
		}
		fid := ppmwcid[2]
		err = c.Bind(&m)
		if err != nil {
			ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由解析失败")
			return err
		}
		f, err = mod.FindDataHistory(db, c, "data_", m, fid, id)
		if err != nil {
			break
		}
	case "rpmhistory":
		var m mod.Limit
		var ppmwcid []string
		ppmwcid, _, _, err = mod.PointtoFactory(db, id)
		if err != nil {
			break
		}
		fid := ppmwcid[2]
		err = c.Bind(&m)
		if err != nil {
			ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由解析失败")
			return err
		}
		f, err = mod.FindDataHistory(db, c, "data_rpm_", m, fid, id)
		if err != nil {
			break
		}
	case "info":
		var m mod.PointInfo
		pid := c.QueryParam("id")
		ppmwfid, pmwname, _, err := mod.PointtoFactory(db, pid)
		if err != nil {
			break
		}
		m.PointName = pmwname[0]
		db.Table("part").Where("id=?", ppmwfid[1]).Pluck("type", &m.PartName)
		m.MachineName = pmwname[2]
		m.WindfarmName = pmwname[3]
		m.FactoryName = pmwname[4]

		f = m
	case "alerts":
		type Tempalerts struct {
			Count    int64       `json:"count,string"`
			Children []mod.Alert `json:"children"`
		}
		var t Tempalerts
		var ff []mod.Alert
		var m mod.Limit
		c.Bind(&m)
		source := c.QueryParam("source")
		if source != "" {
			source, _ := strconv.Atoi(source)
			m.Source = &source
		}
		if err != nil {
			ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由解析失败")
			return err
		}
		ff, t.Count, err = mod.AlertsSearch(db, m, c)
		if err != nil {
			break
		}
		t.Children = ff
		f = t
	}
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
		return err
	}
	ErrNil(c, returnData, f, "成功查询")
	return nil
}

func FindAlert(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	id := c.QueryParam("id")
	i := c.Param("type")
	var f interface{}
	switch i {
	case "band":
		//获取频带报警详细信息
		var balert alert.BandAlert
		balert, err = mod.BandAlertDetail(db, id)
		f = balert
	case "tree":
		//获取故障树详细信息
		var talert alert.TreeAlert
		talert, err = mod.TreeAlertDetail(db, id)
		f = talert
	case "normal":
		var talert alert.ManualAlert
		talert, err = mod.ManualAlertDetail(db, id)
		f = talert
	case "options":
		var ai mod.AlertInfo
		ai.SearchBox = c.QueryParam("search_box")
		var temp []string = make([]string, 0)

		sub := db.Table("alert").Group(ai.SearchBox).Select(ai.SearchBox)
		if ai.SearchBox == "type" {
			sub.Not("type =? OR type = ?", "故障树", "频带幅值").Scan(&temp)
			temp = append(temp, "故障树", "频带幅值")
		} else {
			sub.Scan(&temp)
		}
		ai.Options = temp
		f = ai.Options
	case "search":
		var ppmwcid []string
		ppmwcid, _, _, err = mod.PointtoFactory(db, id)
		if err != nil {
			ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
			return err
		}
		fid := ppmwcid[2]
		var ai mod.AlertInfo
		ai.SearchBox = c.QueryParam("search_box")
		var temp []string = make([]string, 0)
		err = db.Table("data_" + fid).Group(ai.SearchBox).Select(ai.SearchBox).Scan(&temp).Error
		if err != nil {
			break
		}
		ai.Options = temp
		f = ai.Options
	}

	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
		return err
	}
	ErrNil(c, returnData, f, "成功查询")
	return nil
}
func PostDataLimit(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	//历史数据
	if err != nil {
		ErrCheck(c, returnData, err, "创建失败")
		return err
	}
	ErrNil(c, returnData, nil, "成功创建")
	return nil
}

//* 查找某一级下一级的所有内容
func FindInfo(c echo.Context) error {
	var dst interface{}
	var table string
	var err error
	returnData := mod.ReturnData{}
	i := c.Param("type")
	switch i {
	case "company":
		dst = new([]mod.Factory)
		table = "factory"
	case "windFields":
		dst = new([]mod.Windfarm)
		table = "windfarm"
	}
	err = db.Table(table).Omit("created_at", "updated_at").Preload(clause.Associations).Find(dst).Error
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
		return err
	}
	ErrNil(c, returnData, dst, "成功查找")
	return nil
}

//* api/v1/xx/:id
func UpdateInfo(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var count int64
	mm := make(map[string]interface{})
	var mmt interface{}

	id := c.QueryParam("id")
	// iid, bid := GetId(id)
	// uiid, _ := strconv.ParseUint(iid, 10, 64)
	err = json.NewDecoder(c.Request().Body).Decode(&mm)
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由body解析失败")
		return err
	}
	mm["id"] = id
	i := c.Param("type")
	var table string
	switch i {
	case "company":
		var m mod.Factory
		mod.MaptoStruct(mm, &m)
		//* 识别重复
		db.Table("factory").
			Not("id = ?", id).
			Select("name").
			Where("name = ?", m.Name).Count(&count)
		mmt = m
		table = "factory"
	case "windField":
		var m mod.Windfarm
		mod.MaptoStruct(mm, &m)
		var parent string
		db.Table("factory").Where("id = ? ", m.FactoryID).Select("uuid").Scan(&parent)
		//* 识别重复
		db.Table("windfarm").
			Not("id = ?", id).
			Where("factory_uuid = ? AND name = ?", parent, m.Name).Count(&count)
		mmt = m
		table = "windfarm"

	case "fan":
		var m mod.Machine
		mod.MaptoStruct(mm, &m)
		var parent string
		db.Table("windfarm").Where("id = ? ", m.WindfarmID).Select("uuid").Scan(&parent)
		db.Table("machine").
			Not("id = ?", id).
			Where("windfarm_uuid = ? AND name = ?", parent, m.Name).Count(&count)
		mmt = m
		table = "machine"
	}
	if count != 0 {
		err = errors.New("existing name")
	} else {
		if p, ok := mmt.(mod.Machine); ok {
			err = db.Transaction(func(tx *gorm.DB) error {
				//! 特殊 针对风机更新的事务（部件 测点和特征值的更新）
				err = mod.MachineUpdate(tx, p)
				return err
			})
		} else {
			err = db.Table(table).Omit(clause.Associations).Omit("status").
				Clauses(clause.Locking{Strength: "UPDATE"}).Updates(mmt).Error
		}
	}
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 更新信息失败")
		return err
	}
	ErrNil(c, returnData, nil, "成功更新")
	return nil
}

//TODO 修改报警详细信息
func UpdateAlert(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	var m mod.Alert
	c.Bind(&m)
	err = db.Table("alert").Where("id=?", m.ID).
		Select("level", "strategy", "desc", "source", "suggest", "handle").Clauses(clause.Locking{Strength: "UPDATE"}).
		Updates(m).Error
	if err != nil {
		ErrCheck(c, returnData, err, "更新失败")
	}
	ErrNil(c, returnData, nil, "更新成功")
	return err
}
func InsertInfo(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var mmt interface{}
	mm := make(map[string]interface{})
	err = json.NewDecoder(c.Request().Body).Decode(&mm)
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由body解析失败")
		return err
	}
	var count int64
	var desccount int64
	i := c.Param("type")
	var table string
	switch i {
	case "company":
		var m mod.Factory
		mod.MaptoStruct(mm, &m)
		db.Table("factory").Select("name").Where("name = ?", m.Name).Count(&count)
		mmt = &m
		table = "factory"
	case "windField":
		var m mod.Windfarm
		mod.MaptoStruct(mm, &m)
		fmt.Println(mm)
		fmt.Println(m)
		var parent mod.Factory
		db.Table("factory").Where("id = ? ", m.FactoryID).Select("uuid").First(&parent)
		m.FactoryUUID = parent.UUID
		db.Table("windfarm").
			Where("factory_uuid = ? AND 'desc' = ?", m.FactoryUUID, m.Name).
			Count(&desccount)
		db.Table("windfarm").
			Where("factory_uuid = ? AND name = ?", m.FactoryUUID, m.Desc).
			Count(&count)
		mmt = &m
		table = "windfarm"
	case "fan":
		var m mod.Machine
		mod.MaptoStruct(mm, &m)
		var parent mod.Windfarm
		err = db.Table("windfarm").Select("uuid").
			Where("id = ? ", m.WindfarmID).First(&parent).Error
		if err != nil {
			return err
		}
		m.WindfarmUUID = parent.UUID
		db.Table("machine").
			Where("windfarm_uuid = ? ", m.WindfarmUUID).
			Where("'desc' = ?", m.Name).
			Count(&desccount)
		db.Table("machine").
			Where("windfarm_uuid = ? AND name = ?", m.WindfarmUUID, m.Desc).
			Count(&count)
		table = "machine"
		//批量导入风机 风机前缀+序号  <10数补足十位为0
		if m.EndNum != 0 {
			for k := 0; k < m.EndNum; k++ {
				i := m.StartNum + k
				di := m.DescStartNum + k
				var istr string
				if 0 <= i && i < 10 {
					istr = fmt.Sprintf("0%v", i)
				} else {
					istr = fmt.Sprintf("%v", i)
				}
				var distr string
				if 0 <= di && di < 10 {
					distr = fmt.Sprintf("0%v", di)
				} else {
					distr = fmt.Sprintf("%v", di)
				}
				db.Table("machine").Select("windfarm_id", "desc").
					Where("windfarm_uuid = ? AND 'desc' = ?", m.WindfarmUUID, fmt.Sprintf("%v%v", m.FanFront, istr)).
					Count(&count)
				db.Table("machine").Select("windfarm_id", "name").
					Where("windfarm_uuid = ? AND name = ?", m.WindfarmUUID, fmt.Sprintf("%v%v", m.DescFront, distr)).
					Count(&desccount)
				if count == 0 && desccount == 0 {
					mtemp := new(mod.Machine)
					mod.MaptoStruct(mm, &mtemp)
					mtemp.WindfarmUUID = parent.UUID
					mtemp.Desc = fmt.Sprintf("%v%v", mtemp.FanFront, istr)
					mtemp.Name = fmt.Sprintf("%v%v", mtemp.DescFront, distr)
					if err = db.Table(table).Create(&mtemp).Error; err != nil {
						ErrCheck(c, returnData, err, "创建失败")
						return err
					}
				} else {
					continue
				}
			}
			ErrNil(c, returnData, nil, "成功创建")
			return nil
		} else {
			mmt = &m
		}
	}
	if desccount != 0 {
		err = errors.New("existing desc")
		WarnCheck(c, returnData, err, "该级下已有该编号，创建失败")
		return err
	} else if count != 0 {
		err = errors.New("existing name")
		WarnCheck(c, returnData, err, "该级下已有该名称，创建失败")
		return err
	} else {
		err = db.Transaction(func(tx *gorm.DB) error {
			if err = tx.Table(table).Create(mmt).Error; err != nil {
				return err
			}
			return nil
		})
	}
	if err != nil {
		ErrCheck(c, returnData, err, "创建失败")
		return err
	}
	ErrNil(c, returnData, nil, "成功创建")
	return nil
}

func InsertAlert(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var m mod.Alert
	c.Bind(&m)
	m.Source = 1
	if m.Desc == "" {
		m.Desc = m.Type
	}
	ppmwfid, _, _, err := mod.PointtoFactory(db, m.PointID)
	if err != nil {
		ErrCheck(c, returnData, err, "创建失败")
	}
	db.Table("part").Where("id=?", ppmwfid[1]).Pluck("name", &m.Location)
	var tempdata mod.Data
	db.Table("data_"+ppmwfid[2]).Where("id=?", m.DataID).Select("uuid", "rpm", "time_set", "point_uuid").
		First(&tempdata)
	m.DataUUID = tempdata.UUID
	m.Rpm = tempdata.Rpm
	m.TimeSet = tempdata.TimeSet
	m.PointUUID = tempdata.PointUUID
	err = db.Transaction(func(tx *gorm.DB) error {
		if err = tx.Table("alert").Create(&m).Error; err != nil {
			return err
		}
		m.ManualAlert.AlertUUID = m.UUID
		if err = tx.Table("manual_alert").Create(&m.ManualAlert).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		ErrCheck(c, returnData, err, "创建失败")
		return err
	}
	ErrNil(c, returnData, nil, "成功创建")
	return nil
}

//* 风机文件：存在即更新，不存在即导入
//* api/v1/fan/parts 导入部件
func FileUpload(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var file *multipart.FileHeader

	file, err = c.FormFile("fan_parts")
	if err != nil {
		ErrCheck(c, returnData, err, "原文件上传失败")
		return err
	}
	fn := strings.Split(file.Filename, ".")
	if fn[1] != "toml" {
		err = errors.New("unsupported file type")
		ErrCheck(c, returnData, err, "不支持该文件类型")
		return err
	}
	src, err := file.Open()
	if err != nil {
		ErrCheck(c, returnData, err, "原文件打开失败")
		return err
	}
	defer src.Close()

	m, err := mod.MachineFileUpdate(src, db)
	if err != nil {
		ErrCheck(c, returnData, err, "风机文件导入失败")
		return err
	}
	ErrNil(c, returnData, m, "文件读取成功")

	return nil
}

//* api/v1/data 导入/更新数据
func CheckMPointData(ipport string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var err error
		returnData := mod.ReturnData{}
		file, err := c.FormFile("data_upload")
		if err != nil {
			ErrCheck(c, returnData, err, "原文件上传失败")

			return err
		}
		src, err := file.Open()
		if err != nil {
			ErrCheck(c, returnData, err, "原文件打开失败")
			return err
		}
		defer src.Close()
		filetype := strings.Split(file.Filename, ".")
		var info string
		var filedata []byte
		// 判断文件类型 不同文件的导入
		info, filedata, err = mod.TypeRead(filetype[len(filetype)-1], src)
		if err != nil {
			ErrCheck(c, returnData, err, "导入文件失败")
			return err
		}
		// 找测点并导入数据库
		var pdata mod.Data
		err = pdata.DataInfoGet(db, info, filedata)
		if err != nil {
			ErrCheck(c, returnData, err, "未找到测点")
			return err
		}

		err = mod.CheckData(db, &pdata)
		if pdata.ID != 0 {
			ErrNil(c, returnData, true, "已有该数据。")
			return err
		}
		if err != nil {
			ErrCheck(c, returnData, err, "数据表查询错误")
			return err
		}
		if err = mod.InsertData(db, db, ipport, pdata); err != nil {
			ErrCheck(c, returnData, err, "导入数据出错")
			return err
		}
		ErrNil(c, returnData, false, "无该数据，导入数据成功。")
		// }
		return nil
	}
}

func OverMPointData(ipport string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var err error
		returnData := mod.ReturnData{}
		file, err := c.FormFile("data_upload")
		if err != nil {
			ErrCheck(c, returnData, err, "原文件上传失败")
			return err
		}
		src, err := file.Open()
		if err != nil {
			ErrCheck(c, returnData, err, "原文件打开失败")
			return err
		}
		defer src.Close()
		filetype := strings.Split(file.Filename, ".")
		var info string
		var filedata []byte
		// 判断文件类型 不同文件的导入
		info, filedata, err = mod.TypeRead(filetype[len(filetype)-1], src)
		if err != nil {
			ErrCheck(c, returnData, err, "导入文件失败")
			return err
		}

		// 找测点并导入数据库
		var pdata mod.Data
		err = pdata.DataInfoGet(db, info, filedata)
		if err != nil {
			ErrCheck(c, returnData, err, "未找到测点")
			return err
		}
		err = mod.CheckData(db, &pdata)
		if err != nil {
			ErrCheck(c, returnData, err, "数据表查询错误")
			return err
		}
		//! 覆盖会删除源数据相关报警信息
		err = db.Transaction(func(tx *gorm.DB) error {
			var alerttodelete []mod.Alert
			var err error
			tx.Table("alert").Where("data_uuid=?", pdata.UUID).
				Find(&alerttodelete)
			for k := range alerttodelete {
				err = tx.Table("alert").Unscoped().Delete(&alerttodelete[k]).Error
				if err != nil {
					return err
				}
			}
			if err = mod.InsertData(db, tx, ipport, pdata); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			ErrCheck(c, returnData, err, "导入数据出错")
			return err
		}
		ErrNil(c, returnData, nil, "导入数据成功")
		return nil
	}
}

//* api/v1/xx delete
func DeleteInfo(c echo.Context) error {
	var err error
	var dst interface{}
	var table string
	returnData := mod.ReturnData{}
	id := c.QueryParam("id")
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" id解析失败")
		return err
	}
	i := c.Param("type")
	var fid []string
	switch i {
	case "company":
		dst = new(mod.Factory)
		table = "factory"
		db.Table("factory").Where("factory.id=?", id).
			Joins("right join windfarm on windfarm.factory_uuid=factory.uuid").
			Joins("right join machine on windfarm.uuid=machine.windfarm_uuid").
			Select("machine.id").Find(&fid)

	case "windField":
		dst = new(mod.Windfarm)
		table = "windfarm"
		db.Table("windfarm").Where("windfarm.id=?", id).
			Joins("right join machine on windfarm.uuid=machine.windfarm_uuid").
			Select("machine.id").Find(&fid)

	case "fan":
		dst = new(mod.Machine)
		table = "machine"
		fid = append(fid, id)
	case "data":
		dst = new(mod.Data)
		pid := c.QueryParam("point_id")
		ppmwcid, _, _, err := mod.PointtoFactory(db, pid)
		if err != nil {
			ErrCheck(c, returnData, err, "测点id定位失败，删除失败。")
			return err
		}
		table = "data_" + ppmwcid[2]
	case "rpmhistory":
		dst = new(mod.Data)
		pid := c.QueryParam("point_id")
		ppmwcid, _, _, err := mod.PointtoFactory(db, pid)
		if err != nil {
			ErrCheck(c, returnData, err, "测点id定位失败，删除失败。")
			return err
		}
		table = "data_rpm_" + ppmwcid[2]
	case "measuringPoint":
		dst = new(mod.Point)
		table = "point"

	case "characteristic":
		dst = new(mod.Property)
		table = "property"
	case "alert":
		table = "alert"
		dst = new(mod.Alert)
	}
	if strings.ContainsAny(table, "_") {
		var uuid string
		db.Table(table).Last(dst, id)
		db.Table(table).Where("id=?", id).Pluck("uuid", &uuid)
		err = db.Table(table).Unscoped().Delete(dst).Error
		if err != nil {
			ErrCheck(c, returnData, err, "删除失败。")
			return err
		}
		err = db.Table(strings.Replace(table, "data", "wave", 1)).Unscoped().Where("data_uuid=?", uuid).Delete(&mod.Wave{}).Error
		if err != nil {
			return err
		}
	} else {
		db.Table(table).Last(dst, id)
		err = db.Table(table).Unscoped().Delete(dst).Error
		for _, v := range fid {
			err = mod.ChecktoDropTable(db, "data_"+v)
			if err != nil {
				return err
			}
			err = mod.ChecktoDropTable(db, "wave_"+v)
			if err != nil {
				return err
			}
			err = mod.ChecktoDropTable(db, "data_rpm_"+v)
			if err != nil {
				return err
			}
			err = mod.ChecktoDropTable(db, "wave_rpm_"+v)
			if err != nil {
				return err
			}
		}
	}

	if err != nil {
		ErrCheck(c, returnData, err, "删除失败。")
		return err
	}
	// }
	ErrNil(c, returnData, nil, "删除成功。")
	return nil
}

func DeleteStd(c echo.Context) error {
	var err error
	var table string
	var info []uint
	var model interface{}
	returnData := mod.ReturnData{}

	var mm map[string]string
	json.NewDecoder(c.Request().Body).Decode(&mm)
	i := mm["upper"]
	ii := mm["version"]

	switch i {
	case "fan":
		table = "machine_std"
		model = &mod.MachineStd{}
		ii = mm["id"]
		err = db.Transaction(func(tx *gorm.DB) error {
			err = tx.Table(table).Unscoped().Delete(model, ii).Error
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			ErrCheck(c, returnData, err, "删除错误")
		}
		ErrNil(c, returnData, nil, "删除成功")
		return nil
	case "measuringPoint":
		table = "point_std"
		model = &mod.Point{}
	case "characteristic":
		table = "property_std"
		model = &mod.Property{}
	case "band":
		table = i
		model = &alert.Band{}
	case "tree":
		table = i
		model = &alert.Tree{}
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		err = tx.Table(table).Omit("created_at", "updated_at").Where("version=?", ii).Select("id").Find(&info).Error
		if err != nil {
			return err
		}
		err = tx.Table(table).Unscoped().Delete(model, info).Error
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		ErrCheck(c, returnData, err, "删除错误")
	}
	ErrNil(c, returnData, nil, "删除成功")
	return nil
}

//* 数据绘图
func DataPlot(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	id := c.QueryParam("id")
	pid := c.QueryParam("point_id")
	datatype := c.QueryParam("datatype")
	ctype := c.QueryParam("characteristic")
	ppmwcid, _, _, err := mod.PointtoFactory(db, pid)
	if err != nil {
		ErrCheck(c, returnData, err, "测点id定位失败。")
		return err
	}
	fid := ppmwcid[2]
	var tableprefix string
	var plot mod.DatatoPlot
	if datatype == "TACH" {
		tableprefix = "rpm_"
		err = plot.Plot(db, tableprefix, fid, id)
		if err != nil {
			ErrCheck(c, returnData, err, "时频图调取数据错误")
			return err
		}
	} else {
		tableprefix = ""
		if ctype == "undefined" || ctype == "" {
			err = plot.CPlot(db, tableprefix, fid, id, "rmsvalue")
			if err != nil {
				ErrCheck(c, returnData, err, "趋势图调取数据错误")
				return err
			}
		} else if ctype != "" {
			err = plot.CPlot(db, tableprefix, fid, id, ctype)
			if err != nil {
				ErrCheck(c, returnData, err, "趋势图调取数据错误")
				return err
			}
		}
		err = plot.Plot(db, tableprefix, fid, id)
		if err != nil {
			ErrCheck(c, returnData, err, "时频图调取数据错误")
			return err
		}
	}
	//所有数据添加到datatoplot结构体中
	ErrNil(c, returnData, plot, "调取数据成功")
	return err
}

func MultiDataPlot(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	ctype := c.QueryParam("characteristic")
	mplot := c.QueryParam("current")
	mplot = "{\"current\":" + mplot + "}"
	var m mod.MultiDatatoPlot
	json.Unmarshal([]byte(mplot), &m)
	m.Plot(db, ctype)
	if err != nil {
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}
	ErrNil(c, returnData, m, "调取数据成功")
	return err
}
func GetFanDataCurrentPlot(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	fid := c.QueryParam("id")     //风机id
	ptype := c.QueryParam("type") //测点id
	var m mod.MultiDatatoPlot
	m.Currentplot = make([]mod.CurrentPlot, 0)
	//找测点和限制条件，填充m
	db.Table("machine").
		Joins("right join part on machine.uuid = part.machine_uuid").
		Joins("right join point on part.uuid = point.part_uuid").
		Where("machine.id=?", fid).Where("point.id=?", ptype).
		Select("point.id AS point_id , point.name AS legend").
		Scan(&m.Currentplot)
	m.FanStaticPlot(db, "rmsvalue", fid)
	if err != nil {
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}
	ErrNil(c, returnData, m, "调取数据成功")
	return err
}

//TODO linux系统试验
func AnalyseDataPlot(exepath string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var err error
		returnData := mod.ReturnData{}
		var m mod.AnalysetoPlot
		dataidstr := c.QueryParam("id")
		pid := c.QueryParam("point_id")
		var arg []string
		for i := 1; ; i++ {
			arg1 := c.QueryParam("arg" + strconv.Itoa(i))
			if arg1 == "" {
				break
			}
			arg = append(arg, arg1)
		}
		//传输
		var shmname string = strconv.Itoa(int(time.Now().Local().UnixNano()))
		var s mod.ShmInfo = mod.ShmInfo{Name: shmname, Count: 8000}

		ppmwcid, _, _, err := mod.PointtoFactory(db, pid)
		if err != nil {
			ErrCheck(c, returnData, err, "调取数据错误")
			return err
		}
		wid := ppmwcid[3]
		atype := c.Param("type")
		//根据不同类型算法运算 获取传回的数据
		err = m.AnalyseHandler(dbconfig, exepath, atype, wid, dataidstr, s, arg)
		if err != nil {
			ErrCheck(c, returnData, err, "调取数据错误")
			return err
		}
		ErrNil(c, returnData, m, "调取数据成功")
		return err
	}
}
func AnalyseDataPlot_2(ipport string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var err error
		returnData := mod.ReturnData{}
		var m mod.AnalysetoPlot
		dataidstr := c.QueryParam("id")
		pid := c.QueryParam("point_id")
		var arg []string
		for i := 1; ; i++ {
			arg1 := c.QueryParam("arg" + strconv.Itoa(i))
			if arg1 == "" {
				break
			}
			arg = append(arg, arg1)
		}
		ppmwcid, _, _, err := mod.PointtoFactory(db, pid)
		if err != nil {
			ErrCheck(c, returnData, err, "调取数据错误")
			return err
		}
		fid := ppmwcid[2]
		atype := c.Param("type")
		//根据不同类型算法运算 获取传回的数据
		err = m.AnalyseHandler_2(db, ipport, atype, fid, dataidstr, arg...)
		//循环尝试三次
		for i := 0; i < 3; i++ {
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				err = m.AnalyseHandler_2(db, ipport, atype, fid, dataidstr, arg...)
			}
		}
		if err != nil {
			ErrCheck(c, returnData, err, "调取数据错误")
			return err
		}
		ErrNil(c, returnData, m, "调取数据成功")
		return err
	}
}
func AnalyseDataFunc(c echo.Context) error {
	returnData := mod.ReturnData{}
	f := mod.GetAnalysisOption()
	ErrNil(c, returnData, f, "success")
	return nil
}

func FindStd(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}

	var table string
	var info interface{}
	i := c.QueryParam("upper")
	ii := c.QueryParam("version")

	switch i {
	case "fan":
		table = "machine_std"
		ii = c.QueryParam("id")
		if ii == "" {
			//查询所有版本信息
			type StdVersion struct {
				ID      string `json:"id"`
				Version string `json:"version"`
				Desc    string `json:"desc"`
			}
			var version []StdVersion
			err = db.Table(table).Where("deleted_at is NULL").Find(&version).Error
			if err != nil {
				ErrCheck(c, returnData, err, "标准文件版本查询错误")
				return err
			}
			ErrNil(c, returnData, version, "查询成功")
		} else {
			//查询具体
			var info mod.MachineStd
			err = db.Table(table).Where("id=?", ii).Preload(clause.Associations).Find(&info).Error
			if err != nil {
				ErrCheck(c, returnData, err, "查询错误")
				return err
			}
			var fan mod.Machine
			err = json.Unmarshal(info.Set, &fan)
			fan.FanVersion = info.Version
			if err != nil {
				ErrCheck(c, returnData, err, "查询错误")
				return err
			}
			ErrNil(c, returnData, fan, "查询成功")
		}
		return nil
	case "measuringPoint":
		table = "point_std"
		info = new([]mod.PointStd)
	case "characteristic":
		table = "property_std"
		info = new([]mod.PropertyStd)
	case "band", "tree":
		table = i
		if i == "tree" {
			info = new([]alert.Tree)
		}
		if i == "band" {
			info = new([]alert.Band)
		}
	}
	if ii == "" {
		//查询所有版本信息
		type StdVersion struct {
			Version string `json:"version"`
			Desc    string `json:"desc"`
		}
		var version []StdVersion
		err = db.Table(table).Where("deleted_at is NULL").Select("version").Find(&version).Error
		if err != nil {
			ErrCheck(c, returnData, err, "标准文件版本查询错误")
			return err
		}
		ErrNil(c, returnData, version, "查询成功")
	} else {
		//查询具体
		err = db.Table(table).Where("version=?", ii).Preload(clause.Associations).Find(info).Error
		if err != nil {
			ErrCheck(c, returnData, err, "查询错误")
			return err
		}
		ErrNil(c, returnData, info, "查询成功")
	}
	return nil
}

func UpdateStatus(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	t := c.Param("type")
	var m map[string]interface{}
	if err = json.NewDecoder(c.Request().Body).Decode(&m); err != nil {
		return err
	}
	switch t {
	case "windfield":
		if err = db.Table("windfarm").Where("id=?", m["id"]).Clauses(clause.Locking{Strength: "UPDATE"}).
			Update("status", m["status"]).Error; err != nil {
			break
		}
	case "fan":
		if err = db.Table("machine").Where("id=?", m["id"]).Clauses(clause.Locking{Strength: "UPDATE"}).
			Update("status", m["status"]).Error; err != nil {
			break
		}
		//对应风场下所有风机check 并修改风场的状态
		_, _, err = mod.StatusCheck(m["id"], "windfarm", "machine", db)
		if err != nil {
			break
		}
	case "measuringPoint":
		var oldtime time.Time
		db.Table("point").Where("id=?", m["id"]).Select("last_data_time").Scan(&oldtime)
		if oldtime.AddDate(0, 0, 3).Before(time.Now()) {
			ErrCheck(c, returnData, errors.New("update fails"), "风机下测点三天内无数据，无状态，修改状态失败")
			return nil
		}
		err = db.Table("point").Where("id=?", m["id"]).Clauses(clause.Locking{Strength: "UPDATE"}).Update("status", m["status"]).Error
		if err != nil {
			break
		}
		var ppmwcid []string
		ppmwcid, _, _, err = mod.PointtoFactory(db, m["id"])
		if err != nil {
			break
		}
		//先比较是否为有数据
		// db.Table("point").Where("id=?", m["id"]).Scan("last_data_time")
		//对应风机下所有测点check 并修改风机的状态、风场状态
		_, _, err := mod.StatusCheck(ppmwcid[0], "part", "point", db)
		if err != nil {
			break
		}
		if _, _, err = mod.StatusCheck(ppmwcid[1], "machine", "part", db); err != nil {
			break
		}
		if _, _, err = mod.StatusCheck(ppmwcid[2], "windfarm", "machine", db); err != nil {
			break
		}

	}
	if err != nil {
		ErrCheck(c, returnData, err, "更新状态错误")
		return err
	}
	ErrNil(c, returnData, nil, "更新成功")
	return err
}

//*********运行统计handler
func GetFaultCounts(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	fid := c.QueryParam("id")
	keyword := c.QueryParam("keyword")
	fcs, err := mod.MonthFaultCounts(db, fid, keyword)
	if err != nil {
		ErrCheck(c, returnData, err, "故障数统计错误")
		return err
	}
	ErrNil(c, returnData, fcs, "故障数统计成功")
	return err
}

func GetStatisticsContent(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	id := c.QueryParam("id")
	keyword := c.QueryParam("type")
	type StatisticsContent struct {
		CompanyName     string `json:"company_name"`
		WindfieldNumber int64  `json:"windfield_number,string"`
		WindfieldName   string `json:"windfield_name"`
		FanNumber       int64  `json:"fan_number,string"`
		FanName         string `json:"fan_name"`
		WorkStartdate   string `json:"work_startdate"` //投运时间
		Health          string `json:"duration"`       //全生命周期
	}
	var sc StatisticsContent
	sub := db.Table("factory").
		Joins("right join windfarm on factory.uuid = windfarm.factory_uuid").
		Joins("right join machine on windfarm.uuid = machine.windfarm_uuid").
		Select("factory.id AS factory_id , windfarm.id AS windfarm_id,machine.id AS machine_id,factory.name AS factory_name , windfarm.name AS windfarm_name,machine.desc AS machine_name")
	//查询
	switch keyword {
	case "1": //公司
		sub = sub.Where("factory.id=?", id)
		db.Table("(?) as tree", sub).Distinct("factory_name").Select("factory_name").Limit(1).Scan(&sc.CompanyName)
		db.Table("(?) as tree", sub).Distinct("windfarm_id").Count(&sc.WindfieldNumber)
		db.Table("(?) as tree", sub).Distinct("machine_id").Count(&sc.FanNumber)

	case "2": //风场
		sub = sub.Where("windfarm.id=?", id)
		db.Table("(?) as tree", sub).Distinct("windfarm_name").Select("windfarm_name").Limit(1).Scan(&sc.WindfieldName)
		db.Table("(?) as tree", sub).Distinct("factory_name").Select("factory_name").Limit(1).Scan(&sc.CompanyName)
		db.Table("(?) as tree", sub).Distinct("windfarm_id").Count(&sc.WindfieldNumber)
		db.Table("(?) as tree", sub).Distinct("machine_id").Count(&sc.FanNumber)
	case "3": //风机
		sub = sub.Where("machine.id=?", id)
		db.Table("(?) as tree", sub).Distinct("machine_name").Select("machine_name").Limit(1).Scan(&sc.FanName)
		db.Table("(?) as tree", sub).Distinct("windfarm_name").Select("windfarm_name").Limit(1).Scan(&sc.WindfieldName)
		db.Table("(?) as tree", sub).Distinct("factory_name").Select("factory_name").Limit(1).Scan(&sc.CompanyName)
		db.Table("(?) as tree", sub).Distinct("windfarm_id").Count(&sc.WindfieldNumber)
		db.Table("(?) as tree", sub).Distinct("machine_id").Count(&sc.FanNumber)
		var f mod.Machine
		db.Table("machine").Where("id=?", id).First(&f)
		sc.WorkStartdate = f.BuiltTime
		bt, err := time.ParseInLocation("2006-01-02", f.BuiltTime, time.Local)
		if err != nil {
			break
		}
		nt := time.Now()
		gap := nt.Sub(bt).Hours()
		sc.Health = strconv.FormatFloat(1-gap/24/365/20, 'f', 4, 32)
	}
	if err != nil {
		ErrCheck(c, returnData, err, "查询错误")
		return err
	}
	ErrNil(c, returnData, sc, "查询成功")
	return err
}
func GetStatisticsStatus(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	id := c.QueryParam("id")
	keyword := c.QueryParam("keyword")
	type StatisticsStatus struct {
		Status  string `json:"status"`
		Number  int64  `json:"number"`
		Percent int64  `json:"percent"`
	}
	var ss []StatisticsStatus = []StatisticsStatus{
		{Status: "0", Number: 0, Percent: 0},
		{Status: "1", Number: 0, Percent: 0},
		{Status: "2", Number: 0, Percent: 0},
		{Status: "3", Number: 0, Percent: 0},
	}
	sub := db.Table("factory").
		Joins("right join windfarm on factory.uuid = windfarm.factory_uuid").
		Joins("right join machine on windfarm.uuid = machine.windfarm_uuid").
		Select("factory.id AS factory_id , windfarm.id AS windfarm_id,machine.id AS machine_id,machine.status AS machine_status")
		//查询
	var ex []map[string]interface{}
	switch keyword {
	case "company": //公司
		sub = sub.Where("factory.id=?", id)
		db.Table("(?) as tree", sub).Group("machine_status").
			Select("machine_status,COUNT(*)").Scan(&ex)
	case "windfield": //风场
		sub = sub.Where("windfarm.id=?", id)
		db.Table("(?) as tree", sub).Group("machine_status").
			Select("machine_status,COUNT(*)").Scan(&ex)
	}
	var sum int64
	for _, v := range ex {
		ss[v["machine_status"].(int64)].Number = v["COUNT(*)"].(int64)
		sum = sum + v["COUNT(*)"].(int64)
	}
	if sum != 0 {
		for k := range ss {
			ss[k].Percent = ss[k].Number * 100 / sum
		}
	}
	ss[0].Percent = 100 - ss[1].Percent - ss[2].Percent - ss[3].Percent
	if err != nil {
		ErrCheck(c, returnData, err, "查询错误")
		return err
	}
	ErrNil(c, returnData, ss, "查询成功")
	return err
}
func GetFaultLevel(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	fid := c.QueryParam("id")
	keyword := c.QueryParam("keyword")
	fcs, err := mod.MonthFaultLevel(db, fid, keyword)
	if err != nil {
		ErrCheck(c, returnData, err, "故障数统计错误")
		return err
	}
	ErrNil(c, returnData, fcs, "查询成功")
	return err
}
func GetPartFault(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	fid := c.QueryParam("id")
	keyword := c.QueryParam("keyword")

	fcs, err := mod.MonthPartFault(db, fid, keyword)
	if err != nil {
		ErrCheck(c, returnData, err, "查询错误")
		return err
	}
	ErrNil(c, returnData, fcs, "查询成功")
	return err
}
func GetFaultLogs(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	wid := c.QueryParam("id")
	keyword := c.QueryParam("keyword")

	type LogAlert struct {
		FanName string `json:"fan_name"`
		mod.Alert
	}
	var ff []LogAlert
	var m mod.Limit
	c.Bind(&m)
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 路由解析失败")
		return err
	}
	switch keyword {
	case "company":
		var temppid []string
		if wid == "" {
			var wids []string
			db.Table("factory").Pluck("id", &wids)
			for k := range wids {
				temppid = append(temppid, mod.UppertoPoint(db, "factory", wids[k])...)
			}
		} else {
			temppid = mod.UppertoPoint(db, "factory", wid)
		}
		err = db.Table("alert").Order("time_set desc").
			Where("point_uuid IN ?", temppid).
			Scopes(mod.Paginate(c.Request())).Find(&ff).Error
		if err != nil {
			return err
		}
		//* 需要查询联表的信息填入
		for k := range ff {
			_, pmwname, _, err := mod.PointtoFactory(db, ff[k].PointID)
			if err != nil {
				return err
			}
			ff[k].Machine = pmwname[2]
			ff[k].FanName = pmwname[2]
			ff[k].Windfarm = pmwname[3]
			ff[k].Factory = pmwname[4]
			ff[k].Time = mod.TimetoStr(ff[k].TimeSet).Format("2006-01-02 15:04:05")
		}
	case "windfield":
		temppid := mod.UppertoPoint(db, "windfarm", wid)
		err = db.Table("alert").Order("time_set desc").
			Where("point_uuid IN ?", temppid).
			Scopes(mod.Paginate(c.Request())).Find(&ff).Error
		if err != nil {
			return err
		}
		//* 需要查询联表的信息填入
		for k := range ff {
			_, pmwname, _, err := mod.PointtoFactory(db, ff[k].PointID)
			if err != nil {
				return err
			}
			ff[k].Machine = pmwname[2]
			ff[k].FanName = pmwname[2]
			ff[k].Windfarm = pmwname[3]
			ff[k].Factory = pmwname[4]
			ff[k].Time = mod.TimetoStr(ff[k].TimeSet).Format("2006-01-02 15:04:05")
		}
	}
	if err != nil {
		ErrCheck(c, returnData, err, "查询错误")
		return err
	}
	ErrNil(c, returnData, ff, "查询成功")
	return err
}
func GetTrend(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	fid := c.QueryParam("id")
	keytype := c.QueryParam("type")
	keyword := c.QueryParam("keyword")
	fcs, err := mod.FaultTrend(db, fid, keytype, keyword)
	if err != nil {
		ErrCheck(c, returnData, err, "查询错误")
		return err
	}
	ErrNil(c, returnData, fcs, "查询成功")
	return err
}
func GetPartTrend(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	fid := c.QueryParam("id")
	keytype := c.QueryParam("type")
	keyword := c.QueryParam("keyword")
	fcs, err := mod.FaultPartTrend(db, fid, keytype, keyword)
	if err != nil {
		ErrCheck(c, returnData, err, "查询错误")
		return err
	}
	ErrNil(c, returnData, fcs, "查询成功")
	return err
}

func OutputXlsx(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	var outputjobset mod.OutputJob
	outputjobset.New()
	c.Bind(&outputjobset.JobSet)
	if outputjobset.JobSet.Starttime == "" {
		outputjobset.JobSet.Starttime = "2000-01-01 00:00:00"
	}
	if outputjobset.JobSet.Endtime == "" {
		outputjobset.JobSet.Endtime = "2200-01-01 00:00:00"
	}
	if outputjobset.JobSet.MaxRpm == 0 {
		outputjobset.JobSet.MaxRpm = 999999
	}
	switch outputjobset.JobSet.FileType {
	case "1":
		err = outputjobset.OutputData(db)
	case "2":
		err = outputjobset.OutputAlert(db)
	}
	if err != nil {
		ErrNil(c, returnData, outputjobset, "导出任务完成")
		return err
	}
	ErrNil(c, returnData, outputjobset, "导出任务完成")
	return nil
}
func OutputDocx(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	var outputjobset mod.OutputJob
	outputjobset.New()
	c.Bind(&outputjobset.JobSet)
	if outputjobset.JobSet.Starttime == "" {
		outputjobset.JobSet.Starttime = "2000-01-01 00:00:00"
	}
	if outputjobset.JobSet.Endtime == "" {
		outputjobset.JobSet.Endtime = "2200-01-01 00:00:00"
	}
	if outputjobset.JobSet.MaxRpm == 0 {
		outputjobset.JobSet.MaxRpm = 999999
	}
	switch outputjobset.JobSet.FileType {
	case "1":
		err = outputjobset.OutputLog(db)
	case "2":
		err = outputjobset.OutputReport(db)
	}
	if err != nil {
		ErrNil(c, returnData, outputjobset, "导出任务完成")
		return err
	}
	ErrNil(c, returnData, outputjobset, "导出任务完成")
	return nil
}

func OutputDB(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	var outputjobset mod.OutputJob
	c.Bind(&outputjobset.JobSet)
	if outputjobset.JobSet.Starttime == "" {
		outputjobset.JobSet.Starttime = "2000-01-01 00:00:00"
	}
	if outputjobset.JobSet.Endtime == "" {
		outputjobset.JobSet.Endtime = "2200-01-01 00:00:00"
	}
	if outputjobset.JobSet.MaxRpm == 0 {
		outputjobset.JobSet.MaxRpm = 999999
	}
	sqlfilename, err := mod.TableBackUp(db, outputjobset.JobSet.Limit, outputjobset.JobSet.FilePath)
	if err != nil {
		outputjobset.OutputFiles = append(outputjobset.OutputFiles, &mod.OutputFile{FileName: sqlfilename, FileStatus: false})
		ErrNil(c, returnData, outputjobset, "数据库备份完成")
		return err
	}
	outputjobset.OutputFiles = append(outputjobset.OutputFiles, &mod.OutputFile{FileName: sqlfilename, FileStatus: true})
	ErrNil(c, returnData, outputjobset, "数据库备份完成")
	return nil
}
func InputDB(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	dbfile, err := c.FormFile("db_file")
	if err != nil {
		ErrCheck(c, returnData, err, "源文件获取失败")
		return err
	}
	src, err := dbfile.Open()
	if err != nil {
		ErrCheck(c, returnData, err, "源文件打开失败")
		return err
	}
	defer src.Close()
	err = mod.TableInsert_2(db, src)
	if err != nil {
		ErrCheck(c, returnData, err, "导入失败")
		return err
	}
	err = mod.TableCombine(db)
	if err != nil {
		ErrCheck(c, returnData, err, "合并失败")
		return err
	}
	ErrNil(c, returnData, nil, "input success")
	return err
}

//传给前端文件下载
func DownloadOutput(c echo.Context) error {
	filename := c.QueryParam("file_name")
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		var rd mod.ReturnData
		ErrCheck(c, rd, err, "file is not exist")
	}
	return c.File(filename)

}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func AlertBroadcast(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		mainlog.Error("可视化连接失败%v", err)
		return err
	}
	defer ws.Close()
	//第一次  所有未确认的报警信息
	var alerts []mod.Alert
	db.Table("alert").Where("broadcast =?", 0).
		Omit(clause.Associations).Find(&alerts)
	for k := range alerts {
		_, names, _, _ := mod.PointtoFactory(db, alerts[k].PointID)
		alerts[k].Time = mod.TimetoStr(alerts[k].TimeSet).Format("2006-01-02 15:04:05")
		alerts[k].Machine = names[2]
		if alerts[k].Level == 2 {
			alerts[k].BroadcastMessage = "注意：" + alerts[k].Location + alerts[k].Desc
		}
		if alerts[k].Level == 3 {
			alerts[k].BroadcastMessage = "报警：" + alerts[k].Location + alerts[k].Desc
		}
	}
	var rd mod.ReturnData
	rd.Code = 200
	rd.Message = "success"
	rd.Data = alerts
	rdjson, err := json.Marshal(rd)
	if err != nil {
		mainlog.Error("ws序列化 %v", err)
	}
	err = ws.WriteMessage(websocket.TextMessage, rdjson)
	if err != nil {
		mainlog.Error("ws发送失败 %v", err)
	}
	wschannel := make(chan struct{})

	//之后，等待通道的报警信息
	go func() {
		for {
			time.Sleep(1 * time.Second)
			select {
			case newalert := <-mod.Alertmessage:
				_, names, _, _ := mod.PointtoFactory(db, newalert.PointID)
				newalert.Time = mod.TimetoStr(newalert.TimeSet).Format("2006-01-02 15:04:05")
				newalert.Machine = names[2]
				if newalert.Level == 2 {
					newalert.BroadcastMessage = "注意：" + newalert.Location + newalert.Desc
				}
				if newalert.Level == 3 {
					newalert.BroadcastMessage = "报警：" + newalert.Location + newalert.Desc
				}
				var rd mod.ReturnData
				rd.Code = 200
				rd.Message = "success"
				rd.Data = newalert
				rdjson, err := json.Marshal(rd)
				if err != nil {
					mainlog.Error("ws序列化 %v", err)
				}
				err = ws.WriteMessage(websocket.TextMessage, rdjson)
				if err != nil {
					mainlog.Error("ws发送失败 %v", err)
				}
			case <-wschannel:
				return
			}
		}
	}()
	for {
		_, _, err = ws.ReadMessage()
		if err != nil {
			wschannel <- struct{}{}
			return err
		}
		time.Sleep(1 * time.Second)
	}
}

func AlertConfirm(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	type confirm struct {
		ID []string `json:"id"`
	}
	var confirms confirm
	c.Bind(&confirms)
	for k := range confirms.ID {
		if confirms.ID[k] != "" {
			var tid uint
			if db.Table("alert").Where("id=?", confirms.ID[k]).Pluck("id", &tid); tid == 0 {
				ErrNil(c, returnData, nil, "原数据已删除，报警信息删除")
				return err
			}
			err = db.Table("alert").Where("id=?", confirms.ID[k]).Update("broadcast", 1).Error
			if err != nil {
				ErrCheck(c, returnData, err, "confirm fail")
				return err
			}
		}
	}
	ErrNil(c, returnData, nil, "confirm success")
	return err
}
