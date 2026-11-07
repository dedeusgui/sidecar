package billing

import "testing"

func TestLateFeeIsTwoPercent(t *testing.T) {
	if LateFeePercent != 2 {
		t.Fatalf("LateFeePercent = %d, want 2", LateFeePercent)
	}
}
