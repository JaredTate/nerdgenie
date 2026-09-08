package testkit

// DrawsFrames puts down how many animation frames the fake page draws in a
// quarter of a second, which every screenshot after it carries as
// FramesDrawn, so a test can photograph a page that is alive or one whose
// loop has stopped. Until a test sets it, the fake page draws none.
func (worker *FakeBrowserWorker) DrawsFrames(frames int) {
	worker.guard.Lock()
	defer worker.guard.Unlock()
	worker.framesDrawn = frames
}
