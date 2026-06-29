package main

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
	baseURL := strings.TrimRight(os.Getenv("ALERTMANAGER_URL"), "/")
	if baseURL == "" {
		return nil, nil
	}

	token := os.Getenv("ALERTMANAGER_TOKEN")
	if token == "" {
		if path := os.Getenv("ALERTMANAGER_TOKEN_FILE"); path != "" {
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read ALERTMANAGER_TOKEN_FILE: %w", err)
			}
			token = strings.TrimSpace(string(b))
		}
	}

	insecure := os.Getenv("ALERTMANAGER_TLS_INSECURE") == "true"
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

func (c *alertmanagerClient) FetchActive(ctx context.Context) ([]amAlert, error) {
	u, err := url.Parse(c.baseURL + "/api/v2/alerts")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("active", "true")
	q.Set("silenced", "false")
	q.Set("inhibited", "false")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
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

func parsePollInterval() time.Duration {
	raw := os.Getenv("ALERTMANAGER_POLL_INTERVAL")
	if raw == "" {
		return 30 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("invalid ALERTMANAGER_POLL_INTERVAL %q, using 30s: %v", raw, err)
		return 30 * time.Second
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

		alerts, err := client.FetchActive(ctx)
		if err != nil {
			log.Printf("alertmanager poll: %v", err)
			return
		}

		polled := make([]PolledAlert, 0, len(alerts))
		for _, a := range alerts {
			if a.Fingerprint == "" {
				continue
			}
			status := "firing"
			if a.Status.State == "suppressed" {
				status = "suppressed"
			}
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
