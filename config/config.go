package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

type Config struct {
	APIDevKey  string `json:"api_dev_key"`
	APIUserKey string `json:"api_user_key"`
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pastry", "config.json"), nil
}

func Load() (*Config, error) {
	c := &Config{}
	p, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", p, err)
	}
	return c, nil
}

func Save(c *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func (c *Config) HasDevKey() bool {
	return c != nil && c.APIDevKey != ""
}

func (c *Config) HasUserKey() bool {
	return c != nil && c.APIUserKey != ""
}

func (c *Config) DevKey() string {
	return c.APIDevKey
}

func (c *Config) UserKey() string {
	return c.APIUserKey
}

// Prompt asks a question on the terminal and returns the trimmed answer. It
// reads from the controlling terminal when available so that prompting still
// works while stdin is a pipe (e.g. `<command> | pastry`), falling back to stdin.
func Prompt(question string) (string, error) {
	f, owned, err := terminal()
	if err != nil {
		return "", err
	}
	if owned {
		defer f.Close()
	}
	fmt.Fprint(f, question)
	r := bufio.NewReader(f)
	line, err := r.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// PromptPassword asks for a password, suppressing echo.
func PromptPassword(question string) (string, error) {
	f, owned, err := terminal()
	if err != nil {
		return "", err
	}
	if owned {
		defer f.Close()
	}
	fmt.Fprint(f, question)
	password, err := term.ReadPassword(int(f.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Fprintln(f)
	return string(password), nil
}

// terminal returns a handle to the controlling terminal, or falls back to
// stdin. The boolean reports whether the caller owns (and must close) it.
func terminal() (f *os.File, owned bool, err error) {
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return tty, true, nil
	}
	if os.Stdin != nil {
		return os.Stdin, false, nil
	}
	return nil, false, errors.New("no terminal available")
}
