package mod

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"main/alert"
	"mime/multipart"
	"net/http"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
	"gorm.io/gorm"
)

// 将GBK编码的字符串转换为utf-8编码
func ConvertGBK2Str(gbkStr string) string {
	//如果是[]byte格式的字符串，可以使用Bytes方法
	b, err := simplifiedchinese.GBK.NewDecoder().String(gbkStr)
	if err != nil {
		return b //如果转换失败返回空字符串
	}
	return b
}

func preNUm(data byte) int {
	var mask byte = 0x80
	var num int = 0
	//8bit中首个0bit前有多少个1bits
	for i := 0; i < 8; i++ {
		if (data & mask) == mask {
			num++
			mask = mask >> 1
		} else {
			break
		}
	}
	return num
}
func isUtf8(data []byte) bool {
	i := 0
	for i < len(data) {
		if (data[i] & 0x80) == 0x00 {
			// 0XXX_XXXX
			i++
			continue
		} else if num := preNUm(data[i]); num > 2 {
			// 110X_XXXX 10XX_XXXX
			// 1110_XXXX 10XX_XXXX 10XX_XXXX
			// 1111_0XXX 10XX_XXXX 10XX_XXXX 10XX_XXXX
			// 1111_10XX 10XX_XXXX 10XX_XXXX 10XX_XXXX 10XX_XXXX
			// 1111_110X 10XX_XXXX 10XX_XXXX 10XX_XXXX 10XX_XXXX 10XX_XXXX
			// preNUm() 返回首个字节的8个bits中首个0bit前面1bit的个数，该数量也是该字符所使用的字节数
			i++
			for j := 0; j < num-1; j++ {
				//判断后面的 num - 1 个字节是不是都是10开头
				if (data[i] & 0xc0) != 0x80 {
					return false
				}
				i++
			}
		} else {
			//其他情况说明不是utf-8
			return false
		}
	}
	return true
}

/*预处理从不同文件读入的波形数据*/
func TypeRead(ftype string, src multipart.File, parsing Parsing) (info string, data []byte, err error) {
	fileSuffix := path.Ext(ftype)
	fileNameWithOutSuffix := strings.TrimSuffix(ftype, fileSuffix)
	if fileSuffix == ".txt" {
		return ReadTXTfile(fileNameWithOutSuffix, src, parsing)
	} else if fileSuffix == ".csv" {
		return ReadCSVfile(fileNameWithOutSuffix, src, parsing)
	} else {
		return ReadEXCELfile(fileNameWithOutSuffix, src, parsing)
	}
}

// 从excel xlsx读入
func ReadEXCELfile(fileName string, file multipart.File, parsing Parsing) (info string, data []byte, err error) {
	reader, err := excelize.OpenReader(file)
	if err != nil {
		return info, nil, err
	}
	rows, err := reader.GetRows(reader.GetSheetName(0))
	if err != nil {
		return info, nil, err
	}
	switch parsing.Type {
	case 0:
		info = rows[0][0]
	case 1:
		info = fileName
	}
	var buffer bytes.Buffer
	for k, v := range rows {
		if k > 0 {
			bdata := []byte(strings.TrimRight(v[0], "0") + " ")
			buffer.Write(bdata)
		}
	}
	return info, buffer.Bytes(), nil
}

// 从csv读入
func ReadCSVfile(fileName string, file multipart.File, parsing Parsing) (info string, data []byte, err error) {
	reader := csv.NewReader(file)
	switch parsing.Type {
	case 0:
		reader.LazyQuotes = true
		infor, _ := reader.Read()
		if len(infor) == 0 {
			return info, nil, errors.New("无数据信息。")
		}
		if isUtf8([]byte(infor[0])) {
			info = strings.TrimSpace(infor[0])
		} else {
			info = strings.TrimSpace(ConvertGBK2Str(infor[0]))
		}
	case 1:
		info = fileName
	}
	if err != nil {
		return info, nil, err
	}
	var buffer bytes.Buffer
	for {
		csvdata, err := reader.Read() // 按行读取数据,可控制读取部分
		if err == io.EOF {
			break
		}
		bdata := []byte(strings.TrimRight(csvdata[0], "0") + " ")
		buffer.Write(bdata)
	}

	return info, buffer.Bytes(), nil
}

