# Registro de progresso da missão R1–R7

Versionado; reconstruir o estado por Git + testes + este arquivo, não por memória de sessão. Fonte de requisitos: [roadmap-next-iterations.md](roadmap-next-iterations.md). **Atualização:** 2026-11-08. Base: `main` @ `576c68c` (PR #6). Legenda: ✅ feito e testado · 🟡 parcial · ⬜ pendente · ⛔ bloqueio externo.

## Matriz executável (auditada no HEAD 576c68c)

| Entrega | Existia antes | Lacuna real | Teste de aceite | Estado |
|---|---|---|---|---|
| R1.1 corpus 20–30 tarefas de edição, splits, holdout novo | 1 tarefa chi (`run_edit_pilot.py`); 10 repos só de localização | corpus verificável + oráculos + splits | `lab.py validate-corpus` ok em todas | ✅ 20/20 revalidadas (§Corpus) |
| R1.2 harness de edição isolado | piloto de 1 tarefa, Codex+bwrap | harness genérico, worktree descartável, escopo, braços balanceados | `python3 -m unittest bench.lab.test_lab` (23 testes) | ✅ (isolamento de rede do agente: ver eval-lab §6) |
| R1.3 replay offline versionado + ablação + rollback | `compare_variants.py`, ablações em `sidecar-eval` | `Tuning`, grade, determinismo, run key, `--replay-check` | `go test ./cmd/sidecar-eval ./internal/retrieve`; replay determinístico | ✅ |
| R1.4 perf p50/p95/p99 cold/warm por estágio | `hook_probe_ms` apenas | `--perf` por estágio | `sidecar-eval --perf` | ✅ (cold n pequeno) |
| R1.5 ROI do feedback (3 braços) | feedback com integridade (PR #6) | arms + semeadura por raiz estável | — | ⬜ |
| R1.6 scorecard unificado | `bench/scorecard.py` (OFF/ON) | seções por evidência, pareado, custo/correta | `lab.py report` + testes | ✅ |
| R1.7 comparação OFF/V1/V2/componentes | OFF/ON | braços V1, V2; componentes dependem de R2–R5 | `lab.py run --arms OFF,V1,V2,V2+adaptive,V2+brief` | ✅ medido (225 execuções, §Benchmark limpo); R4/R5/R6 sem braço por falta de canal interativo |
| R2 Adaptive Context Engine | V2 com abstenção parcial | política por intenção, E17, termos genéricos | gates R2 | 🟡 rodada 1: [eval-r2](eval-r2-2026-11-08.md) — política `adaptive` opt-in (sem perda offline, −6/−16% tokens); H1 inconclusiva (flag off); H2 rejeitada; E17 = lacuna de dicionário. Gate de sucesso agentic pendente |
| R3 Task Evidence Brief | `ContractEvidence`, packer | brief rastreável | gate R3 | 🟡 [task-brief](task-brief.md): `SIDECAR_FEATURES=brief` opt-in; qualidade offline idêntica, tokens 417→340; piloto agentic: ver §Piloto R3 |
| R4 TaskUncertainty / grooming | — | detector, política, decisões, benchmark | gate R4 | 🟡 [grooming](grooming.md): `SIDECAR_FEATURES=groom` opt-in; 0/17 perguntas desnecessárias, 8/11 corretas (fixture interna); benefício líquido não medido |
| R5 Impact/Verification | `ContractEvidence` | blast radius, verificações | gate R5 | 🟡 [change-impact](change-impact.md): `sidecar impact` + nota opt-in; histórico real: 46%/63%/49% (chi/click/express) com 3–6 arquivos sinalizados; gate agentic pendente |
| R6 Session Intelligence | `SessionObserver` passivo | traces, estagnação | gate R6 | 🟡 [session-intelligence](session-intelligence.md): classificação passiva + sombra + `sidecar progress` + métricas no lab; sem transporte de intervenção (gate pendente) |
| R7 Studio/adapters/release | docs de integração | estático, matriz de suporte | gate R7 | 🟡 [studio](studio.md) (7 vistas, verificado em Chromium); instalação/uninstall/hooks E2E reais com Claude Code ([r7-install-e2e](../bench/results/r7-install-e2e-2026-11-08.md)); Codex/Antigravity/PreCompact/release real não verificados |

## Corpus (R1.1)

`lab.py manifest` (2026-11-08): **20 tarefas** — development 8, validation 7, holdout 5; classes: locate 2, debug 4, edit 4, multi-file 2, contract-change 4, negative 2, ambiguous 1, voice 1. Repositórios: chi 7, click 6, express 4, sinatra 3. As 20 passaram em `validate-corpus` por mim (oráculo falha na base, passa com o `reference.patch`), independentemente do relato dos autores. **Abaixo do alvo de 24:** MON-* (3 tarefas) bloqueadas por ambiente; EXP-VOI-01 recusada (candidato a defeito conflita com teste existente; não se escolhe autoridade entre código e teste). Cobertura de `ambiguous` e `voice` é de 1 tarefa cada: insuficiente para concluir sobre grooming/voz (R4). Holdout: `holdout_ledger.jsonl` inexistente, nunca executado.

## Piloto de desenvolvimento OFF/V1/V2 (R1, evidência agentic, Haiku)

[bench/results/lab-dev1-2026-11-08](../bench/results/lab-dev1-2026-11-08/): 8 tarefas de development × 3 braços × 1 repetição = 24 execuções, `claude-haiku-5-5` (ID servido == solicitado em todas), Claude Code 2.1.319, config isolada por execução, todas comparáveis, custo medido total **US$ 0,177**. Oráculo sem rede via `unshare -rn`; sem `bwrap`.

| braço | pass (oráculo + escopo + invariantes) | oráculo oculto passou | violações de escopo (em código não-teste) | custo/correta | tool calls |
|---|---:|---:|---:|---:|---:|
| OFF | 6/8 | 7/8 | 2 (1) | 0,0107 | 11 |
| V1 | 5/8 | 8/8 | 3 (0) | 0,0113 | 9,5 |
| V2 | 6/8 | 8/8 | 2 (0) | 0,0094 | 9,6 |

**Leitura honesta:** N=8, 1 repetição, 1 modelo: **inconclusivo**. Diferenças pareadas V2−OFF: Δpass 0, Δcusto −US$ 0,0009 (IC95 de pass degenerado). Não é ganho comprovado.

**Achado de auditoria do corpus:** 5 das 6 reprovações de OFF/V1/V2 são só de escopo — o agente adicionou arquivos de teste legítimos (`tests/test_shell_completion.py`, `tests/test_options.py`, `middleware/zz_repro_test.go`) fora de `allowed_paths`, e o oráculo oculto passou. O `scope.allowed_paths` das tarefas click é estrito demais. Não alterei vereditos nem o corpus depois de ver os resultados; o scorecard agora reporta separadamente `hidden_tests_passed` e `scope_violations_non_test`. Revisão de escopo (permitir testes) deve ser versionada como nova versão do corpus e re-medida, não aplicada retroativamente. Uma violação em código não-teste (CHANGES.md em CLK-AMB-01/OFF, junto com falha do oráculo) permanece.

## Testes realmente executados

- Base `576c68c`: `go build/vet/test ./...` verdes (2026-11-08).
- `python3 -m unittest bench.lab.test_lab`: 23 ok.
- `go test ./cmd/sidecar-eval ./internal/retrieve` com `Tuning`/replay: ok; `sidecar-eval --replay` sobre `bench/cases.json`: determinístico, 7 variantes (fixtures internas: só sanidade, **não** usadas para escolher parâmetros).
- `sidecar-eval --perf 20` (fixtures internas, in-process): V1 warm total p50 3,5 ms / p95 6,4 ms; V2 warm total p50 5,6 ms / p95 11,2 ms (n=1489 chamadas por engine; cold n=2).

## Gastos de modelo

≈ US$ 2,5 de US$ 10: benchmark limpo US$ 1,75 (225 execuções), piloto R3 US$ 0,45, piloto dev US$ 0,18, verificações e sondas ≈ US$ 0,1.

## Benchmark limpo OFF/V1/V2/componentes (R1.7, evidência agentic)

[benchmarks-2026-11-08.md](benchmarks-2026-11-08.md): 225 execuções (15 tarefas × 5 braços × 3 repetições), `claude-haiku-5-5`, todas comparáveis, US$ 1,75, isolamento corrigido, holdout intocado. **Sem melhora comprovada**: V2+brief vs OFF aprovação estrita +8,6%, tool calls −4,8%, tokens −8,8%, custo −4,0% (todos IC95 cruzam zero). Testes ocultos 225/225 em todos os braços (teto de capacidade: o corpus mede eficiência e escopo, não capacidade); 5 de 63 comparações "significativas" é compatível com acaso; tempo de parede contaminado por CPU compartilhada. Os pilotos anteriores (`lab-dev1`, R3) ficam como **superados e possivelmente contaminados**. Índice e pistas para o V3: [benchmarks.md](benchmarks.md).

## Revisão independente (2026-11-08)

Dois revisores Haiku (somente leitura) + eu. Cada achado foi **reproduzido ou verificado no código** antes de aceito. Corrigidos neste PR, com teste de regressão (e controle que falha sem a correção quando aplicável): isolamento do gabarito no lab; timeout fora das taxas; teto de gasto com custo desconhecido; glob e `regex_absent`; holdout em replay e repetição de avaliação final; limites aplicados antes do filtro em `impact` e aviso de truncamento/histórico; busca vazia tratada como repetição no observador; `groom` — pergunta perdida sem evidência, resposta com negação/substring virando decisão, política adaptativa tirando contratos de pedidos de edição, perguntas de valor para renomeações, falsos positivos de contradição, repetição da mesma pergunta gastando a cota, estado não atômico, `awaitingAnswer` sem TTL; sanitizador de `ctxbuild` com delimitador dividido por espaço. **Não corrigidos / aceitos:** sem sandbox de SO (código rodado por `go test` lê o gabarito); `hashID` sem sal; amostra e independência do `--impact-history`; `git worktree add` no repositório avaliado; estado de sessão sem lock entre hooks paralelos (suspeita, não reproduzida).

## Bloqueios e riscos conhecidos

- ⛔ Sem `bwrap` no ambiente cloud: oráculo isolado de rede por `unshare -rn`; agente `claude` sem sandbox de SO.
- ⛔ monolog (MON-*): `composer install` falha — o proxy da sessão devolve 403 para api.github.com/codeload.github.com, então PHPUnit não instala; 3 tarefas pendentes (corpus alvo cai de 24 para 21). Sem contorno por outro host.
- Corpus escrito por subagentes + mesmo time do produto: viés de autoria registrado.

## Próxima ação concreta

Corpus v2 mais difícil (testes ocultos passaram 225/225), canal interativo para medir R4/R5/R6, sandbox de SO antes de gastar o holdout, e portão de abstenção para prompts conversacionais (L10). Ver pistas L01–L12 em benchmarks.md.
