package recorder

import "testing"

func TestParseCodegen(t *testing.T) {
	src := `import asyncio
from playwright.async_api import async_playwright

async def main():
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=False)
        context = await browser.new_context()
        page = await context.new_page()
        await page.goto("https://app.example.com/login")
        await page.get_by_role("textbox", name="Email").fill("qa@example.com")
        await page.get_by_role("textbox", name="Password").fill("hunter2")
        await page.get_by_role("button", name="Sign in").click()
        await page.goto("https://app.example.com/dashboard")
        await page.get_by_text("Welcome").click()
`
	base, steps := parseCodegen(src)
	if base != "https://app.example.com/login" {
		t.Errorf("base url wrong: %q", base)
	}
	if len(steps) < 8 {
		t.Errorf("expected several steps, got %d", len(steps))
	}
	// wait_for_networkidle should appear after every goto / click / press.
	waits := 0
	for _, s := range steps {
		if s.Action == "wait_for_networkidle" {
			waits++
		}
	}
	if waits < 2 {
		t.Errorf("expected wait_for_networkidle to be auto-inserted after goto/click; got %d waits", waits)
	}
}

func TestParseCodegen_LocatorBased(t *testing.T) {
	src := `        await page.goto("https://example.com")
        await page.locator("#submit").click()
        await page.locator("input[name=q]").fill("hello")
`
	base, steps := parseCodegen(src)
	if base != "https://example.com" {
		t.Errorf("base url wrong: %q", base)
	}
	foundClick, foundFill := false, false
	for _, s := range steps {
		if s.Action == "click" && s.Selector == "#submit" {
			foundClick = true
		}
		if s.Action == "fill" && s.Selector == "input[name=q]" && s.Value == "hello" {
			foundFill = true
		}
	}
	if !foundClick {
		t.Errorf("locator click not parsed")
	}
	if !foundFill {
		t.Errorf("locator fill not parsed")
	}
}

func TestParseCodegen_FileUpload(t *testing.T) {
	src := `        await page.goto("https://example.com/upload")
        await page.locator("input[type=file]").set_input_files("/tmp/my-file.png")
`
	_, steps := parseCodegen(src)
	found := false
	for _, s := range steps {
		if s.Action == "upload_file" && s.Selector == "input[type=file]" && s.Path == "/tmp/my-file.png" {
			found = true
		}
	}
	if !found {
		t.Errorf("set_input_files not parsed into upload_file step")
		for i, s := range steps {
			t.Logf("step %d: %+v", i, s)
		}
	}
}
