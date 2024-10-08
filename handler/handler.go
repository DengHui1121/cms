package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	"io"
	"main/alert"
	"main/mod"
	"main/utils"
	"mime/multipart"
	"net/http"
	"os"
	"path"
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

// *登录
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
			Select("id", "username", "password", "level", "windfarm_ids_str").
			Scan(&existuser).RowsAffected
		if ra == 0 {
			err = errors.New("wrong username")
			ErrCheck(c, returnData, err, "账号名错误")
			return err
		}
		if existuser.Password != mm.Password {
			err = errors.New("wrong password")
			ErrCheck(c, returnData, err, "密码错误")
			return err
		}
		existuser.WindfarmIdsStrToArr()
		ErrNil(c, returnData, mod.PublicUser{User: &existuser}, "登录成功")
		return err
	}
}

// *修改账号
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
		updatesMap := make(map[string]interface{})
		mm.WindfarmIdsArrToStr()
		updatesMap["windfarm_ids_str"] = mm.WindfarmIdsStr

		if mm.Password != "" {
			updatesMap["password"] = mm.Password
		}
		if err = db.Table("user").Where("id=?", mm.ID).Clauses(clause.Locking{Strength: "UPDATE"}).
			Updates(updatesMap).Error; err != nil {
			ErrCheck(c, returnData, err, "修改错误")
			return err
		}
	case "add":
		mm.WindfarmIdsArrToStr()
		if mm.Level == 0 || mm.Password == "" || mm.Username == "" {
			err = errors.New("missing information")
			ErrCheck(c, returnData, nil, "账号信息不完整")
			return err
		}
		ra := db.Table("user").Where("username =?", mm.Username).
			Select("id", "username", "password", "level", "windfarm_ids_str").
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
			Select("id", "username", "level", "windfarm_ids_str").
			Scan(&userlist).Error
		for k := range userlist {
			userlist[k].WindfarmIdsStrToArr()
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

// * 标准文件读取。相同版本号的直接覆盖。
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

// 标准文件更新
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

// * api/v1/structure
func FindAll(c echo.Context) error {
	var err error
	uid := c.QueryParam("uid")
	var user mod.User
	if uid != "" {
		db.Model(&mod.User{}).Where("id = ? ", uid).Find(&user)
	}
	returnData := mod.ReturnData{}
	if user.WindfarmIdsStr == "" && user.Level == 3 {
		ErrNil(c, returnData, []int{}, "查询成功")
		return nil
	}
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
	if user.WindfarmIdsStr != "" {
		err = db.Table("factory").Find(&f).Error
		for i, factory := range f {
			err = db.Table("windfarm").Where(fmt.Sprintf("windfarm.id in (%s) AND windfarm.factory_uuid = '%s'", user.WindfarmIdsStr, factory.UUID)).Preload("Machines.Parts.Points").Find(&f[i].Windfarms).Error
		}
	} else {
		err = db.Table("factory").Preload("Windfarms.Machines.Parts.Points").Find(&f).Error
	}
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
		return err
	}
	ErrNil(c, returnData, f, "成功查询")
	return err
}

// * api/v1/xx?id=
func FindTree(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	id := c.QueryParam("id")
	uid := c.QueryParam("uid")
	var user mod.User
	if uid != "" {
		db.Model(&mod.User{}).Where("id = ? ", uid).Find(&user)
	}
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
		db2 := db.Table("factory").Omit("created_at", "updated_at")
		if user.WindfarmIdsStr == "" && user.Level == 3 {
			ErrNil(c, returnData, []int{}, "查询成功")
			return nil
		}
		if user.WindfarmIdsStr != "" {
			db2.Preload("Windfarms", fmt.Sprintf("windfarm.id IN (%s)", user.WindfarmIdsStr))
		} else {
			db2.Preload(clause.Associations)
		}
		err = db2.Find(&ff, id).Error
		f = ff.Windfarms
	case "windField":
		var ff mod.Windfarm2
		err = db.Table("windfarm").Select("windfarm.*, factory.name factoryName, factory.id factoryId").Omit("created_at", "updated_at").Joins("LEFT JOIN factory ON windfarm.factory_uuid = factory.uuid").Preload("Machines").
			Last(&ff, id).Error
		err = db.Table("machine").Select("COUNT(machine.id)").Where("machine.windfarm_uuid = ?", ff.UUID).Find(&ff.MachineCounts).Error
		var machineTypes []sql.NullString
		err = db.Table("machine").Select("DISTINCT machine.machine_type_num").Where("machine.windfarm_uuid = ?",
			ff.UUID).Find(&machineTypes).Error
		for _, machineType := range machineTypes {
			if machineType.Valid {
				ff.MachineType += machineType.String + "、"
			} else {
				ff.MachineType += ""
			}

		}
		if ff.MachineType != "" {
			ff.MachineType = strings.TrimRight(ff.MachineType, "、")
		}
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
			*mod.Windfarm2
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
		if user.WindfarmIdsStr == "" && user.Level == 3 {
			ErrNil(c, returnData, []int{}, "查询成功")
			return nil
		}
		db3 := db.Table("windfarm").Omit("created_at", "updated_at")
		if user.WindfarmIdsStr != "" {
			db3 = db.Where(fmt.Sprintf("windfarm.id in (%s)", user.WindfarmIdsStr))
		}
		err = db3.Preload(clause.Associations).Find(&ff, id).Error
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
		db.Table("part").Where("id = ?", ppmwfid[1]).Pluck("type", &m.PartName)
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
			sub.Not("type = ? OR type = ?", "故障树", "频带幅值").Scan(&temp)
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

// * 查找某一级下一级的所有内容
func FindInfo(c echo.Context) error {
	var dst interface{}
	var table string
	var err error
	uid := c.QueryParam("uid")
	returnData := mod.ReturnData{}
	i := c.Param("type")
	var user mod.User
	if uid != "" {
		db.Model(&mod.User{}).Where("id = ? ", uid).Find(&user)
	}
	var midDB *gorm.DB

	switch i {
	case "company":
		dst = new([]mod.Factory)
		table = "factory"
		midDB = db.Table(table)
		if user.WindfarmIdsStr == "" && user.Level == 3 {
			ErrNil(c, returnData, []int{}, "成功查找")
			return nil
		}
		if user.WindfarmIdsStr != "" {
			midDB = midDB.Preload(clause.Associations, fmt.Sprintf("windfarm.id in (%s)", user.WindfarmIdsStr))
		} else {
			midDB = midDB.Preload(clause.Associations)
		}
	case "windFields":
		dst = new([]mod.Windfarm2)
		table = "windfarm"
		midDB = db.Table(table)
		if user.WindfarmIdsStr == "" && user.Level == 3 {
			ErrNil(c, returnData, []int{}, "成功查找")
			return nil
		}
		if user.WindfarmIdsStr != "" {
			midDB = midDB.Where(fmt.Sprintf("windfarm.id in (%s)", user.WindfarmIdsStr))
		}
		midDB = midDB.Joins("left join factory on factory.uuid = windfarm.factory_uuid").Select("windfarm.*, factory.id factoryId, factory.name factoryName")
	}
	err = midDB.Omit("created_at", "updated_at").Find(dst).Error
	if err != nil {
		ErrCheck(c, returnData, err, c.Request().URL.String()+" 查找信息失败")
		return err
	}
	ErrNil(c, returnData, dst, "成功查找")
	return nil
}

// * api/v1/xx/:id
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

// TODO 修改报警详细信息
func UpdateAlert(c echo.Context) error {
	var err error
	var returnData mod.ReturnData
	var m mod.Alert
	c.Bind(&m)
	err = db.Table("alert").Where("id = ?", m.ID).
		Select("level", "strategy", "desc", "source", "suggest", "handle",
			"confirm").Clauses(clause.Locking{Strength: "UPDATE"}).
		Updates(m).Error
	if err != nil {
		ErrCheck(c, returnData, err, "更新失败")
	}
	ErrNil(c, returnData, nil, "更新成功")
	return err
}

// 新建公司 风场 风机
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
		//fmt.Println(mm)
		//fmt.Println(m)
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
	db.Table("data_"+ppmwfid[2]).Where("id = ?", m.DataID).Select("uuid", "rpm", "time_set", "point_uuid").
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

// * 风机文件：存在即更新，不存在即导入
// * api/v1/fan/parts 导入部件
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

type alertDesc struct {
	Suggest string
	Desc    string
}

var alertDescs = map[string]alertDesc{
	"主轴承": {Suggest: "建议检查主轴振动和异响情况"},
	"齿轮箱": {Suggest: "建议及时检查齿轮箱振动和异响情况"},
	"发电机": {Suggest: "建议及时登机检查发电机振动和异响情况"},
}

// * api/v1/data 导入/更新数据
func CheckMPointData(ipport string) echo.HandlerFunc {
	return func(c echo.Context) error {
		returnData := mod.ReturnData{}
		var err error
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			ErrCheck(c, returnData, err, "解析方式参数错误")
			return err
		}
		var parsing mod.Parsing
		if err = db.Table("parsing").Where("id = ? and is_del = ?", id, false).Find(&parsing).Error; err != nil {
			ErrCheck(c, returnData, err, "参数错误")
			return err
		}

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
		fullFileName := file.Filename
		var info string
		var filedata []byte
		// 判断文件类型 不同文件的导入
		info, filedata, err = mod.TypeRead(fullFileName, src, parsing)
		if err != nil {
			ErrCheck(c, returnData, err, "导入文件失败")
			return err
		}
		// 找测点并导入数据库
		var pdata mod.Data
		err = pdata.DataInfoGet(db, info, filedata, parsing)
		if err != nil {
			ErrCheck(c, returnData, err, "未找到测点")
			return err
		}
		// 检查数据是否存在，将创建时间和修改时间进行填充
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
			if err = mod.InsertData(db, tx, ipport, &pdata); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			ErrCheck(c, returnData, err, "数据导入失败")
			return err
		}
		// FIXME 数据导入完成后，开始调用预警算法, 产生报警需要更新风机月报警次数和日报警
		pid := strconv.FormatUint(uint64(pdata.PointID), 10)
		ppmwcid, _, _, err := mod.PointtoFactory(db, pid)
		if err != nil {
			return err
		}
		fid := ppmwcid[2]
		//执行预警算法, 需不需要新开协程计算
		var algorithms []mod.Algorithm
		if err = db.Table("algorithm").Where("point_uuid = ? and enabled = true and is_del = false", pdata.PointUUID).Find(&algorithms).Error; err != nil {
			err = errors.New("未找到算法")
			ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时，查找相关算法异常")
			return err
		}
		var postBody mod.AlgorithmReqBody
		if err = db.Table("point").Select("windfarm.`desc` windfarmName, machine.`name` machineName, point.`name` pointName").Joins("left join part on part.uuid = point.part_uuid").
			Joins("left join machine on machine.uuid= part.machine_uuid").Joins("left join windfarm on windfarm.uuid = machine.windfarm_uuid").Where("point.uuid = ?", pdata.PointUUID).
			Find(&postBody).Error; err != nil {
			ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时，查找算法相关参数异常")
			return err
		}
		//postBody.MachineName = "1#"
		//postBody.WindfarmName = "马鬃山风场"
		//postBody.PointName = "main_frontbearing_1A_5000Hz"
		postBody.Data = pdata.Wave.DataString
		postBody.SampleRate = strconv.Itoa(pdata.SampleFreq) + "Hz"
		postBody.SampleTime = time.Unix(pdata.TimeSet, 0).Format("2006_01_02_15:04")
		postBody.Rpm = strconv.Itoa(int(pdata.Rpm)) + "rpm"
		client := resty.New()
		tx := db.Begin()
		for _, algorithm := range algorithms {
			switch algorithm.Type {
			case "A":
				var responseBody mod.AlgorithmRepBodyA
				resp, err := client.R().SetHeader("Content-Type", "application/json").SetBody(postBody).SetResult(&responseBody).Post(algorithm.Url)
				fmt.Println(resp.Body())
				if err != nil {
					tx.Rollback()
					err = errors.New("算法请求发起失败" + err.Error())
					ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时 ")
					return err
				} else {
					if resp.StatusCode() != 200 {
						tx.Rollback()
						err = errors.New("算法请求失败。err:" + resp.Status())
						ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时 ")
						return err
					}
					if responseBody.Success == "True" && responseBody.Error == "0" {
						//将responseBody中的TypiFeatureSource转换成responseBody.TypiFeature
						for index, value := range responseBody.TypiFeatureSource {
							switch index {
							case 0:
								responseBody.TypiFeature.MeanFre = float64(value)
							case 1:
								responseBody.TypiFeature.SquareFre = float64(value)
							case 2:
								responseBody.TypiFeature.GravFre = float64(value)
							case 3:
								responseBody.TypiFeature.SecGravFre = float64(value)
							case 4:
								responseBody.TypiFeature.GravRatio = float64(value)
							case 5:
								responseBody.TypiFeature.StandDeviate = float64(value)
							}
						}
						pdata.TypiFeature = responseBody.TypiFeature
						if err = tx.Table("data_"+fid).Omit("Wave").Where("id = ?", pdata.ID).Updates(&pdata).Error; err != nil {
							tx.Rollback()
							err = errors.New("更新结果到数据表失败")
							ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时 ")
							return err
						}

						algorithmResultA := mod.AlgorithmResultA{
							DataUUID:       pdata.UUID,
							AlgorithmID:    algorithm.Id,
							FTendencyFloat: responseBody.FTendency.Translate(),
							TTendencyFloat: responseBody.TTendency.Translate(),
							TypiFeature:    responseBody.TypiFeature,
							CreateTime:     mod.GetCurrentTime(),
							UpdateTime:     mod.GetCurrentTime(),
							DataTime:       mod.TimetoStr(pdata.TimeSet).Format("2006-01-02 15:04:05"),
						}
						//插入结果表
						if err = tx.Table("algorithm_result_a").Create(&algorithmResultA).Error; err != nil {
							tx.Rollback()
							err = errors.New("结果表新增记录失败")
							ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时 ")
							return err
						}
						// 补充报警基础信息
						alerts := make([]mod.Alert, 0)
						aler := mod.Alert{
							DataUUID:  pdata.UUID,
							PointUUID: pdata.PointUUID,
							Location:  postBody.PointName,
							Type:      "预警算法",
							Strategy:  algorithm.Name,
							TimeSet:   pdata.TimeSet,
							Rpm:       pdata.Rpm,
							Confirm:   1,
							Source:    0,
						}
						var partType string
						if err = db.Table("point").Select("part.type").Joins("left join part on part.uuid = point.part_uuid").Where("point.uuid = ?", pdata.PointUUID).Find(&partType).Error; err != nil {
							return err
						}
						//  开始处理频域注意和报警
						if algorithmResultA.FScore > algorithmResultA.FLevel1 && algorithmResultA.FScore < algorithmResultA.FLevel2 {
							// 报警表单：注意 level为 2
							aler.Level = 2
							aler.Desc, aler.Suggest = GetDescAndSuggestByLevel(2, partType, "F", aler.Location)
							alerts = append(alerts, aler)
						}
						if algorithmResultA.FScore > algorithmResultA.FLevel2 {
							// 报警表单：报警 level为 3
							aler.Level = 3
							aler.Desc, aler.Suggest = GetDescAndSuggestByLevel(3, partType, "F", aler.Location)
							alerts = append(alerts, aler)
						}
						// 开始处理时域注意和报警
						if algorithmResultA.TScore > algorithmResultA.TLevel1 && algorithmResultA.TScore < algorithmResultA.TLevel2 {
							// 报警表单：注意 level为 2
							aler.Level = 2
							aler.Desc, aler.Suggest = GetDescAndSuggestByLevel(2, partType, "T", aler.Location)
							alerts = append(alerts, aler)
						}
						if algorithmResultA.TScore > algorithmResultA.TLevel2 {
							// 报警表单：报警 level为 3
							aler.Level = 3
							aler.Desc, aler.Suggest = GetDescAndSuggestByLevel(3, partType, "T", aler.Location)
							alerts = append(alerts, aler)
						}
						if len(alerts) > 0 {
							// 插入报警表
							if err = tx.Table("alert").Create(&alerts).Error; err != nil {
								tx.Rollback()
								err = errors.New("报警表单新增记录失败")
								ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时 ")
							}
						}
					} else if responseBody.Success == "False" && responseBody.Error == "0" {
						tx.Rollback()
						err = errors.New("算法客户端运行异常")
						ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时 ")
					} else {
						switch responseBody.Error {
						case "1":
							err = errors.New("风场名错误")
						case "2":
							err = errors.New("风机号错误")
						case "3":
							err = errors.New("测点名错误")
						case "4":
							err = errors.New("数据长度错误")
						case "5":
							err = errors.New("采样频率错误")
						case "6":
							err = errors.New("风速错误 ")
						}
						tx.Rollback()
						ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时 ")
					}

				}
			case "B":
				var responseBody mod.AlgorithmRepBodyB
				resp, err := client.R().SetHeader("Content-Type", "application/json").SetBody(postBody).SetResult(&responseBody).Post(algorithm.Url)
				if err != nil {
					tx.Rollback()
					err = errors.New("算法请求发起失败。err:" + err.Error())
					return err
				} else {
					if resp.StatusCode() != 200 {
						tx.Rollback()
						err = errors.New("算法请求失败。err:" + resp.Status())
						return err
					}
					if responseBody.Success == "True" && responseBody.Error == "0" {
						//故障诊断结果和概率插入报警表
						algorithmResultB := mod.AlgorithmResultB{
							DataDTO:     responseBody.Data.Translate(),
							DataUUID:    pdata.UUID,
							AlgorithmID: algorithm.Id,
							CreateTime:  mod.GetCurrentTime(),
							UpdateTime:  mod.GetCurrentTime(),
							DataTime:    mod.TimetoStr(pdata.TimeSet).Format("2006-01-02 15:04:05"),
						}
						//插入结果表
						if err = tx.Table("algorithm_result_b").Create(&algorithmResultB).Error; err != nil {
							tx.Rollback()
							err = errors.New("故障诊断结果插入失败")
							return err
						}
						// 查询出测点所属部件,根据部件选择合适的报警处理建议，以及拼接故障描述
						type pointInfos struct {
							PointName string `gorm:"column:pointName"`
							PartType  string `gorm:"column:partType"`
						}
						var pointInfo pointInfos
						if err = tx.Table("point").Joins("left join part on part.uuid = point.part_uuid").Select("part.type partType, point.name pointName").Find(&pointInfo).Error; err != nil {
							tx.Rollback()
							err = errors.New("查询出测点所属部件失败")
							return err
						}

						if responseBody.Data.FaultName != "" {
							//插入报警表
							aler := mod.Alert{
								DataUUID:  pdata.UUID,
								PointUUID: pdata.PointUUID,
								Location:  postBody.PointName,
								Type:      "预警算法",
								Strategy:  algorithm.Name,
								Desc:      fmt.Sprintf("%s", pointInfo.PointName),
								TimeSet:   pdata.TimeSet,
								Rpm:       pdata.Rpm,
								Suggest:   alertDescs[pointInfo.PartType].Suggest,
								Confirm:   1,
								Source:    0,
							}
							tag := mod.CheckTagExist(tx, pdata.PointUUID, responseBody.Data.FaultName)
							if err = tx.Table("alert").Create(&aler).Error; err != nil {
								tx.Rollback()
								err = errors.New("报警插入失败")
							}

							// TODO 报警插入后更新日报警和月报警
							if err = mod.UpdateReportAfterAlert(tx, aler); err != nil {
								tx.Rollback()
								err = errors.New("日报警和月报警更新失败")
								return err
							}
							// 将id更新到pdata.tag中
							if err = tx.Table("data_"+fid).Where("uuid =?", pdata.UUID).Update("tag", fmt.Sprintf("%d-%d", tag.FaultTagFirstID, tag.Id)).Error; err != nil {
								tx.Rollback()
								err = errors.New("数据更新失败")
								return err
							}
						}
					} else if responseBody.Success == "False" && responseBody.Error == "0" {
						err = errors.New("算法运行失败")
						tx.Rollback()
					} else {
						switch responseBody.Error {
						case "1":
							err = errors.New("风场名错误")
						case "2":
							err = errors.New("风机号错误")
						case "3":
							err = errors.New("测点名错误")
						case "4":
							err = errors.New("数据长度错误")
						case "5":
							err = errors.New("采样频率错误")
						case "6":
							err = errors.New("风速错误 ")
						}
					}
				}
			}

			//if err = algorithm.ExecuteAlgorithm(&pdata, db, fid); err != nil {
			//	ErrCheck(c, returnData, err, "数据保存成功，执行预警算法过程时，执行算法异常")
			//	return err
			//}

		}
		tx.Commit()
		ErrNil(c, returnData, false, "导入数据成功。")
		return nil
	}
}

