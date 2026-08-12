// Package version reports build and release version information.
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
)

// Info is printable version metadata.
type Info struct {
	Description string
}

// Current returns the release tag, using the nearest git tag in a source
// checkout and Go build metadata in installed binaries.
func Current() Info {
	result := Info{Description: Version}
	if result.Description == "dev" {
		if value := gitOutput("describe", "--tags", "--abbrev=0"); value != "" {
			result.Description = value
		}
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		if result.Description == "dev" && build.Main.Version != "" && build.Main.Version != "(devel)" {
			result.Description = build.Main.Version
		}
	}
	return result
}

// String returns the version line used by -version and help.
func (i Info) String() string {
	return fmt.Sprintf("cisco-rader %s", i.Description)
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
