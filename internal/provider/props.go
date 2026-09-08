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
	"path"

	"encoding/json"
	"github.com/JaredTate/nerdgenie/internal/contract"
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
	// modelPath is the model file the server loaded, as it names it.
	modelPath string
}

// ModelFileOf is the base name of the model file a local daemon has loaded for
// this alias, or the alias's model name when the server does not say, which
// is what a screen shows at its top so a person knows which model is running.
func ModelFileOf(alias contract.ModelAlias) string {
	if facts, found := probeLocalServer(alias.BaseAddress); found && facts.modelPath != "" {
		return path.Base(facts.modelPath)
	}
	return alias.ModelName
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
		ModelPath                 string `json:"model_path"`
		DefaultGenerationSettings *struct {
			ContextLength int `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}{}
	if err := json.Unmarshal(body, &shaped); err != nil || shaped.DefaultGenerationSettings == nil {
		return localServerFacts{}, false
	}
	return localServerFacts{contextLength: shaped.DefaultGenerationSettings.ContextLength, modelPath: shaped.ModelPath}, true
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

// isLoopbackBaseAddress says whether a base address names a server on this
// machine, from the address alone and without asking whether it is up. The
// local daemon is told whether to think whether or not it answered the probe,
// so a harness that started before the daemon does not silently lose the
// thinking-off default: the qwen chat template thinks at its highest effort
// when nothing tells it not to, and the probe only reads the window.
func isLoopbackBaseAddress(baseAddress string) bool {
	parsed, err := url.Parse(baseAddress)
	if err != nil || parsed.Host == "" {
		return false
	}
	return isLoopbackHost(parsed.Hostname())
}

// isLoopbackHost says whether a host name is this machine.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

// IsLoopbackBaseAddressForTest exposes isLoopbackBaseAddress to the package's
// black-box tests.
func IsLoopbackBaseAddressForTest(baseAddress string) bool { return isLoopbackBaseAddress(baseAddress) }
