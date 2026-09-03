// Package channel is the queue every channel feeds, the router that sorts what
// arrives, the event stream every channel subscribes to, and the local socket
// that is itself a channel.
//
// A channel is anything a user can talk through, and every one of them does the
// same six things: it receives a message, sends a reply, sends a file, shows a
// preview and collects an answer, asks for a secret without echoing it, and says
// whether it is working. This package owns the parts that sit behind all of
// them. Everything a channel receives goes into one queue in the single SQLite
// file before anything looks at it, so a message cannot be lost and a message
// the agent stopped part way through is handed out again with a marker saying
// so. The router then decides what each message is: a slash command, a saved
// skill's trigger, or work for the model, and hands each kind to a function the
// program's wiring supplies, so nothing here knows anything about the agent
// loop. Everything the agent sends goes out on one event stream that every
// attached channel subscribes to, and a reader that falls too far behind is
// dropped rather than allowed to stall the loop. The local socket is the first
// channel: a Unix socket in the home folder, readable by nobody but the agent's
// own user account, speaking one JSON message per line, that the terminal
// attaches to and that is a channel like any other.
package channel
