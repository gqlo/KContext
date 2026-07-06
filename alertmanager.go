package kcontext

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const DefaultAlertmanagerURL = "https://localhost:9094"

const defaultAlertmanagerURL = DefaultAlertmanagerURL

type alertmanagerClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

type amAlert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	Fingerprint  string            `json:"fingerprint"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	UpdatedAt    time.Time         `json:"updatedAt"`
	Status       struct {
		State string `json:"state"`
	} `json:"status"`
}

func newAlertmanagerClient() (*alertmanagerClient, error) {
	if !alertmanagerPollingEnabled() {
		return nil, nil
	}

	baseURL := alertmanagerURL()
	token, tokenSource, err := alertmanagerToken()
	if err != nil {
		return nil, err
	}
	insecure := alertmanagerTLSInsecure()

	if _, ok := os.LookupEnv("ALERTMANAGER_URL"); !ok {
		log.Printf("ALERTMANAGER_URL not set, using default %s", baseURL)
	}
	if _, ok := os.LookupEnv("ALERTMANAGER_TLS_INSECURE"); !ok {
		log.Printf("ALERTMANAGER_TLS_INSECURE not set, using default true (port-forward TLS)")
	}
	if tokenSource != "" {
		log.Printf("Alertmanager auth: %s", tokenSource)
	} else if token == "" {
		log.Print("WARNING: no Alertmanager token")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec

	return &alertmanagerClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
		},
	}, nil
}

func alertmanagerPollingEnabled() bool {
	v, ok := os.LookupEnv("ALERTMANAGER_URL")
	if !ok {
		return true
	}
	return strings.TrimSpace(v) != ""
}

func alertmanagerURL() string {
	v, ok := os.LookupEnv("ALERTMANAGER_URL")
	if !ok {
		return defaultAlertmanagerURL
	}
	return strings.TrimRight(strings.TrimSpace(v), "/")
}

func alertmanagerTLSInsecure() bool {
	v, ok := os.LookupEnv("ALERTMANAGER_TLS_INSECURE")
	if !ok {
		return true
	}
	return v == "true"
}

func alertmanagerToken() (token, source string, err error) {
	if path := strings.TrimSpace(os.Getenv("ALERTMANAGER_TOKEN_FILE")); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("read ALERTMANAGER_TOKEN_FILE: %w", err)
		}
		return strings.TrimSpace(string(b)), "ALERTMANAGER_TOKEN_FILE", nil
	}

	token, err = resolveAutoServiceAccountToken()
	if err != nil {
		return "", "", err
	}
	return token, "oc create token", nil
}

func (c *alertmanagerClient) FetchAlerts(ctx context.Context) ([]amAlert, error) {
	u, err := alertmanagerAlertsURL(c.baseURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("alertmanager %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var alerts []amAlert
	if err := json.NewDecoder(resp.Body).Decode(&alerts); err != nil {
		return nil, err
	}
	return alerts, nil
}

// alertmanagerAlertsURL builds the Alertmanager alerts API URL with default
// inclusive filters (active, silenced, inhibited, unprocessed) so all alerts
// currently held in Alertmanager are returned.
func alertmanagerAlertsURL(baseURL string) (string, error) {
	u, err := url.Parse(baseURL + "/api/v2/alerts")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("active", "true")
	q.Set("silenced", "true")
	q.Set("inhibited", "true")
	q.Set("unprocessed", "true")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func polledAlertStatus(a amAlert) string {
	if a.Status.State == "suppressed" {
		return "suppressed"
	}
	if !a.EndsAt.IsZero() && a.EndsAt.Year() >= 1970 && !a.EndsAt.After(time.Now().UTC()) {
		return "resolved"
	}
	return "firing"
}

func parsePollInterval() time.Duration {
	raw := os.Getenv("ALERTMANAGER_POLL_INTERVAL")
	if raw == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("invalid ALERTMANAGER_POLL_INTERVAL %q, using 10s: %v", raw, err)
		return 10 * time.Second
	}
	if d < 5*time.Second {
		return 5 * time.Second
	}
	return d
}

func startAlertmanagerPoller(store *AlertStore, client *alertmanagerClient) {
	interval := parsePollInterval()
	log.Printf("Alertmanager polling enabled: %s every %s", client.baseURL, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	poll := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		alerts, err := client.FetchAlerts(ctx)
		if err != nil {
			log.Printf("alertmanager poll: %v", err)
			return
		}

		polled := make([]PolledAlert, 0, len(alerts))
		for _, a := range alerts {
			if a.Fingerprint == "" {
				continue
			}
			status := polledAlertStatus(a)
			polled = append(polled, PolledAlert{
				Fingerprint: a.Fingerprint,
				Alert: Alert{
					Labels:       a.Labels,
					Annotations:  a.Annotations,
					Status:       status,
					Fingerprint:  a.Fingerprint,
					GeneratorURL: a.GeneratorURL,
					StartsAt:     a.StartsAt,
					EndsAt:       a.EndsAt,
					UpdatedAt:    a.UpdatedAt,
				},
			})
		}

		newCount, resolvedCount, err := store.SyncPolled(ctx, polled)
		if err != nil {
			log.Printf("sync polled alerts: %v", err)
			return
		}
		if newCount > 0 || resolvedCount > 0 {
			log.Printf("alertmanager poll: %d new, %d resolved", newCount, resolvedCount)
		}
	}

	poll()
	for range ticker.C {
		poll()
	}
}

// AlertmanagerAlertsURL returns the alerts API URL used for polling.
func AlertmanagerAlertsURL(baseURL string) (string, error) {
	return alertmanagerAlertsURL(baseURL)
}

// AlertmanagerPollingEnabled reports whether Alertmanager polling is active.
func AlertmanagerPollingEnabled() bool { return alertmanagerPollingEnabled() }

// AlertmanagerURL returns the configured Alertmanager base URL.
func AlertmanagerURL() string { return alertmanagerURL() }

// AlertmanagerTLSInsecure reports whether TLS verification is skipped.
func AlertmanagerTLSInsecure() bool { return alertmanagerTLSInsecure() }

// ResolveAlertmanagerToken resolves the bearer token and its source label.
func ResolveAlertmanagerToken() (token, source string, err error) {
	return alertmanagerToken()
}
