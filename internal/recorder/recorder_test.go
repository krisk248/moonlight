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

	// First step should be goto / (relative), then screenshot, then 3 actions, etc.
	if len(steps) < 8 {
		t.Errorf("expected several steps, got %d", len(steps))
	}

	// Find the fill that captured Email
	var emailFill, signin *struct{ idx int }
	_ = signin
	for i, s := range steps {
		if s.Action == "fill" && s.Role == "textbox" && s.RoleName == "Email" {
			emailFill = &struct{ idx int }{i}
		}
	}
	if emailFill == nil {
		t.Errorf("did not parse get_by_role textbox Email .fill")
		for i, s := range steps {
			t.Logf("step %d: %+v", i, s)
		}
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
