package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const secretKEY = "12312412414124"

type ChangeSettingsReq struct {
	EmailTimeOffset uint `json:"EmailTimeOffset" binding:"required"`
	EmailsPerDay    uint `json:"EmailsPerDay" binding:"required"`
}

type signUpForm struct {
	Email    string `form:"email" binding:"required"`
	Password string `form:"password" binding:"required"`
}

type AddLeadsReq struct {
	CampaignID uint      `json:"CampaignID" binding:"required"`
	Leads      []LeadReq `json:"Leads" binding:"required"`
}

type StartCampaignReq struct {
	FirstEmailOffset uint `json:"FirstEmailOffset"`
}

type LeadReq struct {
	FirstName        string `json:"FirstName" binding:"required"`
	LastName         string `json:"LastName" binding:"required"`
	Email            string `json:"Email" binding:"required"`
	PersonalizedLine string `json:"PersonalizedLine" binding:"required"`
}

type CampaignReq struct {
	Name string `form:"Name" binding:"required"`
}
type AddEmailSeqReq struct {
	CampaignID uint        `json:"CampaignID" binding:"required"`
	EmailSeq   EmailSeqReq `json:"EmailSeq" binding:"required"`
}
type EmailSeqReq struct {
	Emails []EmailReq `json:"Emails" binding:"required"`
}

type EmailReq struct {
	Subject    string `json:"Subject" binding:"required"`
	From       string `json:"From" binding:"required"`
	Body       string `json:"Body" binding:"required"`
	TimeOffset uint   `json:"TimeOffset" binding:"required"`
}

func makeRoutes(r *gin.Engine) {
	r.GET("/pixels/:userid/:campaignid/:emailno/:taskid", pixelHandler)
	r.GET("/unsubscribe/:userid/:campaignid/:emailno/:taskid", unsubscribeHandler)
	r.POST("/api/user/signup", signUpHandler)
	r.POST("/api/user/signin", signInHandler)
	r.GET("/api/user/getuser", requireAuth, getUserData)
	r.POST("/api/user/settings/update", requireAuth, updateUserSettingsHandler)
	r.GET("/api/user/connect/gmail", requireAuth, gmailConnectionHandler)
	r.GET("/api/campaign/get_campaigns", requireAuth, getCampaingsHandler)
	r.GET("/api/campaign/get_campaigns/detailed", requireAuth, getCampaingsDetailedHandler)
	r.GET("/api/campaign/:id/get_campaign", requireAuth, getCampaignHandler)
	r.GET("/api/campaign/:id/stop", requireAuth, stopCampaignHandler)
	r.POST("/api/campaign/:id/start", requireAuth, startCampaignHandler)
	r.GET("/api/campaign/:id/get_campaign/detailed", requireAuth, getCampaingDetailedHandler)
	r.POST("/api/campaign/create", requireAuth, createCampaignHandler)
	r.POST("/api/campaign/add_leads", requireAuth, addLeadsToCampaignHandler)
	r.POST("/api/campaign/add_email_seq", requireAuth, addEmailSeqToCampaign)
	r.GET("/api/campaign/:id/stats", requireAuth, getCampaignStats)

	r.GET("/links/:userid/:campaignid/:emailno/:taskid", handleLinkTracking)
	r.GET("/callbacks/auth/google", googleAuth, handleGoogleAuthCallback)

	r.POST("/callbacks/email/gmail", gmailCallBackHandler)
	// r.Handlers
}
func getUserData(c *gin.Context) {
	userID, _ := c.Get("userID")
	fmt.Println(userID)
	db := openDB()
	var user User
	user.ID = userID.(uint)
	db.Preload(clause.Associations).Where("id = ?", userID).Find(&user)
	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}
