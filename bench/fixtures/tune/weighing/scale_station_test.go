package weighing

import "testing"

func TestConfirmWeightRejectsEmptyPlate(t *testing.T) {
	s := &ScaleStation{}
	if s.ConfirmWeight("", 100) {
		t.Fatal("empty plate accepted")
	}
}
