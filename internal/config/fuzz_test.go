package config_test

import (
	"path/filepath"
	"testing"

	"github.com/JaredTate/coeus/internal/config"
	"github.com/JaredTate/coeus/internal/contract"
)

// FuzzTheLoaderNeverPanics throws arbitrary bytes at the configuration reader.
// Every one of them has to come back either as a configuration that obeys the
// rules or as an error saying what is wrong, and never as a panic, because the
// configuration file is the first outside text the program reads and a crash
// there stops Coeus from starting at all.
func FuzzTheLoaderNeverPanics(f *testing.F) {
	f.Add("")
	f.Add("defaultmodel = \"local\"\n")
	f.Add("[caps]\nroundspertask = 100\ntimepertask = \"1h\"\n")
	f.Add("[[models]]\nname = \"local\"\nprovider = \"openai\"\nbaseaddress = \"http://127.0.0.1:19091/v1\"\nmodelname = \"local-coder\"\ncontextlength = 262144\n")
	f.Add("handofftimeout = 1800000000000\n")
	f.Add("[[[models]]]\n")
	f.Add("defaultmodel = \n")
	f.Add("\x00\x01\x02")
	f.Add("models = [{name = \"x\"}]\n")
	f.Add("[caps]\nroundspertask = 99999999999999999999\n")
	f.Add("signalaccount = \"+15125550123\"\nsandboxroots = [\"/\"]\n")
	f.Add("a.b.c.d.e.f.g.h = 1\n")

	home := contract.NewHome(filepath.Join("/nowhere", contract.HomeFolderName))
	f.Fuzz(func(t *testing.T, document string) {
		settings, err := config.Parse(home, []byte(document))
		if err != nil {
			return
		}
		if settings.DefaultModel == "" {
			t.Errorf("this configuration was accepted with no default model:\n%q", document)
		}
		named := false
		for _, alias := range settings.Models {
			if alias.ContextLength <= 0 {
				t.Errorf("this configuration was accepted with an alias holding %d tokens:\n%q", alias.ContextLength, document)
			}
			if alias.Name == settings.DefaultModel {
				named = true
			}
		}
		if !named {
			t.Errorf("this configuration was accepted with a default model no alias is called:\n%q", document)
		}
		if settings.Caps.RoundsPerTask <= 0 || settings.HandoffTimeout <= 0 {
			t.Errorf("this configuration was accepted with a cap of zero or less:\n%q", document)
		}
		if len(settings.SandboxRoots) == 0 {
			t.Errorf("this configuration was accepted with no sandbox roots:\n%q", document)
		}
	})
}
