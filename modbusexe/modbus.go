package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"main/mbserver"
	"main/mod"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/goburrow/modbus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"gorm.io/gorm"
)

//全局变量
var (
	config   = flag.String("dbconfig", "./GormConfig.toml", "数据库连接配置")
	db       *gorm.DB
	dbconfig *mod.GormConfig
	fi       *mbserver.FanIPID   = &mbserver.FanIPID{}
	ad       *mbserver.FanConfig = &mbserver.FanConfig{}
	mlog     *mod.Log            = &mod.Log{}
	serv     *mbserver.Server
	servset  *mbserver.ServerSet = &mbserver.ServerSet{IP: "127.0.0.1", Port: "502"}
	active   bool
	port     = flag.String("port", "3001", "服务占用端口")
)

//初始化连接数据库
func init() {
	var err error
	//初始map
	fi.IPIDs = make(map[string]mbserver.FanSet)
	fi.ErrIPIDs = make(map[string]mbserver.FanSet)
	if err = mlog.Loginit("./log/modbuslog", "0 0 0 1 1/1 ?"); err != nil {
		mlog.Error("日志初始化错误。%v", err)
	}
	flag.Parse()
	_, err = toml.DecodeFile(*config, &dbconfig)
	if err != nil {
		mlog.Error("数据库配置文件读取错误。%v", err)
	}
	db, err = dbconfig.GormOpen()
	if err != nil {
		mlog.Error("数据库配置错误。%v", err)
		os.Exit(-1)
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			mlog.Error("Panic!%v", r)
		}
	}()
	/*自测
	var src io.Reader
	src, _ = os.Open("./Fanset.xlsx")
	fi.StructRead(src, db)
	src, _ = os.Open("./Addrset.xlsx")
	ad.StructRead(src, db)
	ModbusTCPServer("0.0.0.0:502")
	go ModbusTCPClient("127.0.0.1:502")
	select {}
	*/
	Start()

}
func Start() {
	mlog.Info("ModbusTCP通讯模块开始运行")
	e := echo.New()

	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"*", "Content-Type"},
	}))
	e.Use(middleware.Recover())
	e.POST("api/v1/send/set/:type", FileRead)
	e.POST("api/v1/send/set/fan/insert", PostFan)
	e.GET("api/v1/send", GetServerSet)
	e.POST("api/v1/send/start", ModbusTCPSend)
	e.POST("api/v1/send/end", ModbusTCPEnd)
	e.DELETE("api/v1/send/set/fan", DeleteFan)
	e.Start(":" + *port)
}
func GetServerSet(c echo.Context) error {
	returnData := mod.ReturnData{}
	var jsonipid mbserver.JsonFanIPID = mbserver.JsonFanIPID{
		IPIDs:    []mbserver.FanSet{},
		ErrIPIDs: []mbserver.FanSet{},
	}

	for _, v := range fi.IPs {
		jsonipid.IPIDs = append(jsonipid.IPIDs, fi.IPIDs[v])
	}
	for _, v := range fi.ErrIPs {
		jsonipid.ErrIPIDs = append(jsonipid.ErrIPIDs, fi.ErrIPIDs[v])
	}
	servset.Fan = jsonipid
	servset.Active = active
	ErrNil(c, returnData, servset, "当前配置信息查找成功")
	return nil
}
func PostFan(c echo.Context) error {
	var err error
	var msg mbserver.FanSet
	returnData := mod.ReturnData{}
	err = json.NewDecoder(c.Request().Body).Decode(&msg)
	if err != nil || msg.IP == "" || msg.FactoryName == "" || msg.WindfarmName == "" || msg.MachineName == "" {
		ErrCheck(c, returnData, err, "请求错误")
		return err
	}
	db.Table("factory").Where("id=?", msg.FactoryName).Pluck("name", &msg.FactoryName)
	db.Table("windfarm").Where("id=?", msg.WindfarmName).Pluck("name", &msg.WindfarmName)
	msg.MachineID = msg.MachineName
	db.Table("machine").Where("id=?", msg.MachineName).Pluck("name", &msg.MachineName)

	if _, ok := fi.IPIDs[msg.IP]; !ok {
		fi.IPs = append(fi.IPs, msg.IP)
	}
	fi.IPIDs[msg.IP] = msg

	var jsonipid mbserver.JsonFanIPID
	for _, v := range fi.IPs {
		jsonipid.IPIDs = append(jsonipid.IPIDs, fi.IPIDs[v])
	}
	for _, v := range fi.ErrIPs {
		jsonipid.ErrIPIDs = append(jsonipid.ErrIPIDs, fi.ErrIPIDs[v])
	}
	if err != nil {
		ErrCheck(c, returnData, err, "风机插入失败")
		return err
	}

	ErrNil(c, returnData, jsonipid, "风机插入成功")
	return nil
}
func FileRead(c echo.Context) error {
	var err error
	var msg interface{}
	returnData := mod.ReturnData{}

	i := c.Param("type")
	var file *multipart.FileHeader
	file, err = c.FormFile("modbus_set")
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
	switch i {
	case "fan":
		err = fi.StructRead(src, db)
		var jsonipid mbserver.JsonFanIPID = mbserver.JsonFanIPID{
			IPIDs:    []mbserver.FanSet{},
			ErrIPIDs: []mbserver.FanSet{},
		}
		for _, v := range fi.IPs {
			jsonipid.IPIDs = append(jsonipid.IPIDs, fi.IPIDs[v])
		}
		for _, v := range fi.ErrIPs {
			jsonipid.ErrIPIDs = append(jsonipid.ErrIPIDs, fi.ErrIPIDs[v])
		}
		msg = jsonipid
	case "addr":
		err = ad.StructRead(src, db)
		msg = nil
	}
	if err != nil {
		ErrCheck(c, returnData, err, "文件导入失败")
		return err
	}
	ErrNil(c, returnData, msg, "文件导入成功")
	return nil
}
func DeleteFan(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}

	var dip []string
	c.Bind(&dip)

	for _, v := range dip {
		for k := range fi.IPs {
			if fi.IPs[k] == v {
				fi.IPs[k] = "delete"
				delete(fi.IPIDs, v)
			}
		}
		for k := range fi.ErrIPs {
			if fi.ErrIPs[k] == v {
				fi.ErrIPs[k] = "delete"
				delete(fi.ErrIPIDs, v)
			}
		}

	}
	var temperr []string
	for _, v := range fi.ErrIPs {
		if v != "delete" {
			temperr = append(temperr, v)
		}
	}
	fi.ErrIPs = temperr

	var temp []string

	for _, v := range fi.IPs {
		if v != "delete" {
			temp = append(temp, v)
		}
	}
	fi.IPs = temp

	var jsonipid mbserver.JsonFanIPID
	for _, v := range fi.IPs {
		jsonipid.IPIDs = append(jsonipid.IPIDs, fi.IPIDs[v])
	}
	for _, v := range fi.ErrIPs {
		jsonipid.ErrIPIDs = append(jsonipid.ErrIPIDs, fi.ErrIPIDs[v])
	}
	if err != nil {
		ErrCheck(c, returnData, err, "风机删除失败")
		return err
	}
	ErrNil(c, returnData, jsonipid, "风机删除成功")
	return nil
}

