package kcontext

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAlertmanagerPFNamespace  = "openshift-monitoring"
	DefaultAlertmanagerPFService    = "alertmanager-main"
	DefaultAlertmanagerPFLocalPort  = 9094
	DefaultAlertmanagerPFRemotePort = 9094

	defaultPFNamespace  = DefaultAlertmanagerPFNamespace
	defaultPFService    = DefaultAlertmanagerPFService
	defaultPFLocalPort  = DefaultAlertmanagerPFLocalPort
	defaultPFRemotePort = DefaultAlertmanagerPFRemotePort
)

type alertmanagerPortForward struct {
	cmd       *exec.Cmd
	localPort int
	cfg       pfConfig
}

func startAlertmanagerPortForward(cfg pfConfig) (*alertmanagerPortForward, error) {
	oc := resolveOcBinary()
	if oc == "" {
		return nil, fmt.Errorf("oc not found")
	}

	addr := fmt.Sprintf("%d:%d", cfg.localPort, cfg.remotePort)
	args := []string{"port-forward", "-n", cfg.namespace, "svc/" + cfg.service, addr}
	cmd := exec.Command(oc, args...)
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("oc port-forward: %w", err)
	}

	pf := &alertmanagerPortForward{cmd: cmd, localPort: cfg.localPort, cfg: cfg}
	if err := waitForPort(cfg.localPort, 30*time.Second); err != nil {
		pf.stop()
		return nil, fmt.Errorf("port-forward did not become ready on %d: %w", cfg.localPort, err)
	}

	log.Printf("Alertmanager port-forward started: oc %s (localhost:%d)", strings.Join(args, " "), cfg.localPort)
	return pf, nil
}

func maybeStartAlertmanagerPortForward() (*alertmanagerPortForward, error) {
	if !shouldStartAlertmanagerPortForward() {
		return nil, nil
	}

	cfg, err := alertmanagerPortForwardConfig()
	if err != nil {
		return nil, err
	}

	if portListening(cfg.localPort) {
		log.Printf("Alertmanager port %d already listening, skipping oc port-forward", cfg.localPort)
		return nil, nil
	}

	return startAlertmanagerPortForward(cfg)
}

func (pf *alertmanagerPortForward) stop() {
	if pf == nil || pf.cmd == nil || pf.cmd.Process == nil {
		return
	}
	_ = pf.cmd.Process.Kill()
	_, _ = pf.cmd.Process.Wait()
	pf.cmd = nil
}

func (pf *alertmanagerPortForward) restart() (*alertmanagerPortForward, error) {
	if pf == nil {
		return nil, fmt.Errorf("no port-forward to restart")
	}
	cfg := pf.cfg
	pf.stop()
	return startAlertmanagerPortForward(cfg)
}

type pfConfig struct {
	namespace  string
	service    string
	localPort  int
	remotePort int
}

func alertmanagerPortForwardConfig() (pfConfig, error) {
	localPort, err := envIntOr("ALERTMANAGER_PF_LOCAL_PORT", defaultPFLocalPort)
	if err != nil {
		return pfConfig{}, fmt.Errorf("ALERTMANAGER_PF_LOCAL_PORT: %w", err)
	}
	remotePort, err := envIntOr("ALERTMANAGER_PF_REMOTE_PORT", defaultPFRemotePort)
	if err != nil {
		return pfConfig{}, fmt.Errorf("ALERTMANAGER_PF_REMOTE_PORT: %w", err)
	}
	ns := strings.TrimSpace(os.Getenv("ALERTMANAGER_PF_NAMESPACE"))
	if ns == "" {
		ns = defaultPFNamespace
	}
	svc := strings.TrimSpace(os.Getenv("ALERTMANAGER_PF_SERVICE"))
	if svc == "" {
		svc = defaultPFService
	}
	return pfConfig{
		namespace:  ns,
		service:    svc,
		localPort:  localPort,
		remotePort: remotePort,
	}, nil
}

func shouldStartAlertmanagerPortForward() bool {
	if !alertmanagerPollingEnabled() {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv("ALERTMANAGER_PORT_FORWARD"))) {
	case "false", "0", "no", "off":
		return false
	case "true", "1", "yes", "on":
		return isLocalhostAlertmanagerURL(alertmanagerURL())
	}

	// auto (default): start when URL unset (implicit localhost) or explicit localhost URL
	if _, ok := os.LookupEnv("ALERTMANAGER_URL"); !ok {
		return true
	}
	return isLocalhostAlertmanagerURL(alertmanagerURL())
}

func isLocalhostAlertmanagerURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func envIntOr(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("out of range: %d", n)
	}
	return n, nil
}

func portListening(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func waitForPort(port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return context.DeadlineExceeded
}

// AlertmanagerPFConfig holds port-forward target configuration.
type AlertmanagerPFConfig struct {
	Namespace  string
	Service    string
	LocalPort  int
	RemotePort int
}

// ShouldStartAlertmanagerPortForward reports whether auto port-forward should run.
func ShouldStartAlertmanagerPortForward() bool { return shouldStartAlertmanagerPortForward() }

// IsLocalhostAlertmanagerURL reports whether baseURL targets local port-forward.
func IsLocalhostAlertmanagerURL(baseURL string) bool { return isLocalhostAlertmanagerURL(baseURL) }

// AlertmanagerPortForwardConfig reads port-forward env configuration.
func AlertmanagerPortForwardConfig() (AlertmanagerPFConfig, error) {
	cfg, err := alertmanagerPortForwardConfig()
	if err != nil {
		return AlertmanagerPFConfig{}, err
	}
	return AlertmanagerPFConfig{
		Namespace:  cfg.namespace,
		Service:    cfg.service,
		LocalPort:  cfg.localPort,
		RemotePort: cfg.remotePort,
	}, nil
}