var modelName = map[string]string{
	"R": "有效值模型",
	"F": "频域残差模型",
	"T": "时域残差模型",
}

// @Title GetDescAndSuggestByLevel
// @Description 根据level，partType，alerType，location获取faultDesc和faultSuggest
// @Param level 故障等级
// @Param partType 部件类型
// @Param alerType 报警类型
// @Param location 测点名
// @Return desc 描述
// @Return suggest 建议
func GetDescAndSuggestByLevel(level int, partType, alerType, location string) (desc, suggest string) {
	switch {
	// 主轴承
	case (level == 1 || level == 0) && partType == "主轴承":
		return "振动幅值趋势平稳；无明显轴承故障频率", "建议正常运行"
	case level == 2 && partType == "主轴承":
		return fmt.Sprintf("%s%s振幅超限", location, modelName[alerType]), "建议注脂改善润滑"
	case level == 3 && partType == "主轴承":
		return fmt.Sprintf("%s%s振幅报警", location, modelName[alerType]), "建议检查主轴振动和异响情况"

	// 齿轮箱
	case (level == 1 || level == 0) && partType == "齿轮箱":
		return "振动幅值趋势平稳；无明显轴承或齿轮故障频率", "建议正常运行"
	case level == 2 && partType == "齿轮箱":
		return fmt.Sprintf("%s%s振幅超限，建议巡检时关注齿轮箱振动异响情况", location, modelName[alerType]), "建议关注齿轮箱振动和异响情况"
	case level == 3 && partType == "齿轮箱":
		return fmt.Sprintf("%s%s振幅报警，建议及时等机检查", location, modelName[alerType]), "建议及时检查齿轮箱振动和异响情况"

	// 发电机
	case (level == 1 || level == 0) && partType == "发电机":
		return "振动幅值趋势平稳；无明显轴承故障频率", "建议正常运行"
	case level == 2 && partType == "发电机":
		return fmt.Sprintf("%s%s振幅超限", location, modelName[alerType]), "建议关注发电机润滑、振动和异响情况"
	case level == 3 && partType == "发电机":
		return fmt.Sprintf("%s%s振幅报警", location, modelName[alerType]), "建议及时登机检查发电机振动和异响情况"

	// 机舱
	case (level == 1 || level == 0) && partType == "机舱":
	case level == 2 && partType == "机舱":
	case level == 3 && partType == "机舱":
		return

	// 塔筒
	case (level == 1 || level == 0) && partType == "塔筒":
	case level == 2 && partType == "塔筒":
	case level == 3 && partType == "塔筒":
		return

	// 叶片
	case (level == 1 || level == 0) && partType == "叶片":
	case level == 2 && partType == "叶片":
	case level == 3 && partType == "叶片":
		return
	}
	return
}

