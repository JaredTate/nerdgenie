package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/JaredTate/nerdgenie/internal/clock"
	"github.com/JaredTate/nerdgenie/internal/desktop"
)

// openTheDesktop starts the Go side of the desktop when the worker bundle is
// beside the binary, and reports in the doctor's own manner when it is not: the
// agent comes up, the computer tool refuses, and the line says what to do.
func (running *agent) openTheDesktop() *desktop.Desktop {
	bundle := theDesktopBundle()
	if _, err := os.Stat(bundle); err != nil {
		running.note("the desktop worker is not built, so the computer tool is switched off; it belongs at " + bundle)
		return nil
	}
	node, err := exec.LookPath("node")
	if err != nil {
		running.note("node is not on the PATH, so the computer tool is switched off; install Node and start again")
		return nil
	}
	start, err := desktop.ProcessStart([]string{node, bundle}, running.noteLine)
	if err != nil {
		running.note("the desktop worker could not be set up, so the computer tool is switched off: " + err.Error())
		return nil
	}
	made, err := desktop.New(desktop.Options{
		Start:      start,
		Channel:    running.userChannel(),
		Permission: running.decider,
		Clock:      clock.System(),
		Note:       running.noteLine,
	})
	if err != nil {
		running.note("the desktop could not be built, so the computer tool is switched off: " + err.Error())
		return nil
	}
	return made
}

// theDesktopBundle is where the desktop worker is installed: beside the binary,
// under workers/desktop/main.js, which is what "make build" writes.
func theDesktopBundle() string {
	program, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(program), "workers", "desktop", "main.js")
}
