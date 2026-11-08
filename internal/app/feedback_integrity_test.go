package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/brief"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/paths"
	"github.com/dedeusgui/sidecar/internal/store"
)

var testHarnessKey = []byte("harness-key-for-tests-only")

// gitFeedbackProject is an indexed Git worktree, so the oracle artifact can name a real HEAD commit.
func gitFeedbackProject(t *testing.T) string {
	t.Helper()
	root := indexedFeedbackProject(t)
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return root
}

func headOf(t *testing.T, root string) string {
	t.Helper()
	head, err := gitx.Head(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return head
}

// signedArtifact is what the harness writes: its identity fields plus an HMAC under its own key.
func signedArtifact(t *testing.T, root, session, status string) OracleArtifact {
	t.Helper()
	a := OracleArtifact{SchemaVersion: 1, RunID: "run-1", SessionID: session, TaskID: "task-1",
		Commit: headOf(t, root), OracleSHA256: strings.Repeat("ab", 32), Status: status}
	a.Digest = OracleDigest(testHarnessKey, a)
	return a
}

// deliver records in the session's observer state that the hook injected these items, through the same
// code path the hook uses.
func deliver(t *testing.T, root, session string, keys ...string) {
	t.Helper()
	recordEvidence(root, adapter.Claude, session, keys)
}

// observe sets what the observer saw of a file in the session: an edit or a read.
func observe(t *testing.T, root, session, file string, edit bool) {
	t.Helper()
	statePath := paths.ObserverState(root, string(adapter.Claude), session)
	s := brief.LoadSignals(statePath, worktreeVersion(root), time.Now())
	if edit {
		s.Edits[hashID(root, file)] = 1
	} else {
		s.Reads[hashID(root, file)] = 1
	}
	if err := brief.SaveSignals(statePath, s); err != nil {
		t.Fatal(err)
	}
}

const releaseID = "release/handler.go#method:Release@6"

func recordOne(t *testing.T, root string, report store.AgentFeedbackReport) store.FeedbackRecord {
	t.Helper()
	recs, err := RecordFeedback(root, report)
	if err != nil || len(recs) != 1 {
		t.Fatalf("RecordFeedback = %d records (%v), want 1", len(recs), err)
	}
	return recs[0]
}

// A report cannot promote its own provenance by editing a field.
func TestForgedProvenanceInReportIsRefused(t *testing.T) {
	root := gitFeedbackProject(t)
	if _, err := RecordFeedback(root, store.AgentFeedbackReport{SchemaVersion: 1, OracleStatus: "passed",
		OracleProvenance: "harness", HelpfulEvidenceIDs: []string{releaseID}}); err == nil {
		t.Fatal("a report declared harness provenance and was recorded")
	}
	if _, err := RecordFeedback(root, store.AgentFeedbackReport{SchemaVersion: 1, OracleStatus: "passed",
		OracleProvenance: "human", HelpfulEvidenceIDs: []string{releaseID}}); err == nil {
		t.Fatal("a report declared human provenance and was recorded")
	}
}

// An artifact is accepted only when the harness signed it, for this session and this commit.
func TestOracleArtifactMustVerify(t *testing.T) {
	root := gitFeedbackProject(t)
	const session = "s-art"
	deliver(t, root, session, "function|release/handler.go|Release")
	good := signedArtifact(t, root, session, "passed")
	report := store.AgentFeedbackReport{SchemaVersion: 1, SessionID: session, HelpfulEvidenceIDs: []string{releaseID}}

	tampered := good
	tampered.Status = "failed" // a changed field with the old digest
	cases := map[string]struct {
		art OracleArtifact
		key []byte
	}{
		"tampered field":  {tampered, testHarnessKey},
		"wrong key":       {good, []byte("not-the-harness-key!!")},
		"short key":       {good, []byte("short")},
		"other session":   {signedArtifact(t, root, "other", "passed"), testHarnessKey},
		"stale commit":    {withCommit(good, strings.Repeat("0", 40), testHarnessKey), testHarnessKey},
		"unknown status":  {withStatus(good, "unknown", testHarnessKey), testHarnessKey},
		"bad oracle hash": {withHash(good, "not-hex", testHarnessKey), testHarnessKey},
	}
	for name, c := range cases {
		if _, err := RecordFeedbackWithOracle(root, report, c.art, c.key); err == nil {
			t.Errorf("%s: artifact accepted", name)
		}
	}
	if list, _ := ListFeedback(root); len(list) != 0 {
		t.Fatalf("refused artifacts wrote %d records", len(list))
	}
	recs, err := RecordFeedbackWithOracle(root, report, good, testHarnessKey)
	if err != nil || len(recs) != 1 {
		t.Fatalf("valid artifact refused: %d records, %v", len(recs), err)
	}
	if r := recs[0]; r.Provenance != store.ProvenanceHarness || r.Tier != store.TierStrong || r.OracleRunID != "run-1" {
		t.Fatalf("verified artifact = provenance %q, tier %q, run %q; want harness, strong, run-1", r.Provenance, r.Tier, r.OracleRunID)
	}
}

func withCommit(a OracleArtifact, commit string, key []byte) OracleArtifact {
	a.Commit = commit
	a.Digest = OracleDigest(key, a)
	return a
}

func withStatus(a OracleArtifact, status string, key []byte) OracleArtifact {
	a.Status = status
	a.Digest = OracleDigest(key, a)
	return a
}

func withHash(a OracleArtifact, hash string, key []byte) OracleArtifact {
	a.OracleSHA256 = hash
	a.Digest = OracleDigest(key, a)
	return a
}

// Evidence that was never delivered cannot be strong or medium, even with a passing independent oracle.
func TestEvidenceNeverDeliveredIsNotAttributed(t *testing.T) {
	root := gitFeedbackProject(t)
	const session = "s-never"
	observe(t, root, session, "release/handler.go", true) // edited, but the hook never delivered it
	art := signedArtifact(t, root, session, "passed")
	recs, err := RecordFeedbackWithOracle(root, store.AgentFeedbackReport{SchemaVersion: 1, SessionID: session,
		HelpfulEvidenceIDs: []string{releaseID}}, art, testHarnessKey)
	if err != nil || len(recs) != 1 {
		t.Fatalf("record: %d, %v", len(recs), err)
	}
	if r := recs[0]; r.Presented || r.Tier != store.TierWeak {
		t.Fatalf("undelivered evidence = presented %v, tier %q; want false, weak", r.Presented, r.Tier)
	}
}

// An ID that names nothing in the index is refused before anything is written.
func TestUnknownEvidenceIDIsRefused(t *testing.T) {
	root := gitFeedbackProject(t)
	for _, id := range []string{"missing/file.go#Thing", "release/handler.go#NoSuchSymbol"} {
		if _, err := RecordFeedback(root, store.AgentFeedbackReport{SchemaVersion: 1, HelpfulEvidenceIDs: []string{id}}); err == nil {
			t.Errorf("unknown evidence %q was recorded", id)
		}
	}
	if list, _ := ListFeedback(root); len(list) != 0 {
		t.Fatalf("unknown IDs wrote %d records", len(list))
	}
}

// The same evidence cannot be reported both helpful and misleading.
func TestContradictoryReportIsRefused(t *testing.T) {
	root := gitFeedbackProject(t)
	if _, err := RecordFeedback(root, store.AgentFeedbackReport{SchemaVersion: 1,
		HelpfulEvidenceIDs: []string{releaseID}, MisleadingEvidenceIDs: []string{releaseID}}); err == nil {
		t.Fatal("contradictory report was recorded")
	}
}

// An oracle run that failed on delivered evidence is a strong negative.
func TestFailedIndependentOracleIsStrongNegative(t *testing.T) {
	root := gitFeedbackProject(t)
	const session = "s-fail"
	deliver(t, root, session, "function|release/handler.go|Release")
	art := signedArtifact(t, root, session, "failed")
	recs, err := RecordFeedbackWithOracle(root, store.AgentFeedbackReport{SchemaVersion: 1, SessionID: session,
		MisleadingEvidenceIDs: []string{releaseID}}, art, testHarnessKey)
	if err != nil || len(recs) != 1 {
		t.Fatalf("record: %d, %v", len(recs), err)
	}
	if r := recs[0]; r.Tier != store.TierStrong || r.OraclePassed != -1 {
		t.Fatalf("failed independent oracle = tier %q, outcome %d; want strong, -1", r.Tier, r.OraclePassed)
	}
}

// Delivered and used without any further read: presented, but the trace shows no interaction, so weak.
func TestDeliveredWithoutReadStaysWeak(t *testing.T) {
	root := gitFeedbackProject(t)
	const session = "s-noread"
	deliver(t, root, session, "function|release/handler.go|Release")
	r := recordOne(t, root, store.AgentFeedbackReport{SchemaVersion: 1, SessionID: session,
		OracleStatus: "passed", HelpfulEvidenceIDs: []string{releaseID}})
	if !r.Presented || r.EvidenceUsed != store.EvidenceUnknown || r.Tier != store.TierWeak {
		t.Fatalf("delivered without read = presented %v, used %q, tier %q; want true, unknown, weak", r.Presented, r.EvidenceUsed, r.Tier)
	}
}

// Delivered, edited, and the final test failed: medium, because the edit ties it to the patch. The
// failing oracle is a self-report, so it cannot make the record strong.
func TestEditWithFailingTestIsMediumNotStrong(t *testing.T) {
	root := gitFeedbackProject(t)
	const session = "s-edit"
	deliver(t, root, session, "function|release/handler.go|Release")
	observe(t, root, session, "release/handler.go", true)
	r := recordOne(t, root, store.AgentFeedbackReport{SchemaVersion: 1, SessionID: session,
		OracleStatus: "failed", HelpfulEvidenceIDs: []string{releaseID}})
	if r.Tier != store.TierMedium || r.EvidenceUsed != store.EvidenceConfirmed {
		t.Fatalf("edit with failing self-reported test = tier %q, used %q; want medium, confirmed", r.Tier, r.EvidenceUsed)
	}
}

// Feedback for a session that never received the evidence is not attributed, even if the file was
// delivered to another session.
func TestOtherSessionDoesNotAttributeEvidence(t *testing.T) {
	root := gitFeedbackProject(t)
	deliver(t, root, "s-other", "function|release/handler.go|Release")
	observe(t, root, "s-mine", "release/handler.go", true)
	r := recordOne(t, root, store.AgentFeedbackReport{SchemaVersion: 1, SessionID: "s-mine",
		OracleStatus: "passed", HelpfulEvidenceIDs: []string{releaseID}})
	if r.Presented || r.Tier != store.TierWeak {
		t.Fatalf("other session's delivery attributed to this one: presented %v, tier %q", r.Presented, r.Tier)
	}
}

// A commit made after delivery invalidates the observer state, so the evidence is no longer attributed
// to the session.
func TestNewCommitInvalidatesPresentation(t *testing.T) {
	root := gitFeedbackProject(t)
	const session = "s-commit"
	deliver(t, root, session, "function|release/handler.go|Release")
	observe(t, root, session, "release/handler.go", true)
	if err := os.WriteFile(filepath.Join(root, "queue", "limits.go"), []byte("package queue\n\nconst MaxActiveTickets = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", root, "commit", "-q", "-am", "change")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	r := recordOne(t, root, store.AgentFeedbackReport{SchemaVersion: 1, SessionID: session,
		OracleStatus: "passed", HelpfulEvidenceIDs: []string{releaseID}})
	if r.Presented || r.Tier != store.TierWeak {
		t.Fatalf("state from before a commit still attributes evidence: presented %v, tier %q", r.Presented, r.Tier)
	}
}

// Oracle absent: no outcome, no provenance, weak, and the artifact path needs a real outcome.
func TestMissingOracleIsUnknownAndWeak(t *testing.T) {
	root := gitFeedbackProject(t)
	r := recordOne(t, root, store.AgentFeedbackReport{SchemaVersion: 1, TaskCompleted: true,
		HelpfulEvidenceIDs: []string{releaseID}})
	if r.OracleStatus != store.OracleUnknown || r.Tier != store.TierWeak {
		t.Fatalf("no oracle = %q, %q; want unknown, weak", r.OracleStatus, r.Tier)
	}
}