// 覆盖数据上传接口
func OverMPointData(ipport string) echo.HandlerFunc {
	return func(c echo.Context) error {
		var err error
		returnData := mod.ReturnData{}
		idStr := c.Param("id")
		id, err := strconv.Atoi(idStr)
		if err != nil {
			ErrCheck(c, returnData, err, "参数错误")
			return err
		}
		var parsing mod.Parsing
		if err = db.Table("parsing").Where("id = ? and is_del = ?", id, false).Find(&parsing).Error; err != nil {
			ErrCheck(c, returnData, err, "参数错误")
			return err
		}
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
		fullFileName := file.Filename
		var info string
		var filedata []byte
		// 判断文件类型 不同文件的导入
		info, filedata, err = mod.TypeRead(fullFileName, src, parsing)
		if err != nil {
			ErrCheck(c, returnData, err, "导入文件失败")
			return err
		}

		// 找测点并导入数据库
		var pdata mod.Data
		err = pdata.DataInfoGet(db, info, filedata, parsing)
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
			if err = mod.InsertData(db, tx, ipport, &pdata); err != nil {
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

// * api/v1/xx delete
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

// * 数据绘图
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
		} else if ctype == "time" {
			var res mod.TimePlot
			var algorithmResultA []mod.AlgorithmResultA
			if err = db.Select("ara.*").Table(fmt.Sprintf("data_%s data", fid)).
				Joins("LEFT JOIN algorithm_result_a ara ON ara.data_uuid = data.uuid").
				Where("data.id = ?", id).Find(&algorithmResultA).Error; err != nil {
				ErrCheck(c, returnData, err, "调取数据失败")
			}
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.TLev1 = value.TLevel1
					res.TLev2 = value.TLevel2
					res.TScore = append(res.TScore, value.TScore)
					res.XAxis = append(res.XAxis, value.DataTime)
				}
			} else {
				res.TScore = emptyFloat
				res.XAxis = emptyString
			}
			plot.Time = res
		} else if ctype == "frequency" {
			var res mod.FrequencyPlot

			var algorithmResultA []mod.AlgorithmResultA
			if err = db.Select("ara.*").Table(fmt.Sprintf("data_%s data", fid)).
				Joins("LEFT JOIN algorithm_result_a ara ON ara.data_uuid = data.uuid").
				Where("data.id = ?", id).Find(&algorithmResultA).Error; err != nil {
				ErrCheck(c, returnData, err, "调取数据失败")
			}
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.FLev1 = value.FLevel1
					res.FLev2 = value.FLevel2
					res.FScore = append(res.FScore, value.FScore)
					res.XAxis = append(res.XAxis, value.DataTime)

				}
			} else {
				res.FScore = emptyFloat
				res.XAxis = emptyString
			}
			plot.Frequency = res
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

// 测点绘制趋势图
func GetFanDataCurrentPlot(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	fid := c.QueryParam("id")       //风机id
	ptype := c.QueryParam("type")   //测点id
	models := c.QueryParam("model") //算法模型
	startTime := c.QueryParam("startTime")
	endTime := c.QueryParam("endTime")
	var startTimeSet, endTimeSet int64
	if startTime != "" && endTime != "" {
		startTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", startTime)
		endTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", endTime)
	} else {
		now := time.Now()
		oneMonthAgo := now.AddDate(0, -1, 0).Unix()
		startTimeSet = oneMonthAgo
		endTimeSet = now.Unix()
	}

	m := mod.MultiDatatoPlot{
		StartTime: mod.TimetoStrFormat("2006-01-02 15:04:05", startTimeSet),
		EndTime:   mod.TimetoStrFormat("2006-01-02 15:04:05", endTimeSet),
	}
	m.Currentplot = make([]mod.CurrentPlot, 0)
	//找测点和限制条件，填充m
	db.Table("machine").
		Joins("right join part on machine.uuid = part.machine_uuid").
		Joins("right join point on part.uuid = point.part_uuid").
		Where("machine.id=?", fid).Where("point.id=?", ptype).
		Select("point.id AS point_id , point.name AS legend").
		Scan(&m.Currentplot)
	if err = m.FanStaticPlot(db, models, fid, startTimeSet, endTimeSet); err != nil {
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}
	ErrNil(c, returnData, m, "调取数据成功")
	return err
}

var (
	emptyFloat  = make([]float64, 0)
	emptyString = make([]string, 0)
)

