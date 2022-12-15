package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/api/gmail/v1"
	"gopkg.in/gomail.v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func checkThreadForUpdate(thread *gmail.Thread) bool {
	fmt.Println(len(thread.Messages))
	return false
}

func sendMessage(srv *gmail.Service, email *Email, pixel string, upreachOutLabel string, unsubscribeLink string) string {
	m := gomail.NewMessage()

	m.SetHeader("From", email.From)
	m.SetHeader("To", email.To)
	m.SetHeader("Subject", email.Subject)
	m.SetBody("text/html", email.Body+pixel+unsubscribeLink)
	buffer := new(bytes.Buffer)
	m.WriteTo(buffer)
	var msg gmail.Message
	msg.Raw = base64.URLEncoding.EncodeToString(buffer.Bytes())
	msg.LabelIds = append(msg.LabelIds, upreachOutLabel)
	// msg.LabelIds = append(msg.LabelIds)
	mId, _ := srv.Users.Messages.Send("me", &msg).Do()
	return mId.Id
}
func makePixel(userId uint, campaignId uint, pixelId uint, emailno uint) string {
	base := os.Getenv("TRACKINGBASEURL") + "pixels/"
	user := fmt.Sprint(userId)
	campaign := fmt.Sprint(campaignId)
	pixel := user + "/" + campaign + "/" + fmt.Sprint(emailno) + "/" + fmt.Sprint(pixelId) + ".png"
	return "<img src=\"" + base + pixel + "\"/>"
}
func getAllMessageIds(m []*gmail.Message) []string {
	var result []string

	for _, message := range m {

		for _, v := range message.Payload.Headers {
			// fmt.Println(v.Name)
			switch v.Name {
			case "Message-Id":
				result = append(result, v.Value)
			default:
				continue
			}
		}

	}
	return result
}

func sendMessageToThread(srv *gmail.Service, thread *gmail.Thread, email *Email, pixel string, upreachOutLabel string, unsubscribeLink string) string {
	_, _, subject, messageId := parseHeaders(thread.Messages[0].Payload.Headers)
	m := gomail.NewMessage()
	m.SetHeader("From", email.From)

	m.SetHeader("In-Reply-To", messageId)

	m.SetHeader("References", strings.Join(getAllMessageIds(thread.Messages), " "))
	m.SetHeader("To", email.To)
	m.SetHeader("Subject", subject)
	m.SetBody("text/html", email.Body+pixel+unsubscribeLink)

	buffer := new(bytes.Buffer)
	m.WriteTo(buffer)
	var msg gmail.Message
	msg.Raw = base64.URLEncoding.EncodeToString(buffer.Bytes())
	msg.ThreadId = thread.Id
	msg.LabelIds = append(msg.LabelIds, upreachOutLabel)
	mId, _ := srv.Users.Messages.Send("me", &msg).Do()

	return mId.Id
}

func parseHeaders(headers []*gmail.MessagePartHeader) (string, string, string, string) {
	from := ""
	to := ""
	subject := ""
	messageid := ""
	for _, v := range headers {
		// fmt.Println(v.Name)
		switch v.Name {
		case "From":
			from = v.Value
		case "To":
			to = v.Value
		case "Subject":
			subject = v.Value
		case "Message-Id":
			messageid = v.Value
		default:
			continue
		}
	}
	return from, to, subject, messageid
}
func emailSeqReqToEmailSeq(e_seq EmailSeqReq) []Email {
	var seq []Email
	for _, v := range e_seq.Emails {
		// fmt.Println(time.Duration(v.Offset * 1000000000))
		seq = append(seq, Email{
			Subject:    v.Subject,
			Body:       v.Body,
			TimeOffset: time.Duration(v.TimeOffset * 1000000000),
		})

	}
	return seq
}

func emailSequenceFromSequenceReq(req ReqBody, minuteMultiplier string) EmailSequence {
	var e_seq []Email

	for _, v := range req.Emails {
		// fmt.Println(time.Duration(v.Offset * 1000000000))
		e_seq = append(e_seq, Email{
			Subject:    v.Subject,
			Body:       v.Body,
			TimeOffset: time.Duration(v.Offset * 1000000000),
		})

	}
	// parsedMinuteMultiplier, _ := strconv.ParseFloat(minuteMultiplier, 32)
	//  uint(float64(time.Minute) * parsedMinuteMultiplier)
	return EmailSequence{Emails: e_seq}
}

type Request struct {
	RequestType string  `json:"req_type"`
	ReqBody     ReqBody `json:"req"`
}

type ReqBody struct {
	Emails []EmailRequest `json:"emails"`
}

type EmailRequest struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Offset  uint   `json:"offset"`
}

func personalizeEmailBodyFromLeadAndTemplate(template string, lead *TaskLead) string {
	t := strings.ReplaceAll(template, "[FNAME]", lead.FirstName)
	t = strings.ReplaceAll(t, "[LNAME]", lead.LastName)
	t = strings.ReplaceAll(t, "[PLINE]", lead.PersonalizedLine)
	return t
}

func personalizeSubjectLineFromLeadAndTemplate(template string, lead *TaskLead) string {
	t := strings.ReplaceAll(template, "[FNAME]", lead.FirstName)
	t = strings.ReplaceAll(t, "[LNAME]", lead.LastName)
	t = strings.ReplaceAll(t, "[PLINE]", lead.PersonalizedLine)
	return t
}
func handleLinkTracking(c *gin.Context) {
	db := openDB()
	taskDb := openChronDB()

	var eT ExecutedTask

	c.Param("taskid")
	taskDb.Where("id = ?", c.Param("taskid")).First(&eT)

	var campaign Campaign

	db.Preload(clause.Associations).First(&campaign, c.Param("campaignid"))
	var user User
	user.ID = campaign.UserID
	db.Preload(clause.Associations).First(&user)

	user.Stats.LinkClicks += 1
	campaign.Stats.LinkClicks += 1
	campaign.Stats.ClickThroughRate = uint((float64(campaign.Stats.LinkClicks) / float64(campaign.Stats.EmailsSent)) * 100)
	campaign.Stats.StepStats[eT.EmailNumber].LinkClicks += 1
	campaign.Stats.StepStats[eT.EmailNumber].ClickThroughRate = uint(float64(campaign.Stats.StepStats[eT.EmailNumber].LinkClicks) / float64(campaign.Stats.StepStats[eT.EmailNumber].SentEmails) * 100)

	db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&user)
	taskDb.Save(&eT)
	db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&campaign)

	// c.Param("campaignid")
	// c.Param("userid")
	// c.Param("emailno")
	decodedStr, _ := url.QueryUnescape(c.Query("url"))
	c.Redirect(http.StatusTemporaryRedirect, decodedStr)
}
