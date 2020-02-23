package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/anaskhan96/soup"
	"github.com/axgle/mahonia"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jinzhu/gorm"
	"github.com/parnurzeal/gorequest"
	"github.com/spf13/viper"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"time"
)

func main() {
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		log.Fatal(err)
	}
	dbLink := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true", viper.GetString("db.username"),
		viper.GetString("db.password"), viper.GetString("db.url"),
		viper.GetString("db.port"), viper.GetString("db.database"))

	db, err := gorm.Open("mysql", dbLink)
	if err != nil {
		log.Fatal(err)
	}
	db.LogMode(viper.GetBool("db.debug"))

	var start, step, year int
	var auto bool
	var mode string
	flag.BoolVar(&auto, "auto", false, "auto mode")
	flag.IntVar(&step, "step", viper.GetInt("step"), "step for crawling")
	flag.IntVar(&start, "start", viper.GetInt("start"), "starting student number")
	flag.IntVar(&year, "year", viper.GetInt("year"), "year for GAOKAO")
	flag.StringVar(&mode, "mode", "s", "a for admission, s for score")
	flag.Parse()
	switch mode {
	case "a":
		AutoMultipleAdDataByStep(db, step)
	case "s":
		if auto {
			f, err := os.Open("IDPrefix.json")
			if err != nil {
				log.Fatal(err)
			}
			m := make(map[string]string)
			err = json.NewDecoder(f).Decode(&m)
			if err != nil {
				log.Fatal(err)
			}
			for prefix, city := range m {
				idPrefix := MustParseInt(prefix)
				fmt.Println(city)
				AutoMultipleRawData(year, idPrefix, db)
			}
		} else {
			MultipleRawDataByStepLimitSize(step, start, db)
		}
	}
}

func AutoMultipleAdDataByStep(db *gorm.DB, step int) {
	size := 100
	for i := 0; i < step; i += size {
		var noAdDatas []*VNoAd
		db.Limit(size).Find(&noAdDatas)
		var rawDatas []*TRawData
		for _, noAd := range noAdDatas {
			rawDatas = append(rawDatas, &TRawData{StudentNum: noAd.StudentNum, StudentName: noAd.StudentName})
		}

		MultipleAdDataByRawData(len(rawDatas), rawDatas, db)
	}
}

func AutoMultipleRawData(year int, idPrefix int, db *gorm.DB) {
	nums := []int{11, 13, 15}
	for _, num := range nums {
		start := MustParseInt(fmt.Sprintf("%d%d%d", year, idPrefix, num)) * 10000
		step := 2000
		count := -1
		for count != 0 {
			count = MultipleRawDataByStepLimitSize(step, start, db)
			start += step
		}
	}
}

func MultipleADData(db *gorm.DB) {
	var adData TAdmissionData
	db.Last(&adData)

	var data TRawData
	db.Where(&TRawData{StudentNum: adData.StudentNum}).First(&data)

	size := 100
	id := data.ID
	MultipleAdDatasById(db, id, size)
}

func MultipleAdDatasById(db *gorm.DB, id uint, size int) int {
	var rawDatas []*TRawData
	db.Where("id > ?", id).Limit(size).Find(&rawDatas)

	return MultipleAdDataByRawData(size, rawDatas, db)
}

func MultipleAdDataByRawData(size int, rawDatas []*TRawData, db *gorm.DB) int {
	var adDatas = make([]*TAdmissionData, size)
	wg := sync.WaitGroup{}
	wg.Add(size)
	for i := range rawDatas {
		go func(i int, group *sync.WaitGroup) {
			adDatas[i] = AdDataByData(rawDatas[i])
			group.Done()
		}(i, &wg)
	}
	wg.Wait()
	count := 0
	for _, adData := range adDatas {
		if adData != nil {
			db.Create(adData)
			count++
		}
	}
	return count
}

func AdDataByData(data *TRawData) *TAdmissionData {
	studentNum := data.StudentNum
	studentName := data.StudentName
	adData := AdData(studentNum, studentName)
	return adData
}

func AdDataByNum(db *gorm.DB, num string) {
	var data TRawData
	db.Where(&TRawData{StudentNum: num}).First(&data)
	studentNum := data.StudentNum
	studentName := data.StudentName
	adData := AdData(studentNum, studentName)
	if adData == nil {
		return
	}
	db.Create(adData)
}

