// The two rules in this file were adapted from Hermes: it asks a local server
// what it really is before trusting the configuration, at
// ~/Code/hermes-agent/agent/model_metadata.py, where the probe reads
// default_generation_settings from the props endpoint, and it sends a
// thinking-off hint only to a server known to understand one, at
// ~/Code/hermes-agent/plugins/model-providers/custom/__init__.py. Hermes tries
// several endpoints in a waterfall; Nerd Genie asks one question of one kind of
// server, because there is one kind of local server on the machine it runs on.

package provider

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// probeTimeout is how long the one question at construction may take. It is
// short because a local server answers at once and a server that does not
// answer is simply not one of these.
const probeTimeout = 2 * time.Second

// maxPropsBytes caps how much of a server's answer about itself is read.
const maxPropsBytes = 1 << 20

// localServerFacts is what a local server said about itself.
type localServerFacts struct {
	// contextLength is the window the server really loaded the model with,
	// which can be smaller than the one config.toml names.
	contextLength int
}

// probeLocalServer asks a server on a loopback address about itself, and says
// whether it answered the way llama-server does. A server anywhere else, or one
// that answers anything else, is left alone: it gets no extra fields and its
// configured window stands.
func probeLocalServer(baseAddress string) (localServerFacts, bool) {
	address, ok := propsAddress(baseAddress)
	if !ok {
		return localServerFacts{}, false
	}
	client := &http.Client{Timeout: probeTimeout}
	answer, err := client.Get(address)
	if err != nil {
		return localServerFacts{}, false
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return localServerFacts{}, false
	}
	body, err := io.ReadAll(io.LimitReader(answer.Body, maxPropsBytes))
	if err != nil {
		return localServerFacts{}, false
	}
	return readProps(body)
}

// readProps takes the window out of a server's answer about itself, and says
// whether the answer was the shape llama-server sends.
func readProps(body []byte) (localServerFacts, bool) {
	shaped := struct {
		DefaultGenerationSettings *struct {
			ContextLength int `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}{}
	if err := json.Unmarshal(body, &shaped); err != nil || shaped.DefaultGenerationSettings == nil {
		return localServerFacts{}, false
	}
	return localServerFacts{contextLength: shaped.DefaultGenerationSettings.ContextLength}, true
}

// propsAddress turns a base address into the address of the question, and says
// whether the base address is on this machine at all. Only a loopback address is
// probed, because a server somewhere else is not one this rule is about.
func propsAddress(baseAddress string) (string, bool) {
	parsed, err := url.Parse(baseAddress)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	if !isLoopbackHost(parsed.Hostname()) {
		return "", false
	}
	parsed.Path = strings.TrimSuffix(strings.TrimSuffix(parsed.Path, "/"), "/v1") + "/props"
	parsed.RawQuery = ""
	return parsed.String(), true
}

// isLoopbackHost says whether a host name is this machine.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}