func gmailCallBackHandler(c *gin.Context) {
	req, _ := io.ReadAll(c.Request.Body)
	fmt.Println(string(req))
	fmt.Println(c.Request.URL)
}
func handleGoogleAuthCallback(c *gin.Context) {
	userID, _ := c.Get("userID")
	db := openDB()
	var user User
	user.ID = userID.(uint)
	db.Preload(clause.Associations).Where("id = ?", userID).Find(&user)
	b, _ := os.ReadFile("./credentials/credentials.json")
	config, _ := google.ConfigFromJSON(b, gmail.MailGoogleComScope)
	tok, err := config.Exchange(context.TODO(), c.Query("code"))

	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	//save token to user Account
	user.GmailAccessToken = *tok

	go func() {
		//INIT UPREACH LABELS FOR USER INBOX TO KEEP CLEAN AND USE SUBSCRIBTIONS
		srv := getGmailService(&user.GmailAccessToken)

		labels, _ := srv.Users.Labels.List("me").Do()

		var labelIDMAIN string
		var labelIDOUT string

		for _, v := range labels.Labels {
			if v.Name == "UPREACH" {
				labelIDMAIN = v.Id
				continue
			}
			if v.Name == "UPREACH.OUTREACH" {
				labelIDOUT = v.Id
			}

		}

		if labelIDMAIN == "" {
			label, _ := srv.Users.Labels.Create("me", &gmail.Label{
				LabelListVisibility:   "labelShowIfUnread",
				MessageListVisibility: "hide",
				Name:                  "UPREACH",
				Type:                  "user",
			}).Do()
			user.UPREACHLABELID = label.Id
		} else {
			user.UPREACHLABELID = labelIDMAIN
		}
		if labelIDOUT == "" {
			label, _ := srv.Users.Labels.Create("me", &gmail.Label{
				LabelListVisibility:   "labelShowIfUnread",
				MessageListVisibility: "hide",
				Name:                  "UPREACH.OUTREACH",
				Type:                  "user",
			}).Do()
			user.UPREACHLABELIDOUT = label.Id

		} else {
			user.UPREACHLABELIDOUT = labelIDMAIN
		}

		user.GmailActivated = true
		db.Save(&user)
	}()
	// switch to a redirect
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`
	<html>
<head>
</head>
<body onload="waitFiveSec()"> <!--it will wait to load-->

<!-- your html... -->
<script>
  function waitFiveSec(){
	window.location.replace("http://localhost:3000/dashboard");
  }
</script>
<h1>You Can Close This Window Now </h1>
</body>
</html>`))
}

func gmailConnectionHandler(c *gin.Context) {
	userID, _ := c.Get("userID")
	db := openDB()
	var user User
	user.ID = userID.(uint)
	db.Preload(clause.Associations).Where("id = ?", userID).Find(&user)
	// ctx := context.Background()
	b, _ := os.ReadFile("./credentials/credentials.json")
	config, _ := google.ConfigFromJSON(b, gmail.MailGoogleComScope)

	authURL := config.AuthCodeURL(c.Request.Header.Get("X-API-KEY"), oauth2.AccessTypeOffline)

	c.JSON(http.StatusOK, gin.H{
		"authUrl": authURL,
	})

}

func updateUserSettingsHandler(c *gin.Context) {
	userID, _ := c.Get("userID")
	db := openDB()

	var s_req ChangeSettingsReq
	c.ShouldBindJSON(&s_req)

	var user User
	user.ID = userID.(uint)

	db.Preload(clause.Associations).Where("id = ?", userID).Find(&user)
	user.Settings.EmailTimeOffset = s_req.EmailTimeOffset
	user.Settings.EmailsPerDay = s_req.EmailsPerDay
	db.Save(&user)

}

func getCampaignStats(c *gin.Context) {

	userID, _ := c.Get("userID")

	db := openDB()
	taskDb := openChronDB()

	var campaign Campaign
	var user User

	user.ID = userID.(uint)
	campaign.UserID = userID.(uint)
	cId, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	campaign.ID = uint(cId)

	db.Preload(clause.Associations).Where("id = ?", userID).Find(&campaign)

	var taskCampaign TaskCampaign

	taskDb.Preload(clause.Associations).Where("id = ?", campaign.TaskCampaignID).Find(&taskCampaign)
	// fmt.Println(taskCampaign.Tasks)
	// fmt.Println(campaign.Stats.StepStats)

	c.JSON(http.StatusOK, gin.H{
		"stats": campaign.Stats,
	})

}

