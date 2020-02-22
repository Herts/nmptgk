package main

import (
	"github.com/jinzhu/gorm"
)

type TRawData struct {
	gorm.Model
	Lang             string
	Math             string
	Combination      string
	SecLang          string
	Speaking         string
	Listening        string
	StudentNum       string `gorm:"primary_key"`
	StudentName      string
	LangScore        float64 `gorm:"type:decimal(10,2)"`
	MathScore        float64 `gorm:"type:decimal(10,2)"`
	CombinationScore float64 `gorm:"type:decimal(10,2)"`
	SecLangScore     float64 `gorm:"type:decimal(10,2)"`
	SpeakingScore    float64 `gorm:"type:decimal(10,2)"`
	ListeningScore   float64 `gorm:"type:decimal(10,2)"`
	TotalScore       float64 `gorm:"type:decimal(10,2)"`
	Date             string
}

type TAdmissionData struct {
	gorm.Model
	StudentNum  string `gorm:"primary_key,unique"`
	StudentName string
	School      string
	Major       string
	ADType      string
	ADMethod    string
}
