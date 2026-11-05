package fusion

import "testing"

func TestRRFRewardsItemsRankedHighInMultipleLists(t *testing.T) {
	lists := [][]string{
		{"a", "b", "c"},
		{"b", "a", "d"},
		{"b", "e"},
	}
	scores := RRF(lists, 60)
	order := Order(scores)
	if order[0] != "b" {
		t.Fatalf("expected b first (ranked 1,2,1), got order %v scores %v", order, scores)
	}
	want := 1.0/61 + 1.0/62 + 1.0/61
	if got := scores["b"]; !approx(got, want) {
		t.Fatalf("RRF(b) = %v, want %v", got, want)
	}
}

func TestOrderBreaksTiesByKey(t *testing.T) {
	scores := map[string]float64{"z": 1, "a": 1, "m": 2}
	got := Order(scores)
	want := []string{"m", "a", "z"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Order = %v, want %v", got, want)
		}
	}
}

func TestDiversifyCapsItemsPerFileButKeepsOtherFiles(t *testing.T) {
	items := []Scored{
		{Key: "f1#1", File: "f1", Score: 9},
		{Key: "f1#2", File: "f1", Score: 8},
		{Key: "f1#3", File: "f1", Score: 7},
		{Key: "f2#1", File: "f2", Score: 6},
		{Key: "f1#4", File: "f1", Score: 5},
	}
	got := Diversify(items, 2, 10)
	keys := make([]string, len(got))
	for i, g := range got {
		keys[i] = g.Key
	}
	want := []string{"f1#1", "f1#2", "f2#1"}
	if len(keys) != len(want) {
		t.Fatalf("Diversify kept %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("Diversify kept %v, want %v", keys, want)
		}
	}
}

func TestDiversifyRespectsLimit(t *testing.T) {
	var items []Scored
	for i := 0; i < 10; i++ {
		items = append(items, Scored{Key: string(rune('a' + i)), File: string(rune('a' + i)), Score: float64(10 - i)})
	}
	if got := Diversify(items, 2, 3); len(got) != 3 {
		t.Fatalf("limit 3 returned %d items", len(got))
	}
}

func approx(a, b float64) bool {
	d := a - b
	return d < 1e-12 && d > -1e-12
}
