package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"mvdan.cc/xurls/v2"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TaskCampaign struct {
	gorm.Model
	CampaignId       uint
	Tasks            []Task
	ExecutedTasks    []ExecutedTask
	Stats            TaskCampaignStats `gorm:"embedded"`
	FirstEmailOffset uint
}

type TaskLead struct {
	FirstName        string
	LastName         string
	Email            string
	PersonalizedLine string
	CampaignID       uint
}
type TaskCampaignStats struct {
	TasksAmount uint
}
type TaskStats struct {
	Opened     bool
	EmailOpens uint
}

type Task struct {
	gorm.Model
	ExecutionTime  time.Time
	EmailNumber    uint
	ThreadId       string
	UserId         uint
	TaskCampaignID uint
	Lead           TaskLead `gorm:"embedded"`
	CampaignId     uint
}

type ExecutedTask struct {
	gorm.Model
	CampaignId     uint
	UserId         uint
	Lead           TaskLead `gorm:"embedded"`
	ThreadId       string
	MessageId      string
	EmailNumber    uint
	TaskCampaignID uint
	TaskStats      `gorm:"embedded"`
}

func scheduleTask(taskdb *gorm.DB, task Task) {
	taskdb.Create(&task)
}
func rescheduleTask(taskdb *gorm.DB, task *Task, duration time.Duration) {
	taskdb.Model(&task).Update("execution_time", task.ExecutionTime.Add(duration))
}
func executeTask(db *gorm.DB, taskdb *gorm.DB, task *Task) {

	defer func() {
		if r := recover(); r != nil {
			err := r.(error)
			fmt.Println(err)
			//if error happens we need to somehome tell the user they need to get de activated
			var user User
			user.ID = task.UserId
			db.First(&user)
			user.GmailActivated = false
			db.Save(&user)
			//and we need to notify them by sending them the link
			//if auth goes out
			//on email somehow
		}
	}()

	var campaign Campaign
	db.Preload("EmailSequence.Emails").Preload(clause.Associations).Where("id = ?", task.CampaignId).First(&campaign)
	var user User
	user.ID = task.UserId
	db.First(&user)
	// fmt.Print(campaign.Stats.StepStats)

	campaign.Stats.StepStats[task.EmailNumber].SentEmails += 1

	// fmt.Println("TASK ID:", task.ID)
	//check for rate limit
	if user.EmailsSentTodayBuffer.UnLockDate.Before(time.Now()) && user.EmailsSentTodayBuffer.RateLimited {
		user.EmailsSentTodayBuffer.RateLimited = false
		user.EmailsSentTodayBuffer.EmailsSentToday = 0
	}

	if user.EmailsSentTodayBuffer.RateLimited {
		fmt.Println("Rate Limit Hit : Rescheduling in 24hrs")
		rescheduleTask(taskdb, task, 24*time.Hour)
		return
	}

	v := campaign.EmailSequence.Emails[task.EmailNumber]
	eT := ExecutedTask{
		CampaignId: campaign.ID,
		UserId:     task.UserId,
	}
	taskdb.Create(&eT)

	email := Email{
		Subject: personalizeSubjectLineFromLeadAndTemplate(v.Subject, &task.Lead),
		From:    user.Email,
		To:      task.Lead.Email,
		Body:    personalizeEmailBodyFromLeadAndTemplate(v.Body, &task.Lead),
	}

	var m_id string
	var threadId string
	var thread *gmail.Thread

	srv := getGmailService(&user.GmailAccessToken)

	pixel := makePixel(user.ID, campaign.ID, eT.ID, task.EmailNumber)

	unsubscribeLink := makeUnsubscribeLink(user.ID, campaign.ID, eT.ID, task.EmailNumber)
	rxStrict := xurls.Strict()
	res := rxStrict.FindAllString(email.Body, -1)
	for _, v := range res {
		if !strings.Contains(email.Body, `src="`+v+`"`) && !strings.Contains(email.Body, `src='`+v+`'`) && !strings.Contains(email.Body, `>`+v+`<`) {
			trackingLink := makeTrackingLink(v, user.ID, campaign.ID, eT.ID, task.EmailNumber)
			email.Body = replaceLinkWithTrackingLink(v, trackingLink, &email)
		}

	}

	if len(task.ThreadId) > 1 {

		thread, _ = srv.Users.Threads.Get("me", task.ThreadId).Do()
		//thread.HistoryId
		if len(thread.Messages) == int(task.EmailNumber) {
			threadId = thread.Id
			sendMessageToThread(srv, thread, &email, pixel, user.UPREACHLABELID, unsubscribeLink)
		}
	} else {
		m_id = sendMessage(srv, &email, pixel, user.UPREACHLABELID, unsubscribeLink)
		message, _ := srv.Users.Messages.Get("me", m_id).Format("minimal").Do()
		threadId = message.ThreadId

		srv.Users.Threads.Modify("me", message.ThreadId, &gmail.ModifyThreadRequest{
			AddLabelIds: []string{user.UPREACHLABELID},
		}).Do()
	}
	eT.EmailNumber = task.EmailNumber
	eT.MessageId = m_id
	eT.ThreadId = threadId
	eT.Lead = task.Lead

	campaign.Stats.EmailsSent += 1
	user.Stats.EmailsSent += 1
	user.EmailsSentTodayBuffer.EmailsSentToday += 1

	//db.Save(&campaign)
	db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&campaign)
	taskdb.Save(&eT)
	rateLimitOffset := time.Hour * 0

	if user.EmailsSentTodayBuffer.EmailsSentToday == user.Settings.EmailsPerDay && !user.EmailsSentTodayBuffer.RateLimited {
		user.EmailsSentTodayBuffer.UnLockDate = time.Now().Add(time.Hour * 24)
		user.EmailsSentTodayBuffer.RateLimited = true
		rateLimitOffset = time.Hour * 24
	}

	db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&user)
	if len(campaign.EmailSequence.Emails) > int(task.EmailNumber+1) {
		fmt.Println(rateLimitOffset)
		scheduleTask(taskdb, Task{
			ExecutionTime:  time.Now().Add(time.Duration(campaign.EmailSequence.Emails[task.EmailNumber+1].TimeOffset)).Add(rateLimitOffset),
			TaskCampaignID: campaign.TaskCampaignID,
			EmailNumber:    task.EmailNumber + 1,
			ThreadId:       threadId,
			UserId:         user.ID,
			CampaignId:     task.CampaignId,
			Lead:           task.Lead,
		})
	}
	clearTask(taskdb, task)

}
func makeUnsubscribeLink(userId uint, campaignId uint, pixelId uint, emailno uint) string {
	base := os.Getenv("TRACKINGBASEURL") + "unsubscribe/"
	user := fmt.Sprint(userId)
	campaign := fmt.Sprint(campaignId)
	pixel := user + "/" + campaign + "/" + fmt.Sprint(emailno) + "/" + fmt.Sprint(pixelId)
	return `<div>Don't Want to recieve any more messages ? <a href="` + base + pixel + `"> unsubscribe</a></div>`
}
func replaceLinkWithTrackingLink(linkToReplace string, trackingLink string, email *Email) string {
	htmlLink := `<a href="` + trackingLink + `">` + linkToReplace + `</a>`

	return strings.Replace(email.Body, linkToReplace, htmlLink, 1)
}
func makeTrackingLink(link string, userId uint, campaignId uint, pixelId uint, emailno uint) string {
	base := os.Getenv("TRACKINGBASEURL") + "links/"
	encodedStr := url.QueryEscape(link)
	user := fmt.Sprint(userId)
	campaign := fmt.Sprint(campaignId)
	tracker := user + "/" + campaign + "/" + fmt.Sprint(emailno) + "/" + fmt.Sprint(pixelId)
	fmt.Println(base + tracker + "?url=" + encodedStr)
	return base + tracker + "?url=" + encodedStr
}
func clearTask(taskdb *gorm.DB, task *Task) {
	taskdb.Delete(&task)
}

