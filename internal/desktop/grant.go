package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/JaredTate/coeus/internal/contract"
)

// MaxApplicationNameRunes is how long an application's name may be. It is a
// program's name or a window's title, and the user reads it in a preview, so a
// name longer than a line is a name nobody can judge.
const MaxApplicationNameRunes = 120

// MaxGrantedApplications is how many applications one session may grant. A task
// works in one application or two, and twenty is far past what a person would
// say yes to in one sitting, so anything beyond it is a task that has gone wrong.
const MaxGrantedApplications = 20

// grant asks the user for the one application the agent may use, once per
// session. Design section 10 says the user grants access to an application once
// per session, and that actions inside it that cannot be undone still get a
// preview of their own.
func (desktop *Desktop) grant(ctx context.Context, application string) error {
	if application == "" {
		return errors.New("the desktop was asked to open an application with no name, so say which one")
	}
	if letters := utf8.RuneCountInString(application); letters > MaxApplicationNameRunes {
		return fmt.Errorf("the application name is %d characters long and the most that may be asked for is %d, so name the program itself,"+
			" such as gedit, or the title of its window", letters, MaxApplicationNameRunes)
	}
	if err := desktop.roomToGrantOneMore(application); err != nil {
		return err
	}
	desktop.guard.Lock()
	already := desktop.granted[application]
	desktop.guard.Unlock()
	if already {
		return nil
	}

	answer, err := desktop.options.Channel.ShowPreview(ctx, contract.Preview{
		ID:    "desktop-grant-" + application,
		Title: fmt.Sprintf("use the application %s on your desktop", application),
		Body: fmt.Sprintf(
			"The agent wants to open %s on your screen and use it: it will be able to see that window, click its controls, and type into it until this task ends. Nothing outside that window is clicked or typed into.",
			application),
	})
	if err != nil {
		return fmt.Errorf("the user could not be asked whether the agent may use %s: %w", application, err)
	}
	if answer.Answer == contract.AnswerReject {
		return fmt.Errorf("the user did not grant the application %s, so the desktop cannot open it", application)
	}

	desktop.guard.Lock()
	desktop.granted[application] = true
	desktop.guard.Unlock()
	desktop.options.Note("the user granted the desktop application %q for this session", application)
	return nil
}

// roomToGrantOneMore refuses an application the session has no room to keep, so
// that the list of granted applications cannot grow without a limit.
func (desktop *Desktop) roomToGrantOneMore(application string) error {
	desktop.guard.Lock()
	defer desktop.guard.Unlock()
	if desktop.granted[application] || len(desktop.granted) < MaxGrantedApplications {
		return nil
	}
	return fmt.Errorf("this session has already granted %d applications, which is all it keeps, so finish in one of those or start a new task",
		MaxGrantedApplications)
}

// permit puts one action that cannot be undone to the permission function, and
// shows the user a preview when the ruling is to ask.
func (desktop *Desktop) permit(ctx context.Context, intent string, element string, text string) error {
	written, err := json.Marshal(map[string]string{"intent": intent, "element": element, "text": text})
	if err != nil {
		return fmt.Errorf("the desktop call %q could not be written down for the permission function: %w", intent, err)
	}
	request := contract.PermissionRequest{ToolName: contract.ToolComputer, Input: written}
	decision, err := desktop.options.Permission.Decide(ctx, request)
	if err != nil {
		return fmt.Errorf("the permission function could not rule on the desktop action %q: %w", intent, err)
	}

	switch decision.Ruling {
	case contract.RulingAllow:
		return nil
	case contract.RulingDeny:
		return fmt.Errorf("the desktop action %q is not allowed: %s", intent, decision.Reason)
	case contract.RulingStop:
		return fmt.Errorf("the desktop action %q needs a yes and nobody is there to give one, so the task stops: %s", intent, decision.Reason)
	case contract.RulingAsk:
		return desktop.askTheUser(ctx, request, intent, decision, text)
	default:
		return fmt.Errorf("the permission function ruled %q on the desktop action %q, which is not one of the four rulings", decision.Ruling, intent)
	}
}

// askTheUser shows the preview and remembers the answer for the session.
func (desktop *Desktop) askTheUser(
	ctx context.Context,
	request contract.PermissionRequest,
	intent string,
	decision contract.PermissionDecision,
	text string,
) error {
	answer, err := desktop.options.Channel.ShowPreview(ctx, contract.Preview{
		ID:    "desktop-action",
		Title: intent,
		Body:  previewBody(decision.PreviewText, text),
	})
	if err != nil {
		return fmt.Errorf("the user could not be shown what the desktop was about to do: %w", err)
	}
	if err := desktop.options.Permission.Remember(request, answer.Answer, decision.Reason); err != nil {
		return fmt.Errorf("the user's answer about the desktop action could not be remembered: %w", err)
	}
	if answer.Answer == contract.AnswerReject {
		return fmt.Errorf("the user refused the desktop action %q", intent)
	}
	return nil
}

// previewBody is exactly what is about to happen, which is the permission
// function's own words plus the text that is about to be typed or pasted.
func previewBody(previewText string, text string) string {
	if text == "" {
		return previewText
	}
	if previewText == "" {
		return text
	}
	return previewText + "\n\n" + text
}
