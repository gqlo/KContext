package kcontext

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const DefaultAlertmanagerURL = "https://localhost:9094"

const (
	defaultAlertmanagerURL      = DefaultAlertmanagerURL
	alertmanagerClientTimeout   = 15 * time.Second
	alertmanagerPollCtxTimeout  = 20 * time.Second
	alertmanagerPFRestartMinGap = 30 * time.Second
)

type alertmanagerClient struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

type alertmanagerHTTPError struct {
	StatusCode int
	Body       string
}

func (e *alertmanagerHTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("alertmanager %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("alertmanager %d", e.StatusCode)
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

type alertmanagerPollSession struct {
	mu            sync.Mutex
	store         *AlertStore
	client        *alertmanagerClient
	pf            *alertmanagerPortForward
	pfOwned       bool
	lastPFRestart time.Time
}

func newAlertmanagerHTTPTransport(insecure bool) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure} //nolint:gosec
	transport.DisableKeepAlives = true
	return transport
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

	return &alertmanagerClient{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout:   alertmanagerClientTimeout,
			Transport: newAlertmanagerHTTPTransport(insecure),
		},
	}, nil
}

func (c *alertmanagerClient) refreshTransport() {
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	c.httpClient = &http.Client{
		Timeout:   alertmanagerClientTimeout,
		Transport: newAlertmanagerHTTPTransport(alertmanagerTLSInsecure()),
	}
}

func (c *alertmanagerClient) refreshToken() error {
	token, source, err := alertmanagerToken()
	if err != nil {
		return err
	}
	c.token = token
	if source != "" {
		log.Printf("Alertmanager auth refreshed: %s", source)
	}
	return nil
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
		token := strings.TrimSpace(string(b))
		SetClusterAuthToken(token)
		return token, "ALERTMANAGER_TOKEN_FILE", nil
	}

	token, err = resolveAutoServiceAccountToken()
	if err != nil {
		return "", "", err
	}
	SetClusterAuthToken(token)
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
		return nil, &alertmanagerHTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
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

func newAlertmanagerPollSession(store *AlertStore) (*alertmanagerPollSession, error) {
	client, err := newAlertmanagerClient()
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, nil
	}

	pf, err := maybeStartAlertmanagerPortForward()
	if err != nil {
		log.Printf("WARNING: Alertmanager port-forward: %v", err)
	}

	return &alertmanagerPollSession{
		store:   store,
		client:  client,
		pf:      pf,
		pfOwned: pf != nil,
	}, nil
}

func (s *alertmanagerPollSession) run() {
	interval := parsePollInterval()
	log.Printf("Alertmanager polling enabled: %s every %s", s.client.baseURL, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	poll := func() {
		ctx, cancel := context.WithTimeout(context.Background(), alertmanagerPollCtxTimeout)
		defer cancel()

		alerts, err := s.client.FetchAlerts(ctx)
		if err != nil {
			log.Printf("alertmanager poll: %v", err)
			s.recoverFromPollError(err)
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

		newCount, resolvedCount, err := s.store.SyncPolled(ctx, polled)
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

func (s *alertmanagerPollSession) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pf != nil {
		s.pf.stop()
		s.pf = nil
	}
}

func (s *alertmanagerPollSession) recoverFromPollError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.client.refreshTransport()

	if isAlertmanagerUnauthorized(err) {
		if tokenErr := s.client.refreshToken(); tokenErr != nil {
			log.Printf("alertmanager token refresh: %v", tokenErr)
		}
	}

	if !shouldRecoverPortForward(s.client.baseURL) {
		return
	}

	if s.pfOwned && s.pf != nil {
		if time.Since(s.lastPFRestart) < alertmanagerPFRestartMinGap {
			return
		}
		newPF, pfErr := s.pf.restart()
		if pfErr != nil {
			log.Printf("alertmanager port-forward restart: %v", pfErr)
			return
		}
		s.pf = newPF
		s.lastPFRestart = time.Now()
		log.Printf("Alertmanager port-forward restarted after poll failure")
		return
	}

	cfg, cfgErr := alertmanagerPortForwardConfig()
	if cfgErr != nil {
		log.Printf("alertmanager port-forward recovery: %v", cfgErr)
		return
	}
	if portListening(cfg.localPort) {
		log.Printf("alertmanager poll failed and port %d is in use by another process; restart port-forward manually or set ALERTMANAGER_PORT_FORWARD=true after freeing the port", cfg.localPort)
		return
	}

	newPF, pfErr := startAlertmanagerPortForward(cfg)
	if pfErr != nil {
		log.Printf("alertmanager port-forward start: %v", pfErr)
		return
	}
	s.pf = newPF
	s.pfOwned = true
	s.lastPFRestart = time.Now()
	log.Printf("Alertmanager port-forward started after poll failure")
}

func shouldRecoverPortForward(baseURL string) bool {
	if !shouldStartAlertmanagerPortForward() {
		return false
	}
	return isLocalhostAlertmanagerURL(baseURL)
}

func isAlertmanagerUnauthorized(err error) bool {
	var httpErr *alertmanagerHTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusUnauthorized
}

// AlertmanagerAlertFingerprint is a minimal alert identity returned by FetchAlertmanagerAlerts.
type AlertmanagerAlertFingerprint struct {
	Fingerprint string
}

// NewAlertmanagerHTTPTransport returns the HTTP transport used for Alertmanager polling.
func NewAlertmanagerHTTPTransport(insecure bool) http.RoundTripper {
	return newAlertmanagerHTTPTransport(insecure)
}

// FetchAlertmanagerAlerts requests the Alertmanager alerts API once.
func FetchAlertmanagerAlerts(ctx context.Context, baseURL, token string, httpClient *http.Client) ([]AlertmanagerAlertFingerprint, error) {
	c := &alertmanagerClient{
		baseURL:    baseURL,
		token:      token,
		httpClient: httpClient,
	}
	alerts, err := c.FetchAlerts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AlertmanagerAlertFingerprint, len(alerts))
	for i, a := range alerts {
		out[i] = AlertmanagerAlertFingerprint{Fingerprint: a.Fingerprint}
	}
	return out, nil
}

// RefreshAlertmanagerHTTPClient replaces the transport on an Alertmanager HTTP client.
func RefreshAlertmanagerHTTPClient(c *http.Client) {
	if transport, ok := c.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	*c = http.Client{
		Timeout:   c.Timeout,
		Transport: newAlertmanagerHTTPTransport(alertmanagerTLSInsecure()),
	}
	if c.Timeout == 0 {
		c.Timeout = alertmanagerClientTimeout
	}
}

// IsAlertmanagerUnauthorized reports whether err is HTTP 401 from Alertmanager.
func IsAlertmanagerUnauthorized(err error) bool {
	return isAlertmanagerUnauthorized(err)
}

// ShouldRecoverPortForward reports whether poll failures should restart port-forward.
func ShouldRecoverPortForward(baseURL string) bool {
	return shouldRecoverPortForward(baseURL)
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
