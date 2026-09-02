package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
)

// settingsChecker carries what every check needs: the configuration being
// checked, the file it came from, which line each key is written on, and the
// user's home directory, which the sandbox rules are measured from.
type settingsChecker struct {
	settings contract.Config
	path     string
	lines    keyLine
	userHome string
}

// run works through the checks in the order a person reads the file, and returns
// the first thing that is wrong. One problem at a time is easier to fix than a
// list, because fixing the first often fixes the rest.
func (checker settingsChecker) run() error {
	for _, one := range []func() error{
		checker.checkModelAliases,
		checker.checkDefaultAndFallback,
		checker.checkSandboxRoots,
		checker.checkCaps,
		checker.checkLengthsOfTime,
		checker.checkSignalAccount,
		checker.checkAddressesAndPaths,
		checker.checkAskMeFirst,
		checker.checkPermissionRules,
	} {
		if err := one(); err != nil {
			return err
		}
	}
	return nil
}

// complain builds a problem about one key, with the line the file writes it on.
func (checker settingsChecker) complain(key string, advice string) error {
	return Problem{Path: checker.path, Line: checker.lines.of(key), Key: key, Advice: advice}
}

// checkModelAliases holds the rules every model alias must obey: a provider kind
// that is one of the three, an address or a program to match it, a model name, a
// context length above zero, and a key that is a reference rather than a secret.
func (checker settingsChecker) checkModelAliases() error {
	if len(checker.settings.Models) == 0 {
		return checker.complain("models", "there are no model aliases, so add a [[models]] block naming at least one model to talk to")
	}
	for at, alias := range checker.settings.Models {
		where := "models." + strconv.Itoa(at)
		if err := checker.checkOneAlias(where, alias); err != nil {
			return err
		}
	}
	return nil
}

// checkOneAlias holds the rules for a single alias, named by its place in the
// file so that the message points at the right block.
func (checker settingsChecker) checkOneAlias(where string, alias contract.ModelAlias) error {
	switch {
	case strings.TrimSpace(alias.Name) == "":
		return checker.complain(where+".name", "this model alias has no name, so give it the short name you want to call it by, such as \"local\"")
	case !contract.KnownProviderKind(alias.Provider):
		return checker.complain(where+".provider", fmt.Sprintf(
			"the provider %q is not one Coeus knows, so use one of %s", alias.Provider, listOfProviderKinds()))
	case strings.TrimSpace(alias.ModelName) == "":
		return checker.complain(where+".model_name", "this model alias does not say which model to ask for, so set model_name to the name the server or the program knows it by")
	case alias.ContextLength <= 0:
		return checker.complain(where+".context_length", fmt.Sprintf(
			"the context length is %d, so set it to how many tokens the model can hold, which is the number the working context is sized from", alias.ContextLength))
	}
	if err := checker.checkAliasReach(where, alias); err != nil {
		return err
	}
	if alias.KeyReference != "" {
		if _, isReference := contract.SecretReferenceName(alias.KeyReference); !isReference {
			return checker.complain(where+".key_reference", fmt.Sprintf(
				"the key is %q, and a key is never written here, so put it in the vault and refer to it as %sname",
				alias.KeyReference, contract.SecretReferencePrefix))
		}
	}
	return nil
}

// checkAliasReach holds the part of an alias that says how to reach the model:
// an OpenAI-compatible server needs an address, and a command-line provider
// needs one of the two vendor programs.
func (checker settingsChecker) checkAliasReach(where string, alias contract.ModelAlias) error {
	if alias.Provider == contract.ProviderOpenAI && strings.TrimSpace(alias.BaseAddress) == "" {
		return checker.complain(where+".base_address",
			"an openai alias needs the address of the server that answers it, such as \"http://127.0.0.1:19091/v1\"")
	}
	if alias.BaseAddress != "" && !isWebAddress(alias.BaseAddress) {
		return checker.complain(where+".base_address", fmt.Sprintf(
			"the address %q is not a web address, so write it with a scheme and a host, such as \"http://127.0.0.1:19091/v1\"", alias.BaseAddress))
	}
	if alias.Provider == contract.ProviderCommandLine && alias.Program != contract.ClaudeProgram && alias.Program != contract.CodexProgram {
		return checker.complain(where+".program", fmt.Sprintf(
			"a cli alias runs the vendor's own program, so set program to %q or %q rather than %q",
			contract.ClaudeProgram, contract.CodexProgram, alias.Program))
	}
	return nil
}