// 算法绘制趋势图
func GetFanDataCurrentAlgorithmPlotA(c echo.Context) (err error) {
	var returnData mod.ReturnData
	fanIdStr := c.QueryParam("fanId")
	pointIdStr := c.QueryParam("pointId")
	algorithmIdStr := c.Param("id")
	startTime := c.QueryParam("startTime")
	endTime := c.QueryParam("endTime")
	typeStr := c.QueryParam("type")
	//如果没有传startTime, endTime,则获取最近一个月的数据
	if startTime == "" && endTime == "" {
		startTime = time.Now().AddDate(0, -1, 0).Format("2006-01-02 15:04:05")
		endTime = time.Now().Format("2006-01-02 15:04:05")
	}

	algorithmId, err := strconv.Atoi(algorithmIdStr)
	if err != nil {
		mainlog.Error("转换算法id错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}

	fanId, err := strconv.Atoi(fanIdStr)
	if err != nil {
		mainlog.Error("转换风机id错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}

	pointId, err := strconv.Atoi(pointIdStr)
	if err != nil {
		mainlog.Error("转换测点id错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}

	// 首先查询出所有数据
	var algorithmResultA []mod.AlgorithmResultA
	if err = db.Table("point").Select("a.*,data.time_set as timeSet").Joins(fmt.Sprintf("left join data_%d data on data.point_uuid = point.uuid", fanId)).
		Joins("left join algorithm_result_a a on a.data_uuid = data.uuid").Where("point.id = ? and a.algorithm_id = ? AND a.create_time >= ? AND a.create_time <= ?", pointId, algorithmId, startTime, endTime).
		Order("data.time_set").Find(&algorithmResultA).Error; err != nil {
		mainlog.Error("获取算法错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}
	// 如果没有接收type,则返回A类算法所有画图参数
	if typeStr == "" {
		// 直接返回所有画图结构体
		var res mod.AlgorithmPlotA
		res.StartTime = startTime
		res.EndTime = endTime
		if len(algorithmResultA) > 0 {
			for _, value := range algorithmResultA {
				res.TimePlot.TLev1 = value.TLevel1
				res.TimePlot.TLev2 = value.TLevel2
				res.TimePlot.TScore = append(res.TimePlot.TScore, value.TScore)
				res.TimePlot.XAxis = append(res.TimePlot.XAxis, value.CreateTime)
				res.FrequencyPlot.FLev1 = value.FLevel1
				res.FrequencyPlot.FLev2 = value.FLevel2
				res.FrequencyPlot.FScore = append(res.FrequencyPlot.FScore, value.FScore)
				res.FrequencyPlot.XAxis = append(res.FrequencyPlot.XAxis, value.CreateTime)
				res.EigenValuePlot.TypiFeature.MeanFre = append(res.EigenValuePlot.TypiFeature.MeanFre, value.MeanFre)
				res.EigenValuePlot.TypiFeature.SquareFre = append(res.EigenValuePlot.TypiFeature.SquareFre, value.SquareFre)
				res.EigenValuePlot.TypiFeature.GravFre = append(res.EigenValuePlot.TypiFeature.GravFre, value.GravFre)
				res.EigenValuePlot.TypiFeature.SecGravFre = append(res.EigenValuePlot.TypiFeature.SecGravFre, value.SecGravFre)
				res.EigenValuePlot.TypiFeature.GravRatio = append(res.EigenValuePlot.TypiFeature.GravRatio, value.GravRatio)
				res.EigenValuePlot.TypiFeature.StandDeviate = append(res.EigenValuePlot.TypiFeature.StandDeviate, value.StandDeviate)
				res.EigenValuePlot.XAxis = append(res.EigenValuePlot.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
			}
		} else {
			res.TimePlot.TScore = emptyFloat
			res.TimePlot.XAxis = emptyString
			res.FrequencyPlot.FScore = emptyFloat
			res.FrequencyPlot.XAxis = emptyString
			res.EigenValuePlot.TypiFeature.MeanFre = emptyFloat
			res.EigenValuePlot.TypiFeature.SquareFre = emptyFloat
			res.EigenValuePlot.TypiFeature.GravFre = emptyFloat
			res.EigenValuePlot.TypiFeature.SecGravFre = emptyFloat
			res.EigenValuePlot.TypiFeature.GravRatio = emptyFloat
			res.EigenValuePlot.TypiFeature.StandDeviate = emptyFloat
			res.EigenValuePlot.XAxis = emptyString
		}
		ErrNil(c, returnData, res, "调取数据成功")
	} else {
		// typestr 不为空，返回对应type的结构体
		switch typeStr {
		case "time":
			var res mod.TimePlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.TLev1 = value.TLevel1
					res.TLev2 = value.TLevel2
					res.TScore = append(res.TScore, value.TScore)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.TScore = emptyFloat
				res.XAxis = emptyString
			}

			ErrNil(c, returnData, res, "调取数据成功")
		case "frequency":
			var res mod.FrequencyPlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.FLev1 = value.FLevel1
					res.FLev2 = value.FLevel2
					res.FScore = append(res.FScore, value.FScore)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.FScore = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		case "eigenvalue":
			var res mod.EigenValuePlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.TypiFeature.MeanFre = append(res.TypiFeature.MeanFre, value.MeanFre)
					res.TypiFeature.SquareFre = append(res.TypiFeature.SquareFre, value.SquareFre)
					res.TypiFeature.GravFre = append(res.TypiFeature.GravFre, value.GravFre)
					res.TypiFeature.SecGravFre = append(res.TypiFeature.SecGravFre, value.SecGravFre)
					res.TypiFeature.GravRatio = append(res.TypiFeature.GravRatio, value.GravRatio)
					res.TypiFeature.StandDeviate = append(res.TypiFeature.StandDeviate, value.StandDeviate)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.TypiFeature.MeanFre = emptyFloat
				res.TypiFeature.SquareFre = emptyFloat
				res.TypiFeature.GravFre = emptyFloat
				res.TypiFeature.SecGravFre = emptyFloat
				res.TypiFeature.GravRatio = emptyFloat
				res.TypiFeature.StandDeviate = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		case "meanfre":
			var res mod.MeanFrePlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.MeanFres = append(res.MeanFres, value.MeanFre)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.MeanFres = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		case "squarefre":
			var res mod.SquareFrePlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.SquareFres = append(res.SquareFres, value.SquareFre)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.SquareFres = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		case "gravfre":
			var res mod.GravFrePlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.GravFres = append(res.GravFres, value.GravFre)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.GravFres = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		case "secgravfre":
			var res mod.SecGravFrePlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.SecGravFres = append(res.SecGravFres, value.SecGravFre)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.SecGravFres = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		case "gravratio":
			var res mod.GravRatioPlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.GravRatios = append(res.GravRatios, value.GravRatio)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.GravRatios = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		case "standdeviate":
			var res mod.StandDeviatePlot
			res.StartTime = startTime
			res.EndTime = endTime
			if len(algorithmResultA) > 0 {
				for _, value := range algorithmResultA {
					res.StandDeviates = append(res.StandDeviates, value.StandDeviate)
					res.XAxis = append(res.XAxis, mod.TimetoStr(value.DataTimeSet).Format("2006-01-02 15:04:05"))
				}
			} else {
				res.StandDeviates = emptyFloat
				res.XAxis = emptyString
			}
			ErrNil(c, returnData, res, "调取数据成功")
		}
	}
	return
}

func GetFanDataCurrentAlgorithmPlotB(c echo.Context) (err error) {
	var returnData mod.ReturnData
	fanIdStr := c.QueryParam("fanId")
	pointIdStr := c.QueryParam("pointId")
	algorithmIdStr := c.Param("id")
	startTime := c.QueryParam("startTime")
	endTime := c.QueryParam("endTime")
	//如果没有传startTime, endTime,则获取最近一个月的数据
	if startTime == "" && endTime == "" {
		startTime = time.Now().AddDate(0, -1, 0).Format("2006-01-02 15:04:05")
		endTime = time.Now().Format("2006-01-02 15:04:05")
	}

	algorithmId, err := strconv.Atoi(algorithmIdStr)
	if err != nil {
		mainlog.Error("转换算法id错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}

	fanId, err := strconv.Atoi(fanIdStr)
	if err != nil {
		mainlog.Error("转换风机id错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}

	pointId, err := strconv.Atoi(pointIdStr)
	if err != nil {
		mainlog.Error("转换测点id错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}

	//查询所有原始数据
	var algorithmResultB []mod.AlgorithmResultB
	if err = db.Table("point").Select("b.*").Joins(fmt.Sprintf("left join data_%d data on data.point_uuid = point.uuid", fanId)).
		Joins("left join algorithm_result_b b on b.data_uuid = data.uuid").Where("point.id = ? and b.algorithm_id = ? AND data_time  ? AND ?", pointId, algorithmId, startTime, endTime).
		Find(&algorithmResultB).Error; err != nil {
		mainlog.Error("获取算法错误")
		ErrCheck(c, returnData, err, "调取数据错误")
		return err
	}
	var res mod.AlgorithmPlotB
	if len(algorithmResultB) > 0 {
		for _, value := range algorithmResultB {
			res.Coordinates.X = append(res.Coordinates.X, value.X)
			res.Coordinates.Y = append(res.Coordinates.Y, value.Y)
			res.Coordinates.Z = append(res.Coordinates.Z, value.Z)
			res.FaultName = append(res.FaultName, value.FaultName)
		}
	} else {
		res.Coordinates.X = emptyFloat
		res.Coordinates.Y = emptyFloat
		res.Coordinates.Z = emptyFloat
		res.FaultName = emptyString
	}
	ErrNil(c, returnData, res, "调取数据成功")

	return
}

// TODO linux系统试验
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
func AnalyseDataFunc(c echo.Context) (err error) {
	returnData := mod.ReturnData{}
	pointUUID := c.QueryParam("pointUUID")
	f := mod.GetAnalysisOption()
	var algorithms []mod.Algorithm
	if err = db.Table("algorithm").Where("point_uuid = ? AND enabled = true AND is_del = false", pointUUID).Find(&algorithms).Error; err != nil {
		ErrCheck(c, returnData, err, "查询错误")
	}
	for _, v := range algorithms {
		f = append(f, mod.AnalysisOption{
			Value:        int(v.Id),
			Label:        v.Name,
			RpmAvailable: false,
			DataUrl:      v.Url,
			Type:         v.Type,
		})
	}
	ErrNil(c, returnData, f, "success")
	return
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
			var fan mod.Machine2
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

// *********运行统计handler
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

// 传给前端文件下载
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
	db.Table("alert").Where("broadcast = 0 and confirm = 2").Omit(clause.Associations).Find(&alerts)
	//midDB := db.Table("alert").Joins("right join point on point.uuid = alert.point_uuid").Joins("right join part on part.uuid = point.part_uuid").
	//	Joins("right join machine on machine.uuid = part.machine_uuid").Joins("right join windfarm on windfarm.uuid = machine.windfarm.uuid").
	//	Where("broadcast = ?", 0).Omit(clause.Associations)
	//for _, value := range alerts {
	//	midDB.Select("windfarm.name").Find(&value.Windfarm)
	//	midDB.Select("point.name").Find(&value.Point)
	//}
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
		ID     []string `json:"id"`
		Status string   `json:"status"`
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
			status, _ := strconv.Atoi(confirms.Status)
			err = db.Table("alert").Where("id=?", confirms.ID[k]).Updates(&mod.Alert{Broadcast: 1, CheckTime: mod.GetCurrentTime(), Confirm: status}).Error
			if err != nil {
				ErrCheck(c, returnData, err, "confirm fail")
				return err
			}
		}
	}
	ErrNil(c, returnData, nil, "confirm success")
	return err
}

// FactoryDataUpdateHandler 厂家数据上传接口
func FactoryDataUpdateHandler(ipport string) echo.HandlerFunc {
	return func(c echo.Context) (err error) {
		var returnData mod.ReturnData
		var factoryData mod.UpdateFactoryData

		farmIdStr := c.Param("farmid")
		turbineIdStr := c.Param("turbineId")
		if err = c.Bind(&factoryData); err != nil {
			ErrCheck(c, returnData, err, "数据解析失败")
			return err
		}

		if farmIdStr == "" {
			mainlog.Error("风场id转换int失败: %v", err)
			ErrCheck(c, returnData, err, "风场id转换int失败")
			return err
		}
		if turbineIdStr == "" {
			mainlog.Error("风机id转换int失败: %v", err)
			ErrCheck(c, returnData, err, "风机id转换int失败")
			return err
		}

		var data mod.Data
		if data, err = factoryData.InsertFactoryData(db, farmIdStr, turbineIdStr, ipport); err != nil {
			mainlog.Error("插入数据发生异常: %v", err)
			ErrCheck(c, returnData, err, "数据库插入数据发行异常")
			return err
		}

		ErrNil(c, returnData, data, "成功")
		return nil
	}
}

// 获取风机或风场的算法预警统计
func GetAlgorithmHandler(c echo.Context) (err error) {
	typeStr := c.Param("type")
	idStr := c.QueryParam("id")
	var returnData mod.ReturnData
	var algorith []mod.AlgorithmStatistic
	if idStr == "" {
		idStr = "0"
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败")
		ErrCheck(c, returnData, err, "id string转int失败")
		return err
	}
	switch typeStr {
	case "fan":
		query1 := db.Table("machine").
			Select("alert.strategy AS `name`, COUNT(alert.strategy) AS counts, ROUND(CAST(COUNT(CASE WHEN alert.confirm = 1 THEN 1 END) AS FLOAT) / COUNT(alert.strategy) * 100, 2) AS accuracy").
			Joins("RIGHT JOIN part ON part.machine_uuid = machine.uuid").
			Joins("RIGHT JOIN point ON point.part_uuid = part.uuid").
			Joins("RIGHT JOIN alert ON alert.point_uuid = point.uuid").
			Joins("LEFT JOIN algorithm ON algorithm.name = alert.strategy").
			Where("machine.id = ?", id).
			Where("alert.type != ?", "预警算法").
			Group("alert.strategy")
		query2 := db.Table("machine").
			Select("alert.strategy AS `name`, COUNT(alert.strategy) AS counts, ROUND(CAST(COUNT(CASE WHEN alert.confirm = 1 THEN 1 END) AS FLOAT) / COUNT(alert.strategy) * 100, 2) AS accuracy").
			Joins("RIGHT JOIN part ON part.machine_uuid = machine.uuid").
			Joins("RIGHT JOIN point ON point.part_uuid = part.uuid").
			Joins("RIGHT JOIN alert ON alert.point_uuid = point.uuid").
			Joins("LEFT JOIN algorithm ON algorithm.name = alert.strategy").
			Where("machine.id = ?", id).
			Where("alert.type = ?", "预警算法").
			Where("algorithm.is_del = ?", false).
			Group("alert.strategy")
		if err = db.Select("name, counts, accuracy").Table("(? UNION ALL ?) AS union_table", query1, query2).Find(&algorith).Error; err != nil {

			mainlog.Error("获取风机预警统计失败")
			ErrCheck(c, returnData, err, "获取风机预警统计失败")
			return err

		}

	case "farm":
		query1 := db.Table("windfarm").
			Select("alert.strategy AS `name`, COUNT(alert.strategy) AS counts, ROUND(CAST(COUNT(CASE WHEN alert.confirm = 1 THEN 1 END) AS FLOAT) / COUNT(alert.strategy) * 100, 2) AS accuracy").
			Joins("RIGHT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
			Joins("RIGHT JOIN part ON part.machine_uuid = machine.uuid").
			Joins("RIGHT JOIN point ON point.part_uuid = part.uuid").
			Joins("RIGHT JOIN alert ON alert.point_uuid = point.uuid").
			Joins("LEFT JOIN algorithm ON algorithm.name = alert.strategy").
			Where("windfarm.id = ?", id).
			Where("alert.type != ?", "预警算法").
			Group("alert.strategy")

		// 第二个子查询
		query2 := db.Table("windfarm").
			Select("alert.strategy AS `name`, COUNT(alert.strategy) AS counts, ROUND(CAST(COUNT(CASE WHEN alert.confirm = 1 THEN 1 END) AS FLOAT) / COUNT(alert.strategy) * 100, 2) AS accuracy").
			Joins("RIGHT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
			Joins("RIGHT JOIN part ON part.machine_uuid = machine.uuid").
			Joins("RIGHT JOIN point ON point.part_uuid = part.uuid").
			Joins("RIGHT JOIN alert ON alert.point_uuid = point.uuid").
			Joins("LEFT JOIN algorithm ON algorithm.name = alert.strategy").
			Where("windfarm.id = ?", id).
			Where("alert.type = ?", "预警算法").
			Where("algorithm.is_del = ?", false).
			Group("alert.strategy")
		if err = db.Select("name, counts, accuracy").Table("(? UNION ALL ?) AS union_table", query1, query2).Find(&algorith).Error; err != nil {
			mainlog.Error("获取风场预警统计失败")
			ErrCheck(c, returnData, err, "获取风场预警统计失败")
			return err

		}
	}
	ErrNil(c, returnData, algorith, "获取预警统计")
	return
}

// 获取风场故障反馈记录
func GetFarmFaultFeedBackHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var faultBack mod.FaultBackVo
	startTime := c.QueryParam("startTime")
	endTime := c.QueryParam("endTime")
	location := c.QueryParam("location")
	levelStr := c.QueryParam("level")
	fanIdStr := c.QueryParam("fanId")
	tagStr := c.QueryParam("tag")
	idStr := c.Param("id")
	part := c.QueryParam("part")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("风场id转int失败")
		ErrCheck(c, returnData, err, "风场id转int失败")
		return
	}
	condition := "1 = 1"
	if fanIdStr != "" {
		fanId, _ := strconv.Atoi(fanIdStr)
		condition = condition + fmt.Sprintf(" AND machineId = %d", fanId)
	}
	if location != "" {
		condition = condition + fmt.Sprintf(" AND location LIKE '%%%s%%'", location)
	}
	if levelStr != "" {
		level, _ := strconv.Atoi(levelStr)
		condition = condition + fmt.Sprintf(" AND level = %d", level)
	}
	if tagStr != "" {
		conditions := []string{}
		for _, s := range strings.Split(tagStr, ",") {
			conditions = append(conditions, fmt.Sprintf("`desc` LIKE '%%%s%%'", s))
		}
		condition = condition + " AND (" + strings.Join(conditions, " OR ") + ")"
	}

	if startTime != "" && endTime != "" {
		var startTimeSet, endTimeSet int64
		startTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", startTime)
		endTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", endTime)
		condition = condition + fmt.Sprintf(" AND timeSet BETWEEN %d AND %d", startTimeSet, endTimeSet)
	}

	if part != "" {
		condition = condition + fmt.Sprintf(" AND partType = '%s'", part)
	}

	midDB := db.Raw("(? UNION ALL ?) as temp",

		db.Table("alert").Select("alert.id id,machine.id machineId, alert.time_set timeSet,alert.time_set endTimeSet,alert.source source, machine.`desc` turbineName, alert.`level` `level` ,point.`name` location,alert.`desc` `desc`, part.type partType").
			Joins("LEFT JOIN point ON point.uuid = alert.point_uuid").Joins("LEFT JOIN part on part.uuid = point.part_uuid").
			Joins("LEFT JOIN machine on machine.uuid = part.machine_uuid").Joins("LEFT JOIN windfarm on windfarm.uuid = machine.windfarm_uuid").
			Where("windfarm.id = ? AND alert.deleted_at IS NULL", id),

		//故障反馈表：如果测点uuid不为空，则location显示point.name，测点uuid不存在,location为part.name
		db.Table("fault_back fb").
			Select("fb.id,machine.id machineId, fb.start_time_set timeSet, fb.end_time_set as endTimeSet,fb.source source, machine.`desc` turbineName, fb.`status` `status`, COALESCE(point.`name`, part.`name`) AS location,fb.tag `desc`,part.type partType").
			Joins("LEFT JOIN fault_tag_second ft on ft.id = fb.tag ").Joins("LEFT JOIN part on part.uuid = fb.part_uuid").
			Joins("LEFT JOIN point on point.uuid = fb.point_uuid").Joins("LEFT JOIN machine on machine.uuid = fb.machine_uuid").
			Joins("LEFT JOIN windfarm on windfarm.uuid = machine.windfarm_uuid").Where("windfarm.id = ? AND fb.is_del = FALSE", id),
	)

	if err = db.Table("?", midDB).Where(condition).Count(&faultBack.Total).Error; err != nil {
		mainlog.Error("获取风场故障反馈记录总数失败")
		ErrCheck(c, returnData, err, "获取风场故障反馈记录总数失败")
	}
	if err = db.Table("?", midDB).Order("timeSet DESC").Where(condition).Find(&faultBack.List).Error; err != nil {
		mainlog.Error("获取风场故障反馈记录失败")
		ErrCheck(c, returnData, err, "获取风场故障反馈记录失败")
	}

	//处理时间戳 --> 时间
	for key, value := range faultBack.List {
		if value.Source != 2 {
			faultBack.List[key].FaultTime = mod.TimetoStrFormat("2006-01-02 15:04:05", value.StartTimeSet)
		} else {
			faultBack.List[key].FaultTime = mod.TimetoStrFormat("2006-01-02 15:04:05", value.StartTimeSet) + "~" + mod.TimetoStrFormat("2006-01-02 15:04:05", value.EndTimeSet)
		}
	}
	ErrNil(c, returnData, faultBack, "获取风场故障反馈记录成功")
	return
}

// 新增算法
func AddAlgorithmHandler(c echo.Context) (err error) {
	var algorithm, algorithmDTO mod.Algorithm
	var returnData mod.ReturnData

	if err := c.Bind(&algorithm); err != nil {
		ErrCheck(c, returnData, err, "参数错误")
		mainlog.Error("参数错误 %v", err)
		return err
	}

	if err = db.Table("algorithm").Where("name = ? and is_del = false", algorithm.Name).Find(&algorithmDTO).Error; err != nil {
		mainlog.Error("新增算法时，查重失败 %v", err)
		ErrCheck(c, returnData, err, "新增算法时，查重失败")
		return err
	}
	if algorithmDTO.Id != 0 {
		err = errors.New("已存在同名算法")
		mainlog.Error("已存在同名算法 %v", err)
		ErrCheck(c, returnData, err, "已存在同名算法")
		return err
	}

	algorithm.CreateTime = mod.GetCurrentTime()
	algorithm.UpdateTime = mod.GetCurrentTime()
	algorithm.StartTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", algorithm.StartTime)
	algorithm.EndTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", algorithm.EndTime)
	if strings.Contains(algorithm.Name, "时频残差") {
		algorithm.Type = "A"
	} else if strings.Contains(algorithm.Name, "故障类型") {
		algorithm.Type = "B"
	} else {
		err = errors.New("算法名不包含时频残差或故障类型")
		ErrCheck(c, returnData, err, "算法名不包含时频残差或故障类型")
		return err
	}
	if err = db.Table("algorithm").Create(&algorithm).Error; err != nil {
		mainlog.Error("新增算法失败 %v", err)
		ErrCheck(c, returnData, err, "新增算法失败")
		return err
	}

	ErrNil(c, returnData, nil, "新增算法成功")
	return
}

// 删除算法
func DeleteAlgorithmHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, c.Request().URL.String()+" id解析失败")
		return err
	}

	if err = db.Table("algorithm").Where("id = ?", id).Updates(&mod.Algorithm{UpdateTime: mod.GetCurrentTime(), IsDel: true}).Error; err != nil {
		mainlog.Error("更新算法失败 %d %v", id, err)
		ErrCheck(c, returnData, err, "更新算法失败")
		return err

	}

	ErrNil(c, returnData, nil, "删除算法成功")
	return
}

// 更新算法
func UpdateAlgorithmHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var algorithm, algorithm2 mod.Algorithm
	if err := c.Bind(&algorithm); err != nil {
		mainlog.Error("参数错误 %v", err)
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}
	//更新前名字查重判断
	if err = db.Table("algorithm").Where("name = ? and id != ? and is_del =false", algorithm.Name, algorithm.Id).Find(&algorithm2).Error; err != nil {
		mainlog.Error("更新算法时，查重失败 %v", err)
		ErrCheck(c, returnData, err, "更新算法时，查重失败")
		return err
	}
	if algorithm2.Id != 0 {
		err = errors.New("已存在同名算法")
		mainlog.Error("已存在同名算法 %v", err)
		ErrCheck(c, returnData, err, "已存在同名算法")
		return err
	}
	algorithm.UpdateTime = mod.GetCurrentTime()
	algorithm.StartTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", algorithm.StartTime)
	algorithm.EndTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", algorithm.EndTime)
	if err = db.Table("algorithm").Where("id = ?", algorithm.Id).Save(&algorithm).Error; err != nil {
		mainlog.Error("更新算法失败 %v", err)
		ErrCheck(c, returnData, err, "更新算法失败")
		return err
	}

	ErrNil(c, returnData, nil, "更新算法成功")
	return
}

// 获取算法
func GetAlgorithmListHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var res mod.AlgorithmVo
	pageSizeStr := c.QueryParam("pageSize")
	pageNumStr := c.QueryParam("pageNum")
	keyword := c.QueryParam("keyword")
	if pageSizeStr == "" {
		pageSizeStr = "99"
	}

	if pageNumStr == "" {
		pageNumStr = "1"
	}
	pageSize, err := strconv.Atoi(pageSizeStr)

	if err != nil {
		mainlog.Error("解析pageSize失败 %v", err)
		ErrCheck(c, returnData, err, "解析pageSize失败")
		return err
	}

	pageNum, err := strconv.Atoi(pageNumStr)
	if err != nil {
		mainlog.Error("解析pageNum失败 %v", err)
		ErrCheck(c, returnData, err, "解析pageNum失败")
		return err
	}

	if err = db.Table("algorithm").Where("is_del = false and name like ?", "%"+keyword+"%").Count(&res.Total).Error; err != nil {
		mainlog.Error("获取算法总数失败 %v", err)
		ErrCheck(c, returnData, err, "获取算法总数失败")
		return err
	}

	if err = db.Table("algorithm").Where("is_del = false and name like ?", "%"+keyword+"%").Limit(pageSize).Offset((pageNum - 1) * pageSize).Find(&res.List).Error; err != nil {
		mainlog.Error("获取算法失败 %v", err)
		ErrCheck(c, returnData, err, "获取算法失败")
		return err
	}
	for i, algorithm := range res.List {
		res.List[i].StartTime = time.Unix(algorithm.StartTimeSet, 0).Format("2006-01-02 15:04:05")
		res.List[i].EndTime = time.Unix(algorithm.EndTimeSet, 0).Format("2006-01-02 15:04:05")
	}
	ErrNil(c, returnData, res, "获取算法成功")
	return
}

// 根据对应测点获取算法列表
func GetAlgorithmListByPointUUIDHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var res mod.AlgorithmVo
	pointUUID := c.QueryParam("pointUUID")

	if err = db.Table("algorithm").Where("is_del = false and enabled = true and point_uuid = ?", pointUUID).Count(&res.Total).Error; err != nil {
		mainlog.Error("获取算法总数失败 %v", err)
		ErrCheck(c, returnData, err, "获取算法总数失败")
		return err
	}

	if err = db.Table("algorithm").Where("is_del = false and enabled = true and point_uuid = ?", pointUUID).Find(&res.List).Error; err != nil {
		mainlog.Error("获取算法失败 %v", err)
		ErrCheck(c, returnData, err, "获取算法失败")
		return err
	}

	ErrNil(c, returnData, res, "获取解析方式成功")
	return
}

// 启动算法
func StartAlgorithmHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var algorithm mod.Algorithm
	algorithmIdStr := c.QueryParam("id")
	if algorithm.Id, err = strconv.Atoi(algorithmIdStr); err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, c.Request().URL.String()+" id解析失败")
		return
	}
	// 查找算法相关参数
	if err = db.Table("algorithm").Where("id = ?", algorithm.Id).Find(&algorithm).Error; err != nil {
		mainlog.Error("获取算法失败 %v", err)
		ErrCheck(c, returnData, err, "获取算法失败")
		return
	}
	// 查找测点所属风机，组建data表名
	var machine mod.Machine
	if err = db.Table("point").Joins("RIGHT JOIN part ON part.uuid = point.part_uuid").
		Joins("RIGHT JOIN machine on machine.uuid = part.machine_uuid").Select("machine.*").Where("point.uuid = ?", algorithm.PointUUID).
		Find(&machine).Error; err != nil {
		ErrCheck(c, returnData, err, "获取测点所属风机失败")
		return
	}

	dataTableName := fmt.Sprintf("data_%d", machine.ID)
	waveTableName := fmt.Sprintf("wave_%d", machine.ID)
	// 查找所有未执行预警算法的数据 is_predicted

	var dataList []mod.Data
	if err = db.Table(dataTableName).Where("point_uuid = ? AND time_set >= ? AND time_set <= ? AND is_predicted = false", algorithm.PointUUID, algorithm.StartTimeSet, algorithm.EndTimeSet).Find(&dataList).Error; err != nil {
		mainlog.Error("获取数据失败 %v", err)
		ErrCheck(c, returnData, err, "获取数据失败")
		return
	}
	for i := range dataList {
		// 查找原始数据
		if err = db.Table(waveTableName).Where("data_uuid = ?", dataList[i].UUID).Find(&dataList[i].Wave).Error; err != nil {
			mainlog.Error("获取原始数据失败 %v", err)
			ErrCheck(c, returnData, err, "获取原始数据失败")
			return
		}

		//dataList[i].Wave.DataString = strings.Trim(string(dataList[i].Wave.File), " ")
		// 执行算法
		if err = algorithm.ExecuteAlgorithm(&dataList[i], db, strconv.Itoa(int(machine.ID))); err != nil {
			mainlog.Error("执行算法失败 %v", err)
			ErrCheck(c, returnData, err, "执行算法失败")
			return
		}
	}
	ErrNil(c, returnData, nil, "启动算法成功")
	return
}
func GetHistoryByIdHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	idStr := c.Param("id")
	pageSizeStr := c.QueryParam("pageSize")
	if pageSizeStr == "" {
		pageSizeStr = "99999"
	}
	pageNumStr := c.QueryParam("pageNum")
	if pageNumStr == "" {
		pageNumStr = "1"
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil {
		mainlog.Error("pageSize转换失败 %v", err)
		ErrCheck(c, returnData, err, "pageSize转换失败")
		return
	}
	pageNum, err := strconv.Atoi(pageNumStr)
	if err != nil {
		mainlog.Error("pageNum转换失败 %v", err)
		ErrCheck(c, returnData, err, "pageNum转换失败")
		return
	}
	id, err := strconv.Atoi(idStr)
	var algorithm mod.Algorithm
	if db.Model(&mod.Algorithm{}).Where("id = ? AND is_del = false", id).Find(&algorithm).RowsAffected <= 0 {
		ErrCheck(c, returnData, nil, "算法不存在")
		return
	}
	res := struct {
		List  interface{} `json:"list"`
		Total int64       `json:"total"`
	}{}
	var tableName string
	switch algorithm.Type {
	case "A":
		tableName = "algorithm_result_a"
		res.List = []mod.AlgorithmResultA{}
	case "B":
		tableName = "algorithm_result_b"
		res.List = []mod.AlgorithmResultB{}
	}
	if err = db.Table(tableName).Where("algorithm_id = ? AND is_del = false", algorithm.Id).Count(&res.Total).Error; err != nil {
		ErrCheck(c, returnData, nil, "获取算法历史执行记录失败")
		return
	}
	if err = db.Table(tableName).Where("algorithm_id = ? AND is_del = false", algorithm.Id).Order("id DESC ").Offset(pageSize * (pageNum - 1)).Limit(pageSize).Find(&res.List).Error; err != nil {
		ErrCheck(c, returnData, nil, "获取算法结果失败")
		return
	}
	ErrNil(c, returnData, res, "获取算法历史执行记录成功")
	return
}

// 获取解析方式列表
func GetParsingHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var parsing mod.ParsingRESP
	pageSizeStr := c.QueryParam("pageSize")
	pageNumStr := c.QueryParam("pageNum")

	if pageNumStr == "" {
		pageNumStr = "0"
	}
	if pageSizeStr == "" {
		pageSizeStr = "99"
	}
	pageSize, err := strconv.Atoi(pageSizeStr)
	if err != nil {
		mainlog.Error("解析pageSize失败 %v", err)
		ErrCheck(c, returnData, err, "解析pageSize失败")
		return err
	}
	pageNum, err := strconv.Atoi(pageNumStr)
	if err != nil {
		mainlog.Error("解析pageNum失败 %v", err)
		ErrCheck(c, returnData, err, "解析pageNum失败")
		return err
	}
	if err = db.Table("parsing").Where("is_del = false").Offset((pageNum - 1) * pageSize).Limit(pageSize).Find(&parsing.List).Error; err != nil {
		mainlog.Error("获取解析方式失败 %v", err)
		ErrCheck(c, returnData, err, "获取解析方式失败")
		return err
	}

	if err = db.Table("parsing").Where("is_del = false").Count(&parsing.Total).Error; err != nil {
		mainlog.Error("获取解析方式失败 %v", err)
		ErrCheck(c, returnData, err, "获取解析方式失败")
		return err
	}

	ErrNil(c, returnData, parsing, "获取解析方式成功")
	return
}

// 新增解析方式
func AddParsingHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var parsingDTO mod.Parsing
	if err = c.Bind(&parsingDTO); err != nil {
		mainlog.Error("参数错误 %v", err)
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}

	var parsing mod.Parsing
	if err = db.Table("parsing").Where("name = ? AND is_del = false", parsingDTO.Name).Find(&parsing).Error; err != nil {
		mainlog.Error("获取解析方式失败 %v", err)
		ErrCheck(c, returnData, err, "获取解析方式失败")
		return err
	}
	if parsing.Id != 0 {
		err = errors.New("解析方式已存在")
		ErrCheck(c, returnData, err, "解析方式已存在")
		return err
	}
	parsingDTO.CreateTime = mod.GetCurrentTime()
	parsingDTO.UpdateTime = mod.GetCurrentTime()
	if err = db.Table("parsing").Create(&parsingDTO).Error; err != nil {
		mainlog.Error("新增解析方式失败 %v", err)
		ErrCheck(c, returnData, err, "新增解析方式失败")
		return err
	}
	ErrNil(c, returnData, true, "新增解析方式成功")
	return
}

// 删除解析方式
func DeleteParsingHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, c.Request().URL.String()+" id解析失败")
		return err
	}

	if err = db.Table("parsing").Where("id = ?", id).Updates(&mod.Parsing{UpdateTime: mod.GetCurrentTime(), IsDel: true}).Error; err != nil {
		mainlog.Error("删除解析方式失败 %v", err)
		ErrCheck(c, returnData, err, "删除解析方式失败")
		return err
	}

	ErrNil(c, returnData, true, "删除解析方式成功")
	return
}