func addEmailSeqToCampaign(c *gin.Context) {
	var e_seq_req AddEmailSeqReq
	//REQUIRES AUTH
	c.ShouldBindJSON(&e_seq_req)
	db := openDB()
	var campaign Campaign
	campaign.ID = e_seq_req.CampaignID
	db.Preload("EmailSequence.Emails").Preload(clause.Associations).First(&campaign)
	db.Where("email_sequence_id = ?", campaign.EmailSequence.ID).Delete(&Email{})
	campaign.EmailSequence.Emails = emailSeqReqToEmailSeq(e_seq_req.EmailSeq)
	campaign.Stats.SequenceLength = uint(len(campaign.EmailSequence.Emails))

	db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&campaign)
}
func addLeadsToCampaignHandler(c *gin.Context) {
	var l_req AddLeadsReq
	//REQUIRES AUTH
	c.ShouldBindJSON(&l_req)
	db := openDB()
	var campaign Campaign
	campaign.ID = l_req.CampaignID
	db.Preload(clause.Associations).Find(&campaign)
	campaign.Leads = append(campaign.Leads, leadReqsToLeads(l_req.Leads)...)
	campaign.Stats.LeadsAmount += uint(len(campaign.Leads))
	db.Save(&campaign)
	// userID, _ := c.Get("userID")
	// db := openDB()
	// var campaign Campaign
	// db.Preload(clause.Associations).Where("id = ?", userID).Find(&campaigns)

}
func leadReqsToLeads(lead_reqs []LeadReq) []Lead {
	var leads []Lead
	for _, v := range lead_reqs {
		leads = append(leads, Lead{
			FirstName:        v.FirstName,
			LastName:         v.LastName,
			Email:            v.Email,
			PersonalizedLine: v.PersonalizedLine,
		})
	}
	return leads
}
func stopCampaignHandler(c *gin.Context) {

	userID, _ := c.Get("userID")
	db := openDB()
	taskDb := openChronDB()

	var taskCampaign TaskCampaign

	var campaign Campaign
	var user User

	user.ID = userID.(uint)

	campaign.UserID = userID.(uint)
	cId, _ := strconv.ParseUint(c.Param("id"), 10, 32)

	campaign.ID = uint(cId)

	db.Where("id = ?", userID).Find(&campaign)
	campaign.TaskCampaignID = 0
	db.Save(&campaign)

	taskCampaign.CampaignId = campaign.ID

	taskDb.Preload(clause.Associations).Find(&taskCampaign)

	if len(taskCampaign.Tasks) > 0 {
		taskDb.Delete(&taskCampaign.Tasks)
	}
	if len(taskCampaign.ExecutedTasks) > 0 {
		taskDb.Delete(&taskCampaign.ExecutedTasks)
	}

	//taskDb.Delete(&taskCampaign.Tasks, &taskCampaign.ExecutedTasks)

	taskDb.Delete(&taskCampaign)

}

func startCampaignHandler(c *gin.Context) {
	var startReq StartCampaignReq

	c.BindJSON(&startReq)
	fmt.Println(startReq)
	userID, _ := c.Get("userID")
	db := openDB()
	var campaign Campaign
	var user User
	user.ID = userID.(uint)

	campaign.UserID = userID.(uint)
	cId, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	campaign.ID = uint(cId)
	db.Preload("EmailSequence.Emails").Preload(clause.Associations).Where("id = ?", userID).Find(&campaign)
	if startReq.FirstEmailOffset != 0 {
		campaign.FirstEmailOffset = startReq.FirstEmailOffset
	}
	for range campaign.EmailSequence.Emails {
		campaign.Stats.StepStats = append(campaign.Stats.StepStats, StepStat{
			SentEmails:   0,
			OpenedEmails: 0,
		})
	}
	db.Save(&campaign)
	db.First(&user)
	go startCampaign(&campaign, &user, db, openChronDB())
}
func getCampaignHandler(c *gin.Context) {
	//GET ALL CAMPAIGNS WHERE THE OWNER IS THE CURRENT USER FROM MIDDLEWARE AUTH
	//REQUIRES AUTH
	userID, _ := c.Get("userID")
	db := openDB()
	var campaign Campaign
	campaign.UserID = userID.(uint)
	cId, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	campaign.ID = uint(cId)
	db.Find(&campaign)
	c.JSON(http.StatusOK, gin.H{
		"campaign": campaign,
	})
}

