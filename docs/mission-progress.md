# Registro de progresso da missão R1–R7

Versionado; reconstruir o estado por Git + testes + este arquivo, não por memória de sessão. Fonte de requisitos: [roadmap-next-iterations.md](roadmap-next-iterations.md). **Atualização:** 2026-11-08. Base: `main` @ `576c68c` (PR #6). Legenda: ✅ feito e testado · 🟡 parcial · ⬜ pendente · ⛔ bloqueio externo.

## Matriz executável (auditada no HEAD 576c68c)

| Entrega | Existia antes | Lacuna real | Teste de aceite | Estado |
|---|---|---|---|---|
| R1.1 corpus 20–30 tarefas de edição, splits, holdout novo | 1 tarefa chi (`run_edit_pilot.py`); 10 repos só de localização | corpus verificável + oráculos + splits | `lab.py validate-corpus` ok em todas | 🟡 ver §Corpus |
| R1.2 harness de edição isolado | piloto de 1 tarefa, Codex+bwrap | harness genérico, worktree descartável, escopo, braços balanceados | `python3 -m unittest bench.lab.test_lab` (23 testes) | ✅ (isolamento de rede do agente: ver eval-lab §6) |
| R1.3 replay offline versionado + ablação + rollback | `compare_variants.py`, ablações em `sidecar-eval` | `Tuning`, grade, determinismo, run key, `--replay-check` | `go test ./cmd/sidecar-eval ./internal/retrieve`; replay determinístico | ✅ |
| R1.4 perf p50/p95/p99 cold/warm por estágio | `hook_probe_ms` apenas | `--perf` por estágio | `sidecar-eval --perf` | ✅ (cold n pequeno) |
| R1.5 ROI do feedback (3 braços) | feedback com integridade (PR #6) | arms + semeadura por raiz estável | — | ⬜ |
| R1.6 scorecard unificado | `bench/scorecard.py` (OFF/ON) | seções por evidência, pareado, custo/correta | `lab.py report` + testes | ✅ |
| R1.7 comparação OFF/V1/V2/componentes | OFF/ON | braços V1, V2; componentes dependem de R2–R5 | `lab.py run --arms OFF,V1,V2` | 🟡 |
| R2 Adaptive Context Engine | V2 com abstenção parcial | política por intenção, E17, termos genéricos | gates R2 | ⬜ |
| R3 Task Evidence Brief | `ContractEvidence`, packer | brief rastreável | gate R3 | ⬜ |
| R4 TaskUncertainty / grooming | — | detector, política, decisões, benchmark | gate R4 | ⬜ |
| R5 Impact/Verification | `ContractEvidence` | blast radius, verificações | gate R5 | ⬜ |
| R6 Session Intelligence | `SessionObserver` passivo | traces, estagnação | gate R6 | ⬜ |
| R7 Studio/adapters/release | docs de integração | estático, matriz de suporte | gate R7 | ⬜ |

## Corpus (R1.1)

Preenchido a cada integração (contagem real de `lab.py manifest`).

## Testes realmente executados

- Base `576c68c`: `go build/vet/test ./...` verdes (2026-11-08).
- `python3 -m unittest bench.lab.test_lab`: 23 ok.
- `go test ./cmd/sidecar-eval ./internal/retrieve` com `Tuning`/replay: ok; `sidecar-eval --replay` sobre `bench/cases.json`: determinístico, 7 variantes (fixtures internas: só sanidade, **não** usadas para escolher parâmetros).
- `sidecar-eval --perf 20` (fixtures internas, in-process): V1 warm total p50 3,5 ms / p95 6,4 ms; V2 warm total p50 5,6 ms / p95 11,2 ms (n=1489 chamadas por engine; cold n=2).

## Gastos de modelo

US$ 0,00 nesta missão até aqui (teto de US$ 10 não consumido).

## Bloqueios e riscos conhecidos

- ⛔ Sem `bwrap` no ambiente cloud: oráculo isolado de rede por `unshare -rn`; agente `claude` sem sandbox de SO.
- ⛔ monolog (MON-*): `composer install` falha — o proxy da sessão devolve 403 para api.github.com/codeload.github.com, então PHPUnit não instala; 3 tarefas pendentes (corpus alvo cai de 24 para 21). Sem contorno por outro host.
- Corpus escrito por subagentes + mesmo time do produto: viés de autoria registrado.

## Próxima ação concreta

Integrar o corpus validado, abrir o PR de R1, rodar piloto OFF/V1/V2 em desenvolvimento com teto de gasto e começar R2.
