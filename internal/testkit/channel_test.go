package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/JaredTate/coeus/internal/contract"
	"github.com/JaredTate/coeus/internal/testkit"
)

func TestTheFakeChannelHandsBackWhatTheTestPushedIntoIt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	channel := testkit.NewFakeChannel("terminal")

	inbound, err := channel.Receive(ctx)
	if err != nil {
		t.Fatalf("attaching to the channel failed: %v", err)
	}
	channel.Push(contract.Inbound{ID: "1", Sender: "jared", Text: "post the anniversary tweet"})

	select {
	case message := <-inbound:
		if message.Text != "post the anniversary tweet" {
			t.Errorf("the message came out as %q, want the one that was pushed", message.Text)
		}
		if message.Channel != "terminal" {
			t.Errorf("the message says it came through %q, want terminal", message.Channel)
		}
	case <-time.After(time.Second):
		t.Fatal("nothing came out of the channel within a second")
	}
}

func TestTheFakeChannelRecordsEveryReplyAndEveryFile(t *testing.T) {
	ctx := context.Background()
	channel := testkit.NewFakeChannel("signal")

	if err := channel.Send(ctx, "the post is up"); err != nil {
		t.Fatalf("sending failed: %v", err)
	}
	if err := channel.SendFile(ctx, "/tmp/shot.png", "the browser right now"); err != nil {
		t.Fatalf("sending a file failed: %v", err)
	}

	if sent := channel.Sent(); len(sent) != 1 || sent[0] != "the post is up" {
		t.Errorf("the channel recorded the replies %v, want the one that was sent", sent)
	}
	files := channel.Files()
	if len(files) != 1 || files[0].Path != "/tmp/shot.png" || files[0].Caption != "the browser right now" {
		t.Errorf("the channel recorded the files %+v, want the one that was sent", files)
	}
}

func TestTheFakeChannelAnswersAPreviewTheWayTheTestSaid(t *testing.T) {
	ctx := context.Background()
	channel := testkit.NewFakeChannel("terminal")
	channel.AnswerPreviewsWith(contract.AnswerAlways)

	answer, err := channel.ShowPreview(ctx, contract.Preview{ID: "3", Title: "run a command", Body: "sudo apt install ripgrep"})
	if err != nil {
		t.Fatalf("showing the preview failed: %v", err)
	}
	if answer != contract.AnswerAlways {
		t.Errorf("the preview was answered %q, want %q", answer, contract.AnswerAlways)
	}
	if previews := channel.Previews(); len(previews) != 1 || previews[0].Body == "" {
		t.Errorf("the channel recorded the previews %+v, want the one that was shown with its body", previews)
	}
}

func TestTheFakeChannelAnswersAMaskedPromptOrRefusesToShowOne(t *testing.T) {
	ctx := context.Background()
	channel := testkit.NewFakeChannel("terminal")
	channel.AnswerSecretWith("correct horse battery staple")

	secret, err := channel.AskSecret(ctx, "the password for x.com")
	if err != nil {
		t.Fatalf("asking for a secret failed: %v", err)
	}
	if secret != "correct horse battery staple" {
		t.Errorf("the masked prompt gave back %q, want what the test set", secret)
	}

	overSignal := testkit.NewFakeChannel("signal")
	overSignal.CannotMaskSecrets()
	if _, err := overSignal.AskSecret(ctx, "the password"); err == nil {
		t.Error("a channel that cannot hide what is typed still asked for a secret, and it must refuse")
	}
}

func TestTheFakeChannelReportsTheHealthTheTestSet(t *testing.T) {
	ctx := context.Background()
	channel := testkit.NewFakeChannel("signal")

	if health := channel.Health(ctx); !health.Healthy {
		t.Error("a fresh fake channel reports itself unhealthy")
	}
	channel.SetHealth(false, "signal-cli is not answering, so restart it")
	health := channel.Health(ctx)
	if health.Healthy || health.Detail == "" {
		t.Errorf("the channel reports %+v, want unhealthy with a reason", health)
	}
}

func TestTheFakeChannelKeepsTheChannelContract(t *testing.T) {
	if err := testkit.CheckChannel(context.Background(), testkit.NewFakeChannel("terminal")); err != nil {
		t.Fatalf("the fake channel does not keep the channel contract: %v", err)
	}
}
