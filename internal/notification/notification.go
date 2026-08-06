// Package notification formats and sends release changes through chatxgo.
package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/cisco-rader/internal/model"
)

// LoadConfig reads a named profile from a config.ini file. An explicit path
// must exist. With no explicit path, chatxgo's standard lookup is used and
// environment variables provide a fallback when no file exists.
func LoadConfig(path, profile string) (notify.Config, error) {
	explicit := strings.TrimSpace(path) != ""
	if !explicit {
		var err error
		path, err = notify.DefaultConfigPath()
		if err != nil {
			return notify.Config{}, err
		}
	}
	if _, err := os.Stat(path); err == nil {
		return notify.LoadConfigFile(path, profile)
	} else if !errors.Is(err, os.ErrNotExist) {
		return notify.Config{}, fmt.Errorf("inspect chat configuration: %w", err)
	}
	if explicit {
		return notify.Config{}, fmt.Errorf("chat configuration file %q does not exist", path)
	}
	if strings.TrimSpace(profile) != "" && !strings.EqualFold(profile, notify.DefaultProfile) {
		return notify.Config{}, fmt.Errorf("chat profile %q requires a config.ini file", profile)
	}
	return notify.ConfigFromEnv(), nil
}

// Message builds a portable Markdown notification for one changed site.
func Message(change model.SiteDiff, now time.Time) notify.Message {
	var body strings.Builder
	writeSection(&body, "Suggested Release", change.Suggested)
	writeSection(&body, "Latest Release", change.Latest)
	fmt.Fprintf(&body, "- Download page:\n  - %s", change.Site.URL)
	return notify.Message{
		Subject: fmt.Sprintf("[%s]%s", now.Format("2006-01-02 15:04 MST"), change.Site.Name),
		Body:    body.String(),
	}
}

func writeSection(body *strings.Builder, name string, change model.SectionDiff) {
	if !change.Changed() {
		return
	}
	fmt.Fprintf(body, "- %s:\n", name)
	for _, version := range change.Added {
		fmt.Fprintf(body, "  - Added: %s\n", version)
	}
	for _, version := range change.Removed {
		fmt.Fprintf(body, "  - Removed: %s\n", version)
	}
}

// Send dispatches a notification and returns all per-tool failures.
func Send(ctx context.Context, cfg notify.Config, message notify.Message) ([]notify.Result, error) {
	dispatcher, err := notify.NewDispatcher(cfg)
	if err != nil {
		return nil, err
	}
	return dispatcher.Send(ctx, message)
}
