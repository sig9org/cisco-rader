// Package selfupdate updates cisco-rader from GitHub releases.
package selfupdate

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	su "github.com/creativeprojects/go-selfupdate"
)

const repository = "sig9org/cisco-rader"

// Update replaces the running executable with the newest compatible release.
func Update(ctx context.Context, currentVersion string) (string, error) {
	repo := su.ParseSlug(repository)
	latest, found, err := su.DetectLatest(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("detect latest release: %w", err)
	}
	if !found {
		return "", fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	current := normalizeVersion(currentVersion)
	if current != "" && latest.LessOrEqual(current) {
		return fmt.Sprintf("Already up to date (current %s, latest %s).", current, latest.Version()), nil
	}
	executable, err := su.ExecutablePath()
	if err != nil {
		return "", fmt.Errorf("locate running executable: %w", err)
	}
	release, err := su.UpdateCommand(ctx, executable, current, repo)
	if err != nil {
		return "", fmt.Errorf("update executable: %w", err)
	}
	return fmt.Sprintf("Updated cisco-rader to %s.", release.Version()), nil
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "dev" {
		return ""
	}
	if before, _, ok := strings.Cut(value, "-"); ok {
		return before
	}
	return value
}
