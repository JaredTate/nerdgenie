// Package provider turns one request into a streamed reply through the
// Anthropic API, the OpenAI-compatible API, or a vendor's command-line program
// running on the user's own subscription.
//
// Every model the user names in the configuration becomes one value that
// answers the contract.Model interface. The two wire protocols are spoken
// directly over net/http, because the harness owns its own stream parsers and
// fuzzes them. The third kind runs "claude -p" or "codex exec" in an empty
// scratch folder, feeds the prompt on standard input, and reads the text back,
// which is how the two cloud models are reached on a machine with no API keys.
// Around any of them the package puts a retry wrapper, which tries three times
// with a growing wait and never retries a request that was simply too long, and
// a fallback chain, which moves on to the next model when the first one is out
// of tries.
package provider
