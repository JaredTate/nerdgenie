package main

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/config"
	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// FuzzTheSettingWriterNeverBreaksTheConfiguration throws any configuration file
// at the writer behind "nerdgenie signal link" and holds the one rule that matters:
// a file that loaded before still loads afterwards, and the account is really in
// it. The configuration is text from outside the program, and a write that
// breaks it takes every subcommand down with exit 78, which the service unit
// reads as "do not restart".
func FuzzTheSettingWriterNeverBreaksTheConfiguration(f *testing.F) {
	f.Add("default_model = \"local\"\n")
	f.Add("default_model = \"local\"\n\n[[models]]\nname = \"local\"\n")
	f.Add("[[models]]\nname = \"local\"\n")
	f.Add("signal_account = \"+15550000000\"\ndefault_model = \"local\"\n")
	f.Add("# a comment\n\n[caps]\nrounds_per_task = 10\n")
	f.Add("")
	f.Add("\x00\xff\xfe")

	home := testkit.NewTempHome(f)
	f.Fuzz(func(t *testing.T, written string) {
		before, loadedBefore := config.Parse(home, []byte(written))

		after := replaceOrAddSetting(written, accountSetting, accountSetting+" = \"+15550001111\"")
		settings, err := config.Parse(home, []byte(after))

		if loadedBefore != nil {
			// A file that did not load before says nothing about the writer.
			return
		}
		if err != nil {
			t.Fatalf("a configuration that loaded before does not load after the account was written into it:\n%v\n\nbefore:\n%q\n\nafter:\n%q",
				err, written, after)
		}
		if settings.SignalAccount != "+15550001111" {
			t.Fatalf("the account was written as %q rather than the number linked; the file was:\n%q\n\nand became:\n%q",
				settings.SignalAccount, written, after)
		}
		if !strings.HasSuffix(after, "\n") {
			t.Fatalf("the configuration was left without a newline on the end: %q", after)
		}
		_ = before
	})
}

// FuzzTheRecordLineIsAlwaysOneLine holds the rule the strip needs: whatever a
// user asked for, the line a screen draws about a task is one line.
func FuzzTheRecordLineIsAlwaysOneLine(f *testing.F) {
	f.Add("count the jars")
	f.Add("a line\nand another")
	f.Add(strings.Repeat("long ", 200))
	f.Add("")

	f.Fuzz(func(t *testing.T, ask string) {
		running := &agent{settings: contract.DefaultConfig()}
		running.noteRecordLine("task 3 started · " + ask)

		held := running.lastRecordLine()
		if strings.Contains(held, "\n") {
			t.Fatalf("the record line holds a line break, so the strip would be torn in two: %q", held)
		}
	})
}
