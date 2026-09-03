// Package web is the web tool: it searches the web, and it fetches a public
// page as text.
//
// Two things make it more than a wrapper round an HTTP request. The first is
// where it is allowed to go. A name is resolved once and the connection is made
// to that exact number, so that a name which answers differently the second time
// cannot send the agent somewhere else; every number in one table of ranges that
// are not the public web is refused, from the private ranges and this machine to
// the shared address space a tailnet lives on, the ranges kept aside for
// documentation, and the prefixes that carry an IPv4 address inside an IPv6 one;
// a host written in octal, in hexadecimal, or as one long integer is refused
// before it is looked up, because the machine's own name lookup reads those as
// addresses; and every hop of a redirect goes through the same check with the
// page that sent the agent there named in the refusal. A host the settings allow
// is a host and a port together, and only a host written as a number is reached
// on the strength of the settings alone, because whoever answers for a name is
// the one choosing the number. The second is what it hands back. Everything from outside comes back inside a wrapper with a
// random boundary on it, saying in plain words that the text is data and not
// instructions, so that a page telling the agent to send a password reads as a
// page that says so and nothing more. Search works with no key and no server:
// with a SearXNG address in the configuration it asks that, and with none it
// reads the DuckDuckGo results page as text.
package web
