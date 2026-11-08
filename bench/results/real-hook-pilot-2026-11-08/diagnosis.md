# Diagnóstico de falhas (automático, hipóteses)

Fonte: `bench/results/real-hook-pilot-2026-11-08/runs.jsonl` · execuções: 20 · falhas: 1

Causas são candidatas para revisão, não verdades. Reproduza uma execução com o caso e o braço indicados.

| Execução | Modelo | Categoria | Sucesso | Causas candidatas | Respostas | Gold no contexto | Linha |
|---|---|---|---|---|---|---|---|
| OFF/H01-weight/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/feedback.go, internal/app/feedback.go, internal/app/feedback_oracle_test.go, internal/store/feedback_test.go, docs/agent-feedback.md | None | 1 |
| ON/H01-weight/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/feedback.go, internal/app/feedback.go, internal/app/feedback_oracle_test.go, internal/store/feedback_test.go, internal/store/store.go | True | 2 |
| ON/H01-weight/rep1 | claude-haiku-5-5 | direct | True | — | internal/store/feedback.go, internal/app/feedback.go, internal/app/feedback_oracle_test.go, internal/store/feedback_test.go, cmd/sidecar/main.go | True | 3 |
| OFF/H01-weight/rep1 | claude-haiku-5-5 | direct | True | — | internal/store/feedback.go, internal/app/feedback.go, internal/app/feedback_oracle_test.go, internal/store/feedback_test.go, docs/agent-feedback.md | None | 4 |
| OFF/H02-migration/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/store.go, internal/store/feedback.go, internal/store/feedback_test.go, internal/app/feedback_oracle_test.go, internal/store/migration_test.go | None | 5 |
| ON/H02-migration/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/store.go, internal/store/feedback_test.go, internal/store/feedback.go, internal/store/migration_test.go, internal/app/feedback.go | True | 6 |
| ON/H02-migration/rep1 | claude-haiku-5-5 | direct | True | — | internal/store/store.go, internal/store/feedback.go, internal/store/feedback_test.go, internal/store/migration_test.go | True | 7 |
| OFF/H02-migration/rep1 | claude-haiku-5-5 | direct | True | — | internal/store/store.go, internal/store/feedback.go, internal/app/feedback.go, internal/store/feedback_test.go, internal/store/migration_test.go | None | 8 |
| OFF/H03-trace/rep0 | claude-haiku-5-5 | messy_pt | True | — | internal/app/feedback.go, internal/app/observer.go, internal/adapter/tool.go, docs/agent-feedback.md, internal/store/feedback.go | None | 9 |
| ON/H03-trace/rep0 | claude-haiku-5-5 | messy_pt | True | — | internal/app/feedback.go, internal/app/feedback_test.go, internal/store/feedback.go, docs/agent-feedback.md | True | 10 |
| ON/H03-trace/rep1 | claude-haiku-5-5 | messy_pt | True | — | internal/app/feedback.go, internal/app/observer.go, internal/brief/session.go, internal/store/feedback.go, internal/app/feedback_test.go | True | 11 |
| OFF/H03-trace/rep1 | claude-haiku-5-5 | messy_pt | True | — | internal/app/feedback.go, internal/app/observer.go, internal/adapter/tool.go, internal/store/feedback.go, internal/brief/session.go | None | 12 |
| OFF/H04-model/rep0 | claude-haiku-5-5 | messy_pt | False | wrong_answer | bench/agentic/real_hook_pilot.py, bench/agentic/test_real_hook_pilot.py, bench/agentic/run_edit_pilot.py, internal/benchagent/benchagent.go, docs/evaluation-plan.md | None | 13 |
| ON/H04-model/rep0 | claude-haiku-5-5 | messy_pt | True | — | bench/agentic/real_hook_pilot.py, bench/agent/run_agents.py, bench/agentic/test_real_hook_pilot.py | False | 14 |
| ON/H04-model/rep1 | claude-haiku-5-5 | messy_pt | True | — | bench/agentic/real_hook_pilot.py, bench/agent/run_agents.py, bench/agentic/test_real_hook_pilot.py, bench/agent/test_run_agents.py | False | 15 |
| OFF/H04-model/rep1 | claude-haiku-5-5 | messy_pt | True | — | bench/agent/run_agents.py, bench/agentic/real_hook_pilot.py, bench/agent/test_run_agents.py, bench/agentic/test_real_hook_pilot.py | None | 16 |
| OFF/N01-negative/rep0 | claude-haiku-5-5 | negative | True | — | — | None | 17 |
| ON/N01-negative/rep0 | claude-haiku-5-5 | negative | True | false_positive_injection | — | None | 18 |
| ON/N01-negative/rep1 | claude-haiku-5-5 | negative | True | false_positive_injection | — | None | 19 |
| OFF/N01-negative/rep1 | claude-haiku-5-5 | negative | True | — | — | None | 20 |
