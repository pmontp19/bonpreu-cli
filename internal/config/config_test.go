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

func TestLoadConfigReadsValue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)
	if _, err := EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte(`{"default_max_eur":42.5}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	c, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if c.DefaultMaxEUR != 42.5 {
		t.Fatalf("DefaultMaxEUR = %v, want 42.5", c.DefaultMaxEUR)
	}
}

func TestLoadConfigFrom_ExplicitPathOverridesDefault(t *testing.T) {
	// BONPREU_HOME points somewhere with no config.json; LoadConfigFrom must
	// still read the explicit --config path rather than falling through.
	t.Setenv("BONPREU_HOME", t.TempDir())
	altDir := t.TempDir()
	altPath := filepath.Join(altDir, "alt-config.json")
	if err := os.WriteFile(altPath, []byte(`{"default_max_eur":9.99}`), 0o600); err != nil {
		t.Fatalf("write alt config: %v", err)
	}
	c, err := LoadConfigFrom(altPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if c.DefaultMaxEUR != 9.99 {
		t.Fatalf("DefaultMaxEUR = %v, want 9.99", c.DefaultMaxEUR)
	}
}

func TestLoadConfigFrom_MissingReturnsEmpty(t *testing.T) {
	c, err := LoadConfigFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoadConfigFrom_MalformedJSONErrors(t *testing.T) {
	altPath := filepath.Join(t.TempDir(), "bad-config.json")
	if err := os.WriteFile(altPath, []byte(`{not json`), 0o600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}
	if _, err := LoadConfigFrom(altPath); err == nil {
		t.Fatal("malformed config JSON must error, not return a zero-value config silently")
	}
}

func TestSaveConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)

	cfg := &Config{DefaultMaxEUR: 50, DefaultDestinations: map[string]string{"cc": "pickup-1"}}
	if err := SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.DefaultMaxEUR != 50 || got.DefaultDestinations["cc"] != "pickup-1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestSaveConfigTo_ExplicitPath(t *testing.T) {
	altPath := filepath.Join(t.TempDir(), "alt-config.json")
	cfg := &Config{DefaultDestinations: map[string]string{"home": "addr-1"}}
	if err := SaveConfigTo(altPath, cfg); err != nil {
		t.Fatalf("SaveConfigTo: %v", err)
	}
	got, err := LoadConfigFrom(altPath)
	if err != nil {
		t.Fatalf("LoadConfigFrom: %v", err)
	}
	if got.DefaultDestinations["home"] != "addr-1" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestCacheRoundTripAndMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)

	// Missing cache file yields an initialized, empty cache (not an error).
	c, err := LoadCache()
	if err != nil {
		t.Fatalf("LoadCache (missing): %v", err)
	}
	if c.RetailerToProduct == nil {
		t.Fatal("expected initialized map for missing cache")
	}

	c.RetailerToProduct["20991"] = "uuid-abc"
	if err := SaveCache(c); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	got, err := LoadCache()
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if got.RetailerToProduct["20991"] != "uuid-abc" {
		t.Fatalf("cache round-trip mismatch: %+v", got.RetailerToProduct)
	}

	info, err := os.Stat(filepath.Join(dir, CacheFile))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("cache file perms = %o, want 0600", perm)
	}
}

func TestLoadSessionMissingErrors(t *testing.T) {
	t.Setenv("BONPREU_HOME", t.TempDir())
	if _, err := LoadSession(); err == nil {
		t.Fatal("expected error loading a missing session")
	}
}

func TestDirFallsBackToHome(t *testing.T) {
	t.Setenv("BONPREU_HOME", "")
	d, err := Dir()
	if err != nil {
		t.Fatalf("Dir: %v", err)
	}
	if filepath.Base(d) != "."+DirName {
		t.Errorf("Dir = %q, want it to end in .%s", d, DirName)
	}
}

func TestLoadSessionMalformedErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, SessionFile), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadSession(); err == nil {
		t.Fatal("expected parse error on malformed session")
	}
}

func TestSaveSessionDirCreateError(t *testing.T) {
	// Point BONPREU_HOME at a path *under* a regular file so MkdirAll fails.
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("BONPREU_HOME", filepath.Join(file, "nested"))
	if err := SaveSession(&Session{}); err == nil {
		t.Fatal("expected EnsureDir/MkdirAll error when home is under a file")
	}
}

func TestLoadCacheMalformedErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, CacheFile), []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadCache(); err == nil {
		t.Fatal("expected parse error on malformed cache")
	}
}

func TestLoadConfigMalformedErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected parse error on malformed config")
	}
}

func TestSaveCacheDirCreateError(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("BONPREU_HOME", filepath.Join(file, "nested"))
	if err := SaveCache(&IDCache{}); err == nil {
		t.Fatal("expected error when home is under a file")
	}
}

func TestPathHelpers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BONPREU_HOME", dir)
	for name, fn := range map[string]func() (string, error){
		SessionFile: SessionPath,
		ConfigFile:  ConfigPath,
		CacheFile:   CachePath,
	} {
		p, err := fn()
		if err != nil {
			t.Fatalf("%s path: %v", name, err)
		}
		if want := filepath.Join(dir, name); p != want {
			t.Errorf("%s path = %q, want %q", name, p, want)
		}
	}
}
