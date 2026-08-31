package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"pastry/config"
	"pastry/pastebin"
)

// fakeClient lets tests stub every pastebin operation.
type fakeClient struct {
	create  func(opts pastebin.CreateOptions) (string, error)
	readPub func(key string) ([]byte, error)
	readPri func(key string) ([]byte, error)
	del     func(key string) error
	list    func(limit int) ([]pastebin.Paste, error)
	login   func(username, password string) (string, error)
	user    func() (pastebin.User, error)
}

func (f *fakeClient) Create(opts pastebin.CreateOptions) (string, error) {
	if f.create == nil {
		return "", errors.New("unexpected Create call")
	}
	return f.create(opts)
}

func (f *fakeClient) ReadPublic(key string) ([]byte, error) {
	if f.readPub == nil {
		return nil, errors.New("unexpected ReadPublic call")
	}
	return f.readPub(key)
}

func (f *fakeClient) ReadPrivate(key string) ([]byte, error) {
	if f.readPri == nil {
		return nil, errors.New("unexpected ReadPrivate call")
	}
	return f.readPri(key)
}

func (f *fakeClient) Delete(key string) error {
	if f.del == nil {
		return errors.New("unexpected Delete call")
	}
	return f.del(key)
}

func (f *fakeClient) List(limit int) ([]pastebin.Paste, error) {
	if f.list == nil {
		return nil, errors.New("unexpected List call")
	}
	return f.list(limit)
}

func (f *fakeClient) Login(username, password string) (string, error) {
	if f.login == nil {
		return "", errors.New("unexpected Login call")
	}
	return f.login(username, password)
}

func (f *fakeClient) UserDetails() (pastebin.User, error) {
	if f.user == nil {
		return pastebin.User{}, errors.New("unexpected UserDetails call")
	}
	return f.user()
}

type runResult struct {
	code   int
	stdout string
	stderr string
}

func runApp(t *testing.T, stdin io.Reader, client pasteClient, cfg *config.Config, args ...string) runResult {
	t.Helper()
	var out, errb bytes.Buffer
	cli := &CLI{
		stdin:  stdin,
		stdout: &out,
		stderr: &errb,
		cfg:    cfg,
	}
	cli.newClient = func(_, _ string) pasteClient {
		if client == nil {
			t.Fatal("no fake client set for this command")
		}
		return client
	}
	cli.prompt = func(string) (string, error) { return "", errors.New("prompt disabled in test") }
	cli.promptPassword = func(string) (string, error) { return "", errors.New("prompt disabled in test") }
	return runResult{code: cli.run(args), stdout: out.String(), stderr: errb.String()}
}

func loggedInConfig() *config.Config {
	return &config.Config{APIDevKey: "devkey", APIUserKey: "userkey"}
}

