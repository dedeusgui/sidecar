# Handoff: benchmark de agentes reais e estado do Sidecar (2026-11-08)

Documento para quem continuar o trabalho (outro agente ou pessoa). Tudo aqui foi verificado em `main`; o que **não** foi feito está marcado. Contrato de engenharia: [AGENTS.md](../AGENTS.md).

## 1. Onde estamos

| Frente | Estado | Evidência |
|---|---|---|
| Recuperação offline, 10 repositórios reais (50 casos) | Pronta e reexecutada em 2026-11-08 | [ten-repos-benchmark.md](../bench/results/ten-repos-benchmark.md): 50/50 em `lexical+grafo`, `v2.1` e `v2`; voz 10/10 Hit@5; 0/10 falso positivo |
| Simulação agêntica (política fixa de grep, sem LLM) | Pronta | [agentic-ab-report.md](../bench/results/agentic-ab-report.md), [real-repos-ab-score.md](../bench/results/real-repos-ab-score.md) |
| Relatório consolidado (voz, feedback, limites) | Pronto, revisado sem números inventados | [feedback-and-voice-roi.md](../bench/results/feedback-and-voice-roi.md) |
| Dicionário PT-BR→EN algorítmico | Pronto | `internal/lexical/cognates.go`, `lexicon_pt_en.tsv`, `$SIDECAR_HOME/lexicon.json` |
| Feedback pós-tarefa (`sidecar feedback record/list/purge`) | Implementado e testado; ROI **não** medido com agente real | `internal/store/feedback.go`, `internal/retrieve` (`FeedbackMultipliers`) |
| **Benchmark de agentes reais (Claude Code, Codex, Antigravity CLI)** | **Runner pronto; matriz completa NÃO executada** | [bench/agent/run_agents.py](../bench/agent/run_agents.py) (seção 3) |

> [!WARNING]
> Não existe ainda nenhum score por cliente. Qualquer tabela "Claude vs Codex vs Antigravity" anterior a este documento era inventada e foi removida. Só vale o que sair de `run_agents.py --yes-spend`.

## 2. O que cada benchmark prova (e não prova)

- **Recuperação offline** (`sidecar-eval --external-cases`): o Sidecar acha o arquivo certo? Sem modelo. Prova controle de regressão; **não** prova que o agente fica mais rápido. Casos de um autor só, split de desenvolvimento, voz com molde repetido.
- **Simulação agêntica**: mede chamadas e tokens de ferramenta sob uma política fixa. Resolução igual (83%); ON usa ~30% menos chamadas e ~35% menos tokens; tempo de ferramenta **maior** (~144 contra ~96 ms). Tarefas `R01`–`R06`, simples e poucas.
- **Agente real** (este handoff): única forma de medir tempo de parede, tokens, custo e sucesso com agente caixa-preta.

## 3. Runner de agentes reais

Arquivo: `bench/agent/run_agents.py` (Python stdlib). Gasta dinheiro; sem `--yes-spend` só planeja.

**Desenho**
- Casos: `bench/ten_repos_cases.json` (categorias `direct`, `messy_pt`, `voice_mode`, `negative`), checkouts fixos criados por `bench/agentic/setup_10_repos.sh /tmp/sidecar-real-bench`.
- Tarefa: localização somente leitura. O agente recebe o prompt do caso mais um sufixo fixo e deve terminar com uma linha `{"files": [...]}`. Oráculo = gold de arquivo do manifesto (`success` = Hit@5; negativos exigem `{"files": []}`). O agente não vê o gold.
- Isolamento: `git clone --shared` + checkout do commit fixado em diretório temporário por execução; detecta edição acidental (`edited_files`).
- Braço ON: contexto do binário real `sidecar hook --agent claude`, alimentado com payload `UserPromptSubmit`, **prefixado ao prompt** nos três clientes (mesmo canal, comparação justa). **Não** é o hook instalado do cliente e não toca em settings do usuário. O índice é quente (`index` antes das medições); `sidecar_ms` registra o custo do hook.
- Ordem das execuções embaralhada (seed fixa) para reduzir viés de carga e cache.
- Modos: Claude `-p --output-format json --permission-mode plan --disallowedTools Edit Write NotebookEdit --max-budget-usd`; Codex `exec --json --ephemeral --ignore-user-config -s read-only`; Antigravity `--print --output-format json --mode plan`.
- Saídas: `runs.jsonl` (um registro por execução), `raw/` (stdout/stderr por execução), `agent-benchmark.md` (relatório: resumo por agente/braço, por categoria, diferença pareada ON−OFF com IC 95% por bootstrap só com 3+ pares).

**Modelos econômicos usados nos exemplos** (verifique com `agy models`, `codex` config e `claude --model`): Claude `haiku`, Codex `gpt-6-luna`, Antigravity `gemini-3.8-flash-low`. O nome "Codex Mini" não existe nesta instalação.

