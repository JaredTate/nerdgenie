//go:build integration

package config_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

// TestARealConfigurationFileLoadsAndTheDoctorReadsTheSameHome writes the file
// "coeus init" would write into a temporary home on the real filesystem, loads
// it, and runs the doctor over it, which is the whole of what wave 3 asks of
// this package.
func TestARealConfigurationFileLoadsAndTheDoctorReadsTheSameHome(t *testing.T) {
	home := testkit.NewTempHome(t)
	t.Setenv(config.HomeVariable, home.Root)

	document := strings.Join([]string{
		"# The configuration coeus init writes on a fresh machine.",
		`defaultmodel = "local"`,
		`fallbackchain = ["cloud"]`,
		`signalaccount = "+15125550123"`,
		`handofftimeout = "30m"`,
		"",
		"[[models]]",
		`name = "local"`,
		`provider = "openai"`,
		`baseaddress = "http://127.0.0.1:19091/v1"`,
		`modelname = "local-coder"`,
		"contextlength = 262144",
		"",
		"[[models]]",
		`name = "cloud"`,
		`provider = "cli"`,
		`program = "claude"`,
		`modelname = "opus"`,
		"contextlength = 200000",
		"",
		"[caps]",
		"roundspertask = 100",
		`timepertask = "1h"`,
		`timepertool = "7m"`,
		`timeperturn = "15m"`,
		"queuedmessages = 100",
		"",
	}, "\n")
	if err := os.WriteFile(home.ConfigFile(), []byte(document), contract.DataFileMode); err != nil {
		t.Fatalf("cannot write the configuration file: %v", err)
	}

	found, err := config.HomeFolder()
	if err != nil {
		t.Fatalf("finding the home folder from %s failed: %v", config.HomeVariable, err)
	}
	if found.Root != home.Root {
		t.Fatalf("the home folder is %q, want the temporary one at %q", found.Root, home.Root)
	}

	settings, err := config.Load(found)
	if err != nil {
		t.Fatalf("loading a real configuration file from disk failed: %v", err)
	}
	if len(settings.Models) != 2 || settings.FallbackChain[0] != "cloud" {
		t.Errorf("the configuration came back as %+v, want the two aliases and the fallback the file names", settings)
	}
	if want := filepath.Join(home.BrowserFolder(), "default"); settings.BrowserProfilePath != want {
		t.Errorf("the browser profile is %q, want %q inside the temporary home", settings.BrowserProfilePath, want)
	}

	report := config.Doctor(context.Background(), found)
	if report.Root != home.Root {
		t.Errorf("the doctor looked in %q, want the temporary home %q", report.Root, home.Root)
	}
	if finding := findingAbout(t, report, "config.toml"); finding.Result != config.Fine {
		t.Errorf("the doctor reports the real configuration file %s: %s, want it fine", finding.Result, finding.Detail)
	}
	if printed := report.String(); !strings.Contains(printed, home.Root) {
		t.Errorf("the printed report does not name the home folder:\n%s", printed)
	}
}
