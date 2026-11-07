package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// requiredScenarioCategories are the twelve areas docs/roadmap.md (E1) asks the suite to cover.
// Two grammar-related areas and the generated-file check share item 10 of the roadmap.
var requiredScenarioCategories = []string{
	"vague_pt", "numeric_rule", "conflict", "contract_test", "impact", "debug", "extend",
	"continuation", "multi_module", "grammar_rust", "grammar_php", "generated_ignored",
	"bilingual_pt_en", "negative",
}

// TestV2ScenarioSuite checks the shipped suite's shape: at least twelve cases, every required
// category present, and gold that points at real files and names in the fixture. A typo in gold
// would otherwise look like a retrieval miss.
func TestV2ScenarioSuite(t *testing.T) {
	raw, err := os.ReadFile("../../bench/v2_scenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	var cf casesFile
	if err := json.Unmarshal(raw, &cf); err != nil {
		t.Fatal(err)
	}
	if len(cf.Cases) < 12 {
		t.Fatalf("suite has %d cases, want at least 12", len(cf.Cases))
	}
	spec, ok := cf.Repos["v2"]
	if !ok {
		t.Fatal("suite must define repo v2")
	}
	fixture := filepath.Join("../..", spec.Fixture)

	categories := map[string]bool{}
	ids := map[string]bool{}
	for _, c := range cf.Cases {
		if ids[c.ID] {
			t.Fatalf("case id %s repeated", c.ID)
		}
		ids[c.ID] = true
		categories[c.Category] = true
		if c.Negative && len(c.Relevant) > 0 {
			t.Errorf("%s: negative case lists relevant items", c.ID)
		}
		if !c.Negative && len(c.Relevant) == 0 {
			t.Errorf("%s: positive case has no gold", c.ID)
		}
		for _, g := range c.Relevant {
			checkGoldInFixture(t, c.ID, fixture, g)
		}
		for _, p := range c.MustNot {
			if _, err := os.Stat(filepath.Join(fixture, p)); err != nil {
				t.Errorf("%s: must_not path %s is not in the fixture", c.ID, p)
			}
		}
	}
	for _, want := range requiredScenarioCategories {
		if !categories[want] {
			t.Errorf("no scenario covers category %q", want)
		}
	}
}

func checkGoldInFixture(t *testing.T, id, fixture string, g gold) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture, g.Path))
	if err != nil {
		t.Errorf("%s: gold path %s is not in the fixture", id, g.Path)
		return
	}
	if g.Name != "" && !strings.Contains(string(data), g.Name) {
		t.Errorf("%s: gold name %s does not appear in %s", id, g.Name, g.Path)
	}
}
