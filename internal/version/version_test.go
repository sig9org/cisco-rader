package version

import (
	"strings"
	"testing"
)

func TestInfoStringContainsOnlyReleaseVersion(t *testing.T) {
	info := Info{Description: "v0.0.2"}

	if got, want := info.String(), "cisco-rader v0.0.2"; got != want {
		t.Fatalf("Info.String() = %q, want %q", got, want)
	}
}

func TestCurrentUsesNearestGitTag(t *testing.T) {
	info := Current()

	if info.Description == "dev" {
		t.Skip("git metadata is unavailable")
	}
	if !strings.HasPrefix(info.Description, "v") {
		t.Fatalf("Current().Description = %q, want a Git tag", info.Description)
	}
	if info.String() != "cisco-rader "+info.Description {
		t.Fatalf("Current().String() = %q, contains non-tag metadata", info.String())
	}
}
