package kcontext

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultKContextNamespace     = "kcontext"
	DefaultKContextSA            = "kcontext"
	DefaultKContextTokenDuration = "8760h"
)

type saTokenConfig struct {
	namespace string
	sa        string
	duration  string
	deployDir string
}

func saTokenConfigFromEnv() saTokenConfig {
	deployDir := strings.TrimSpace(os.Getenv("KCONTEXT_DEPLOY_DIR"))
	if deployDir == "" {
		deployDir = openshiftDeployDir()
	}

	return saTokenConfig{
		namespace: envOrDefault("KCONTEXT_NAMESPACE", DefaultKContextNamespace),
		sa:        envOrDefault("KCONTEXT_SA", DefaultKContextSA),
		duration:  envOrDefault("KCONTEXT_TOKEN_DURATION", DefaultKContextTokenDuration),
		deployDir: deployDir,
	}
}

func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func openshiftDeployDir() string {
	if dir := strings.TrimSpace(os.Getenv("KCONTEXT_DEPLOY_DIR")); dir != "" {
		return dir
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	dir := cwd
	for {
		candidate := filepath.Join(dir, "deploy", "openshift")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return filepath.Join(cwd, "deploy", "openshift")
}

func ensureOpenShiftRBAC(ctx context.Context, deployDir string) error {
	if deployDir == "" {
		return fmt.Errorf("openshift deploy dir not found (set KCONTEXT_DEPLOY_DIR)")
	}
	if _, err := os.Stat(deployDir); err != nil {
		return fmt.Errorf("openshift deploy dir %q: %w", deployDir, err)
	}

	oc := resolveOcBinary()
	if oc == "" {
		return fmt.Errorf("oc binary not found")
	}

	_, err := runOcCommand(ctx, oc, "apply", "-f", deployDir)
	return err
}

func mintServiceAccountToken(ctx context.Context, cfg saTokenConfig) (string, error) {
	oc := resolveOcBinary()
	if oc == "" {
		return "", fmt.Errorf("oc binary not found")
	}

	token, err := runOcCommand(ctx, oc, "create", "token", cfg.sa,
		"-n", cfg.namespace,
		"--duration="+cfg.duration,
	)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("oc create token returned empty token")
	}
	return token, nil
}

func resolveAutoServiceAccountToken() (string, error) {
	cfg := saTokenConfigFromEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	log.Printf("Applying OpenShift RBAC from %s...", cfg.deployDir)
	if err := ensureOpenShiftRBAC(ctx, cfg.deployDir); err != nil {
		return "", fmt.Errorf("apply RBAC: %w", err)
	}

	log.Printf("Creating token for serviceaccount/%s in %s (duration=%s)...",
		cfg.sa, cfg.namespace, cfg.duration)
	token, err := mintServiceAccountToken(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("create token: %w", err)
	}
	return token, nil
}

func ensureClusterAuthToken() error {
	if clusterAuthTokenValue() != "" {
		return nil
	}
	if path := strings.TrimSpace(os.Getenv("ALERTMANAGER_TOKEN_FILE")); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		SetClusterAuthToken(string(b))
		return nil
	}
	token, err := resolveAutoServiceAccountToken()
	if err != nil {
		return err
	}
	SetClusterAuthToken(token)
	return nil
}

// OpenShiftDeployDir returns the directory passed to oc apply -f for RBAC manifests.
func OpenShiftDeployDir() string {
	return saTokenConfigFromEnv().deployDir
}

// MintServiceAccountToken applies RBAC manifests and mints a token (for tests).
func MintServiceAccountToken() (string, error) {
	return resolveAutoServiceAccountToken()
}
