package pastebin

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// testClient spins up an httptest server and points a Client at it.
func testClient(t *testing.T, h http.Handler) *Client {
	return testClientWithKeys(t, h, "devkey", "userkey")
}

func testClientWithKeys(t *testing.T, h http.Handler, devKey, userKey string) *Client {
	t.Helper()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	c := New(devKey, userKey)
	c.hc = ts.Client()
	c.base = ts.URL
	return c
}

// readForm parses a form body, asserting the content type.
func readForm(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
		t.Fatalf("content type = %q, want form-encoded", ct)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		t.Fatalf("parse form: %v", err)
	}
	return form
}

func TestCreateGuest(t *testing.T) {
	c := testClientWithKeys(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q, want POST", r.Method)
		}
		if got := r.URL.Path; got != "/api/api_post.php" {
			t.Fatalf("path = %q", got)
		}
		form := readForm(t, r)
		if got := form.Get("api_option"); got != "paste" {
			t.Errorf("api_option = %q", got)
		}
		if got := form.Get("api_dev_key"); got != "devkey" {
			t.Errorf("api_dev_key = %q", got)
		}
		if got := form.Get("api_paste_code"); got != "hello" {
			t.Errorf("api_paste_code = %q", got)
		}
		if _, ok := form["api_user_key"]; ok {
			t.Error("guest paste must not send api_user_key")
		}
		w.Write([]byte("https://pastebin.com/AbC12345"))
	}), "devkey", "")

	got, err := c.Create(CreateOptions{Code: "hello"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if want := "https://pastebin.com/AbC12345"; got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}

func TestCreateUserTitled(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := readForm(t, r)
		if got := form.Get("api_user_key"); got != "userkey" {
			t.Errorf("api_user_key = %q", got)
		}
		if got := form.Get("api_paste_name"); got != "my title" {
			t.Errorf("api_paste_name = %q", got)
		}
		if got := form.Get("api_paste_format"); got != "go" {
			t.Errorf("api_paste_format = %q", got)
		}
		if got := form.Get("api_paste_expire_date"); got != "1H" {
			t.Errorf("api_paste_expire_date = %q", got)
		}
		if got := form.Get("api_paste_private"); got != "2" {
			t.Errorf("api_paste_private = %q", got)
		}
		w.Write([]byte("https://pastebin.com/xyz98765"))
	}))

	got, err := c.Create(CreateOptions{
		Title:   "my title",
		Format:  "go",
		Expire:  "1H",
		Private: "2",
		Code:    "package main",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if want := "https://pastebin.com/xyz98765"; got != want {
		t.Errorf("url = %q, want %q", got, want)
	}
}

// TestCreateEncodesForm verifies the exact wire encoding matches what curl
// -d / PHP urlencode would send: spaces as '+', '&' and '=' percent-encoded,
// UTF-8 bytes percent-encoded.
func TestCreateEncodesForm(t *testing.T) {
	var rawBody string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
		}
		b, _ := io.ReadAll(r.Body)
		rawBody = string(b)
		w.Write([]byte("https://pastebin.com/abc12345"))
	}))

	if _, err := c.Create(CreateOptions{
		Title: "a b&c=d é",
		Code:  "x & y = z",
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	for _, frag := range []string{
		"api_option=paste",
		"api_dev_key=devkey",
		"api_paste_name=a+b%26c%3Dd+%C3%A9",
		"api_paste_code=x+%26+y+%3D+z",
	} {
		if !strings.Contains(rawBody, frag) {
			t.Errorf("body %q missing %q", rawBody, frag)
		}
	}
	if strings.Contains(rawBody, "a b&c=d") || strings.Contains(rawBody, "é") {
		t.Errorf("body contains raw (unencoded) characters: %q", rawBody)
	}
}

func TestCreateEmptyCode(t *testing.T) {
	c := New("devkey", "")
	if _, err := c.Create(CreateOptions{}); err == nil {
		t.Fatal("Create with empty code should error before any HTTP request")
	}
}

func TestCreateAPIError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Bad API request, api_paste_code was empty"))
	}))

	_, err := c.Create(CreateOptions{Code: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	if !strings.Contains(apiErr.Message, "api_paste_code was empty") {
		t.Errorf("message = %q", apiErr.Message)
	}
}

func TestReadPublic(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %q, want GET", r.Method)
		}
		if got := r.URL.Path; got != "/raw/AbC12345" {
			t.Fatalf("path = %q", got)
		}
		w.Write([]byte("raw paste content\n"))
	}))

	got, err := c.ReadPublic("AbC12345")
	if err != nil {
		t.Fatalf("ReadPublic: %v", err)
	}
	if want := "raw paste content\n"; string(got) != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestReadPublicNotFound(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	_, err := c.ReadPublic("gone")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestReadPrivate(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/api_raw.php" {
			t.Fatalf("path = %q", got)
		}
		form := readForm(t, r)
		if got := form.Get("api_option"); got != "show_paste" {
			t.Errorf("api_option = %q", got)
		}
		if got := form.Get("api_paste_key"); got != "secret" {
			t.Errorf("api_paste_key = %q", got)
		}
		if got := form.Get("api_user_key"); got != "userkey" {
			t.Errorf("api_user_key = %q", got)
		}
		w.Write([]byte("private content"))
	}))

	got, err := c.ReadPrivate("secret")
	if err != nil {
		t.Fatalf("ReadPrivate: %v", err)
	}
	if want := "private content"; string(got) != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestDelete(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := readForm(t, r)
		if got := form.Get("api_option"); got != "delete" {
			t.Errorf("api_option = %q", got)
		}
		if got := form.Get("api_paste_key"); got != "AbC12345" {
			t.Errorf("api_paste_key = %q", got)
		}
		w.Write([]byte("Paste Removed"))
	}))

	if err := c.Delete("AbC12345"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestDeleteAPIError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Bad API request, invalid permission to remove paste"))
	}))

	err := c.Delete("nope")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
}

