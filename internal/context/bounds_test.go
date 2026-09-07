package context

import "testing"

// TestEveryNamedSizeIsTheNumberItIsMeantToBe is finding 19 of the wave 6 gate
// review. Three of this package's six named sizes could be set to anything at
// all and every test still passed: SoulBytes at 400, BoundaryLength at 8,
// MaxInstructionWords at 900. A number nothing pins is a number a careless edit
// moves without anybody noticing.
//
// Each is written here as the literal it is meant to be, with the reason it is
// that number, so that changing one means changing this test and reading the
// reason first. The three that were already pinned are the token estimate's
// ratios, which a test catches when the estimate reads low.
func TestEveryNamedSizeIsTheNumberItIsMeantToBe(t *testing.T) {
	for _, check := range []struct {
		name string
		is   int
		want int
		why  string
	}{
		{"SoulBytes", SoulBytes, 4000,
			"SOUL.md rides in every prompt to every model, and four thousand bytes is about six hundred words, which is a page"},
		{"BoundaryLength", BoundaryLength, 16,
			"sixteen hexadecimal characters is eight random bytes, which nothing the agent reads can guess in the life of one task"},
		{"MaxInstructionWords", MaxInstructionWords, 600,
			"design section 5 says the instruction text is under six hundred words since the paragraph on the project's documents joined it on 7 September 2026, and this is that promise held to"},
		{"MaxSkillsInPrompt", MaxSkillsInPrompt, 20,
			"the store holds two hundred skills, and twenty lines is as much of that as may ride above the cache line on every call"},
		{"MaxSkillLineRunes", MaxSkillLineRunes, 200,
			"a skill's one-line description is capped at two hundred characters in the store, so its line in the prompt is capped at the same"},
	} {
		if check.is != check.want {
			t.Errorf("%s is %d and it is meant to be %d: %s", check.name, check.is, check.want, check.why)
		}
	}
}

// TestABoundaryTooShortOrOfAnOddLengthIsRefused is the other half of finding 19.
// NewBoundary asked for BoundaryLength/2 random bytes, so an odd length would
// have quietly made one character fewer than the name promises, and a short one
// would have made a boundary a page could work through. Neither is caught by
// reading the constant, so it is caught here.
func TestABoundaryTooShortOrOfAnOddLengthIsRefused(t *testing.T) {
	for _, check := range []struct {
		name    string
		length  int
		allowed bool
	}{
		{"the length the harness ships with", BoundaryLength, true},
		{"a longer even length", 32, true},
		{"an odd length, which would make half a byte fewer than it says", 17, false},
		{"an even length under the smallest safe one", smallestBoundary - 2, false},
		{"no length at all", 0, false},
		{"a negative length", -2, false},
	} {
		bytes, err := boundaryBytes(check.length)
		if check.allowed && err != nil {
			t.Errorf("%s (%d) was refused: %v", check.name, check.length, err)
			continue
		}
		if !check.allowed {
			if err == nil {
				t.Errorf("%s (%d) was allowed, and it makes a boundary that is not what its name says", check.name, check.length)
			}
			continue
		}
		if bytes*2 != check.length {
			t.Errorf("%s (%d) asks for %d random bytes, which writes %d characters, not %d",
				check.name, check.length, bytes, bytes*2, check.length)
		}
	}
}
