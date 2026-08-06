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
	site       string
	chatConfig string
	profile    string
	noNotify   bool
	noSave     bool
	dryRun     bool
	debug      bool
	silent     bool
	update     bool
	showVer    bool
	showHelp   bool
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
	fs.StringVar(&opts.site, "site", "", "site configuration file")
	fs.StringVar(&opts.chatConfig, "config", "", "chat configuration file")
	fs.StringVar(&opts.profile, "p", "default", "chat configuration profile")
	fs.StringVar(&opts.profile, "profile", "default", "chat configuration profile")
	fs.BoolVar(&opts.noNotify, "no-notify", false, "do not send chat notifications")
	fs.BoolVar(&opts.noSave, "no-save", false, "do not save the current state")
	fs.BoolVar(&opts.dryRun, "dryrun", false, "show the planned operation without running it")
	fs.BoolVar(&opts.debug, "debug", false, "print timestamped debug tracing to stdout")
	fs.BoolVar(&opts.silent, "silent", false, "suppress normal stdout messages")
	fs.BoolVar(&opts.update, "update", false, "update cisco-rader to the latest release and exit")
	fs.BoolVar(&opts.showVer, "v", false, "print version information and exit")
	fs.BoolVar(&opts.showVer, "version", false, "print version information and exit")
	fs.BoolVar(&opts.showHelp, "h", false, "show this help message and exit")
	fs.BoolVar(&opts.showHelp, "help", false, "show this help message and exit")
	fs.Usage = func() { printUsage(stdout, version.Current()) }
	if err := fs.Parse(args); err != nil {
		return 2
	}
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
	return monitor(ctx, opts, logger)
}

func monitor(ctx context.Context, opts options, logger *logx.Logger) int {
	sitePath, err := config.ResolveSitePath(opts.site)
	if err != nil {
		logger.Errorf("Configuration error: %v", err)
		return 1
	}
	cfg, err := config.Load(sitePath)
	if err != nil {
		logger.Errorf("Configuration error: %v", err)
		return 1
	}
	statePath := config.StatePath(sitePath)
	logger.Debugf("site file: %s", sitePath)
	logger.Debugf("state file: %s", statePath)
	if opts.dryRun {
		logger.Infof("Dry run: would check %d site(s) from %s.", len(cfg.Sites), sitePath)
		logger.Infof("Dry run: notifications=%t, state saving=%t (%s).", !opts.noNotify, !opts.noSave, statePath)
		return 0
	}

	saved, err := state.Load(statePath)
	if err != nil {
		logger.Errorf("State error: %v", err)
		return 1
	}
	var browserLog io.Writer
	if opts.debug {
		browserLog = logger.Out
	}
	logger.Debugf("launching visible Chrome/Chromium")
	browser, err := scraper.Open(ctx, 45*time.Second, browserLog)
	if err != nil {
		logger.Errorf("Browser error: %v", err)
		return 1
	}
	defer browser.Close()

	changes := make([]model.SiteDiff, 0)
	failed := false
	for _, site := range cfg.Sites {
		logger.Infof("Checking %s (%s)", site.Name, site.URL)
		snapshot, fetchErr := browser.Fetch(ctx, site)
		if fetchErr != nil {
			logger.Errorf("[%s] Check failed: %v", site.Name, fetchErr)
			failed = true
			continue
		}
		logger.Infof("[%s] Suggested Release: %s", site.Name, displayVersions(snapshot.Suggested))
		logger.Infof("[%s] Latest Release: %s", site.Name, displayVersions(snapshot.Latest))
		previous, found := saved.Sites[site.URL]
		var previousPtr *model.Snapshot
		if found {
			previousPtr = &previous
		}
		change := diff.Compute(site, previousPtr, snapshot)
		if change.FirstRun {
			logger.Infof("[%s] Initial state recorded; notification skipped.", site.Name)
		} else if change.Changed() {
			logger.Warnf("[%s] Release changes detected.", site.Name)
			changes = append(changes, change)
		} else {
			logger.Infof("[%s] No changes.", site.Name)
		}
		saved.Sites[site.URL] = snapshot
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
		chatConfig, configErr := notification.LoadConfig(opts.chatConfig, opts.profile)
		if configErr != nil {
			logger.Errorf("Notification configuration error: %v", configErr)
			failed = true
		} else {
			now := time.Now()
			for _, change := range changes {
				results, sendErr := notification.Send(ctx, chatConfig, notification.Message(change, now))
				if sendErr != nil {
					if errors.Is(sendErr, notify.ErrNoRecipients) {
						logger.Errorf("Notification error: no chat destination is configured")
					} else {
						logger.Errorf("Notification for %s failed: %v", change.Site.Name, sendErr)
					}
					failed = true
				}
				for _, result := range results {
					if result.Err != nil {
						logger.Errorf("Notification for %s to %s failed: %v", change.Site.Name, result.Tool, result.Err)
						failed = true
					} else {
						logger.Infof("Notification for %s sent to %s.", change.Site.Name, result.Tool)
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
Release changes. A visible Chrome or Chromium window is required.

Flags:
      -site string     site configuration file (default: sites.yml, then site.yaml)
      -config string   chat configuration file (default: config.ini lookup)
  -p, -profile string  chat configuration profile (default: "default")
      -no-notify       do not send chat notifications when releases change
      -no-save         do not write the derived *_state YAML file
      -dryrun          show the planned operation without fetching, notifying, or saving
      -silent          suppress normal stdout messages
      -debug           print timestamped debug tracing to stdout (overrides -silent)
      -update          update cisco-rader to the latest release and exit
  -v, -version         print version information and exit
  -h, -help            show this help message and exit
`, info.String())
}
