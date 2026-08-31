package pastebin

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const userAgent = "pastry/0.1.0"

// ErrNotFound is returned when a paste cannot be fetched because it is
// missing or not publicly accessible.
var ErrNotFound = errors.New("paste not found or not accessible")

// APIError is a verbatim error message returned by the pastebin API.
type APIError struct {
	Message string
}

func (e *APIError) Error() string {
	return "pastebin: " + e.Message
}

// Client talks to the pastebin.com API (https://pastebin.com/doc_api)
type Client struct {
	DevKey  string
	UserKey string

	hc   *http.Client
	base string
}

// New returns a Client for the given credentials.
func New(devKey, userKey string) *Client {
	return &Client{
		DevKey:  devKey,
		UserKey: userKey,
		hc:      http.DefaultClient,
		base:    "https://pastebin.com",
	}
}

// Create submits a new paste and returns its URL.
func (c *Client) Create(opts CreateOptions) (string, error) {
	if opts.Code == "" {
		return "", errors.New("paste content is empty")
	}

	form := url.Values{}
	form.Set("api_option", "paste")
	form.Set("api_dev_key", c.DevKey)

	userKey := opts.UserKey
	if userKey == "" {
		userKey = c.UserKey
	}
	if userKey != "" {
		form.Set("api_user_key", userKey)
	}

	form.Set("api_paste_code", opts.Code)
	if opts.Title != "" {
		form.Set("api_paste_name", opts.Title)
	}
	if opts.Format != "" {
		form.Set("api_paste_format", opts.Format)
	}
	if opts.Expire != "" {
		form.Set("api_paste_expire_date", opts.Expire)
	}
	if opts.Private != "" {
		form.Set("api_paste_private", opts.Private)
	}
	if opts.Folder != "" {
		form.Set("api_folder_key", opts.Folder)
	}

	body, err := c.postForm(c.base+apiPost, form)
	if err != nil {
		return "", err
	}

	link := strings.TrimSpace(string(body))
	if !strings.HasPrefix(link, "http") {
		return "", newAPIError(link)
	}
	return link, nil
}

// ReadPublic fetches the raw content of a public or unlisted paste.
func (c *Client) ReadPublic(key string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.base+apiRawPaste+key, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pastebin: unexpected status %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

// ReadPrivate fetches the raw content of a private paste.
func (c *Client) ReadPrivate(key string) ([]byte, error) {
	form := url.Values{}
	form.Set("api_option", "show_paste")
	form.Set("api_dev_key", c.DevKey)
	form.Set("api_user_key", c.UserKey)
	form.Set("api_paste_key", key)

	body, err := c.postForm(c.base+apiRaw, form)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// Delete removes a paste owned by the logged-in user.
func (c *Client) Delete(key string) error {
	form := url.Values{}
	form.Set("api_option", "delete")
	form.Set("api_dev_key", c.DevKey)
	form.Set("api_user_key", c.UserKey)
	form.Set("api_paste_key", key)

	body, err := c.postForm(c.base+apiPost, form)
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(string(body)), "paste removed") {
		return newAPIError(strings.TrimSpace(string(body)))
	}
	return nil
}

// List returns the logged-in user's pastes.
func (c *Client) List(limit int) ([]Paste, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	form := url.Values{}
	form.Set("api_option", "list")
	form.Set("api_dev_key", c.DevKey)
	form.Set("api_user_key", c.UserKey)
	form.Set("api_results_limit", strconv.Itoa(limit))

	body, err := c.postForm(c.base+apiPost, form)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(body)) == "No pastes found." {
		return nil, nil
	}

	pastes, err := decodePastes(body)
	if err != nil {
		return nil, fmt.Errorf("pastebin: decode list response: %w", err)
	}
	return pastes, nil
}

// Login exchanges username/password for an api_user_key.
func (c *Client) Login(username, password string) (string, error) {
	form := url.Values{}
	form.Set("api_dev_key", c.DevKey)
	form.Set("api_user_name", username)
	form.Set("api_user_password", password)

	body, err := c.postForm(c.base+apiLogin, form)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(string(body))
	if key == "" {
		return "", errors.New("pastebin: empty login response")
	}
	return key, nil
}

// UserDetails returns the logged-in user's account info.
func (c *Client) UserDetails() (User, error) {
	form := url.Values{}
	form.Set("api_option", "userdetails")
	form.Set("api_dev_key", c.DevKey)
	form.Set("api_user_key", c.UserKey)

	body, err := c.postForm(c.base+apiPost, form)
	if err != nil {
		return User{}, err
	}

	var u User
	if err := xml.Unmarshal(body, &u); err != nil {
		return User{}, fmt.Errorf("pastebin: decode userdetails response: %w", err)
	}
	return u, nil
}

// postForm sends an application/x-www-form-urlencoded POST and returns the
// response body, surfacing pastebin API error messages as *APIError.
func (c *Client) postForm(endpoint string, form url.Values) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if isAPIError(body) {
		return nil, newAPIError(strings.TrimSpace(string(body)))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pastebin: unexpected status %d: %s", resp.StatusCode, body)
	}
	return body, nil
}

func isAPIError(body []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(body)), "Bad API request")
}

// decodePastes parses a list API response into Paste entries. Pastebin
// returns the entries as consecutive <paste> root elements (no wrapping
// root), so we walk the token stream rather than Unmarshal a single struct.
func decodePastes(body []byte) ([]Paste, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))
	var pastes []Paste
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "paste" {
			continue
		}
		var p Paste
		if err := dec.DecodeElement(&p, &se); err != nil {
			return nil, err
		}
		pastes = append(pastes, p)
	}
	return pastes, nil
}

func newAPIError(msg string) error {
	return &APIError{Message: msg}
}
