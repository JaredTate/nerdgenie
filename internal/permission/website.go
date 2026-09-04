// Which call is a visit to which website. A skill's permissions block names the
// sites it may visit, and the harness has to answer the same question of every
// call the skill makes: is this that site? A pattern over the readable form
// cannot answer it, because "example.com" is a run of characters that appears in
// api.example.com, in example.com.evil.net, and in a link written inside
// somewhere else's address. The host of the address is a comparison, and a
// comparison has no such holes.

package permission

import (
	"net/url"
	"strings"

	"github.com/JaredTate/nerdgenie/internal/contract"
)

// MaxHostRunes is how long a host name in a standing approval may be. A real
// host name is under this, and the cap stops a permissions block from carrying a
// page of text where a site belongs.
const MaxHostRunes = 253

// addressFieldsByTool names, for each tool that visits a website, the input
// field that carries the address, first one present. Every other tool has no
// address in its call, so no approval for a website can ever cover it.
var addressFieldsByTool = map[string][]string{
	contract.ToolWeb:          {"url"},
	contract.ToolBrowserOpen:  {"url"},
	contract.ToolBrowserRead:  {"url"},
	contract.ToolBrowserLogin: {"site", "url"},
}

// ToolsThatVisitAWebsite returns the tools whose call names the website it goes
// to, which are the only tools an approval for a website may cover. A skill's
// permissions block turns into one approval per site over these.
func ToolsThatVisitAWebsite() []string {
	return []string{contract.ToolWeb, contract.ToolBrowserOpen, contract.ToolBrowserRead, contract.ToolBrowserLogin}
}

// theSiteThisCallVisits returns the host of the address one call names, with its
// port still on it when it was written with one, folded to lowercase. It is
// empty when the call names no address, and then no approval for a website
// covers the call.
func theSiteThisCallVisits(request contract.PermissionRequest) string {
	fields := readFields(request.Input)
	for _, name := range addressFieldsByTool[request.ToolName] {
		address, written := stringField(fields, name)
		if written && strings.TrimSpace(address) != "" {
			return hostOfAddress(address)
		}
	}
	return ""
}

// hostOfAddress returns the host an address names, the port kept, because a
// service on another port is another service. An address written as a bare host
// name, which is how a login names its site, is its own host.
func hostOfAddress(address string) string {
	tidied := strings.ToLower(strings.TrimSpace(address))
	if !strings.Contains(tidied, "//") {
		return tidied
	}
	parsed, err := url.Parse(tidied)
	if err != nil || parsed.Host == "" {
		return tidied
	}
	return parsed.Host
}

// checkHostName says what is wrong with the site a standing approval names, and
// nothing when it is a plain host name. A star, a port, a scheme, a path, or a
// space means the block wrote something that is not one website, and an approval
// that covers more than one website is the permission slip the wave 6 security
// review wrote itself.
func checkHostName(skill string, host string) error {
	if letters := []rune(host); len(letters) > MaxHostRunes {
		return errorAboutTheHost(skill, host, "is longer than a host name can be, so name the one site the skill visits")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return errorAboutTheHost(skill, host, "has an empty part between its dots, so write the site as a person would type it")
		}
		if !everyLetterBelongsInAHostName(label) {
			return errorAboutTheHost(skill, host, "is not a host name, so write one site, such as example.com, with no scheme, no port, no path, and no star")
		}
	}
	return nil
}

// everyLetterBelongsInAHostName says whether one part of a host name is written
// with the letters, digits and hyphens a host name is made of.
func everyLetterBelongsInAHostName(label string) bool {
	for _, letter := range label {
		fromTheAlphabet := (letter >= 'a' && letter <= 'z') || (letter >= 'A' && letter <= 'Z')
		if !fromTheAlphabet && !(letter >= '0' && letter <= '9') && letter != '-' {
			return false
		}
	}
	return true
}