// checkDefaultAndFallback holds the rule that the model Coeus reaches for, and
// every model it falls back to, is an alias the file actually defines.
func (checker settingsChecker) checkDefaultAndFallback() error {
	named := make([]string, 0, len(checker.settings.Models))
	for _, alias := range checker.settings.Models {
		named = append(named, alias.Name)
	}
	if checker.settings.DefaultModel == "" {
		return checker.complain("default_model", fmt.Sprintf(
			"no model is the default, so set default_model to one of %s", strings.Join(quoteEach(named), ", ")))
	}
	if !slices.Contains(named, checker.settings.DefaultModel) {
		return checker.complain("default_model", fmt.Sprintf(
			"there is no model alias called %q, so name one of %s or add a [[models]] block for it",
			checker.settings.DefaultModel, strings.Join(quoteEach(named), ", ")))
	}
	for _, fallback := range checker.settings.FallbackChain {
		if !slices.Contains(named, fallback) {
			return checker.complain("fallback_chain", fmt.Sprintf(
				"the fallback chain names %q, and there is no model alias called that, so use one of %s",
				fallback, strings.Join(quoteEach(named), ", ")))
		}
	}
	return nil
}

// checkSandboxRoots holds the rule from design section 11: the folders a
// sandboxed command may reach are full paths, and none of them is the agent's
// own home folder, the vault, the browser profiles, or the user's SSH keys, nor
// a folder above the user's home directory, which would put every account on the
// machine inside the fence. The user's home directory itself is allowed, and is
// the shipped default, because the sandbox masks the excluded paths out of it.
func (checker settingsChecker) checkSandboxRoots() error {
	if len(checker.settings.SandboxRoots) == 0 {
		return checker.complain("sandbox_roots", "there are no sandbox roots, so a sandboxed command could reach nothing; leave the key out to use the default")
	}
	for _, root := range checker.settings.SandboxRoots {
		if err := contract.CheckSandboxRoot(root, checker.userHome); err != nil {
			return checker.complain("sandbox_roots", err.Error())
		}
		if clean := filepath.Clean(root); clean != checker.userHome && folderHolds(clean, checker.userHome) {
			return checker.complain("sandbox_roots", fmt.Sprintf(
				"the sandbox root %q is above your home directory %q, so a sandboxed command could reach every account on the machine; use your home directory or a folder inside it",
				root, checker.userHome))
		}
	}
	return nil
}

// checkCaps holds the rule that every cap counted in whole things is above zero,
// because a cap of zero stops the agent before it starts and a cap below zero
// means nothing at all.
func (checker settingsChecker) checkCaps() error {
	caps := checker.settings.Caps
	memory := checker.settings.MemoryCaps
	for _, limit := range []struct {
		key    string
		amount int
	}{
		{"caps.rounds_per_task", caps.RoundsPerTask},
		{"caps.queued_messages", caps.QueuedMessages},
		{"caps.tool_output_bytes", caps.ToolOutputBytes},
		{"caps.identical_call_window", caps.IdenticalCallWindow},
		{"memory_caps.world_facts_bytes", memory.WorldFactsBytes},
		{"memory_caps.user_facts_bytes", memory.UserFactsBytes},
	} {
		if limit.amount <= 0 {
			return checker.complain(limit.key, fmt.Sprintf(
				"this cap is %d, and a cap of zero or less stops the agent before it starts, so set it above zero or leave the key out to use the default",
				limit.amount))
		}
	}
	return nil
}

// checkLengthsOfTime holds the rule that every budget of time is above zero,
// including the handoff timeout, which is how long a browser handoff waits for
// the person before giving up.
func (checker settingsChecker) checkLengthsOfTime() error {
	caps := checker.settings.Caps
	for _, budget := range []struct {
		key    string
		amount time.Duration
	}{
		{"caps.time_per_task", caps.TimePerTask},
		{"caps.time_per_tool", caps.TimePerTool},
		{"caps.time_per_turn", caps.TimePerTurn},
		{"handoff_timeout", checker.settings.HandoffTimeout},
	} {
		if budget.amount <= 0 {
			return checker.complain(budget.key, fmt.Sprintf(
				"this is %s, and a length of time that is zero or less leaves no time to work, so write one such as \"30m\" or leave the key out to use the default",
				budget.amount))
		}
	}
	return nil
}

// checkSignalAccount holds the rule that the phone number Coeus is linked to is
// written the way Signal writes it, with a plus and a country code.
func (checker settingsChecker) checkSignalAccount() error {
	account := checker.settings.SignalAccount
	if account == "" || looksLikeAPhoneNumber(account) {
		return nil
	}
	return checker.complain("signal_account", fmt.Sprintf(
		"the Signal account %q is not a phone number in international form, so write it as a plus, a country code, and the number, such as \"+15125550123\"", account))
}

