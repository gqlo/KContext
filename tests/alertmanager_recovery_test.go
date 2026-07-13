package kcontext_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gqlo/kcontext"
)

func TestFetchAlerts_unauthorized(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: kcontext.NewAlertmanagerHTTPTransport(true),
	}

	_, err := kcontext.FetchAlertmanagerAlerts(context.Background(), srv.URL, "old-token", client)
	if !kcontext.IsAlertmanagerUnauthorized(err) {
		t.Fatalf("FetchAlertmanagerAlerts() error = %v, want unauthorized", err)
	}
}

func TestFetchAlerts_success(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"fingerprint": "abc123",
			"labels":      map[string]string{"alertname": "TestAlert"},
		}})
	}))
	t.Cleanup(srv.Close)

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: kcontext.NewAlertmanagerHTTPTransport(true),
	}

	alerts, err := kcontext.FetchAlertmanagerAlerts(context.Background(), srv.URL, "", client)
	if err != nil {
		t.Fatalf("FetchAlertmanagerAlerts() error: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Fingerprint != "abc123" {
		t.Fatalf("alerts = %+v", alerts)
	}
}

func TestAlertmanagerClient_refreshTransport(t *testing.T) {
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: kcontext.NewAlertmanagerHTTPTransport(true),
	}
	oldTransport := client.Transport
	kcontext.RefreshAlertmanagerHTTPClient(client)
	if client.Transport == oldTransport {
		t.Fatal("RefreshAlertmanagerHTTPClient() did not replace transport")
	}
}

func TestShouldRecoverPortForward(t *testing.T) {
	t.Setenv("ALERTMANAGER_URL", "https://localhost:9094")
	t.Setenv("ALERTMANAGER_PORT_FORWARD", "true")
	if !kcontext.ShouldRecoverPortForward("https://localhost:9094") {
		t.Fatal("want recovery for owned localhost port-forward")
	}

	t.Setenv("ALERTMANAGER_URL", "https://alertmanager-main.openshift-monitoring.svc:9094")
	if kcontext.ShouldRecoverPortForward("https://alertmanager-main.openshift-monitoring.svc:9094") {
		t.Fatal("should not recover port-forward for in-cluster URL")
	}
}
