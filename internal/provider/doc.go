// Package provider turns one request into a streamed reply through the
// Anthropic API, the OpenAI-compatible API, the Responses API on OpenAI's
// ChatGPT backend, or a vendor's command-line program running on the user's
// own subscription.
//
// Every model the user names in the configuration becomes one value that
// answers the contract.Model interface. The three wire protocols are spoken
// directly over net/http, because the harness owns its own stream parsers and
// fuzzes them. The ChatGPT backend is reached with the sign-in that the
// vendor's own program, the one run as "codex exec", keeps in its own file,
// read and never written, so that a machine with no API key can still drive a
// GPT model with the harness's own tools. The fourth
// kind runs "claude -p" or "codex exec" in a scratch
// folder of its own, holding nothing but that call's system prompt, feeds the
// conversation on standard input, and reads the text back, which is how the two
// cloud models are reached on a machine with no API keys.
// Around any of them the package puts a retry wrapper, which tries three times
// with a growing wait and never retries a request that was simply too long, and
// a fallback chain, which moves on to the next model when the first one is out
// of tries.
package provider
