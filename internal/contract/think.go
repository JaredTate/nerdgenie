package contract

import "strings"

// Think is how hard a model is asked to think before it answers. It is written
// on a model alias in config.toml as think = "medium", and the "/think" command
// changes it for the rest of the session. Each provider turns it into whatever
// its own wire calls the same thing.
type Think string

// The six levels a person may ask for, easiest first, and the empty value that
// means none of them.
const (
	// ThinkDefault is the empty value: nothing is said about thinking and the
	// provider's own default stands, which is what Nerd Genie sent before this
	// setting existed.
	ThinkDefault Think = ""
	// ThinkOff asks the model not to think before it answers. A provider that
	// cannot switch its thinking off refuses this level by name rather than
	// pretending it worked.
	ThinkOff Think = "off"
	// ThinkLow asks for the least thinking the model offers.
	ThinkLow Think = "low"
	// ThinkMedium is the middle of the ladder, and is a good trade of
	// thoroughness against what a call costs.
	ThinkMedium Think = "medium"
	// ThinkHigh asks the model to think hard before it answers.
	ThinkHigh Think = "high"
	// ThinkXHigh is one step harder than high, on the models that offer it.
	ThinkXHigh Think = "xhigh"
	// ThinkMax is the most thinking the model offers, for work where being
	// right matters more than what it costs.
	ThinkMax Think = "max"
)

// ThinkLevels returns the six levels a person may name, easiest first, which is
// the order "/think" and every message about a wrong level print them in.
func ThinkLevels() []Think {
	return []Think{ThinkOff, ThinkLow, ThinkMedium, ThinkHigh, ThinkXHigh, ThinkMax}
}

// KnownThink says whether a value is one a person may write: one of the six
// levels, or the empty value, which means the provider's own default.
func KnownThink(level Think) bool {
	if level == ThinkDefault {
		return true
	}
	for _, known := range ThinkLevels() {
		if level == known {
			return true
		}
	}
	return false
}

// ThinkLevelsSentence names the six levels the way a message names them, as
// "off, low, medium, high, xhigh, and max", so that every refusal can tell the
// person what to write instead.
func ThinkLevelsSentence() string {
	named := make([]string, 0, len(ThinkLevels()))
	for _, level := range ThinkLevels() {
		named = append(named, string(level))
	}
	last := len(named) - 1
	return strings.Join(named[:last], ", ") + ", and " + named[last]
}
