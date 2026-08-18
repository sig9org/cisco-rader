package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sig9org/chatxgo/notify"
	"github.com/sig9org/cisco-rader/internal/config"
	"github.com/sig9org/cisco-rader/internal/diff"
	"github.com/sig9org/cisco-rader/internal/logx"
	"github.com/sig9org/cisco-rader/internal/model"
	"github.com/sig9org/cisco-rader/internal/notification"
	"github.com/sig9org/cisco-rader/internal/scraper"
	"github.com/sig9org/cisco-rader/internal/selfupdate"
	"github.com/sig9org/cisco-rader/internal/state"
	"github.com/sig9org/cisco-rader/internal/version"
)

type options struct {
	separate  bool
	headless  bool
	userAgent string
	timeout   time.Duration
	noNotify  bool
	noSave    bool
	init      bool
	dryRun    bool
	debug     bool
	debugSet  bool
	silent    bool
	update    bool
	showVer   bool
	showHelp  bool
}

type siteCheckResult struct {
	site     model.Site
	snapshot model.Snapshot
	change   model.SiteDiff
	err      error
}

type siteFetcher interface {
	Fetch(context.Context, model.Site) (model.Snapshot, error)
	MaxConcurrentFetches() int
}

// checkSites fetches sites with a bounded worker pool. Results retain the
// configuration order so grouped notifications and state writes are stable.
func checkSites(ctx context.Context, browser siteFetcher, sites []model.Site, previous map[string]model.Snapshot, threads int, logger *logx.Logger) []siteCheckResult {
	workerCount := threads
	if workerCount <= 0 {
		workerCount = 1
	} else if workerCount > len(sites) {
		workerCount = len(sites)
	}
	if browserLimit := browser.MaxConcurrentFetches(); browserLimit > 0 && workerCount > browserLimit {
		workerCount = browserLimit
	}
	logger.Debugf("checking %d site(s) with %d worker(s)", len(sites), workerCount)
	results := make([]siteCheckResult, len(sites))
	jobs := make(chan int, len(sites))
	for index := range sites {
		jobs <- index
	}
	close(jobs)
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				site := sites[index]
				logger.Infof("[%s] Checking...", site.Name)
				logger.Debugf("[%s] Checking URL %s", site.Name, site.URL)
				snapshot, err := browser.Fetch(ctx, site)
				if err != nil {
					results[index] = siteCheckResult{site: site, err: err}
					logger.Errorf("[%s] Check failed: %v", site.Name, err)
					continue
				}
				logger.Debugf("[%s] Rendered product: %s", site.Name, snapshot.ProductName)
				logger.Debugf("[%s] Suggested Release: %s", site.Name, displayVersions(snapshot.Suggested))
				logger.Debugf("[%s] Latest Release: %s", site.Name, displayVersions(snapshot.Latest))
				previousSnapshot, found := previous[site.URL]
				var previousPtr *model.Snapshot
				if found {
					previousPtr = &previousSnapshot
				}
				results[index] = siteCheckResult{
					site: site, snapshot: snapshot,
					change: diff.Compute(site, previousPtr, snapshot),
				}
			}
		}()
	}
	wg.Wait()
	return results
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	var opts options
	fs := flag.NewFlagSet("cisco-rader", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var configPath string
	fs.StringVar(&configPath, "config", "", "configuration file (default: config.yml)")
	fs.BoolVar(&opts.debug, "debug", false, "print timestamped debug tracing to stdout (overrides settings.debug)")
	fs.BoolVar(&opts.dryRun, "dryrun", false, "show the planned operation without running it")
	fs.BoolVar(&opts.showHelp, "h", false, "show this help message and exit")
	fs.BoolVar(&opts.showHelp, "help", false, "show this help message and exit")
	fs.BoolVar(&opts.init, "init", false, "recreate the state file from the current site values")
	fs.BoolVar(&opts.noNotify, "no-notify", false, "do not send chat notifications")
	fs.BoolVar(&opts.noSave, "no-save", false, "do not save the current state")
	fs.BoolVar(&opts.update, "update", false, "update cisco-rader to the latest release and exit")
	fs.BoolVar(&opts.showVer, "v", false, "print version information and exit")
	fs.BoolVar(&opts.showVer, "version", false, "print version information and exit")
	fs.Usage = func() { printUsage(stdout, version.Current()) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "debug" {
			opts.debugSet = true
		}
	})
	if fs.NArg() != 0 {
		fmt.Fprintf(stderr, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	info := version.Current()
	if opts.showHelp {
		printUsage(stdout, info)
		return 0
	}
	if opts.showVer {
		fmt.Fprintln(stdout, info.String())
		return 0
	}
	logger := &logx.Logger{Out: stdout, Err: stderr, Silent: opts.silent, Debug: opts.debug}
	if opts.update {
		logger.Debugf("checking GitHub releases for an update")
		message, err := selfupdate.Update(ctx, info.ReleaseVersion())
		if err != nil {
			logger.Errorf("Update failed: %v", err)
			return 1
		}
		logger.Infof("%s", message)
		return 0
	}
	return monitor(ctx, opts, configPath, logger)
}

func monitor(ctx context.Context, opts options, configPath string, logger *logx.Logger) int {
	configPath, err := config.ResolveConfigPath(configPath)
	if err != nil {
		logger.Errorf("Configuration error: %v", err)
		return 1
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Errorf("Configuration error: %v", err)
		return 1
	}
	opts.separate = cfg.Settings.Separate
	opts.headless = cfg.Settings.Headless
	opts.userAgent = cfg.Settings.UserAgent
	opts.timeout = cfg.Settings.Timeout
	opts.silent = cfg.Settings.Silent
	if !opts.debugSet {
		opts.debug = cfg.Settings.Debug
	}
	logger.Silent = opts.silent
	logger.Debug = opts.debug
	logger.Debugf("settings: threads=%d, debug=%t, headless=%t", cfg.Settings.Threads, cfg.Settings.Debug, cfg.Settings.Headless)
	if opts.timeout <= 0 {
		logger.Errorf("Configuration error: settings.timeout must be greater than zero")
		return 1
	}
	statePath := config.StatePath(configPath)
	logger.Debugf("config file: %s", configPath)
	logger.Debugf("state file: %s", statePath)
	mode := "visible"
	if opts.headless {
		mode = "headless"
	}
	if opts.dryRun {
		logger.Infof("Dry run: would check %d site(s) from %s.", len(cfg.Sites), configPath)
		logger.Infof("Dry run: notifications=%t, state saving=%t (%s).", !opts.noNotify, !opts.noSave, statePath)
		logger.Infof("Dry run: separate notifications=%t.", opts.separate)
		logger.Infof("Dry run: threads=%d.", cfg.Settings.Threads)
		logger.Infof("Dry run: browser mode=%s, timeout=%s, custom User-Agent=%t.", mode, opts.timeout, strings.TrimSpace(opts.userAgent) != "")
		return 0
	}

	var saved state.File
	if opts.init {
		saved = state.File{Sites: map[string]model.Snapshot{}}
		logger.Debugf("initializing state file %s", statePath)
	} else {
		saved, err = state.Load(statePath)
		if err != nil {
			logger.Errorf("State error: %v", err)
			return 1
		}
	}
	var browserLog io.Writer
	if opts.debug {
		browserLog = logger.Out
	}
	workerCount := cfg.Settings.Threads
	if workerCount > len(cfg.Sites) {
		workerCount = len(cfg.Sites)
	}
	logger.Debugf("launching %d independent %s Chrome/Chromium worker(s)", workerCount, mode)
	browser, err := scraper.OpenPool(ctx, scraper.Options{
		Timeout:   opts.timeout,
		Headless:  opts.headless,
		UserAgent: opts.userAgent,
		Log:       browserLog,
	}, workerCount)
	if err != nil {
		logger.Errorf("Browser error: %v", err)
		return 1
	}
	defer browser.Close()
	logger.Debugf("User-Agent: %s", browser.UserAgent())

	results := checkSites(ctx, browser, cfg.Sites, saved.Sites, cfg.Settings.Threads, logger)
	changes := make([]model.SiteDiff, 0)
	failed := false
	for _, result := range results {
		if result.err != nil {
			failed = true
			continue
		}
		saved.Sites[result.site.URL] = result.snapshot
		if result.change.FirstRun {
			logger.Debugf("[%s] Initial state recorded; notification skipped.", result.site.Name)
		} else if result.change.Changed() {
			logger.Successf("[%s] Release changes detected.", result.site.Name)
			changes = append(changes, result.change)
		}
	}

	if opts.noSave {
		logger.Debugf("state saving disabled by -no-save")
	} else if err := state.Save(statePath, saved); err != nil {
		logger.Errorf("State error: %v", err)
		failed = true
	} else {
		logger.Debugf("saved state to %s", statePath)
	}

	if len(changes) != 0 && !opts.noNotify {
		var mentions []notify.Mention
		if cfg.Settings.Mention != "" {
			mention, mentionErr := notify.ParseMention(cfg.Settings.Mention)
			if mentionErr != nil {
				logger.Errorf("Notification configuration error: invalid mention: %v", mentionErr)
				failed = true
			} else {
				mentions = append(mentions, mention)
			}
		}
		chatConfig := cfg.Notifications.ChatConfig()
		logger.Debugf("loaded notification configuration from %s", configPath)
		if cfg.Settings.Mention == "" || len(mentions) != 0 {
			messages := notification.Messages(changes, time.Now(), opts.separate, mentions...)
			for _, message := range messages {
				results, sendErr := notification.Send(ctx, chatConfig, message)
				if sendErr != nil {
					if errors.Is(sendErr, notify.ErrNoRecipients) {
						logger.Errorf("Notification error: no chat destination is configured")
					} else {
						logger.Errorf("Notification %q failed: %v", message.Subject, sendErr)
					}
					failed = true
				}
				for _, result := range results {
					toolName := chatToolDisplayName(result.Tool)
					if result.Err != nil {
						logger.Errorf("Notification %q to %s failed: %v", message.Subject, toolName, result.Err)
						failed = true
					} else {
						logger.Successf("[%s] Notification sent to %s.", message.Subject, toolName)
					}
				}
				if errors.Is(sendErr, notify.ErrNoRecipients) {
					break
				}
			}
		}
	} else if len(changes) != 0 {
		logger.Debugf("notification disabled by -no-notify")
	}
	if failed {
		return 1
	}
	return 0
}

func chatToolDisplayName(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "teams":
		return "Microsoft Teams"
	case "webex":
		return "Cisco Webex"
	case "slack":
		return "Slack"
	default:
		return tool
	}
}

func displayVersions(values []string) string {
	if len(values) == 0 {
		return "(none)"
	}
	return strings.Join(values, ", ")
}

func printUsage(w io.Writer, info version.Info) {
	fmt.Fprintf(w, `%s

usage: cisco-rader [flags]

Monitor Cisco Software Download pages for Suggested Release and Latest
Release changes. Chrome or Chromium is displayed by default.

Flags:
      -config string      configuration file (default: config.yml)
      -debug              print timestamped debug tracing to stdout (overrides settings.debug)
      -dryrun             show the planned operation without fetching, notifying, or saving
  -h, -help               show this help message and exit
      -init               recreate the state file from the current site values
      -no-notify          do not send chat notifications when releases change
      -no-save            do not write the derived *_state YAML file
      -update             update cisco-rader to the latest release and exit
  -v, -version            print version information and exit
`, info.String())
}
