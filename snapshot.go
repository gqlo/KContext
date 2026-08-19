package kcontext

import (
	"context"
	"log"
	"os"
	"sync"
	"time"
)

const (
	defaultSnapshotRefreshInterval = 5 * time.Second
	defaultFilterStatsTTL          = 5 * time.Second
	maxFilterStatsEntries          = 128
)

func snapshotRefreshInterval() time.Duration {
	raw := os.Getenv("KCONTEXT_SNAPSHOT_INTERVAL")
	if raw == "" {
		return defaultSnapshotRefreshInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultSnapshotRefreshInterval
	}
	return d
}

type filteredAlertView struct {
	alerts                []StoredAlert
	namespaceRanks        []NamespaceAlertRank
	namespaces            []string
	hasEmptyNamespace     bool
	computedAt            time.Time
}

type AlertSnapshotView struct {
	Alerts                []StoredAlert
	NamespaceRanks        []NamespaceAlertRank
	Namespaces            []string
	HasEmptyNamespace     bool
}

type AlertSnapshotCache struct {
	store *AlertStore

	mu       sync.RWMutex
	alerts   []StoredAlert
	loadedAt time.Time

	filterMu    sync.RWMutex
	filterStats map[string]filteredAlertView

	refreshCh chan struct{}
	stopCh    chan struct{}
}

func newAlertSnapshotCache(store *AlertStore) *AlertSnapshotCache {
	return &AlertSnapshotCache{
		store:       store,
		filterStats: make(map[string]filteredAlertView),
		refreshCh:   make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
	}
}

// NewAlertSnapshotCacheForTest builds a snapshot cache without starting the background refresher.
func NewAlertSnapshotCacheForTest(store *AlertStore) *AlertSnapshotCache {
	return newAlertSnapshotCache(store)
}

// StartForTest starts the background refresher (used by tests).
func (c *AlertSnapshotCache) StartForTest() {
	c.start()
}

func (c *AlertSnapshotCache) start() {
	c.refresh(context.Background())

	go func() {
		ticker := time.NewTicker(snapshotRefreshInterval())
		defer ticker.Stop()
		for {
			select {
			case <-c.stopCh:
				return
			case <-ticker.C:
				c.refresh(context.Background())
			case <-c.refreshCh:
				c.refresh(context.Background())
			}
		}
	}()
}

func (c *AlertSnapshotCache) Stop() {
	close(c.stopCh)
}

func (c *AlertSnapshotCache) MarkDirty() {
	select {
	case c.refreshCh <- struct{}{}:
	default:
	}
}

func (c *AlertSnapshotCache) RefreshNow(ctx context.Context) {
	c.refresh(ctx)
}

func (c *AlertSnapshotCache) refresh(ctx context.Context) {
	if c.store == nil {
		return
	}

	loadCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	alerts, err := c.store.List(loadCtx, 0)
	cancel()
	if err != nil {
		log.Printf("snapshot refresh: list alerts: %v", err)
		return
	}

	c.mu.Lock()
	c.alerts = alerts
	c.loadedAt = time.Now()
	c.mu.Unlock()

	c.filterMu.Lock()
	c.filterStats = make(map[string]filteredAlertView)
	c.filterMu.Unlock()
}

func (c *AlertSnapshotCache) Total() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.alerts)
}

func (c *AlertSnapshotCache) getByID(id string) *StoredAlert {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := range c.alerts {
		if c.alerts[i].ID == id {
			a := c.alerts[i]
			return &a
		}
	}
	return nil
}

func (c *AlertSnapshotCache) FilteredView(filters AlertFilters) AlertSnapshotView {
	view := c.filteredView(filters)
	return AlertSnapshotView{
		Alerts:            view.alerts,
		NamespaceRanks:    view.namespaceRanks,
		Namespaces:        view.namespaces,
		HasEmptyNamespace: view.hasEmptyNamespace,
	}
}

func (c *AlertSnapshotCache) filteredView(filters AlertFilters) filteredAlertView {
	key := filters.StatsKey()

	c.filterMu.RLock()
	if view, ok := c.filterStats[key]; ok && time.Since(view.computedAt) < defaultFilterStatsTTL {
		c.filterMu.RUnlock()
		return view
	}
	c.filterMu.RUnlock()

	c.mu.RLock()
	base := c.alerts
	c.mu.RUnlock()

	filtered := FilterAlerts(base, filters)
	view := filteredAlertView{
		alerts:            filtered,
		namespaceRanks:    RankAlertsByNamespace(filtered),
		namespaces:        NamespacesForFilter(filtered, filters.Namespace),
		hasEmptyNamespace: AlertsHaveEmptyNamespace(filtered),
		computedAt:        time.Now(),
	}

	c.filterMu.Lock()
	if len(c.filterStats) >= maxFilterStatsEntries {
		c.filterStats = make(map[string]filteredAlertView)
	}
	c.filterStats[key] = view
	c.filterMu.Unlock()

	return view
}
