package kcontext

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
