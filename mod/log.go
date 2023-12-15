// ^ 日志相关
package mod

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/robfig/cron/v3"
)

var modlog *Log

const (
	logflag = log.Ldate | log.Ltime
)

type Log struct {
	// LogFile     io.Writer
	LogFile     *os.File
	InfoLogger  *log.Logger
	WarnLogger  *log.Logger
	ErrorLogger *log.Logger
}

func (l *Log) Info(format string, v ...interface{}) {
	l.InfoLogger.Printf(format, v...)
	fmt.Printf(format+"\r\n", v...)
}
func (l *Log) Error(format string, v ...interface{}) {
	l.ErrorLogger.Printf(format, v...)
	fmt.Printf(format+"\r\n", v...)
}
func (l *Log) Warn(format string, v ...interface{}) {
	l.WarnLogger.Printf(format, v...)
}

func (l *Log) Loginit(logfolder string, cronstring string) error {
	var err error
	_, err = os.Stat(logfolder)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(logfolder, os.ModePerm); err != nil {
			fmt.Println("路径错误。")
			return err
		}
	}
	//每个月定时任务
	task := func() {
		if l.LogFile != nil {
			l.LogFile.Close()
		}
		fn := time.Now().Format("2006-01") + ".log"
		l.LogFile, err = os.OpenFile(logfolder+"/"+fn, os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModeAppend|os.ModePerm)
		if err != nil {
			log.Fatalf("create log file err %+v", err)
		}
		l.InfoLogger = log.New(l.LogFile, "[INFO]", logflag)
		l.WarnLogger = log.New(l.LogFile, "[WARN]", logflag)
		l.ErrorLogger = log.New(l.LogFile, "[ERRO]", logflag)
		l.InfoLogger.SetOutput(l.LogFile)
		l.WarnLogger.SetOutput(l.LogFile)
		l.ErrorLogger.SetOutput(l.LogFile)
	}
	task()
	modlog = l
	logcron := cron.New(cron.WithParser(cron.NewParser(
		cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor,
	)), cron.WithChain(
		cron.Recover(cron.DefaultLogger), // or use cron.DefaultLogger
	))
	_, err = logcron.AddFunc(cronstring, task)
	if err != nil {
		return err
	}
	logcron.Start()
	return nil
}

func (l *Log) LoginitDaily(logfolder string, cronstring string) error {
	var err error
	_, err = os.Stat(logfolder)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(logfolder, os.ModePerm); err != nil {
			fmt.Println("路径错误。")
			return err
		}
	}
	//每个月定时任务
	task := func() {
		fn := time.Now().Format("2006-01-02") + ".log"
		l.LogFile, err = os.OpenFile(logfolder+"/"+fn, os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModeAppend|os.ModePerm)
		if err != nil {
			log.Fatalf("create log file err %+v", err)
		}
		l.InfoLogger = log.New(l.LogFile, "[INFO]", logflag)
		l.WarnLogger = log.New(l.LogFile, "[WARN]", logflag)
		l.ErrorLogger = log.New(l.LogFile, "[ERRO]", logflag)
		l.InfoLogger.SetOutput(l.LogFile)
		l.WarnLogger.SetOutput(l.LogFile)
		l.ErrorLogger.SetOutput(l.LogFile)
	}
	task()
	modlog = l
	logcron := cron.New(cron.WithParser(cron.NewParser(
		cron.SecondOptional|cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow|cron.Descriptor,
	)), cron.WithChain(
		cron.Recover(cron.DefaultLogger), // or use cron.DefaultLogger
	))
	_, err = logcron.AddFunc(cronstring, task)
	if err != nil {
		return err
	}
	logcron.Start()
	return nil
}
