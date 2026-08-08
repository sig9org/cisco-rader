package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStatePath(t *testing.T) {
	tests := map[string]string{
		"sites.yml":                    "sites_state.yml",
		"site.yaml":                    "site_state.yaml",
		filepath.Join("etc", "my.yml"): filepath.Join("etc", "my_state.yml"),
	}
	for input, want := range tests {
		if got := StatePath(input); got != want {
			t.Errorf("StatePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sites.yml")
	data := []byte("settings:\n  mention: ' user@example.com '\nsites:\n  - name: Test Product\n    url: https://software.cisco.com/example\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Sites) != 1 || cfg.Sites[0].Name != "Test Product" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.Settings.Mention != "user@example.com" {
		t.Fatalf("mention = %q", cfg.Settings.Mention)
	}
}

func TestLoadRejectsNonHTTPSURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sites.yml")
	if err := os.WriteFile(path, []byte("sites:\n  - url: http://example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted a non-HTTPS URL")
	}
}
