package write

import "regexp"

// TheOnScreenBrowserLine is what a write or an edit is refused with when the
// content it would put on disk starts a browser nobody can see. The browser
// runs on the screen, where the person watches it, and that is a product rule:
// on the fresh game build the model wrote two Playwright scripts that launched
// a headless Chrome and play-tested the game through them, off the screen, in a
// task whose ask said "in Chrome as a human player would".
const TheOnScreenBrowserLine = "refused: this would start a headless browser, and the browser runs on the screen where the person watches it. " +
	"Drive and check a page through the browser tools instead: browser_open, browser_read, browser_click, browser_press, " +
	"browser_screenshot, and browser_read's ask field for a question of the page's own script"

// aHeadlessLaunch matches the ways a script starts a browser with no window: a
// Playwright or Puppeteer launch whose headless option is on, in any of the
// spellings those libraries take, or Chrome's own flag.
var aHeadlessLaunch = regexp.MustCompile(`headless\s*[:=]\s*(true|['"](new|old|shell|chrome)['"])|--headless\b`)

// StartsAHeadlessBrowser says whether content that is about to be written
// starts a browser nobody can see.
func StartsAHeadlessBrowser(content string) bool {
	return aHeadlessLaunch.MatchString(content)
}