func getCampaingsDetailedHandler(c *gin.Context) {
	//GET ALL CAMPAIGNS WHERE THE OWNER IS THE CURRENT USER FROM MIDDLEWARE AUTH
	//REQUIRES AUTH
	userID, _ := c.Get("userID")
	db := openDB()
	var campaigns []Campaign
	db.Preload("EmailSequence.Emails").Preload(clause.Associations).Where("id = ?", userID).Find(&campaigns)
	c.JSON(http.StatusOK, gin.H{
		"campaigns": campaigns,
	})
}
func createCampaignHandler(c *gin.Context) {
	fmt.Println("here")
	var c_req CampaignReq
	//REQUIRES AUTH
	c.Bind(&c_req)

	userID, _ := c.Get("userID")

	db := openDB()
	var user User
	user.ID = userID.(uint)
	db.First(&user)
	campaign := Campaign{
		Name:   c_req.Name,
		Status: "off",
	}

	user.Campaings = append(user.Campaings, campaign)
	db.Save(&user)

	var campaigns []Campaign
	db.Preload(clause.Associations).Where("id = ?", userID).Find(&campaigns)
	c.JSON(http.StatusOK, gin.H{
		"campaigns": campaigns,
	})
}
func getCampaingsHandler(c *gin.Context) {
	//GET ALL CAMPAIGNS WHERE THE OWNER IS THE CURRENT USER FROM MIDDLEWARE AUTH
	//REQUIRES AUTH
	userID, _ := c.Get("userID")
	db := openDB()
	var campaigns []Campaign
	db.Where("id = ?", userID).Find(&campaigns)
	c.JSON(http.StatusOK, gin.H{
		"campaigns": campaigns,
	})
}

func getCampaingDetailedHandler(c *gin.Context) {
	userID, _ := c.Get("userID")
	db := openDB()
	var campaign Campaign
	campaign.UserID = userID.(uint)
	cId, _ := strconv.ParseUint(c.Param("id"), 10, 32)
	campaign.ID = uint(cId)
	db.Preload("EmailSequence.Emails").Preload(clause.Associations).Find(&campaign)
	c.JSON(http.StatusOK, gin.H{
		"campaign": campaign,
	})

}
func signInHandler(c *gin.Context) {
	var form signUpForm
	c.Bind(&form)
	var user User
	db := openDB()
	user.Email = form.Email
	db.Preload(clause.Associations).First(&user)
	if user.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid Email Or PassWord",
		})
		return
	}
	err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(form.Password))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid Email Or PassWord",
		})
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    user.ID,
		"expiry": time.Now().Add(time.Hour * 24 * 30).Unix(),
	})

	tokenString, err := token.SignedString([]byte(secretKEY))
	if err != nil {
		fmt.Println(err)
	}

	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
		"user":  user,
	})
}
func signUpHandler(c *gin.Context) {
	var form signUpForm
	c.Bind(&form)
	db := openDB()
	var user User
	count := int64(0)
	db.Model(&User{}).Where("email = ?", form.Email).Count(&count)
	exists := count > 0

	if exists {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Email Already Exists",
		})
		return
	}
	user.Email = form.Email

	hpass, _ := bcrypt.GenerateFromPassword([]byte(form.Password), 10)

	user.Password = string(hpass)

	user.Settings.EmailsPerDay = 20
	user.Settings.EmailTimeOffset = 60
	user.EmailsSentTodayBuffer = EmailSentBuffer{
		EmailsSentToday: 0,
		UnLockDate:      time.Now(),
		RateLimited:     false,
	}

	db.Save(&user)

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    user.ID,
		"expiry": time.Now().Add(time.Hour * 24 * 30).Unix(),
	})

	tokenString, err := token.SignedString([]byte(secretKEY))
	if err != nil {
		fmt.Println(err)
	}
	c.JSON(http.StatusOK, gin.H{
		"token": tokenString,
		"user":  user,
	})
}

