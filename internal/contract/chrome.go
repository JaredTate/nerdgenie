package contract

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// chromeNames are the names Chrome goes by on a Linux PATH, in the order they
// are tried. Google's own package installs google-chrome-stable and links
// google-chrome to it, and the distributions' builds are chromium and
// chromium-browser.
var chromeNames = []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser"}

// mostLauncherBytes is how much of a candidate is read to tell a launcher
// script from the real program. A launcher is a few lines, so this holds the
// whole of one, and a real program is hundreds of megabytes whose first two
// bytes already say that it is not a script.
const mostLauncherBytes = 64 * 1024

// scriptMark is how a script begins.
const scriptMark = "#!"

// forcedHeadless is the option a launcher script adds when it starts Chrome
// with no window, whatever it was asked for.
const forcedHeadless = "--headless"

// chromeInstallAdvice is what to do when no Chrome is left.
const chromeInstallAdvice = "install Google Chrome or Chromium"

// FindChrome looks for the real Chrome on the PATH, under the four names it
// goes by, in order, and returns the first that is not a launcher script
// forcing headless. Such a script, one that begins "#!" and whose text holds
// "--headless", is passed over and named in passedOver, because a Chrome with
// no window is no use to an agent that promises the person a window on the
// screen, and it was every Chrome the agent started on jared-rosie on 7
// September 2026. Every folder on the PATH is looked in for each name, not
// only the first that answers, because on that machine the launchers in
// /usr/local/bin hid the real google-chrome-stable in /usr/bin behind them.
// The error names every candidate tried and what to install when none is
// left. The browser worker is handed the path and the doctor reports it, so
// that the two never disagree about which Chrome is meant.
func FindChrome() (path string, passedOver []string, err error) {
	tried := []string{}
	for _, name := range chromeNames {
		candidates := programsNamed(name)
		if len(candidates) == 0 {
			tried = append(tried, name+" is not on the PATH")
			continue
		}
		for _, found := range candidates {
			forcing, unreadable := forcesHeadless(found)
			if unreadable != nil {
				tried = append(tried, fmt.Sprintf("%s at %s could not be read: %v", name, found, unreadable))
				continue
			}
			if forcing {
				passedOver = append(passedOver, found)
				tried = append(tried, fmt.Sprintf("%s at %s is a launcher script that forces %s, which gives the agent no window",
					name, found, forcedHeadless))
				continue
			}
			return found, passedOver, nil
		}
	}
	return "", passedOver, fmt.Errorf("no Chrome that opens a window was found: %s; %s", strings.Join(tried, ", "), chromeInstallAdvice)
}

// programsNamed is every executable file of that name on the PATH, in the
// PATH's order, so that a name shadowed by a launcher in an earlier folder can
// still be found in a later one.
func programsNamed(name string) []string {
	found := []string{}
	for _, folder := range filepath.SplitList(os.Getenv("PATH")) {
		if folder == "" {
			continue
		}
		candidate := filepath.Join(folder, name)
		about, err := os.Stat(candidate)
		if err != nil || about.IsDir() || about.Mode()&0o111 == 0 {
			continue
		}
		found = append(found, candidate)
	}
	return found
}

// forcesHeadless says whether the file is a launcher script that starts Chrome
// with no window: it begins "#!" and its text holds "--headless".
func forcesHeadless(path string) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = file.Close() }()
	head, err := io.ReadAll(io.LimitReader(file, mostLauncherBytes))
	if err != nil {
		return false, err
	}
	return bytes.HasPrefix(head, []byte(scriptMark)) && bytes.Contains(head, []byte(forcedHeadless)), nil
}
