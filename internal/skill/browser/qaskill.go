package browser

import (
	"context"
	"errors"

	"github.com/JaredTate/nerdgenie/internal/contract"
	"github.com/JaredTate/nerdgenie/internal/skill"
)

// QASkillName is the folder name of the skill shipped with the program for
// checking an app on screen.
const QASkillName = "qa"

// qaDescription is the one line about the quality skill that rides in the
// model's prompt.
const qaDescription = "Walks an app step by step, photographs each step, and reports whether every expected state is what it sees."

// theFilingStep is the number of the shipped walk's last step, which files a
// report and therefore cannot be undone. A visual check stops before it and the
// dry run stops before it too.
const theFilingStep = 4

// ShippedQASkill is the quality skill exactly as it is shipped in the skills
// folder of this repository. The folder on disk is what a person reads and edits
// for their own app; this is what the program installs, and a test in this
// package fails if the two ever say different things.
func ShippedQASkill() map[string][]byte {
	return map[string][]byte{
		skill.DescriptionFile: skill.RenderDescriptionFile(skill.Definition{
			Name:        QASkillName,
			Description: qaDescription,
			Triggers:    []string{"visual check", "check the app"},
			Permissions: skill.Permissions{
				DailyLimit:        skill.DefaultDailyLimit,
				IrreversibleSteps: []int{theFilingStep},
			},
		}),
		skill.StepsFile:     RenderSteps(qaWalk()),
		skill.TestFile:      skill.RenderTestFile(QASkillName, skill.DryRunPlan{}),
		skill.ChangelogFile: []byte(qaChangelog),
	}
}

// qaWalk is the walk the skill ships with. It is an example rather than a rule:
// it fills in the report form of the app under test and files it, which is the
// shape most checks take, and the steps are meant to be edited for the app being
// checked. The elements carry no reference, because a reference belongs to one
// reading of one page, so every step here is found by its role and name.
func qaWalk() []Step {
	return []Step{
		{
			Number: 1, Intent: "Open the app.", Tool: contract.ToolBrowserOpen,
			Address:     "{{arguments}}",
			Expectation: "the report form is on the screen",
		},
		{
			Number: 2, Intent: "Say who is reporting.", Tool: contract.ToolBrowserType, Typed: "Nerd Genie",
			Element:     Descriptor{Role: "textbox", Name: "Your name"},
			Expectation: "the name box holds what was typed",
		},
		{
			Number: 3, Intent: "Say what happened.", Tool: contract.ToolBrowserType,
			Typed:       "The visual check walked the app and photographed every step.",
			Element:     Descriptor{Role: "textbox", Name: "What happened"},
			Expectation: "the box about what happened holds the words",
		},
		{
			Number: theFilingStep, Intent: "File the report.", Tool: contract.ToolBrowserClick,
			Element:     Descriptor{Role: "button", Name: "File the report"},
			Expectation: "the page says the report was filed",
		},
	}
}

// qaChangelog is the first line of the shipped skill's changelog. Every skill
// folder carries one, and this one says where the skill came from and how to be
// rid of it.
const qaChangelog = `# changelog for qa

- shipped with Nerd Genie. The walk below the heading in steps.md is an example: edit it for the app you are checking, and give the address of that app as the skill's argument. To undo: /skills remove
`

// InstallQASkill writes the shipped quality skill into the skills folder. The
// store keeps the copy it replaces, so installing it over a walk somebody edited
// leaves that walk behind in the skill's versions folder rather than throwing it
// away.
func InstallQASkill(ctx context.Context, saver SkillSaver) error {
	if saver == nil {
		return errors.New("the quality skill has no skill store to install into, so pass the store to save through")
	}
	return saver.Save(ctx, contract.SkillSavedByPerson, QASkillName, ShippedQASkill())
}
