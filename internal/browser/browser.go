// Package browser wraps playwright-go into a tiny ergonomic API matched to
// our YAML step actions: goto/click/fill/select/press/wait_for/wait_ms/scroll/screenshot.
package browser

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"

	"github.com/krisk248/moonlight/internal/scenario"
)

// (imports above are intentionally kept compact; filepath is now used by Download)


type Browser struct {
	pw       *playwright.Playwright
	browser  playwright.Browser
	context  playwright.BrowserContext
	page     playwright.Page
	baseURL  string

	// Per-step event buffers — runner snapshots these between steps.
	ConsoleMessages []ConsoleEntry
	FailedRequests  []NetworkEntry
	PageErrors      []string
}

type ConsoleEntry struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Location string `json:"location"`
}

type NetworkEntry struct {
	URL     string `json:"url"`
	Method  string `json:"method"`
	Status  int    `json:"status,omitempty"`
	Failure string `json:"failure,omitempty"`
}

type Opts struct {
	BaseURL          string
	Viewport         scenario.Viewport
	Headless         bool
	DefaultTimeoutMS int
	StorageStatePath string
}

func New(opts Opts) (*Browser, error) {
	if opts.Viewport.Width == 0 {
		opts.Viewport = scenario.Viewport{Width: 1280, Height: 720}
	}
	if opts.DefaultTimeoutMS == 0 {
		opts.DefaultTimeoutMS = 15000
	}

	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("playwright start: %w", err)
	}
	br, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(opts.Headless),
	})
	if err != nil {
		pw.Stop()
		return nil, fmt.Errorf("launch chromium: %w", err)
	}

	ctxOpts := playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{Width: opts.Viewport.Width, Height: opts.Viewport.Height},
	}
	if opts.StorageStatePath != "" {
		if _, err := os.Stat(opts.StorageStatePath); err == nil {
			ctxOpts.StorageStatePath = playwright.String(opts.StorageStatePath)
		}
	}
	ctx, err := br.NewContext(ctxOpts)
	if err != nil {
		br.Close()
		pw.Stop()
		return nil, fmt.Errorf("new context: %w", err)
	}
	ctx.SetDefaultTimeout(float64(opts.DefaultTimeoutMS))

	page, err := ctx.NewPage()
	if err != nil {
		ctx.Close()
		br.Close()
		pw.Stop()
		return nil, fmt.Errorf("new page: %w", err)
	}

	b := &Browser{
		pw:      pw,
		browser: br,
		context: ctx,
		page:    page,
		baseURL: strings.TrimRight(opts.BaseURL, "/") + "/",
	}

	page.OnConsole(func(msg playwright.ConsoleMessage) {
		loc := msg.Location()
		b.ConsoleMessages = append(b.ConsoleMessages, ConsoleEntry{
			Type:     msg.Type(),
			Text:     truncate(msg.Text(), 500),
			Location: fmt.Sprintf("%s:%d", loc.URL, loc.LineNumber),
		})
	})
	page.OnPageError(func(err error) {
		b.PageErrors = append(b.PageErrors, truncate(err.Error(), 500))
	})
	page.OnRequestFailed(func(req playwright.Request) {
		failure := ""
		if f := req.Failure(); f != nil {
			failure = f.Error()
		}
		b.FailedRequests = append(b.FailedRequests, NetworkEntry{
			URL:     truncate(req.URL(), 300),
			Method:  req.Method(),
			Failure: truncate(failure, 200),
		})
	})
	page.OnResponse(func(resp playwright.Response) {
		if resp.Status() >= 400 {
			b.FailedRequests = append(b.FailedRequests, NetworkEntry{
				URL:    truncate(resp.URL(), 300),
				Method: resp.Request().Method(),
				Status: resp.Status(),
			})
		}
	})

	return b, nil
}

// Page exposes the underlying Playwright page for callers (like the dom
// package) that need raw access.
func (b *Browser) Page() playwright.Page { return b.page }

func (b *Browser) Close() {
	if b.context != nil {
		b.context.Close()
	}
	if b.browser != nil {
		b.browser.Close()
	}
	if b.pw != nil {
		b.pw.Stop()
	}
}

func (b *Browser) resolve(u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	base, _ := url.Parse(b.baseURL)
	rel, _ := url.Parse(strings.TrimLeft(u, "/"))
	return base.ResolveReference(rel).String()
}

