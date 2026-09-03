package loop_test

import (
	"strings"

	"github.com/JaredTate/coeus/internal/contract"
)

// wholeRequestText joins everything a request would put in front of the model,
// the way the fake model's own expectation check does.
func wholeRequestText(request contract.Request) string {
	pieces := []string{}
	for _, block := range request.SystemBlocks {
		pieces = append(pieces, block.Text)
	}
	for _, message := range request.Messages {
		pieces = append(pieces, message.Text)
		for _, result := range message.ToolResults {
			pieces = append(pieces, result.Text)
		}
	}
	return strings.Join(pieces, "\n")
}