func TestList(t *testing.T) {
	const sample = `<paste>
<paste_key>0b42rwhf</paste_key>
<paste_date>1297953260</paste_date>
<paste_title>javascript test</paste_title>
<paste_size>15</paste_size>
<paste_expire_date>1297956860</paste_expire_date>
<paste_private>0</paste_private>
<paste_format_long>JavaScript</paste_format_long>
<paste_format_short>javascript</paste_format_short>
<paste_url>https://pastebin.com/0b42rwhf</paste_url>
<paste_hits>15</paste_hits>
</paste>
<paste>
<paste_key>0C343n0d</paste_key>
<paste_title>Welcome To Pastebin V3</paste_title>
<paste_url>https://pastebin.com/0C343n0d</paste_url>
</paste>`

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := readForm(t, r)
		if got := form.Get("api_option"); got != "list" {
			t.Errorf("api_option = %q", got)
		}
		if got := form.Get("api_results_limit"); got != "100" {
			t.Errorf("api_results_limit = %q", got)
		}
		w.Write([]byte(sample))
	}))

	pastes, err := c.List(100)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pastes) != 2 {
		t.Fatalf("len = %d, want 2", len(pastes))
	}
	if got := pastes[0].Key; got != "0b42rwhf" {
		t.Errorf("first key = %q", got)
	}
	if got := pastes[0].Title; got != "javascript test" {
		t.Errorf("first title = %q", got)
	}
	if got := pastes[1].Title; got != "Welcome To Pastebin V3" {
		t.Errorf("second title = %q", got)
	}
}

func TestListNoPastes(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("No pastes found."))
	}))

	pastes, err := c.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if pastes != nil {
		t.Errorf("pastes = %v, want nil", pastes)
	}
}

func TestListAPIError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Bad API request, invalid api_user_key"))
	}))

	if _, err := c.List(50); err == nil {
		t.Fatal("expected error")
	}
}

