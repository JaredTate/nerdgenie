package contract_test

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
	"unicode"

	"github.com/JaredTate/coeus/internal/contract"
)

func TestDefaultConfigMatchesTheDocumentedDefaults(t *testing.T) {
	settings := contract.DefaultConfig()

	if settings.DefaultModel != contract.LocalModelAlias {
		t.Errorf("default model is %q, want %q", settings.DefaultModel, contract.LocalModelAlias)
	}
	if len(settings.Models) != 1 {
		t.Fatalf("default configuration ships %d model aliases, want exactly the local one", len(settings.Models))
	}

	local := settings.Models[0]
	if local.Name != contract.LocalModelAlias {
		t.Errorf("the shipped alias is named %q, want %q", local.Name, contract.LocalModelAlias)
	}
	if local.Provider != contract.ProviderOpenAI {
		t.Errorf("the local alias speaks %q, want %q", local.Provider, contract.ProviderOpenAI)
	}
	if local.BaseAddress != "http://127.0.0.1:19091/v1" {
		t.Errorf("the local alias points at %q, want the llama-server daemon", local.BaseAddress)
	}
	if local.ModelName != "local-coder" {
		t.Errorf("the local alias asks for model %q, want local-coder", local.ModelName)
	}
	if local.ContextLength != 262144 {
		t.Errorf("the local alias holds %d tokens, want 262144", local.ContextLength)
	}
	if local.KeyReference != "" {
		t.Errorf("the local alias wants key %q, want no key at all", local.KeyReference)
	}
}

func TestDefaultConfigCarriesTheCapsFromTheDesign(t *testing.T) {
	caps := contract.DefaultConfig().Caps

	checks := []struct {
		what string
		got  int
		want int
	}{
		{"rounds per task", caps.RoundsPerTask, 100},
		{"queued messages", caps.QueuedMessages, 100},
		{"identical-call window", caps.IdenticalCallWindow, 20},
		{"tool output bytes", caps.ToolOutputBytes, 30000},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("the cap on %s is %d, want %d", check.what, check.got, check.want)
		}
	}

	durations := []struct {
		what string
		got  time.Duration
		want time.Duration
	}{
		{"time per task", caps.TimePerTask, time.Hour},
		{"time per tool", caps.TimePerTool, 7 * time.Minute},
		{"time per turn", caps.TimePerTurn, 15 * time.Minute},
	}
	for _, check := range durations {
		if check.got != check.want {
			t.Errorf("the cap on %s is %s, want %s", check.what, check.got, check.want)
		}
	}
}

func TestDefaultConfigCarriesTheMemoryCapsAndTheHandoffTimeout(t *testing.T) {
	settings := contract.DefaultConfig()

	if settings.MemoryCaps.WorldFactsBytes <= 0 {
		t.Errorf("MEMORY.md has a cap of %d bytes, want a positive cap", settings.MemoryCaps.WorldFactsBytes)
	}
	if settings.MemoryCaps.UserFactsBytes <= 0 {
		t.Errorf("USER.md has a cap of %d bytes, want a positive cap", settings.MemoryCaps.UserFactsBytes)
	}
	if settings.HandoffTimeout <= 0 {
		t.Errorf("the handoff timeout is %s, want a positive timeout", settings.HandoffTimeout)
	}
	if settings.SearchServerAddress != "" {
		t.Errorf("the search server defaults to %q, want empty so the web tool falls back to DuckDuckGo", settings.SearchServerAddress)
	}
	if len(settings.FallbackChain) != 0 {
		t.Errorf("the fallback chain defaults to %v, want empty until the user names a second model", settings.FallbackChain)
	}
}

func TestExcludedFromSandboxNamesTheFourPathsTheDesignProtects(t *testing.T) {
	home := "/home/someone"
	excluded := contract.ExcludedFromSandbox(home)

	wanted := []string{
		filepath.Join(home, ".coeus"),
		filepath.Join(home, ".coeus", "vault.age"),
		filepath.Join(home, ".coeus", "browser"),
		filepath.Join(home, ".ssh"),
	}
	for _, want := range wanted {
		if !slices.Contains(excluded, want) {
			t.Errorf("the sandbox exclusion list is %v, and it is missing %q", excluded, want)
		}
	}
}

