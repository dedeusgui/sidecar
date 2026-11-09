# Sidecar V3.5 — implementation roadmap and agent handoff

**Status:** planning, no V3.5 runtime delivered. **Start from latest `main`** and [architecture.md](architecture.md); don't assume the historical baseline SHA is still HEAD. Use [evaluation.md](evaluation.md) for proof gates and [docs/v3/progress.md](../v3/progress.md) for facts. Pre-existing PR #21 is merged; no need to recreate V3.

## Operational contract for the implementation agent

1. Read `AGENTS.md`, `docs/v3.5/README.md`, `architecture.md`, `evaluation.md`, `docs/v3/next-steps.md`, `docs/v3/progress.md`, and the precise code packages being changed. Avoid reading unrelated files wholesale.
2. Examine `git status`, `git log -8`, repository CI, outstanding PRs and branch before changes. Do not overwrite others' work.
3. **Implement, test and measure** a genuinely unified V3.5; do not stop at another architecture file, passive hooks or scaffolding. One small evidence-backed PR at a time. Retain all existing V1/V2/V3 code and behavior unless a scoped compatibility fix is justified.
4. Keep a granular progress/checkpoint in `docs/v3.5/progress.md` (task, code, tests, metrics, regressions, next exact command). This file is to be created by the implementation agent, **not** an assertion of work already done.
5. Respect cost: **no model/API spend without explicit user authorization and a cap**; run deterministic offline validation freely. Do not touch the real final holdout.
6. Reuse existing APIs and avoid inventing `model-aware` magic or unsupported transport. A feature counts as delivered only after functional test of the enabled path; “no CLI knowledge” must be tested, not just described.
7. One source of truth for evidence, one task plan, one global packer. Keep the CLI as a compatible adapter, not a separate engine.
8. Any new idea from CodeGraph, Headroom, RepoMap, Serena or Context7: consult up to two relevant implementations pinned in [V3 references](../v3/upstream-references.md), verify file license/compatibility, link SHA in PR. Reproduce independently, no upstream prompt instructions.

## Stage 0 — lock baselines and gather unbiased tests before tuning

**Why first:** V3 E1 shows an important regression on short domain prompts; existing independent E1 has already been examined. Don't fix it and then report improvements on the same used corpus.

- Snapshot current V2/V3 output, index SHA/extraction versions and latency/cold/warm, hooked+non-hooked.
- Create a **new**, explicitly separate E1 cohort: ≥30 positive short PT-BR/EN domain prompts, ≥40 genuinely unrelated/near-miss negatives in unseen repos/languages; independent author/reviewer whenever possible. Freeze hashes before changing tuning. If authored by agent implementing router, mark as **author-coupled**, not truly independent.
- Freeze an independent E2 validation navigation split before tuning new graph chains; retain old 7 tasks as *development diagnostics* (24/26 CodeGraph, 22/26 Sidecar facts). Favor ≥20 tasks in ≥5 languages with independently verified span/edge gold.
- Instrument candidate ranking/confidence, prompt class, final rendered token count, feature activation/skip and source-cost estimates. No raw prompt/code in product telemetry. Keep `unknown` if server cannot report.
- Validate actual customer-workflow constraints: small repo, huge monorepo, no Git, ignored/vendor trees, missing grammars, untrusted comments, dirty worktree, current user/agent permissions.
- Decide **primary E1/E2/E3 metrics, acceptable quality regressions, expense budget and ablations** before modifying the engine. No paid run without consent.

**Acceptance:** reproducible controls, reference output snapshots and held-out sets with precise provenance and declared limitations. If independence is impossible, proceed with implementation but report lower confidence.

## PR 1 — unify TaskContext, EvidenceRegistry and planning (no behavior changes)

**Goal:** introduce internal contracts behind the opt-in engine and prove **V1/V2/V3 unchanged**.

- `internal/engine`: add planning API (`Plan` from existing V3 decision), phase+observability+budget; keep current gate code callable as-is. Use `store` and `retrieve.Result`; derive canonical evidence IDs and verified/candidate/unknown, not new database/index.
- `internal/brief` and `internal/ctxbuild`: expose structured guidance/evidence items, not only ready-made text. Keep current public Render/TaskItems/Compose contracts.
- `internal/app`: adapter from current input into task/session snapshot, with worktree/version and capability info. Read existing observer/brief state defensively.
- Add a dry-run decision trace that **states what would be included/excluded** with deterministic ordering and no content leakage.
- Property tests: same input/index state -> same decision; no false VERIFIED; no mixed worktrees; empty/no repo; budget 0; malicious comments; concurrent index write.
- Legacy snapshots: absent `SIDECAR_ENGINE=v3.5` => same hooks, flags, output and CLI semantics.

