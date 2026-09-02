// Package repair finds the tool calls in a model reply, however the model wrote
// them.
//
// A model with a tool interface hands the harness a list of calls the provider
// already parsed. A model reached through a command-line program, and any small
// model on a local machine, has no such interface: it writes its calls as text,
// and it writes them in whatever shape it happens to have learned. A model that
// reasons aloud also writes calls it has not decided to make, inside the tags it
// thinks in, and nothing in there is ever read as a call or handed back as the
// answer. This package reads all of those shapes and hands the loop one answer:
// the calls, the text that is left once the thinking and the envelopes are taken
// out, and, when something looked like a call and could not be read, a message
// for the model that names the real tools. It never panics and it never returns
// both a problem and calls, because design section 3, rule 5 says the agent loop
// never crashes because of something the model wrote.
package repair
