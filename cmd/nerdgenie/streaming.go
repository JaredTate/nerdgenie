package main

import "context"

// The reply's pieces travel from the model to the screens through the socket as
// they are written, and a retry withdraws them (brief 6.8). These two are the
// loop's and the provider's seams for that.

// streamReplyPiece hands the screens one piece of the reply the model is
// writing. A piece that cannot be sent is dropped, because the finished reply
// carries the whole text and is the authority over the pieces.
func (running *agent) streamReplyPiece(piece string) {
	if running.socket == nil {
		return
	}
	_ = running.socket.SendDelta(context.Background(), piece)
}

// finishStreamedReply lets the end of the reply out to the screens the moment
// the model has finished it, before its calls run, so that the last words of
// a sentence land before the line for the call and not after it.
func (running *agent) finishStreamedReply() {
	if running.socket == nil {
		return
	}
	_ = running.socket.FinishDelta(context.Background())
}

// withdrawStreamedReply tells the screens to take the partial reply down,
// because the call behind it failed and is being tried again.
func (running *agent) withdrawStreamedReply() {
	if running.socket == nil {
		return
	}
	_ = running.socket.ResetDelta(context.Background())
}

// wroteUnseen passes what the provider says the model wrote unseen, its
// thinking and its tool calls, to the watched model's count of the call in
// progress, and drops it before the watched model exists.
func (running *agent) wroteUnseen(characters int) {
	if running.watched != nil {
		running.watched.countUnseen(characters)
	}
}
