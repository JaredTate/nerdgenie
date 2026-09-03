package skill

import (
	"fmt"
	"slices"
	"strings"

	"github.com/JaredTate/coeus/internal/permission"
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

// visitsAWebsite says whether a call to this tool carries a web address, which
// is what makes it a call a permissions block has anything to say about. The
// list is the permission function's own, so that the block's gate and the
// standing approval it becomes are never about different tools.
func visitsAWebsite(tool string) bool {
	return slices.Contains(permission.ToolsThatVisitAWebsite(), tool)
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
