# Sidecar V3 — progresso de implementação

Atualizado em 2026-11-09. Baseline: `main@d15aedb` (PR #19 documentação V3 e PR #20 laboratório P0-A **já mergeados**; CI da `main` verde). Branch de trabalho `ccr-8eafd1e3-wdjnbb`. **Não há merge nem execução paga.** Custo pago acumulado nesta campanha: US$ 0.

## Estado por entrega

| Entrega | Estado | Evidência / limite |
|---|---|---|
| P0-A laboratório (PR #20) | Mergeado; **auditado com sandbox real** nesta sessão | ver [eval-lab.md §5d](../eval-lab.md): 4 falhas reproduzidas e corrigidas; volta autenticada, `--resume` e contadores reais **não verificados** (sem credencial/orçamento) |
| P0-B Relevance Gate + Effort Router | Não iniciado neste checkpoint | próximo passo |
| P1 `sidecar explore` | Não iniciado | |
| Context Sufficiency | Não iniciado | `internal/progress` não conhece threads; o harness agora marca `context_scope` |
| Brief/Groom/Impact/Session sob o router | Não iniciado | |
| Suítes E0–E5 competitivas | Não iniciado | CodeGraph não instalado neste ambiente |

## Checkpoint P0-A (auditoria 2026-11-09)

- Reproduzido com `bwrap` e `claude` reais (sem custo): suítes de Click, Express e Sinatra **não rodavam** no sandbox do agente; sidecar/codegraph eram executáveis por caminho absoluto no braço OFF; `message_id` usava o `uuid` da linha do stream; contexto do hook era atribuído a subagentes.
- Corrigido com testes de regressão (mutação confirmou que falham sem a correção). `python3 -m unittest bench.lab.test_lab bench.lab.test_analyze`: 92 testes OK.
- Canal residual **declarado, não fechado**: rede aberta no sandbox + corpus público no GitHub. Mitigação = detecção e invalidação, não bloqueio. Para o holdout, o protocolo continua exigindo gabarito fora do alcance (avaliador independente) antes de qualquer alegação.
- Nunca aberto: holdout (`SIN-EDIT-01` e demais). Probes usaram apenas tarefas de development/validation.

## Próximo passo exato

P0-B: gate determinístico opt-in `SIDECAR_ENGINE=v3` sobre `internal/retrieve`, com E1 V2×V3.
