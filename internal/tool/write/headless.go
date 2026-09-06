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

// The two shapes of a headless launch. The option is a Playwright or
// Puppeteer launch whose headless option is on, in any of the spellings those
// libraries take; the flag is Chrome's own, either on a command line that
// runs a browser or quoted as an argument handed to one. Words about the
// thing are not the thing: a chapter
// on crawlers, a README naming the flag, or a setting in a file that launches
// no browser are all written, because the harness writes books as well as
// scripts.
var (
	aHeadlessOption = regexp.MustCompile(`headless\s*[:=]\s*(true|['"](new|old|shell|chrome)['"])`)
	aBrowserLaunch  = regexp.MustCompile(`(?i)launch\s*\(|puppeteer|playwright|chromium|webdriver|selenium`)
	aHeadlessFlag   = regexp.MustCompile(`(?i)\b(google-chrome|chrome|chromium|chromium-browser|msedge|firefox)(\.exe)?(\s+\S+)*\s+--headless\b|['"]--headless(=\w+)?['"]`)
)

// StartsAHeadlessBrowser says whether content that is about to be written
// starts a browser nobody can see: the headless option in a file that
// launches a browser, or the headless flag on a line that runs one.
func StartsAHeadlessBrowser(content string) bool {
	if aHeadlessOption.MatchString(content) && aBrowserLaunch.MatchString(content) {
		return true
	}
	return aHeadlessFlag.MatchString(content)
}
