# Sidecar V3.5 — evaluation protocol and promotion gates

**Status:** planned tests, not measured results. Historical source: `docs/v3/progress.md` and `docs/benchmarks.md` (B17–B20). This supplements [the V3 methodology](../v3/evaluation-protocol.md) rather than replacing it. The V3.5 core purpose is **correct task outcomes with less total agent work**, especially for models with limited budgets, not simply context compression.

## 1. Baseline and claims discipline

As of historical `main@16aa8f6`:
- V3 Relevance Gate vs V2 on 180 difficult negative prompts: **30→14 false positives** (16.7%→7.8%). On independent 40 negatives: **13→9** (32.5%→22.5%). **Target of <5% not met.**
- V3 preserves external 10-repo Hit@5 at 97.5% while average positive context drops approximately 418→299 estimated tokens. **It regresses** short domain/internal fixtures Hit@5 **67.5%→57.5%** (empty positives **5→13/40**).
- Navigation facts in 7 offline questions: V2 hook 14/26, V3 hook 13/26, `sidecar explore` **22/26** (38.9k chars), CodeGraph CLI explore **24/26** (114.3k chars). More coverage ≠ successful answer; fewer chars ≠ lower cost.
- Aider RepoMap baseline under “no files in chat” is intentionally nonstandard. It **cannot support** a general claim of superiority over Aider usage with chat-selected files.
- V3 sandbox/hook client setup has **not** demonstrated an authenticated, paid full-agent end-to-end improvement. All V3.5 claims remain **unverified** until matched campaigns.

**No blind holdout reuse.** E1 “independent” 70 cases already examined; never treat them as a new untouched holdout. The final editing holdout of 9 tasks remains gated under the existing lab protocol.

## 2. The core questions for V3.5

**Q1: correct relevance?** Does it deliver evidence for short domain tasks that V3 rejected, while abstaining for near-miss/out-of-repo prompts?

**Q2: enough information without a proprietary command?** Does **Auto** (regular hook) let the agent correctly answer a flow or edit with fewer external Read/Grep/Bash calls than V3-hook/OFF/CodeGraph controls?

**Q3: useful hybrid Brief?** Does a 2–4 line task orientation **plus concrete code** improve behavior compared with `V3` and `V3.5 no-brief`, at equal budget, without hiding needed facts behind a summary?

**Q4: marginal feature utility?** Does adding Groom, Impact, Memory and session-phase awareness help *eligible* scenarios rather than increasing tokens, spurious questions or regressions elsewhere?

**Q5: total efficiency?** After index, hook overhead, tool schema, cache and failed attempts, what is **cost per correct task** and wall time? Does the change preserve invariants and scope?

## 3. Eval suites with matched controls

| Suite | Controls / candidates | Oracle and primary question | Budget/risks |
|---|---|---|---|
| E0 integrity/compat | V1/V2/V3 prior vs after PR, hook/install/CLI | Same outputs/safety and fail-open; no repo code execution; costs stay `unknown` when missing | No model. Includes cross-OS, cap, sandbox, invalidation. |
| E1 Relevance | V2/V3; V3.5 gate-only; V3.5 unified | Macro-per-repo Hit@5, short domain recall, neg FP, false abstain, tokens and p95 | New external validation **frozen before tuning**; existing suites are dev/regressions. |
| E2 Navigation facts offline | V2/V3 hooks; V3.5 autopilot; `sidecar explore`; CodeGraph pinned CLI/MCP; BM25/Aider RepoMap with caveats | Independently verified facts, code spans, task-complete coverage, **useful facts per delivered 1k chars/tokens**, strict wall/cache | ≥20 questions / ≥5 languages split by repo; no blind tuning. |
| E2 Navigation agentic | OFF, V3 hook, V3.5 **Auto** (no Sidecar tool allowed), V3.5 Auto+optional tool, CodeGraph with matched permissions | **Correct answer** with verifiable citations, sufficiency next-action, tool calls, cost/latency per correct answer | Model API paid only with explicit consent; 4–8 development pilot first. |
| E3 Real code editing | OFF, V2, V2+brief, V3, V3.5 minimal, V3.5 unified | **Hidden behavior tests + contract invariants + non-test scope**, total cost/success | Sandbox/oracle outside agent FS; 32 task corpus is development/validation only, sealed holdout untouched. |
| E4 Clarification/Impact | V3.5 minus one feature vs enabled; multi-turn | Correct final behavior vs hidden user choice; useful question rate, rework and cost | Conditional user simulator: *only* answer if agent asks relevant question before editing. |
| E5 Session | V3 vs V3.5 session enabled/disabled | Correctness after compaction, repeated reads/loops, response latency and cost | Do not interpret “moved_on” as success. |
| E6 Integration/UX | Claude Code, Codex, Antigravity where supported | **Zero tool learning**: standard prompt → useful context; never rely on `sidecar explore` or custom skill; installation/reversal works | Client matrices must show exact real hook support, not generic assertions. |

### Definition of “zero tool learning”

For each agent type under test, run the same task with **no Sidecar skill, no custom instruction, no mention of commands in the user prompt**. Primary arm must **not have access** to a Sidecar CLI/tool. The automatic hook alone must provide useful evidence and maintain task success. A second arm may grant `sidecar explore` and should demonstrate additive value. If only the second arm improves, **Autopilot has not met the primary product goal**.

A tool-hint line is itself additional context and a behavioral instruction. Measure it separately; don't count it as proof of invisible use.

## 4. Global packer and feature ablations

Run lightweight deterministic ablations **before** expensive agents:

