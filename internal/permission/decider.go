// The three answers a user may give, and the rule that the last matching rule
// wins, are OpenCode's permission design, at
// ~/Code/opencode/packages/opencode/src/permission/index.ts. Coeus keeps the
// answers in memory for one session only, because nothing durable belongs in a
// decision the user can take back by restarting.

package permission

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxRememberedAnswers is how many answers one session keeps. An answer is
// remembered for one readable form, so a session that reaches this number has
// been asked a thousand different questions and something is wrong.
const MaxRememberedAnswers = 1000

// rememberedAnswer is what one answer from the user turned into: a ruling to
// give the next call that reduces to the same thing, and the words to give with
// it.
type rememberedAnswer struct {
	ruling contract.PermissionRuling
	reason string
}

// Decider is the permission function: the harness's rulebook for what a tool
// call is allowed to do.
type Decider struct {
	clock contract.Clock
	book  *Rulebook

	guard      sync.Mutex
	remembered map[string]rememberedAnswer
	standing   []*standingApproval
	// yolo says every call that would have asked runs without asking, for this
	// session only. UseYolo sets it and YoloIsOn reads it.
	yolo bool
}

// New builds a permission function from ~/.coeus/config.toml and the clock a
// standing approval's expiry is read against. The shipped entries the user kept
// in "ask_me_first" are read first and the rules in "permission_rules" after
// them, so the last rule that matches is the user's own when the user wrote one.
// It returns an error naming whatever in the configuration cannot be used, which
// includes an entry nobody shipped.
func New(configuration contract.Config, clock contract.Clock) (*Decider, error) {
	if clock == nil {
		return nil, errors.New("the permission function was built without a clock, so pass one, because a standing approval expires at a time")
	}

	rules, err := RulesForAskMeFirst(configuration.AskMeFirst)
	if err != nil {
		return nil, err
	}
	book, err := NewRulebook(append(rules, rulesFromConfiguration(configuration.PermissionRules)...))
	if err != nil {
		return nil, err
	}
	return &Decider{clock: clock, book: book, remembered: map[string]rememberedAnswer{}}, nil
}

// rulesFromConfiguration turns the rules the user wrote in config.toml into the
// rules the rulebook matches with. A rule from a file has no reason of its own,
// so it is described by itself when it wins.
func rulesFromConfiguration(written []contract.PermissionRule) []Rule {
	rules := make([]Rule, 0, len(written))
	for _, rule := range written {
		rules = append(rules, Rule{Tool: rule.Tool, Pattern: rule.Pattern, Action: rule.Action})
	}
	return rules
}

// Decide rules on one call: allow it, ask the user about it, or refuse it. A
// rule that refuses is read first, because nothing outranks it. The answer the
// user gave earlier in this session is read next, because the user's word for
// the session outranks a rule that would have let the call run without asking.
// After those, a call no rule covers is allowed, because the agent runs on its
// own by default, unless its readable form is not the whole story, and then it
// is put to the user, because a form that leaves something out cannot be ruled
// on — save for a shell command that only reads, which runs without a preview
// even then, because a command that only reads never needed the yes. The rules
// are matched against the readable form and, for a shell command,
// against the same form with its flags spelled out, so that a rule about a flag
// holds however the flag was written.
func (decider *Decider) Decide(ctx context.Context, request contract.PermissionRequest) (contract.PermissionDecision, error) {
	if err := ctx.Err(); err != nil {
		return contract.PermissionDecision{}, fmt.Errorf("the turn was stopped before the %s call could be ruled on: %w", request.ToolName, err)
	}

	reduced, note := reduceCall(request)
	matched, covered := decider.book.Match(request.ToolName, formsToMatchOn(request.ToolName, reduced)...)
	if covered && matched.Action == contract.RulingDeny {
		return contract.PermissionDecision{Ruling: matched.Action, Reason: reasonOf(matched, reduced)}, nil
	}
	if answered, alreadyAnswered := decider.answerAlreadyGiven(request.ToolName, reduced); alreadyAnswered {
		return contract.PermissionDecision{Ruling: answered.ruling, Reason: answered.reason}, nil
	}

	switch {
	case covered && matched.Action == contract.RulingAllow:
		return contract.PermissionDecision{Ruling: matched.Action, Reason: reasonOf(matched, reduced)}, nil
	case covered:
		return decider.ruleOnSomethingToAskAbout(request, reduced, reasonOf(matched, reduced), matched.FromTheAskMeFirstList), nil
	case note != "":
		if commandOnlyReads(request) {
			return contract.PermissionDecision{
				Ruling: contract.RulingAllow,
				Reason: fmt.Sprintf("%q only reads, so it runs even though its readable form does not say everything it does", reduced),
			}, nil
		}
		return decider.ruleOnSomethingToAskAbout(request, reduced, whyTheFormLeavesSomethingOut(note, reduced), false), nil
	default:
		return contract.PermissionDecision{
			Ruling: contract.RulingAllow,
			Reason: fmt.Sprintf("no rule on your ask-me-first list covers %q, so it runs", reduced),
		}, nil
	}
}

