# Diagnóstico de falhas (automático, hipóteses)

Fonte: `bench/results/real-hook-pilot-isolated-2026-11-08/runs.jsonl` · execuções: 10 · falhas: 1

Classes: experiment_violation (edição ou contaminação: não é falha do agente), infra_failure, oracle_failure (resposta ilegível pelo oráculo), retrieval_failure (gold fora do contexto entregue), task_failure (resposta errada com o gold disponível), false_positive, false_positive_injection. Causas são candidatas para revisão, não verdades; `explain` mostra registros do ranking, não causalidade.

| Execução | Modelo | Categoria | Sucesso | Classes | Respostas | Gold no contexto | Linha |
|---|---|---|---|---|---|---|---|
| OFF/H01-weight/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/feedback.go, internal/app/feedback.go, internal/app/feedback_oracle_test.go, internal/store/feedback_test.go, internal/app/oracle.go | None | 1 |
| ON/H01-weight/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/feedback.go, internal/store/feedback_test.go, internal/app/feedback_oracle_test.go | True | 2 |
| ON/H02-migration/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/store.go, internal/store/feedback.go, internal/store/feedback_test.go, internal/store/migration_test.go | True | 3 |
| OFF/H02-migration/rep0 | claude-haiku-5-5 | direct | True | — | internal/store/store.go, internal/store/feedback.go, internal/store/migration_test.go, internal/store/feedback_test.go, docs/agent-feedback.md | None | 4 |
| OFF/H03-trace/rep0 | claude-haiku-5-5 | messy_pt | True | — | internal/app/feedback.go, internal/app/observer.go, internal/adapter/tool.go, internal/brief/session.go, internal/store/feedback.go | None | 5 |
| ON/H03-trace/rep0 | claude-haiku-5-5 | messy_pt | True | — | internal/app/feedback.go, internal/app/observer.go, internal/brief/session.go, internal/store/feedback.go, internal/app/feedback_test.go | True | 6 |
| ON/H04-model/rep0 | claude-haiku-5-5 | messy_pt | True | — | bench/agent/run_agents.py, bench/agentic/real_hook_pilot.py, bench/agent/test_run_agents.py, bench/agentic/test_real_hook_pilot.py, docs/handoff-agent-benchmark.md | False | 7 |
| OFF/H04-model/rep0 | claude-haiku-5-5 | messy_pt | True | — | bench/agent/run_agents.py, bench/agent/test_run_agents.py, docs/eval-audit-2026-11-08.md, bench/agentic/test_real_hook_pilot.py, docs/handoff-agent-benchmark.md | None | 8 |
| OFF/N01-negative/rep0 | claude-haiku-5-5 | negative | True | — | — | None | 9 |
| ON/N01-negative/rep0 | claude-haiku-5-5 | negative | False | false_positive_injection, false_positive | internal/retrieve/abstention_test.go, bench/negatives/hard_negatives.json, bench/negatives/hard_negatives_external.json, docs/agent-pilot-2026-11-08.md, docs/eval-audit-2026-11-08.md | None | 10 |

## Explain associado (candidatos e posição do gold)

### ON/N01-negative/rep0 — classes: false_positive_injection, false_positive
- intenção: ['general'] (confiança 0); candidatos: 15; entregues: 8; orçamento: 1500 tokens; estimado: 282 tokens
- posição do gold entre candidatos: ausente
  - não-gold entre os 5 primeiros: `internal/retrieve/voice_mode_test.go#constant:naturalSpokenPrompt@98` score 0.03278688524590164, cobertura 1, exato False, razão: own text matches
  - não-gold entre os 5 primeiros: `internal/retrieve/voice_mode_test.go#function:TestUserNaturalSpokenPortugueseVoicePrompt@96` score 0.03225806451612903, cobertura 1, exato False, razão: own text matches
  - não-gold entre os 5 primeiros: `internal/lexical/lexical.go#file:lexical.go@1` score 0.031746031746031744, cobertura 1, exato False, razão: own text matches
  - não-gold entre os 5 primeiros: `internal/retrieve/retrieve.go#file:retrieve.go@1` score 0.03125, cobertura 1, exato False, razão: own text matches
  - não-gold entre os 5 primeiros: `internal/retrieve/abstention_test.go#function:abstentionFixture@15` score 0.03076923076923077, cobertura 1, exato False, razão: own text matches
- latência por fase (ms): {'analyze': 1.179, 'contract_pack': 15.396, 'exact': 0.504, 'fingerprint': 7.536, 'fusion': 0.107, 'lexical': 0.458, 'structural': 3.727, 'total': 21.537}

## Casos negativos não limpos

O texto do prompt aparece no checkout; nesses casos a recuperação pode casar com a própria pergunta:

- OFF/N01-negative/rep0: bench/agentic/real_hook_pilot_cases.json, bench/negatives/hard_negatives.json, bench/negatives/hard_negatives_external.json, docs/agent-pilot-2026-11-08.md, internal/retrieve/abstention_test.go
- ON/N01-negative/rep0: bench/agentic/real_hook_pilot_cases.json, bench/negatives/hard_negatives.json, bench/negatives/hard_negatives_external.json, docs/agent-pilot-2026-11-08.md, internal/retrieve/abstention_test.go

