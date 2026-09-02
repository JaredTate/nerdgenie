package permission

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// MaxStandingApprovals is how many standing approvals one session holds. A skill
// registers one or two, so a session that reaches this number has a skill in a
// loop and the cap is what stops it.
const MaxStandingApprovals = 100

// StandingApproval is a skill's standing permission for one kind of call: the
// readable form it covers, how many times it may be used, and when it runs out.
// A call it covers is allowed until the limit or the expiry, and after either
// one the call asks again. The loop registers these when it runs a skill the
// user has already approved.
type StandingApproval struct {
	// Skill is the skill that holds the approval, and is what the user is told
	// when the approval allows a call.
	Skill string
	// ReducedForm is the readable form the approval covers. A form with no star
	// in it covers exactly that one call, and a star stands for any run of
	// characters, the same way a rule's pattern does.
	ReducedForm string
	// Limit is how many calls the approval may allow.
	Limit int
	// Expires is when it runs out, and it is never empty.
	Expires time.Time
}

// standingApproval is one registered approval with its pattern already compiled
// and its uses counted.
type standingApproval struct {
	approval StandingApproval
	pattern  *regexp.Regexp
	used     int
}

// RegisterStandingApproval gives the permission function one standing approval,
// and returns an error naming what is missing when it cannot be used.
func (decider *Decider) RegisterStandingApproval(approval StandingApproval) error {
	compiled, err := compileStandingApproval(approval)
	if err != nil {
		return err
	}

	decider.guard.Lock()
	defer decider.guard.Unlock()
	if len(decider.standing) >= MaxStandingApprovals {
		return fmt.Errorf("this session already holds %d standing approvals, which is all it keeps, so the skill %q cannot add another", MaxStandingApprovals, approval.Skill)
	}
	decider.standing = append(decider.standing, compiled)
	return nil
}

// compileStandingApproval checks one standing approval and compiles the readable
// form it covers.
func compileStandingApproval(approval StandingApproval) (*standingApproval, error) {
	if strings.TrimSpace(approval.Skill) == "" {
		return nil, fmt.Errorf("a standing approval for %q names no skill, so say which skill holds it", approval.ReducedForm)
	}
	if strings.TrimSpace(approval.ReducedForm) == "" {
		return nil, fmt.Errorf("the standing approval held by the skill %q names no readable form, so say which calls it covers", approval.Skill)
	}
	if approval.Limit <= 0 {
		return nil, fmt.Errorf("the standing approval held by the skill %q may be used %d times, so give it a limit above zero", approval.Skill, approval.Limit)
	}
	if approval.Expires.IsZero() {
		return nil, fmt.Errorf("the standing approval held by the skill %q never expires, so give it a time to run out at", approval.Skill)
	}

	pattern, err := compilePattern(approval.ReducedForm)
	if err != nil {
		return nil, err
	}
	return &standingApproval{approval: approval, pattern: pattern}, nil
}

// useStandingApproval allows the call when a skill holds an unexpired approval
// for it that is still inside its limit, and counts the use. It says why in the
// words the user will read.
func (decider *Decider) useStandingApproval(reduced string) (string, bool) {
	now := decider.clock.Now()

	decider.guard.Lock()
	defer decider.guard.Unlock()
	for _, held := range decider.standing {
		if held.used >= held.approval.Limit || !now.Before(held.approval.Expires) {
			continue
		}
		if !held.pattern.MatchString(reduced) {
			continue
		}
		held.used++
		return fmt.Sprintf("the skill %q holds a standing approval for %q, and this is use %d of %d", held.approval.Skill, held.approval.ReducedForm, held.used, held.approval.Limit), true
	}
	return "", false
}