// 更新解析方式
func UpdateParsingHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var parsing, parsingDTO mod.Parsing
	if err = c.Bind(&parsing); err != nil {
		mainlog.Error("参数错误 %v", err)
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}

	if err = db.Table("parsing").Where("id != ? AND name = ? AND is_del = false", parsing.Id, parsing.Name).Find(&parsingDTO).Error; err != nil {
		mainlog.Error("更新时，查重失败：%v", err)
		ErrCheck(c, returnData, err, "更新时，查重失败")
		return err
	}

	if parsingDTO.Id != 0 {
		err = errors.New("解析方式已存在")
		ErrCheck(c, returnData, err, "解析方式已存在")
		return err
	}

	parsing.UpdateTime = mod.GetCurrentTime()
	if err = db.Table("parsing").Updates(&parsing).Error; err != nil {
		mainlog.Error("更新解析方式失败 %v", err)
		ErrCheck(c, returnData, err, "更新解析方式失败")
		return err
	}
	ErrNil(c, returnData, true, "更新解析方式成功")
	return
}

func GetFaultTagByTypeHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var res []mod.FaultTagFirst
	part := c.Param("part")
	condition := ""
	if part == ":part" || part == "" || part == "undefined" {
		condition = "type IN ('主轴承','齿轮箱','发电机','叶片','塔筒','机舱') AND is_del = false"
	} else {
		condition = fmt.Sprintf("type = '%s' AND is_del = false", part)
	}
	if err = db.Table("fault_tag_first").Where(condition).Preload("Childrens").Find(&res).Error; err != nil {
		mainlog.Error("获取故障标签失败 %v", err)
		ErrCheck(c, returnData, err, "获取故障标签失败")
		return err
	}
	ErrNil(c, returnData, res, "获取故障标签成功")
	return
}

