package main

//cms 对所有子程序的管理和启动
import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"main/mod"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/shirou/gopsutil/process"
)

var (
	config    = flag.String("dbconfig", "./GormConfig.toml", "数据库连接配置")
	modconfig = flag.String("mod", "./modinit.toml", "服务配置")
	// tomlstd   = flag.String("demostd", "n", "是否导入测试用标准文件，默认不导入")
)

type ModWatch struct {
	MainPort string
	Config   string
	wg       *sync.WaitGroup
	Mod      []ModProcess
	ModIndex map[string]*ModProcess
}

var initMessage *ModWatch = &ModWatch{}
var mainlog *mod.Log = &mod.Log{}
var dataurl string

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
}

func (initMessage *ModWatch) InitMod(modconfig string) error {
	initMessage.Config = *config
	initMessage.ModIndex = make(map[string]*ModProcess)
	initMessage.wg = &sync.WaitGroup{}
	_, err := toml.DecodeFile(modconfig, initMessage)
	if err != nil {
		mainlog.Error("从文件初始化服务配置错误，按默认进行。%v", err)
		//默认
		initMessage.Mod = []ModProcess{
			{
				Name: "CMS系统",
				Port: "3000",
				Dir:  "./cmsProgram",
			},
			{
				Name: "数据分析服务",
				Port: "3006",
				Dir:  "./analysis/CMSSP_CROW",
			},
			{
				Name: "ModbusTCP服务",
				Port: "3001",
				Dir:  "./analysis/cmsModbus",
			},
			{
				Name: "数据自动导入服务",
				Port: "3002",
				Dir:  "./analysis/cmsDatawatch",
			},
		}
	}
	//添加索引
	for k, v := range initMessage.Mod {
		if runtime.GOOS == "windows" {
			initMessage.Mod[k].Dir = initMessage.Mod[k].Dir + ".exe"
		}
		initMessage.ModIndex[v.Name] = &initMessage.Mod[k]
		initMessage.Mod[k].Exitchan = make(chan struct{}, 1)
		initMessage.Mod[k].Ticker = time.NewTicker(time.Second)
		_, err := net.DialTimeout("tcp", "127.0.0.1:"+initMessage.Mod[k].Port, 500*time.Millisecond)
		if err == nil {
			fmt.Println(initMessage.Mod[k].Name, " 127.0.0.1:"+initMessage.Mod[k].Port, "端口已占用!")
			return errors.New("wrong port")
		}
	}
	fmt.Println("设置端口均未占用ok")
	if p, ok := initMessage.ModIndex["数据分析服务"]; ok {
		p.Args = append(p.Args, []string{p.Port}...)
		dataurl = "localhost:" + p.Port
	}
	if p, ok := initMessage.ModIndex["CMS系统"]; ok {
		p.Args = append(p.Args, []string{"-dbconfig", initMessage.Config, "-dataurl", dataurl, "-p", p.Port}...)

	}
	if p, ok := initMessage.ModIndex["ModbusTCP服务"]; ok {
		p.Args = append(p.Args, []string{"-dbconfig", initMessage.Config}...)

	}
	if p, ok := initMessage.ModIndex["数据自动导入服务"]; ok {
		p.Args = append(p.Args, []string{"-dbconfig", initMessage.Config, "-dataip", "localhost", "-dataport", initMessage.ModIndex["数据分析服务"].Port}...)
	}
	//监测端口是否正常
	return nil
}

func (mw *ModWatch) Run() {
	sigchan := make(chan os.Signal, 1)
	modchan := make(chan struct{}, 1)

	mw.wg.Add(1)
	signal.Notify(sigchan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	go func() {
		<-sigchan
		for k := range mw.Mod {
			mw.Mod[k].Exitchan <- struct{}{}
		}
		mw.wg.Done()
	}()
	if mw.Mod[0].Name == "CMS系统" {
		mw.wg.Add(1)
		go func(k int) {
			err := mw.Mod[k].Run()
			if err != nil {
				fmt.Println("CMS主程序启动有误，退出CMS。")
				sigchan <- syscall.SIGQUIT
				close(modchan)
			} else {
				modchan <- struct{}{}
			}
			<-mw.Mod[k].Exitchan
			mw.wg.Done()
		}(0)
		time.Sleep(1 * time.Second)
	}
	if _, ok := <-modchan; ok {
		for k := 1; k < len(mw.Mod); k++ {
			mw.wg.Add(1)
			go func(k int) {
				mw.Mod[k].Run()
				mw.wg.Done()
			}(k)
		}
		close(modchan)
	}
	mw.wg.Wait()
	for k := range mw.Mod {
		close(mw.Mod[k].Exitchan)
	}
	fmt.Println("退出所有服务")
	os.Exit(1)
}

type ModProcess struct {
	Name     string
	Port     string
	Dir      string
	Args     []string
	pid      int
	cmd      *exec.Cmd
	Exitchan chan struct{}
	Ticker   *time.Ticker
}

func (mp *ModProcess) exec() error {
	mp.cmd = exec.Command(mp.Dir, mp.Args...)
	err := mp.cmd.Start()
	if err != nil {
		return err
	} else {
		mainlog.Info("启动%s模块成功 http server started on [::]:%s", mp.Name, mp.Port)
		mp.pid = mp.cmd.Process.Pid //获得进程ID
		err = mp.cmd.Wait()
		if err != nil {
			return err
		}
		return nil
	}
}
func (mp *ModProcess) Run() (err error) {
	mp.cmd = exec.Command(mp.Dir, mp.Args...)

	//实时获取输出参数
	// 获取输出管道
	stdout, err := mp.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	// 在goroutine中循环读取输出
	go func() {
		reader := bufio.NewReader(stdout)
		for {
			line, _, err := reader.ReadLine()
			if err != nil {
				break
			}
			fmt.Println(string(line))
		}
	}()
	//

	err = mp.cmd.Start()
	if err != nil {
		mainlog.Error("启动%s模块失败 Error:%s", mp.Name, err)
		return err
	} else {
		mainlog.Info("启动%s模块成功 http server started on [::]:%s", mp.Name, mp.Port)
		mp.pid = mp.cmd.Process.Pid //获得进程ID
		go func() {
			for {
				select {
				case <-mp.Exitchan:
					return
				case <-mp.Ticker.C:
					j, err := process.PidExists(int32(mp.pid))
					if err != nil {
						mainlog.Error("PID监控错误 err:%s", err)
					}
					if !j {
						mainlog.Error("监测到子服务pid %v 程序%s中断，准备重启... ", mp.pid, mp.Name)
						mp.exec()
						if mp.pid == 0 {
							mainlog.Error("%s启动失败。%s", mp.Name, err)
							return
						}
					}
				}
			}
		}()
	}
	go mp.cmd.Wait()
	return err
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			mainlog.Error("Panic!%v", r)
		}
	}()
	//进程管理
	err := initMessage.InitMod(*modconfig)
	if err != nil {
		return
	}
	initMessage.Run()
}
