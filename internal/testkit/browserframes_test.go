package testkit_test

import (
	"context"
	"testing"

	"github.com/JaredTate/nerdgenie/internal/testkit"
)

// TestTheFakeScreenshotCarriesTheFramesATestSets: a screenshot from the real
// worker says how many animation frames the page drew in a quarter of a
// second, so that a still scene is never taken for a broken camera. The fake
// carries the number a test sets, and none until one is set.
func TestTheFakeScreenshotCarriesTheFramesATestSets(t *testing.T) {
	ctx := context.Background()
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	if _, err := worker.Open(ctx, testkit.FixtureSimplePage); err != nil {
		t.Fatalf("opening the page failed: %v", err)
	}

	still, err := worker.Screenshot(ctx)
	if err != nil || still.FramesDrawn != 0 {
		t.Errorf("before a test sets the frames, the screenshot says %d were drawn with error %v, want 0", still.FramesDrawn, err)
	}
	worker.DrawsFrames(15)
	drawing, err := worker.Screenshot(ctx)
	if err != nil || drawing.FramesDrawn != 15 {
		t.Errorf("after a test sets 15 frames, the screenshot says %d were drawn with error %v", drawing.FramesDrawn, err)
	}
}

// TestTheProtocolServerPassesTheFramesDrawnThrough holds the fake's protocol
// server to the same wire as the real worker: the screenshot answer carries
// framesDrawn, as PROTOCOL.md's screenshot section says.
func TestTheProtocolServerPassesTheFramesDrawnThrough(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	defer worker.Close()
	worker.DrawsFrames(15)
	server := testkit.NewBrowserProtocolServer(t, worker)

	callProtocol(t, server.SocketPath(),
		`{"jsonrpc":"2.0","id":1,"method":"open","params":{"url":"`+testkit.FixtureSimplePage+`"}}`)
	answer := callProtocol(t, server.SocketPath(), `{"jsonrpc":"2.0","id":2,"method":"screenshot","params":{}}`)
	result := resultOf(t, answer)
	if frames, isNumber := result["framesDrawn"].(float64); !isNumber || frames != 15 {
		t.Errorf("the screenshot answered framesDrawn %v, want 15 on the wire", result["framesDrawn"])
	}
}

// TestTheFakeScreenshotSaysThePageIsVisibleUnlessATestHidesIt: a still page
// with no animation loop draws no frames, which is not a stopped page, so the
// screenshot also says whether the page is visible and answering; the fake
// says so unless a test hides the page.
func TestTheFakeScreenshotSaysThePageIsVisibleUnlessATestHidesIt(t *testing.T) {
	worker := testkit.NewFakeBrowserWorker()
	if _, err := worker.Open(context.Background(), testkit.FixtureSimplePage); err != nil {
		t.Fatalf("cannot open the fixture page: %v", err)
	}
	seen, err := worker.Screenshot(context.Background())
	if err != nil || !seen.Visible {
		t.Errorf("before a test hides the page, the screenshot says visible %v with error %v, want true", seen.Visible, err)
	}
	worker.HidesThePage()
	hidden, err := worker.Screenshot(context.Background())
	if err != nil || hidden.Visible {
		t.Errorf("after a test hides the page, the screenshot says visible %v with error %v, want false", hidden.Visible, err)
	}
}
