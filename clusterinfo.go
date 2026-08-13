package kcontext

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const clusterMetaTTL = 5 * time.Minute

// ClusterMeta holds OpenShift cluster summary info for the dashboard sidebar.
type ClusterMeta struct {
	MasterNodes string
	WorkerNodes string
	OCPVersion  string
	CNVVersion  string
	ODFVersion  string
}

func (m ClusterMeta) empty() bool {
	return m.MasterNodes == "" && m.WorkerNodes == "" && m.OCPVersion == "" && m.CNVVersion == "" && m.ODFVersion == ""
}

// NodesDisplay formats master and worker counts for the sidebar.
func (m ClusterMeta) NodesDisplay() string {
	var parts []string
	if m.MasterNodes != "" {
		parts = append(parts, m.MasterNodes+" master")
	}
	if m.WorkerNodes != "" {
		parts = append(parts, m.WorkerNodes+" worker")
	}
	return strings.Join(parts, " · ")
}

type csvLookup struct {
	namespace string
	label     string
}

var odfCSVLookups = []csvLookup{
	{namespace: "openshift-storage", label: "operators.coreos.com/odf-operator.openshift-storage"},
	{namespace: "openshift-storage", label: "operators.coreos.com/ocs-operator.openshift-storage"},
	{namespace: "openshift-storage", label: "operators.coreos.com/odf.rhods-operator.openshift-storage"},
}

// FetchClusterMeta queries the cluster via oc for node count and component versions.
func FetchClusterMeta(ctx context.Context) ClusterMeta {
	oc := resolveOcBinary()
	if oc == "" {
		return ClusterMeta{}
	}

	var meta ClusterMeta
	var wg sync.WaitGroup
	var mu sync.Mutex

	set := func(fn func() string, dst *string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v := fn(); v != "" {
				mu.Lock()
				*dst = v
				mu.Unlock()
			}
		}()
	}

	set(func() string { return fetchOCPVersion(ctx) }, &meta.OCPVersion)
	set(func() string {
		return fetchCSVVersion(ctx, "openshift-cnv", "operators.coreos.com/kubevirt-hyperconverged.openshift-cnv")
	}, &meta.CNVVersion)
	set(func() string { return fetchODFVersion(ctx) }, &meta.ODFVersion)

	masters, workers := fetchNodeCounts(ctx)
	if masters > 0 {
		meta.MasterNodes = strconv.Itoa(masters)
	}
	if workers > 0 {
		meta.WorkerNodes = strconv.Itoa(workers)
	}

	wg.Wait()
	return meta
}

func fetchNodeCounts(ctx context.Context) (masters, workers int) {
	out, err := runOcLoggedIn(ctx, "get", "nodes", "--no-headers")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		roles := fields[2]
		if nodeHasRole(roles, "master") || nodeHasRole(roles, "control-plane") {
			masters++
		}
		if nodeHasRole(roles, "worker") {
			workers++
		}
	}
	return masters, workers
}

func nodeHasRole(roles, role string) bool {
	for _, r := range strings.Split(roles, ",") {
		if strings.TrimSpace(r) == role {
			return true
		}
	}
	return false
}

func fetchOCPVersion(ctx context.Context) string {
	out, err := runOcLoggedIn(ctx, "get", "clusterversion", "version",
		"-o", "jsonpath={.status.desired.version}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func fetchCSVVersion(ctx context.Context, namespace, label string) string {
	out, err := runOcLoggedIn(ctx, "get", "csv", "-n", namespace,
		"-l", label,
		"-o", "jsonpath={.items[0].spec.version}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func fetchODFVersion(ctx context.Context) string {
	for _, lookup := range odfCSVLookups {
		if v := fetchCSVVersion(ctx, lookup.namespace, lookup.label); v != "" {
			return v
		}
	}
	return ""
}

func (s *Server) cachedClusterMeta() ClusterMeta {
	s.clusterMetaMu.RLock()
	if !s.clusterMetaAt.IsZero() && time.Since(s.clusterMetaAt) < clusterMetaTTL {
		meta := s.clusterMeta
		s.clusterMetaMu.RUnlock()
		return meta
	}
	stale := s.clusterMeta
	s.clusterMetaMu.RUnlock()

	s.triggerClusterMetaRefresh()
	return stale
}

func (s *Server) refreshClusterMetaLoop() {
	s.refreshClusterMeta()

	ticker := time.NewTicker(clusterMetaTTL)
	defer ticker.Stop()
	for range ticker.C {
		s.refreshClusterMeta()
	}
}

func (s *Server) triggerClusterMetaRefresh() {
	go s.refreshClusterMeta()
}

func (s *Server) refreshClusterMeta() {
	if !atomic.CompareAndSwapInt32(&s.clusterMetaRefreshing, 0, 1) {
		return
	}
	defer atomic.StoreInt32(&s.clusterMetaRefreshing, 0)

	metaCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ensureClusterAuthToken(); err != nil {
		log.Printf("cluster meta: token: %v", err)
	}

	meta := FetchClusterMeta(metaCtx)
	if meta.empty() {
		if resolveOcBinary() != "" {
			log.Print("cluster meta: oc queries returned no data (check oc login and permissions)")
		}
		return
	}

	s.clusterMetaMu.Lock()
	s.clusterMeta = meta
	s.clusterMetaAt = time.Now()
	s.clusterMetaMu.Unlock()
}
