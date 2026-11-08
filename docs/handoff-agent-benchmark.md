# Handoff: benchmark de agentes reais e estado do Sidecar (2026-11-08)

Atualização: consulte [benchmark-consolidation.md](benchmark-consolidation.md) para o inventário e bloqueios atuais; `--real-hooks` está desativado até validar o isolamento.

Documento histórico para quem continuar o trabalho. Os relatos abaixo descrevem a rodada original; estado e restrições atuais estão na consolidação. Contrato de engenharia: [AGENTS.md](../AGENTS.md).

## 1. Onde estamos

| Frente | Estado | Evidência |
|---|---|---|
| Recuperação offline, 10 repositórios reais (50 casos) | Pronta e reexecutada em 2026-11-08 | [ten-repos-benchmark.md](../bench/results/ten-repos-benchmark.md): 50/50 em `lexical+grafo`, `v2.1` e `v2`; voz 10/10 Hit@5; 0/10 falso positivo |
| Simulação agêntica (política fixa de grep, sem LLM) | Pronta | [agentic-ab-report.md](../bench/results/agentic-ab-report.md), [real-repos-ab-score.md](../bench/results/real-repos-ab-score.md) |
| Relatório consolidado (voz, feedback, limites) | Pronto, revisado sem números inventados | [feedback-and-voice-roi.md](../bench/results/feedback-and-voice-roi.md) |
| Dicionário PT-BR→EN algorítmico | Pronto | `internal/lexical/cognates.go`, `lexicon_pt_en.tsv`, `$SIDECAR_HOME/lexicon.json` |
| Feedback pós-tarefa (`sidecar feedback record/list/purge`) | Implementado e testado; ROI **não** medido com agente real | `internal/store/feedback.go`, `internal/retrieve` (`FeedbackMultipliers`) |
| **Benchmark de agentes reais** | Runner em qualificação; matriz completa não executada | [bench/agent/run_agents.py](../bench/agent/run_agents.py) e [consolidação](benchmark-consolidation.md) |

> [!WARNING]
> Os smokes históricos abaixo não constituem score por cliente. Para comparações, conferir isolamento, dados, falhas e limitações no relatório de cada rodada.

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
- Braço ON: contexto do binário real `sidecar hook --agent claude`, alimentado com payload `UserPromptSubmit`, **prefixado ao prompt**. **Não** é o hook instalado do cliente. O índice é quente (`index` antes das medições); `sidecar_ms` registra o custo do hook.
- Pares adjacentes em ordem AB/BA balanceada, com semente fixa. Falhas interrompem a rodada e pares incompletos são registrados.
- Modos: Claude `-p --output-format json --permission-mode plan --disallowedTools Edit Write NotebookEdit --max-budget-usd`, com settings globais excluídos; Codex `exec --json --ephemeral --ignore-user-config -s read-only`. Antigravity está bloqueado até qualificar isolamento.
- Saídas: `run-metadata.json`, `runs.jsonl` e `agent-benchmark.md`; `raw/` é opcional com `--save-raw` e pode conter dados sensíveis. O bootstrap agrupa repetições por tarefa e requer pelo menos três tarefas pareadas.

**Modelos econômicos usados nos exemplos** (verifique com `agy models`, `codex` config e `claude --model`): Claude `claude-haiku-5-5` (ID explícito), Codex `gpt-6-luna`, Antigravity `gemini-3.8-flash-low`. O nome "Codex Mini" não existe nesta instalação.

> [!WARNING]
> **Alias `haiku` não é Haiku (verificado em 2026-11-08).** Com `claude -p --model haiku` o `modelUsage` reportou `claude-sonnet-5-5`, e o custo de uma chamada trivial foi US$ 0,163. Com `claude-haiku-5-5` o custo foi US$ 0,0083. `run_agents.py` agora recusa aliases de Claude e registra `served_models`. **Os números de Claude das seções 3 e 6 foram gerados com o alias e não são de Haiku.**

**Comandos**
```bash
bash bench/agentic/setup_10_repos.sh /tmp/sidecar-real-bench      # clona os 10 repositórios fixados
go build -o /tmp/sidecar-agent-bench ./cmd/sidecar
# plano (sem gastar):
python3 bench/agent/run_agents.py --manifest /tmp/sidecar-real-bench/ten-repos-manifest.json
# exemplo de piloto pago: pré-registrar protocolo e conferir isolamento antes
python3 bench/agent/run_agents.py --manifest /tmp/sidecar-real-bench/ten-repos-manifest.json \
  --sidecar /tmp/sidecar-agent-bench --agents claude:haiku codex:gpt-6-luna \
  --per-category 1 --reps 1 --max-budget 0.3 --total-budget 8 \
  --out-dir /tmp/agent-bench --yes-spend
# relatório a partir de runs.jsonl:
python3 bench/agent/run_agents.py --report /tmp/agent-bench/runs.jsonl --report-md /tmp/agent-benchmark.md
```

**Smoke executado (1 caso `R01-1`, ON e OFF)**
- Claude (alias `haiku`, servido por Sonnet): OFF 12,7 s, 74.579 tokens de entrada (com cache), US$ 0,129; ON 14,8 s, 76.768 tokens, US$ 0,133; ambos acertaram em 2 turnos; hook ~47 ms. O OFF pode ter sido contaminado por hooks globais; não é comparação válida.
- Codex e Antigravity: ver seção 6.

## 4. Próximos passos, em ordem

1. Auditar o piloto pré-registrado e seus pares, custos, isolamento e falhas conforme a [consolidação](benchmark-consolidation.md).
2. Corrigir limitações do instrumento antes de expandir; qualificar Antigravity separadamente.
3. Planejar nova rodada de medição com orçamento e desenho adequados; pré-registrar versões dos clientes, modelos e preços no relatório.
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

**Atualização 2026-11-08:** piloto E2E com hook real (`bench/agentic/real_hook_pilot.py`, Claude Code, `claude-haiku-5-5`, 5 casos × 2 repetições × 2 braços) está em [agent-pilot-2026-11-08.md](agent-pilot-2026-11-08.md). O hook real foi disparado e sua resposta contendo `additionalContext` foi verificada no stream de eventos. Não é prova de ganho: os casos são poucos, escritos pelo mesmo autor e ambos os braços acertaram quase tudo.

Com uma amostra por braço não há conclusão (variância de rede/modelo é grande); serve para validar o runner.

Problemas achados e corrigidos no runner:
- **Antigravity headless** nega comandos sem regra de permissão. O runner passa `--settings` de sessão liberando só `command(grep|rg|cat|ls|find|head|tail|sed|wc|git)`; não usa `--dangerously-skip-permissions`.
- **Antigravity** devolve a resposta no campo `response` do JSON, com aspas escapadas; o parser agora lê esse campo antes de procurar `{"files": [...]}`.
- **Codex**: tokens e tool calls vêm dos eventos `turn.completed` e `item.completed` do `--json`; custo em US$ não é informado (`n/d`).
- **Claude Code**: custo e tokens vêm do JSON final; contagem de tool calls é `n/d` (usar `stream-json` se for preciso).