**Comandos**
```bash
bash bench/agentic/setup_10_repos.sh /tmp/sidecar-real-bench      # clona os 10 repositórios fixados
go build -o ~/.local/bin/sidecar ./cmd/sidecar
# plano (sem gastar):
python3 bench/agent/run_agents.py --manifest /tmp/sidecar-real-bench/ten-repos-manifest.json
# rodada piloto (gasta; começar pequeno):
python3 bench/agent/run_agents.py --manifest /tmp/sidecar-real-bench/ten-repos-manifest.json \
  --agents claude:haiku codex:gpt-6-luna agy:gemini-3.8-flash-low \
  --per-category 2 --reps 1 --out-dir /tmp/agent-bench --yes-spend
# relatório a partir de runs.jsonl (copiar para o repo ao final):
python3 bench/agent/run_agents.py --report /tmp/agent-bench/runs.jsonl --report-md bench/results/agent-benchmark.md
```

**Smoke executado (1 caso `R01-1`, ON e OFF)**
- Claude Haiku: OFF 12,7 s, 74.579 tokens de entrada (com cache), US$ 0,129; ON 14,8 s, 76.768 tokens, US$ 0,133; ambos acertaram em 2 turnos; hook ~47 ms. Conclusão: em tarefa trivial o Sidecar não ajuda (e custa um pouco) — esperado; um caso não é resultado.
- Codex e Antigravity: ver seção 6.

## 4. Próximos passos, em ordem

1. Corrigir o que o smoke de Codex/Antigravity apontar (parsers de tokens/chamadas; `tool_calls` é `n/d` no Claude porque o JSON não expõe a contagem — usar `--output-format stream-json` se for necessário).
2. Rodada piloto: `--per-category 2 --reps 1` (48 execuções; Claude custa ~US$ 0,13 por execução, então ~US$ 2 só nele). Conferir `raw/` por falhas de formato.
3. Rodada de medição: `--per-category 5 --reps 3`+ (IC exige 3+ pares). Pré-registrar versões dos clientes, modelos e preços no relatório.
4. Adicionar **tarefas mais difíceis que localização** (multi-arquivo, edição com oráculo de teste independente do repositório do agente). O manifesto `bench/agentic/manifest.jsonl` tem rascunhos `A01`–`A0x` e `R01`–`R06` verificadas; os 10 repositórios têm apenas gold de localização.
5. Adicionar prompts de voz **independentes** (gravados/transcritos por outra pessoa) e congelar um **holdout** antes de olhar resultados.
6. Medir o hook real instalado de cada cliente (variante `--real-hooks`, hoje ausente) em vez do prefixo de prompt, com `sidecar install` em HOME isolado (consentimento e backup conforme AGENTS.md).
7. Só então avaliar o ROI do `sidecar feedback` com agente real (hoje há um único par controlado em `chi`).
8. Ablação isolada da poda de `expand`/`pickSeeds` (reduz latência; sem A/B próprio).

## 5. Pendências e riscos conhecidos

- Latência de voz: mediana `v2` 16 ms, máximo 30 ms (`v2.1`: 8 ms / 11 ms). O teste de 15 ms usa o melhor de 6 execuções em fixture pequeno.
- `real-repos-ab-score.md` tem tabelas da primeira coleta (há nota no topo); valores atuais em `agentic-ab-report.md` e `real-repos-scores.json`.
- `bench/agentic/instrumented_sim.py` lê agora o TSV (`--dict internal/lexical/lexicon_pt_en.tsv`); versões antigas esperavam `dictionary.go`.
- `/tmp/sidecar-real-bench` é apagado em reinícios; recriar com `setup_10_repos.sh`. Nunca rodar `sidecar-eval --report-md` com manifesto ausente sem checar: o comando falha antes de escrever, mas confirme com `git status`.
- Custo: nada neste repositório deve rodar agentes pagos em CI.
- Codex 6.1 Terra não foi despachado nesta rodada; o trabalho foi fechado direto.

## 6. Status do smoke de Codex e Antigravity (1 caso `R01-1`, uma execução por braço; não é resultado)

| Agente | Braço | Tempo (s) | Tokens in | Tool calls | Acertou |
|---|---|---:|---:|---:|---|
| Codex `gpt-6-luna` | OFF | 13,5 | 73.343 | 2 | sim |
| Codex `gpt-6-luna` | ON | 12,1 | 40.756 | 1 | sim |
| Antigravity `gemini-3.8-flash-low` | OFF | 34,8 | 91.783 | n/d | sim |
| Antigravity `gemini-3.8-flash-low` | ON | 16,1 | 31.211 | n/d | sim |

Com uma amostra por braço não há conclusão (variância de rede/modelo é grande); serve para validar o runner.

Problemas achados e corrigidos no runner:
- **Antigravity headless** nega comandos sem regra de permissão. O runner passa `--settings` de sessão liberando só `command(grep|rg|cat|ls|find|head|tail|sed|wc|git)`; não usa `--dangerously-skip-permissions`.
- **Antigravity** devolve a resposta no campo `response` do JSON, com aspas escapadas; o parser agora lê esse campo antes de procurar `{"files": [...]}`.
- **Codex**: tokens e tool calls vêm dos eventos `turn.completed` e `item.completed` do `--json`; custo em US$ não é informado (`n/d`).
- **Claude Code**: custo e tokens vêm do JSON final; contagem de tool calls é `n/d` (usar `stream-json` se for preciso).
