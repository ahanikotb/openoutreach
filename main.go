package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-co-op/gocron"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "./credentials/token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	// fmt.Println(tok)
	return config.Client(context.Background(), tok)
}
func getClientFromUser(config *oauth2.Config) *http.Client {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tokFile := "./credentials/token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code: %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web: %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)

	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func main() {
	godotenv.Load(".env")
	r := gin.Default()

	// r.Use(cors.AllowAll())

	if (os.Getenv("ENVIRONMENT")) == "DEV" {
		if err := os.Remove("./db/tasks.db"); err != nil {
			log.Fatal(err)
		}
		if err := os.Remove("./db/test.db"); err != nil {
			log.Fatal(err)
		}
		src := "./db/backup/test.db"
		dest := "./db/test.db"

		bytesRead, err := ioutil.ReadFile(src)

		if err != nil {
			log.Fatal(err)
		}

		err = ioutil.WriteFile(dest, bytesRead, 0644)

		if err != nil {
			log.Fatal(err)
		}

	}

	db := openDB()
	chronDb := openChronDB()
	// testApp(db, chronDb)

	//Start Chron
	s := gocron.NewScheduler(time.UTC)

	EXECUTEEVERYX, _ := strconv.ParseInt(os.Getenv("EXECUTEEVERYX"), 10, 32)
	STATSCHRONOFFSET, _ := strconv.ParseInt(os.Getenv("STATSOFFSETTING"), 10, 32)

	// EXECUTION CHRON
	s.Every(int(EXECUTEEVERYX)).Second().Tag("SEND").Do(executionChron, db, chronDb)

	//CHECK FOR REPLYS
	s.Every(int(EXECUTEEVERYX)).Second().Tag("STATS").Do(func() {
		//RUN THE CHRON AFTER OFFSET
		time.Sleep(time.Duration(time.Second.Nanoseconds() * (STATSCHRONOFFSET)))
		statsChron(db, chronDb)
	})

	s.StartAsync()
	r.Use(CORSMiddleware())

	makeRoutes(r)
	// r.Use(CORSMiddleware())

	r.Run()

}
