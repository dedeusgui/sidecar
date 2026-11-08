# Piloto E2E com hook real do Claude Code (2026-11-08)

**Estado:** piloto de instrumento. Valida que o caminho ponta a ponta funciona e revela uma falha de abstenção. **Não é prova de ganho nem de perda** do Sidecar: são 5 casos, 2 repetições, escritos pelo mesmo autor, com ceiling effect (19 de 20 execuções acertaram).

Artefatos: [`bench/results/real-hook-pilot-2026-11-08/`](../bench/results/real-hook-pilot-2026-11-08/) (`runs.jsonl`, `run-metadata.json`, `agent-benchmark.md`, `diagnosis.md`, `pilot.log`).

## O que foi executado

| Item | Valor |
|---|---|
| Cliente | Claude Code 2.1.319 (`claude -p`, `--output-format stream-json --include-hook-events`) |
| Modelo | `claude-haiku-5-5`, verificado em `modelUsage` das 20 execuções |
| Repositório | `sidecar` em `312efd6`, um clone `--shared` descartável por execução, árvore verificada limpa antes e depois |
| Casos | 4 de localização (2 diretos, 2 em PT-BR informal) e 1 negativo, em [`real_hook_pilot_cases.json`](../bench/agentic/real_hook_pilot_cases.json) |
| Execuções | 20 = 5 casos × 2 braços × 2 repetições, ordem AB/BA |
| Custo medido | **US$ 0,1079** no piloto (teto reservado US$ 2,00; teto por execução US$ 0,10, que não foi atingido) |
| Custo total de chamadas reais nesta sessão | ≈ US$ 0,29, incluindo smokes e a sonda do hook |
| Oráculo | gold de arquivo do caso; o agente nunca vê o gold nem pode editar |

**Braço ON.** O hook real `UserPromptSubmit` (`sidecar hook --agent claude`) é registrado só nesta execução por `--settings`. O cliente o dispara. A entrega é verificada no stream: `system/hook_started` seguido de `system/hook_response`, cujo `output` contém `hookSpecificOutput.additionalContext`. **Disparo sozinho não conta como entrega.**

**Braço OFF.** Sem hook.

**Isolamento aplicado a ambos os braços:** `--setting-sources project,local` (configurações de usuário não carregam), `--strict-mcp-config`, `--disable-slash-commands`, `Edit`/`Write`/`NotebookEdit` bloqueados, modo `plan`, `SIDECAR_HOME` próprio por execução.

**Não provado:** ausência de plugins, skills e MCP globais no processo. Não há teste canário nem `CLAUDE_CONFIG_DIR` isolado. Por isso `--real-hooks` em `run_agents.py` continua **desativado**.

## Resultados

| Braço | Sucesso (Hit@5) | Hit@1 | Chamadas de ferramenta (média) | Turnos (média) | Tokens de entrada (média) | Tempo (média) | Custo (média) |
|---|---:|---:|---:|---:|---:|---:|---:|
| OFF | 9/10 (90%) | 7/8 | 3,2 | 4,2 | 189.152 | 17,5 s | US$ 0,0060 |
| ON | 10/10 (100%) | 6/8 | 2,6 | 3,6 | 144.477 | 16,5 s | US$ 0,0048 |

Hit@1 conta só os 8 casos não negativos. O braço ON teve mais arquivos corretos entre os cinco primeiros (Hit@5), mas menos no primeiro lugar (Hit@1). Em amostra tão pequena, a diferença pode ser ruído.

Diferença pareada ON − OFF por caso (média das repetições): tempo −0,9 s, chamadas −0,6, tokens de entrada −44.676, custo −US$ 0,0011. O sinal varia entre os casos (ver `agent-benchmark.md`). **Não há conclusão estatística.**

## Diagnóstico das falhas (automático, `bench/agentic/diagnose_runs.py`)

