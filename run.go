package kcontext

import (
	"log"
	"net/http"
	"os"
)

// Run starts the KContext HTTP server and optional Alertmanager poller.
func Run() {
	store, err := NewAlertStore()
	if err != nil {
		log.Fatal(err)
	}

	srv := NewServer(store, os.Getenv("SLACK_TOKEN"), os.Getenv("SLACK_CHANNEL_ID"))

	pf, err := maybeStartAlertmanagerPortForward()
	if err != nil {
		log.Printf("WARNING: Alertmanager port-forward: %v", err)
	} else if pf != nil {
		defer pf.stop()
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
		addr = ":8083"
	}

	http.HandleFunc("/", srv.HandleAlertsPage)
	http.HandleFunc("/alert", srv.HandleAlertDetail)
	http.HandleFunc("/webhook", srv.HandleWebhook)

	log.Printf("KContext listening on %s", addr)
	if srv.SlackEnabled() {
		log.Print("Slack notifications enabled")
	}
	log.Fatal(http.ListenAndServe(addr, nil))
}
