package config_test

import (
	"path/filepath"
	"slices"
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
	f.Add("default_model = \"local\"\n")
	f.Add("[caps]\nrounds_per_task = 100\ntime_per_task = \"1h\"\n")
	f.Add("[[models]]\nname = \"local\"\nprovider = \"openai\"\nbase_address = \"http://127.0.0.1:19091/v1\"\nmodel_name = \"local-coder\"\ncontext_length = 262144\n")
	f.Add("handoff_timeout = 1800000000000\n")
	f.Add("[[[models]]]\n")
	f.Add("default_model = \n")
	f.Add("\x00\x01\x02")
	f.Add("models = [{name = \"x\"}]\n")
	f.Add("[caps]\nrounds_per_task = 99999999999999999999\n")
	f.Add("signal_account = \"+15125550123\"\nsandbox_roots = [\"/\"]\n")
	f.Add("a.b.c.d.e.f.g.h = 1\n")
	f.Add("ask_me_first = []\n")
	f.Add("[[permission_rules]]\ntool = \"shell\"\npattern = \"git push*\"\naction = \"ask\"\n")

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
		for _, entry := range settings.AskMeFirst {
			if !slices.Contains(contract.DefaultAskMeFirst(), entry) {
				t.Errorf("this configuration was accepted with the ask-me-first entry %q, which nobody ships:\n%q", entry, document)
			}
		}
		for _, rule := range settings.PermissionRules {
			if rule.Tool == "" || rule.Pattern == "" || !knownRuleAction(rule.Action) {
				t.Errorf("this configuration was accepted with the permission rule %+v:\n%q", rule, document)
			}
		}
	})
}

// knownRuleAction says whether an action is one a user may write in a rule. The
// fourth ruling, stop, is what the permission function decides for an unattended
// run, and is never written in the file.
func knownRuleAction(action contract.PermissionRuling) bool {
	return action == contract.RulingAllow || action == contract.RulingAsk || action == contract.RulingDeny
}