func ModbusTCPSend(c echo.Context) error {
	var err error
	returnData := mod.ReturnData{}
	var mm map[string]string
	if err = json.NewDecoder(c.Request().Body).Decode(&mm); err != nil {
		return err
	}
	var ipport string
	if _, ok := mm["ip"]; ok {
		if _, ok := mm["port"]; ok {
			ipport = mm["ip"] + ":" + mm["port"]
			servset.IP = mm["ip"]
			servset.Port = mm["port"]
		} else {
			err = errors.New("wrong ip and port")
		}
	} else {
		err = errors.New("wrong ip and port")
	}
	if err != nil {
		ErrCheck(c, returnData, err, "ipport格式错误")
		return err
	}
	ModbusTCPServer(ipport)
	if err != nil {
		ErrCheck(c, returnData, err, "ModbusTCP服务启动失败")
		return err
	}
	ErrNil(c, returnData, nil, "ModbusTCP服务启动成功")
	active = true
	return nil
}
func ModbusTCPEnd(c echo.Context) error {
	returnData := mod.ReturnData{}
	if serv != nil {
		serv.Close()
	}
	ErrNil(c, returnData, nil, "ModbusTCP终止成功")
	active = false
	return nil
}

// 服务器server启动
func ModbusTCPServer(ipport string) error {
	serv = mbserver.NewServer()
	serv.RegisterFunctionHandler(4,
		func(s *mbserver.Server, frame mbserver.Framer) ([]byte, *mbserver.Exception) {
			addr := frame.GetRemoteAddr()
			// fmt.Printf("server收到地址%v发送请求\n", addr)
			fdata := frame.GetData()
			register := int(binary.BigEndian.Uint16(fdata[0:2]))
			numRegs := int(binary.BigEndian.Uint16(fdata[2:4]))
			endRegister := register + numRegs
			if endRegister > 65535 {
				return []byte{}, &mbserver.IllegalDataAddress
			}
			dataSize := numRegs * 2

			data := make([]byte, 1+dataSize)
			data[0] = byte(dataSize)
			var result []byte
			if id, ok := mbserver.DataIDFind(fi, addr); ok {
				result = mbserver.DataFind(db, ad, id, register, numRegs, s)
			}
			copy(data[1:], result)
			// fmt.Printf("server向地址%v发送测点状态响应数据(uint16)%v\n", addr, result)
			return data, &mbserver.Success
		})

	err := serv.ListenTCP(ipport)
	if err != nil {
		return err
	}
	return nil
}

// 模拟client
func ModbusTCPClient(ipport string) {
	register := 0
	numRegs := 6

	fmt.Printf("client请求从%v开始的%v个地址状态数据\n", register, numRegs)
	handler := modbus.NewTCPClientHandler(ipport)
	handler.Connect()
	defer handler.Close()
	client := modbus.NewClient(handler)
	results, err := client.ReadHoldingRegisters(uint16(register), uint16(numRegs))

	if err != nil {
		fmt.Println(err)
	}
	resultuint := mbserver.BytesToUint16(results)
	fmt.Println("client收到返回状态值并解析：", resultuint)
}

func ErrCheck(c echo.Context, returnData mod.ReturnData, err error, info string) {
	returnData.Code = http.StatusBadRequest
	returnData.Message = info + "。err:" + err.Error()
	returnData.Data = nil
	mlog.Error(returnData.Message)
	c.JSON(200, returnData)
}

func ErrNil(c echo.Context, returnData mod.ReturnData, d interface{}, info string) {
	returnData.Code = http.StatusOK
	returnData.Data = d
	returnData.Message = info
	c.JSON(200, returnData)
}
