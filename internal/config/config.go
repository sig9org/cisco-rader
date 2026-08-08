// Package config loads and validates monitored Cisco sites.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/sig9org/cisco-rader/internal/model"
	"gopkg.in/yaml.v3"
)

// File is the sites configuration document.
type File struct {
	Settings Settings     `yaml:"settings"`
	Sites    []model.Site `yaml:"sites"`
}

// Settings controls notification behavior shared by all monitored sites.
type Settings struct {
	Mention string `yaml:"mention"`
}

// ResolveSitePath returns an explicitly requested path, or the first existing
// default. sites.yml takes precedence over site.yaml.
func ResolveSitePath(explicit string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	for _, candidate := range []string{"sites.yml", "site.yaml"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("inspect site file %q: %w", candidate, err)
		}
	}
	return "", errors.New("no site file found (looked for sites.yml and site.yaml)")
}

// StatePath derives the state file path from the site file path.
func StatePath(sitePath string) string {
	ext := filepath.Ext(sitePath)
	base := strings.TrimSuffix(filepath.Base(sitePath), ext)
	return filepath.Join(filepath.Dir(sitePath), base+"_state"+ext)
}

// Load reads and validates a sites file.
func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}, fmt.Errorf("read site file %q: %w", path, err)
	}
	var cfg File
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return File{}, fmt.Errorf("parse site file %q: %w", path, err)
	}
	if len(cfg.Sites) == 0 {
		return File{}, errors.New("site file contains no sites")
	}
	cfg.Settings.Mention = strings.TrimSpace(cfg.Settings.Mention)
	seen := make(map[string]struct{}, len(cfg.Sites))
	for i := range cfg.Sites {
		site := &cfg.Sites[i]
		site.Name = strings.TrimSpace(site.Name)
		site.URL = strings.TrimSpace(site.URL)
		parsed, parseErr := url.ParseRequestURI(site.URL)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return File{}, fmt.Errorf("sites[%d].url must be a valid HTTPS URL", i)
		}
		if site.Name == "" {
			site.Name = site.URL
		}
		if _, exists := seen[site.URL]; exists {
			return File{}, fmt.Errorf("duplicate site URL %q", site.URL)
		}
		seen[site.URL] = struct{}{}
	}
	return cfg, nil
}
