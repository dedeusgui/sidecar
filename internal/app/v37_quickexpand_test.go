package app

import "testing"

// The automatic QuickExpand is opt-in for V3.7 only. On the external cohorts it lowered voice-mode
// top-3 (6 -> 9 when disabled) and cost per delivered gold without adding gold. V3.5 and V3.6 keep it.
func TestQuickExpandIsOptInForV37Only(t *testing.T) {
	t.Setenv("SIDECAR_FEATURES", "")
	if quickExpandEnabled(true) {
		t.Fatal("V3.7 must not expand automatically by default")
	}
	if !quickExpandEnabled(false) {
		t.Fatal("non-V3.7 engines keep QuickExpand enabled")
	}
	t.Setenv("SIDECAR_FEATURES", "quickexpand")
	if !quickExpandEnabled(true) {
		t.Fatal("V3.7 expands when the quickexpand feature is requested explicitly")
	}
}
