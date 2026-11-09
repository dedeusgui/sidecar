# Sidecar V3 P0 Lab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make lab measurements conservative, conditional, attributable, and protected from oracle access before tuning V3.

**Architecture:** Keep the current Python lab and Go Sidecar runtime boundaries. Fix turn aggregation, clarification delivery, per-repository test permissions, OS filesystem isolation, and trace/usage capture inside `bench/lab` and its existing Claude stream parser. Scripted runs remain infrastructure evidence; agentic execution fails closed when filesystem isolation is unavailable.

**Tech Stack:** Python 3 stdlib, existing Claude Code JSONL stream, Linux `bwrap` when available, existing Go CLI.

**Spec:** `docs/v3/CODEX-HANDOFF.md` section A and `docs/v3/evaluation-protocol.md` E0.

## Global Constraints

- Do not spend paid model credits or change billing.
- Do not read or execute holdout tasks; do not alter oracle files after observing agent responses.
- Keep V1/V2 defaults and feature flags unchanged.
- Missing measurements stay unknown; simulated output never counts as agentic evidence.
- The agent can write only in its disposable checkout and per-run temporary state; OFF cannot discover Sidecar or CodeGraph through the sandbox PATH.

## Review Focus

- A missing turn cost or token counter must make the merged total unknown; test both missing-turn orders and timeout/budget records.
- An ambiguous-task answer must be delivered only after a relevant, pre-edit question; test no question, useful, irrelevant, duplicate, absent-tool, and expired-session cases.
- A child process must not read oracle/reference paths by absolute path or constructed path; test `bwrap` against a host-only sentinel.
- Test runners must be available for Go, Python, JavaScript, Ruby, Rust, Java, and PHP tasks with equal permissions across arms; test repository-to-allowlist mapping.
- Tool outcomes need stable order, thread/subagent identity, and correlated errors; parallel calls emitted before context delivery must not count as post-context navigation.

---

### Task 1: Conservative turn aggregation

**Files:**
- Modify: `bench/lab/lab.py::merge_turns`, `summarize`
- Test: `bench/lab/test_lab.py::TwoTurnTests`

**Interfaces:** Keep the current `merge_turns(turns) -> dict` shape and scorecard fields; preserve optionality for all aggregate counters.

- [ ] Add regressions for one-turn unknown cost, a known+unknown two-turn pair in both orders, unknown tokens/tool calls, timeout, and budget accounting.
- [ ] Run the focused tests and verify the new regressions fail against the current partial-sum behavior.
- [ ] Make every aggregate numeric field unknown when any constituent turn is unknown; keep `cost_per_success` unknown if any comparable run has unknown cost.
- [ ] Run `python3 -m unittest bench.lab.test_lab bench.lab.test_analyze`.

### Task 2: Conditional clarification protocol

**Files:**
- Modify: `bench/lab/lab.py::followup_for`, `ClaudeAgent.run`
- Test: `bench/lab/test_lab.py::TwoTurnTests`

**Interfaces:** `followup_for(task, first_turn=None) -> str`; add a pure predicate for recognizing a relevant clarification from a successful AskUserQuestion event or a pre-edit assistant question grounded in task options.

- [ ] Add tests for useful, ignored, irrelevant, duplicate, unavailable, and expired questions; assert that an unasked decision is never in the second prompt.
- [ ] Implement one neutral follow-up for all arms unless the first turn contains a relevant clarification; deliver the canned answer at most once.
- [ ] Record only clarification status/reason, not raw prompt or question text, in run records.
- [ ] Run focused tests and the full Python lab tests.

### Task 3: Edit-intent negation regressions

**Files:**
- Modify: `internal/retrieve/policy.go::IsEditRequest`
- Test: `internal/retrieve/policy_test.go`, `internal/groom/groom_test.go`

**Interfaces:** Keep `IsEditRequest(prompt string) bool` and `groom.Analyze`/`groom.Decide` contracts.

- [ ] Add regression cases for multi-clause PT-BR/English prompts, “do not change X; fix Y”, quoted imperative text, and read-only prompts; verify read-only cases do not produce a grooming question.
- [ ] Run focused Go tests and observe the quoted-text regression fail before changing production code.
- [ ] Fix quote/polarity handling with the smallest deterministic change; preserve positive edits in other clauses.
- [ ] Run the focused Go packages.