func TestCheckSandboxRootRefusesAnythingInsideAnExcludedPath(t *testing.T) {
	home := "/home/someone"

	tests := []struct {
		name    string
		root    string
		refused bool
	}{
		{"the home folder itself is refused, because it holds the excluded paths", home, true},
		{"the work folder is allowed", filepath.Join(home, "coeus"), false},
		{"a project folder is allowed", filepath.Join(home, "Code", "coeus"), false},
		{"the agent's own home folder is refused", filepath.Join(home, ".coeus"), true},
		{"a folder inside the agent's home is refused", filepath.Join(home, ".coeus", "skills"), true},
		{"the vault file is refused", filepath.Join(home, ".coeus", "vault.age"), true},
		{"the browser profile folder is refused", filepath.Join(home, ".coeus", "browser"), true},
		{"the ssh key folder is refused", filepath.Join(home, ".ssh"), true},
		{"a folder inside ssh is refused", filepath.Join(home, ".ssh", "keys"), true},
		{"a relative path is refused because it cannot be checked", "work", true},
		{"an empty root is refused", "", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := contract.CheckSandboxRoot(test.root, home)
			if test.refused && err == nil {
				t.Fatalf("root %q was allowed, want it refused", test.root)
			}
			if !test.refused && err != nil {
				t.Fatalf("root %q was refused with %q, want it allowed", test.root, err)
			}
		})
	}
}

func TestDefaultSandboxRootsPassTheirOwnCheck(t *testing.T) {
	home := "/home/someone"
	for _, root := range contract.DefaultSandboxRoots(home) {
		if err := contract.CheckSandboxRoot(root, home); err != nil {
			t.Errorf("the default sandbox root %q fails its own check: %v", root, err)
		}
	}
}

// TestEveryConfigurationFieldHasASnakeCaseTOMLKey pins the key names a person
// writes in config.toml: the field name in lower case with underscores between
// the words, such as default_model, which is what TOML files conventionally use.
func TestEveryConfigurationFieldHasASnakeCaseTOMLKey(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeFor[contract.Config](),
		reflect.TypeFor[contract.ModelAlias](),
		reflect.TypeFor[contract.Caps](),
		reflect.TypeFor[contract.MemoryCaps](),
	}
	for _, typ := range types {
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			want := snakeCase(field.Name)
			if got := field.Tag.Get("toml"); got != want {
				t.Errorf("%s.%s has the toml key %q, want %q", typ.Name(), field.Name, got, want)
			}
		}
	}
}

// snakeCase turns a Go field name such as BaseAddress into base_address.
func snakeCase(name string) string {
	var out []rune
	for index, letter := range name {
		if index > 0 && letter >= 'A' && letter <= 'Z' {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(letter))
	}
	return string(out)
}

func TestTheConfigurationCarriesTheAskMeFirstListAndTheUserRules(t *testing.T) {
	defaults := contract.DefaultConfig()
	if !reflect.DeepEqual(defaults.AskMeFirst, contract.DefaultAskMeFirst()) {
		t.Errorf("a fresh configuration's ask-me-first list is %v, want the three shipped entries %v", defaults.AskMeFirst, contract.DefaultAskMeFirst())
	}
	if len(defaults.PermissionRules) != 0 {
		t.Errorf("a fresh configuration has %d user rules, want none", len(defaults.PermissionRules))
	}
	rule := contract.PermissionRule{Tool: "shell", Pattern: "git push*", Action: contract.RulingAsk}
	if rule.Tool != "shell" || rule.Pattern != "git push*" || rule.Action != contract.RulingAsk {
		t.Errorf("a permission rule did not hold its three parts: %+v", rule)
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[contract.PermissionRule]()} {
		for index := 0; index < typ.NumField(); index++ {
			field := typ.Field(index)
			if got, want := field.Tag.Get("toml"), snakeCase(field.Name); got != want {
				t.Errorf("%s.%s has the toml key %q, want %q", typ.Name(), field.Name, got, want)
			}
		}
	}
}

func TestTheDefaultSandboxRootIsAWorkFolderAndARootMayNotHoldAnExcludedPath(t *testing.T) {
	userHome := filepath.Join("/home", "someone")
	roots := contract.DefaultSandboxRoots(userHome)
	if len(roots) != 1 || roots[0] != filepath.Join(userHome, "coeus") {
		t.Errorf("the default sandbox roots are %v, want the one work folder %s", roots, filepath.Join(userHome, "coeus"))
	}
	for _, root := range []string{userHome, "/home", "/", filepath.Join(userHome, ".coeus"), filepath.Join(userHome, ".ssh"), filepath.Join(userHome, ".coeus", "browser")} {
		if err := contract.CheckSandboxRoot(root, userHome); err == nil {
			t.Errorf("the root %q was accepted, and it is or holds a path that must stay outside the sandbox", root)
		}
	}
	for _, root := range []string{filepath.Join(userHome, "coeus"), filepath.Join(userHome, "Code"), "/srv/work"} {
		if err := contract.CheckSandboxRoot(root, userHome); err != nil {
			t.Errorf("the root %q was refused: %v", root, err)
		}
	}
}
