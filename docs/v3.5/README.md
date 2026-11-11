# Sidecar V3.5 — Unified Context Engine

**Status: implementada, opt-in (`SIDECAR_ENGINE=v3.5`), gates de avaliação pendentes; ver [progress.md](progress.md).** Design branch from `main@16aa8f68187bf0de9781f2d0a333ffde91713b90` (PR #21 is the implemented experimental **V3**). V3.5 will remain opt-in until benchmark validation and release gates pass.

The design and implementation sections below preserve the original proposal and may describe targets rather than measured behavior. For later offline and agent experiments, see [progress.md](progress.md) and the [campaign report](../../bench/results/v35-campaign-2026-11-10/relatorio.md).

**North star:** “Install once, ask normally, get the smallest **sufficient** context automatically.” Every feature already built stays available, coordinated by **one planner, one evidence registry, one global packer, one bounded session view**. The agent is not required to know or use `sidecar explore` or any Sidecar-specific command.

## Core documents

- [Architecture, integration contracts, target data flow](architecture.md): **source of truth for the planned engine**. Includes current code gaps, TaskContext/Phase, hybrid Brief + code, automatic structural expansion, optional CLI, Groom/Impact/Memory/Session, global budget, failure and privacy semantics.
- [Implementation plan and agent handoff](implementation-plan.md): **PR 0–5** with dependencies, test cases, migration policy, rollback and runnable safe checks; includes an implementer handoff prompt.
- [Evaluation and promotion gates](evaluation.md): E0–E6, baseline limitations, no-tool-learning tests, feature ablations, correctness/cost and separate model/holdout authorization.

## What changes / what does not

| Dimension | V3 today | Proposed V3.5 |
|---|---|---|
| Context arrival | Prompt hook automatically delivers V3 retrieval | **Same invisible hook**; better task/phase-aware selection |
| Brief | Opt-in; may replace conventional evidence | **Compact orientation alongside high-value actual code**, one budget |
| Explore | Optional separate CLI, hint given in hook | Shared read-only service; **bounded automatic QuickExpand** plus optional CLI DeepExplore |
| Groom/Impact | Independent optional appenders | **Evidence-backed guidance proposals**, selected at proper task phase before global packing |
| Session/Memory | Passive observer + separate sent-item/decision state | One coherent task snapshot and bounded invalidation policy, still passive where required |
| Cost | Feature-specific overhead can accumulate | Single allocation budget, deterministic limits, transparent measured savings/losses |
| Compatibility | V1 default, V2 and V3 opt-in | Preserve legacy behavior; **V3.5 opt-in only after implementation** |

## First implementation action

**Don't start by adding new integrations or retuning the already-consumed E1 evaluation.** First freeze a new independent E1 cohort and new E2 navigation validation, then build the shared TaskContext + evidence ledger and a hybrid MicroBrief/packer. Only then add automatic QuickExpand and integrate the other guidance modules.

The current V3 **has documented regressions** on short domain prompts (internal Hit@5 67.5% V2 → 57.5% V3), and Explore is 22/26 facts offline vs CodeGraph 24/26. V3.5 must address these honestly; no net efficiency with real agents has been demonstrated so far.

For current delivered functionality and measured results, see [V3 progress](../v3/progress.md), [V3 next steps](../v3/next-steps.md), and [benchmark registry](../benchmarks.md).