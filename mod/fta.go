package mod

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"main/alert"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 从标准文件读取故障树
// 查询对应参数写进故障树
// 获得结构体
// 输入数据id和测点id
// TODO 2023/12/15  新增将故障说明插入到故障标签中，以便于后续的数据标签模糊查询
func TreeAlert(db *gorm.DB, pdata Data, ipport string) (uint8, error, []int) {
	tagIds := make([]int, 0)
	var maxlevel uint8
	//由测点定位到所属部件
	ppmwfid, _, _, err := PointtoFactory(db, pdata.PointID)
	if err != nil {
		return 0, err, nil
	}

	var parttype string
	var partname string //以部件名称筛选
	err = db.Table("part").Where("id=?", ppmwfid[1]).Pluck("type", &parttype).Error
	if err != nil {
		return 0, err, nil
	}
	err = db.Table("part").Where("id=?", ppmwfid[1]).Pluck("name", &partname).Error
	if err != nil {
		return 0, err, nil
	}

	//故障树版本：测点 → 机器部件
	var treeversion string
	err = db.Table("point").Where("uuid=?", pdata.PointUUID).Pluck("tree_version", &treeversion).Error
	if err != nil {
		return 0, err, nil
	}
	if treeversion == "" {
		err = db.Table("machine").Where("id=?", ppmwfid[2]).Pluck("tree_version", &treeversion).Error
		if err != nil {
			return 0, err, nil
		}
	}
	if treeversion == "" {
		return 0, nil, nil
	}
	faultpath := "./faulttree/"
	//数据索引
	var index map[string]interface{} = make(map[string]interface{})
	MaptoStruct(pdata, &index)
	//根据部件类型搜索故障树
	_, err = os.Stat(faultpath + parttype)
	if os.IsNotExist(err) {
		return 0, err, nil
	}
	rd, err := ioutil.ReadDir(faultpath + parttype)
	if err != nil {
		return 0, err, nil
	}
	// fmt.Println("./faulttree/"+parttype, treeversion)
	// fmt.Println(rd)

	var filemap []string
	for _, fi := range rd {
		if !fi.IsDir() {
			if path.Ext(fi.Name()) == ".toml" {
				//版本_部件类型_名称.toml 版本满足设备故障树版本，类型满足测点所属部件类型
				//info[0] 版本
				//info[1] type
				info := strings.Split(fi.Name(), "_")
				if info[0] == treeversion {
					filemap = append(filemap, faultpath+parttype+"/"+fi.Name())
				}
			}
		}
	}
	for k := range filemap {
		file, err := os.Open(filemap[k])
		if err != nil {
			return 0, err, nil
		}
		//对每个故障树进行获取、解析
		basict := new(alert.BasicTree)
		err = FileGet(file, basict)
		if err != nil {
			return 0, err, nil
		}
		if basict.Type != partname {
			continue
		}
		file, err = os.Open(filemap[k])
		if err != nil {
			return 0, err, nil
		}
		t, err := TreeGet(file)
		if err != nil {
			return 0, err, nil
		}
		// * 计算和报警写入
		err = TreeCalculate_2(t, db, pdata, index, ppmwfid, ipport)
		if err != nil {
			return 0, err, nil
		}
		// Dubug 打印
		// jsonBytes, _ := json.MarshalIndent(t.Nodes, "", "\t")
		// fmt.Println(string(jsonBytes))
		//*报警插入
		level, ids, err := TreeAlertSet_2(t, db, pdata, filemap[k])
		if err != nil {
			return 0, err, nil
		}
		if maxlevel < level {
			maxlevel = level
		}
		tagIds = append(tagIds, ids...)

	}
	return maxlevel, nil, tagIds
}

// 读取故障树的toml到故障树结构体
// 各个节点解析。获取底层的特征值字段
// 索引风机、索引设备组态中特征值的具体字段得到值
// 自动安排id和layer编号
// 输出：结构体 alert.tree
func TreeGet(file io.Reader) (*alert.Tree, error) {
	t := new(alert.Tree)
	err := FileGet(file, t)
	if err != nil {
		return nil, err
	}
	t.NodesMap = make(map[int][]*alert.Node)
	t.ValueMap = make(map[string]interface{})

	id := new(int)
	l := new(int)
	*id = 1
	*l = 0
	RangeNodesID(t.Nodes, id)
	RangeNodesLayer(t.Nodes, l, &t.Layer, t.NodesMap)
	return t, nil
}

