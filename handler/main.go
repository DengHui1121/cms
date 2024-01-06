package main

//cms 主要后台程序
import (
	"flag"
	"fmt"
	"main/mod"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"gorm.io/gorm"
)

var (
	config  = flag.String("dbconfig", "./GormConfig.toml", "数据库连接配置")
	port    = flag.String("p", "3000", "服务端口")
	dataurl = flag.String("dataurl", "localhost:3005", "数据服务占用端口")
	DB      = db
)

var db *gorm.DB
var dbconfig *mod.GormConfig
var mainlog *mod.Log = &mod.Log{}

func init() {
	var err error
	_, err = os.Stat("./log")
	if os.IsNotExist(err) {
		if err = os.Mkdir("./log", os.ModePerm); err != nil {
			fmt.Println("路径错误。")
			return
		}
	}
	if err = mainlog.Loginit("./log/mainlog", "0 0 0 1 1/1 ?"); err != nil {
		mainlog.Error("日志初始化错误。%v", err)
	}

	flag.Parse()
	//检查数据库
	_, err = toml.DecodeFile(*config, &dbconfig)
	if err != nil {
		mainlog.Error("数据库配置文件读取错误。%v", err)
	}
	err = mod.CreateSchema(dbconfig)
	if err != nil {
		mainlog.Error("数据库配置文件读取错误。%v", err)
	}
	db, err = dbconfig.GormOpen()
	if err != nil {
		mainlog.Error("数据库配置错误。%v", err)
		os.Exit(-1)
	}

	//是否迁移表格
	err = mod.TableCheck(db)
	if err != nil {
		mainlog.Error("数据库更新失败。%v", err)
		os.Exit(-1)
	}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			mainlog.Error("Panic!%v", r)
		}
	}()
	Start()
}

