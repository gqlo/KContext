package kcontext_test

import (
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gqlo/kcontext"
	"github.com/redis/go-redis/v9"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

func testStore(t *testing.T) *kcontext.AlertStore {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return kcontext.NewAlertStoreWithRedis(rdb)
}

func testServer(t *testing.T) *kcontext.Server {
	t.Helper()
	return kcontext.NewServer(testStore(t), "", "")
}

func sampleAlert(id, severity, status, source, ns, alertname string, received time.Time) kcontext.StoredAlert {
	return kcontext.StoredAlert{
		ID:         id,
		ReceivedAt: received,
		Source:     source,
		Status:     status,
		Labels: map[string]string{
			"severity":  severity,
			"alertname": alertname,
			"namespace": ns,
		},
		Annotations: map[string]string{"summary": "test"},
	}
}