func openChronDB() *gorm.DB {
	db, _ := gorm.Open(sqlite.Open("./db/tasks.db"), &gorm.Config{})
	db.AutoMigrate(&Task{}, &ExecutedTask{}, &TaskCampaign{})
	return db
}

func startCampaign(campaign *Campaign, user *User, db *gorm.DB, taskdb *gorm.DB) {
	campaign.Status = "started"
	tC := taskCampaignFromCampaign(campaign)
	taskdb.Create(&tC)
	campaign.TaskCampaignID = tC.ID
	db.Save(&campaign)
	scheduleTaskCampaign(&tC, campaign, taskdb, user.Settings.EmailTimeOffset)
}

func scheduleTaskCampaign(tc *TaskCampaign, c *Campaign, taskdb *gorm.DB, offset uint) {

	for leadIndex := range c.Leads {
		execTime := time.Now().Add(time.Duration((int(leadIndex) * int(offset*1e+9)))).Add(time.Duration(c.FirstEmailOffset * 1e+9))
		task := Task{
			ExecutionTime:  execTime,
			EmailNumber:    uint(0),
			UserId:         c.UserID,
			TaskCampaignID: c.TaskCampaignID,
			CampaignId:     c.EmailSequence.CampaignID,
			Lead:           leadToTaskDB(&c.Leads[leadIndex]),
		}
		tc.Tasks = append(tc.Tasks, task)
	}

	tc.Stats.TasksAmount = uint(len(tc.Tasks))
	taskdb.Save(&tc)
}