// 故障树分析、逻辑计算
func TreeCalculate_2(t *alert.Tree, db *gorm.DB, pdata Data, index map[string]interface{}, ppmwfid []string, ipport string) error {
	// 先给rpm
	index["rpm"] = float64(pdata.Rpm) / 60
	//遍历故障树每一层,由大到小
	for i := t.Layer; i >= 0; i-- {
		//找到该层所有节点，遍历该层所有节点
		for _, node := range t.NodesMap[i] {
			if node.Leaves[0] == -1 { //是否为每一分支最底层节点
				if !node.Message {
					//获取用于对比的truevalue
					NodeValueGet_2(t, node, db, pdata, index, ppmwfid)
				} else {
					node.Result = true
				}
			} else {
				if !node.Message {
					//调用节点计算函数
					NodeLogic(node)
				}
			}
		}
	}
	//* 阶段判断
	if t.Nodes[0].Result {
		if len(t.Stages) > 0 {
			for k := range t.Stages {
				for kc := range t.Stages[k].Calculate {
					if t.Stages[k].Calculate[kc].Goal1 != "" {
						if _, ok := t.ValueMap[t.Stages[k].Calculate[kc].Goal1]; !ok {
							//频谱计算
							if strings.Contains(t.Stages[k].Calculate[kc].Goal1, "rms_band") {
								//* 测试
								ss, err := BandAnalysis_2(db, ipport, ppmwfid, pdata, t.Stages[k].Calculate[kc].Goal1)
								// 避免错误退出
								for i := 0; i < 3; i++ {
									if err != nil {
										time.Sleep(100 * time.Millisecond)
										ss, err = BandAnalysis_2(db, ipport, ppmwfid, pdata, t.Stages[k].Calculate[kc].Goal1)
									}
								}
								if err != nil {
									return err
								}
								//*
								t.ValueMap[t.Stages[k].Calculate[kc].Goal1] = ss
							} else {
								if _, ok := index[t.Stages[k].Calculate[kc].Goal1]; ok {
									t.ValueMap[t.Stages[k].Calculate[kc].Goal1] = float32(index[t.Stages[k].Calculate[kc].Goal1].(float64))
								} else if _, ok := index["result"].(map[string]interface{})[t.Stages[k].Calculate[kc].Goal1]; ok {
									//从data.result中
									t.ValueMap[t.Stages[k].Calculate[kc].Goal1] = float32(index["result"].(map[string]interface{})[t.Stages[k].Calculate[kc].Goal1].(float64))
								} else {
									//从property中
									f, err := NodeColumnIndexGet_2(db, ppmwfid, pdata, t.Stages[k].Calculate[kc].Goal1)
									t.ValueMap[t.Stages[k].Calculate[kc].Goal1] = f
									if err != nil {
										return err
									}
								}
								// fmt.Println(t.Stages[k].Calculate[kc].Goal1, t.ValueMap[t.Stages[k].Calculate[kc].Goal1])
							}
						}
					}
					//大小比较
					if t.ValueMap[t.Stages[k].Calculate[kc].Goal1].(float32) < t.Stages[k].Calculate[kc].Upper && t.ValueMap[t.Stages[k].Calculate[kc].Goal1].(float32) > t.Stages[k].Calculate[kc].Lower {
						t.Stages[k].Result = true
						t.Stages[k].TrueValue = append(t.Stages[k].TrueValue, t.ValueMap[t.Stages[k].Calculate[kc].Goal1].(float32))
					} else {
						t.Stages[k].Result = false
						t.Stages[k].TrueValue = append(t.Stages[k].TrueValue, t.ValueMap[t.Stages[k].Calculate[kc].Goal1].(float32))
					}
				}
			}
		}
	}
	return nil
}

func GetSpectrumValue(key float32, fs int, y []float32) (r float32) {
	rx := key / (float32(fs) / float32(len(y)))
	if rx < 0 || int(rx) >= len(y) {
		r = float32(0)
	} else {
		r = y[int(rx)]
	}
	return
}

