# Benchmarks do Sidecar: índice, nível de evidência e pistas de melhoria

Gerado de `bench/registry.json` (2026-11-09) por `python3 bench/consolidate.py`. **Não edite à mão**: edite o registro e regenere.

## Níveis de evidência

- **infra**: prova que uma peça funciona (teste, E2E de instalação); não mede ganho
- **offline**: recuperação/política medida sem agente; não é resultado de desenvolvimento
- **agentic**: agente real, tarefa de edição, oráculo independente; único nível que sustenta alegação de ganho, e só com IC e N adequado

## Benchmarks

| id | nível | pergunta | resultado | status | ressalva |
|---|---|---|---|---|---|
| B01-retrieval-internal | offline | A recuperação acha o gold em fixtures internas (tune/eval/holdout)? | gate mínimo e regressão; sem alegação de utilidade | referência de regressão | viés de autoria; holdout interno já foi visto |
| B02-retrieval-external | offline | A recuperação acha o gold em 10 repositórios reais fixados? | V2 Hit@1 50,0%, Hit@5 97,5%, MRR 0,694 (reproduz o PR #6) | referência | 40 casos, localização apenas |
| B03-hard-negatives | offline | A V2 injeta contexto em prompts fora de assunto? | falsos positivos dev+validation 28/180 (15,6%), igual ao PR #6 | referência | holdout deste conjunto não é cego |
| B04-long-prompts | offline | Prompts longos/ruidosos (voz, PT-BR) recuperam e se abstêm? | V2: Hit@5 87,5% nos positivos longos; **50% de falsos positivos** nos negativos longos (3/6) | regressão; não cego | N=22; uma perda = 6 pontos |
| B05-replay | offline | Algum parâmetro da confiança/abstenção melhora a recuperação sem perder recall? | H1 (≥6 termos exigem suporte ≥3): FP dev 15→11%, val 16,2→8,8%, perde 1 de 16 positivos longos → inconclusiva; H2 (exceção por IDF) rejeitada | inconclusivo; defaults mantidos | determinismo provado; grade pequena |
| B06-perf | offline | Quanto custa o pipeline por estágio (cold/warm, p50/p95/p99)? | V1 total p50 7,5 / p95 13,4 ms; V2 p50 13,7 / p95 56,7; V2+adaptive p50 10,5 / p95 58,6 (cauda sem ganho) | referência | em processo; sem custo do processo do hook nem do cliente; cold n=10 |
| B07-brief-tokens | offline | O brief injeta menos texto que o bloco padrão com a mesma recuperação? | tokens médios 417→340 (positivos), 454→385 (longos); Hit@5/FP idênticos | referência | menos tokens não prova economia para o agente |
| B08-groom | offline | A política pergunta só o que importa? | 0/17 perguntas desnecessárias; 8/11 perguntas corretas; 3 erros mantidos (A06, A07, A09) | regressão | autor do detector escreveu os casos; sem benefício líquido medido |
| B09-impact-history | offline | A análise de impacto nomeia o arquivo que o mantenedor editou junto? | estrutural+co-mudança 46% (chi), 63% (click), 49% (express) com 3–6 arquivos; baseline mesmo diretório 71%/96%/42% com 17/15/5 | referência | sem IC; 3 primeiros arquivos por commit; express estrutural 0% |
| B10-lab-pilot-dev1 | agentic | OFF/V1/V2 em tarefas de edição reais (piloto de desenvolvimento) | inconclusivo; V1/V2 vs OFF ≈ −12% tool calls/tokens/custo, IC cruza zero | SUPERADO por B12; rodou antes da correção de isolamento | possivelmente contaminado (agente podia ler o gabarito); sem traces |
| B11-lab-pilot-r3 | agentic | V2, V2+adaptive, V2+brief em 15 tarefas | inconclusivo; único IC fora de zero é tempo do brief vs V2 (−26%), não confiável (CPU compartilhada) | SUPERADO por B12; rodou antes da correção de isolamento | possivelmente contaminado; excluiu tarefas de localização e negativas |
| B12-lab-bench-clean | agentic | Qual o efeito de V1, V2 e dos componentes (adaptive, brief) em aprovação, tool calls, tokens, custo e tempo? | sem melhora comprovada: V2+brief vs OFF aprovação +8,6%, tool calls −4,8%, tokens −8,8%, custo −4,0% (todos IC95 cruzam zero); testes ocultos 225/225 em todos os braços (teto); 5 de 63 comparações "significativas" ≈ acaso | referência (agentic) para eficiência; não mede capacidade | um modelo, 15 tarefas, 3 reps; teto de capacidade; tempo de parede contaminado por CPU; R4/R5/R6 não exercidos |
| B13-install-e2e | infra | Instalar, disparar hooks reais e desinstalar preserva a configuração do usuário? | hooks e statusLine de terceiros preservados; UserPromptSubmit/PreToolUse/PostToolUse disparam; settings idêntico após uninstall | referência | PreCompact, Codex, Antigravity, upgrade e Windows/macOS não verificados |
| B14-studio-browser | infra | O Studio renderiza dados reais sem erro e sem executar conteúdo hostil? | 0 erros de console, 0 requisições de rede, sem rolagem horizontal; payload hostil exibido como texto | referência | sem linha do tempo por sessão |
| B15-lab-two-turn | agentic | As notas de impacto (R5) e de pergunta (R4) mudam o resultado com um usuário simulado na segunda volta? | inconclusivo (N=6): nota de impacto entregue em 3/6 execuções e de pergunta em 2/6; falso positivo real do R4 (verbo negado) corrigido | parcial; não é referência | 6 tarefas; usuário simulado; rodada interrompida; sem holdout |
| B16-corpus-v2 | infra | As 12 tarefas difíceis novas têm oráculo satisfazível e não vacuoso? | 12/12: o oráculo falha na base e passa com o patch de referência; manifesto com 32 tarefas validadas | referência de infraestrutura | dificuldade projetada, nunca medida com agente; autoria por subagentes |
| B17-v3-relevance-router | offline | A Engine V3 (RelevanceGate + EffortRouter, opt-in) reduz contexto indevido sem perder recall em relação à V2? | FP B03 30/180→14/180; longos 3/6→1/6; independente 13/40→9/40 com Hit@5 igual (83,3%) e 0 positivos vazios; tokens nos positivos −29% a −31%; fixtures internas curtas de domínio perdem Hit@5 67,5%→57,5% (FN 5→13) | inconclusivo para promoção; V3 continua opt-in | regras desenhadas pelo autor do código; uma revisão após ver o agregado do split eval interno (não-cego); meta FP<5% não atingida; nenhum efeito agentic medido |
| B18-explore-fact-coverage | offline | Os fatos exigidos por um oráculo de navegação chegam ao agente pelo hook V2/V3, pelo sidecar explore ou pelo codegraph explore (mesmas tarefas), e a que custo? | fatos presentes: hook V2 14/26 (12,4k chars), hook V3 13/26 (8,1k), sidecar explore 22/26 com 4/7 tarefas completas (38,9k), codegraph explore 1.6.2 24/26 com 6/7 (114,3k) | referência offline; nenhuma economia agentic demonstrada | fato presente não é resposta correta; N=7; perguntas escritas pelo autor do Sidecar; CodeGraph via CLI (mesma saída da ferramenta MCP), telemetria desligada; nenhum resultado agentic |
| B19-aider-repomap-baseline | offline | Como o RepoMap do Aider (prompt como identificadores mencionados, sem arquivos no chat) se compara à V3 nos mesmos casos? | RepoMap Hit@5 30%/37,5%/20% (ten/longos/independente) vs V3 97,5%/87,5%/83,3%; RepoMap não se abstém (FP 100%) e entrega ~1000 tokens por prompt | baseline de referência | RepoMap foi desenhado como mapa com arquivos no chat, não como recuperação por prompt; uso aqui é o modo sem chat |
| B20-ranking-determinism | infra | O ranking é idêntico entre construções independentes do índice? | antes: 10 listas de resultado diferiam (caso R05-1, empates entre homônimos desempatados por ID de linha); depois: 0 | corrigido | o teste sintético não reproduz o caso real; a prova é a comparação das 3 construções |

### Dados e comandos

**B01-retrieval-internal** — dados: bench/cases.json (71 casos, 3 fixtures); escrito pelo autor do código  
comando: `go run ./cmd/sidecar-eval --cases bench/cases.json`  
artefatos: `bench/cases.json`

**B02-retrieval-external** — dados: bench/ten_repos_cases.json (40 positivos, commits fixados)  
comando: `bash bench/agentic/setup_10_repos.sh /tmp/sidecar-real-bench && go run ./cmd/sidecar-eval --external-cases /tmp/sidecar-real-bench/ten-repos-manifest.json`  
artefatos: `bench/ten_repos_cases.json`

**B03-hard-negatives** — dados: bench/negatives (240 prompts × 10 repos; holdout antigo foi visto)  
comando: `python3 bench/negatives/make_manifest.py --checkout DIR OUT && go run ./cmd/sidecar-eval --external-cases OUT`  
artefatos: `bench/negatives/hard_negatives.json`

**B04-long-prompts** — dados: bench/r2/long_prompts.json (16 positivos + 6 negativos longos; escritos DEPOIS de ver o modo de falha)  
comando: `python3 bench/r2/make_manifest.py --checkout DIR OUT && go run ./cmd/sidecar-eval --external-cases OUT`  
artefatos: `bench/r2/long_prompts.json`, `docs/eval-r2-2026-11-08.md`

**B05-replay** — dados: bench/replay/params.json sobre B02/B03/B04 (holdout excluído por padrão)  
comando: `go run ./cmd/sidecar-eval --external-cases OUT --replay bench/replay/params.json`  
artefatos: `bench/replay/params.json`, `docs/eval-r2-2026-11-08.md`

**B06-perf** — dados: 10 repositórios externos, n=540 chamadas warm por engine  
comando: `go run ./cmd/sidecar-eval --external-cases OUT --perf 10`  
artefatos: —

**B07-brief-tokens** — dados: B02 e B04  
comando: `go run ./cmd/sidecar-eval --external-cases OUT (método 'v2 + task brief')`  
artefatos: `docs/task-brief.md`

**B08-groom** — dados: bench/groom_cases.json (28 casos rotulados antes de rodar; fixture interna)  
comando: `go run ./cmd/sidecar-eval --cases bench/groom_cases.json --groom`  
artefatos: `bench/groom_cases.json`, `docs/grooming.md`

**B09-impact-history** — dados: histórico real de chi, click e express (commits de 2–6 arquivos)  
comando: `go run ./cmd/sidecar-eval --impact-history REPO --impact-commits 40`  
artefatos: `docs/change-impact.md`

**B10-lab-pilot-dev1** — dados: 8 tarefas development × 3 braços × 1 rep; claude-haiku-5-5; US$ 0,177  
comando: `python3 bench/lab/lab.py run --arms OFF,V1,V2 --splits development`  
artefatos: `bench/results/lab-dev1-2026-11-08`

**B11-lab-pilot-r3** — dados: 15 tarefas × 4 braços × 1 rep; 11 de 60 execuções excluídas (resposta em ~1 s sem modelo servido; causa desconhecida)  
comando: `python3 bench/lab/lab.py run --arms OFF,V2,V2+adaptive,V2+brief`  
artefatos: —

**B12-lab-bench-clean** — dados: 15 tarefas (dev+validation) × 5 braços × 3 repetições = 225 execuções, todas comparáveis; claude-haiku-5-5; US$ 1,75; isolamento corrigido; holdout intocado  
comando: `python3 bench/lab/lab.py run --arms OFF,V1,V2,V2+adaptive,V2+brief --rep-offset N`  
artefatos: `docs/benchmarks-2026-11-08.md`, `bench/results/lab-bench-clean-2026-11-08`

**B13-install-e2e** — dados: Claude Code 2.1.319 real, config isolada  
comando: `ver bench/results/r7-install-e2e-2026-11-08.md`  
artefatos: `bench/results/r7-install-e2e-2026-11-08.md`

**B14-studio-browser** — dados: Chromium via Playwright; 7 abas; viewport desktop e 390 px  
comando: `ver docs/studio.md`  
artefatos: `docs/studio.md`

**B15-lab-two-turn** — dados: 6 tarefas completas × 5 braços × 1 repetição = 30 execuções (rodada interrompida por limite de uso); protocolo de duas voltas; claude-haiku-5-5; US$ 0,61  
comando: `python3 bench/lab/lab.py run --turns 2 --arms OFF,V2,V2+brief,V2+impact,V2+groom`  
artefatos: `bench/results/lab-two-turn-2026-11-08`, `docs/benchmarks-2026-11-08.md`

**B16-corpus-v2** — dados: 12 tarefas (chi, click, express, sinatra; 4 dev, 4 validation, 4 holdout selado); reexecutadas por mim, não só pelos autores  
comando: `python3 bench/lab/lab.py validate-task <ID>`  
artefatos: `bench/lab/corpus/manifest.json`

**B17-v3-relevance-router** — dados: B02/B03/B04 dev+validation, fixtures internas tune+eval, cenários V2 e conjunto independente novo (bench/v3/e1_independent.json, 30 positivos + 40 negativos, congelado antes do código V3); holdout excluído  
comando: `go run ./cmd/sidecar-eval --external-cases OUT --out R.json --v3-decisions D.json && python3 bench/v3/e1_report.py --base 'v2 (intent + contract + packer)' --new 'v3 (relevance gate + effort router)' suite=R.json`  
artefatos: `bench/results/v3-e1-2026-11-09/e1-v2-vs-v3.md`, `bench/results/v3-e1-2026-11-09/e1-v2-vs-v3.json`, `bench/v3/e1_independent.json`

**B18-explore-fact-coverage** — dados: bench/lab/corpus-nav: 7 perguntas de fluxo (chi, click, express, hono, monolog, serde-json, sinatra), 26 fatos verificados por grep antes de qualquer execução  
comando: `python3 bench/v3/e2_offline.py --sidecar BIN --checkout DIR --codegraph node_modules/.bin/codegraph`  
artefatos: `bench/results/v3-e1-2026-11-09/e2-offline.md`, `bench/results/v3-e1-2026-11-09/e2-offline.json`, `bench/lab/corpus-nav/manifest.json`

**B19-aider-repomap-baseline** — dados: mesmas suítes externas do B17; aider-chat 0.86.2 em venv isolado, map-tokens 1024  
comando: `$AIDER_PY bench/v3/repomap_baseline.py MANIFEST OUT && python3 bench/v3/e1_report.py --base 'aider repomap' --new 'v3 (relevance gate + effort router)' suite=COMBINED.json`  
artefatos: `bench/results/v3-e1-2026-11-09/e1-repomap-vs-v3.md`, `bench/results/v3-e1-2026-11-09/repomap-independent-outcomes.json`, `bench/v3/repomap_baseline.py`

**B20-ranking-determinism** — dados: ten-repos, todos os métodos do sidecar-eval, 3 construções do índice antes e depois da correção  
comando: `go run ./cmd/sidecar-eval --external-cases OUT --out det-N.json (3x) e comparar listas top`  
artefatos: `internal/retrieve/determinism_test.go`

## Pistas de melhoria (insumos do V3)

Cada pista aponta o benchmark que a sustenta e o que mediria a confirmação. São hipóteses, não decisões.

| id | origem | problema observado | hipótese para o V3 | como confirmar | prioridade |
|---|---|---|---|---|---|
| L01 | B02, B04 | Lacuna de dicionário PT→EN: 'alíquota' sem tradução derruba o caso E17; nenhum parâmetro de confiança o separa de falsos positivos | Derivar equivalências do próprio vocabulário do repositório (comentários, docs, nomes) em vez de só dicionário estático | B02/B04 mais um conjunto novo de PT-BR; medir Hit@5 sem subir FP | alta |
| L02 | B03, B04 | Prompts longos fora de assunto vazam (50%); contagem de termos não separa âncora rara de termo genérico | Sinal de especificidade por raridade do termo no vocabulário de NOMES do repositório (não só texto) | B04 (N maior) + B03 sem perder positivos longos | alta |
| L03 | B09 | Grafo estrutural não resolve funções atribuídas (`res.x = function`): 0% em express | Extrator reconhecer definições por atribuição em JS/TS; combinar com co-mudança como sinal próprio | B09 em express e outro repositório JS | média |
| L04 | B08 | Clarificação perde pedidos de valor sem palavra de limite (LateFeePercent) e alvos ambíguos sem candidatos comparáveis | Detectar parâmetros numéricos pelas constantes do repositório e comparar candidatos por especificidade, não só por score | B08 ampliado com casos externos e rótulos de terceiros | média |
| L05 | B10, B12 | Escopo estrito do corpus reprova agentes que só adicionaram testes; tarefas aparecem 'não resolvidas' por calibração, não por qualidade | Corpus v2: testes permitidos por padrão; reportar sempre aprovação estrita e testes ocultos | B12 com as duas métricas (já reportadas) | alta (infra de medição) |
| L06 | B08, B12 | Grooming (R4) e intervenção (R6) não podem ser avaliados sem canal de duas vias com o usuário/agente | Desenhar um canal verificado (pergunta nativa interativa e entrega no meio do turno) e um protocolo com usuário simulado e respostas pré-definidas | novo benchmark agentic multi-turno; E2E por cliente | alta (bloqueia R4/R6) |
| L07 | B12 | Tokens do agente são dominados por leitura de cache/arquivos (centenas de milhares), enquanto o contexto injetado tem ~1,5 mil caracteres | Atacar o custo onde ele está: reduzir releituras e resultados de ferramenta, não só melhorar o contexto inicial | B12: correlação entre tokens e chamadas por classe de tarefa; análise de hot spots | a decidir com os dados finais |
| L08 | B10, B11 | Execuções que voltam em ~1 s sem modelo servido (causa desconhecida) removem tarefas inteiras da comparação | Registrar stderr e o evento de resultado do cliente em toda execução para diagnosticar; repetir automaticamente execuções sem resposta | B12 não deve ter exclusões; se tiver, investigar | média |
| L09 | B12 | Isolamento do agente sem sandbox de SO: código que o agente roda lê o gabarito | Executar braços em contêiner ou usuário separado | teste de sonda que tente ler o oráculo via código executado | alta para o holdout final |
| L10 | B12, B03 | No prompt negativo de express, V1/V2 injetaram contexto em 12/12 execuções (886–1103 chars) mesmo sendo conversa sem relação | Portão de abstenção por tipo de pedido (conversacional/sem alvo no repositório) antes de qualquer injeção | B03/B04 e um braço negativo em B12 com mais tarefas negativas | alta |
| L11 | B12 | Testes ocultos passaram em 225/225 execuções: o corpus não discrimina capacidade (8 de 15 tarefas resolvidas por todos) | Corpus v2 com tarefas mais difíceis e mais por classe; reservar o holdout para ele | B12 repetido com o corpus v2: algum braço deve falhar teste oculto | alta (infra de medição) |
| L12 | B12 | Efeito do contexto é heterogêneo por tarefa nos dois sentidos (ajuda CHI-CC-01, prejudica CHI-DBG-01 e CLK-CC-01) | Política seletiva por classe de tarefa e especificidade, medida por tarefa com repetições suficientes | B12 com ≥ 5 repetições nas tarefas que dividem os braços | alta |
| L13 | B15, B08 | O R4 não detecta pedido vago de estilo (CLK-AMB-01) e pode tratar verbo negado como edição (corrigido) | Derivar alternativas do próprio repositório (formas existentes de apresentar/validar) antes de perguntar; tratar cláusulas proibidas como leitura | B08 ampliado + corpus com ≥ 5 tarefas ambíguas detectáveis | média |
| L14 | B15, B16 | A rodada de duas voltas e o holdout final ficaram incompletos por limite de uso; o corpus v2 nunca foi rodado com agente | Cachear dependências por tarefa (npm/bundle; venv não copia) e rodar a rodada completa e o holdout uma única vez | B15 com ≥ 15 tarefas e ≥ 3 repetições; holdout com --final | alta (fecha lacunas de medição) |
| L15 | B17 | Prompts curtos na linguagem de domínio do repositório têm o mesmo perfil léxico de negativos (1-2 termos casados); a V3 perde 8 positivos internos | Sinal de especificidade por raridade de NOMES do repositório (L02) ou vocabulário derivado de docs/comentários (L01), avaliado em conjunto independente NOVO | novo conjunto independente; B17 não pode mais ser usado para escolher | alta |
| L16 | B18 | Explore não entrega handler/write do Monolog, route! do Sinatra e parse_args do Click; entrega ~4x mais caracteres que o hook | Expandir cadeia por chamadas a partir do melhor seed (callers/callees com corpo) e cortar símbolos sem termo da consulta | B18 + E2 agentic com suficiência por entrega | média |
