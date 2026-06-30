package kcontext

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	alertsKey          = "kcontext:alerts"
	activeFingerprints = "kcontext:active-fingerprints"
	fpStateKey         = "kcontext:fp-state"
	fpMetaKey          = "kcontext:fp-meta"
)

type Alert struct {
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	Status       string            `json:"status"`
	Fingerprint  string            `json:"fingerprint,omitempty"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
	StartsAt     time.Time         `json:"startsAt,omitempty"`
	EndsAt       time.Time         `json:"endsAt,omitempty"`
	UpdatedAt    time.Time         `json:"updatedAt,omitempty"`
}

type StoredAlert struct {
	ID           string            `json:"id"`
	ReceivedAt   time.Time         `json:"received_at"`
	Source       string            `json:"source"`
	Fingerprint  string            `json:"fingerprint,omitempty"`
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	GeneratorURL string            `json:"generatorURL,omitempty"`
	StartsAt     time.Time         `json:"startsAt,omitempty"`
	EndsAt       time.Time         `json:"endsAt,omitempty"`
	UpdatedAt    time.Time         `json:"updatedAt,omitempty"`
}

func (a *StoredAlert) UnmarshalJSON(data []byte) error {
	type alias StoredAlert
	aux := struct {
		alias
		ReceivedAt json.RawMessage `json:"received_at"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*a = StoredAlert(aux.alias)
	if t := ParseFlexibleTime(aux.ReceivedAt); !t.IsZero() {
		a.ReceivedAt = t
	} else if !a.ReceivedAt.IsZero() {
		a.ReceivedAt = a.ReceivedAt.UTC()
	}
	return nil
}

func (a StoredAlert) RunbookURL() string {
	for _, key := range []string{"runbook_url", "runbook"} {
		u := strings.TrimSpace(a.Annotations[key])
		if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") {
			return u
		}
	}
	return ""
}

func (a StoredAlert) Namespace() string {
	for _, key := range []string{"namespace", "kubernetes_namespace", "exported_namespace", "ns"} {
		if v := strings.TrimSpace(a.Labels[key]); v != "" {
			return v
		}
	}
	if v := strings.TrimSpace(a.Annotations["namespace"]); v != "" {
		return v
	}
	return ""
}

func (a StoredAlert) Pod() string {
	for _, key := range []string{"pod", "kubernetes_pod_name", "pod_name"} {
		if v := strings.TrimSpace(a.Labels[key]); v != "" {
			return v
		}
	}
	return ""
}

func (a StoredAlert) Severity() string {
	return strings.ToLower(strings.TrimSpace(a.Labels["severity"]))
}

// DisplayTime returns the best timestamp for dashboard display: alert startsAt when
// available, otherwise when KContext received/stored the alert.
func (a StoredAlert) DisplayTime() time.Time {
	if !a.StartsAt.IsZero() && a.StartsAt.Year() >= 1970 {
		return a.StartsAt.UTC()
	}
	if !a.ReceivedAt.IsZero() && a.ReceivedAt.Year() >= 1970 {
		return a.ReceivedAt.UTC()
	}
	return time.Time{}
}

func (a StoredAlert) RowClass() string {
	switch strings.ToLower(a.Status) {
	case "resolved":
		return "row-resolved"
	case "suppressed":
		return "row-suppressed"
	default:
		sev := a.Severity()
		if sev == "" {
			sev = "none"
		}
		return "row-firing row-severity-" + sev
	}
}

type PolledAlert struct {
	Fingerprint string
	Alert       Alert
}

type AlertStore struct {
	rdb *redis.Client
}

// NewAlertStoreWithRedis constructs an AlertStore backed by an existing Redis client.
// Intended for tests; production code should use NewAlertStore.
func NewAlertStoreWithRedis(rdb *redis.Client) *AlertStore {
	return &AlertStore{rdb: rdb}
}

func NewAlertStore() (*AlertStore, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping %s: %w", addr, err)
	}

	return &AlertStore{rdb: rdb}, nil
}

func (s *AlertStore) Save(ctx context.Context, alert Alert) error {
	return s.saveStored(ctx, NewStoredFromAlert(alert, "webhook"))
}