// 从txt读取数据，其中数据按空格分开（按文件的约定）
func ReadTXTfile(fileName string, file multipart.File, parsing Parsing) (info string, data []byte, err error) {
	reader := bufio.NewReader(file)

	// type 为 0 表示首行解析， 1 表示文件名解析
	switch parsing.Type {
	case 0:
		info, err = reader.ReadString('\n')
		fmt.Println(info)
	case 1:
		info = fileName
		fmt.Println(fileName)
	}

	//编码转换
	if isUtf8([]byte(info)) {
		info = strings.TrimSpace(info)
	} else {
		info = strings.TrimSpace(ConvertGBK2Str(info))
	}

	if err != nil {
		return info, nil, err
	}

	//* 处理数据位数，依次写进buffer，最后存进数据库
	var buffer bytes.Buffer
	for {
		strTmp, err := reader.ReadString(' ')
		if err != nil {
			if err == io.EOF {
				strTmp = strings.TrimRight(strTmp, " ")
				strTmp = strings.TrimRight(strTmp, "0")
				buffer.Write([]byte(strTmp + " "))
				break
			}
			break
		}
		strTmp = strings.TrimRight(strTmp, " ")
		strTmp = strings.TrimRight(strTmp, "0")
		buffer.Write([]byte(strTmp + " "))
	}
	return info, buffer.Bytes(), err
}

// eg: 约定字段的索引：风场名称(0) 风机名称(1)  测点名称(2) 数据长度(3) 采样频率(4) 数据类型(5) 测量参数(6) 转速(7) 时间(8) 其他信息(9)
// eg: 实际info：大唐江西太阳山风电场（0）_风机#01（1）_齿轮箱低速轴（2#测点）_径向（9）_32.768K（3）_25600HZ（4）_Timewave（5）_加速度（6）_1RPM（7）_20220101201122（8）
// eg: 定义info：0_1_2_9_3_4_5_6_7_8
// @Title DataInfoGet
// @Description 解析数据描述信息，将数据信息和数据存入数据库
// @Author MuXi 2023-12-15 15:50:30
// @Param db
// @Param info 文件info信息
// @Param filedata 文件数据
// @Param parsing 解析方式
// @Return error
func (dd *Data) DataInfoGet(db *gorm.DB, info string, filedata []byte, parsing Parsing) error {
	var err error
	var dataInfo DataInfo
	fmt.Println(parsing.Separator)
	// 实际info为数据格式，在文件中读取。
	str := strings.Split(info, parsing.Separator)
	if len(str) != parsing.Length {
		err = errors.New("文件解析格式出现错误")
		return err
	}
	// 分割解析方式中的定义info
	// 遍历定义info，当前字符的下标为key，value为约定好的字段编号，例如：下表：0， value：3则表示实际info中，下标为0的字段对应的测点
	infoIndexs := strings.Split(parsing.DataInfo, parsing.Separator)
	//切割后，将infoindexs 中的每个元素前后空格删除
	// TODO 遍历每个字段并删除前后空格
	for key, value := range infoIndexs {
		switch value {
		case "0":
			dataInfo.Windfarm = strings.TrimSpace(str[key])
		case "1":
			dataInfo.Machine = strings.TrimSpace(str[key])
		case "2":
			dataInfo.Point = strings.TrimSpace(str[key])
		case "3":
			dataInfo.Length = strings.TrimSpace(str[key])
		case "4":
			dataInfo.SampleRate = strings.TrimSpace(str[key])
		case "5":
			dataInfo.DataType = strings.TrimSpace(str[key])
		case "6":
			dataInfo.Parameter = strings.TrimSpace(str[key])
		case "7":
			dataInfo.Rpm = strings.TrimSpace(str[key])
		case "8":
			dataInfo.Time = strings.TrimSpace(str[key])
		case "9":
			dataInfo.Other = strings.TrimSpace(str[key])
		}
	}
	wname := dataInfo.Windfarm
	mname := dataInfo.Machine
	ppname := dataInfo.Point
	pdirection := dataInfo.Other

	//根据文件名找到测点id并关联
	midDB := db.Table("windfarm").
		Select("point.ID AS ID", "point.name AS name").
		Joins("join machine on windfarm.uuid = machine.windfarm_uuid").
		Joins("join part on machine.uuid = part.machine_uuid").
		Joins("join point on part.uuid = point.part_uuid").
		Where("windfarm.name = ?", wname).
		Where("machine.name = ?", mname)

	var goalpoint Point
	// 其它信息， 可能为空，需要动态拼接查询条件
	if pdirection == "" {
		err = midDB.Where("point.name = ?", ppname).First(&goalpoint).Error
	} else {
		err = midDB.Where("point.name = ? AND point.direction = ?", ppname, pdirection).First(&goalpoint).Error
	}
	if err != nil {
		err = errors.New("point missing." + err.Error())
		return err
	}

	// 根据参数获取数据所需相关参数，填充。
	var pointid uint = goalpoint.ID
	dd.PointID = goalpoint.ID
	//uuid
	var p Point
	err = db.Table("point").Where("id=?", pointid).Select("uuid").First(&p).Error
	if err != nil {
		return err
	}
	dd.PointUUID = p.UUID
	dd.Filepath = info
	dd.Length = strings.ToUpper(dataInfo.Length)
	// freq, err := strconv.ParseFloat(strings.Trim(strings.ToUpper(str[5]), "HZ"), 32)
	// 将hz 大写后移除HZ，在进行分割，保留采样频率整数
	freq, err := strconv.Atoi(strings.Split(strings.Trim(strings.ToUpper(dataInfo.SampleRate), "HZ"), ".")[0])
	if err != nil {
		return errors.New("频率格式错误" + err.Error())
	}
	dd.SampleFreq = freq
	dd.Datatype = strings.ToUpper(dataInfo.DataType)
	dd.Measuredefine = dataInfo.Parameter
	rpm, err := strconv.ParseFloat(strings.Trim(dataInfo.Rpm, "RPM"), 32)
	if err != nil {
		return errors.New("转速格式错误")
	}
	dd.Rpm = float32(rpm)
	if len([]rune(dataInfo.Time)) != 14 {
		return errors.New("时间格式错误，应包含年月日时分秒14位数。")
	}
	ddtime, err := time.ParseInLocation("20060102150405", dataInfo.Time, time.Local)
	if err != nil {
		return err
	}
	dd.Time = ddtime.Format("2006-01-02 15:04:05")
	dd.TimeSet = ddtime.Unix()
	//数据解析到float32
	dd.Wave.File = filedata
	var originy []float32 = make([]float32, 0)
	origin := strings.Trim(string(dd.Wave.File), " ")
	onum := strings.Split(origin, " ")
	dd.Wave.DataString = origin
	for _, v := range onum {
		temp, _ := strconv.ParseFloat(v, 32)
		originy = append(originy, float32(temp))
	}
	dd.Wave.DataFloat, err = Encode(originy)
	if err != nil {
		return err
	}
	return nil
}