**Acceptance:** all tests/lint, no regression in offline E1 V1/V2/V3, opt-in plumbing present but no activation change. No feature duplication.

## PR 2 — one global packer + hybrid MicroBrief

**Goal:** produce **Brief together with minimal selected code and tests**, not replace all sections, and reserve one global token budget.

- `internal/brief/task.go::Build` stays as evidence-backed source; introduce `MicroBriefProposal` and matching code/test/contract proposals.
- `internal/ctxbuild`: item priority, source fingerprint/role, global token allocation including marker and heading overhead, deterministic tie-break, truncation with qualifier/provenance preserved, emitted keys.
- `internal/app`: collect proposals **before** packing; remove `post-pack budget expansion` **only for V3.5**, keeping legacy behavior under V1/V2/V3 until version migration. Proposals from Memory/Groom/Impact share allocation.
- Multi-surface output must not duplicate the same fact in Brief/CODE/CONTRACTS. Guarantee at least one *actual* relevant code snippet on edit/complex tasks when available, not only an outline or speculative path.
- Test LEAN/BRIEF/VERIFY across edited/no-edit/greeting; combined output under budget; TaskCheck not crowded out by a low-value Git item; safety escaping/markers, output length and session dedup.
- Baseline arms: V3 vs V3.5 without Brief / with MicroBrief / with hybrid code. Measure hallucinated confidence and E1+E2 offline fact coverage, not just total bytes.

**Acceptance:** a unit/integration test proves Brief+code coexist, strict shared budget, no duplicates, no changed legacy output, no E1 quality regression. No assertion of model savings yet.

## PR 3 — shared Explore core + automatic QuickExpand

**Goal:** Sidecar helps without the agent invoking `sidecar explore`, while keeping the advanced CLI functional.

- Refactor `internal/explore/explore.go` so `QuickExpand` and `DeepExplore` use one read-only traversal, index, ranked seed set, source windows, freshness and provenance. Keep CLI `sidecar-explore/1` JSON/text compatible; version explicitly if breaking it.
- Add optional **AUTO** path for tasks that need multi-hop navigation, complex changes, verified contract neighbors or demonstrable low sufficiency, with strict hook time and token budgets. No background daemon. Fallback to base retrieval if index stale/timeout/edges unknown.
- Fix Monolog `handle`/`write`, Sinatra `route!`, Click `parse_args` as development cases, and test on truly unseen validation repos. Do not stuff entire class trees; prefer high-value call chain/source.
- Integrate discovery without teaching CLI commands: on supported prompts the initial context already contains the minimal route; optional native tool/CLI remains available, advertised only when actually invocable. Document exactly which clients support what, including Codex/Antigravity limitations.
- Test `QuickExpand` absent/disabled/timeout/empty, `DeepExplore` CLI old/new parity, deterministic output, cold/reindex, lines/Windows paths, dynamic dispatch vs CANDIDATE, strict compute budget.
- E2 offline: facts present AND bytes per correct fact. E2 agentic only when authorized: correctness, extra searches, calls, cost per success. Compare against CodeGraph matched arm without contamination.

**Acceptance:** no user/agent-specific command required to get navigation assistance, CLI remains optional and compatible, no unbounded hook work. No automatic deeper search on off-topic/simple prompts.

## PR 4 — Task Guidance unified: Groom, Impact, Memory, Session

**Goal:** make these features a **phase-aware proposal system** rather than separate append-only text generators.

- `Groom`: produce one grounded open question only when alternative materially changes behavior, user answer unavailable from repo and client may interact. Keep TTL, pending-answer semantics and noninteractive CI policy. If client cannot ask, propose safe assumption+uncertainty only when justified; no auto answer or repeated question.
- `Impact`: on **actual uncommitted edits** or strong change relevance, propose verified affected contracts/tests/consumers; label scope and candidate edges; never assert tests executed just because a path is mentioned. No unrelated impact note at first prompt.
- `Memory`: explicit decisions bound to file hash/TTL/session are the highest-confidence relevant context; do not contaminate unrelated tasks. Invalidate on changed worktree where existing contracts require.
- `Session`: consumed/updated from passive observed events. On verified duplicate search, make future evidence more directly sufficient; after PreCompact, allow relevant evidence again; after actual failed test emphasize error/verification. Don't pretend the observer can interrupt agent mid-turn.
- Tie all proposals into the **same plan and packer**, enforcing one outgoing context and budget. Avoid feature-specific budgets that force overrun. Failure of one module must fail open for whole hook.
- Test multi-turn with pending questions, changed files, stale index, no observer events, compaction, already-present decisions, conflicting docs, invalid tool events and malicious repo text.
- Bench ablations: `V3.5 minimal`, `+microbrief`, `+autoexplore`, `+groom`, `+impact`, `+session`, `full combined` (factor selectively, not all combinations in paid suite). A feature must have nonnegative marginal utility for its eligible task class.

