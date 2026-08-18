package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sig9org/cisco-rader/internal/logx"
	"github.com/sig9org/cisco-rader/internal/model"
)

type blockingSiteFetcher struct {
	entered chan struct{}
	release chan struct{}
	mu      sync.Mutex
	active  int
	maximum int
}

func (f *blockingSiteFetcher) MaxConcurrentFetches() int { return 3 }

func (f *blockingSiteFetcher) Fetch(ctx context.Context, site model.Site) (model.Snapshot, error) {
	f.mu.Lock()
	f.active++
	if f.active > f.maximum {
		f.maximum = f.active
	}
	f.mu.Unlock()
	f.entered <- struct{}{}
	select {
	case <-f.release:
	case <-ctx.Done():
		return model.Snapshot{}, ctx.Err()
	}
	f.mu.Lock()
	f.active--
	f.mu.Unlock()
	return model.Snapshot{ProductName: site.Name, Latest: []string{site.URL}}, nil
}

func TestCheckSitesRunsIndependentFetchesConcurrently(t *testing.T) {
	sites := []model.Site{
		{Name: "Product A", URL: "https://example.com/a"},
		{Name: "Product B", URL: "https://example.com/b"},
		{Name: "Product C", URL: "https://example.com/c"},
	}
	fetcher := &blockingSiteFetcher{
		entered: make(chan struct{}, len(sites)),
		release: make(chan struct{}),
	}
	logger := &logx.Logger{Out: io.Discard, Err: io.Discard}
	done := make(chan []siteCheckResult, 1)
	go func() {
		done <- checkSites(context.Background(), fetcher, sites, nil, 3, logger)
	}()
	for range sites {
		select {
		case <-fetcher.entered:
		case <-time.After(time.Second):
			t.Fatal("site fetches did not start concurrently")
		}
	}
	close(fetcher.release)
	results := <-done
	if fetcher.maximum != 3 {
		t.Fatalf("maximum concurrent fetches = %d, want 3", fetcher.maximum)
	}
	for index, result := range results {
		if result.err != nil {
			t.Fatalf("result %d failed: %v", index, result.err)
		}
		if got := result.snapshot.Latest; len(got) != 1 || got[0] != sites[index].URL {
			t.Fatalf("result %d mixed with another site: %#v", index, result.snapshot)
		}
	}
}

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"usage: cisco-rader [flags]", "-config string", "-v, -version", "-h, -help"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("help does not contain %q", want)
		}
	}
	if strings.Contains(stdout.String(), "-p,") {
		t.Errorf("help still advertises removed -p option")
	}
	for _, removed := range []string{"-site", "-profile", "-separate", "-headless", "-timeout", "-silent"} {
		if strings.Contains(stdout.String(), removed) {
			t.Errorf("help still advertises removed CLI option %q", removed)
		}
	}
}

func TestRunHelpListsFlagsAlphabetically(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}

	var got []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "-") {
			got = append(got, strings.Fields(line)[0])
		}
	}
	want := []string{"-config", "-debug", "-dryrun", "-h,", "-init", "-no-notify", "-no-save", "-update", "-v,"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("help flags = %#v, want %#v", got, want)
	}
}

func TestRunRejectsRemovedShortProfileOption(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-p", "webex"}, &stdout, &stderr); code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -p") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestRunHelpAlignsFlagDescriptions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	descriptions := []string{
		"configuration file",
		"do not send chat notifications",
		"do not write the derived",
		"show the planned operation",
		"update cisco-rader",
		"print version information",
		"show this help message",
	}
	wantColumn := -1
	for _, description := range descriptions {
		column := strings.Index(stdout.String(), description)
		if column < 0 {
			t.Fatalf("help does not contain %q", description)
		}
		lineStart := strings.LastIndex(stdout.String()[:column], "\n") + 1
		column -= lineStart
		if wantColumn == -1 {
			wantColumn = column
		} else if column != wantColumn {
			t.Errorf("%q starts at column %d, want %d", description, column, wantColumn)
		}
	}
}

func TestRunDryRunDoesNotCreateState(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	data := []byte("notifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - name: Test\n    url: https://software.cisco.com/test\n")
	if err := os.WriteFile(sitePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"-config", sitePath, "-dryrun"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "custom_state.yml")); !os.IsNotExist(err) {
		t.Fatalf("dry run created state: %v", err)
	}
	if !strings.Contains(stdout.String(), "would check 1 site(s)") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
}

func TestRunDryRunReportsBrowserOptions(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	var stdout, stderr bytes.Buffer
	data := []byte("settings:\n  headless: true\n  user-agent: Custom Agent\n  timeout: 1m30s\nnotifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - url: https://software.cisco.com/test\n")
	if err := os.WriteFile(sitePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-config", sitePath, "-dryrun"}
	if code := run(context.Background(), args, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, stderr=%s", code, stderr.String())
	}
	want := "browser mode=headless, timeout=1m30s, custom User-Agent=true"
	if !strings.Contains(stdout.String(), want) {
		t.Fatalf("stdout does not contain %q: %s", want, stdout.String())
	}
}

func TestChatToolDisplayName(t *testing.T) {
	tests := map[string]string{
		"teams":  "Microsoft Teams",
		"webex":  "Cisco Webex",
		"slack":  "Slack",
		"custom": "custom",
	}
	for input, want := range tests {
		if got := chatToolDisplayName(input); got != want {
			t.Errorf("chatToolDisplayName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDebugOverridesSilent(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	var stdout, stderr bytes.Buffer
	data := []byte("settings:\n  silent: true\n  debug: true\nnotifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - url: https://software.cisco.com/test\n")
	if err := os.WriteFile(sitePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	code := run(context.Background(), []string{"-config", sitePath, "-dryrun"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "[DEBUG]") || !strings.Contains(stdout.String(), "Dry run") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLIDebugOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	data := []byte("settings:\n  debug: false\nnotifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - url: https://software.cisco.com/test\n")
	if err := os.WriteFile(sitePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-config", sitePath, "-dryrun", "-debug"}, &stdout, &stderr)
	if code != 0 || !strings.Contains(stdout.String(), "[DEBUG]") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCLIExplicitDebugFalseOverridesConfig(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	data := []byte("settings:\n  debug: true\nnotifications:\n  teams:\n    destination: https://example.com/teams\nsites:\n  - url: https://software.cisco.com/test\n")
	if err := os.WriteFile(sitePath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-config", sitePath, "-dryrun", "-debug=false"}, &stdout, &stderr)
	if code != 0 || strings.Contains(stdout.String(), "[DEBUG]") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}
