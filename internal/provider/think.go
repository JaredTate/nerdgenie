package provider

import (
	"fmt"

	"github.com/JaredTate/coeus/internal/contract"
)

// thinkFor is the level one call is made at. There are two places a level can
// come from and this is the only place they meet: the request carries the level
// the person chose with "/think" this session, and it wins; when it carries
// none, the level written on the model alias in config.toml stands.
func thinkFor(request contract.Request, alias contract.ModelAlias) contract.Think {
	if request.Think != contract.ThinkDefault {
		return request.Think
	}
	return alias.Think
}

// CheckThink says whether one model can be asked to think at one level, and
// says what is wrong when it cannot. It is what the "/think" command asks
// before it changes anything, and what every provider in this package asks
// before it builds a call, so that the rules live in one place.
//
// There is one model-specific rule. The claude program has no way to switch its
// thinking off: with MAX_THINKING_TOKENS=0 in its environment and --effort high,
// version 2.1.259 still thought (319 and 379 thinking tokens on the same puzzle
// against 450 without the variable), so "off" is refused there by name rather
// than accepted and quietly ignored.
func CheckThink(alias contract.ModelAlias, level contract.Think) error {
	if !contract.KnownThink(level) {
		return fmt.Errorf("the model %q is asked to think at %q, and the levels are %s, so ask for one of those",
			alias.Name, level, contract.ThinkLevelsSentence())
	}
	if level == contract.ThinkOff && alias.Provider == contract.ProviderCommandLine && alias.Program == contract.ClaudeProgram {
		return fmt.Errorf("the %s program cannot switch its thinking off, so ask the model %q to think at %q instead of %q",
			contract.ClaudeProgram, alias.Name, contract.ThinkLow, contract.ThinkOff)
	}
	return nil
}

// asksForThinking says whether a level asks the model to think at all, which is
// every level above off. The empty default asks for nothing, and a provider
// that has to say one way or the other treats it as it treated it before this
// setting existed.
func asksForThinking(level contract.Think) bool {
	return level != contract.ThinkDefault && level != contract.ThinkOff
}
