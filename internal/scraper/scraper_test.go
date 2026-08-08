package scraper

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	html := `<html><body>
<h2 id="release-product-title">Catalyst 9800 Wireless Controller</h2>
<ul class="p-tree-root-children">
  <li class="p-tree-node">Suggested Release
    <ul class="p-tree-node-children">
      <li class="p-tree-node"><span class="p-tree-node-label">17.9.5 Suggested Release</span></li>
    </ul>
  </li>
  <li class="p-tree-node">Latest Release
    <ul class="p-tree-node-children">
      <li class="p-tree-node"><span class="p-tree-node-label">17.15.2 Latest Release</span></li>
      <li class="p-tree-node"><span class="p-tree-node-label">17.12.5</span></li>
      <li class="p-tree-node"><span class="p-tree-node-label">17.15.2</span></li>
    </ul>
  </li>
</ul></body></html>`
	now := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	got, err := Parse(strings.NewReader(html), now)
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductName != "Catalyst 9800 Wireless Controller" {
		t.Fatalf("product = %q", got.ProductName)
	}
	if strings.Join(got.Suggested, ",") != "17.9.5" {
		t.Fatalf("suggested = %#v", got.Suggested)
	}
	if strings.Join(got.Latest, ",") != "17.15.2,17.12.5" {
		t.Fatalf("latest = %#v", got.Latest)
	}
}

func TestParseMissingSections(t *testing.T) {
	got, err := Parse(strings.NewReader("<html><body></body></html>"), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProductName != "Unknown Product" || len(got.Suggested) != 0 || len(got.Latest) != 0 {
		t.Fatalf("unexpected snapshot: %#v", got)
	}
}

func TestBrowserUserAgentHidesHeadlessMarker(t *testing.T) {
	got := browserUserAgent("", "Mozilla/5.0 HeadlessChrome/140.0.0.0 Safari/537.36")
	want := "Mozilla/5.0 Chrome/140.0.0.0 Safari/537.36"
	if got != want {
		t.Fatalf("browserUserAgent = %q, want %q", got, want)
	}
}

func TestBrowserUserAgentPrefersConfiguredValue(t *testing.T) {
	got := browserUserAgent(" Custom Agent ", "Detected Agent")
	if got != "Custom Agent" {
		t.Fatalf("browserUserAgent = %q, want %q", got, "Custom Agent")
	}
}

func TestBrowserUserAgentAccessor(t *testing.T) {
	browser := &Browser{userAgent: "Custom Agent"}
	if got := browser.UserAgent(); got != "Custom Agent" {
		t.Fatalf("UserAgent = %q, want %q", got, "Custom Agent")
	}
}

func TestParseFindsSectionsBelowWrapperNodes(t *testing.T) {
	html := `<html><body>
<ul class="p-tree-root-children">
  <div class="p-treenode">
    <li class="p-tree-node"><span class="p-tree-node-label">Suggested Release</span>
      <ul class="p-tree-node-children">
        <div class="p-treenode">
          <li class="p-tree-node"><span class="p-tree-node-label">6.0.6Suggested Release</span></li>
        </div>
      </ul>
    </li>
  </div>
  <div class="p-treenode">
    <li class="p-tree-node"><span class="p-tree-node-label">Latest Release</span>
      <ul class="p-tree-node-children">
        <div class="p-treenode">
          <li class="p-tree-node"><span class="p-tree-node-label">6.0.7Latest Release</span></li>
        </div>
      </ul>
    </li>
  </div>
</ul></body></html>`
	got, err := Parse(strings.NewReader(html), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Suggested, ",") != "6.0.6" {
		t.Fatalf("suggested = %#v", got.Suggested)
	}
	if strings.Join(got.Latest, ",") != "6.0.7" {
		t.Fatalf("latest = %#v", got.Latest)
	}
}