// formsToMatchOn returns the forms the rules are matched against: the readable
// form the user sees and, for a shell command, the same form with its flags
// spelled out, so that a rule about a flag holds however the flag was written.
// Nothing but a shell command has flags, so nothing else has a second form.
func formsToMatchOn(toolName string, reduced string) []string {
	if toolName != contract.ToolShell {
		return []string{reduced}
	}
	return []string{reduced, flagsSpelledOut(reduced)}
}

// answerAlreadyGiven returns the answer the user gave earlier in this session
// about a call that reduces to the same thing, and says whether there was one.
func (decider *Decider) answerAlreadyGiven(toolName string, reduced string) (rememberedAnswer, bool) {
	decider.guard.Lock()
	defer decider.guard.Unlock()
	answered, alreadyAnswered := decider.remembered[rememberedKey(toolName, reduced)]
	return answered, alreadyAnswered
}

// whyTheFormLeavesSomethingOut says, in the words the user and the model both
// read, why a call whose readable form is not the whole story needs a yes.
func whyTheFormLeavesSomethingOut(note string, reduced string) string {
	if note == buildsItselfNote {
		return fmt.Sprintf("the command %q works out part of itself while it runs, so nothing here can say what it will really do", reduced)
	}
	if note == unclosedQuoteNote {
		return fmt.Sprintf("the command %q opens a quote and never closes it, so nothing here can say where one word ends and the next begins", reduced)
	}
	return fmt.Sprintf("the call %q was too long to read to the end, so nothing here can say what the rest of it does", reduced)
}

// ruleOnSomethingToAskAbout takes a call that needs a yes, which the user has
// not already answered about in this session. While yolo is on, the yes has
// been given in advance for everything, so the call runs and the ruling says
// so. Otherwise a run with nobody there to answer stops before anything else is
// read, because a call that needs a yes and can be given none does not run
// whoever holds an approval for it. A call the user's ask-me-first list caught
// goes straight to the user, because that list is what the user asked to see
// first and a skill's standing approval is not the user's word. Only what is
// left is offered to the standing approvals.
func (decider *Decider) ruleOnSomethingToAskAbout(request contract.PermissionRequest, reduced string, why string, onTheAskMeFirstList bool) contract.PermissionDecision {
	if decider.YoloIsOn() {
		return allowedByYolo(request, reduced, why)
	}
	if request.Unattended {
		return contract.PermissionDecision{
			Ruling:      contract.RulingStop,
			Reason:      fmt.Sprintf("nobody is there to answer, and %q needs a yes first: %s. The task stops and reports instead of waiting.", reduced, why),
			PreviewText: previewOf(request, reduced),
		}
	}
	if !onTheAskMeFirstList {
		if approval, standing := decider.useStandingApproval(request, reduced); standing {
			return contract.PermissionDecision{Ruling: contract.RulingAllow, Reason: approval}
		}
	}
	return contract.PermissionDecision{
		Ruling:      contract.RulingAsk,
		Reason:      why,
		PreviewText: previewOf(request, reduced),
	}
}

// Remember records the user's answer for the rest of the session. An answer of
// once records nothing, because once means this call and nothing more.
func (decider *Decider) Remember(request contract.PermissionRequest, answer contract.PreviewAnswer, reason string) error {
	if !contract.KnownPreviewAnswer(answer) {
		return fmt.Errorf("the answer %q is not one the harness understands, so use once, always, or reject", answer)
	}
	if answer == contract.AnswerOnce {
		return nil
	}

	reduced := Reduce(request)
	key := rememberedKey(request.ToolName, reduced)

	decider.guard.Lock()
	defer decider.guard.Unlock()
	_, alreadyAnswered := decider.remembered[key]
	if !alreadyAnswered && len(decider.remembered) >= MaxRememberedAnswers {
		return fmt.Errorf("this session already holds %d remembered answers, which is all it keeps, so start a new session to answer any more", MaxRememberedAnswers)
	}
	decider.remembered[key] = rulingForAnswer(answer, reduced, reason)
	return nil
}

// rulingForAnswer turns the user's answer into the ruling every later call with
// the same readable form will get, and the words to give with it.
func rulingForAnswer(answer contract.PreviewAnswer, reduced string, reason string) rememberedAnswer {
	if answer == contract.AnswerAlways {
		return rememberedAnswer{
			ruling: contract.RulingAllow,
			reason: fmt.Sprintf("you allowed %q for the rest of this session", reduced),
		}
	}
	if strings.TrimSpace(reason) == "" {
		return rememberedAnswer{
			ruling: contract.RulingDeny,
			reason: fmt.Sprintf("you refused %q earlier in this session", reduced),
		}
	}
	return rememberedAnswer{
		ruling: contract.RulingDeny,
		reason: fmt.Sprintf("you refused %q earlier in this session, saying: %s", reduced, strings.TrimSpace(reason)),
	}
}

// rememberedKey is what an answer is filed under: the tool and the readable
// form, kept apart by a character no readable form can hold.
func rememberedKey(toolName string, reduced string) string {
	return toolName + "\x00" + reduced
}

// reasonOf is the rule's own words when it has any, and a sentence built from
// the rule itself when it has none, because a ruling with no reason tells the
// user and the model nothing.
func reasonOf(rule Rule, reduced string) string {
	if strings.TrimSpace(rule.Reason) != "" {
		return rule.Reason
	}
	return fmt.Sprintf("your rule %q on the tool %q says to %s %q", rule.Pattern, rule.Tool, rule.Action, reduced)
}