// 获取故障标签
func GetFaultTagHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var res []mod.FaultTagFirst
	part := c.QueryParam("part_uuid")
	condition := "1 = 1 AND is_del = false"
	if part != "" {
		// 查询部件的类型
		var partType string
		if err = db.Table("part").Select("type").Where("uuid = ?", part).Find(&partType).Error; err != nil {
			mainlog.Error("获取故障标签失败 %v", err)
			ErrCheck(c, returnData, err, "获取故障标签失败")
			return err
		}
		condition = condition + fmt.Sprintf(" AND `type` = '%s'", partType)
	}
	if err = db.Model(&mod.FaultTagFirst{}).Where(condition).Preload("Childrens", "is_del = false").Find(&res).Error; err != nil {
		mainlog.Error("获取故障标签失败 %v", err)
		ErrCheck(c, returnData, err, "获取故障标签失败")
		return err
	}
	ErrNil(c, returnData, res, "获取故障标签成功")
	return
}

// 新增故障标签
func AddFaultTagHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var faultTag mod.FaultTagSecond
	if err = c.Bind(&faultTag); err != nil {
		mainlog.Error("参数错误 %v", err)
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}
	faultTag.Source = false
	if err = db.Model(mod.FaultTagSecond{}).Create(&faultTag).Error; err != nil {
		mainlog.Error("新增故障标签失败 %v", err)
		ErrCheck(c, returnData, err, "新增故障标签失败")
		return err
	}

	ErrNil(c, returnData, true, "新增故障标签成功")
	return
}

// 修改故障标签
func UpdateFaultTagHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var faultTag mod.FaultTagSecond
	if err = c.Bind(&faultTag); err != nil {
		mainlog.Error("参数错误 %v", err)
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}

	if err = db.Model(mod.FaultTagSecond{}).Where("id = ? AND is_del = false", faultTag.Id).Updates(&faultTag).Error; err != nil {
		mainlog.Error("更新故障标签失败 %v", err)
		ErrCheck(c, returnData, err, "更新故障标签失败")
		return err
	}

	ErrNil(c, returnData, true, "更新故障标签成功")
	return
}

// @Title DeleteFaultTagHandler
// @Description 删除故障反馈标签
// @Author MuXi 2023-12-14 14:14:04
// @Param c
// @Return err
func DeleteFaultTagHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	idStr := c.Param("id")
	if idStr == "" {
		mainlog.Error("参数错误")
		ErrCheck(c, returnData, nil, "参数错误")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, c.Request().URL.String()+" id解析失败")
		return
	}

	//首先查询当前标签所有的子标签
	if err = db.Model(mod.FaultTagSecond{}).Where("id = ? AND is_del = false", id).Updates(&mod.FaultTagSecond{IsDel: true}).Error; err != nil {
		mainlog.Error("删除故障标签失败 %v", err)
		ErrCheck(c, returnData, err, "删除故障标签失败")
		return err
	}

	ErrNil(c, returnData, true, "删除故障标签成功")
	return
}

// AddFaultFeedbackHandler 是一个处理添加故障反馈的函数
func AddFaultFeedbackHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var faultFeedback mod.FaultBack

	// 解析传入的故障反馈参数
	if err = c.Bind(&faultFeedback); err != nil {
		mainlog.Error("参数错误 %v", err)
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}

	// 开始数据库事务
	tx := db.Begin()

	// 插入故障信息到故障信息表
	faultFeedback.StartTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", faultFeedback.FaultStartTime)
	faultFeedback.EndTimeSet, _ = mod.StrtoTime("2006-01-02 15:04:05", faultFeedback.FaultEndTime)
	faultFeedback.CreateTime = mod.GetCurrentTime()
	faultFeedback.UpdateTime = mod.GetCurrentTime()
	if err = tx.Table("fault_back").Create(&faultFeedback).Error; err != nil {
		tx.Rollback()
		mainlog.Error("新增故障反馈失败 %v", err)
		ErrCheck(c, returnData, err, "新增故障反馈失败")
		return err
	}

	var machineIdStr string
	// 根据风机UUID查询风机ID
	if err = tx.Table("machine").Where("uuid = ?", faultFeedback.MachineUUID).Select("id").Find(&machineIdStr).Error; err != nil {
		tx.Rollback()
		mainlog.Error("查询风机id失败 %v", err)
		ErrCheck(c, returnData, err, "查询风机id失败")
		return err
	}
	if faultFeedback.PointUUID == "" {
		// 如果故障反馈信息中 point_uuid 为空，则更新相关联的所有在 FaultStartTime 和 FaultEndTime 之间的测点数据
		var associatedPointsUUID []string
		// 查询故障相关部件的所有测点
		if err = tx.Table("point").Select("point.uuid").Joins("right join part on part.uuid = point.part_uuid").
			Where("part.uuid = ?", faultFeedback.PartUUID).Find(&associatedPointsUUID).Error; err != nil {
			tx.Rollback()
			mainlog.Error("查询相关测点失败: %v", err)
			ErrCheck(c, returnData, err, "新增故障反馈失败：查找测点数据失败")
			return err
		}
		// 遍历更新所有相关测点在指定时间范围内的数据, 不为空则追加,
		// 例如：tag = "1,2,3" faultFeedback.Tag = "1,2,5", 则将data的tag更新为"1,2,3,5"
		for _, pointUUID := range associatedPointsUUID {
			//查询该测点在指定时间范围内的数据
			var datas []mod.Data
			if err = tx.Table("data_"+machineIdStr).Where("point_uuid = ? AND status != 1 AND time_set BETWEEN ? AND ?", pointUUID, faultFeedback.StartTimeSet, faultFeedback.EndTimeSet).
				Find(&datas).Error; err != nil {
				tx.Rollback()
				mainlog.Error("查询测点数据失败: %v", err)
				ErrCheck(c, returnData, err, "新增故障反失败：查询测点数据失败")
			}
			// 开始更新测点数据
			for _, data := range datas {
				var tag string
				// 如果该data的tag为空, 直接赋值
				if data.Tag != "" {
					tag = appendTags(data.Tag, faultFeedback.Tag)
				} else {
					tag = faultFeedback.Tag
				}
				if err = tx.Table("data_"+machineIdStr).Where("id =?", data.ID).Update("tag", tag).Error; err != nil {
					tx.Rollback()
					mainlog.Error("更新测点数据失败: %v", err)
					ErrCheck(c, returnData, err, "新增故障反馈失败：更新测点数据失败")
					return err
				}
			}
		}
	} else {
		// 如果故障反馈信息中 point_uuid 不为空，则更新该测点在 FaultStartTime 和 FaultEndTime 之间的数据
		// 查询该测点在指定时间范围内的数据
		var datas []mod.Data
		if err = tx.Table("data_"+machineIdStr).Where("point_uuid =? AND status != 1 AND time_set BETWEEN ? AND ?", faultFeedback.PointUUID, faultFeedback.StartTimeSet, faultFeedback.EndTimeSet).
			Find(&datas).Error; err != nil {
			tx.Rollback()
			mainlog.Error("查询指定测点数据失败: %v", err)
			ErrCheck(c, returnData, err, "查询指定测点数据失败")
		}
		// 开始更新测点数据
		for _, data := range datas {
			var tag string
			// 如果该data的tag为空, 直接赋值
			// 不为空调用appendTags函数，将该标签追加到已有的标签中
			if data.Tag != "" {
				tag = appendTags(data.Tag, faultFeedback.Tag)
			} else {
				tag = faultFeedback.Tag
			}
			if err = tx.Table("data_"+machineIdStr).Where("id =?", data.ID).Update("tag", tag).Error; err != nil {
				tx.Rollback()
				mainlog.Error("更新测点数据失败: %v", err)
				ErrCheck(c, returnData, err, "新增故障反馈失败：更新测点数据失败")
				return err
			}
		}
	}
	tx.Commit()
	ErrNil(c, returnData, true, "新增故障反馈成功")
	return
}

