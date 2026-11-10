package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dedeusgui/sidecar/internal/adapter"
	"github.com/dedeusgui/sidecar/internal/ctxbuild"
	"github.com/dedeusgui/sidecar/internal/engine"
	"github.com/dedeusgui/sidecar/internal/engine/contract"
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/groom"
	"github.com/dedeusgui/sidecar/internal/repo"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// AnswerDecisionTTL is how long a user's answer to a clarification stays valid. It is short on purpose:
// the answer settles one task, not a standing rule. A changed file or an expired TTL invalidates it.
// groomFrameTokens is the block frame (markers and preamble) that a note needs when there is no other evidence.
const groomFrameTokens = 48

const AnswerDecisionTTL = 48 * time.Hour

// groomNonInteractive reports a session where nobody can answer a question (CI, cloud automation).
func groomNonInteractive() bool {
	return os.Getenv("SIDECAR_ASK") == "never" || os.Getenv("CI") == "true"
}

// applyGroom runs the clarification policy (feature "groom") and attaches at most one note to res. It
// also turns an unambiguous answer to the previous question into a decision bound to the session and the
// file hash. Every failure is returned for the log and leaves the prompt untouched (fail open).
func applyGroom(st *store.Store, root, statePath string, agent adapter.Agent, session, prompt string, res *retrieve.Result, now time.Time, rec map[string]any) error {
	gpath := ""
	if statePath != "" {
		gpath = statePath + ".groom.json"
	}
	state := groom.LoadState(gpath)
	state.Turn++
	if p := state.Pending; p != nil {
		switch {
		case state.Turn-p.Turn > groom.PendingTurns:
			state.Pending = nil
		default:
			if i := groom.MatchAnswer(prompt, p.Options); i >= 0 {
				if err := recordAnswer(st, root, session, *p, p.Options[i], now); err != nil {
					rec["groom_answer_error"] = err.Error()
				} else {
					rec["groom_answered"] = true
				}
				state.Pending = nil
				groom.SaveState(gpath, state)
				return nil // the answer is not a new task: no further question this turn
			}
		}
	}
	rep := groom.Analyze(prompt, *res)
	pol := groom.DefaultPolicy()
	pol.NonInteractive = groomNonInteractive()
	d := groom.Decide(rep, prompt, state.Asked, pol)
	if p := state.Pending; p != nil && d.Gap != nil && p.Decision == d.Gap.Decision {
		// The same question is still waiting for its answer: do not repeat it and do not spend the session quota.
		rec["groom_action"] = "pending"
		groom.SaveState(gpath, state)
		return nil
	}
	rec["groom_states"] = rep.States
	rec["groom_action"] = d.Action
	if d.Action != groom.None && d.Gap != nil {
		text := groom.Render(agent, d)
		fp := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))[:16]
		res.Evidence = append(res.Evidence, retrieve.Evidence{
			Hit:     store.SymbolHit{Kind: "task_check", Name: string(d.Gap.State), FileHash: fp},
			Verdict: "INFERRED", Section: ctxbuild.TaskCheck, Source: "groom", Confidence: 1,
			ID: "groom|" + fp, Note: text, TokenEstimate: ctxbuild.EstimateTokens(text),
		})
		// The note is allowed beyond the profile budget by its own size plus the block frame (about 40 tokens): without the
		// frame a prompt with no other evidence (budget 0) would drop the question while still spending the quota.
		res.Trace.Budget += ctxbuild.EstimateTokens(text) + groomFrameTokens
		if d.Action == groom.Ask {
			state.Asked++
			state.Pending = &groom.Pending{Decision: d.Gap.Decision, Options: d.Gap.Options, Path: primaryPath(*d.Gap, *res), Turn: state.Turn}
		}
	}
	groom.SaveState(gpath, state)
	return nil
}

func primaryPath(g groom.Gap, res retrieve.Result) string {
	for _, ev := range res.Evidence {
		if ev.Note == "" && ev.Hit.Path != "" {
			return ev.Hit.Path
		}
	}
	return ""
}

// recordAnswer stores the chosen option as an explicit decision (origin user_answer) bound to the session
// and the primary evidence file's current hash. Only a deterministic match of an option reaches here.
func recordAnswer(st *store.Store, root, session string, p groom.Pending, chosen groom.Option, now time.Time) error {
	if p.Path == "" {
		return fmt.Errorf("no evidence file to bind the answer to")
	}
	hash, ok := diskHash(root, p.Path)
	if !ok {
		return fmt.Errorf("%s no longer exists", p.Path)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	commit, _ := gitx.Head(ctx, root)
	summary := strings.Join(strings.Fields(fmt.Sprintf("User answer: %s -> %s", p.Decision, chosen.Label)), " ")
	_, err := st.InsertDecision(store.DecisionRecord{RepoRoot: root, Worktree: root, SymbolOrPath: p.Path, FilePath: p.Path,
		CommitHash: commit, FileHash: hash, Summary: summary, Origin: "user_answer", SessionID: session,
		CreatedAt: now.Unix(), TTLSeconds: int64(AnswerDecisionTTL / time.Second)})
	return err
}

// awaitingAnswer reports whether the session has an unanswered clarification, so a short reply such as
// "B" is read as an answer instead of being skipped as a tiny prompt. A slash command is never an answer.
func awaitingAnswer(opts HookOptions, in adapter.Input) bool {
	p := strings.TrimSpace(in.Prompt)
	if p == "" || strings.HasPrefix(p, "/") || in.SessionID == "" || !retrieve.FeatureEnabled("groom") {
		return false
	}
	if os.Getenv("SIDECAR_ENGINE") == engine.Version35 {
		cwd := in.CWD
		if cwd == "" {
			cwd = "."
		}
		root, _ := repo.FindRoot(cwd)
		scope := contract.NormalizeScope(contract.Scope{RepoRoot: root, WorktreeID: root,
			SessionID: in.SessionID, WorktreeRevision: worktreeVersion(root)})
		state := LoadGroomProposalState(scope)
		return state.Pending != nil && state.Turn+1-state.Pending.Turn <= groom.PendingTurns
	}
	statePath := opts.StatePath(string(opts.Agent), in.SessionID)
	if statePath == "" {
		return false
	}
	st := groom.LoadState(statePath + ".groom.json")
	// The pending question expires after PendingTurns prompts; this prompt would be turn st.Turn+1.
	return st.Pending != nil && st.Turn+1-st.Pending.Turn <= groom.PendingTurns
}