// 获取故障树中所需参数 （频率需要在频谱中找对应的幅值）
func NodeValueGet_2(t *alert.Tree, n *alert.Node, db *gorm.DB, pdata Data, index map[string]interface{}, ppmwfid []string) (err error) {
	//properties get
	if _, ok := t.ValueMap["spectrum"]; !ok {
		t.DataTime = pdata.TimeSet
		t.Rpm = pdata.Rpm
		y := make([]float32, len(pdata.Wave.SpectrumFloat)/4)
		Decode(pdata.Wave.SpectrumFloat, &y)
		//频谱
		t.ValueMap["spectrum"] = y
		t.ValueMap["sample_freq"] = pdata.SampleFreq
	}

	if n.Calculate.Goal1 != "" {
		if _, ok := t.ValueMap[n.Calculate.Goal1]; !ok {
			//先找data的result中的结果，再找property
			t.Index = n.Calculate.Goal1
			//从data中
			if _, ok := index[n.Calculate.Goal1]; ok {
				t.ValueMap[n.Calculate.Goal1] = float32(index[n.Calculate.Goal1].(float64))
			} else if _, ok := index["result"].(map[string]interface{})[n.Calculate.Goal1]; ok {
				//从data.result中
				t.ValueMap[n.Calculate.Goal1] = float32(index["result"].(map[string]interface{})[n.Calculate.Goal1].(float64))
			} else {
				//从property中
				f, err := NodeColumnIndexGet_2(db, ppmwfid, pdata, n.Calculate.Goal1)
				t.ValueMap[n.Calculate.Goal1] = f
				if err != nil {
					return err
				}
			}
			// fmt.Println(n.Calculate.Goal1, t.ValueMap[n.Calculate.Goal1], index[n.Calculate.Goal1])
		}
	}
	//goal 2 rpm get
	if n.Calculate.Goal2 != "" {
		if _, ok := index[n.Calculate.Goal2]; ok {
			t.ValueMap[n.Calculate.Goal2] = float32(index[n.Calculate.Goal2].(float64))
		} else if _, ok := index["result"].(map[string]interface{})[n.Calculate.Goal2]; ok {
			//从data.result中
			t.ValueMap[n.Calculate.Goal2] = float32(index["result"].(map[string]interface{})[n.Calculate.Goal2].(float64))
		} else {
			//从property中
			f, err := NodeColumnIndexGet_2(db, ppmwfid, pdata, n.Calculate.Goal2)
			t.ValueMap[n.Calculate.Goal2] = f
			if err != nil {
				return err
			}
		}
	}
	if n.Calculate.LowerGoal != "" {
		if _, ok := t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Lower, n.Calculate.LowerGoal)]; !ok {
			var f float32
			if _, ok := index[n.Calculate.LowerGoal]; ok {
				f = float32(index[n.Calculate.LowerGoal].(float64))
			} else if _, ok := index["result"].(map[string]interface{})[n.Calculate.LowerGoal]; ok {
				//从data.result中
				f = float32(index["result"].(map[string]interface{})[n.Calculate.LowerGoal].(float64))
			} else {
				f, err = NodeColumnIndexGet_2(db, ppmwfid, pdata, n.Calculate.LowerGoal)
				if err != nil {
					return err
				}
			}
			x := n.Calculate.Lower * f
			rf := GetSpectrumValue(x, t.ValueMap["sample_freq"].(int), t.ValueMap["spectrum"].([]float32))
			t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Lower, n.Calculate.LowerGoal)] = rf
			// fmt.Println(fmt.Sprintf("%v*%v", n.Calculate.Lower, n.Calculate.LowerGoal), t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Lower, n.Calculate.LowerGoal)])
		}
	}
	if n.Calculate.UpperGoal != "" {
		if _, ok := t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Upper, n.Calculate.UpperGoal)]; !ok {
			var f float32
			if _, ok := index[n.Calculate.LowerGoal]; ok {
				f = float32(index[n.Calculate.LowerGoal].(float64))
			} else if _, ok := index["result"].(map[string]interface{})[n.Calculate.LowerGoal]; ok {
				//从data.result中
				f = float32(index["result"].(map[string]interface{})[n.Calculate.LowerGoal].(float64))
			} else {
				f, err = NodeColumnIndexGet_2(db, ppmwfid, pdata, n.Calculate.UpperGoal)
				if err != nil {
					return err
				}
			}
			x := n.Calculate.Upper * f
			rf := GetSpectrumValue(x, t.ValueMap["sample_freq"].(int), t.ValueMap["spectrum"].([]float32))
			t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Upper, n.Calculate.UpperGoal)] = rf
			// fmt.Println(fmt.Sprintf("%v*%v", n.Calculate.Upper, n.Calculate.UpperGoal), t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Upper, n.Calculate.UpperGoal)])
		}
	}
	x := NodeCalaulte(t, n)
	rf := GetSpectrumValue(x, t.ValueMap["sample_freq"].(int), t.ValueMap["spectrum"].([]float32))
	n.TrueValue = append(n.TrueValue, rf)
	//有上下限比较
	if n.Calculate.UpperGoal != "" || n.Calculate.LowerGoal != "" {
		var lower, upper float32
		if v, ok := t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Lower, n.Calculate.LowerGoal)]; ok {
			lower = v.(float32)
		} else {
			lower = n.Calculate.Lower
		}
		if v, ok := t.ValueMap[fmt.Sprintf("%v*%v", n.Calculate.Upper, n.Calculate.UpperGoal)]; ok {
			upper = v.(float32)
		} else {
			upper = n.Calculate.Upper
		}

		if n.TrueValue[0] < upper && n.TrueValue[0] > lower {
			n.Result = true
		} else {
			n.Result = false
		}
	} else {
		n.Result = NodeCompare(n.Calculate.Method, n.TrueValue[0], n.Calculate.Standard)
	}
	return nil
}

