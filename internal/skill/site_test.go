package skill_test

import (
	"strings"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/skill"
)

// skillNamingSite is a SKILL.md whose permissions block names one website, so
// that a test can see what the reader makes of the site line and nothing else.
func skillNamingSite(site string) []byte {
	return []byte("# tidy-up\n\nTidies the work folder every morning.\n\n## Permissions\n\n- site: " + site + "\n")
}

func TestASiteHasToBeABareHostName(t *testing.T) {
	refused := []struct {
		what string
		site string
	}{
		{"a star, which stands for every call there is", "*"},
		{"a question mark", "news.example.?om"},
		{"a whole address rather than a host", "https://news.example.com"},
		{"a path after the host", "news.example.com/prices"},
		{"two hosts with a space between them", "news.example.com evil.example.net"},
		{"a port after the host", "news.example.com:8443"},
		{"a login name in front of the host", "reader@news.example.com"},
		{"nothing between two of its dots", "news..example.com"},
		{"a part that ends in a hyphen", "news-.example.com"},
		{"a character no host name holds", "news_example.com"},
		{"a part longer than a part of a host name may be", strings.Repeat("a", 64) + ".example.com"},
		{"a name longer than the domain name system carries", strings.Repeat("a.", 130) + "com"},
	}
	for _, one := range refused {
		t.Run(one.what, func(t *testing.T) {
			_, err := skill.ParseDescriptionFile(skillNamingSite(one.site))
			if err == nil {
				t.Fatalf("the site %q was read as a website; a site becomes a standing approval in the permission function,"+
					" so anything that is not one bare host name has to be refused when the skill is parsed", one.site)
			}
			if !strings.Contains(err.Error(), "site") || !strings.Contains(err.Error(), one.site) {
				t.Errorf("the message is %q, and it has to name the site line and what it says", err)
			}
		})
	}
}

func TestAHostNameIsReadAsTheWebsiteItNames(t *testing.T) {
	accepted := []struct {
		site string
		want string
	}{
		{"news.example.com", "news.example.com"},
		{"NEWS.Example.com", "news.example.com"},
		{"localhost", "localhost"},
		{"a-b.example.co.uk", "a-b.example.co.uk"},
		{"10.0.0.1", "10.0.0.1"},
	}
	for _, one := range accepted {
		t.Run(one.site, func(t *testing.T) {
			definition, err := skill.ParseDescriptionFile(skillNamingSite(one.site))
			if err != nil {
				t.Fatalf("the host name %q was refused: %v", one.site, err)
			}
			if len(definition.Permissions.Sites) != 1 || definition.Permissions.Sites[0] != one.want {
				t.Errorf("the block names the websites %v, want the one host name %q", definition.Permissions.Sites, one.want)
			}
		})
	}
}
