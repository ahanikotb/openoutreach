package main

import (
	"time"

	"golang.org/x/oauth2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Campaign struct {
	gorm.Model
	TaskCampaignID   uint
	EmailSequence    EmailSequence
	Leads            []Lead
	TimeStarted      time.Time
	Status           string
	UserID           uint
	Stats            Stats `gorm:"embedded"`
	Name             string
	FirstEmailOffset uint
}
type Stats struct {
	Unsubscribes     uint
	Opens            uint
	Replies          uint
	SequenceLength   uint
	LeadsAmount      uint
	OpenRate         uint
	EmailsSent       uint
	ReplyRate        uint
	LinkClicks       uint
	ClickThroughRate uint
	StepStats        []StepStat
}
type StepStat struct {
	gorm.Model
	Unsubscribes     uint
	CampaignID       uint
	SentEmails       uint
	OpenedEmails     uint
	OpenRate         uint
	Replies          uint
	ReplyRate        uint
	LinkClicks       uint
	ClickThroughRate uint
}

type Lead struct {
	gorm.Model
	FirstName        string
	LastName         string
	Email            string
	PersonalizedLine string
	CampaignID       uint
}

type EmailSequence struct {
	gorm.Model
	Emails     []Email
	CampaignID uint
}

type Email struct {
	gorm.Model
	Subject         string
	From            string
	To              string
	Body            string
	TimeOffset      time.Duration
	EmailSequenceID uint
}

type User struct {
	gorm.Model
	Email                 string
	Password              string
	Settings              UserSettings `gorm:"embedded"`
	Campaings             []Campaign
	GmailAccessToken      oauth2.Token    `gorm:"embedded"`
	EmailsSentTodayBuffer EmailSentBuffer `gorm:"embedded"`
	UPREACHLABELID        string
	UPREACHLABELIDOUT     string
	GmailActivated        bool
	HISTORYID             uint
	LASTSYNCTIME          time.Time
	Stats                 UserStats `gorm:"embedded"`
}

type UserStats struct {
	Opens            uint
	Replies          uint
	OpenRate         uint
	EmailsSent       uint
	ReplyRate        uint
	LinkClicks       uint
	ClickThroughRate uint
	Unsubscribes     uint
}
type EmailSentBuffer struct {
	EmailsSentToday uint
	UnLockDate      time.Time
	RateLimited     bool
}
type UserSettings struct {
	EmailTimeOffset uint
	EmailsPerDay    uint
}

func openDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open("./db/test.db"), &gorm.Config{Logger: logger.Default})
	db.AutoMigrate(&User{}, &Campaign{}, &EmailSequence{}, &Email{}, &Lead{}, &StepStat{})
	db.Set("gorm:auto_preload", true)
	return db
}
