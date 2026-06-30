package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadSessionRoundTripAndPerms(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)

	s := &Session{
		Cookies:                  map[string]string{"VISITORID": "abc", "global_sid": "xyz"},
		CSRFToken:                "csrf-1",
		RegionID:                 "r1",
		EcomRequestSourceVersion: "2.0.0-x",
	}
	if err := SaveSession(s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := LoadSession()
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if got.Cookies["VISITORID"] != "abc" || got.CSRFToken != "csrf-1" || got.RegionID != "r1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.EcomRequestSource != SourceWeb {
		t.Fatalf("expected default source %q, got %q", SourceWeb, got.EcomRequestSource)
	}

	info, err := os.Stat(filepath.Join(dir, SessionFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("session file perms = %o, want 0600", perm)
	}
}

func TestLoadConfigMissingReturnsEmpty(t *testing.T) {
	t.Setenv("BONPREU_HOME", t.TempDir())
	c, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil config")
	}
}
