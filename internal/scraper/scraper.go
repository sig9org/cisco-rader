// Package scraper retrieves rendered Cisco Software Download release pages.
package scraper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/sig9org/cisco-rader/internal/model"
)

const releaseTreeSelector = ".p-tree-root-children"

// Browser renders pages in a visible local Chrome or Chromium window.
type Browser struct {
	launcher *launcher.Launcher
	browser  *rod.Browser
	timeout  time.Duration
}

// Open starts a visible browser. Cisco may deny automated headless sessions,
// so cisco-rader deliberately requires an active desktop session.
func Open(ctx context.Context, timeout time.Duration, browserLog io.Writer) (*Browser, error) {
	bin, ok := launcher.LookPath()
	if !ok {
		return nil, errors.New("Chrome or Chromium was not found; install a supported browser and try again")
	}
	l := launcher.New().Context(ctx).Bin(bin).Headless(false).Leakless(false)
	if browserLog != nil {
		l.Logger(browserLog)
	}
	controlURL, err := l.Launch()
	if err != nil {
		return nil, fmt.Errorf("launch visible browser: %w", err)
	}
	b := rod.New().ControlURL(controlURL).Context(ctx).NoDefaultDevice()
	if err := b.Connect(); err != nil {
		l.Kill()
		return nil, fmt.Errorf("connect to browser: %w", err)
	}
	return &Browser{launcher: l, browser: b, timeout: timeout}, nil
}

// Close stops the browser and removes its temporary profile.
func (b *Browser) Close() {
	if b == nil {
		return
	}
	if b.browser != nil {
		_ = b.browser.Close()
	}
	if b.launcher != nil {
		b.launcher.Kill()
		b.launcher.Cleanup()
	}
}

// Fetch loads a site and extracts its Suggested and Latest releases.
func (b *Browser) Fetch(ctx context.Context, site model.Site) (model.Snapshot, error) {
	page, err := b.browser.Context(ctx).Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("create browser page: %w", err)
	}
	defer func() { _ = page.Close() }()

	timed := page.Timeout(b.timeout)
	if err := timed.Navigate(site.URL); err != nil {
		return model.Snapshot{}, fmt.Errorf("navigate to %s: %w", site.URL, err)
	}
	if _, err := timed.Element(releaseTreeSelector); err != nil {
		return model.Snapshot{}, fmt.Errorf("wait for Cisco release tree: %w", err)
	}
	select {
	case <-ctx.Done():
		return model.Snapshot{}, ctx.Err()
	case <-time.After(2 * time.Second):
	}
	html, err := page.HTML()
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("read rendered page: %w", err)
	}
	snapshot, err := Parse(strings.NewReader(html), time.Now().UTC())
	if err != nil {
		return model.Snapshot{}, err
	}
	if len(snapshot.Suggested) == 0 && len(snapshot.Latest) == 0 {
		return model.Snapshot{}, errors.New("the page contained no Suggested Release or Latest Release entries")
	}
	return snapshot, nil
}

// Parse extracts release information from rendered Cisco page HTML.
func Parse(r io.Reader, fetchedAt time.Time) (model.Snapshot, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return model.Snapshot{}, fmt.Errorf("parse rendered page: %w", err)
	}
	productName := strings.TrimSpace(doc.Find("h2#release-product-title").First().Text())
	if productName == "" {
		productName = "Unknown Product"
	}
	result := model.Snapshot{ProductName: productName, FetchedAt: fetchedAt}
	root := doc.Find("ul.p-tree-root-children").First()
	suggestedFound := false
	latestFound := false
	root.Find("li.p-tree-node").EachWithBreak(func(_ int, node *goquery.Selection) bool {
		text := node.Text()
		if !suggestedFound && strings.Contains(text, "Suggested Release") {
			result.Suggested = versions(node)
			suggestedFound = true
		}
		if !latestFound && strings.Contains(text, "Latest Release") {
			result.Latest = versions(node)
			latestFound = true
		}
		return !suggestedFound || !latestFound
	})
	return result, nil
}

func versions(section *goquery.Selection) []string {
	children := section.Find("ul.p-tree-node-children").First()
	seen := map[string]struct{}{}
	result := []string{}
	children.Find("li.p-tree-node").Each(func(_ int, node *goquery.Selection) {
		label := node.Find("span.p-tree-node-label").First()
		if label.Length() == 0 {
			return
		}
		value := strings.TrimSpace(label.Text())
		for _, suffix := range []string{"Suggested Release", "Latest Release", "All Release"} {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
		}
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	})
	return result
}
