package main

import (
	"io/ioutil"
	"main/mod"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"gorm.io/gorm"
)

func WatchFile(folder string) error {
	var err error
	watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	//folder 下所有文件夹进行监视
	err = watcher.Add(folder)
	if err != nil {
		ilog.Error(err.Error())
	}
	GetDir(folder, watcher)
	// var optionstr []string
	go func() {
		for {
			time.Sleep(500 * time.Millisecond)
			select {
			case <-dws.ControlChan:
				watcher.Close()
				dws = new(DataWatch)
				dws.ControlChan = make(chan struct{})
				return
			case event, ok := <-watcher.Events:
				//TODO debug
				dlog.Info("watch file: %s option：%s", event.Name, event.Op.String())
				if !ok {
					continue
				}
				// optionstr = append(optionstr, event.Name+event.Op.String())
				if event.Op.String() == "CREATE" {
					event.Name = strings.Replace(event.Name, "\\", "/", -1)
					efile, err := os.Stat(event.Name)
					if err != nil {
						ilog.Error(err.Error())
						continue
					}
					//是否为文件夹。1.添加文件夹的监视路径。2.对文件夹中所有文件导入。
					if efile.IsDir() {
						//添加文件夹的监视路径。
						GetDirtoInsert(event.Name, watcher)
						watcher.Add(event.Name)
					} else {
						FiletoInsertMap[event.Name] = 0
						go func(FiletoInsertMap map[string]int, datafile string) {
							time.Sleep(5 * time.Second)
							if FiletoInsertMap[event.Name] == 0 {
								if _, err := os.Stat(event.Name); err == nil {
									FiletoInsertLoop <- event.Name
									delete(FiletoInsertMap, event.Name)
								} else {
									delete(FiletoInsertMap, event.Name)
									return
								}
							}
						}(FiletoInsertMap, event.Name)
					}
				}
				if event.Op.String() == "WRITE" {
					event.Name = strings.Replace(event.Name, "\\", "/", -1)
					efile, err := os.Stat(event.Name)
					if os.IsNotExist(err) {
						continue
					}
					if efile.IsDir() {
						continue
					}
					if _, ok := FiletoInsertMap[event.Name]; ok {
						FiletoInsertMap[event.Name]++
					}
				}
				if event.Op.String() == "CHMOD" {
					event.Name = strings.Replace(event.Name, "\\", "/", -1)
					efile, err := os.Stat(event.Name)
					if os.IsNotExist(err) {
						continue
					}
					if efile.IsDir() {
						continue
					}
					event.Name = strings.Replace(event.Name, "\\", "/", -1)
					if p, ok := FiletoInsertMap[event.Name]; ok {
						if p >= 1 {
							FiletoInsertLoop <- event.Name
							delete(FiletoInsertMap, event.Name)
						}
					}
				}
				if event.Op.String() == "RENAME" || event.Op.String() == "REMOVE" {
					event.Name = strings.Replace(event.Name, "\\", "/", -1)
					watcher.Remove(event.Name)
				}
			case err, ok := <-watcher.Errors:
				if ok {
					ilog.Error(err.Error())
					continue
				}
			}
		}
	}()
	for {
		if file, ok := <-FiletoInsertLoop; ok {
			DataOpt(file)
		} else if !ok {
			return nil
		}
	}
	return nil
}

func GetDir(folder string, fs *fsnotify.Watcher) {
	rd, _ := ioutil.ReadDir(folder)
	for _, file := range rd {
		if file.IsDir() {
			fs.Add(folder + "/" + strings.Replace(file.Name(), "\\", "/", -1))
			GetDir(folder+"/"+file.Name(), fs)
		}
	}
}
func GetDirtoInsert(folder string, fs *fsnotify.Watcher) {
	rd, _ := ioutil.ReadDir(folder)
	for _, file := range rd {
		if file.IsDir() {
			fs.Add(folder + "/" + file.Name())
			GetDirtoInsert(folder+"/"+file.Name(), fs)
		} else {
			err := InsertPointData(*dataip+":"+*dataport, folder+"/"+file.Name())
			if err != nil {
				ilog.Error(err.Error())
			} else {
				if err := os.Remove(folder + "/" + file.Name()); err != nil {
					dlog.Error("删除数据失败。%s", err.Error())
				} else {
					dlog.Info("导入后删除数据 %s", folder+"/"+file.Name())
				}
			}
		}
	}
}

func InsertPointData(dataipport string, filename string) error {
	var err error
	var src *os.File
	file := strings.Split(filename, "\\")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return nil
	}
	src, err = os.Open(filename)
	if err != nil {
		return err
	}
	defer src.Close()
	var info string
	var filedata []byte
	var parsing mod.Parsing
	if err = db.Table("parsing").Where("is_del = false").First(&parsing).Error; err != nil {
		return err
	}
	filetype := strings.Split(file[len(file)-1], ".")
	// 判断文件类型 不同文件的导入
	info, filedata, err = mod.TypeRead(filetype[len(filetype)-1], src, parsing)
	if err != nil {
		return err
	}
	// 找测点并导入数据库
	var pdata mod.Data
	err = pdata.DataInfoGet(db, info, filedata, parsing)
	if err != nil {
		return err
	}
	err = mod.CheckData(db, &pdata)
	if err != nil {
		return err
	}
	if pdata.ID != 0 {
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
			if err = mod.InsertData(db, tx, dataipport, pdata); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}
		dlog.Info("%v 文件，覆盖了数据。", filename)
	} else {
		if err = mod.InsertData(db, db, dataipport, pdata); err != nil {
			return err
		}
		dlog.Info("%v 文件，写入了数据。", filename)
	}
	return nil
}