**Acceptance:** no duplicated instruction, no nonsensical question/verification, end-to-end integration tests for actual hooks/events, no regression on valid legacy flows.

## PR 5 — readiness, surface UX, agentic evaluation and release gates

- Add opt-in `SIDECAR_ENGINE=v3.5` after earlier PRs: install/doctor/explain/status and CLI should state version accurately; V1 remains default and V2/V3 are accessible. Do not confuse `v3.5` with a currently available flag before implementation.
- Validate real adapters individually (Claude Code, Codex, Antigravity): automatic prompt-time injection, optional tool path, observe/precompact event capability, fail-open, install/uninstall, no reconfiguration per repository. Support matrix with **unsupported/unverified** explicit.
- Measure hooks and explore: p50/p95/p99, startup/cold index/reindex, non-repo, small/medium/large repo, concurrent sessions, corrupt cache/locked DB, Windows/macOS/Linux where supported.
- Offline E1/E2 with frozen validation; agentic E2/E3/E4/E5 **only after credentials and spending cap authorized**, same tasks/commits/model/permits; cost per correct outcome, CI clustered by repo/task. No silent model alias substitution.
- Freeze code and budgets, then one final private holdout with strong filesystem isolation and independent oracle, only when explicitly approved. Label weak/absent evidence as unknown, not success.
- Promote only if quality noninferior, real economic/time gain and no safety regression. Otherwise keep V3.5 opt-in and report exactly where it loses.
- Update README, usage, docs/benchmarks through registry, `docs/v3.5/progress.md`, changelog and release/rollback docs; migrate only after all gates.

**Acceptance:** automated acceptance tests + reproducible outcomes + compatibility matrix. A complete toolchain and green CI alone are not agentic proof.

## Cross-cutting checklist for every PR

- [ ] Does a regular prompt get useful context **without** a Sidecar command or a prompt-embedded tutorial?
- [ ] Is the output actionable but minimal, without repeating Brief facts under CODE?
- [ ] Does **one** planner select providers and **one** packer pay for all emitted bytes?
- [ ] Are source labels, freshness, confidence and “not executed” preserved?
- [ ] Is any telemetry/model cost unknown where the source didn't supply it?
- [ ] Do legacy engines retain current behavior with regression coverage?
- [ ] Can the module fail without breaking the agent's task?
- [ ] Are the new benchmark results matched, honest and not tuned to holdout?
- [ ] Have package tests, race subset, lint, cross-build and deterministic eval run?
- [ ] Does the PR state alternative, rollback, counterexamples and exactly what remains unverified?

## Validation commands / safe defaults

```bash
go test -count=1 ./...
go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/
python3 -m unittest bench.lab.test_lab bench.lab.test_analyze bench.lab.test_sufficiency
python3 bench/consolidate.py --check
go run ./cmd/sidecar doctor
# Inspect bench/v3/*.py help before running offline suites; pin external repos and tool versions.
# Paid E2/E3/E4/E5: NOT authorized by this document; require explicit separate spending approval.
```

**Recommended first agent task:** create the new E1 and E2 evaluation cohort BEFORE tuning, capture baseline snapshots, then ship PR 1+2 (shared TaskContext and hybrid Brief/packer). The user’s desired experience is “ask normally, get the right context”; don't jump straight to adding tool adapters or a large auto-explore payload.

## Brief handoff prompt

> Implement the Sidecar V3.5 Unified Context Engine according to `docs/v3.5/architecture.md`, `implementation-plan.md` and `evaluation.md`, beginning from the latest main and preserving V1/V2/V3. Prioritize automatic, efficient context with a hybrid Task Brief plus selected actual code, shared evidence/provenance, bounded automatic structural navigation and one global packer; `sidecar explore` remains optional. Create a fresh independent E1/E2 cohort before gate/graph tuning. Implement in incremental reviewed PRs, update `docs/v3.5/progress.md`, verify tests and regressions, do not run paid agentic experiments or sealed holdout without authorization. Never claim that V3.5 has been shipped or beaten CodeGraph until same-task, correct-outcome benchmarks prove it.