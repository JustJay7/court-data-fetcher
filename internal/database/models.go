package database

import (
	"time"

	"gorm.io/gorm"
)

type QueryLog struct {
	gorm.Model
	CaseType     string    `json:"case_type"`
	CaseNumber   string    `json:"case_number"`
	FilingYear   string    `json:"filing_year"`
	RawResponse  string    `json:"raw_response" gorm:"type:text"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message"`
	QueryTime    time.Time `json:"query_time"`
	IPAddress    string    `json:"ip_address"`
}

type CaseInfo struct {
	gorm.Model
	QueryLogID    uint      `json:"query_log_id"`
	CaseNumber    string    `json:"case_number" gorm:"index"`
	CaseType      string    `json:"case_type"`
	FilingYear    string    `json:"filing_year"`
	FilingDate    time.Time `json:"filing_date"`
	NextHearing   time.Time `json:"next_hearing"`
	Status        string    `json:"status"`
	Judge         string    `json:"judge"`
	CourtComplex  string    `json:"court_complex"`
	Parties       []Party   `json:"parties" gorm:"foreignKey:CaseInfoID"`
	Orders        []Order   `json:"orders" gorm:"foreignKey:CaseInfoID"`
}

type Party struct {
	gorm.Model
	CaseInfoID    uint   `json:"case_info_id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	AdvocateName  string `json:"advocate_name"`
	AdvocateCode  string `json:"advocate_code"`
}

type Order struct {
	gorm.Model
	CaseInfoID   uint      `json:"case_info_id"`
	OrderDate    time.Time `json:"order_date"`
	Description  string    `json:"description"`
	PDFLink      string    `json:"pdf_link"`
	OrderType    string    `json:"order_type"`
	JudgeName    string    `json:"judge_name"`
	Downloaded   bool      `json:"downloaded"`
	LocalPath    string    `json:"local_path"`
}

func (QueryLog) TableName() string {
	return "query_logs"
}

func (CaseInfo) TableName() string {
	return "case_infos"
}

func (Party) TableName() string {
	return "parties"
}

func (Order) TableName() string {
	return "orders"
}