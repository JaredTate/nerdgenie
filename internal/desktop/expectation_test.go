package desktop

import (
	"reflect"
	"testing"
)

// The act-and-assert rule says every action states what the model expected to
// happen and the worker checks it. It only holds if the expectation is on the
// method the caller actually calls: while the expectation lived on a twin of
// each action, the only caller in the running program called the plain name and
// the expectation the model wrote was thrown away before it left the tool.

// theFiveActions is each action and how many things it carries besides its
// receiver and its context: what to act on, and what the model expected.
var theFiveActions = map[string]int{
	"Launch": 2,
	"Click":  2,
	"Type":   2,
	"Press":  2,
	"Drag":   3,
}

func TestEveryActionCarriesWhatTheModelExpectedAndHasNoTwin(t *testing.T) {
	shape := reflect.TypeOf(&Desktop{})
	for name, carried := range theFiveActions {
		method, found := shape.MethodByName(name)
		if !found {
			t.Errorf("the desktop has no %s at all, and it is one of the five actions", name)
			continue
		}
		if carries := method.Type.NumIn() - 2; carries != carried {
			t.Errorf("the arguments %s takes besides its receiver and its context number %d, want %d: what to act on, and what the model"+
				" expected to happen", name, carries, carried)
			continue
		}
		if last := method.Type.In(method.Type.NumIn() - 1); last.Kind() != reflect.String {
			t.Errorf("the last thing %s carries is a %s, and it must be the expectation, written as a string", name, last)
		}
		if _, twin := shape.MethodByName(name + "Expecting"); twin {
			t.Errorf("%sExpecting is still there, and a twin is how the expectation was lost: the only caller in the running program calls %s",
				name, name)
		}
	}
}
