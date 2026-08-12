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
	separate   bool
	headless   bool
	userAgent  string
	timeout    time.Duration
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
	fs.StringVar(&opts.profile, "profile", "default", "chat configuration profile")
	fs.BoolVar(&opts.separate, "separate", false, "send each software update as a separate message")
	fs.BoolVar(&opts.headless, "headless", false, "run Chrome or Chromium without displaying its UI")
	fs.StringVar(&opts.userAgent, "user-agent", "", "browser User-Agent (default: detected Chrome User-Agent)")
	fs.DurationVar(&opts.timeout, "timeout", 45*time.Second, "release information retrieval timeout")
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
	if opts.timeout <= 0 {
		fmt.Fprintln(stderr, "-timeout must be greater than zero")
		return 2
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
	mode := "visible"
	if opts.headless {
		mode = "headless"
	}
	if opts.dryRun {
		logger.Infof("Dry run: would check %d site(s) from %s.", len(cfg.Sites), sitePath)
		logger.Infof("Dry run: notifications=%t, state saving=%t (%s).", !opts.noNotify, !opts.noSave, statePath)
		logger.Infof("Dry run: separate notifications=%t.", opts.separate)
		logger.Infof("Dry run: browser mode=%s, timeout=%s, custom User-Agent=%t.", mode, opts.timeout, strings.TrimSpace(opts.userAgent) != "")
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
	logger.Debugf("launching %s Chrome/Chromium", mode)
	browser, err := scraper.Open(ctx, scraper.Options{
		Timeout:   opts.timeout,
		Headless:  opts.headless,
		UserAgent: opts.userAgent,
		Log:       browserLog,
	})
	if err != nil {
		logger.Errorf("Browser error: %v", err)
		return 1
	}
	defer browser.Close()
	logger.Debugf("User-Agent: %s", browser.UserAgent())

	changes := make([]model.SiteDiff, 0)
	failed := false
	for _, site := range cfg.Sites {
		logger.Infof("[%s] Checking %s", site.Name, site.URL)
		snapshot, fetchErr := browser.Fetch(ctx, site)
		if fetchErr != nil {
			logger.Errorf("[%s] Check failed: %v", site.Name, fetchErr)
			failed = true
			continue
		}
		logger.Debugf("[%s] Suggested Release: %s", site.Name, displayVersions(snapshot.Suggested))
		logger.Debugf("[%s] Latest Release: %s", site.Name, displayVersions(snapshot.Latest))
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
			logger.Debugf("[%s] No changes.", site.Name)
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
		chatConfig, configErr := notification.LoadConfig(opts.chatConfig, opts.profile)
		if configErr == nil {
			logger.Debugf("loaded chat configuration profile: %s", opts.profile)
		}
		if configErr != nil {
			logger.Errorf("Notification configuration error: %v", configErr)
			failed = true
		} else if cfg.Settings.Mention == "" || len(mentions) != 0 {
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
						logger.Infof("Notification %q sent to %s.", message.Subject, toolName)
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
      -site string        site configuration file (default: sites.yml, then site.yaml)
      -config string      chat configuration file (default: config.ini lookup)
      -profile string     chat configuration profile (default: "default")
      -separate           send each software update as a separate message
      -headless           run Chrome or Chromium without displaying its UI
      -user-agent string  browser User-Agent (default: detected Chrome User-Agent)
      -timeout duration   release information retrieval timeout (default: 45s)
      -no-notify          do not send chat notifications when releases change
      -no-save            do not write the derived *_state YAML file
      -dryrun             show the planned operation without fetching, notifying, or saving
      -silent             suppress normal stdout messages
      -debug              print timestamped debug tracing to stdout (overrides -silent)
      -update             update cisco-rader to the latest release and exit
  -v, -version            print version information and exit
  -h, -help               show this help message and exit
`, info.String())
}