Só uma execução falhou: `OFF/H04-model/rep0`. A resposta indicou arquivos de `bench/agentic/` em vez de `bench/agent/run_agents.py`. Causa candidata: `wrong_answer`, sem contexto para comparar.

**Falha de abstenção (a principal achada pelo piloto).** No caso negativo `N01` ("como faço pão de fermentação natural em casa?") o hook entregou **1.307 caracteres de contexto em 2 de 2 execuções**, apesar de não haver evidência relevante no repositório. A consulta é a mesma no `sidecar hook`, sem modelo:

- `internal/retrieve/voice_mode_test.go#constant:naturalSpokenPrompt@98` (confiança 0,95)
- `internal/lexical/lexical.go#file:lexical.go@1` (0,90), "Package lexical normalizes natural language…"
- `internal/retrieve/retrieve.go#file:retrieve.go@1` (0,90)

A palavra comum "natural" casa com identificadores e comentários. A intenção ficou `general` (confiança 0), e o portão de abstenção não bloqueou. A resposta do agente foi correta (`{"files": []}`), por isso o desfecho não mudou, mas o contexto custa tokens e pode levar o agente a seguir ruído. O diagnosticador marca esse caso como `false_positive_injection`, mesmo com resposta correta.

**Recuperação perdida no caso H04.** Nas duas repetições ON, o gold `bench/agent/run_agents.py` não estava no contexto entregue. O agente o encontrou com ferramentas e acertou. Isso mostra que a recuperação falhou e a ferramenta compensou.

## Limitações

- **Amostra mínima:** 5 casos e 2 repetições por braço. O teto de desempenho (19/20) impede discriminar.
- **Viés de autoria e de repositório:** os casos foram escritos pelo mesmo autor do código do Sidecar, e o repositório pinado contém testes e documentação recém-escritos. Os dois braços veem esses arquivos, o que favorece localização, mas é igual entre eles. A regra de [evaluation-plan.md](evaluation-plan.md#validade-externa-e-prompts-realistas) exige repositórios externos e prompts não escritos pelo autor.
- **Apenas Haiku, apenas Claude Code.** Codex e Antigravity não foram executados.
- **Custo** é o relatado pelo cliente (`total_cost_usd`), não cobrança.
- **Isolamento parcial** (ver acima).
- Não há caso de voz longa (ruído de fala) e a categoria negativa tem um único caso. Ambas as categorias precisam de amostra própria.

## Como reproduzir

```bash
go build -o /tmp/sidecar-pilot ./cmd/sidecar
# plano sem gasto
python3 bench/agentic/real_hook_pilot.py --repo . --commit 312efd6 --sidecar /tmp/sidecar-pilot \
    --cases bench/agentic/real_hook_pilot_cases.json --reps 2 --max-budget 0.10 --total-budget 2.0 --out-dir /tmp/pilot
# execução paga (teto reservado US$ 2,00)
python3 bench/agentic/real_hook_pilot.py ... --yes-spend
python3 bench/agent/run_agents.py --report /tmp/pilot/runs.jsonl --report-md /tmp/pilot/agent-benchmark.md
python3 bench/agentic/diagnose_runs.py /tmp/pilot/runs.jsonl --md /tmp/pilot/diagnosis.md
```

Hashes de `pilot`, `runner`, `sidecar` e `cases` estão em `run-metadata.json`.

## Próximos passos antes de qualquer conclusão

1. Corrigir ou calibrar a abstenção para termos genéricos e incluir negativos como caso de regressão em `bench/cases.json`, medindo com `sidecar-eval` (A/B, ablação e holdout intocado).
2. Instrumentar `CLAUDE_CONFIG_DIR` isolado e um canário de configuração global, para provar o isolamento antes de reabilitar `--real-hooks`.
3. Trocar os casos por tarefas de repositório externo com oráculo independente (teste que o agente não vê), com repetições suficientes para intervalo de confiança.
4. Só então medir ON × OFF com tempo até evidência correta, chamadas e custo.
