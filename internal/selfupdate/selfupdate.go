// Package selfupdate updates cisco-rader from GitHub releases.
package selfupdate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/google/go-github/v86/github"
)

const repository = "sig9org/cisco-rader"

// Update replaces the running executable with the newest compatible release.
func Update(ctx context.Context, currentVersion string) (string, error) {
	owner, name, _ := strings.Cut(repository, "/")
	client := github.NewClient(nil)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		client = client.WithAuthToken(token)
	}
	releases, _, err := client.Repositories.ListReleases(ctx, owner, name, nil)
	if err != nil {
		return "", fmt.Errorf("detect latest release: %w", err)
	}
	current := normalizeVersion(currentVersion)
	var latest *github.RepositoryRelease
	var latestVersion *semver.Version
	for _, release := range releases {
		if release.GetDraft() || release.GetPrerelease() {
			continue
		}
		version, parseErr := semver.NewVersion(release.GetTagName())
		if parseErr == nil && (latestVersion == nil || version.GreaterThan(latestVersion)) {
			latest, latestVersion = release, version
		}
	}
	if latest == nil {
		return "", fmt.Errorf("no compatible release found")
	}
	if current != "" {
		currentVersion, parseErr := semver.NewVersion(current)
		if parseErr == nil && !latestVersion.GreaterThan(currentVersion) {
			return fmt.Sprintf("Already up to date (current %s, latest %s).", current, latestVersion), nil
		}
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate running executable: %w", err)
	}
	asset := releaseAsset(latest)
	if asset == nil {
		return "", fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	data, _, err := client.Repositories.DownloadReleaseAsset(ctx, owner, name, asset.GetID(), http.DefaultClient)
	if err != nil {
		return "", fmt.Errorf("download update: %w", err)
	}
	defer data.Close()
	if err := replaceExecutable(executable, data); err != nil {
		return "", fmt.Errorf("update executable: %w", err)
	}
	return fmt.Sprintf("Updated cisco-rader to %s.", latestVersion), nil
}

func releaseAsset(release *github.RepositoryRelease) *github.ReleaseAsset {
	suffix := "_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		suffix += ".exe"
	}
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.GetName(), suffix) {
			return asset
		}
	}
	return nil
}

func replaceExecutable(path string, source io.Reader) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".cisco-rader-update-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err = io.Copy(temp, source); err == nil {
		err = temp.Chmod(0o755)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempName, path)
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
