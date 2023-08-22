package main

// 用于导入离线数据文本的小程序
import (
	"encoding/json"
	"flag"
	"fmt"
	"main/mod"
	"net/http"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/fsnotify/fsnotify"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

var (
	dbconfig   mod.GormConfig
	db         *gorm.DB
	dataport            = flag.String("dataport", "3006", "数据分析端口。默认3006")
	dataip              = flag.String("dataip", "localhost", "数据分析ip。默认本地")
	config              = flag.String("dbconfig", "./GormConfig.toml", "数据库连接配置")
	datafolder          = flag.String("f", "", "监视文件夹")
	ilog       *mod.Log = &mod.Log{} //记录错误日志
	dlog       *mod.Log = &mod.Log{} //记录数据日志
	watcher    *fsnotify.Watcher
	port                  = flag.String("port", "3002", "服务占用端口")
	dws        *DataWatch = &DataWatch{ControlChan: make(chan struct{})}
)

type DataWatch struct {
	Folder      string        `json:"path"`
	Actice      bool          `json:"active"`
	ControlChan chan struct{} `json:"-"`
}

func init() {
	var err error
	if err = ilog.Loginit("./log/watchlog", "0 0 0 1 1/1 ?"); err != nil {
		ilog.Error("错误日志初始化错误")
	}
	if err = dlog.LoginitDaily("./log/datalog", "0 0 0 1/1 * *"); err != nil {
		ilog.Error("数据日志初始化错误")
	}
	flag.Parse()
	_, err = toml.DecodeFile(*config, &dbconfig)
	if err != nil {
		ilog.Error("数据库配置文件读取错误")
	}
	db, err = dbconfig.GormOpen()
	if err != nil {
		ilog.Error("数据库配置错误")
		os.Exit(-1)
	}
	_, err = os.Stat(*datafolder)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(*datafolder, os.ModePerm); err != nil {
			fmt.Println("默认监视路径错误。")
			return
		}
	}
	dws.Folder = strings.Replace(*datafolder, "\\", "/", -1)
	if dws.Folder != "" {
		ilog.Info("监听%v开始", dws.Folder)
		FiletoInsertLoop = make(chan string, 1000)
		go func(f string) {
			WatchFile(f)
		}(dws.Folder)
		dws.Actice = true
	}
}
func main() {
	defer func() {
		if r := recover(); r != nil {
			ilog.Error("Panic!%v", r)
		}
	}()
	defer close(FiletoInsertLoop)
	Start()
}
func Start() {
	ilog.Info("数据自动导入模块开始运行")
	e := echo.New()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"*", "Content-Type"},
	}))
	e.Use(middleware.Recover())
	e.GET("api/v1/watch", WatchInfo)
	e.POST("api/v1/watch/start", WatchStart)
	e.POST("api/v1/watch/end", WatchEnd)
	e.Start(":" + *port)
}

func WatchInfo(c echo.Context) error {
	returnData := mod.ReturnData{}
	ErrNil(c, returnData, dws, "")
	return nil
}
func WatchEnd(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	if err != nil {
		ErrCheck(c, returnData, err, "自动数据导入关闭失败")
		return err
	}
	_, err = os.Stat(dws.Folder)
	if err != nil {
		ErrCheck(c, returnData, err, "监测文件夹不存在")
		return err
	}
	dws.ControlChan <- struct{}{}
	close(FiletoInsertLoop)
	//关闭子进程
	ilog.Info("监听%s 终止", dws.Folder)
	dws.Actice = false
	dws.Folder = ""
	ErrNil(c, returnData, nil, "自动数据导入关闭成功")
	return nil
}

func WatchStart(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	mm := make(map[string]string)
	if err = json.NewDecoder(c.Request().Body).Decode(&mm); err != nil {
		ErrCheck(c, returnData, err, "请求body解析失败")
		return err
	}
	dws.Folder = strings.Replace(mm["folder"], "\\", "/", -1)
	_, err = os.Stat(dws.Folder)
	if err != nil {
		ErrCheck(c, returnData, err, "监测文件夹不存在")
		return err
	}
	ilog.Info("监听%v开始", dws.Folder)
	FiletoInsertLoop = make(chan string, 1000)
	go func(f string) {
		WatchFile(f)
	}(dws.Folder)
	dws.Actice = true
	if err != nil {
		ErrCheck(c, returnData, err, "自动数据导入启动失败")
		return err
	}
	ErrNil(c, returnData, nil, "自动数据导入启动成功")
	return nil
}

var FiletoInsertMap map[string]int = make(map[string]int)

//通道控制
var FiletoInsertLoop chan string = make(chan string, 1000)

func ErrCheck(c echo.Context, returnData mod.ReturnData, err error, info string) {
	returnData.Code = http.StatusBadRequest
	returnData.Message = info + "。err:" + err.Error()
	returnData.Data = nil
	ilog.Error(returnData.Message)
	c.JSON(200, returnData)
}

func ErrNil(c echo.Context, returnData mod.ReturnData, d interface{}, info string) {
	returnData.Code = http.StatusOK
	returnData.Data = d
	returnData.Message = info
	c.JSON(200, returnData)
}

func DataOpt(datafile string) {
	var err error
	if _, err = os.Stat(datafile); err != nil {
		if os.IsNotExist(err) {
			dlog.Error("不存在数据文件%s。%s", datafile, err.Error())
			return
		}
		dlog.Error("导入失败。%s 保留数据%s", err.Error(), datafile)
	} else {
		err = InsertPointData(*dataip+":"+*dataport, datafile)
		if err != nil {
			dlog.Error("导入失败。%s 保留数据%s", err.Error(), datafile)
		} else {
			//删除源文件
			if err := os.Remove(datafile); err != nil {
				dlog.Error("删除数据失败。%s", err.Error())
			} else {
				dlog.Info("导入后删除数据 %s", datafile)
			}
		}
	}
}
