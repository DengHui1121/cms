// ^ 数据库连接、配置相关的函数和结构体
package mod

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"main/alert"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var dbconfig *GormConfig

type GormConfig struct {
	Admin    string
	Password string
	Net      string
	Addr     string
	Port     string
	Schema   string
}

func (info *GormConfig) GormOpen() (db *gorm.DB, err error) {
	dbconfig = info
	dsn := fmt.Sprintf("%v:%v@%v(%v:%v)/%v?charset=utf8&parseTime=True&loc=Local",
		info.Admin, info.Password, info.Net, info.Addr, info.Port, info.Schema)
	newlogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			LogLevel: logger.Info,
			Colorful: true,
		},
	)
	db, err = gorm.Open(mysql.New(mysql.Config{
		DSN:               dsn, // DSN data source name
		DefaultStringSize: 256, // string 类型字段的默认长度
	}), &gorm.Config{
		Logger:                 newlogger,
		SkipDefaultTransaction: false,
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// SetMaxIdleConns 设置空闲连接池中连接的最大数量
	sqlDB.SetMaxIdleConns(50)
	// SetMaxOpenConns 设置打开数据库连接的最大数量。
	sqlDB.SetMaxOpenConns(100)
	// SetConnMaxLifetime 设置了连接可复用的最大时间。
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

var modelsList = []Models{
	{Name: "有效值模型", NameEn: "rmsvalue"},
	{Name: "峭度指标模型", NameEn: "indexkur"},
	{Name: "脉冲模型", NameEn: "indexi"},
	{Name: "波形模型", NameEn: "indexk"},
	{Name: "裕度指标模型", NameEn: "indexl"},
	{Name: "峰值指标模型", NameEn: "indexc"},
	{Name: "方根赋值模型", NameEn: "indexxr"},
	{Name: "最大值模型", NameEn: "indexmax"},
	{Name: "最小值模型", NameEn: "indexmin"},
	{Name: "均值模型", NameEn: "indexmean"},
	{Name: "平均赋值模型", NameEn: "indexeven"},
}
var faultTags = []FaultTagFirst{
	{Type: "主轴承", Name: "自动报警"},
	{Type: "发电机", Name: "自动报警"},
	{Type: "齿轮箱", Name: "自动报警"},
	{Type: "叶片", Name: "自动报警"},
	{Type: "塔筒", Name: "自动报警"},
	{Type: "机舱", Name: "自动报警"},
	{
		Type: "主轴承",
		Name: "主轴轴承故障",
		Childrens: []FaultTagSecond{
			{Name: "主轴轴承内圈故障", Source: false},
			{Name: "主轴轴承外圈故障", Source: false},
			{Name: "主轴轴承保持架故障", Source: false},
			{Name: "主轴轴承滚动体故障", Source: false},
		},
	},
	{
		Type: "发电机",
		Name: "发电机前轴承故障",
		Childrens: []FaultTagSecond{
			{Name: "发电机前轴承内圈故障", Source: false},
			{Name: "发电机前轴承外圈故障", Source: false},
			{Name: "发电机前轴承保持架故障", Source: false},
			{Name: "发电机前轴承滚动体故障", Source: false},
		},
	},
	{
		Type: "发电机",
		Name: "发电机后轴承故障",
		Childrens: []FaultTagSecond{
			{Name: "发电机后轴承内圈故障", Source: false},
			{Name: "发电机后轴承外圈故障", Source: false},
			{Name: "发电机后轴承保持架故障", Source: false},
			{Name: "发电机后轴承滚动体故障", Source: false},
		},
	},
	{
		Type: "齿轮箱",
		Name: "行星级故障",
		Childrens: []FaultTagSecond{
			{Name: "Ⅰ级太阳轮齿轮故障", Source: false},
			{Name: "Ⅱ级太阳轮齿轮故障", Source: false},
			{Name: "Ⅰ级行星轮齿轮故障", Source: false},
			{Name: "Ⅱ级行星轮齿轮故障", Source: false},
			{Name: "Ⅰ级行星轮转架前轴承故障", Source: false},
			{Name: "Ⅱ级行星轮转架前轴承故障", Source: false},
			{Name: "Ⅰ级行星轮转架后轴承故障", Source: false},
			{Name: "Ⅱ级行星轮转架后轴承故障", Source: false},
		},
	},
	{
		Type: "齿轮箱",
		Name: "低速级故障",
		Childrens: []FaultTagSecond{
			{Name: "低速级齿轮故障", Source: false},
			{Name: "低速级驱动端轴承故障", Source: false},
			{Name: "低速级非驱动端轴承故障", Source: false},
		},
	},
	{
		Type: "齿轮箱",
		Name: "中速级故障",
		Childrens: []FaultTagSecond{
			{Name: "中速级大齿轮故障", Source: false},
			{Name: "中速级小齿轮故障", Source: false},
			{Name: "中速级驱动端轴承故障", Source: false},
			{Name: "中速级非驱动端轴承故障", Source: false},
		},
	},
	{
		Type: "齿轮箱",
		Name: "高速级故障",
		Childrens: []FaultTagSecond{
			{Name: "高速级齿轮故障", Source: false},
			{Name: "高速级驱动端轴承故障", Source: false},
			{Name: "高速级非驱动端轴承故障", Source: false},
		},
	},
	{
		Type: "叶片",
		Name: "叶片断裂故障",
		Childrens: []FaultTagSecond{
			{Name: "叶片撞击断裂故障", Source: false},
			{Name: "叶片疲劳断裂故障", Source: false},
		},
	},
	{
		Type: "叶片",
		Name: "叶片异常振动故障",
		Childrens: []FaultTagSecond{
			{Name: "叶片涡轮震荡", Source: false},
			{Name: "叶片挥舞异常震荡", Source: false},
			{Name: "叶片摆振异常震荡", Source: false},
		},
	},
	{
		Type: "叶片",
		Name: "叶片损伤故障",
		Childrens: []FaultTagSecond{
			{Name: "叶片壳体损伤(叶片壳体鼓包分层、叶片粘接开裂、叶片雷击开裂)", Source: false},
			{Name: "叶片结构损伤(叶片主梁损伤、腹板裂纹)", Source: false},
		},
	},
	{
		Type: "叶片",
		Name: "叶片运行异常",
		Childrens: []FaultTagSecond{
			{Name: "叶片融冰、覆冰、气动不平衡、固有频率偏移", Source: false},
		},
	},
	{
		Type: "塔筒",
		Name: "塔筒晃度异常",
		Childrens: []FaultTagSecond{
			{Name: "塔筒垂直度故障", Source: false},
			{Name: "塔筒晃动量趋势量异常、塔筒轴心轨迹偏差超限", Source: false},
			{Name: "塔筒结构松动故障", Source: false},
		},
	},
	{
		Type: "塔筒",
		Name: "塔筒倾角异常",
		Childrens: []FaultTagSecond{
			{Name: "塔筒倾覆角度超限、塔筒垂直度出现偏移", Source: false},
		},
	},
	{
		Type: "塔筒",
		Name: "基础不均匀沉降故障",
		Childrens: []FaultTagSecond{
			{Name: "基础不均匀沉降故障", Source: false},
		},
	},
}

// ^ 根据结构体建立数据表
func TableCheck(db *gorm.DB) (err error) {
	//基本信息迁移
	if !db.Migrator().HasTable("user") {
		db.Migrator().CreateTable(&User{})
		db.Create(&User{Username: "xnadmin", Password: "xnadmin", Level: 1})
	} else {
		err = db.AutoMigrate(&User{})
		if err != nil {
			return err
		}
	}
	//基本表格迁移 数据表格
	err = db.AutoMigrate(&Factory{}, &Windfarm{}, &Machine{}, &Part{}, &Property{}, &Point{}, &Alert{}, &MachineStd{},
		&Algorithm{}, &AlgorithmResultA{}, &AlgorithmResultB{}, &Parsing{}, &FaultBack{}, &FaultTagFirst{}, &FaultTagSecond{}, &File{}, &Models{},
	)
	if err != nil {
		return err
	}
	// var f Factory = Factory{Name: "大唐四川发电有限公司新能源分公司"}
	// 检查模型是否存在，不存在则插入数据库
	for _, model := range modelsList {
		var existingModel Models
		result := db.Where(&Models{Name: model.Name}).FirstOrCreate(&existingModel, &model)
		if result.Error != nil {
			// 处理错误
		}
	}

	//// 检查故障标签是否存在，不存在则插入数据库
	// 检查并创建一级故障标签
	checkTag(faultTags, db)
	//for _, tag := range faultTags {
	//	var existingTag FaultTag
	//	result := db.Where(&FaultTag{Name: tag.Name}).FirstOrCreate(&existingTag, &tag)
	//	if result.Error != nil {
	//		// 处理错误
	//	}
	//}
	// db.Table("factory").Where("name=?", f.Name).FirstOrCreate(&f)

	//band列改名
	if db.Table("band").Migrator().HasColumn(&alert.Band{}, "range") {
		db.Table("band").Migrator().RenameColumn(&alert.Band{}, "range", "band_range")
	}
	// 报警表格迁移
	// 新增故障树详细信息存储
	err = db.AutoMigrate(&alert.Band{}, &alert.BandAlert{}, &alert.TreeAlert{}, &alert.ManualAlert{})
	if err != nil {
		return err
	}

	//检测风场下属相关数据表是否存在
	var fs []Machine
	err = db.Table("machine").Select("id", "uuid").Scan(&fs).Error
	if err != nil {
		return err
	}
	if len(fs) != 0 {
		for _, v := range fs {
			db.Table(fmt.Sprintf("data_%v", v.ID)).AutoMigrate(&Data_Update{})
			db.Table(fmt.Sprintf("wave_%v", v.ID)).AutoMigrate(&Wave_Update{})
			// 转速
			db.Table(fmt.Sprintf("data_rpm_%v", v.ID)).AutoMigrate(&Data_Update{})
			db.Table(fmt.Sprintf("wave_rpm_%v", v.ID)).AutoMigrate(&Wave_Update{})
		}
	}
	//迁移日月统计表
	err = db.AutoMigrate(&WindfarmMonthReport{}, &WindfarmDailyReport{}, &MachineMonthReport{}, &MachineDailyReport{})
	if err != nil {
		return err
	}
	if db.Migrator().HasTable("data") {
		db.Migrator().DropTable("data")
	}
	return nil
}

func checkTag(first []FaultTagFirst, db *gorm.DB) {
	for _, tag := range first {
		var existingTag FaultTagFirst
		result := db.Where(&FaultTagFirst{Name: tag.Name}).FirstOrCreate(&existingTag, &tag)
		if result.Error != nil {
			// 处理错误
		}

		// 创建或查找一级故障标签成功后，处理与其关联的二级故障标签
		for _, childTag := range tag.Childrens {
			var existingChildTag FaultTagSecond
			childResult := db.Where(&FaultTagSecond{Name: childTag.Name}).FirstOrCreate(&existingChildTag, &childTag)
			if childResult.Error != nil {
				// 处理错误
			}

			// 如果你的数据模型需要在创建一级标签后将二级标签与其关联，可以在这里进行处理
			existingTag.Childrens = append(existingTag.Childrens, existingChildTag)
		}
		// 更新一级故障标签以关联其二级故障标签
		db.Save(&existingTag)
	}
}

// 检查数据库是否存在，若不存在则新建。
func CreateSchema(dbconfig *GormConfig) (err error) {
	dsn := fmt.Sprintf("%v:%v@%v(%v:%v)/%v?charset=utf8&parseTime=True&loc=Local",
		dbconfig.Admin, dbconfig.Password, dbconfig.Net, dbconfig.Addr, dbconfig.Port, "information_schema")
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: dsn, // DSN data source name
	}))
	if err != nil {
		return err
	}
	var c int64
	db.Table("SCHEMATA").Where("SCHEMA_NAME = ?", dbconfig.Schema).Count(&c)
	if c != 0 {
		return
	} else {
		sqlstr := "create DATABASE " + dbconfig.Schema
		db.Exec(sqlstr)
		fmt.Println("未检测到配置文件中数据库。已新建数据库:", dbconfig.Schema)
		return
	}
}

