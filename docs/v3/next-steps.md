# Sidecar V3 — o que foi entregue e próximos passos

> **Nova arquitetura planejada:** a unificação das capacidades da V3 em um motor V3.5, com Brief + código selecionado, AutoExplore e packer global, está descrita em [docs/v3.5](../v3.5/README.md). **Esta página continua sendo o registro factual da V3**; não indica que a V3.5 esteja implementada. O primeiro gate de implementação V3.5 é criar um novo conjunto E1/E2 independente *antes* de alterar o relevance gate ou o Explore.


**Atualizado:** 2026-11-09, `main@46d6d39` (PR #21 mergeado). Detalhes, números e protocolo: [progress.md](progress.md). Regras: [AGENTS.md](../../AGENTS.md), [evaluation-protocol.md](evaluation-protocol.md).

**Situação de orçamento:** sem quota para execuções agentic pagas. Tudo marcado **[grátis]** pode ser feito agora; **[pago]** só com autorização explícita e teto (`--max-total-usd`).

## 1. O que existe hoje

| Área | Entregue | Onde | Estado |
|---|---|---|---|
| Laboratório (P0-A) | Sandbox bwrap validado de verdade; runtimes Go/Python/Node/Ruby funcionam isolados; gabarito ilegível por código executado; OFF não alcança Sidecar/CodeGraph; proxy/CA iguais entre braços; preflight `claude auth status` | `bench/lab/lab.py` | pronto; **autenticação real e `--resume` não validados** (pago) |
| Engine V3 (P0-B) | RelevanceGate + EffortRouter determinísticos sobre a V2, `SIDECAR_ENGINE=v3`; decisão explicável no `explain --engine v3` e no log (só metadados) | `internal/engine` | experimental, opt-in; V1 default |
| Explore (P1) | `sidecar explore --query ... [--json]`, contrato `sidecar-explore/1` | `internal/explore`, [explore.md](../explore.md) | experimental; sem MCP/adaptador |
| Suficiência (P0-C) | Próximo ato após cada entrega por thread/subagente; releitura redundante, contexto desatualizado, entrega sem efeito; tokens por origem com `unknown` | `bench/lab/sufficiency.py` | pronto; nunca exercido com agente |
| Braços A/B | `OFF`, `V1`, `V2`, `V2+feature`, `V3`, `V3-hook`, `V3+feature`, `CG` (CLI), `CG-mcp` | `bench/lab/lab.py` | prontos; testados com cliente falso |
| Corpora | Edição v2 (32 tarefas, 9 holdout selado); navegação E2 (7 tarefas com oráculo de fatos); E1 independente (70 casos, já usado) | `bench/lab/corpus*`, `bench/v3/e1_independent.json` | holdout intacto |
| Baselines offline | textual, BM25, lexical+grafo, V1/V2, Aider RepoMap, CodeGraph explore (cobertura de fatos) | `cmd/sidecar-eval`, `bench/v3/*.py` | prontos |
| Correções globais | Funções JS/TS atribuídas indexadas (L03); ranking determinístico entre construções do índice | `internal/extract`, `internal/retrieve` | na `main` |

**Resultados que temos (todos offline, nenhum agentic):**
- V3 vs V2: falsos positivos 30→14/180 (B03), 13→9/40 no conjunto independente, Hit@5 igual nos repositórios externos, −29% tokens. **Perde recall** em prompts curtos de domínio (fixtures internas Hit@5 67,5%→57,5%). Meta FP<5% não atingida.
- Fatos de navegação entregues: hook V2 14/26, sidecar explore 22/26 (38,9k chars), CodeGraph explore 24/26 (114k chars).

## 2. Próximos passos, em ordem

### Etapa A — melhorias gratuitas que destravam as medições [grátis]

1. **Novo conjunto independente de E1** (antes de mexer no gate). O atual já foi usado; precisa de ≥30 positivos curtos de domínio PT-BR + ≥40 negativos, escritos antes de qualquer ajuste e congelados por hash em `progress.md`. Idealmente escrito por outra pessoa.
2. **Recuperar recall em prompts curtos de domínio (L15)** sem reabrir falsos positivos: testar sinal de raridade de nomes do repositório (L02) e vocabulário derivado de docs/comentários (L01). Escolher só com splits de desenvolvimento; medir no conjunto do passo 1. Critério: Hit@5 nas fixtures internas ≥ V2 e FP em B03 ≤ 14/180.
3. **Explore (L16):** expandir a cadeia a partir do melhor seed (callers/callees com corpo) para cobrir Monolog (`handle`→`write`), Sinatra (`route!`) e Click (`parse_args`); reduzir caracteres cortando símbolos sem termo da consulta. Medir com `bench/v3/e2_offline.py --codegraph ...`. Alvo: ≥24/26 fatos com ≤50k chars.
4. **Mais tarefas de navegação:** subir `corpus-nav` de 7 para ≥20, em ≥5 linguagens, com split de validação nunca olhado. Cada fato verificado por grep no commit fixado.
5. **Latência cold/warm da V3 e do explore:** incluir V3 em `sidecar-eval --perf` e medir explore cold (índice novo) e após mudança de arquivo.
6. **Explore com `--json` consumido pelo próprio lab:** garantir que `sufficiency.paths_in_explore_output` reconhece a saída (hoje reconhece o texto).

### Etapa B — piloto de navegação E2 [pago, pequeno]

Objetivo: saber se o explore muda o que o agente faz e se a resposta continua correta. Começar barato:

```bash
go build -o /tmp/sidecar ./cmd/sidecar
npm install --prefix /tmp/cg @colbymchenry/codegraph@1.6.2
LAB_CORPUS=bench/lab/corpus-nav python3 bench/lab/lab.py run --agent claude --model claude-haiku-5-5 \
  --sidecar /tmp/sidecar --codegraph /tmp/cg --arms OFF,V2,V3,V3-hook,CG --reps 2 --splits development \
  --out-dir /tmp/lab-e2 --max-budget 0.5 --max-total-usd <TETO> --yes-spend
python3 bench/lab/lab.py report /tmp/lab-e2/runs.jsonl --md /tmp/lab-e2/scorecard.md
```

Ler, nesta ordem: taxa de respostas corretas (oráculo de fatos); suficiência por entrega (`moved_on`, `read_returned`, `explore_again`); leituras e tool calls; custo e tempo por resposta correta. 4 tarefas × 5 braços × 2 reps = 40 execuções. É piloto: não sustenta claim.

### Etapa C — edição E3 e duas voltas [pago]

1. Piloto de dificuldade em development (as 12 tarefas novas nunca rodaram com agente): `--arms OFF,V2 --reps 1`. Se todos passam, o corpus não discrimina; criar tarefas mais difíceis antes de gastar mais.
2. Campanha `OFF,V2,V2+brief,V3` em development+validation; `--turns 2` para tarefas ambíguas (usuário simulado condicional).
3. Só depois: modelo intermediário e um forte, mesmas tarefas.

### Etapa D — decisão sobre promoção

A V3 só vira default se, nas campanhas acima: correção não inferior à V2 (IC pareado agrupado por tarefa/repo), economia de custo ou tempo por tarefa correta, p95 aceitável, abstenção sem perda relevante de recall, E2E real do hook. Depois: freeze, holdout uma única vez (`--final`, `--frozen-revision`, `--frozen-binary-sha256`), relatório com perdas. Se não passar, continua opt-in.

### Etapa E — capacidades restantes (depois das medições)

- **Brief:** manter atrás de `SIDECAR_FEATURES=brief` até a Etapa C mostrar benefício.
- **Groom/CLARIFY:** ligar a ação `CLARIFY` do router ao `internal/groom` só se E4 (duas voltas condicionais) mostrar menos retrabalho.
- **Impact/VERIFY:** hoje o VERIFY só oferece o explore; acoplar `impact.Analyze` ao hook só com evidência; nunca afirmar verificação não executada.
- **Session Intelligence:** observação já existe; intervenção ativa só com transporte real confirmado e E5.
- **LSP:** opcional; medir antes de ativar.
- **Adaptadores:** MCP ou suporte Codex/Antigravity para o explore só após E2E real por cliente.

## 3. Regras que continuam valendo

- Sem tuning no holdout nem no conjunto independente já usado; qualquer ajuste após ver resultado exige conjunto novo.
- Toda perda vai para `progress.md` e `bench/registry.json` (rodar `python3 bench/consolidate.py`).
- `moved_on` não é correção; cobertura de fatos não é resposta correta; offline não é agentic.
- Pushes diretos na `main` estão autorizados pelo mantenedor, **depois** de `go test ./...`, race, lint (`golangci-lint` v2.5.0), testes Python do lab e CI verde.
