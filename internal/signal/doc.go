// Package signal is the Signal channel: it supervises signal-cli, links the
// account, pairs unknown senders, and carries messages both ways.
//
// The agent talks to Signal through signal-cli, a separate program linked to the
// user's Signal account as a secondary device, the same way the Signal desktop
// application is linked to a phone. This package starts that program as a child
// in its own process group, waits for its health endpoint, reads inbound
// messages from its server-sent-events stream, and sends replies, typing
// indicators, and files back over its JSON-RPC endpoint. A stream that drops or
// goes quiet is reconnected with a growing wait. Attachments are downloaded into
// a capped cache under the home's Signal folder and copied into the inbox, where
// the model can read them.
//
// A sender the agent does not know gets a pairing code and nothing else. The
// codes are kept salted and hashed in a file under the Signal folder, one per
// sender every ten minutes, good for an hour, with a cap on how many wait at
// once that a new sender pushes the oldest code out of rather than being turned
// away by, and a person who types five wrong codes is locked out of typing more.
// The "/pair" command approves the sender whose code matches. Everything the
// channel sends goes through the vault's redactor first, a whole reply before it
// is split rather than each message after, and the channel never asks for a
// secret, because Signal cannot hide what is typed.
package signal
