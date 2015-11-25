package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/bitly/go-simplejson"
	r "github.com/dancannon/gorethink"
)

var dbSession *r.Session

// called before the main function. Sets the DB session
// pointer correctly
func init() {
	var err error
	dbSession, err = r.Connect(r.ConnectOpts{
		Address:  "localhost:28015",
		Database: "twitter_streaming",
	})
	if err != nil {
		log.Fatal(err)
	}
	dbSession.SetMaxOpenConns(10)
}

// processing a single msg from the queue and then
// subsequently deletes it
func processMsg(msg *sqs.Message, svc *sqs.SQS, queueUrl string) {
	// process
	tweetBody := []byte(*msg.Body)
	tweetJson, err := simplejson.NewJson(tweetBody)
	if err != nil {
		log.Println("unable to parse tweet")
		return
	}
	text := tweetJson.Get("text").MustString()
	resp, err := classifyText(text)
	if err != nil {
		log.Println("failed to classify tweet")
	}
	sentiment, err := simplejson.NewJson(resp)
	if err != nil {
		log.Fatal(err)
	}
	// save this to the db
	tweetJson.Set("sentiment", sentiment)

	// insert tweet into DB
	id := insertTweetInDb(tweetJson)
	log.Println("Saving tweet with id", id)

	// delete the message
	//deleteTweet(msg, svc, queueUrl)
}

// deletes a message from the queue
func deleteMsg(msg *sqs.Message, svc *sqs.SQS, queueUrl string) {
	_, err := svc.DeleteMessage(
		&sqs.DeleteMessageInput{
			QueueUrl:      aws.String(queueUrl),
			ReceiptHandle: aws.String(*msg.ReceiptHandle),
		},
	)
	if err != nil {
		log.Println("Unable to delete", err)
	}
}

// classifies a string using the monkeylearn api
func classifyText(text string) ([]byte, error) {
	const API_URL = "https://api.monkeylearn.com/v2/classifiers/cl_qkjxv9Ly/classify/?"
	const API_TOKEN = "Token 8c47cd62c949d26430c775850a6bfdfe798091ac"

	client := &http.Client{}

	// the request body data
	m := map[string]interface{}{
		"text_list": [1]string{text},
	}

	// preparing the request
	mJson, _ := json.Marshal(m)
	contentReader := bytes.NewReader(mJson)
	req, err := http.NewRequest("POST", API_URL, contentReader)
	if err != nil {
		log.Print(err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", API_TOKEN)

	// make the request
	resp, err := client.Do(req)
	if err != nil {
		log.Print(err)
	}
	return ioutil.ReadAll(resp.Body)
}

// inserts the tweet in the db
func insertTweetInDb(tweetJson *simplejson.Json) string {
	// delete this pesky key - creates issues with rethinkdb
	tweetJson.Del("id")
	m, err := tweetJson.Map()
	if err != nil {
		fmt.Println(err)
		return ""
	}
	result, err := r.Table("jstwitter").Insert(m).RunWrite(dbSession)
	if err != nil {
		fmt.Println(err)
		return ""
	}
	return result.GeneratedKeys[0]
}

func main() {
	const QUEUE_NAME = "tweetsQueue"
	const WAIT_TIME = 10

	// initialize
	svc := sqs.New(session.New(), &aws.Config{Region: aws.String("us-east-1")})

	params := &sqs.CreateQueueInput{
		QueueName: aws.String(QUEUE_NAME),
	}

	// get the queue url
	var queueUrl string
	resp, err := svc.CreateQueue(params)
	if err != nil {
		panic(err)
	}
	queueUrl = *resp.QueueUrl

	for {
		// get the messages
		msgs, e := svc.ReceiveMessage(
			&sqs.ReceiveMessageInput{
				QueueUrl:            aws.String(queueUrl),
				MaxNumberOfMessages: aws.Int64(10),
			},
		)
		log.Println("Got", len(msgs.Messages), "messages")
		if e != nil {
			panic(e)
		}

		for _, msg := range msgs.Messages {
			//go processMsg(msg, svc, queueUrl)
			go processMsg(msg, svc, queueUrl)
		}
		// wait for a second for hitting again
		time.Sleep(WAIT_TIME * time.Second)
	}

}