// 调用程序进行频带计算
func BandAnalysis(dbconfig *GormConfig, exepath string, ppmwfid []string, dataid string, freq int, columnname string) ([]string, error) {
	find := regexp.MustCompile("\\(.*\\)")
	refind := find.FindStringSubmatch(columnname)[0]
	rangenum := strings.Split(refind, ",")
	highband, _ := strconv.ParseFloat(strings.Trim(rangenum[1], ")"), 32)
	var commd string
	if highband > float64(freq/2) {
		commd = fmt.Sprintf("7 %v %s %v", freq, strings.Trim(rangenum[0], "("), freq/2)

	} else {
		commd = fmt.Sprintf("7 %v %s %s", freq, strings.Trim(rangenum[0], "("), strings.Trim(rangenum[1], ")"))
	}

	return dbconfig.DataAnalysis(exepath, "data_"+ppmwfid[3], "result_"+ppmwfid[3], dataid, commd)
}

func BandAnalysis_2(db *gorm.DB, ipport string, ppmwfid []string, pdata Data, columnname string) (float32, error) {
	find := regexp.MustCompile("\\(.*\\)")
	refind := find.FindStringSubmatch(columnname)[0]
	rangenum := strings.Split(refind, ",")
	highband, _ := strconv.ParseFloat(strings.Trim(rangenum[1], ")"), 32)
	lowband, _ := strconv.ParseFloat(strings.Trim(rangenum[0], "("), 32)
	if highband > float64(pdata.SampleFreq/2) {
		highband = float64(pdata.SampleFreq) / 2
	}
	var originy []float32 = make([]float32, len(pdata.Wave.DataFloat)/4)
	err := Decode(pdata.Wave.DataFloat, &originy)
	if err != nil {
		return 0, err
	}
	url := "http://" + ipport + "/api/v1/data/trans/6"
	type Basic struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	type DataPost7 struct {
		Datafloat []float32 `json:"data"`
		Freq      int       `json:"fs"`
		Floor     float32   `json:"floor"`
		Upper     float32   `json:"upper"`
		//返回
		Databack float32 `json:"rms"`
		Basic
	}
	postData := DataPost7{
		Datafloat: originy,
		Freq:      pdata.SampleFreq,
		Floor:     float32(lowband),
		Upper:     float32(highband),
	}
	postBody, err := json.Marshal(postData)
	if err != nil {
		return 0, err
	}
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postBody))
	if err != nil {
		return 0, err
	}
	// 读取响应内容
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&postData)
	if err != nil {
		return 0, err
	}
	if postData.Code != 200 {
		return 0, errors.New("计算错误。" + postData.Message)
	}
	return postData.Databack, err
}

// 获得关键字段的值。存储到map中
func NodeColumnIndexGet_2(db *gorm.DB, ppmwfid []string, pdata Data, columnname string) (f float32, err error) {
	//特征值的查找
	_, _, ppmwuuid, err := PointtoFactory(db, ppmwfid[0])
	if err != nil {
		return 0, err
	}
	sub := db.Table("property").
		Where("point_uuid=? AND name_en =?", ppmwuuid[0], strings.ToUpper(columnname)).
		Pluck("value", &f)
	err = sub.Error
	if err != nil {
		return f, err
	}
	r := sub.RowsAffected
	if r == 0 {
		sub = db.Table("property").Where("part_uuid=? AND name_en =?", ppmwuuid[1], strings.ToUpper(columnname)).Pluck("value", &f)
		err = sub.Error
		if err != nil {
			return f, err
		}
		r := sub.RowsAffected
		if r == 0 {
			return 0, errors.New("lack of property")
		}
	}
	// db.Table("part").Where("id=?", ppmwfid[1]).Pluck("uuid", &partuuid)
	// err = db.Table("property").Where("part_uuid=? AND name_en =?", partuuid, strings.ToUpper(columnname)).Pluck("value", &f).Error
	f = f * float32(pdata.Rpm) / 60
	return f, err
}