func NewStoredFromAlert(alert Alert, source string) StoredAlert {
	return StoredAlert{
		ID:           fmt.Sprintf("%d", time.Now().UnixNano()),
		ReceivedAt:   time.Now().UTC(),
		Source:       source,
		Fingerprint:  alert.Fingerprint,
		Status:       alert.Status,
		Labels:       alert.Labels,
		Annotations:  alert.Annotations,
		GeneratorURL: alert.GeneratorURL,
		StartsAt:     alert.StartsAt,
		EndsAt:       alert.EndsAt,
		UpdatedAt:    alert.UpdatedAt,
	}
}

func (s *AlertStore) SyncPolled(ctx context.Context, alerts []PolledAlert) (newCount, resolvedCount int, err error) {
	active := make(map[string]PolledAlert, len(alerts))
	for _, a := range alerts {
		active[a.Fingerprint] = a
	}

	known, err := s.rdb.SMembers(ctx, activeFingerprints).Result()
	if err != nil {
		return 0, 0, err
	}

	for _, polled := range alerts {
		prev, err := s.rdb.HGet(ctx, fpStateKey, polled.Fingerprint).Result()
		if err == nil && prev == polled.Alert.Status {
			continue
		}

		meta, _ := json.Marshal(polled.Alert)
		pipe := s.rdb.Pipeline()
		pipe.HSet(ctx, fpStateKey, polled.Fingerprint, polled.Alert.Status)
		pipe.HSet(ctx, fpMetaKey, polled.Fingerprint, meta)
		pipe.SAdd(ctx, activeFingerprints, polled.Fingerprint)
		if _, err := pipe.Exec(ctx); err != nil {
			return newCount, resolvedCount, err
		}

		stored := NewStoredFromAlert(polled.Alert, "poll")
		stored.Fingerprint = polled.Fingerprint
		if err := s.saveStored(ctx, stored); err != nil {
			return newCount, resolvedCount, err
		}
		newCount++
	}

	for _, fp := range known {
		if _, stillActive := active[fp]; stillActive {
			continue
		}

		metaJSON, err := s.rdb.HGet(ctx, fpMetaKey, fp).Result()
		if err != nil {
			continue
		}

		var cached Alert
		if err := json.Unmarshal([]byte(metaJSON), &cached); err != nil {
			continue
		}
		cached.Status = "resolved"

		pipe := s.rdb.Pipeline()
		pipe.HDel(ctx, fpStateKey, fp)
		pipe.HDel(ctx, fpMetaKey, fp)
		pipe.SRem(ctx, activeFingerprints, fp)
		if _, err := pipe.Exec(ctx); err != nil {
			return newCount, resolvedCount, err
		}

		stored := NewStoredFromAlert(cached, "poll")
		stored.Fingerprint = fp
		stored.Status = "resolved"
		if err := s.saveStored(ctx, stored); err != nil {
			return newCount, resolvedCount, err
		}
		resolvedCount++
	}

	return newCount, resolvedCount, nil
}

func (s *AlertStore) saveStored(ctx context.Context, stored StoredAlert) error {
	if stored.Labels == nil {
		stored.Labels = map[string]string{}
	}
	if stored.Annotations == nil {
		stored.Annotations = map[string]string{}
	}

	body, err := json.Marshal(stored)
	if err != nil {
		return err
	}

	return s.rdb.LPush(ctx, alertsKey, body).Err()
}

// Len returns the number of alerts stored in Redis.
func (s *AlertStore) Len(ctx context.Context) (int64, error) {
	return s.rdb.LLen(ctx, alertsKey).Result()
}

func (s *AlertStore) List(ctx context.Context, limit int64) ([]StoredAlert, error) {
	end := limit - 1
	if limit <= 0 {
		end = -1
	}

	raw, err := s.rdb.LRange(ctx, alertsKey, 0, end).Result()
	if err != nil {
		return nil, err
	}

	alerts := make([]StoredAlert, 0, len(raw))
	for _, item := range raw {
		itemBytes := []byte(item)
		var a StoredAlert
		if err := json.Unmarshal(itemBytes, &a); err != nil {
			continue
		}
		alerts = append(alerts, normalizeReceivedAt(a, itemBytes))
	}
	return alerts, nil
}

func (s *AlertStore) Get(ctx context.Context, id string) (*StoredAlert, error) {
	raw, err := s.rdb.LRange(ctx, alertsKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	for _, item := range raw {
		itemBytes := []byte(item)
		var a StoredAlert
		if err := json.Unmarshal(itemBytes, &a); err != nil {
			continue
		}
		a = normalizeReceivedAt(a, itemBytes)
		if a.ID == id {
			return &a, nil
		}
	}
	return nil, nil
}
