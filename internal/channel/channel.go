// The design in this file is borrowed from ZeroClaw's channel trait at
// ~/Code/zeroclaw/crates/zeroclaw-api/src/channel.rs and written fresh in Go.
// There every way of talking to the agent is one small trait: a name, a send, a
// listen that pushes what arrives into one shared sender, an approval prompt
// that comes back as one of a fixed set of answers, and a health check, so the
// runtime never learns which surface it is talking to. Coeus keeps that shape in
// contract.Channel and makes the local socket the first thing to implement it,
// so the terminal is a channel exactly as Signal is.

package channel

// TerminalChannelName is the name the local socket answers to. It is the name
// the terminal and every other screen attached to the socket send under, and the
// name a command that may only run in the terminal is checked against.
const TerminalChannelName = "terminal"
