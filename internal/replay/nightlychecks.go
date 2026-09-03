package replay

import (
	"context"
	"fmt"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// theMemoryItself is the name the one check about an empty memory carries, so
// that the line the user is sent says which part of the agent was looked at.
const theMemoryItself = "the memory"

// askTheMemory is the twenty-question test, asked of the memory the agent
// actually has.
//
// The test that first wrote these twenty questions lives in internal/memory's
// own testdata, where only "go test" can reach it, and a fixture of somebody
// else's facts would have to be written into the user's memory to be asked
// about, which is not a thing a self-check may do. So the questions are made
// from the twenty newest facts: each fact is asked about in its own words, and
// the fact has to come back in the first three answers, which is as far down as
// the memory hint ever reaches. A memory whose index has quietly stopped
// working fails every one of them.
func (nightly *Nightly) askTheMemory(ctx context.Context) []Check {
	if nightly.settings.Memory == nil {
		return nil
	}
	newest, err := nightly.settings.Memory.Search(ctx, "", TwentyQuestions)
	if err != nil {
		return []Check{{Name: theMemoryItself, Detail: "the memory could not be searched at all: " + err.Error()}}
	}
	if len(newest) == 0 {
		return []Check{{Name: theMemoryItself, Passed: true, Detail: "the memory holds nothing to ask about yet"}}
	}
	checks := make([]Check, 0, len(newest))
	for _, fact := range newest {
		checks = append(checks, nightly.askAbout(ctx, fact))
	}
	return checks
}

// askAbout asks the memory one question and says whether the fact that answers
// it came back near the top.
func (nightly *Nightly) askAbout(ctx context.Context, fact contract.Fact) Check {
	question := questionAbout(fact)
	if question == "" {
		return Check{Name: fact.ID, Passed: true}
	}
	found, err := nightly.settings.Memory.Search(ctx, question, AnswersLookedAt)
	if err != nil {
		return Check{Name: fact.ID, Detail: "the memory could not be searched: " + err.Error()}
	}
	for _, answer := range found {
		if answer.ID == fact.ID {
			return Check{Name: fact.ID, Passed: true}
		}
	}
	return Check{
		Name: fact.ID,
		Detail: fmt.Sprintf("asking %q brought back %s, and %s was not among the first %d",
			question, idsOf(found), fact.ID, AnswersLookedAt),
	}
}

// questionAbout is the question one fact is asked with: its own words, without
// the mark memory puts on a fact something later replaced, and cut to a length
// a search can work with.
func questionAbout(fact contract.Fact) string {
	text := strings.TrimSpace(fact.Text)
	if _, after, found := strings.Cut(text, ") "); found && strings.HasPrefix(text, "(superseded by ") {
		text = strings.TrimSpace(after)
	}
	letters := []rune(text)
	if len(letters) > MaxQuestionRunes {
		text = string(letters[:MaxQuestionRunes])
	}
	return text
}

// idsOf names the facts a question brought back, which is what a failed
// question shows the reader.
func idsOf(facts []contract.Fact) string {
	if len(facts) == 0 {
		return "nothing"
	}
	ids := make([]string, 0, len(facts))
	for _, fact := range facts {
		ids = append(ids, fact.ID)
	}
	return strings.Join(ids, ", ")
}

// dryRunTheSkills runs every skill's dry run, which is the test each skill
// carries: it replays the skill up to the first step that cannot be undone and
// stops there, so nothing a skill does is done again in the night.
func (nightly *Nightly) dryRunTheSkills(ctx context.Context) []Check {
	if nightly.settings.Skills == nil || nightly.settings.DryRun == nil {
		return nil
	}
	listed, err := nightly.settings.Skills.List(ctx)
	if err != nil {
		return []Check{{Name: "the skills", Detail: "the skills could not be listed at all: " + err.Error()}}
	}
	checks := make([]Check, 0, len(listed))
	for at, summary := range listed {
		if at >= MaxSkillsChecked {
			break
		}
		checks = append(checks, nightly.dryRunOne(ctx, summary.Name))
	}
	return checks
}

// dryRunOne runs one skill's dry run and says whether it reached the end.
func (nightly *Nightly) dryRunOne(ctx context.Context, name string) Check {
	if _, err := nightly.settings.DryRun(ctx, name); err != nil {
		return Check{Name: "the skill " + name, Detail: err.Error()}
	}
	return Check{Name: "the skill " + name, Passed: true}
}
