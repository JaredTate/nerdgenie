package permission

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxStandingApprovals is how many standing approvals one session holds. A skill
// registers one for each website its permissions block names, so this is a cap
// on sites, and a session that reaches it has a skill in a loop.
const MaxStandingApprovals = 100

// StandingApproval is a skill's standing permission for one kind of call: what
// it covers, how many times it may be used, and when it runs out. A call it
// covers is allowed until the limit or the expiry, and after either one the call
// asks again. The loop registers these when it runs a skill the user has already
// approved.
//
// What it covers is written one of two ways, and never both. An approval for a
// **website** names the host and the tools that visit a website, and it covers a
// call to one of those tools whose address is that host: a comparison, so a
// subdomain, another port, and a name that merely begins with the site are all
// somewhere else and go to the person. An approval for anything **else** names a
// readable form, which is matched as a pattern.
type StandingApproval struct {
	// Skill is the skill that holds the approval, and is what the user is told
	// when the approval allows a call.
	Skill string
	// Host is the one website the approval covers, written as a person types
	// it: no scheme, no port, no path, and no star. It is set with Tools.
	Host string
	// Tools are the tools the approval covers, and every one of them has to be
	// a tool that visits a website, so that an approval for a site can never
	// cover a command run on this machine.
	Tools []string
	// ReducedForm is the readable form an approval that is not about a website
	// covers. A form with no star in it covers exactly that one call, and a star
	// stands for any run of characters, the same way a rule's pattern does.
	ReducedForm string
	// Limit is how many calls the approval may allow.
	Limit int
	// Expires is when it runs out, and it is never empty.
	Expires time.Time
}

// standingApproval is one registered approval with its pattern already compiled,
// where it has one, and its uses counted.
type standingApproval struct {
	approval StandingApproval
	pattern  *regexp.Regexp
	used     int
}

// covers says whether this approval covers one call: the site the call visits
// for an approval that names a website, and the readable form for one that names
// a pattern.
func (held *standingApproval) covers(request contract.PermissionRequest, reduced string) bool {
	if held.approval.Host == "" {
		return held.pattern.MatchString(reduced)
	}
	if !slices.Contains(held.approval.Tools, request.ToolName) {
		return false
	}
	return theSiteThisCallVisits(request) == held.approval.Host
}

// what says what the approval covers, in the words the user reads when it allows
// a call.
func (held *standingApproval) what() string {
	if held.approval.Host != "" {
		return "the website " + held.approval.Host
	}
	return held.approval.ReducedForm
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

// compileStandingApproval checks one standing approval, and compiles the
// readable form when it covers one rather than a website.
func compileStandingApproval(approval StandingApproval) (*standingApproval, error) {
	if strings.TrimSpace(approval.Skill) == "" {
		return nil, fmt.Errorf("a standing approval for %q names no skill, so say which skill holds it", approval.ReducedForm+approval.Host)
	}
	if err := checkWhatTheApprovalCovers(approval); err != nil {
		return nil, err
	}
	if approval.Limit <= 0 {
		return nil, fmt.Errorf("the standing approval held by the skill %q may be used %d times, so give it a limit above zero", approval.Skill, approval.Limit)
	}
	if approval.Expires.IsZero() {
		return nil, fmt.Errorf("the standing approval held by the skill %q never expires, so give it a time to run out at", approval.Skill)
	}
	if approval.Host != "" {
		return &standingApproval{approval: withTheHostFolded(approval)}, nil
	}

	pattern, err := compilePattern(approval.ReducedForm)
	if err != nil {
		return nil, err
	}
	return &standingApproval{approval: approval, pattern: pattern}, nil
}

// checkWhatTheApprovalCovers says what is wrong with the calls a standing
// approval says it covers: neither a website nor a readable form, both of them
// at once, a site that is not one host name, or a website approval over a tool
// that does not visit a website.
func checkWhatTheApprovalCovers(approval StandingApproval) error {
	host := strings.TrimSpace(approval.Host)
	form := strings.TrimSpace(approval.ReducedForm)
	switch {
	case host == "" && form == "":
		return fmt.Errorf("the standing approval held by the skill %q names no website and no readable form, so say which calls it covers", approval.Skill)
	case host != "" && form != "":
		return fmt.Errorf("the standing approval held by the skill %q names both a website and a readable form, so give it one of the two and not both", approval.Skill)
	case host == "":
		return nil
	}

	if err := checkHostName(approval.Skill, host); err != nil {
		return err
	}
	if len(approval.Tools) == 0 {
		return fmt.Errorf("the standing approval held by the skill %q covers the website %q and names no tool, so name the tools that may visit it", approval.Skill, host)
	}
	for _, toolName := range approval.Tools {
		if _, visits := addressFieldsByTool[toolName]; !visits {
			return fmt.Errorf("the standing approval held by the skill %q covers the website %q and names the tool %q, which visits no website,"+
				" so name only the tools whose call says which site it goes to", approval.Skill, host, toolName)
		}
	}
	return nil
}

// withTheHostFolded returns the approval with its site written the way an
// address is read, so that the comparison of the two is a comparison of the
// same thing.
func withTheHostFolded(approval StandingApproval) StandingApproval {
	approval.Host = strings.ToLower(strings.TrimSpace(approval.Host))
	return approval
}

// errorAboutTheHost says which skill named a site the harness cannot use, and
// what is wrong with it.
func errorAboutTheHost(skill string, host string, what string) error {
	return fmt.Errorf("the standing approval held by the skill %q covers the website %q, which %s", skill, host, what)
}

// useStandingApproval allows the call when a skill holds an unexpired approval
// for it that is still inside its limit, and counts the use. It says why in the
// words the user will read.
func (decider *Decider) useStandingApproval(request contract.PermissionRequest, reduced string) (string, bool) {
	now := decider.clock.Now()

	decider.guard.Lock()
	defer decider.guard.Unlock()
	for _, held := range decider.standing {
		if held.used >= held.approval.Limit || !now.Before(held.approval.Expires) {
			continue
		}
		if !held.covers(request, reduced) {
			continue
		}
		held.used++
		return fmt.Sprintf("the skill %q holds a standing approval for %q, and this is use %d of %d", held.approval.Skill, held.what(), held.used, held.approval.Limit), true
	}
	return "", false
}