| Arm | Enabled capabilities | Diagnosis |
|---|---|---|
| `V3` | Current V3 original output | Historical control; same code/index revision |
| `V35-min` | New gate/planner + basic evidence packer | Overhead and retrieval parity |
| `V35-brief` | + MicroBrief + minimal code, no auto Explore | Does Brief help or displace important code? |
| `V35-auto` | + bounded QuickExpand (no CLI tool) | Core invisible-navigation advantage |
| `V35-groom` | + conditional decisions | False question/invented user choice |
| `V35-impact` | + phase/diff-aware verification | Correctness of change surface/test suggestions |
| `V35-session` | + session-aware rediscovery and cache | Repeated reads and PreCompact behavior |
| `V35-full` | All **eligible** capabilities under one budget | Net benefit, unintended interactions |

Pair scenarios by task/repo/seed. Prefer additive/leave-one-out for genuinely active features; do not use a prohibitively large Cartesian product of every switch in paid model runs. Report zero-output cases, dropped high-priority evidence, section duplication, false VERIFIED, stale snippets and injection bytes.

**Global budget assertions**:
- `rendered_estimated_tokens <= allocated_budget` including markers, headings, hints and guidance;
- same fact from Brief+code+Impact appears at most once **unless** distinct source lines are necessary;
- meaningful edit prompts include actual code (if indexed and relevant), not only a vague overview;
- off-topic results in zero;
- unknown or missing data is not upgraded to verified;
- query with a feature disabled does not run its expensive provider;
- per-client unsupported transport produces no bogus “try this command” hint;
- unchanged session evidence is not blindly resent, but actual compaction/file changes invalidate it;
- exceptions in one feature do not prevent the agent continuing.

## 5. Statistical and experimental integrity

- **Primary outcome for editing:** verified correct pass with hidden tests and invariant/scope gates. `cost_per_success = total observed USD for ALL comparable attempts / number of correct successes`, not the mean over successful runs. Unknown cost => unknown aggregate or carefully documented conservative bound. Failures/timeouts count; don't select runs.
- **Primary outcome for navigation:** independently scored correct flow answer; secondary facts retrieved and post-context behavior. Count missing citations and unsupported assertions.
- Source of tokens: API usage counters with distinct uncached/cache-read/cache-write/output; local char-based estimates are **diagnostic only**; resident context and tool-result tokens unknown unless observed.
- Record parser model served ID, tool permissions, skills, hook definitions, versions, index cold/warm, same repo SHA, machine concurrency, costs, wall p50/p95 and tool availability in each arm. Isolate OFF from Sidecar/CodeGraph **including absolute paths**.
- Matched **repo/task-level clustered** confidence intervals, stratify by task class/language/repo size, disclose smallest cell sizes; no statistical claims from 7 navigation questions or 6 two-turn cases.
- Set a minimum detectable effect and power analysis before committing to many paid trials. Pre-register comparisons, budget, exclusion and acceptance. Do not select only favorable repos/models/paths.
- External refs documented at [V3 upstream references](../v3/upstream-references.md) for CodeGraph sufficiency and residual context, Headroom tool schema/cache overhead, ARB selective calibration. Percentages published by other authors use different tasks and cannot be compared directly to ours.

## 6. Quantitative candidates: hypotheses, not guarantees

| Candidate gate | Measurement | Prior baseline |
|---|---|---|
| Recover short-domain recall | V3.5 Hit@5 no worse than V2 on **new** short-domain cases; no material increase in false injection | Internal V2 67.5%; V3 57.5% |
| Improve negative selectivity | New negatives FP markedly below V3, with positive recall intact; seek <5% as a *target*, not forced | V3 14/180 and 9/40 external cohort |
| Autopilot navigation | E2 correct answers at least noninferior to V3 and CodeGraph under comparable arms; target lower calls/cost | E2 offline V3 13/26 vs Explore 22/26 vs CG 24/26 (not agentic) |
| Explore coverage/compactness | Holdout navigation fact coverage at least competitive, source lines valid, bounded output; development target ≥24/26 with ≤50k chars | Sidecar 22/26, 38.9k chars; CodeGraph 24/26, 114.3k |
| Full task efficiency | Aim 10–15% less cost/success or meaningful wall reduction **while maintaining correctness** | B12 and B15 inconclusive |
| Runtime overhead | No material p95 hook regression from V3 on key size tiers; no unbounded filesystem traversal | p95 varies by repo and run; measure anew |
| V3.5 feature gating | No meaningful FP question/test claim/loop intervention regression per eligible task class | No valid E4/E5 agentic gain measured yet |

The percentage targets must be negotiated/registered **before** agentic runs; don't mutate thresholds after seeing results. A change with regression in hidden task correctness cannot be called a win because it delivered fewer tokens.

## 7. Order of evidence and eventual promotion

1. **Now, free:** E0 compatibility, fresh E1/E2 cohorts, a shared context packer and deterministic tests. Mark old E1 independent set as *consumed*.
2. **Then, free:** demonstrate automatic Brief+code and QuickExpand output, cross-language navigation, realistic latency. Do not claim cost savings.
3. **With spend authorization:** a cheap-model paired E2 pilot on development; fix methodology before scale. An E3 patch pilot to test task discrimination.
4. **Later with authorization:** matched Haiku/Sonnet/strong E2/E3 and targeted E4/E5 ablations; report losers and uncertainty.
5. **Last:** freeze code/revision/bin, explicitly approve spend, run the actual isolated final holdout **once**. No tuning afterward on that holdout. Only promote a V3.5 default if safety, compatibility and correct-outcome/cost gates are satisfied.

**Never treat green CI, a good offline E1 or an output-character reduction as sufficient to flip the default to V3.5.**