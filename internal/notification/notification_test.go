package notification

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sig9org/cisco-rader/internal/model"
)

func TestLoadConfigSelectsProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.ini")
	data := []byte("[default]\nMSTEAMS_DST=https://example.com/default\n\n[webex]\nWEBEX_TOKEN=token\nWEBEX_DST=room\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path, "webex")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Webex.Token != "token" || cfg.Webex.Dest != "room" {
		t.Fatalf("unexpected Webex config: %#v", cfg.Webex)
	}
	if cfg.Teams.Dest != "" {
		t.Fatalf("profile settings leaked: %#v", cfg.Teams)
	}
}

func TestLoadConfigRejectsMissingExplicitFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "missing.ini"), "default")
	if err == nil {
		t.Fatal("LoadConfig accepted a missing explicit file")
	}
}

func TestMessageContainsOnlyProvidedChanges(t *testing.T) {
	location := time.FixedZone("JST", 9*60*60)
	message := Message(model.SiteDiff{
		Site:      model.Site{Name: "Test Product", URL: "https://software.cisco.com/test"},
		Suggested: model.SectionDiff{Added: []string{"2.0"}, Removed: []string{"1.0"}},
	}, time.Date(2026, 8, 6, 8, 42, 0, 0, location))
	if want := "[2026-08-06 08:42 JST]Test Product"; message.Subject != want {
		t.Errorf("subject = %q, want %q", message.Subject, want)
	}
	wantBody := "- Suggested Release:\n" +
		"  - Added: 2.0\n" +
		"  - Removed: 1.0\n" +
		"- Download page:\n" +
		"  - https://software.cisco.com/test"
	if message.Body != wantBody {
		t.Errorf("body = %q, want %q", message.Body, wantBody)
	}
	if strings.Contains(message.Body, "Latest Release") {
		t.Errorf("unchanged section was included: %s", message.Body)
	}
}

func TestMessageFormatsTeamsFriendlyNestedList(t *testing.T) {
	message := Message(model.SiteDiff{
		Site: model.Site{
			Name: "Cisco Modeling Labs",
			URL:  "https://software.cisco.com/download/home/286193282/type/286326381/release/",
		},
		Suggested: model.SectionDiff{Added: []string{"2.10.0"}, Removed: []string{"2.9.0"}},
		Latest:    model.SectionDiff{Added: []string{"2.10.0"}},
	}, time.Time{})
	want := "- Suggested Release:\n" +
		"  - Added: 2.10.0\n" +
		"  - Removed: 2.9.0\n" +
		"- Latest Release:\n" +
		"  - Added: 2.10.0\n" +
		"- Download page:\n" +
		"  - https://software.cisco.com/download/home/286193282/type/286326381/release/"
	if message.Body != want {
		t.Errorf("body = %q, want %q", message.Body, want)
	}
}