// ^ 分页
func Paginate(r *http.Request) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))

		if page == 0 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
		switch {
		case pageSize > 100:
			pageSize = 100
		case pageSize <= 0:
			pageSize = 10
		}
		offset := (page - 1) * pageSize
		return db.Offset(offset).Limit(pageSize)
	}
}

type DBJob struct {
	Limit       Limit
	OutputFiles []*OutputFile `json:"output_file"`
}

// * 导出数据库
// 1.备份到当前库 _backup_表。此时会锁表。
// 2.筛选data_ wave_ data_rpm_ wave_rpm_ alert数据时间筛选
func TableBackUp(db *gorm.DB, limit Limit, sfilepath string) (sqlfilename string, err error) {
	stime, _ := StrtoTime("2006-01-02 15:04:05", limit.Starttime)
	etime, _ := StrtoTime("2006-01-02 15:04:05", limit.Endtime)
	//
	t, err := db.Migrator().GetTables()
	if err != nil {
		return
	}
	for _, v := range t {
		if strings.Contains(v, "_backup_") {
			db.Migrator().DropTable(v)
		}
	}
	t, err = db.Migrator().GetTables()
	if err != nil {
		return
	}
	// return nil
	var OriginDataTableStr []string
	var DataTableStr []string
	var WaveTableStr []string
	var MsgTableStr []string

	prefix := "_backup_" + time.Now().Format("20060102150405") + "_"
	//建表
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, v := range t {
			if strings.Contains(v, "data_") {
				var count int64
				tx.Table(v).Where("time_set BETWEEN ? AND ?", stime, etime).
					Limit(1).Count(&count)
				if count != 0 {
					vw := strings.Replace(v, "data", "wave", 1)
					//新建备份表
					err = tx.Exec(fmt.Sprintf("select * from %s for update;", v)).Error
					if err != nil {
						return err
					}
					err = tx.Exec(fmt.Sprintf("select * from %s for update;", vw)).Error
					if err != nil {
						return err
					}
					err = tx.Table(prefix + v).AutoMigrate(&Data_Update{})
					if err != nil {
						return err
					}
					err = tx.Table(prefix + vw).AutoMigrate(&Wave_Update{})
					if err != nil {
						return err
					}
					DataTableStr = append(DataTableStr, prefix+v)
					OriginDataTableStr = append(OriginDataTableStr, v)
					WaveTableStr = append(WaveTableStr, prefix+vw)
				}
			} else if strings.Contains(v, "wave_") {
				continue
			} else if strings.Contains(v, "alert") {
				err := tx.Exec(fmt.Sprintf("select * from %s for update;", v)).Error
				if err != nil {
					return err
				}
				if err = tx.Exec(fmt.Sprintf("CREATE TABLE %s LIKE %s;", prefix+v, v)).Error; err != nil {
					return err
				}
				MsgTableStr = append(MsgTableStr, prefix+v)
			} else {
				err := tx.Exec(fmt.Sprintf("select * from %s for update;", v)).Error
				if err != nil {
					return err
				}
				if err = tx.Exec(fmt.Sprintf("CREATE TABLE %s LIKE %s;", prefix+v, v)).Error; err != nil {
					return err
				}
				if err = tx.Exec(fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN id INT AUTO_INCREMENT;", prefix+v)).Error; err != nil {
					return err
				}
				if err = tx.Exec(fmt.Sprintf("INSERT INTO %s SELECT * FROM %s;", prefix+v, v)).Error; err != nil {
					return err
				}
				MsgTableStr = append(MsgTableStr, prefix+v)
			}
		}
		return nil
	})

	if err != nil {
		return
	}
	//data wave alert数据迁移
	db.Transaction(func(tx *gorm.DB) error {
		for _, v := range OriginDataTableStr {
			var ds []Data
			//find in batches
			err = tx.Table(v).Where("time_set BETWEEN ? AND ?", stime, etime).
				Preload("Wave", func(db *gorm.DB) *gorm.DB {
					return db.Table(strings.Replace(v, "data", "wave", 1))
				}).
				Preload("Alert").Preload("Alert.BandAlert").Preload("Alert.TreeAlert").Preload("Alert.ManualAlert").
				FindInBatches(&ds, 1000, func(tx *gorm.DB, batch int) error {
					for k := range ds {
						//数据
						err = tx.Table(prefix + v).Omit(clause.Associations).Create(&ds[k]).Error
						err = tx.Table(prefix + strings.Replace(v, "data", "wave", 1)).Create(&ds[k].Wave).Error
						if err != nil {
							return err
						}
						//报警
						err = tx.Table(prefix + "alert").Preload(clause.Associations).Create(&ds[k].Alert).Error
						if err != nil {
							return err
						}
					}
					return nil
				}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	//导出备份的表格到sql文件
	//删除备份表格
	defer func() {
		t, _ := db.Migrator().GetTables()
		for _, v := range t {
			if strings.Contains(v, prefix) {
				db.Migrator().DropTable(v)
			}
		}
	}()

	//操作
	//导出
	var createfile string
	if _, err := os.Stat("./output/temp/"); err != nil {
		if !os.IsExist(err) {
			if err = os.MkdirAll("./output/temp/", os.ModePerm); err != nil {
				return "", err
			}
		}
	}
	if runtime.GOOS == "windows" {
		createfile = "./output/temp/my.ini"
	}
	if runtime.GOOS == "linux" {
		createfile = "./output/temp/my.cnf"
	}
	inifile, err := os.Create(createfile)
	if err != nil {
		return
	}
	defer os.Remove(inifile.Name())
	inifilepath, _ := filepath.Abs(inifile.Name())
	inifilepath = filepath.ToSlash(inifilepath)
	inistr := fmt.Sprintf("[client]\nuser=%s\npassword=%s", dbconfig.Admin, dbconfig.Password)
	inifile.Write([]byte(inistr))
	inifile.Close()
	var opt []string

	opt = []string{"--defaults-extra-file=" + inifilepath, dbconfig.Schema}
	//目标表格
	opt = append(opt, MsgTableStr...)
	opt = append(opt, DataTableStr...)
	opt = append(opt, WaveTableStr...)
	defaultopt := []string{"--skip-comments", "--compact", "--hex-blob"}
	opt = append(opt, defaultopt...)

	cmd := exec.Command("mysqldump", opt...)

	// 设置接收
	if _, err := os.Stat(sfilepath); err != nil {
		if !os.IsExist(err) {
			if err = os.MkdirAll(sfilepath, os.ModePerm); err != nil {
				return "", err
			}
		}
	}
	sqlfilename, err = filepath.Abs(sfilepath)
	sqlfilename = filepath.ToSlash(sqlfilename) + "/mysql_" + time.Now().Format("20060102150405"+".txt")
	file, err := os.Create(sqlfilename)
	if err != nil {
		return
	}
	defer file.Close()
	// 将输出重定向到文件
	cmd.Stdout = file
	// 输出debug
	// var out bytes.Buffer
	// var outerr bytes.Buffer
	// cmd.Stdout = &out
	// cmd.Stderr = &outerr

	err = cmd.Run()
	if err != nil {
		return
	}

	return
}
func TableInsert_2(db *gorm.DB, sqlfile io.Reader) error {
	var err error
	//存储临时文件 到 output/temp
	if _, err := os.Stat("./output/temp"); err != nil {
		if !os.IsExist(err) {
			if err = os.MkdirAll("./output/temp", os.ModePerm); err != nil {
				return err
			}
		}
	}
	sqlfiletemp, err := os.CreateTemp("./output/temp/", "sqltemp_"+fmt.Sprint(time.Now().Unix()))
	if err != nil {
		return err
	}

	sqltname, err := filepath.Abs(sqlfiletemp.Name())
	if err != nil {
		return err
	}
	defer func() {
		sqlfiletemp.Close()
		os.Remove(sqltname)
	}()

	_, err = io.Copy(sqlfiletemp, sqlfile)
	if err != nil {
		return err
	}
	//导入
	var createfile string
	if runtime.GOOS == "windows" {
		createfile = "./output/temp/my.ini"
	}
	if runtime.GOOS == "linux" {
		createfile = "./output/temp/my.cnf"
	}
	inifile, err := os.Create(createfile)
	if err != nil {
		return err
	}
	defer os.Remove(inifile.Name())
	inifilepath, _ := filepath.Abs(inifile.Name())
	inifilepath = filepath.ToSlash(inifilepath)
	inistr := fmt.Sprintf("[mysqld]\nsql-mode=%s\n[client]\nuser=%s\npassword=%s\n", "NO_AUTO_CREATE_USER,NO_ENGINE_SUBSTITUTION", dbconfig.Admin, dbconfig.Password)
	inifile.Write([]byte(inistr))
	inifile.Close()
	insertstr := fmt.Sprintf("use %s;set names utf8;source %s;", dbconfig.Schema, filepath.ToSlash(sqltname))
	opt := []string{"--defaults-extra-file=" + inifilepath, "-e", insertstr}
	//目标表格
	cmd := exec.Command("mysql", opt...)
	err = cmd.Run()
	if err != nil {
		fmt.Println(err)
		return err
	}
	return nil
}

// 直接命令行输入
func TableInsert(db *gorm.DB, sqlfile *os.File) error {
	defer sqlfile.Close()
	//存储临时文件 到 output/temp
	if _, err := os.Stat("./output/temp"); err != nil {
		if !os.IsExist(err) {
			if err = os.MkdirAll("./output/temp", os.ModePerm); err != nil {
				return err
			}
		}
	}
	sqls, _ := ioutil.ReadAll(sqlfile)
	sqlArr := strings.Split(string(sqls), ";")
	for _, sql := range sqlArr {
		sql = strings.TrimSpace(sql)
		if sql == "" {
			continue
		}
		err := db.Exec(sql).Error
		if err != nil {
			return err
		}
	}
	return nil

}

var Tablename map[string]interface{} = map[string]interface{}{
	"factory":               &Factory{},
	"windfarm":              &Windfarm{},
	"machine":               &Machine{},
	"part":                  &Part{},
	"point":                 &Point{},
	"property":              &Property{},
	"point_std":             &PointStd{},
	"property_std":          &PropertyStd{},
	"alert":                 &Alert{},
	"band":                  &alert.Band{},
	"band_alert":            &alert.BandAlert{},
	"tree_alert":            &alert.TreeAlert{},
	"manual_alert":          &alert.ManualAlert{},
	"machine_daily_report":  &MachineDailyReport{},
	"machine_month_report":  &MachineMonthReport{},
	"windfarm_daily_report": &WindfarmDailyReport{},
	"windfarm_month_report": &WindfarmMonthReport{},
}

// * 导入数据库
// 合并所有前缀含backup的表
// 1.设备相关
// 2.数据 报警相关
func TableCombine(db *gorm.DB) error {
	t, err := db.Migrator().GetTables()
	if err != nil {
		return err
	}
	//表的分类
	var BackupTableStr []string
	var DataTableStr []string
	var ReportTableStr []string
	var MsgTableStr []string

	for _, v := range t {
		if strings.Contains(v, "_backup_") {
			BackupTableStr = append(BackupTableStr, v)
			if strings.Contains(v, "data_") {
				DataTableStr = append(DataTableStr, v)
				continue
			}
			if strings.Contains(v, "report") {
				ReportTableStr = append(ReportTableStr, v)
				continue
			}
			if !strings.Contains(v, "data") && !strings.Contains(v, "wave") && !strings.Contains(v, "report") {
				MsgTableStr = append(MsgTableStr, v)
				continue
			}
		}
	}

	//先导入设备表，再导入数据，再导入故障
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, v := range MsgTableStr {
			if strings.Contains(v, "_backup_") {
				vnew := strings.Trim(v, "_backup_")
				vs := strings.SplitN(vnew, "_", 2)
				if _, ok := Tablename[vs[1]]; ok {
					if !db.Migrator().HasTable(vs[1]) {
						db.Migrator().AutoMigrate(Tablename[vs[1]])
					}
				}
			}
		}
		for _, v := range ReportTableStr {
			if strings.Contains(v, "_backup_") {
				vnew := strings.Trim(v, "_backup_")
				vs := strings.SplitN(vnew, "_", 2)
				if _, ok := Tablename[vs[1]]; ok {
					if !db.Migrator().HasTable(vs[1]) {
						db.Migrator().AutoMigrate(Tablename[vs[1]])
					}
				}
			}
		}
		return nil
	})

	//转移
	var AlertTable string
	err = db.Transaction(func(tx *gorm.DB) error {
		err = tx.Transaction(func(ttx *gorm.DB) error {
			//设备
			for _, v := range MsgTableStr {
				if strings.Contains(v, "_backup_") {
					vnew := strings.Trim(v, "_backup_")
					vs := strings.SplitN(vnew, "_", 2)
					if vs[1] == "alert" {
						AlertTable = v
						fmt.Println(AlertTable)
						continue
					} else if _, ok := Tablename[vs[1]]; ok {
						ds := make([]map[string]interface{}, 0)
						var tatalc int64
						db.Table(v).Select("id").Count(&tatalc)
						for i := 0; i*1000 <= int(tatalc); i++ {
							db.Table(v).Model(Tablename[vs[1]]).Omit("id").
								Limit(1000).Offset(i * 1000).Find(&ds)
							for _, v := range ds {
								var c int64
								db.Table(vs[1]).Where("uuid=?", v["uuid"]).Limit(1).Count(&c)
								if c == 0 {
									err = ttx.Model(Tablename[vs[1]]).Table(vs[1]).Clauses(clause.Locking{Strength: "UPDATE"}).
										Create(v).Error
									if err != nil {
										modlog.Error("同步%s数据错误：%s", vs[1], err)
										return err
									}
								}
							}
						}
						if err != nil {
							return err
						}
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		err = tx.Transaction(func(ttx *gorm.DB) error {
			//导入数据
			//1.找到data下所有内容（wave alert band_alert tree_alert manual_alert) 忽略主键
			//2.插入 新建data结构体
			for _, v := range DataTableStr {
				if strings.Contains(v, "_backup_") {
					vnew := strings.Trim(v, "_backup_")
					vs := strings.SplitN(vnew, "_", 2)
					fids := strings.Split(vs[1], "_")
					datatable := strings.TrimSuffix(vs[1], "_"+fids[len(fids)-1])
					//检查数据对应风机在目标数据库的id
					var fid string
					if db.Migrator().HasTable("_backup_" + vs[0] + "_machine") {
						var fan map[string]interface{}
						db.Table("_backup_"+vs[0]+"_machine").Where("id=?", fids[len(fids)-1]).
							Select("uuid", "name").Limit(1).Find(&fan)
						tx.Table("machine").Where("uuid =? AND name=?", fan["uuid"], fan["name"]).Pluck("id", &fid)
						if fid == "" {
							continue
						} else {
							//TODO check 数据表单是否存在
							if !db.Migrator().HasTable("data_" + fid) {
								db.Table(fmt.Sprintf("data_%v", fid)).AutoMigrate(&Data_Update{})
							}
							if !db.Migrator().HasTable("wave_" + fid) {
								db.Table(fmt.Sprintf("wave_%v", fid)).AutoMigrate(&Wave_Update{})
							}
							// 转速
							if !db.Migrator().HasTable("data_rpm_" + fid) {
								db.Table(fmt.Sprintf("data_rpm_%v", fid)).AutoMigrate(&Data_Update{})
							}
							if !db.Migrator().HasTable("wave_rpm_" + fid) {
								db.Table(fmt.Sprintf("wave_rpm_%v", fid)).AutoMigrate(&Wave_Update{})
							}
							if db.Migrator().HasTable("data") {
								db.Migrator().DropTable("data")
							}
						}
					}

					// 粘贴数据
					ds := make([]map[string]interface{}, 0)
					var tatalc int64
					db.Table(v).Select("id").Count(&tatalc)
					for i := 0; i*1000 <= int(tatalc); i++ {
						err = db.Table(v).Model(&Data{}).Omit("id").
							Limit(1000).Offset(i * 1000).Find(&ds).Error
						for k, v := range ds {
							var c int64
							db.Table(datatable+"_"+fid).
								Where("uuid=?", v["uuid"]).Limit(1).Count(&c)
							if c == 0 {
								err = ttx.Model(&Data{}).Table(datatable + "_" + fid).Clauses(clause.Locking{Strength: "UPDATE"}).
									Create(ds[k]).Error
								if err != nil {
									modlog.Error("同步%s数据错误：%s", datatable+"_"+fid, err)
									return err
								}
							}
						}
					}
					if err != nil {
						return err
					}
					ds = make([]map[string]interface{}, 0)
					tatalc = 0
					db.Table(v).Select("id").Count(&tatalc)
					for i := 0; i*1000 <= int(tatalc); i++ {
						err = db.Table(strings.Replace(v, "data", "wave", 1)).Model(&Wave{}).Omit("id").Limit(1000).Offset(i * 1000).Find(&ds).Error
						for k, v := range ds {
							var c int64
							db.Table(datatable+"_"+fid).Where("uuid=?", v["uuid"]).Limit(1).Count(&c)
							if c == 0 {
								err = ttx.Model(&Wave{}).Table(strings.Replace(datatable, "data", "wave", 1) + "_" + fid).Clauses(clause.Locking{Strength: "UPDATE"}).
									Create(ds[k]).Error
								if err != nil {
									modlog.Error("同步%s数据错误：%s", strings.Replace(datatable, "data", "wave", 1), err)
									return err
								}
							}
						}
					}
					if err != nil {
						return err
					}
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		// alert
		if strings.Contains(AlertTable, "_backup_") {
			vnew := strings.Trim(AlertTable, "_backup_")
			vs := strings.SplitN(vnew, "_", 2)
			ds := make([]map[string]interface{}, 0)
			var tatalc int64
			db.Table(AlertTable).Select("id").Count(&tatalc)
			for i := 0; i*1000 <= int(tatalc); i++ {
				err = db.Table(AlertTable).Model(Tablename[vs[1]]).Omit("id").
					Limit(1000).Offset(i * 1000).Find(&ds).Error
				for _, v := range ds {
					var c int64
					db.Table(vs[1]).Where("uuid=?", v["uuid"]).Limit(1).Count(&c)
					if c == 0 {
						err = tx.Transaction(func(ttx *gorm.DB) error {
							err = ttx.Model(Tablename[vs[1]]).Table(vs[1]).Clauses(clause.Locking{Strength: "UPDATE"}).Create(v).Error
							if err != nil {
								return err
							}
							return nil
						})
						if err != nil {
							modlog.Error("同步%s数据错误：%s", vs[1], err)
							return err
						}
						var newalert Alert
						tx.Table(vs[1]).Where("uuid=?", v["uuid"]).Limit(1).Find(&newalert)
						err = UpdateReportAfterAlert(tx, newalert)
						if err != nil {
							modlog.Error("同步%s数据错误：%s", vs[1], err)
							return err
						}
					}
				}
			}
		}

		if err != nil {
			return err
		}
		//删除backup
		for _, v := range BackupTableStr {
			err = db.Migrator().DropTable(v)
			if err != nil {
				return err
			}
		}
		return err
	})
	if err != nil {
		return err
	}
	return nil
}

// 1.相同uuid，按updated_time更新时间更新覆盖。
func GetOutput(reader *bufio.Reader) {
	var sumOutput string //统计屏幕的全部输出内容
	outputBytes := make([]byte, 200)
	for {
		n, err := reader.Read(outputBytes)
		//获取屏幕的实时输出(并不是按照回车分割，所以要结合sumOutput)
		if err != nil {
			if err == io.EOF {
				break
			}
			sumOutput += err.Error()
		}
		output := string(outputBytes[:n])
		sumOutput += output
	}
}