// @Title appendTags
// @Description  自定义函数，用来追加数据标签，去除重复标签
// @Author DengHui 2023-12-25 15:06:52
// @Param existingTag
// @Param newTag
// @Return string
func appendTags(existingTag, newTag string) string {
	if existingTag == "" {
		return newTag
	}
	existingTags := make(map[string]bool)
	// 将已有的标签存入 map
	for _, tag := range strings.Split(existingTag, ",") {
		existingTags[tag] = true
	}
	// 将新的标签分割后，依次加入到已存在标签中
	for _, tag := range strings.Split(newTag, ",") {
		if _, exists := existingTags[tag]; !exists {
			existingTag += "," + tag
			existingTags[tag] = true
		}
	}
	// 去除开头的逗号
	if strings.HasPrefix(existingTag, ",") {
		existingTag = existingTag[1:]
	}
	return existingTag
}

// GetFarmFaultFeedBackByIdHandler
//
//	@Description:根据故障反馈id，以及source来查询故障反馈信息
//	@param c
//	@return err
func GetFarmFaultFeedBackByIdHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	idStr := c.Param("id")
	sourceStr := c.QueryParam("source")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, c.Request().URL.String()+" id解析失败")
		return
	}

	source, err := strconv.Atoi(sourceStr)
	if err != nil {
		mainlog.Error("source string转int失败 %v", err)
		ErrCheck(c, returnData, err, c.Request().URL.String()+" source解析失败")
		return
	}

	switch source {
	case 0, 1:
		var aler mod.FaultBackInfo
		if err = db.Table("alert").
			Select("alert.id id,alert.time_set time_set,alert.source source,alert.check_time check_time,alert.suggest suggest, alert.level status ,machine.uuid machine_uuid, part.uuid part_uuid, part.uuid part_uuid").
			Joins("left join point on point.uuid = alert.point_uuid").Joins("left join part on part.uuid = point.part_uuid").
			Joins("left join machine on machine.uuid = part.machine_uuid").Where("alert.id = ?", id).Find(&aler).Error; err != nil {
			mainlog.Error("根据id查询故障反馈失败 %v", err)
			ErrCheck(c, returnData, err, "根据id查询故障反馈失败")
			return err
		}
		if aler.Id != 0 {
			aler.FaultTime = mod.TimetoStrFormat("2006-01-02 15:04:05", aler.TimeSet)
			ErrNil(c, returnData, aler, "根据id查询故障反馈成功")
		} else {
			ErrNil(c, returnData, nil, "根据id查询故障反馈成功")
		}
	case 2:
		var faultFeedback mod.FaultBackInfo
		if err = db.Table("fault_back fb").Select("fb.*, file.name fileName, file.dir fileDir").Joins("LEFT join file on file.id = fb.file_id and file.is_del = false").Where("fb.id = ? AND fb.is_del = false", id).Find(&faultFeedback).Error; err != nil {
			mainlog.Error("根据id查询故障反馈失败 %v", err)
			ErrCheck(c, returnData, err, "根据id查询故障反馈失败")
			return err
		}

		if faultFeedback.Id != 0 {
			faultFeedback.FaultTime = mod.TimetoStrFormat("2006-01-02 15:04:05", faultFeedback.TimeSet)
			faultFeedback.FaultStartTime = mod.TimetoStrFormat("2006-01-02 15:04:05", faultFeedback.TimeSet)
			faultFeedback.FaultEndTime = mod.TimetoStrFormat("2006-01-02 15:04:05", faultFeedback.EndTimeSet)
			ErrNil(c, returnData, faultFeedback, "根据id查询故障反馈成功")
		} else {
			ErrNil(c, returnData, nil, "根据id查询故障反馈成功")
		}
	}
	return
}

func DeleteFaultFeedbackHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	sourceStr := c.QueryParam("source")
	fmt.Println(sourceStr)
	idStr := c.Param("id")
	if sourceStr == "" {
		err = errors.New("参数错误")
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}
	source, err := strconv.Atoi(sourceStr)
	if err != nil {
		ErrCheck(c, returnData, err, "source解析失败")
		return err
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, c.Request().URL.String()+" id解析失败")
		return err
	}
	// TODO 2023/12/14 删除故障反馈后，需要删除对应的测点数据
	switch source {
	case 0, 1:
		break
	case 2:
		var fault mod.FaultBack
		// 根据id查询故障反馈对应数据
		if err = db.Table("fault_back").Where("id = ? and is_del = false", id).Find(&fault).Error; err != nil {
			mainlog.Error("根据id查询故障反馈失败 %v", err)
			ErrCheck(c, returnData, err, "根据id查询故障反馈失败")
			return err
		}
		var machineId string
		//根据故障信息查询所属风机id
		if err = db.Table("machine").Select("id").Where("uuid = ?", fault.MachineUUID).Find(&machineId).Error; err != nil {
			mainlog.Error("查询故障相关风机失败: %v", err)
			ErrCheck(c, returnData, err, "删除故障反馈失败：查询风机数据失败")
			return err
		}
		tx := db.Begin()
		//首先删除关联文件
		if err = tx.Table("file").Where("id = ?", fault.FileId).Updates(&mod.File{UpdateTime: mod.GetCurrentTime(), IsDel: true}).Error; err != nil {
			tx.Rollback()
			mainlog.Error("删除故障反馈失败 %v", err)
			ErrCheck(c, returnData, err, "删除故障反馈失败")
			return err
		}
		if fault.PointUUID == "" {
			// 如果故障反馈信息中 point_uuid 为空，则删除相关联的所有在 FaultStartTime 和 FaultEndTime 之间的测点数据的标签
			var associatedPointsUUID []string
			// 查询故障相关部件的所有测点
			if err = tx.Table("point").Joins("right join part on part.uuid = point.part_uuid").
				Where("part.uuid = ?", fault.PartUUID).Find(&associatedPointsUUID).Error; err != nil {
				tx.Rollback()
				mainlog.Error("查询相关测点失败: %v", err)
				ErrCheck(c, returnData, err, "删除测点数据标签失败")
				return err
			}
			// 删除所有相关测点在指定时间范围内的标签数据
			for _, pointUUID := range associatedPointsUUID {
				if err = tx.Table("data_"+machineId).Where("point_uuid = ? AND time_set BETWEEN ? AND ?", pointUUID, fault.StartTimeSet, fault.EndTimeSet).
					Update("tag", "").Error; err != nil {
					tx.Rollback()
					mainlog.Error("删除测点数据标签失败: %v", err)
					ErrCheck(c, returnData, err, "删除测点数据标签失败")
					return err
				}
			}
		} else {
			// 如果故障反馈信息中 point_uuid 不为空，则删除相关联的测点数据的标签
			if err = tx.Table("data_"+machineId).Where("point_uuid = ? AND time_set BETWEEN ? AND ?", fault.PointUUID, fault.StartTimeSet, fault.EndTimeSet).
				Update("tag", "").Error; err != nil {
				tx.Rollback()
				mainlog.Error("删除测点数据标签失败: %v", err)
				ErrCheck(c, returnData, err, "删除测点数据标签失败")
				return err
			}
		}
		// 最后更新故障反馈的信息，
		fault.UpdateTime = mod.GetCurrentTime()
		fault.IsDel = true
		if err = tx.Table("fault_back").Where("id = ?", id).Updates(&fault).Error; err != nil {
			tx.Rollback()
			mainlog.Error("删除故障反馈失败 %v", err)
			ErrCheck(c, returnData, err, "删除故障反馈失败")
			return err
		}
		// 提交事务
		tx.Commit()
		ErrNil(c, returnData, true, "删除故障反馈成功")
	}
	return
}

func UpdateFaultFeedbackHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var faultFeedback mod.FaultBackUpdate
	var fault mod.FaultBack
	if err = c.Bind(&faultFeedback); err != nil {
		mainlog.Error("参数错误 %v", err)
		ErrCheck(c, returnData, err, "参数错误")
		return err
	}
	if err = db.Table("fault_back").Where("id =?", faultFeedback.Id).Find(&fault).Error; err != nil {
		mainlog.Error("更新故障反馈失败 %v", err)
		ErrCheck(c, returnData, err, "更新故障反馈失败")
		return err
	}
	tx := db.Begin()
	// 更新故障反馈id, 如果原有故障反馈有附件id， 则删除原有故障反馈的附件，在重新赋值附件id
	if fault.FileId != 0 {
		if err = tx.Table("file").Where("id =?", fault.FileId).Updates(&mod.File{UpdateTime: mod.GetCurrentTime(), IsDel: true}).Error; err != nil {
			tx.Rollback()
			mainlog.Error("删除故障反馈失败 %v", err)
			ErrCheck(c, returnData, err, "删除故障反馈失败")
			return err
		}
	}
	// 先更新附件id为0在更新其他字段

	// 使用map， 避免更新时，updates函数不更近false值
	if err = tx.Table("fault_back").Where("id =?", faultFeedback.Id).Updates(map[string]interface{}{"file_id": 0}).Error; err != nil {
		tx.Rollback()
		mainlog.Error("更新故障反馈失败 %v", err)
		ErrCheck(c, returnData, err, "更新故障反馈失败")
		return err
	}
	// 新附件id不为空，则不管原有附件id是否空，直接赋值为新附件id
	faultFeedback.UpdateTime = mod.GetCurrentTime()
	if err = tx.Table("fault_back").Where("id =?", faultFeedback.Id).Updates(&faultFeedback).Error; err != nil {
		tx.Rollback()
		mainlog.Error("更新故障反馈失败 %v", err)
		ErrCheck(c, returnData, err, "更新故障反馈失败")
		return err
	}
	tx.Commit()
	ErrNil(c, returnData, true, "更新故障反馈成功")
	return
}

func UploadFile(c echo.Context) (err error) {
	var returnData mod.ReturnData
	mainlog.Info("上传文件")
	file, err := c.FormFile("files")
	if err != nil {
		mainlog.Error("上传文件失败 %v", err)
		ErrCheck(c, returnData, err, "上传文件失败")
		return err
	}
	flag := 0
	var fileDTO mod.File
	src, err := file.Open()
	if err != nil {
		flag = 1
	}

	defer src.Close()
	fullFileName := file.Filename                                           //完整名
	fileNameWithSuffix := path.Base(fullFileName)                           //文件名带后缀
	fileSuffix := path.Ext(fileNameWithSuffix)                              //获取后缀
	fileWithOutSuffix := strings.TrimSuffix(fileNameWithSuffix, fileSuffix) //文件名不带后缀
	fileWithOutSuffixMD5 := utils.EncodeMD5(fileWithOutSuffix)              //文件名不带后缀exe -->  加密

	fileDTO.Name = fullFileName
	fileDTO.MD5Name = fileWithOutSuffixMD5 + fileSuffix

	dataDir := "/upload/"
	_, err = os.Stat(dataDir)
	dirFlag := false
	if err != nil {
		if os.IsNotExist(err) {
			mainlog.Info("文件夹不存在，创建文件夹")
			err = os.Mkdir(dataDir, os.ModePerm)
			if err != nil {
				mainlog.Error("创建文件夹失败 %v", err)
				dirFlag = true
			}
		} else {
			dirFlag = true
		}
	}

	if dirFlag {
		flag = 2
	}

	dst, err := os.Create("." + dataDir + fileWithOutSuffixMD5 + fileSuffix)
	if err != nil {
		flag = 3

	}

	defer dst.Close()
	if _, err = io.Copy(dst, src); err != nil {
		flag = 4

	}
	fileDTO.Dir = dataDir + fileDTO.MD5Name
	fileDTO.CreateTime = mod.GetCurrentTime()
	fileDTO.UpdateTime = mod.GetCurrentTime()

	//插入前，进行md5查重
	var fileRESP mod.File
	if err = db.Table("file").Where("md5_name = ? and is_del = false", fileDTO.MD5Name).Find(&fileRESP).Error; err != nil {
		flag = 5

	}
	if fileRESP.Id != 0 {
		flag = 6
	}
	if err = db.Table("file").Create(&fileDTO).Error; err != nil {
		flag = 7
	}
	switch flag {
	case 1:
		err = errors.New("打开文件失败")
	case 2:
		err = errors.New("文件夹创建失败")
	case 3:
		err = errors.New("创建目标文件失败")
	case 4:
		err = errors.New("文件复制失败")
	case 5:
		err = errors.New("查重失败")
	case 6:
		err = errors.New("文件已存在")
	case 7:
		err = errors.New("插入数据库失败")
	}

	if flag == 0 {
		ErrNil(c, returnData, fileDTO, "文件上传成功")
	} else {
		ErrCheck(c, returnData, err, "文件上传失败")

	}
	return
}

func GetAllFileHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	type FileRESP struct {
		Total int64      `json:"total"`
		List  []mod.File `json:"list"`
	}

	var resp FileRESP
	if err = db.Table("file").Where("is_del = false").Count(&resp.Total).Error; err != nil {
		mainlog.Error("获取文件列表失败 %v", err)
		ErrCheck(c, returnData, err, "获取文件列表失败")
		return
	}

	if err = db.Table("file").Where("is_del = false").Find(&resp.List).Error; err != nil {
		mainlog.Error("获取文件列表失败 %v", err)
		ErrCheck(c, returnData, err, "获取文件列表失败")
		return
	}

	ErrNil(c, returnData, resp, "文件列表获取成功")
	return
}

func DeleteFileHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	idStr := c.Param("id")

	id, err := strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, "id string转int失败")
		return
	}

	if err = db.Table("file").Where("id = ?", id).Updates(&mod.File{UpdateTime: mod.GetCurrentTime(), IsDel: true}).Error; err != nil {
		mainlog.Error("删除文件失败 %v", err)
		ErrCheck(c, returnData, err, "删除文件失败")
		return
	}

	ErrNil(c, returnData, nil, "文件删除成功")
	return
}

// 测点趋势图获取模型名称，绘图时传递对应是英文名，英文名作为查询条件,查询数据表中对应的字段
func GetModelsHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	var res mod.ModelsVo
	if err = db.Table("model").Where("is_del = false").Count(&res.Total).Error; err != nil {
		mainlog.Error("获取模型列表失败 %v", err)
		ErrCheck(c, returnData, err, "获取模型列表失败")
		return
	}

	if err = db.Table("model").Where("is_del = false").Find(&res.List).Error; err != nil {
		mainlog.Error("获取模型列表失败 %v", err)
		ErrCheck(c, returnData, err, "获取模型列表失败")
		return
	}

	ErrNil(c, returnData, res, "获取模型列表成功")
	return
}

// 2023年12月11日10:04:35 更新数据标签
// 更新数据标签不做追加，直接赋值
func UpdateDataLabel(c echo.Context) (err error) {
	var returnData mod.ReturnData
	type Condition struct {
		Tag    string `json:"tag"`
		DataId int    `json:"dataId"`
	}
	machineIdStr := c.Param("machineId")
	var condition Condition
	if err = c.Bind(&condition); err != nil {
		mainlog.Error("参数错误")
		ErrCheck(c, returnData, err, "参数错误")
		return
	}
	if condition.DataId == 0 {
		mainlog.Error("参数错误")
		err = errors.New("数据id为空")
		ErrCheck(c, returnData, err, "参数错误")
		return
	}
	if err = db.Table("data_"+machineIdStr).Where("id = ?", condition.DataId).Update("tag", condition.Tag).Error; err != nil {
		mainlog.Error("更新数据标签失败 %v", err)
		ErrCheck(c, returnData, err, "更新数据标签失败")
		return
	}
	ErrNil(c, returnData, nil, "更新数据标签成功")
	return
}

// 获取文档信息
func GetDocumentHandler(c echo.Context) (err error) {
	var returnData mod.ReturnData
	res := mod.DocumentStruct{
		StartTime: c.QueryParam("startTime"),
		EndTime:   c.QueryParam("endTime"),
	}
	res.StartTimeSet, _ = mod.StrtoTime("2006-01-02", res.StartTime)
	res.EndTimeSet, _ = mod.StrtoTime("2006-01-02", res.EndTime)
	idStr := c.Param("id")
	res.WindfarmId, err = strconv.Atoi(idStr)
	if err != nil {
		mainlog.Error("id string转int失败 %v", err)
		ErrCheck(c, returnData, err, "id string转int失败")
		return
	}
	res.SampleTime = res.StartTime + "~" + res.EndTime
	// 1、 首先查询风场、风机数量
	if err = getWindfarmAndMachineCount(&res); err != nil {
		mainlog.Error("获取风场、风机数量失败 %v", err)
		ErrCheck(c, returnData, err, "获取风场、风机数量失败")
		return
	}

	// 2、获取机组信息
	if err = getMachineTypes(&res); err != nil {
		mainlog.Error("获取机组信息 %v", err)
		ErrCheck(c, returnData, err, "获取机组信息失败")
		return
	}

	// 3、获取风机部件信息 主轴承、齿轮箱、发电机、叶片
	if err = getMachineComponentsInfo(&res); err != nil {
		mainlog.Error("获取风机部件信息失败 %v", err)
		ErrCheck(c, returnData, err, "获取风机部件信息失败")
		return
	}

	// 4、获取风机、部件、测点、报警详细信息
	if err = getMachineComponentsDetails(&res); err != nil {
		mainlog.Error("获取风机部件报警详细信息失败 %v", err)
		ErrCheck(c, returnData, err, "获取风机报警详细信息失败")
		return
	}

	ErrNil(c, returnData, res, "报告获取成功")
	return
}

// 首先查询风场、风机数量
func getWindfarmAndMachineCount(res *mod.DocumentStruct) error {

	return db.Table("windfarm").
		Joins("LEFT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
		Where("windfarm.id = ?", res.WindfarmId).
		Select("windfarm.name AS projectName, COUNT(machine.id) AS machineCounts").
		Find(res).Error
}

// 获取机组信息
func getMachineTypes(res *mod.DocumentStruct) error {
	var types []int
	typeMap := map[int]string{
		1: "直驱",
		2: "双馈",
	}
	if err := db.Table("windfarm").
		Joins("LEFT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
		Where("windfarm.id = ?", res.WindfarmId).
		Select("machine.`type`").
		Group("machine.`type`").
		Find(&types).Error; err != nil {
		mainlog.Error("获取风机类型失败 %v", err)
		return err
	}

	for _, v := range types {
		res.MachineType = append(res.MachineType, typeMap[v])
		res.Parameters = append(res.Parameters, mod.DocumentParameter{Type: v, TypeName: typeMap[v]})
	}

	return nil
}

// 获取风机部件信息 主轴承、齿轮箱、发电机、叶片
func getMachineComponentsInfo(res *mod.DocumentStruct) (err error) {
	for index, parameter := range res.Parameters {
		// 获取风机部件信息 主轴承、齿轮箱、发电机、叶片

		err = db.Table("windfarm").
			Joins("LEFT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
			Where("windfarm.id = ?", res.WindfarmId).
			Where("machine.`type` = ?", parameter.Type).Select("COUNT(machine.id)").
			Find(&res.Parameters[index].Count).Error

		err = db.Table("windfarm").
			Joins("LEFT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
			Where("windfarm.id = ?", res.WindfarmId).
			Where("machine.`type` = ?", parameter.Type).Select("DISTINCT machine.mbrtype").Where("machine.mbrtype != ''").
			Find(&res.Parameters[index].Mbrtype).Error
		err = db.Table("windfarm").
			Joins("LEFT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
			Where("windfarm.id = ?", res.WindfarmId).
			Where("machine.`type` = ?", parameter.Type).Select("DISTINCT machine.gbxtype").Where("machine.gbxtype != ''").
			Find(&res.Parameters[index].Gbxtype).Error
		err = db.Table("windfarm").
			Joins("LEFT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
			Where("windfarm.id = ?", res.WindfarmId).
			Where("machine.`type` = ?", parameter.Type).Select("DISTINCT machine.gentype").Where("machine.gentype != ''").
			Find(&res.Parameters[index].Gentype).Error
		err = db.Table("windfarm").
			Joins("LEFT JOIN machine ON machine.windfarm_uuid = windfarm.uuid").
			Where("windfarm.id = ?", res.WindfarmId).
			Where("machine.`type` = ?", parameter.Type).Select("DISTINCT machine.bladetype").Where("machine.bladetype != ''").
			Find(&res.Parameters[index].Bladetype).Error
	}
	return
}

// 获取风机、部件、测点、报警详细信息
func getMachineComponentsDetails(res *mod.DocumentStruct) (err error) {
	//查询风场下所有风机下部件，测点
	if err = db.Model(&mod.Machine{}).
		Joins("LEFT JOIN windfarm ON machine.windfarm_uuid = windfarm.uuid").
		Select("machine.id id, machine.name machineNum,machine.uuid machineUUID").
		Where("windfarm.id = ?", res.WindfarmId).Preload("Parts").Preload("Parts.Points").
		Find(&res.Machines).Error; err != nil {
		return
	}

	// 查询风机下测点的报警信息
	for _, machine := range res.Machines {
		for _, part := range machine.Parts {
			for k, point := range part.Points {
				// 根据风场测点查询最新的一条报警信息
				if err = db.Table("alert").Where("point_uuid = ? AND time_set BETWEEN ? AND ?", point.UUID, res.StartTimeSet, res.EndTimeSet).Order("id DESC").Limit(1).Find(&part.Points[k].Alert).Error; err != nil {
					return
				}
				// 部件正常时，查询时间范围内的有效值数据，
				if part.Points[k].Alert.Level != 2 && part.Points[k].Alert.Level != 3 {

					var partType string
					if err = db.Table("point").Select("part.type").Joins("left join part on part.uuid = point.part_uuid").Where("point.uuid = ?", point.UUID).Find(&partType).Error; err != nil {
						return err
					}

					// 1、找到测点相关信息，查询趋势图
					db.Table("point").Select("id point_id, name point_name").Where("uuid = ?", point.UUID).Scan(&part.Points[k].TrendChart.Currentplot)
					if err = part.Points[k].TrendChart.FanStaticPlot2(db, "rmsvalue", machine.Id, res.StartTimeSet, res.EndTimeSet); err != nil {
						return
					}

					// 2. 没有报警则查询时间范围内最新一条数据, 对应的原始数据、频域图
					if err = part.Points[k].TimeFrequencyPlot.GetCommonDataPlot(db, &part.Points[k], res.StartTimeSet, res.EndTimeSet); err != nil {
						return
					}
					// 3、无报警，根据部件类型、测点名、返回正常情况的描述和处理建议
					part.Points[k].Alert.Desc, part.Points[k].Alert.Suggest = GetDescAndSuggestByLevel(int(part.Points[k].Alert.Level), partType, "", part.Points[k].PointName)
				} else {
					// 存在报警
					var partType string
					if err = db.Table("point").Select("part.type").Joins("left join part on part.uuid = point.part_uuid").Where("point.uuid = ?", point.UUID).Find(&partType).Error; err != nil {
						return err
					}
					// 1、查询趋势图
					db.Table("point").Select("id point_id, name point_name").Where("uuid = ?", point.UUID).Scan(&part.Points[k].TrendChart.Currentplot)
					if err = part.Points[k].TrendChart.FanStaticPlot2(db, "rmsvalue", machine.Id, res.StartTimeSet, res.EndTimeSet); err != nil {
						return
					}
					// 2、查询报警对应的数据记录，查找对应的原始数据、频域图
					if err = part.Points[k].TimeFrequencyPlot.GetCommonDataPlot(db, &part.Points[k], res.StartTimeSet, res.EndTimeSet); err != nil {
						return
					}
					// 报警条目直接读取，报警的建议、故障说明
					//part.Points[k].Alert.Desc, part.Points[k].Alert.Suggest = GetDescAndSuggestByLevel(int(part.Points[k].Alert.Level), partType, part.Points[k].Alert.Type, part.Points[k].PointName)
				}

			}
		}
	}

	return
}
