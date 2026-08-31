package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"pastry/config"
	"pastry/pastebin"
)

// Integration tests drive the real CLI (Run) against the live pastebin.com
// API. They are skipped unless PASTRY_DEV_KEY, PASTRY_USERNAME, and
// PASTRY_PASSWORD are set, and must be run against a dedicated test account
// (login invalidates any prior user key for the account).
//
//   PASTRY_DEV_KEY=... PASTRY_USERNAME=... PASTRY_PASSWORD=... \
//     go test -run Integration -v ./cli/

func integrationCredentials(t *testing.T) (devKey, username, password string) {
	t.Helper()
	devKey = os.Getenv("PASTRY_DEV_KEY")
	username = os.Getenv("PASTRY_USERNAME")
	password = os.Getenv("PASTRY_PASSWORD")
	if devKey == "" || username == "" || password == "" {
		t.Skip("PASTRY_DEV_KEY/PASTRY_USERNAME/PASTRY_PASSWORD not set; skipping integration test")
	}
	return devKey, username, password
}

// setupIntegrationConfig logs in (direct client call, since the CLI login is
// interactive) and writes a real config file under a temp XDG_CONFIG_HOME so
// the CLI can load credentials for the remaining operations.
func setupIntegrationConfig(t *testing.T, devKey, username, password string) {
	t.Helper()
	userKey, err := pastebin.New(devKey, "").Login(username, password)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save(&config.Config{APIDevKey: devKey, APIUserKey: userKey}); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func runCLI(t *testing.T, args []string, stdin string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = Run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

// deleteWithRetry removes a paste via the CLI, retrying up to 5 attempts with
// exponential backoff (1s, 2s, 4s, 8s) so a transient failure doesn't leave a
// paste behind.
func deleteWithRetry(t *testing.T, key string) {
	t.Helper()
	var lastErr string
	for attempt := 0; attempt < 5; attempt++ {
		code, _, stderr := runCLI(t, []string{"delete", key}, "")
		if code == 0 {
			return
		}
		lastErr = stderr
		if attempt < 4 {
			time.Sleep(time.Duration(1<<uint(attempt)) * time.Second)
		}
	}
	t.Errorf("delete %s failed after 5 attempts: %s", key, lastErr)
}

func TestIntegrationCLICreateReadDelete(t *testing.T) {
	devKey, username, password := integrationCredentials(t)
	setupIntegrationConfig(t, devKey, username, password)

	const body = "pastry integration test content"
	code, out, stderr := runCLI(t, []string{"--title", "pastry integration"}, body)
	if code != 0 {
		t.Fatalf("create exit %d: %s", code, stderr)
	}
	link := strings.TrimSpace(out)
	key, err := parseKey(link)
	if err != nil {
		t.Fatalf("parse key from %q: %v", link, err)
	}
	t.Cleanup(func() { deleteWithRetry(t, key) })

	code, out, stderr = runCLI(t, []string{link}, "")
	if code != 0 {
		t.Fatalf("read exit %d: %s", code, stderr)
	}
	if out != body {
		t.Errorf("read = %q, want %q", out, body)
	}
}

func TestIntegrationCLIPrivateRead(t *testing.T) {
	devKey, username, password := integrationCredentials(t)
	setupIntegrationConfig(t, devKey, username, password)

	const body = "pastry integration private content"
	code, out, stderr := runCLI(t, []string{"--private"}, body)
	if code != 0 {
		t.Fatalf("create private exit %d: %s", code, stderr)
	}
	link := strings.TrimSpace(out)
	key, err := parseKey(link)
	if err != nil {
		t.Fatalf("parse key from %q: %v", link, err)
	}
	t.Cleanup(func() { deleteWithRetry(t, key) })

	code, out, stderr = runCLI(t, []string{link}, "")
	if code != 0 {
		t.Fatalf("read private exit %d: %s", code, stderr)
	}
	if out != body {
		t.Errorf("read private = %q, want %q", out, body)
	}
}