// 用于报警分析的antspool
func Start() {
	// Infof("CMS服务器开始运行")
	e := echo.New()
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"Content-Type", "*"},
	}))
	// e.Use(middleware.Recover())
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// 获取请求的 URL 路径
			path := c.Request().URL.Path
			// 如果请求的路径是以 /index 开头的，则重定向
			if strings.HasPrefix(path, "/index") {
				return c.File("./dist/index.html")
			}
			if strings.HasPrefix(path, "/login") {
				return c.File("./dist/index.html")
			}
			return next(c)
		}
	})

	//静态资源目录，包含前端代码
	e.Static("/dist", "./dist/")
	e.Static("/js", "./dist/js/")
	e.Static("/css", "./dist/css/")
	e.Static("/img", "./dist/img/")
	e.Static("/media", "./dist/media/")
	e.Static("/fanModel", "./dist/fanModel/")
	e.Static("/favicon.ico", "./dist/favicon.ico")
	e.Static("/ipConfig.js", "./dist/ipConfig.js")
	e.Static("/upload", "./upload/")

	e.POST("/ais/openapi/v1/:farmid/states/:turbineId", FactoryDataUpdateHandler(*dataurl)) //厂家检测数据上传接口

	cmsbasic := e.Group("api/v1/")
	{
		cmsbasic.POST("login", Login)             //登录
		cmsbasic.PUT(":type", UpdateInfo)         //更新信息 ,末尾?id=xx
		cmsbasic.PUT(":type/alert", UpdateStatus) //更新风机和测点的状态
		cmsbasic.PUT("alerts", UpdateAlert)       //更新信息 ,末尾?id=xx

		cmsbasic.GET("structure", FindAll)       //设备树获取完整信息
		cmsbasic.GET("std", FindStd)             //获得标准信息
		cmsbasic.GET(":type/", FindInfo)         //查找某一级下一级的所有内容
		cmsbasic.GET(":type", FindTree)          //按id查询单个和下级所有信息
		cmsbasic.GET(":upper/:type", FindTree)   //按id查询指定upper的下级type所有信息
		cmsbasic.GET("alerts/:type", FindAlert)  //按id查询指定alert报警相关信息
		cmsbasic.GET("alert/ws", AlertBroadcast) //ws连接通知报警信息

		cmsbasic.GET("data", DataPlot)                          //绘图 末尾?id=xx&point_id=xx
		cmsbasic.GET("data/multiChart", MultiDataPlot)          //对比图 末尾?characteristic=xx
		cmsbasic.GET("data/:type", AnalyseDataPlot_2(*dataurl)) //算法分析图
		cmsbasic.GET("data/analysisoption", AnalyseDataFunc)    //算法分析图

		cmsbasic.POST("fan/parts", FileUpload)                     //导入部件 //目前已不使用。直接使用标准风机创建。
		cmsbasic.POST(":type", InsertInfo)                         //新建 公司 风场 风机，风机批量新建。
		cmsbasic.POST("alert", InsertAlert)                        //新建人工报警信息。
		cmsbasic.POST("data/over/:id", OverMPointData(*dataurl))   //覆盖相同数据
		cmsbasic.POST("data/check/:id", CheckMPointData(*dataurl)) //导入数据文件并分析。
		cmsbasic.POST("std/:type/:option", StdFileUpload)          //导入标准文件 form_data
		cmsbasic.POST("std/fan/info", StdUpdate)                   //更新标准文件 form_data
		cmsbasic.POST("alert/confirm", AlertConfirm)               //确认报警信息
		cmsbasic.POST("limit", PostDataLimit)                      //导入部件 form_data key:fan_parts
		cmsbasic.DELETE(":type", DeleteInfo)                       //删除设备相关信息 末尾?id=xx
		cmsbasic.DELETE("std", DeleteStd)                          //删除标准文件

		//新增算法管理模块
		cmsbasic.POST("algorithm", AddAlgorithmHandler)                     //新建算法
		cmsbasic.DELETE("algorithm/:id", DeleteAlgorithmHandler)            //删除算法
		cmsbasic.PUT("algorithm", UpdateAlgorithmHandler)                   //更新算法
		cmsbasic.GET("algorithm", GetAlgorithmListHandler)                  //获取算法
		cmsbasic.GET("algorithm/point", GetAlgorithmListByPointUUIDHandler) //根据测点uuid获取可以使用的算法

		cmsbasic.GET("parsing", GetParsingHandler)           //获取解析方式
		cmsbasic.POST("parsing", AddParsingHandler)          //新增解析方式
		cmsbasic.PUT("parsing", UpdateParsingHandler)        //更新解析方式
		cmsbasic.DELETE("parsing/:id", DeleteParsingHandler) //删除解析方式

		cmsbasic.GET("faultTag", GetFaultTagHandler)           //获取故障标签
		cmsbasic.PUT("faultTag", UpdateFaultTagHandler)        //更新故障标签
		cmsbasic.POST("faultTag", AddFaultTagHandler)          //新增故障标签
		cmsbasic.DELETE("faultTag/:id", DeleteFaultTagHandler) //删除故障标签

		cmsbasic.POST("upload", UploadFile)              //文件上传
		cmsbasic.GET("upload", GetAllFileHandler)        //文件列表
		cmsbasic.DELETE("upload/:id", DeleteFileHandler) //删除文件

		cmsbasic.GET("models", GetModelsHandler)

		//数据标签修改
		cmsbasic.PUT("data/tag/:machineId", UpdateDataLabel)
	}

	//* 导入导出相关
	outputhandle := e.Group("api/v1/output/")
	{
		outputhandle.POST("xlsx", OutputXlsx)        //导出xlsx文件
		outputhandle.POST("doc", OutputDocx)         //导出docx文件
		outputhandle.GET("dl", DownloadOutput)       //上传文件至前端下载
		outputhandle.GET("document", OutputDocument) //导出word文档
	}

	//*数据库相关
	dbhandle := e.Group("api/v1/db/")
	{
		dbhandle.POST("output", OutputDB) //导出db；
		dbhandle.POST("input", InputDB)   //导入db；
	}

	//* 运行统计
	stastics := e.Group("api/v1/operation/statistics/")
	{
		stastics.GET("fault/counts", GetFaultCounts)                        //故障数统计
		stastics.GET("part/waveform", GetFanDataCurrentPlot)                //风机统计趋势图
		stastics.GET("part/waveformA/:id", GetFanDataCurrentAlgorithmPlotA) //风机测点算法趋势图
		stastics.GET("part/waveformB/:id", GetFanDataCurrentAlgorithmPlotB) //风机测点算法趋势图
		stastics.GET("content", GetStatisticsContent)                       //获取基本信息
		stastics.GET("status", GetStatisticsStatus)                         //获取运行统计状态
		stastics.GET("trend", GetTrend)                                     //获取月度故障等级趋势
		stastics.GET("month/fault/trend", GetPartTrend)                     //获取不同部件的月度故障等级趋势
		stastics.GET("fault/level", GetFaultLevel)                          //获取不同部件故障等级数量
		stastics.GET("part/fault", GetPartFault)                            //获取部件类型故障图
		stastics.GET("fault/logs", GetFaultLogs)                            //获取故障日志

		//----------新增----------//
		stastics.GET(":type/warningAlgorithm", GetAlgorithmHandler)                      //获取风机或风场预警算法统计
		stastics.GET("windfarm/faultFeedBack/:id", GetFarmFaultFeedBackHandler)          //获取风场故障反馈
		stastics.POST("windfarm/faultFeedBack", AddFaultFeedbackHandler)                 //新增故障反馈
		stastics.DELETE("windfarm/faultFeedBack/:id", DeleteFaultFeedbackHandler)        //删除故障反馈
		stastics.PUT("windfarm/faultFeedBack", UpdateFaultFeedbackHandler)               //更新故障反馈
		stastics.GET("windfarm/faultFeedBack/info/:id", GetFarmFaultFeedBackByIdHandler) //根据id获取风场故障反馈
		//----------结束----------//

	}

	//* 用户相关
	user := e.Group("api/v1/user/")
	{
		user.POST(":type", UserOption)   //增加
		user.GET(":type", UserOption)    //查询列表
		user.PUT(":type", UserOption)    //修改
		user.DELETE(":type", UserOption) //删除
	}
	mainlog.Info("服务启动成功，端口：" + *port)
	//端口
	e.Start(":" + *port)
}
