package kcontext

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var (
	clusterAuthMu     sync.RWMutex
	clusterAuthToken  string
)

// SetClusterAuthToken stores the bearer token used for oc cluster queries.
func SetClusterAuthToken(token string) {
	clusterAuthMu.Lock()
	clusterAuthToken = strings.TrimSpace(token)
	clusterAuthMu.Unlock()
}

func clusterAuthTokenValue() string {
	clusterAuthMu.RLock()
	defer clusterAuthMu.RUnlock()
	return clusterAuthToken
}

func resolveOcBinary() string {
	if p, err := exec.LookPath("oc"); err == nil {
		return p
	}
	for _, p := range []string{"/usr/bin/oc", "/usr/local/bin/oc", "/bin/oc"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func runOcCommand(ctx context.Context, name string, args ...string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("oc binary not found")
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()

	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if ee, ok := err.(*exec.ExitError); ok {
			if stderr := strings.TrimSpace(string(ee.Stderr)); stderr != "" {
				msg = stderr
			}
		}
		if msg != "" {
			return "", fmt.Errorf("%v: %s", err, msg)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// runOcLoggedIn runs oc with the current environment, falling back to a login
// shell so KUBECONFIG from interactive profiles is available.
func runOcLoggedIn(ctx context.Context, args ...string) (string, error) {
	oc := resolveOcBinary()
	if oc == "" {
		return "", fmt.Errorf("oc binary not found")
	}

	if token := clusterAuthTokenValue(); token != "" {
		if out, err := runOcCommand(ctx, oc, append([]string{"--token=" + token}, args...)...); err == nil {
			return out, nil
		}
	}

	if out, err := runOcCommand(ctx, oc, args...); err == nil {
		return out, nil
	}

	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuoteArg(arg)
	}
	tokenPrefix := ""
	if token := clusterAuthTokenValue(); token != "" {
		tokenPrefix = "--token=" + shellQuoteArg(token) + " "
	}
	return runOcCommand(ctx, "/bin/bash", "-lc", "oc "+tokenPrefix+strings.Join(quoted, " "))
}
