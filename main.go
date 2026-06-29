package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	store, err := newAlertStore()
	if err != nil {
		log.Fatal(err)
	}

	srv := &server{
		store:       store,
		slackToken:  os.Getenv("SLACK_TOKEN"),
		channelID:   os.Getenv("SLACK_CHANNEL_ID"),
		dailyThread: map[string]string{},
	}

	amClient, err := newAlertmanagerClient()
	if err != nil {
		log.Fatal(err)
	}
	if amClient != nil {
		go startAlertmanagerPoller(store, amClient)
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	http.HandleFunc("/", srv.handleAlertsPage)
	http.HandleFunc("/alert", srv.handleAlertDetail)
	http.HandleFunc("/webhook", srv.handleWebhook)

	log.Printf("KContext listening on %s", addr)
	if srv.slackEnabled() {
		log.Print("Slack notifications enabled")
	}
	log.Fatal(http.ListenAndServe(addr, nil))
}
