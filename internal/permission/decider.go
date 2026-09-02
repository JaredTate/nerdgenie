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
// call no rule covers is allowed, because the agent runs on its own by default.
func (decider *Decider) Decide(ctx context.Context, request contract.PermissionRequest) (contract.PermissionDecision, error) {
	if err := ctx.Err(); err != nil {
		return contract.PermissionDecision{}, fmt.Errorf("the turn was stopped before the %s call could be ruled on: %w", request.ToolName, err)
	}

	reduced := Reduce(request)
	matched, covered := decider.book.Match(request.ToolName, reduced)
	if !covered {
		return contract.PermissionDecision{
			Ruling: contract.RulingAllow,
			Reason: fmt.Sprintf("no rule on your ask-me-first list covers %q, so it runs", reduced),
		}, nil
	}

	switch matched.Action {
	case contract.RulingAllow, contract.RulingDeny:
		return contract.PermissionDecision{Ruling: matched.Action, Reason: reasonOf(matched, reduced)}, nil
	default:
		return decider.ruleOnSomethingToAskAbout(request, matched, reduced), nil
	}
}

// ruleOnSomethingToAskAbout takes a call a rule says to ask about and sees
// whether the user has already answered a call like it in this session, and
// then whether a skill holds a standing approval for it.
func (decider *Decider) ruleOnSomethingToAskAbout(request contract.PermissionRequest, matched Rule, reduced string) contract.PermissionDecision {
	decider.guard.Lock()
	answered, alreadyAnswered := decider.remembered[rememberedKey(request.ToolName, reduced)]
	decider.guard.Unlock()

	if alreadyAnswered {
		return contract.PermissionDecision{Ruling: answered.ruling, Reason: answered.reason}
	}
	if why, standing := decider.useStandingApproval(reduced); standing {
		return contract.PermissionDecision{Ruling: contract.RulingAllow, Reason: why}
	}
	if request.Unattended {
		return contract.PermissionDecision{
			Ruling:      contract.RulingStop,
			Reason:      fmt.Sprintf("nobody is there to answer, and %q is on your ask-me-first list under %q, so the task stops and reports instead of waiting", reduced, reasonOf(matched, reduced)),
			PreviewText: previewOf(request, reduced),
		}
	}
	return contract.PermissionDecision{
		Ruling:      contract.RulingAsk,
		Reason:      reasonOf(matched, reduced),
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
