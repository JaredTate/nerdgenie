// The rule that only a skill's name and one-line description ride in the prompt,
// with the body loaded when the skill is used, is Hermes' skill listing at
// ~/Code/hermes-agent/tools/skills_tool.py, where the head of every skill folder
// is read for the prompt and nothing more. Design section 8 says the same. The
// Go here is written fresh.

package context

import (
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// The two bounds on the skill list. The store holds up to two hundred skills
// with a description of up to two hundred characters each, and all of that in
// front of the model on every call would be forty thousand characters of list,
// so the list stops at twenty lines and every line at two hundred characters.
const (
	// MaxSkillsInPrompt is how many skills the list may name.
	MaxSkillsInPrompt = 20
	// MaxSkillLineRunes is how wide one line of the list may be, name and all.
	MaxSkillLineRunes = 200
)

// skillsHeading is the line over the list, which tells the model what the lines
// are and how to load one. It is the harness's own words, sent on every call to
// a home that has a skill, so its length is counted against the instruction
// budget beside InstructionText.
const skillsHeading = "**Your skills.** Load one by name with the `skill` tool:"

// skillsText writes the skill list as one short block: the heading, then one
// line per skill in the form "name: description". An empty list writes nothing,
// so a home with no skills pays nothing for it.
func skillsText(skills []contract.SkillSummary) string {
	if len(skills) == 0 {
		return ""
	}
	if len(skills) > MaxSkillsInPrompt {
		skills = skills[:MaxSkillsInPrompt]
	}
	lines := []string{skillsHeading}
	for _, skill := range skills {
		lines = append(lines, cutToRunes(skill.Name+": "+skill.Description, MaxSkillLineRunes))
	}
	return strings.Join(lines, "\n")
}

// cutToRunes keeps a line inside its width, counted in characters rather than
// bytes so that a description in any language is cut between letters.
func cutToRunes(line string, width int) string {
	runes := []rune(line)
	if len(runes) <= width {
		return line
	}
	return string(runes[:width])
}