// checkAddressesAndPaths holds the rule that the three settings pointing
// somewhere else, the nightly backup, the browser profile, and the search
// server, are written so that something can be done with them.
func (checker settingsChecker) checkAddressesAndPaths() error {
	if backup := checker.settings.BackupPath; !filepath.IsAbs(backup) {
		return checker.complain("backup_path", fmt.Sprintf(
			"the backup path %q is not a full path, so write it starting from the root of the filesystem, or leave the key out to use the backups folder in the home folder", backup))
	}
	if profile := checker.settings.BrowserProfilePath; !filepath.IsAbs(profile) {
		return checker.complain("browser_profile_path", fmt.Sprintf(
			"the browser profile %q is not a full path, so write it starting from the root of the filesystem, or leave the key out to use the default profile in the home folder", profile))
	}
	if search := checker.settings.SearchServerAddress; search != "" && !isWebAddress(search) {
		return checker.complain("search_server_address", fmt.Sprintf(
			"the search server address %q is not a web address, so write it with a scheme and a host, such as \"https://search.example.com\", or leave the key out to search DuckDuckGo instead", search))
	}
	return nil
}

// checkAskMeFirst holds the rule that every entry the user keeps on the
// ask-me-first list is one of the three the design ships. The list may be
// emptied, which is the user saying they want to be asked about nothing, but it
// cannot name something the permission function has never heard of.
func (checker settingsChecker) checkAskMeFirst() error {
	shipped := contract.DefaultAskMeFirst()
	for _, entry := range checker.settings.AskMeFirst {
		if !slices.Contains(shipped, entry) {
			return checker.complain("ask_me_first", fmt.Sprintf(
				"nothing on the ask-me-first list is called %q, so use one of %s, or empty the list to be asked about nothing",
				entry, strings.Join(quoteEach(shipped), ", ")))
		}
	}
	return nil
}

// checkPermissionRules holds the rules a rule of the user's own must obey: it
// names a tool, it has a pattern to match on, and it says what to do.
func (checker settingsChecker) checkPermissionRules() error {
	for at, rule := range checker.settings.PermissionRules {
		where := "permission_rules." + strconv.Itoa(at)
		if err := checker.checkOneRule(where, rule); err != nil {
			return err
		}
	}
	return nil
}

// checkOneRule holds the three parts of one line of the user's rulebook, named
// by its place in the file so that the message points at the right block.
func (checker settingsChecker) checkOneRule(where string, rule contract.PermissionRule) error {
	switch {
	case strings.TrimSpace(rule.Tool) == "":
		return checker.complain(where+".tool",
			"this rule does not say which tool it is about, so name a tool or write \"*\" for every tool")
	case strings.TrimSpace(rule.Pattern) == "":
		return checker.complain(where+".pattern",
			"this rule has nothing to match on, so give it a pattern such as \"git push*\", or \"*\" for every call")
	case !writableRuleAction(rule.Action):
		return checker.complain(where+".action", fmt.Sprintf(
			"this rule says to %q, and a rule you write can only say %q, %q, or %q; stopping is what the harness decides on its own when a run is unattended",
			rule.Action, contract.RulingAllow, contract.RulingAsk, contract.RulingDeny))
	}
	return nil
}

// writableRuleAction says whether an action is one a user may write in a rule.
// The fourth ruling, stop, is what the permission function decides for an
// unattended run that has nobody to answer it, so it never appears in the file.
func writableRuleAction(action contract.PermissionRuling) bool {
	switch action {
	case contract.RulingAllow, contract.RulingAsk, contract.RulingDeny:
		return true
	default:
		return false
	}
}

// isWebAddress says whether the text is a web address something can be fetched
// from: it parses, and it has both a scheme and a host.
func isWebAddress(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}
	return parsed.Scheme != "" && parsed.Host != ""
}

// looksLikeAPhoneNumber says whether the text is a phone number in international
// form: a plus, then seven to fifteen digits, the first of which is not a zero.
func looksLikeAPhoneNumber(account string) bool {
	digits, found := strings.CutPrefix(account, "+")
	if !found || len(digits) < 7 || len(digits) > 15 || digits[0] == '0' {
		return false
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// listOfProviderKinds writes the three provider kinds out for an error message.
func listOfProviderKinds() string {
	names := []string{}
	for _, kind := range contract.ProviderKinds() {
		names = append(names, string(kind))
	}
	return strings.Join(quoteEach(names), ", ")
}

// quoteEach puts quotation marks round every name in a list, so that an empty
// name in a message is visible rather than a gap.
func quoteEach(names []string) []string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, strconv.Quote(name))
	}
	return quoted
}

// folderHolds says whether a folder is a path or holds it somewhere inside,
// which is how the sandbox rules ask whether one root swallows another.
func folderHolds(folder string, path string) bool {
	folder, path = filepath.Clean(folder), filepath.Clean(path)
	if folder == path {
		return true
	}
	separator := string(filepath.Separator)
	return strings.HasPrefix(path, strings.TrimSuffix(folder, separator)+separator)
}
