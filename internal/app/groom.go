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
	"github.com/dedeusgui/sidecar/internal/gitx"
	"github.com/dedeusgui/sidecar/internal/groom"
	"github.com/dedeusgui/sidecar/internal/retrieve"
	"github.com/dedeusgui/sidecar/internal/store"
)

// AnswerDecisionTTL is how long a user's answer to a clarification stays valid. It is short on purpose:
// the answer settles one task, not a standing rule. A changed file or an expired TTL invalidates it.
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
		res.Trace.Budget += ctxbuild.EstimateTokens(text) + 8
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
	statePath := opts.StatePath(string(opts.Agent), in.SessionID)
	if statePath == "" {
		return false
	}
	return groom.LoadState(statePath+".groom.json").Pending != nil
}
