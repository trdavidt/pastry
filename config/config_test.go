package config

import (
	"os"
	"strings"
	"testing"
)

// setConfigHome points os.UserConfigDir at a fresh temp dir.
func setConfigHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func TestPath(t *testing.T) {
	setConfigHome(t)
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !strings.HasSuffix(p, "pastry/config.json") {
		t.Errorf("Path = %q, want .../pastry/config.json", p)
	}
}

func TestSaveLoad(t *testing.T) {
	setConfigHome(t)
	want := &Config{APIDevKey: "dev", APIUserKey: "user"}
	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.APIDevKey != "dev" || got.APIUserKey != "user" {
		t.Errorf("got %+v", got)
	}
}

func TestLoadMissing(t *testing.T) {
	setConfigHome(t)
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.APIDevKey != "" || got.APIUserKey != "" {
		t.Errorf("got %+v, want empty", got)
	}
}

func TestSaveSets0600(t *testing.T) {
	home := setConfigHome(t)
	if err := Save(&Config{APIDevKey: "dev"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if !strings.HasPrefix(p, home) {
		t.Fatalf("config not written under test dir: %q", p)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config mode = %o, want 600", perm)
	}
}

func TestHasKeys(t *testing.T) {
	var c *Config
	if c.HasDevKey() || c.HasUserKey() {
		t.Error("nil config should not report keys")
	}
	c = &Config{APIDevKey: "d"}
	if !c.HasDevKey() || c.HasUserKey() {
		t.Error("expected only dev key")
	}
	if c.DevKey() != "d" {
		t.Errorf("DevKey = %q", c.DevKey())
	}
}
