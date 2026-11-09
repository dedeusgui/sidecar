# Sidecar V3 — progresso de implementação

Atualizado em 2026-11-09. Branch `ccr-f508c6e8-0bizc9`, baseada em `main@d15aedb4253c444282a0cb70385f50921e410823` (PR #19 documentação V3 e PR #20 laboratório P0-A **já mergeados**; CI da `main` verde). Nenhum merge feito nesta etapa. **Custo pago: US$ 0** — nenhuma execução agentic paga, nenhum holdout aberto.

## Resumo honesto

| Item | Estado | Evidência |
|---|---|---|
| P0-A laboratório | Infra do Codex revisada; **sandbox bwrap exercido de verdade pela primeira vez**, 3 falhas reais corrigidas | 98 testes Python; sondas reais abaixo |
| P0-B RelevanceGate + EffortRouter | Implementado, opt-in `SIDECAR_ENGINE=v3`; V1 continua default, V2 inalterada | E1 abaixo (B17) |
| P1 `sidecar explore` | Implementado (CLI local, contrato `sidecar-explore/1`) | [docs/explore.md](../explore.md); E2 offline (B18) |
| P0-C suficiência / custo por origem | Implementado no laboratório | `bench/lab/sufficiency.py` + testes sintéticos |
| E2 navegação (agentic) | Corpus e braços prontos; **não executado** (pago) | `bench/lab/corpus-nav` |
| E3–E5 agentic | Infra existente + braços V3; **não executado** (pago) | comandos no fim |
| Braços CodeGraph `CG` (CLI) e `CG-mcp` | Implementados no laboratório; E2E **sem modelo** (cliente falso + CodeGraph 1.6.2 real no sandbox) | `test_cg_arm_runs_codegraph_in_the_sandbox_with_a_fake_client` |
| Promoção da V3 | **Não promovida.** Sem evidência end-to-end | — |

## 1. P0-A — auditoria e validação do laboratório

Ambiente desta sessão: Linux, Go 1.24.7, `claude` 2.1.319 instalado, Python 3.13, Node 22, Ruby 3.3.6 (rbenv), pytest 9.1 (uv tool). `bubblewrap` 0.9.0 foi instalado via apt — **o sandbox nunca tinha rodado de verdade** (o checkpoint anterior registrava `bwrap` ausente; os testes de sandbox eram pulados).

Falhas reproduzidas com bwrap real e corrigidas (`e17d680`):
1. **ruby, npm e pytest não executavam no sandbox**: só o arquivo do executável era montado; ruby (rbenv) não achava `libruby.so`, npm não achava seus módulos, pytest (uv) não achava o interpretador do shebang. Agora o prefixo mínimo de instalação é montado (prefixo do executável, das bibliotecas ELF via `ldd` e do interpretador do shebang); prefixos amplos (`/`, `/root`, `$HOME`, `/opt`...) são recusados e o comando fica indisponível para todos os braços. Regressões: `test_runtime_outside_usr_keeps_its_prefix_libraries`, `test_runtime_prefixes_follow_shebang_and_refuse_broad_mounts`.
2. **Controle OFF podia alcançar CodeGraph/Sidecar global por caminho absoluto** (ex.: `npm -g` dentro de `/opt/node22`, que agora é montado): arquivos `sidecar*`, `codegraph*` e `@colbymchenry` dentro dos prefixos montados são mascarados (`tmpfs` ou `/dev/null`), exceto o binário montado de propósito pelo braço. Regressões: `test_off_arm_cannot_reach_global_codegraph_by_absolute_path`, `test_intended_sidecar_link_survives_competitor_masking`.
3. **`--clearenv` removia proxy/CA**: em rede com proxy o cliente não alcança a API. Agora `HTTPS_PROXY`/`NO_PROXY` e o arquivo de CA são repassados igualmente a todos os braços (nada mais). Preflight gratuito `claude auth status` no sandbox antes de qualquer execução paga (`verify_agent_auth`): exige `loggedIn` e `configDirectory=/agent-config`.

Sondas reais (sem modelo), dentro do sandbox:
- `claude --version` executa; `claude auth status` lê a configuração isolada (`configDirectory=/agent-config`). **Autenticação real não pôde ser validada**: o ambiente usa credencial gerenciada pelo host e `~/.claude/.credentials.json` não contém OAuth do Claude; o preflight falha fechado como esperado.
- Leitura do `reference.patch` por **Python, bash, Node, Ruby e `go test`** com caminho absoluto: todas falham (`ENOENT`). `/home/user` e o repositório do benchmark não existem no namespace; `PATH=/agent-bin`.
- `go test`, `pytest`, `npm`, `node`, `ruby`, `bundle` executam no sandbox (equivalência de runners por linguagem).
- `sidecar index` e o hook (`SIDECAR_ENGINE=v2` e `v3`) rodam no sandbox; o checkout fica limpo (estado só em `/sidecar-home`). O hook V3 abstém-se em “bom dia!” e entrega contexto + linha `ON DEMAND` numa alteração de contrato; `sidecar explore` roda pelo PATH do braço V3.

Itens auditados sem mudança: respostas condicionais do usuário simulado, custo/tokens/cache desconhecidos como `None`, timeout como falha, ordem por chamada/mensagem/thread (testes do Codex cobrem; revisados). **Não validado:** retomada real de sessão (`--resume`) e entrega de hook com cliente autenticado — exige execução paga.

**Nota de integridade:** ao inventariar classes do corpus para o E2, um comando leu `task.json` de todas as tarefas e imprimiu apenas contagens de `class`/`split` (já presentes no manifesto selado), incluindo as 9 de holdout. Nenhum prompt, oráculo, patch ou resultado de holdout foi exibido ou usado. O ledger do holdout continua vazio.

## 2. P0-B — Engine V3 (`internal/engine`)

Decisão determinística sobre `retrieve.Search` (V2 + política adaptativa), sem indexador paralelo, LLM, rede ou embeddings:
- `TaskSignals`: intenção V2, edição (negações e citações removidas), vocabulário de software PT/EN, pista de repositório (“onde”, “which handler”, “este projeto”), localizadores (caminho, identificador, stack frame, código entre crases) **verificados no índice**, suporte léxico da melhor evidência, cobertura, fração de termos presentes no repo, risco de contrato (“keep/preserve/sem alterar/assinatura”).
- `RelevanceGate`: `relevant` / `ambiguous` / `irrelevant`. Abstém-se só com evidência clara de irrelevância; na faixa ambígua entrega LEAN mínimo (2 itens, 200 tokens) marcado `low_confidence`. Conversa (saudação, agradecimento, emoji, pedido de redação sem assunto de código) abstém-se **sem rodar a recuperação**.
- `EffortRouter`: `locate` → LEAN (5 itens/420 tokens); pergunta de fluxo → EXPLORE_AVAILABLE com orçamento LEAN; edição/debug → BRIEF (800); alteração de contrato → VERIFY (1200 + dica do explore); multiarquivo → EXPLORE_AVAILABLE (1200). Brief real só com `SIDECAR_FEATURES=brief` (B12 favorável, B15 não repetiu).
- `Decision`: ação, classe, relevância, orçamento, provedores usados/pulados, itens incluídos/excluídos com motivo, tokens estimados. Log do hook guarda só metadados da decisão.

### Protocolo E1 (sem tuning no conjunto de medição)

1. Baseline V2 reproduzido (B02/B03/B04 idênticos aos registrados).
2. **Conjunto independente congelado antes de qualquer código V3** (`031462e`, `bench/v3/e1_independent.json`, sha256 `a7fe2a49…5f09`): 30 positivos (ruído EN, voz PT longa, edições negadas, comandos citados, identificadores, mudanças de contrato) e 40 negativos (saudação, mundo real com palavras de software ou do domínio do repo, programação geral, meta, recurso ausente, emoji). Mesmo autor do código: viés de autoria.
3. Regras escolhidas só com splits de desenvolvimento (negativos `dev`, ten-repos `development`, fixtures `tune`, longos `dev`). Uma revisão (faixa `ambiguous`) foi feita **depois de ver o agregado do split `eval` interno** (já não-cego), por perder 11 positivos; registrada aqui. O conjunto independente foi rodado uma vez com o motor congelado (`6e4ff2b`); nenhum ajuste depois.

### Resultados E1 (dev+validation; holdout excluído; `bench/results/v3-e1-2026-11-09`)

| Suíte | FP negativos V2 → V3 | Hit@5 V2 → V3 | Positivos vazios V2 → V3 | Tokens positivos V2 → V3 | p95 ms V2 → V3 |
|---|---|---|---|---|---|
| Negativos difíceis B03 (180) | **30 → 14** (16,7% → 7,8%) | — | — | — | 30 → 17 |
| Longos B04 (16+6) | 3/6 → 1/6 | 87,5% → 87,5% | 0 → 0 | 465 → 340 | 37 → 27 |
| Ten-repos B02 (40+10) | 0 → 0 | 97,5% → 97,5% | 0 → 0 | 418 → 299 | 72 → 64 |
| **Independente (30+40), nunca visto** | **13/40 → 9/40** (32,5% → 22,5%) | 83,3% → 83,3% | 0 → 0 | 424 → 294 | 41 → 30 |
| Fixtures internas tune+eval (40+10) | 0 → 0 | **67,5% → 57,5%** | **5 → 13** | 275 → 146 | 16 → 14 |
| Cenários V2 (13+2) | 0 → 0 | 100% → 100% | 0 → 0 | 209 → 172 | 14 → 10 |

Leitura: a V3 reduz falsos positivos e tokens sem perder recall nos repositórios externos, mas **perde recall nas fixtures internas de prompts curtos de domínio** (“limite de cargas simultâneas no despacho”), onde o suporte léxico se confunde com negativos. Meta FP < 5% **não atingida** (7,8% em B03, 22,5% no independente). Dos 9 FPs independentes, 6 são a faixa `ambiguous` (≤ 200 tokens); tokens em negativos caem 119,8 → 37,2. Baselines no mesmo E1: V3 supera BM25 bruto (FP independente 23/40, Hit@5 66,7%) e lexical+grafo. RepoMap do Aider: ver B19.

## 3. Outras correções com efeito em todos os motores

- **L03 resolvida** (`22bd3d0`): funções JS/TS atribuídas (`res.send = function send(…)`, `exports.x = () => …`, `Foo.prototype.bar = …`) agora são símbolos; `extractVersion` 3 força reextração. Efeito na V2: +4 positivos com Hit@5 (3 de Express no independente), −1 (LP-15), +2 FPs em Express.
- **Não-determinismo do ranking** (`0ee01ca`): empates entre símbolos homônimos eram desempatados por ID de linha do SQLite, que depende da ordem de escrita do indexador; 10 listas de resultado mudavam entre 3 construções do índice (R05-1), agora 0. Desempate por caminho/linha/tipo/nome.

## 4. P1 — `sidecar explore`

Contrato e limites em [docs/explore.md](../explore.md). Primeiro teste real expôs: (a) L03 (Express sem `res.send` no índice) e (b) a regra V2 “identificador exato domina” derrubando vizinhos quando `ETag` casava com uma constante de teste. O explore usa opções aditivas do `retrieve` (`ExtraExactKeys` para `res.send`→`send`, `NoExactDominance`); hook V1/V2/V3 inalterados.

**E2 offline (B18, sem modelo, 7 tarefas de navegação, mesmas perguntas para todos):** fatos do oráculo presentes no que cada braço entrega — hook V2 14/26 (1/7 tarefas completas, 12,4 k chars), hook V3 13/26 (1/7, 8,1 k), **sidecar explore 22/26 (4/7, 38,9 k)**, **codegraph explore 1.6.2: 24/26 (6/7, 114,3 k)**. Leitura: o CodeGraph cobre mais fatos (Monolog e Sinatra completos), o Sidecar entrega ~1/3 dos caracteres; nenhum domina. Falhas do Sidecar: Monolog (handler/`write`), Sinatra (`route!`), Click (`parse_args`, que o CodeGraph também perde). É condição para economizar exploração, **não** prova de resposta correta nem de economia.

## 5. P0-C — suficiência e custo por origem

`bench/lab/sufficiency.py` (implementação independente do princípio “próximo ato” do CodeGraph): para cada entrega (contexto do hook ou resultado do explore) classifica o primeiro ato substantivo **posterior** na mesma thread: `explore_again`, `read_returned` (alocação), `read_named`/`read_not_returned` (recall), `search`, `moved_on`, `errored`; ignora chamadas da mesma mensagem (`concurrent`) e bookkeeping; delegação é julgada pelo primeiro ato do subagente; leitura por shell conta. Sinais de sessão: releituras redundantes, leitura de arquivo entregue após edição (contexto desatualizado), entregas sem efeito. Tokens por origem: contadores do servidor como medidos; hook/explore/resultados de ferramenta estimados por caracteres e marcados; ausente = `None`. Persistência só com caminhos em hash. `moved_on` nunca é reportado como acerto: o scorecard mostra ao lado da taxa de aprovação.

## 6. Pendências e próximos passos (ordem)

1. **Autorização + teto de gasto** para pilotos pagos. Sem isso nada abaixo roda.
2. E2 navegação piloto (development, 4 tarefas): `OFF,V2,V3,V3-hook` × 2 reps, Haiku; depois Sonnet. Comando abaixo.
3. Braços CodeGraph prontos: `--codegraph DIR` (pacote `npm install --prefix DIR @colbymchenry/codegraph@1.6.2`, versão gravada em `meta.json`); `codegraph init` roda no sandbox como setup e `.codegraph/` entra na linha de base de escopo; `CODEGRAPH_TELEMETRY=0` (o CodeGraph coleta telemetria por padrão); `CG` permite `Bash(codegraph explore|node|query|context:*)` (paridade com `sidecar explore`); `CG-mcp` passa `--mcp-config` com `codegraph serve --mcp` e só esse servidor é aceito pela checagem de contaminação. OFF/V2/V3 não alcançam o CodeGraph (máscara). Falta: execução real com cliente autenticado.
4. E3 edição: `OFF,V2,V2+brief,V3` no corpus v2 (32 tarefas, holdout fora), piloto de dificuldade antes de dimensionar.
5. Reduzir FN de prompts curtos de domínio sem reabrir FPs: exige **novo** conjunto independente (o atual já foi usado).
6. Explore: recall em Monolog/Sinatra; medir cold/reindex; E2E real.

Comandos (dry-run sem `--yes-spend`; nada pago foi executado):

```bash
go build -o /tmp/sidecar ./cmd/sidecar
npm install --prefix /tmp/cg @colbymchenry/codegraph@1.6.2
LAB_CORPUS=bench/lab/corpus-nav python3 bench/lab/lab.py run --agent claude --model claude-haiku-5-5 \
  --sidecar /tmp/sidecar --codegraph /tmp/cg --arms OFF,V2,V3,V3-hook,CG,CG-mcp --reps 2 --splits development \
  --out-dir /tmp/lab-e2 --max-budget 0.5 --max-total-usd <TETO>            # + --yes-spend quando autorizado
python3 bench/lab/lab.py run --agent claude --model claude-haiku-5-5 --sidecar /tmp/sidecar \
  --arms OFF,V2,V2+brief,V3 --reps 1 --splits development,validation --out-dir /tmp/lab-e3 --max-budget 1 --max-total-usd <TETO>
python3 bench/lab/lab.py report /tmp/lab-e2/runs.jsonl --md /tmp/lab-e2/scorecard.md
```

## 6b. Falha encontrada na revisão própria

Ao adicionar os braços CodeGraph, a atribuição de `SIDECAR_ENGINE`/`SIDECAR_FEATURES` ficou sob o ramo do CodeGraph: os braços V2/V3 rodariam o hook **V1** sem aviso. Encontrado antes de qualquer execução, corrigido em `d90f104` com regressão (`test_each_arm_sees_its_own_engine_and_tools_in_the_sandbox`, que falha no commit anterior).

## 7. Riscos e rollback

- V3 é opt-in; `SIDECAR_ENGINE` vazio/`v1` mantém V1 e `v2` a V2. Rollback: não definir `SIDECAR_ENGINE=v3` ou reverter os commits `6e4ff2b`, `cc2ba2f`.
- Mudanças que afetam todos os motores: extrator L03 (`extractVersion` 3 reindexa) e desempate estável. Rollback: `git revert 22bd3d0 0ee01ca` (o primeiro também remove o explore).
- Riscos: regras léxicas da V3 foram desenhadas pelo autor do código; o conjunto independente também. Recall menor em prompts curtos de domínio. Explore entrega ~4× mais caracteres que o hook; benefício líquido não medido.
