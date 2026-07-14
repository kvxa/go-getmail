package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProcessHealthAndSignalShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}

	bin := filepath.Join(t.TempDir(), "go-getmail-test")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cfgPath := filepath.Join(t.TempDir(), "go-getmail.yaml")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cfg := `
DeleteSource: true
ReconnectDelay: 1
HandleDelay: 5
Metrics:
  ListenAddress: "127.0.0.1:` + itoa(port) + `"
Logging:
  Level: info
Accounts:
- Name: proc-test
  HandleDelay: 0
  Source:
    IMAP:
      Server: 127.0.0.1:1
      Username: u
      Password: p
      Mailbox: INBOX
  Target:
    IMAP:
      Server: 127.0.0.1:2
      Username: u
      Password: p
      Mailbox: INBOX
`
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, cfgPath)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = io.MultiWriter(&stderr, os.Stderr)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	base := "http://127.0.0.1:" + itoa(port)
	deadline := time.Now().Add(5 * time.Second)
	var healthzOK bool
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK && strings.Contains(string(body), `"ok"`) {
				healthzOK = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !healthzOK {
		_ = cmd.Process.Kill()
		t.Fatalf("healthz not ready\nstderr=%s\nstdout=%s", stderr.String(), stdout.String())
	}

	resp, err := http.Get(base + "/readyz")
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
	readyBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		_ = cmd.Process.Kill()
		t.Fatalf("readyz want 503 got %d body=%s", resp.StatusCode, readyBody)
	}
	if !strings.Contains(string(readyBody), "degraded") {
		_ = cmd.Process.Kill()
		t.Fatalf("readyz body=%s", readyBody)
	}

	resp, err = http.Get(base + "/metrics")
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
	metricsBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	metrics := string(metricsBody)
	if !strings.Contains(metrics, "mail_account_state_info") {
		_ = cmd.Process.Kill()
		t.Fatalf("metrics missing state_info:\n%s", metrics)
	}
	if !strings.Contains(metrics, `state="initial"`) && !strings.Contains(metrics, `state="connecting"`) {
		_ = cmd.Process.Kill()
		t.Fatalf("metrics missing expected state labels:\n%s", metrics)
	}
	if !strings.Contains(metrics, "mail_account_state{") {
		_ = cmd.Process.Kill()
		t.Fatalf("metrics missing numeric state:\n%s", metrics)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = cmd.Process.Kill()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("exit err=%v stderr=%s", err, stderr.String())
		}
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit after SIGTERM")
	}

	combined := stderr.String() + stdout.String()
	if !strings.Contains(combined, "Shutdown signal received") {
		t.Fatalf("missing shutdown log:\n%s", combined)
	}
	if !strings.Contains(combined, "Shutdown complete") {
		t.Fatalf("missing shutdown complete:\n%s", combined)
	}
	if !strings.Contains(combined, "state_name=") {
		t.Fatalf("missing state_name in logs:\n%s", combined)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