func taskCampaignFromCampaign(campaign *Campaign) TaskCampaign {
	return TaskCampaign{
		CampaignId:       campaign.ID,
		Tasks:            []Task{},
		ExecutedTasks:    []ExecutedTask{},
		Stats:            TaskCampaignStats{},
		FirstEmailOffset: campaign.FirstEmailOffset,
	}
}

// RUNS EVERY X TO HANDLE JOBS
// EXECUTES TASKS FROM THE DATABASE
func executionChron(appDb *gorm.DB, taskDb *gorm.DB) {
	var tasks []Task
	taskDb.Where("execution_time < ?", time.Now()).Find(&tasks)
	fmt.Println("EXECUTING TASKS")
	for _, t := range tasks {
		fmt.Println(time.Since(t.ExecutionTime))
		//batch execute TASKS IS BETTER FOR API USAGE
		executeTask(appDb, taskDb, &t)
	}
}
func statsChron(appDb *gorm.DB, taskDb *gorm.DB) {
	fmt.Println("GETTING STATS")
	var users []User
	appDb.Where("gmail_activated = true", time.Now()).Find(&users)

	for _, u := range users {
		srv := getGmailService(&u.GmailAccessToken)

		profile, _ := srv.Users.GetProfile("me").Do()

		fmt.Println(u.LASTSYNCTIME.Unix())
		if u.LASTSYNCTIME.Unix() < 0 {
			// case if it is unitialized
			u.LASTSYNCTIME = time.Now().Add(-time.Minute)
		}

		qs := "after:" + fmt.Sprint(u.LASTSYNCTIME.Unix()) + " to:" + profile.EmailAddress
		//fmt.Println(qs)
		u.LASTSYNCTIME = time.Now()

		threads, _ := srv.Users.Messages.List("me").LabelIds("INBOX").Q(qs).Do()

		var executedTask ExecutedTask

		for _, i := range threads.Messages {

			taskDb.Order("email_number DESC").First(&executedTask, "thread_id = ?", i.ThreadId)

			if executedTask.CampaignId != 0 {
				// use the campaign id to update replies
				var campaign Campaign

				campaign.ID = executedTask.CampaignId
				appDb.Preload(clause.Associations).Find(&campaign)

				campaign.Stats.Replies += 1
				campaign.Stats.ReplyRate = uint((float64(campaign.Stats.Replies) / float64(campaign.Stats.LeadsAmount)) * 100)
				u.Stats.Replies += 1

				campaign.Stats.StepStats[executedTask.EmailNumber].Replies += 1
				campaign.Stats.StepStats[executedTask.EmailNumber].ReplyRate = uint(float64(campaign.Stats.StepStats[executedTask.EmailNumber].Replies) / float64(campaign.Stats.StepStats[executedTask.EmailNumber].SentEmails) * 100)

				appDb.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&campaign)

				fmt.Println("ADDING TO ", campaign.Stats.Replies)
				taskDb.Where("thread_id = ?", i.ThreadId).Delete(&Task{})
			}

		}

		appDb.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&u)

	}

}

func getGmailService(tok *oauth2.Token) *gmail.Service {
	ctx := context.Background()
	b, _ := os.ReadFile("./credentials/credentials.json")
	config, _ := google.ConfigFromJSON(b, gmail.MailGoogleComScope)
	// //JUST FOR QUICK TESTING

	client := config.Client(context.Background(), tok)

	srv, _ := gmail.NewService(ctx, option.WithHTTPClient(client))
	return srv
}
func leadToTaskDB(l *Lead) TaskLead {
	return TaskLead{
		FirstName:        l.FirstName,
		LastName:         l.LastName,
		Email:            l.Email,
		PersonalizedLine: l.PersonalizedLine,
		CampaignID:       l.CampaignID,
	}
}