func TestParseKey(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"AbC12345", "AbC12345", false},
		{"https://pastebin.com/AbC12345", "AbC12345", false},
		{"https://pastebin.com/raw/AbC12345", "AbC12345", false},
		{"https://pastebin.com/AbC12345/", "AbC12345", false},
		{"https://pastebin.com/AbC12345?source=x", "AbC12345", false},
		{"pastebin.com/AbC12345", "AbC12345", false},
		{"  AbC12345  ", "AbC12345", false},
		{"", "", true},
		{"   ", "", true},
		{"https://pastebin.com/", "", true},
		{"abc!def", "", true},
	}
	for _, tt := range tests {
		got, err := parseKey(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseKey(%q): expected error, got %q", tt.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseKey(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsPiped(t *testing.T) {
	if !isPiped(strings.NewReader("x")) {
		t.Error("a non-file reader should count as piped")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer pr.Close()
	defer pw.Close()
	if !isPiped(pr) {
		t.Error("an os.Pipe should count as piped")
	}
}

func TestRunReadPublic(t *testing.T) {
	fake := &fakeClient{readPub: func(key string) ([]byte, error) {
		if key != "AbC12345" {
			t.Errorf("key = %q", key)
		}
		return []byte("hello paste\n"), nil
	}}
	res := runApp(t, nil, fake, loggedInConfig(), "AbC12345")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if res.stdout != "hello paste\n" {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestRunReadURL(t *testing.T) {
	fake := &fakeClient{readPub: func(key string) ([]byte, error) {
		if key != "rawKey" {
			t.Errorf("key = %q", key)
		}
		return []byte("body"), nil
	}}
	res := runApp(t, nil, fake, nil, "https://pastebin.com/raw/rawKey")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if res.stdout != "body" {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestRunReadFallsBackToPrivate(t *testing.T) {
	var usedPrivate bool
	fake := &fakeClient{
		readPub: func(string) ([]byte, error) {
			return nil, pastebin.ErrNotFound
		},
		readPri: func(key string) ([]byte, error) {
			usedPrivate = true
			if key != "secret" {
				t.Errorf("key = %q", key)
			}
			return []byte("private body"), nil
		},
	}
	res := runApp(t, nil, fake, loggedInConfig(), "secret")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !usedPrivate {
		t.Error("expected private fallback")
	}
	if res.stdout != "private body" {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestRunReadNotFoundNoUserKey(t *testing.T) {
	fake := &fakeClient{readPub: func(string) ([]byte, error) {
		return nil, pastebin.ErrNotFound
	}}
	res := runApp(t, nil, fake, &config.Config{}, "gone")
	if res.code != 1 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "not found") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestRunCreateFromStdin(t *testing.T) {
	var got pastebin.CreateOptions
	fake := &fakeClient{create: func(opts pastebin.CreateOptions) (string, error) {
		got = opts
		return "https://pastebin.com/NewKey", nil
	}}
	res := runApp(t, strings.NewReader("hi there"), fake, loggedInConfig())
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if res.stdout != "https://pastebin.com/NewKey\n" {
		t.Errorf("stdout = %q", res.stdout)
	}
	if got.Code != "hi there" {
		t.Errorf("Code = %q", got.Code)
	}
	if got.Title != "" {
		t.Errorf("Title = %q, want empty for untitled paste", got.Title)
	}
	if got.Private != "1" {
		t.Errorf("Private = %q, want unlisted default", got.Private)
	}
	if got.Expire != "N" {
		t.Errorf("Expire = %q, want N default", got.Expire)
	}
}

func TestRunCreateTitled(t *testing.T) {
	var got pastebin.CreateOptions
	fake := &fakeClient{create: func(opts pastebin.CreateOptions) (string, error) {
		got = opts
		return "https://pastebin.com/Titled", nil
	}}
	res := runApp(t, strings.NewReader("code"), fake, loggedInConfig(),
		"--title", "My Title", "--format", "go", "--expire", "1H")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if got.Title != "My Title" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Format != "go" {
		t.Errorf("Format = %q", got.Format)
	}
	if got.Expire != "1H" {
		t.Errorf("Expire = %q", got.Expire)
	}
	if got.Private != "1" {
		t.Errorf("Private = %q", got.Private)
	}
}

func TestRunCreatePrivateWithoutLogin(t *testing.T) {
	res := runApp(t, strings.NewReader("x"), nil, &config.Config{APIDevKey: "dev"}, "--private")
	if res.code != 1 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "login") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestRunCreateShortFlags(t *testing.T) {
	var got pastebin.CreateOptions
	fake := &fakeClient{create: func(opts pastebin.CreateOptions) (string, error) {
		got = opts
		return "https://pastebin.com/Short", nil
	}}
	res := runApp(t, strings.NewReader("code"), fake, loggedInConfig(),
		"-t", "Short Title", "-f", "go", "-e", "1W")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if got.Title != "Short Title" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Format != "go" {
		t.Errorf("Format = %q", got.Format)
	}
	if got.Expire != "1W" {
		t.Errorf("Expire = %q", got.Expire)
	}
}

func TestRunCreateGuestForcesNoUserKey(t *testing.T) {
	var gotUserKey string
	fake := &fakeClient{create: func(pastebin.CreateOptions) (string, error) {
		return "https://pastebin.com/Guest", nil
	}}

	var out, errb bytes.Buffer
	cli := &CLI{
		stdin:  strings.NewReader("hi"),
		stdout: &out,
		stderr: &errb,
		cfg:    loggedInConfig(), // has a user key
	}
	cli.newClient = func(dev, user string) pasteClient {
		gotUserKey = user
		return fake
	}
	cli.prompt = func(string) (string, error) { return "", errors.New("prompt disabled") }
	cli.promptPassword = func(string) (string, error) { return "", errors.New("prompt disabled") }

	if code := cli.run([]string{"--guest"}); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errb.String())
	}
	if gotUserKey != "" {
		t.Errorf("user key passed to client = %q, want empty for guest paste", gotUserKey)
	}
	if out.String() != "https://pastebin.com/Guest\n" {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunCreateGuestPrivateConflict(t *testing.T) {
	res := runApp(t, strings.NewReader("x"), nil, loggedInConfig(), "--guest", "--private")
	if res.code != 2 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "--guest") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestRunCreateInvalidExpire(t *testing.T) {
	res := runApp(t, strings.NewReader("x"), nil, loggedInConfig(), "--expire", "forever")
	if res.code != 2 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
}

func TestRunCreateNoContent(t *testing.T) {
	res := runApp(t, strings.NewReader("  \n"), nil, loggedInConfig())
	if res.code != 1 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "no content") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestRunCreateMissingDevKey(t *testing.T) {
	fake := &fakeClient{create: func(pastebin.CreateOptions) (string, error) {
		return "should not be called", nil
	}}
	res := runApp(t, strings.NewReader("x"), fake, &config.Config{})
	if res.code != 1 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "api dev key") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestRunDelete(t *testing.T) {
	var deleted string
	fake := &fakeClient{del: func(key string) error {
		deleted = key
		return nil
	}}
	res := runApp(t, nil, fake, loggedInConfig(), "delete", "https://pastebin.com/AbC12345")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if deleted != "AbC12345" {
		t.Errorf("deleted = %q", deleted)
	}
}

func TestRunDeleteRequiresLogin(t *testing.T) {
	res := runApp(t, nil, nil, &config.Config{APIDevKey: "dev"}, "delete", "AbC12345")
	if res.code != 1 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stderr, "login") {
		t.Errorf("stderr = %q", res.stderr)
	}
}

func TestRunList(t *testing.T) {
	var limit int
	fake := &fakeClient{list: func(l int) ([]pastebin.Paste, error) {
		limit = l
		return []pastebin.Paste{
			{Key: "abc", Title: "first", URL: "https://pastebin.com/abc"},
			{Key: "def", Title: "second", URL: "https://pastebin.com/def"},
		}, nil
	}}
	res := runApp(t, nil, fake, loggedInConfig(), "list", "--limit", "10")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if limit != 10 {
		t.Errorf("limit = %d", limit)
	}
	if !strings.Contains(res.stdout, "abc") || !strings.Contains(res.stdout, "second") {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestRunListRequiresLogin(t *testing.T) {
	res := runApp(t, nil, nil, &config.Config{APIDevKey: "dev"}, "list")
	if res.code != 1 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
}

func TestRunWhoami(t *testing.T) {
	fake := &fakeClient{user: func() (pastebin.User, error) {
		return pastebin.User{Name: "wiz_kitty", AccountType: "1", Email: "w@e.com"}, nil
	}}
	res := runApp(t, nil, fake, loggedInConfig(), "whoami")
	if res.code != 0 {
		t.Fatalf("exit = %d, stderr = %q", res.code, res.stderr)
	}
	if !strings.Contains(res.stdout, "wiz_kitty") || !strings.Contains(res.stdout, "account type: 1") {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestRunLogout(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := loggedInConfig()
	var out, errb bytes.Buffer
	cli := &CLI{stdout: &out, stderr: &errb, cfg: cfg}

	if code := cli.run([]string{"logout"}); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errb.String())
	}
	if cfg.APIUserKey != "" {
		t.Errorf("APIUserKey = %q, want empty", cfg.APIUserKey)
	}
	if !strings.Contains(out.String(), "Logged out") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunLogoutNotLoggedIn(t *testing.T) {
	var out, errb bytes.Buffer
	cli := &CLI{stdout: &out, stderr: &errb, cfg: &config.Config{APIDevKey: "dev"}}

	if code := cli.run([]string{"logout"}); code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, errb.String())
	}
	if !strings.Contains(out.String(), "Not logged in") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunVersion(t *testing.T) {
	old := Version
	Version = "9.9.9"
	defer func() { Version = old }()

	res := runApp(t, nil, nil, nil, "--version")
	if res.code != 0 {
		t.Fatalf("exit = %d", res.code)
	}
	if res.stdout != "pastry 9.9.9\n" {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestRunHelp(t *testing.T) {
	res := runApp(t, nil, nil, nil, "--help")
	if res.code != 0 {
		t.Fatalf("exit = %d", res.code)
	}
	if !strings.Contains(res.stdout, "Usage:") {
		t.Errorf("stdout = %q", res.stdout)
	}
}

func TestRunStaleUserKeyHint(t *testing.T) {
	fake := &fakeClient{list: func(int) ([]pastebin.Paste, error) {
		return nil, &pastebin.APIError{Message: "Bad API request, invalid or expired api_user_key"}
	}}
	res := runApp(t, nil, fake, loggedInConfig(), "list")
	if res.code != 1 {
		t.Fatalf("exit = %d", res.code)
	}
	if !strings.Contains(res.stderr, "pastry login") {
		t.Errorf("stderr = %q", res.stderr)
	}
}
