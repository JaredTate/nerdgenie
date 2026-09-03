package skill

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// The bounds on the website a permissions block may name. They are the bounds
// the domain name system itself keeps, because a site line names one host and
// nothing longer than a host name can be one.
const (
	// MaxSiteBytes is how long the host name on a site line may be.
	MaxSiteBytes = 253
	// MaxSiteLabelBytes is how long one part of that host name, between two
	// dots, may be.
	MaxSiteLabelBytes = 63
)

// checkSite says whether a site line names one bare host name, and returns the
// clause saying what is wrong with it when it does not.
//
// A site is one website: letters, digits, dots and hyphens, and nothing else.
// No star, no question mark, no scheme, no path, no port, no space. The rule is
// this strict because the site becomes a standing approval in the permission
// function, where a star stands for any run of characters: a site line of "*"
// would become an approval matching the readable form of every call the agent
// ever makes, which is a permission slip for everything.
func checkSite(site string) error {
	if site == "" {
		return fmt.Errorf("it says nothing, so write the host name of the website the skill visits, such as news.example.com")
	}
	if len(site) > MaxSiteBytes {
		return fmt.Errorf("it is %d bytes long and a host name is at most %d, so write the host on its own", len(site), MaxSiteBytes)
	}
	for _, label := range strings.Split(site, ".") {
		if err := checkSiteLabel(label); err != nil {
			return err
		}
	}
	return nil
}

// checkSiteLabel says whether one part of a host name, the text between two
// dots, is a part a host name really has.
func checkSiteLabel(label string) error {
	if label == "" {
		return fmt.Errorf("it has nothing between two of its dots, so write the host as its parts with one dot between them")
	}
	if len(label) > MaxSiteLabelBytes {
		return fmt.Errorf("the part %q is %d bytes long and a part of a host name is at most %d, so shorten it", label, len(label), MaxSiteLabelBytes)
	}
	if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
		return fmt.Errorf("the part %q begins or ends with a hyphen and no part of a host name does, so take the hyphen off", label)
	}
	for _, letter := range label {
		if !isHostCharacter(letter) {
			return fmt.Errorf("it holds %q, and a host name holds only letters, digits, dots and hyphens, so write the host on its own", string(letter))
		}
	}
	return nil
}

// webSchemes are the two ways a web address begins, both written out in full
// wherever a standing approval is built. A star in front of the host would let
// any text at all sit between the tool's name and the host, and a shell command
// can carry a website's name inside it.
var webSchemes = []string{"https://", "http://"}

// toolsThatVisitAWebsite are the tools whose call carries a web address. They
// are the only tools a permissions block can approve, because a block names
// websites: it may approve a visit to a website and nothing else, never a shell
// command, never a file change, never money spent somewhere else.
func toolsThatVisitAWebsite() []string {
	return []string{contract.ToolWeb, contract.ToolBrowserOpen, contract.ToolBrowserRead, contract.ToolBrowserLogin}
}

// visitsAWebsite says whether a call to this tool carries a web address, which
// is what makes it a call a permissions block has anything to say about.
func visitsAWebsite(tool string) bool {
	return slices.Contains(toolsThatVisitAWebsite(), tool)
}

// approvalFormsFor returns the readable forms one website's standing approvals
// cover: one for every tool given, every scheme, and both shapes a web address
// takes, which are the host on its own and the host with a path after it.
//
// The tool's name and the scheme are written out in full, so nothing before the
// host is left free and no call to somewhere else can slip its own address in
// front. The host is followed by the end of the form or by the slash that
// starts a path, so news.example.com covers news.example.com/today and covers
// neither notnews.example.com nor news.example.com.evil.net. The host itself
// carries no star and no question mark, because checkSite refused any site line
// that held one, and the permission function reads everything else in a pattern
// as the characters it is.
func approvalFormsFor(site string, tools []string) []string {
	forms := make([]string, 0, len(tools)*len(webSchemes)*2)
	for _, tool := range tools {
		for _, scheme := range webSchemes {
			forms = append(forms, tool+" "+scheme+site, tool+" "+scheme+site+"/*")
		}
	}
	return forms
}

// isHostCharacter says whether one character may stand in a host name. The
// letters are the plain ones a name server carries, so a name written in
// another alphabet is refused rather than half understood.
func isHostCharacter(letter rune) bool {
	switch {
	case letter >= 'a' && letter <= 'z':
		return true
	case letter >= 'A' && letter <= 'Z':
		return true
	case letter >= '0' && letter <= '9':
		return true
	default:
		return letter == '-'
	}
}
