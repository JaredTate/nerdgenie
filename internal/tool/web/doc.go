// Package web is the web tool: it searches the web, and it fetches a public
// page as text.
//
// Two things make it more than a wrapper round an HTTP request. The first is
// where it is allowed to go. A name is resolved once and the connection is made
// to that exact number, so that a name which answers differently the second time
// cannot send the agent somewhere else; every number in the private ranges, on
// the machine itself, or in the ranges a cloud machine keeps its own credentials
// behind is refused, and so is a redirect that leads to one. The second is what
// it hands back. Everything from outside comes back inside a wrapper with a
// random boundary on it, saying in plain words that the text is data and not
// instructions, so that a page telling the agent to send a password reads as a
// page that says so and nothing more. Search works with no key and no server:
// with a SearXNG address in the configuration it asks that, and with none it
// reads the DuckDuckGo results page as text.
package web
