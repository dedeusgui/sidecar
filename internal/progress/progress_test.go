package progress

import "testing"

func ev(kind, key string) Event { return Event{Kind: kind, Key: key} }

func TestExactAndNearSearchRepeatsAreRedundantButAcrossAnEditTheyAreNot(t *testing.T) {
	r := Analyze([]Event{
		ev("search", "handle timeout error"), ev("search", "handle timeout error"),
		ev("search", "error timeout handle"), ev("search", "database migration"),
		ev("edit", "a.go"), ev("search", "handle timeout error"),
	})
	want := []string{New, ExactRepeat, NearRepeat, New, Progress, New}
	for i, w := range want {
		if r.Per[i] != w {
			t.Fatalf("event %d: got %s want %s (%v)", i, r.Per[i], w, r.Per)
		}
	}
	if r.Redundant != 2 {
		t.Fatalf("redundant = %d", r.Redundant)
	}
}

func TestRevisitIsRedundantUntilAnEditResetsTheEpoch(t *testing.T) {
	r := Analyze([]Event{ev("read", "a.go"), ev("read", "a.go"), ev("edit", "a.go"), ev("read", "a.go")})
	if r.Per[1] != Revisit || r.Per[3] != New {
		t.Fatalf("%v", r.Per)
	}
}

func TestOneWordSearchesAreNeverNearRepeats(t *testing.T) {
	r := Analyze([]Event{ev("search", "timeout"), ev("search", "retry")})
	if r.Labels[NearRepeat] != 0 {
		t.Fatalf("%v", r.Per)
	}
}

func TestRepeatedErrorWithoutEditVersusIteratedAfterEdit(t *testing.T) {
	e := func(kind, err string) Event { return Event{Kind: kind, Key: "x", Err: err} }
	r := Analyze([]Event{e("test", "undefined: foo"), e("test", "undefined: foo"), ev("edit", "a.go"), e("test", "undefined: foo")})
	if r.Per[1] != RepeatedError {
		t.Fatalf("same error with no edit between is a loop: %v", r.Per)
	}
	if r.Per[3] != Progress && r.Per[3] != IteratedError {
		t.Fatalf("the same error after an edit is iteration, not a loop: %v", r.Per)
	}
}

func TestStagnationNeedsRedundancyAndNoProgress(t *testing.T) {
	var evs []Event
	for i := 0; i < 8; i++ {
		evs = append(evs, ev("read", "a.go"))
	}
	r := Analyze(evs)
	if r.StagnantWindows == 0 || r.Longest == 0 {
		t.Fatalf("eight reads of one file are stagnation: %+v", r)
	}
	evs[3] = ev("edit", "a.go")
	evs[6] = ev("test", "t")
	if r2 := Analyze(evs); r2.StagnantWindows >= r.StagnantWindows {
		t.Fatalf("an edit and a test inside the window make it iteration: %+v", r2)
	}
	var fresh []Event
	for i := 0; i < 8; i++ {
		fresh = append(fresh, ev("read", string(rune('a'+i))+".go"))
	}
	if r3 := Analyze(fresh); r3.StagnantWindows != 0 || r3.NoveltyRatio != 1 {
		t.Fatalf("reading eight different files is exploration: %+v", r3)
	}
}

func TestShadowDecisionsAreRateLimitedAndNeverDelivered(t *testing.T) {
	var evs []Event
	for i := 0; i < 60; i++ {
		evs = append(evs, ev("read", "a.go"))
	}
	r := Analyze(evs)
	if len(r.Shadows) == 0 || len(r.Shadows) > ShadowMax {
		t.Fatalf("shadows %d", len(r.Shadows))
	}
	for i, s := range r.Shadows {
		if s.Deliver {
			t.Fatal("no transport is verified: nothing may be marked deliverable")
		}
		if i > 0 && s.At-r.Shadows[i-1].At < ShadowGap {
			t.Fatalf("shadow decisions closer than the gap: %+v", r.Shadows)
		}
	}
}

func TestEmptyAndUnobservedSequencesAreNotFindings(t *testing.T) {
	r := Analyze(nil)
	if r.Events != 0 || r.Redundant != 0 || len(r.Shadows) != 0 || r.NoveltyRatio != 0 {
		t.Fatalf("%+v", r)
	}
}