// 底层节点计算
func NodeCalaulte(t *alert.Tree, n *alert.Node) (rx float32) {
	var n1, n2 float32
	if n.Calculate.Goal1 != "" {
		n1 = n.Calculate.Value1 * t.ValueMap[n.Calculate.Goal1].(float32)
	}
	if n.Calculate.Goal2 != "" {
		n2 = n.Calculate.Value2 * t.ValueMap[n.Calculate.Goal2].(float32)
	}
	switch n.Calculate.Cal {
	case "+":
		rx = n1 + n2
	case "-":
		rx = n1 - n2
	case "*":
		rx = n1 * n2
	case "/":
		rx = n1 / n2
	default:
		rx = n1 + n2
	}
	return rx
}

//用于有上下限的比较（阶段判断或底层数据大小判断）

// 节点比较
func NodeCompare(method string, v1, v2 float32) bool {
	switch method {
	case ">":
		return v1 > v2
	case "<":
		return v1 < v2
	case ">=":
		return v1 >= v2
	case "<=":
		return v1 <= v2
	case "!=":
		return v1 != v2
	case "==":
		return v1 == v2
	}
	return false
}

// 节点逻辑
func NodeLogic(n *alert.Node) {
	var bs []bool
	for _, v := range n.Children {
		bs = append(bs, v.Result)

	}
	switch n.Name {
	case "或":
		if len(bs) > 0 {
			n.Result = bs[0]
			for i := 1; i < len(bs); i++ {
				n.Result = n.Result || bs[i]
			}
		}
	case "与":
		if len(bs) > 0 {
			n.Result = bs[0]
			for i := 1; i < len(bs); i++ {
				n.Result = n.Result && bs[i]
			}
		}
	}
}

// 给予ID编号，递归
func RangeNodesID(nodes []alert.Node, id *int) {
	for k, v := range nodes {
		if len(v.Children) != 0 {
			nodes[k].ID = *id
			*id++
			// fmt.Println(id, *id, v.Name, "not base")
			RangeNodesID(v.Children, id)
		} else {
			//计算值
			nodes[k].ID = *id
			*id++
			// fmt.Println(id, *id, v.Name, "base")
		}
	}
}

// 一个节点下所有子节点id的集合，递归
func RangeNodesLayer(nodes []alert.Node, flayer *int, maxlayer *int, m map[int][]*alert.Node) {
	for k, v := range nodes {
		if len(v.Children) != 0 {
			for _, vv := range v.Children {
				nodes[k].Leaves = append(nodes[k].Leaves, vv.ID)
			}
			nodes[k].Layer = *flayer
			m[*flayer] = append(m[*flayer], &nodes[k])
			if *flayer > *maxlayer {
				*maxlayer = *flayer
			}
			*flayer++
			RangeNodesLayer(v.Children, flayer, maxlayer, m)
		} else {
			//计算值
			nodes[k].Leaves = append(nodes[k].Leaves, -1)
			nodes[k].Children = []alert.Node{}
			nodes[k].Layer = *flayer
			if *flayer > *maxlayer {
				*maxlayer = *flayer
			}
			m[*flayer] = append(m[*flayer], &nodes[k])
		}
	}
	*flayer--
}