### Task 4: Repository-specific test permissions

**Files:**
- Modify: `bench/lab/lab.py::ClaudeAgent`, `cmd_run`, CLI defaults
- Test: `bench/lab/test_lab.py::RunnerTests`

**Interfaces:** Add `tools_allow_for(task, requested)` returning a stable list; derive standard test commands from the pinned repository language while retaining explicit `--tools-allow` overrides.

- [ ] Add table-driven tests for the existing Go, Python, JavaScript, and Ruby fixtures plus Rust, Java, and PHP mappings.
- [ ] Apply the same effective list to every arm for a task and persist the resolved list in run metadata.
- [ ] Run focused tests.

### Task 5: Strong agent filesystem sandbox

**Files:**
- Modify: `bench/lab/lab.py::ClaudeAgent`, `cmd_run`, sandbox helpers
- Test: `bench/lab/test_lab.py::RunnerTests`
- Docs: `docs/eval-lab.md`

**Interfaces:** Add `agent_sandbox_command(argv, work, readonly_paths, writable_paths) -> (argv, isolation)`; return an unavailable result rather than starting an agent when Linux filesystem isolation cannot be established.

- [ ] Add an adversarial test proving a child process cannot read a sentinel outside its mounted checkout.
- [ ] Add preflight tests proving the sandbox hides the benchmark root and, for OFF, does not expose Sidecar/CodeGraph binaries.
- [ ] Bind only system runtimes, the disposable checkout, isolated config/state, and the Sidecar binary for enabled arms; do not bind `/`, benchmark corpus, output directory, or user config.
- [ ] Fail closed for paid agent runs when this sandbox is unavailable; preserve scripted infrastructure tests without paid runs.
- [ ] Record agent filesystem isolation separately from oracle network isolation; correct the lab documentation.
- [ ] Run focused tests and the full Python lab tests.

### Task 6: Ordered, correlated tool traces

**Files:**
- Modify: `bench/lab/lab.py::tool_events`, `progress_report`
- Test: `bench/lab/test_lab.py::ToolTraceTests`

**Interfaces:** Each event carries `order`, `thread_id`, `message_id`, `tool_use_id`, optional source timestamp, and completion/error fields; `tool_events(stream_text)` remains ordered.

- [ ] Add tests for subagent threads, multiple calls in one message, matching tool results by ID, failures, and calls before/after the actual hook response.
- [ ] Correlate results with their tool call; stop emitting unlinked generic error events.
- [ ] Mark post-context actions only when the stream proves context was delivered first; preserve `unobserved` when transport data is absent.
- [ ] Run focused tests.

### Task 7: Optional cost and timing attribution

**Files:**
- Modify: `bench/agentic/real_hook_pilot.py::parse_stream`, `bench/lab/lab.py::merge_turns`, `execute_one`, `summarize`, `render_md`
- Test: `bench/lab/test_lab.py`, parser tests in `bench/agentic`

**Interfaces:** Add optional fields `input_uncached`, `cache_read`, `cache_create`, `output`, `tool_result_tokens`, `hook_ms`, `index_ms`, `sync_ms`, `retrieve_ms`, `resident_context_tokens`; absent source counters remain `null`.

- [ ] Add parser fixtures for present, zero, missing, and malformed usage fields.
- [ ] Capture Sidecar hook stage timings from metadata-only local hook logs; do not persist prompt/code or infer tool-result/resident tokens.
- [ ] Preserve old scorecards when fields are absent and expose new columns only when measured.
- [ ] Run focused tests and `python3 bench/consolidate.py --check`.

### Task 8: Checkpoint and commit P0

**Files:**
- Create/update: `docs/v3/progress.md`
- Update: `docs/eval-lab.md`, `docs/benchmarks-2026-11-08.md` only for methodological corrections/results

- [ ] Run Python tests, deterministic lab validation on development/validation only, corpus consolidation checks, and available Go tests; do not access holdout.
- [ ] Review the complete diff for oracle paths, data exposure, false zero counters, compatibility, and sandbox fail-open paths.
- [ ] Record evidence, unavailable tools, unrun paid/holdout experiments, rollback, and the exact next command in `docs/v3/progress.md`.
- [ ] Commit P0-A separately and prepare an auditable PR against `docs/v3-codex-engineering-handoff`.
