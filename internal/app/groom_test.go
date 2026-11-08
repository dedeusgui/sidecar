package app

import (
	"strings"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/groom"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

func TestGroomQuestionIsDeliveredEvenWhenThereIsNoOtherEvidence(t *testing.T) {
	res := retrieve.Result{} // budget 0: nothing else to show
	rec := map[string]any{}
	if err := applyGroom(nil, t.TempDir(), t.TempDir()+"/state", adapter.Claude, "s", "adicione um limite de tentativas no login", &res, time.Now(), rec); err != nil {
		t.Fatal(err)
	}
	if rec["groom_action"] != groom.Ask {
		t.Fatalf("expected a question: %v", rec)
	}
	var items []ctxbuild.Item
	for _, ev := range res.Evidence {
		items = append(items, ctxbuild.Item{Section: ev.Section, Key: ev.Key(), Fingerprint: ev.Fingerprint(), Text: retrieve.Describe(ev)})
	}
	if out := ctxbuild.Build(items, res.Trace.Budget, nil); !strings.Contains(out, "Open decision") {
		t.Fatalf("the question was dropped by the block budget (%d tokens): %q", res.Trace.Budget, out)
	}
}

func TestGroomDoesNotRepeatAPendingQuestionOrSpendTheQuota(t *testing.T) {
	state := t.TempDir() + "/state"
	prompt := "adicione um limite de tentativas no login"
	for turn := 1; turn <= 3; turn++ {
		res := retrieve.Result{}
		rec := map[string]any{}
		_ = applyGroom(nil, t.TempDir(), state, adapter.Claude, "s", prompt, &res, time.Now(), rec)
		if turn == 1 && rec["groom_action"] != groom.Ask {
			t.Fatalf("turn 1 must ask: %v", rec)
		}
		if turn > 1 && (rec["groom_action"] != "pending" || len(res.Evidence) != 0) {
			t.Fatalf("turn %d must not repeat the pending question: %v", turn, rec)
		}
	}
	if st := groom.LoadState(state + ".groom.json"); st.Asked != 1 {
		t.Fatalf("the quota was spent %d times for one question", st.Asked)
	}
}

func TestAwaitingAnswerExpiresWithThePendingQuestion(t *testing.T) {
	state := t.TempDir() + "/state"
	t.Setenv("SIDECAR_FEATURES", "groom")
	groom.SaveState(state+".groom.json", groom.SessionState{Asked: 1, Turn: 1, Pending: &groom.Pending{Decision: "d", Turn: 1, Options: []groom.Option{{Label: "a"}}}})
	opts := HookOptions{Agent: adapter.Claude, StatePath: func(string, string) string { return state }}
	if !awaitingAnswer(opts, adapter.Input{SessionID: "s", Prompt: "B"}) {
		t.Fatal("a fresh pending question must keep short replies alive")
	}
	groom.SaveState(state+".groom.json", groom.SessionState{Asked: 1, Turn: 9, Pending: &groom.Pending{Decision: "d", Turn: 1, Options: []groom.Option{{Label: "a"}}}})
	if awaitingAnswer(opts, adapter.Input{SessionID: "s", Prompt: "B"}) {
		t.Fatal("an expired question must not rescue a tiny prompt")
	}
	if awaitingAnswer(opts, adapter.Input{SessionID: "s", Prompt: "/help"}) {
		t.Fatal("a slash command is never an answer")
	}
}

func TestRecordAnswerBindsToTheSessionAndRefusesWithoutAFile(t *testing.T) {
	if err := recordAnswer((*store.Store)(nil), t.TempDir(), "s", groom.Pending{Decision: "d"}, groom.Option{Label: "x"}, time.Now()); err == nil {
		t.Fatal("an answer with no evidence file to bind to must be refused, not stored unbound")
	}
}