func TestListDefaults(t *testing.T) {
	var gotLimit string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := readForm(t, r)
		gotLimit = form.Get("api_results_limit")
		w.Write([]byte("No pastes found."))
	}))

	if _, err := c.List(0); err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotLimit != "50" {
		t.Errorf("default api_results_limit = %q, want 50", gotLimit)
	}
}

func TestLogin(t *testing.T) {
	const key = "6c6d3fe13b19bbd6e479b705df0a607f"
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Path; got != "/api/api_login.php" {
			t.Fatalf("path = %q", got)
		}
		form := readForm(t, r)
		if got := form.Get("api_dev_key"); got != "devkey" {
			t.Errorf("api_dev_key = %q", got)
		}
		if got := form.Get("api_user_name"); got != "alice" {
			t.Errorf("api_user_name = %q", got)
		}
		if got := form.Get("api_user_password"); got != "s3cret" {
			t.Errorf("api_user_password = %q", got)
		}
		w.Write([]byte(key))
	}))

	got, err := c.Login("alice", "s3cret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got != key {
		t.Errorf("key = %q, want %q", got, key)
	}
}

func TestLoginAPIError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Bad API request, invalid login"))
	}))

	_, err := c.Login("alice", "wrong")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T, want *APIError", err)
	}
	if apiErr.Message != "Bad API request, invalid login" {
		t.Errorf("Message = %q, want %q", apiErr.Message, "Bad API request, invalid login")
	}
}

func TestLoginAcceptsAnyNonEmptyKey(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("totally-not-a-hex-key"))
	}))

	got, err := c.Login("alice", "s3cret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got != "totally-not-a-hex-key" {
		t.Errorf("key = %q", got)
	}
}

func TestLoginEmptyResponse(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("   \n"))
	}))

	if _, err := c.Login("alice", "s3cret"); err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestUserDetails(t *testing.T) {
	const sample = `<user>
<user_name>wiz_kitty</user_name>
<user_format_short>text</user_format_short>
<user_expiration>N</user_expiration>
<user_avatar_url>https://pastebin.com/cache/a/1.jpg</user_avatar_url>
<user_private>1</user_private>
<user_website>https://myawesomesite.com</user_website>
<user_email>wiz@example.com</user_email>
<user_location>New York</user_location>
<user_account_type>0</user_account_type>
</user>`

	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form := readForm(t, r)
		if got := form.Get("api_option"); got != "userdetails" {
			t.Errorf("api_option = %q", got)
		}
		w.Write([]byte(sample))
	}))

	u, err := c.UserDetails()
	if err != nil {
		t.Fatalf("UserDetails: %v", err)
	}
	if u.Name != "wiz_kitty" {
		t.Errorf("Name = %q", u.Name)
	}
	if u.AccountType != "0" {
		t.Errorf("AccountType = %q", u.AccountType)
	}
	if u.Email != "wiz@example.com" {
		t.Errorf("Email = %q", u.Email)
	}
}

func TestUserDetailsError(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Bad API request, invalid api_user_key"))
	}))

	if _, err := c.UserDetails(); err == nil {
		t.Fatal("expected error")
	}
}

func TestListXMLDecodeInline(t *testing.T) {
	// Pastebin returns consecutive <paste> roots with no wrapping element.
	body := `<paste><paste_key>abc</paste_key><paste_title>T</paste_title></paste>
<paste><paste_key>def</paste_key><paste_title>U</paste_title></paste>`
	pastes, err := decodePastes([]byte(body))
	if err != nil {
		t.Fatalf("decodePastes: %v", err)
	}
	if len(pastes) != 2 {
		t.Fatalf("len = %d, want 2", len(pastes))
	}
	if pastes[0].Key != "abc" || pastes[0].Title != "T" {
		t.Errorf("first = %+v", pastes[0])
	}
	if pastes[1].Key != "def" || pastes[1].Title != "U" {
		t.Errorf("second = %+v", pastes[1])
	}
}