// * 调用分析算法exe  windows
func (dbconfig *GormConfig) DataAnalysis(path string, arg ...string) ([]string, error) {
	//分析
	// cmd := exec.Command(path, dbconfig.Addr, dbconfig.Admin, dbconfig.Password, dbconfig.Schema, dbconfig.Port,datatable, resulttable, dataid, arg, shmname)
	arg2 := []string{dbconfig.Addr, dbconfig.Admin, dbconfig.Password, dbconfig.Schema, dbconfig.Port}
	arg2 = append(arg2, arg...)
	cmd := exec.Command(path, arg2...)

	var str []string
	buf, err := cmd.Output()
	//两种系统换行处理
	message := strings.Replace(strings.Trim(string(buf), "\r\n"), "\r\n", "\n", -1)
	messages := strings.Split(message, "\n")
	//按行读取bufstring 最后一行为succeed即为成功，否则为失败
	if messages[len(messages)-1] == "succeed" {
		return messages, nil
	}
	return str, err
}

// 数据服务获取时频分析数据并存到数据库
// 将上传的数据提交给数据分析服务，返回特征值，以及分析结果
// 分析结果存入数据库，将特征值更新在数据表中。
func (data *Data) DataAnalysis_2(db *gorm.DB, ipport string, fid string) (err error) {
	ourl := "http://" + ipport + "/api/v1/data/trans/"
	var originy []float32 = make([]float32, len(data.Wave.DataFloat)/4)
	err = Decode(data.Wave.DataFloat, &originy)
	if err != nil {
		return err
	}
	var url string = ourl + "4"
	type DataPost4 struct {
		Datafloat []float32 `json:"data"`
		Freq      int       `json:"fs"`
		BV1       string    `json:"bv1,omitempty"` //格式：最小值 最大值。中间空格隔开，如：0 100
		BV2       string    `json:"bv2,omitempty"`
		BV3       string    `json:"bv3,omitempty"`
		BV4       string    `json:"bv4,omitempty"`
		BV5       string    `json:"bv5,omitempty"`
		BV6       string    `json:"bv6,omitempty"`
		Databack  []float32 `json:"result"`
		Result
		Code    int    `json:"code"`
		Message string `json:"message,omitempty"`
	}
	postData := DataPost4{
		Datafloat: originy,
		Freq:      data.SampleFreq,
		BV1:       data.BandValue1,
		BV2:       data.BandValue2,
		BV3:       data.BandValue3,
		BV4:       data.BandValue4,
		BV5:       data.BandValue5,
		BV6:       data.BandValue6,
	}
	postBody, err := json.Marshal(postData)
	fmt.Println(postBody)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(postBody))
	if err != nil {
		return err
	}
	// 读取响应内容
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&postData)
	if err != nil {
		return err
	}
	if postData.Code != 200 {
		return errors.New("数据分析服务错误" + postData.Message)
	}
	if len(postData.Databack) == 0 {
		return errors.New("无数据返回")
	}
	data.Wave.SpectrumFloat, err = Encode(postData.Databack)
	data.Result = postData.Result
	if err != nil {
		return err
	}
	return
}