func AdData(studentNum string, studentName string) *TAdmissionData {
	req := gorequest.New()
	_, body, errs := req.Post("https://www1.nm.zsks.cn/xxcx/gkcx/gklqcx.jsp").
		Set("Referer", "https://www1.nm.zsks.cn/xxcx/gkcx/gklqcx.jsp").
		Type("form").
		Send(fmt.Sprintf("v_ksh=%s", studentNum)).
		Send(fmt.Sprintf("v_xm=%s", studentName)).
		Send("query=查  询").
		Retry(2, 2*time.Second, http.StatusBadRequest, http.StatusInternalServerError).
		Timeout(1 * time.Second).
		End()
	if errs != nil {
		log.Println(errs)
		return AdData(studentNum, studentName)
	}

	doc := soup.HTMLParse(body)
	ps := doc.FindAll("p")
	var record []string
	for _, p := range ps {
		record = append(record, p.Text())
	}

	if len(record) < 5 {
		regex := regexp.MustCompile("未找到对应考生")
		notAd := regex.FindString(body)
		if len(notAd) > 0 {
			return &TAdmissionData{
				StudentNum:  studentNum,
				StudentName: studentName,
				School:      "未录取",
			}
		}
		return nil
	}

	record = record[len(record)/2+1:]
	adData := &TAdmissionData{
		StudentNum:  record[0],
		StudentName: record[1],
		School:      record[2],
		Major:       record[3],
		ADType:      record[4],
		ADMethod:    record[5],
	}
	return adData
}

func MultipleRawData(db *gorm.DB, size int) {
	var data TRawData
	db.Order("student_num asc").First(&data)
	start, err := strconv.Atoi(data.StudentNum)
	if err != nil {
		log.Fatal(err)
	}

	MultipleRawDataByStep(size, start, db)
}

func MultipleRawDataByStepLimitSize(step, start int, db *gorm.DB) int {
	size := 50
	i := 0
	count := 0
	for i = 0; i < step/size; i++ {
		tmpStart := start + size*i
		num := MultipleRawDataByStep(size, tmpStart, db)
		count += num
		fmt.Printf("%d students in range from %d to %d\n", num, tmpStart, tmpStart+size)
	}
	count += MultipleRawDataByStep(step%size, start+size*(i-1), db)
	return count
}

func MultipleRawDataByStep(step, start int, db *gorm.DB) int {
	wg := sync.WaitGroup{}
	wg.Add(step)
	rawDatas := make([]*TRawData, step)
	for i := 1; i <= step; i++ {
		go func(i int, group *sync.WaitGroup) {
			stuNum := start + i
			rawData := GetScores(stuNum)
			if rawData == nil {
				wg.Done()
				return
			}
			rawDatas[i-1] = rawData
			wg.Done()
		}(i, &wg)
	}
	wg.Wait()

	count := 0
	for _, data := range rawDatas {
		if data != nil {
			db.Create(data)
			count++
		}
	}
	return count
}

func GetScores(stuNum int) *TRawData {
	_, body, errs := gorequest.New().Get(fmt.Sprintf("https://www1.nm.zsks.cn/query/gkcj_iframe.jsp?ksh=%d", stuNum)).
		Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/80.0.3987.116 Safari/537.36").
		Retry(1, 2*time.Second, http.StatusBadRequest, http.StatusInternalServerError).
		Timeout(1 * time.Second).
		End()
	if errs != nil {
		return GetScores(stuNum)
	}
	body = decoderConvert("gbk", body)
	doc := soup.HTMLParse(body)
	ps := doc.FindAll("p")
	var record []string
	for _, p := range ps {
		record = append(record, p.Text())
	}
	if len(record) == 0 {
		return nil
	}
	fmt.Println(record)
	//length := len(record)
	detail := record[len(record)/2:]
	record = record[:len(record)/2]
	lenOfCol := len(record)
	rawData := &TRawData{
		Lang:             record[2],
		Math:             record[3],
		Combination:      record[4],
		SecLang:          record[5],
		Listening:        record[lenOfCol-3],
		StudentNum:       detail[0],
		StudentName:      detail[1],
		LangScore:        MustParseFloat(detail[2]),
		MathScore:        MustParseFloat(detail[3]),
		CombinationScore: MustParseFloat(detail[4]),
		SecLangScore:     MustParseFloat(detail[5]),
		ListeningScore:   MustParseFloat(detail[lenOfCol-3]),
		TotalScore:       MustParseFloat(detail[lenOfCol-2]),
		Date:             detail[lenOfCol-1],
	}
	if lenOfCol > 9 {
		rawData.Speaking = record[6]
		rawData.SpeakingScore = MustParseFloat(detail[6])
	}
	return rawData
}

func decoderConvert(name string, body string) string {
	return mahonia.NewDecoder(name).ConvertString(body)
}

func MustParseFloat(num string) float64 {
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		log.Println(err)
		return 0
	}
	return f
}

func MustParseInt(num string) int {
	i, err := strconv.Atoi(num)
	if err != nil {
		log.Fatal(err)
	}
	return i
}