// 故障树信息插入
func TreeAlertSet(t *alert.Tree, db *gorm.DB, did string, pid string, filename string) (uint8, error) {
	//故障条目
	var ta Alert
	//第一层主节点状态
	if !t.NodesMap[0][0].Result {
		//没有故障
		return 1, nil
	} else {

		diduint, err := strconv.ParseUint(did, 10, 0)
		if err != nil {
			return 1, err
		}
		ta.DataID = uint(diduint)
		piduint, err := strconv.ParseUint(pid, 10, 0)
		if err != nil {
			return 1, err
		}
		ta.PointID = uint(piduint)
		//ta.location 部件
		ta.Location = t.Type
		//ta.level 默认故障树为4级
		ta.Level = 4
		//ta.type 类型
		ta.Type = "故障树"
		//ta.strategy 策略
		ta.Strategy = t.Index
		//ta.timeSet rpm
		ta.TimeSet = t.DataTime
		ta.Rpm = t.Rpm
		//ta.source
		ta.Source = 0
		//code faulttype 默认赋值“1”，1
		ta.Code = "1"
		ta.Faulttype = 1

		//阶段判断
		if len(t.Stages) > 0 {
			for k := range t.Stages {
				if t.Stages[k].Result {
					//ta.desc
					ta.Desc = t.Stages[k].Name
					ta.TreeAlert.TreeName = t.Stages[k].Name
				}
			}
		} else {
			ta.Desc = "触发 " + t.Name + " 故障树报警"
			ta.TreeAlert.TreeName = t.Name
		}
		ta.TreeAlert.FileName = filename

		err = db.Transaction(func(tx *gorm.DB) error {
			err = tx.Table("alert").Omit(clause.Associations).Create(&ta).Error
			if err != nil {
				return err
			}
			ta.TreeAlert.AlertID = ta.ID
			err = tx.Table("tree_alert").Create(&ta.TreeAlert).Error
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return 1, err
		}
		// fmt.Println(ta)
	}

	return ta.Level, nil
}
func TreeAlertSet_2(t *alert.Tree, db *gorm.DB, pdata Data, filename string) (uint8, []int, error) {
	//故障条目
	var ta Alert
	tagIds := make([]int, 0)
	ta.UUID = pdata.UUID
	ta.DataID = pdata.ID
	ta.DataUUID = pdata.UUID
	ta.PointID = pdata.PointID
	ta.PointUUID = pdata.PointUUID
	var partType string
	db.Table("point").Select("part.type_en").Joins("left join part on part.uuid = point.part_uuid").Where("point.uuid = ?", ta.PointUUID).Find(&partType)
	ta.TimeSet = t.DataTime
	//ta.location 部件
	ta.Location = t.Type
	//第一层主节点状态
	if !t.NodesMap[0][0].Result {
		//没有故障
		ta.Level = 1
		return 1, nil, nil
	} else {
		//ta.level 默认故障树为报警 3级
		ta.Level = 3
		//ta.type 类型
		ta.Type = "故障树"
		//ta.strategy 策略
		ta.Strategy = t.Index
		//ta.timeSet rpm
		ta.TimeSet = t.DataTime
		ta.Rpm = t.Rpm
		//ta.source
		ta.Source = 0
		//阶段判断
		if len(t.Stages) > 0 {
			for k := range t.Stages {
				if t.Stages[k].Result {
					ta.Desc = t.Stages[k].Name + t.Stages[k].Desc
					tagIds = append(tagIds, CheckTagExist(db, ta.PointUUID, ta.Desc))
					ta.Suggest = t.Stages[k].Suggest
					ta.TreeAlert.TreeName = t.Stages[k].Name
				}
			}
		} else {
			ta.Desc = t.Name + t.Desc
			tagIds = append(tagIds, CheckTagExist(db, ta.PointUUID, ta.Desc))
			ta.Suggest = t.Suggest
			ta.TreeAlert.TreeName = t.Name
		}
		ta.TreeAlert.FileTreeName = filename
		tjson, err := json.Marshal(t)
		if err != nil {
			return 1, nil, err
		}
		ta.TreeAlert.TreeJson = tjson

		err = db.Transaction(func(tx *gorm.DB) error {
			err := tx.Table("alert").Omit(clause.Associations).Create(&ta).Error
			if err != nil {
				return err
			}
			ta.TreeAlert.AlertID = ta.ID
			ta.TreeAlert.AlertUUID = ta.UUID
			err = tx.Table("tree_alert").Create(&ta.TreeAlert).Error
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return 1, nil, err
		}
	}
	return ta.Level, tagIds, nil
}
func TreeAlertDetail(db *gorm.DB, id string) (t alert.TreeAlert, err error) {
	var a Alert
	err = db.Table("alert").Preload("TreeAlert").Where("id=?", id).First(&a).Error
	if err != nil {
		return
	}
	err = json.Unmarshal(a.TreeAlert.TreeJson, &t.Tree)
	if err != nil {
		return
	}
	t.TreeName = a.TreeAlert.TreeName
	t.FileTreeName = a.TreeAlert.FileTreeName
	ppmwfid, _, _, err := PointtoFactory(db, a.PointID)
	if err != nil {
		return
	}
	if err = db.Table("data_"+ppmwfid[2]).Select("filepath").Where("uuid=?", a.DataUUID).Scan(&t.FileName).Error; err != nil {
		return
	}
	return t, err
}
