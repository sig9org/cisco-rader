// Package version reports build and source revision information.
package version

import (
	"context"
	"fmt"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"
)

// These values can be overridden with -ldflags at release build time.
var (
	Version = "dev"
	Commit  = "unknown"
)

// Info is printable version metadata.
type Info struct {
	Description string
	Commit      string
}

// Current returns release metadata, using git describe in a source checkout
// and Go VCS build settings in installed binaries.
func Current() Info {
	result := Info{Description: Version, Commit: Commit}
	if result.Description == "dev" {
		if value := gitOutput("describe", "--tags", "--always", "--dirty"); value != "" {
			result.Description = value
		}
	}
	if result.Commit == "unknown" {
		if value := gitOutput("rev-parse", "--short=12", "HEAD"); value != "" {
			result.Commit = value
		}
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		if result.Description == "dev" && build.Main.Version != "" && build.Main.Version != "(devel)" {
			result.Description = build.Main.Version
		}
		for _, setting := range build.Settings {
			if setting.Key == "vcs.revision" && result.Commit == "unknown" {
				result.Commit = short(setting.Value)
			}
		}
	}
	return result
}

// String returns the version line used by -version and help.
func (i Info) String() string {
	return fmt.Sprintf("cisco-rader %s (commit %s)", i.Description, i.Commit)
}

// ReleaseVersion returns the semantic tag portion for self-update checks.
func (i Info) ReleaseVersion() string {
	if before, _, ok := strings.Cut(i.Description, "-"); ok {
		return before
	}
	return i.Description
}

func gitOutput(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "git", args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func short(value string) string {
	if len(value) > 12 {
		return value[:12]
	}
	return value
}