//	func AlertSearch(db *gorm.DB, ppmwcid []string, pointuuid string, measuredefine string) ([]alert.Band, error) {
//		//频带查询
//		var bband []alert.Band
//		err := db.Transaction(func(tx *gorm.DB) error {
//			//获取警报版本 测点信息和部件类型
//			var parttype string //部件名称
//			//如果有type字段
//			if err := tx.Table("part").Where("id=?", ppmwcid[1]).Select("name").Scan(&parttype).Error; err != nil {
//				return err
//			}
//			//从警报标准表获取标准值
//			if err := tx.Table("band").Where("type=? AND value=?", parttype, measuredefine).
//				Find(&bband).Error; err != nil {
//				return err
//			}
//			return nil
//		})
//		return bband, err
//	}
func AlertSearch(db *gorm.DB, ppmwcid []string, pointuuid string, measuredefine string) ([]alert.Band, error) {
	//频带查询
	var bband []alert.Band
	var bbandpoint []alert.Band //测点的频带报警
	var bbandpart []alert.Band  //部件的频带报警
	err := db.Transaction(func(tx *gorm.DB) error {
		//获取警报版本 测点信息和部件类型
		var partuuid string //部件名称
		//从警报标准表获取标准值
		tx.Table("part").
			Joins("join point on part.uuid=point.part_uuid").
			Where("point.uuid=?", pointuuid).
			Pluck("part.uuid", &partuuid)
		if err := tx.Table("band").
			Where("point_uuid=? AND value=?", pointuuid, measuredefine).
			Find(&bbandpoint).Error; err != nil {
			return err
		}
		//每个point band判断有无重复的part band，记录id。再查询part band，排除这些id。
		var outids []uint = []uint{0}
		for k := range bbandpoint {
			var outid uint
			tx.Table("band").
				Where("part_uuid =?", partuuid).
				Where("value =? AND band_range =? AND property =?", measuredefine, bbandpoint[k].Range, bbandpoint[k].Property).
				Pluck("id", &outid)
			if outid != 0 {
				outids = append(outids, outid)
			}
		}
		if err := tx.Table("band").Where("part_uuid=? AND value=?", partuuid, measuredefine).
			Where("id NOT IN ?", outids).
			Find(&bbandpart).Error; err != nil {
			return err
		}
		bband = append(bbandpoint, bbandpart...)
		return nil
	})
	return bband, err
}
func BandUpdate(db *gorm.DB, pdata *Data, bband []alert.Band) {
	//将频带更新至data
	databv := make(map[string]string)
	for k, v := range bband {
		vr := strings.Split(v.Range, " ")
		if len(vr) == 2 {
			var newrange string
			vr1, _ := strconv.Atoi(vr[1])
			if vr1 < pdata.SampleFreq/2 { //! 频带右<1/2采样频率 否则频带右=采样频率/2
				newrange = v.Range
			} else {
				newrange = vr[0] + " " + strconv.FormatFloat(float64(pdata.SampleFreq)/2, 'f', 1, 64)
			}
			//TODO 解析到对应结构体
			databv[fmt.Sprintf("bv%v", k+1)] = newrange
		}

	}
	// var edata Data
	MaptoStruct(databv, pdata)
}

// *  to byte编码
func Encode(src interface{}) ([]byte, error) {
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, src); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// *  to byte解码，定义时候需先确定长度
func Decode(b []byte, dst interface{}) error {
	buf := bytes.NewBuffer(b)
	if err := binary.Read(buf, binary.LittleEndian, dst); err != nil {
		return err
	}
	return nil
}