// locator resolves a Step into a Playwright Locator.
func (b *Browser) locator(s scenario.Step) (playwright.Locator, error) {
	switch {
	case s.Selector != "":
		return b.page.Locator(s.Selector), nil
	case s.Role != "":
		opts := playwright.PageGetByRoleOptions{}
		if s.RoleName != "" {
			opts.Name = s.RoleName
		}
		return b.page.GetByRole(*playwrightAriaRole(s.Role), opts), nil
	case s.Label != "":
		return b.page.GetByLabel(s.Label), nil
	case s.Text != "":
		return b.page.GetByText(s.Text), nil
	}
	return nil, fmt.Errorf("step has no locator: %+v", s)
}

func playwrightAriaRole(s string) *playwright.AriaRole {
	r := playwright.AriaRole(s)
	return &r
}

// ----- step dispatchers (called by the runner) ------------------------------

func (b *Browser) Goto(u string) error {
	_, err := b.page.Goto(b.resolve(u), playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(15000),
	})
	if err != nil {
		// Fall back to load — some apps keep XHR open forever.
		_, err = b.page.Goto(b.resolve(u), playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateLoad,
		})
	}
	return err
}

func (b *Browser) Click(s scenario.Step) error {
	loc, err := b.locator(s)
	if err != nil {
		return err
	}
	return loc.Click()
}

func (b *Browser) Fill(s scenario.Step) error {
	loc, err := b.locator(s)
	if err != nil {
		return err
	}
	return loc.Fill(s.Value)
}

func (b *Browser) Press(s scenario.Step) error {
	loc, err := b.locator(s)
	if err != nil {
		return err
	}
	return loc.Press(s.Key)
}

func (b *Browser) SelectOption(s scenario.Step) error {
	loc, err := b.locator(s)
	if err != nil {
		return err
	}
	_, err = loc.SelectOption(playwright.SelectOptionValues{Values: &[]string{s.Value}})
	return err
}

func (b *Browser) WaitFor(s scenario.Step) error {
	loc, err := b.locator(s)
	if err != nil {
		return err
	}
	return loc.WaitFor()
}

func (b *Browser) WaitMS(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

// WaitForNetworkidle blocks until the page has had no network activity for
// 500ms — the standard "page is done loading" heuristic.
func (b *Browser) WaitForNetworkidle() error {
	return b.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	})
}

// SetStepTimeout temporarily overrides the per-step Playwright timeout for
// the upcoming action. Restored to the context's default at the next call.
func (b *Browser) SetStepTimeout(ms int) {
	if ms <= 0 {
		return
	}
	b.context.SetDefaultTimeout(float64(ms))
}

// ResetTimeout restores the global default timeout the runner gave us.
func (b *Browser) ResetTimeout(globalMS int) {
	b.context.SetDefaultTimeout(float64(globalMS))
}

// Upload sets an <input type=file> to a local path on disk.
func (b *Browser) Upload(s scenario.Step) error {
	loc, err := b.locator(s)
	if err != nil {
		return err
	}
	path := s.Path
	if path == "" {
		return fmt.Errorf("upload_file step missing 'path'")
	}
	return loc.SetInputFiles(path)
}

// Download registers a download listener, clicks the trigger, waits for the
// download to complete, then saves it to disk at the given path.
func (b *Browser) Download(s scenario.Step) error {
	loc, err := b.locator(s)
	if err != nil {
		return err
	}
	saveTo := s.SaveTo
	if saveTo == "" {
		saveTo = s.Path
	}
	if saveTo == "" {
		return fmt.Errorf("download_file step missing 'save_to' (or 'path')")
	}

	// page.ExpectDownload runs the trigger inside a closure that arms the listener.
	dl, err := b.page.ExpectDownload(func() error {
		return loc.Click()
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(saveTo), 0o755); err != nil {
		return err
	}
	return dl.SaveAs(saveTo)
}

func (b *Browser) Scroll(y int) error {
	_, err := b.page.Evaluate(fmt.Sprintf("window.scrollBy(0, %d)", y))
	return err
}

func (b *Browser) Screenshot(path string, fullPage bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, err := b.page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(path),
		FullPage: playwright.Bool(fullPage),
	})
	return err
}

// SnapshotEvents returns + clears the per-step event buffers.
func (b *Browser) SnapshotEvents() (cons []ConsoleEntry, net []NetworkEntry, errs []string) {
	cons = b.ConsoleMessages
	net = b.FailedRequests
	errs = b.PageErrors
	b.ConsoleMessages = nil
	b.FailedRequests = nil
	b.PageErrors = nil
	return
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