func unsubscribeHandler(c *gin.Context) {
	db := openDB()
	taskDb := openChronDB()

	var user User
	userID, _ := strconv.ParseInt(c.Param("userid"), 10, 32)
	user.ID = uint(userID)
	db.Preload(clause.Associations).First(&user)

	var eT ExecutedTask

	var campaign Campaign
	db.Preload(clause.Associations).First(&campaign, c.Param("campaignid"))

	taskDb.Where("id = ?", c.Param("taskid")).First(&eT)
	taskDb.Where("thread_id = ?", eT.ThreadId).Delete(&Task{})
	user.Stats.Unsubscribes += 1
	campaign.Stats.Unsubscribes += 1
	campaign.Stats.StepStats[eT.EmailNumber].Unsubscribes += 1
	db.Save(&user)
	db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&campaign)

	// fmt.Println("OPENRATE: " + fmt.Sprint(campaign.Stats.OpenRate) + "%")

	// c.Param("taskid")
	// c.Param("campaignid")
	// c.Param("userid")
	// c.Param("emailno")

	c.Status(200)
	// width := 1
	// height := 1
	// upLeft := image.Point{0, 0}
	// lowRight := image.Point{width, height}
	// img := image.NewRGBA(image.Rectangle{upLeft, lowRight})
	// c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0, post-check=0, pre-check=0")
	// c.Header("Pragma", "no-cache")
	// png.Encode(c.Writer, img)
}

func pixelHandler(c *gin.Context) {
	db := openDB()
	taskDb := openChronDB()

	var eT ExecutedTask

	taskDb.Where("id = ?", strings.ReplaceAll(c.Param("taskid"), ".png", "")).First(&eT)

	var campaign Campaign

	db.Preload(clause.Associations).First(&campaign, c.Param("campaignid"))

	var user User
	user.ID = campaign.UserID
	db.Preload(clause.Associations).First(&user)

	if !eT.TaskStats.Opened {
		user.Stats.Opens += 1
		campaign.Stats.Opens += 1
		campaign.Stats.OpenRate = uint((float64(campaign.Stats.Opens) / float64(campaign.Stats.EmailsSent)) * 100)
		campaign.Stats.StepStats[eT.EmailNumber].OpenedEmails += 1
		campaign.Stats.StepStats[eT.EmailNumber].OpenRate = uint(float64(campaign.Stats.StepStats[eT.EmailNumber].OpenedEmails) / float64(campaign.Stats.StepStats[eT.EmailNumber].SentEmails) * 100)
		eT.TaskStats.Opened = true
	}
	taskDb.Save(&eT)
	db.Save(&user)
	db.Session(&gorm.Session{FullSaveAssociations: true}).Updates(&campaign)

	// fmt.Println("OPENRATE: " + fmt.Sprint(campaign.Stats.OpenRate) + "%")

	// c.Param("taskid")
	// c.Param("campaignid")
	// c.Param("userid")
	// c.Param("emailno")

	width := 1
	height := 1
	upLeft := image.Point{0, 0}
	lowRight := image.Point{width, height}
	img := image.NewRGBA(image.Rectangle{upLeft, lowRight})
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0, post-check=0, pre-check=0")
	c.Header("Pragma", "no-cache")
	png.Encode(c.Writer, img)
}
