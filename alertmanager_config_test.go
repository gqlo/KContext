package kcontext

import (
	"os"
	"testing"
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

func TestAlertmanagerURL_default(t *testing.T) {
	unsetEnv(t, "ALERTMANAGER_URL")
	if got := alertmanagerURL(); got != defaultAlertmanagerURL {
		t.Fatalf("alertmanagerURL() = %q, want %q", got, defaultAlertmanagerURL)
	}
}

func TestAlertmanagerPollingEnabled(t *testing.T) {
	t.Run("enabled when unset", func(t *testing.T) {
		unsetEnv(t, "ALERTMANAGER_URL")
		if !alertmanagerPollingEnabled() {
			t.Fatal("alertmanagerPollingEnabled() = false, want true when ALERTMANAGER_URL unset")
		}
	})

	t.Run("disabled when empty", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "")
		if alertmanagerPollingEnabled() {
			t.Fatal("alertmanagerPollingEnabled() = true, want false when ALERTMANAGER_URL=\"\"")
		}
	})

	t.Run("enabled when set", func(t *testing.T) {
		t.Setenv("ALERTMANAGER_URL", "https://alertmanager.example:9094")
		if !alertmanagerPollingEnabled() {
			t.Fatal("alertmanagerPollingEnabled() = false, want true when ALERTMANAGER_URL is set")
		}
	})
}

func TestAlertmanagerTLSInsecure_default(t *testing.T) {
	unsetEnv(t, "ALERTMANAGER_TLS_INSECURE")
	if !alertmanagerTLSInsecure() {
		t.Fatal("alertmanagerTLSInsecure() = false, want true by default")
	}
}

func TestAlertmanagerTLSInsecure_explicit(t *testing.T) {
	t.Setenv("ALERTMANAGER_TLS_INSECURE", "false")
	if alertmanagerTLSInsecure() {
		t.Fatal("alertmanagerTLSInsecure() = true, want false when set to false")
	}
}

func TestAlertmanagerToken_fromEnv(t *testing.T) {
	unsetEnv(t, "ALERTMANAGER_TOKEN_FILE")
	t.Setenv("ALERTMANAGER_TOKEN", "test-bearer-token")

	token, source, err := alertmanagerToken()
	if err != nil {
		t.Fatalf("alertmanagerToken() error: %v", err)
	}
	if token != "test-bearer-token" {
		t.Fatalf("token = %q, want %q", token, "test-bearer-token")
	}
	if source != "ALERTMANAGER_TOKEN" {
		t.Fatalf("source = %q, want ALERTMANAGER_TOKEN", source)
	}
}
