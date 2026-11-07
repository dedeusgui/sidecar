# Plano de avaliação: valor real do Sidecar para agentes

**Status: desenho de protocolo; ainda não executado.** O benchmark atual em `docs/benchmark.md` mede ranking offline em fixtures pequenas. Ele **não** comprova economia de tempo, redução de rounds ou maior sucesso de um agente. Esses resultados devem continuar separados.

## Hipóteses falsificáveis

- **H1 — localização:** com Sidecar ligado, o agente identifica a primeira evidência correta (arquivo/símbolo + local) mais cedo, em média/mediana, que sem Sidecar.
- **H2 — esforço:** para mesma tarefa e modelo, há menos buscas/leitura redundante, tool calls e rounds até a primeira evidência válida.
- **H3 — desfecho:** o agente mantém ou melhora a taxa de solução correta com testes independentes; contexto mais rápido mas incorreto é pior.
- **H4 — custo total:** economias em pesquisa/modelo superam overhead de hook, indexação e tokens injetados.
- **H5 — abrangência:** benefícios persistem entre linguagens, mono/multi-repo, monorepos, repositórios sem Git, nomes PT/EN, gramáticas não suportadas e tamanhos. **Não** prometer universalidade antes de demonstrá-la.
- **Nulo plausível:** Sidecar pode atrasar, introduzir distração, falhar no primeiro prompt ou ajudar somente certas classes de tarefa.

## Dois níveis independentes

**Nível A — retrieval determinístico e sem LLM (executável hoje):**
- Manter `go run ./cmd/sidecar-eval --cases bench/cases.json` como regressão de desenvolvimento.
- Comparar `grep/rg` lexical, SQLite BM25 bruto, BM25 + normalização/tradução, + grafo, + Git; avaliar também custo por componente e evidência correta.
- Medir Precision@K, Recall@K, Hit@1/5, MRR, NDCG se houver relevância graduada, cobertura, falso positivo para consultas negativas, p50/p95/p99 cold/warm, tamanho do índice, memória, indexação e tokens de contexto.
- Nenhum conjunto ajustado depois de sua inspeção pode continuar rotulado `holdout` cego.

**Nível B — experimento pareado com agentes (a construir):**
- Condição **OFF**: mesmo agente, ferramentas, permissões, modelo, prompt e instrumentação, mas sem bloco Sidecar. Condição **ON**: Sidecar operacional com hook real; registrar se de fato injetou contexto.
- Rodar em worktree limpo por tentativa, em SHA fixo, com ferramentas/versões fixadas. Não reutilizar memória de conversa, caches de resposta ou arquivos alterados entre condições; separar índices warm/cold e atribuir custos de setup.
- Balancear ordem das execuções ON/OFF por tarefa (AB/BA), alternar ordem dos modelos e usar repetições sob orçamento; inferência não é determinística. Não comparar saída de Haiku em um braço contra Flash em outro.
- Mesmas restrições e timeout/turn-budget nas duas condições. Coletor de métricas idêntico em ON/OFF, sem registrar prompts, código-fonte ou segredos; nenhuma instrumentação pode forçar o agente a usar Sidecar.
- Anotar o **gold** antes da execução: SHA, tarefa, critérios de sucesso, caminho(s)/funções, testes oracle independentes, dificuldade e skills necessárias; revisores cegos para ON/OFF quando possível.

### Métricas primárias e secundárias

| Métrica | Definição operacional |
|---|---|
| **TTFE (time to first evidence)** | Tempo de relógio do envio do prompt até a primeira identificação **correta e verificável** de símbolo/arquivo+trecho gold; ausente = censurada/falha, não zero |
| **Tool calls para evidência** | Número de chamadas de busca/leitura/navegação até a primeira evidência gold |
| **Rounds para evidência** | Número de ciclos de resposta do modelo entre observações/ações até chegar à evidência correta; contar separadamente mensagens, tool-use e novas inferências conforme trace do cliente |
| **Resolução correta** | Teste de aceitação/patch válido sob orçamento pré-definido; não basta resposta plausível |
| **Custo total em tokens** | Entrada + saída, incluindo contexto injetado; divulgar limites da medição por clientes de assinatura |
| **Latência total e overhead** | Tempo até evidência e conclusão, tempo do hook, cold/warm index, latência do modelo e ferramentas quando disponíveis |
| **Desvios / danos** | Busca em arquivo errado, contexto irrelevante, falso positivo, prompt injection, falha de hook, regressão de qualidade |
| **Economia relativa** | (mediana OFF − mediana ON)/mediana OFF e razão OFF/ON; relatar intervalos e valores absolutos |

Não substituir sucesso por Hit@5. Não alegar ganho **do modelo** se só houve busca mais rápida do algoritmo. Reportar distribuição por tarefa e modelo, não apenas média global. Registrar timeouts/erros da plataforma como casos censurados e apresentar denominadores.

### Planejamento de custos — contratos baratos, experimentos curtos

Painel de execução inicialmente **manual e opt-in**:
- Anthropic: **Haiku**; OpenAI/Codex: **Luna** (nome/alias usado pelo operador; fixar ID real e esforço); Google/Antigravity CLI: **Gemini Flash**. Esses nomes não garantem o mesmo limite, preço, versão nem grau de esforço. Registrar versão exata, CLI, modo de autenticação, reasoning/effort e ferramentas efetivamente usadas. Não pressupor permissão de automação ou disponibilidade de API a partir de assinatura.
- Modelos mais caros (ex.: **Tera**) podem ajudar a **desenvolver o harness**, nunca contam como resultados do conjunto barato e não participam de benchmark cego.
- **Fase 0, sem chamadas pagas:** validar corpus, gold, linhas, scripts de replay e medição offline; rodar mocks/fixture.
- **Piloto mínimo:** 4 tarefas × 3 modelos × 2 braços = **24 execuções**, uma tentativa por célula; apenas detectar falhas do protocolo, sem tirar conclusões de produtividade.
- **Expansão:** só após validar a instrumentação; adicionar repetições e tarefas em blocos de tamanho fixo. Uma segunda tentativa pode dobrar as execuções. Definir limite explícito **por sessão** de turnos, wall-time e tokens (se expostos) e **por rodada** de chamadas; abortar ao atingir qualquer teto.
- Registrar quotas disponíveis antes/depois, erros de rate limit e custo monetário apenas quando medido com dados reais. Não converter assinatura mensal em preço marginal inventado.
- Sem benchmark online automático em pull_request/CI; agendar/autorizar separadamente; nunca usar modelos pagos para testar funcionalidades determinísticas.

### Critério de validade

Bloquear divulgação de percentual de melhora quando: poucos pares, runs incompletas não reportadas, confusão de modelo/versão, corpus alterado entre ON e OFF, referência gold contaminada, warm index só em um braço, benchmarks usados no tuning, ou instrumentação assimétrica. Pré-registrar análise principal antes das runs. Preferir **bootstrap pareado** com intervalo de confiança por tarefa (não só por execução), relatório de n e variância; concluir `inconclusivo` quando apropriado.

## Matriz de repositórios propostos (ainda não coletados)

Seleção propositalmente heterogênea. Antes de usar, verificar licença, SHA, tamanho real, políticas de CI, build/testes, e se o projeto permite redistribuição da fixture. Clonar pinado, sem executar scripts arbitrários. Não guardar cópias não autorizadas em `bench/`.

| Candidato | Ecossistema / cenário que pode testar |
|---|---|
| [spring-projects/spring-petclinic](https://github.com/spring-projects/spring-petclinic) | Java/Spring; navegação entre controller/service/repository |
| [nestjs/nest](https://github.com/nestjs/nest) | TypeScript; módulos e referências indiretas |
| [fastapi/fastapi](https://github.com/fastapi/fastapi) | Python; decorators, rotas e testes |
| [gin-gonic/gin](https://github.com/gin-gonic/gin) | Go; interfaces/middleware |
| [dotnet/eShop](https://github.com/dotnet/eShop) | C#; solução com múltiplos serviços |
| [tokio-rs/axum](https://github.com/tokio-rs/axum) | Rust; **fallback**, gramática não suportada atualmente |
| [laravel/framework](https://github.com/laravel/framework) | PHP; **fallback**, gramática não suportada atualmente |
| [microsoft/vscode](https://github.com/microsoft/vscode) | Grande monorepo; carga e limites de indexação |
| Próprio [Sidecar](https://github.com/dedeusgui/sidecar) | Dogfooding e regressão; **não** usar como holdout independente |

Candidatos não constituem dataset pronto. Usar um estrato com arquivos duplicados, código gerado, nomes obfuscados, pastas sem Git e mistura de idioma para verificar degradação. Priorizar tarefas de localizar símbolo, rastrear chamada, identificar teste relevante e corrigir bug pequeno. Reservar tarefas complexas para a etapa posterior; congelar prompts antes de medir.

### Splits e proteção contra vazamento

- `development`: aberto para tuning e regressão, com labels conhecidos.
- `validation`: usado para escolher variantes, nunca para alegar resultado final.
- `blind-test`: novos repositórios/commits/tarefas com gold fechado e execução única após congelar release; resultados publicados uma vez, versões subsequentemente passam a ser test set histórico.
- Pin de dataset deve incluir URI, commit SHA, checksum da anotação, licença, linguagem, estrato, autor/revisor, data e versão do Sidecar.
- Distinguir offline benchmark, benchmark end-to-end e dogfooding em relatórios. Nenhuma generalização sobre “todos os repositórios” sem amostragem adequada.

## Estratégia de corpus: reutilizar benchmarks públicos, não refazer sua pesquisa

**Escolha principal:** usar datasets e métricas públicos para avaliar a recuperação local, mantendo um conjunto **pequeno e próprio** apenas para o harness de hooks e produtividade com modelos baratos.

| Fonte | Escopo verificado | Uso proposto |
|---|---|---|
| [Agent Retrieval Bench](https://agent-retrieval-bench.github.io/) e [release/código](https://github.com/eyuansu62/agent-retrieval-bench) | 427 casos, 25 repositórios, 5 subconjuntos (inclui `no-gold`) | **Primeiro benchmark offline**: recuperar arquivos de trabalho e testar seletividade sem LLM; reutilizar rótulos existentes |
| [ContextBench](https://contextbench.github.io/) e [dataset](https://contextbench.github.io/) | 1.136 tarefas, 66 repositórios, 8 linguagens, contextos gold anotados | Segundo benchmark: seletividade, recall/precision em nível de contexto e evolução para trajetórias de agente; recortar por custo |
| Fixtures locais (`bench/cases.json`) | 71 casos em pequenas fixtures de desenvolvimento; tune/eval/holdout históricos já inspecionados | Testes rápidos de regressão, **não** claim externo |
| Mini suíte agentic Sidecar | 4 tarefas × 3 modelos × ON/OFF = 24 runs no piloto | Validar hooks, TTFE, rounds e instrumentação, não produzir “placar publicável” |

**Reaproveitar labels não é repetir os experimentos da comunidade:** precisamos executar **o algoritmo Sidecar** contra os mesmos casos/commits fixados e medir seus outputs; não precisar produzir manualmente outro gold se a anotação pública satisfaz nossa tarefa. Não copiar resultado de um concorrente como se fosse resultado nosso. Não comparar scores calculados em configurações de corpus/métrica distintas.

Averiguar licença do dataset e repositórios upstream; [Agent Retrieval Bench documenta separadamente licença do avaliador e do código incorporado](https://huggingface.co/datasets/eyuansu71/agent_retrieval_bench/blob/main/DATA_LICENSE.md). Pin de release, SHA e checksum; baixar fora de `bench/` e fora do CI padrão, sem commits de corpus de terceiros.

### Determinismo e reprodutibilidade — duas garantias diferentes

- **Camada offline:** candidatos, classificação de intenção, evidências/packing e métricas de acerto devem ser estáveis para mesmos inputs, versão, configuração, commit e índice congelado. Ordenar empates, fixar semente quando houver amostragem, registrar versões. Latência, cache e hardware não são exatamente determinísticos; reportar distribuição.
- **Camada agentic:** o modelo pode variar mesmo com parâmetros idênticos; não prometer placar determinístico em cada execução. Registrar versões exatas, permissões, temperatura/seed se expostos, número de turnos e ordem AB/BA; repetir pares e relatar variância.
- **Isolamento dos braços:** além de `OFF` e `ON-v2`, medir `ON-v1` e, em ablação, `ON-v2 sem LSP`, `ON-v2 com LSP`, `ON-v2 sem sinais de sessão`. Não alterar múltiplas dimensões sem ablação.
- **Ferramenta de LSP:** ferramenta LSP disponível para o agente e LSP consultado internamente pelo Sidecar **são dimensões distintas**; manter iguais as capacidades disponíveis ao agente em ambos os braços ou explicitar experimento fatorial.
- **Sem telemetria assimétrica:** observador de eventos executa em ambos os braços, jamais entrega dicas extras ao controle OFF.

### Scorecard e metas *propostas*, não resultados

O relatório publicado deve destacar um **vetor** de métricas, não um único score opaco. `Pass@1 / success` é guardrail, `TTFE` e navegação são medidas de eficiência. Separar `first_correct_context_presented` (quando gold foi injetado/mostrado) de `first_correct_context_used_or_identified` (uso verificável em ação, trecho/patch ou avaliação gold), para não declarar vitória apenas porque Sidecar antecipou um snippet que o modelo ignorou.

**Critérios aspiracionais para futura amostra robusta, a validar e pré-registrar:** reduzir a mediana do tempo até evidência correta utilizada em **≥15%** e tool calls de busca/leitura até evidência em **≥20%** versus OFF; manter resolução de tarefa sem degradação significativa, não ampliar taxa de contexto enganoso, preservar p95 de hook warm **≤150 ms** onde é mensurável; reportar tokens/latência de setup. Esses limiares são *metas de produto*, não prova estatística nem gate do piloto de 24 execuções. Sucesso é binário por tarefa com oracle; não declarar “não inferior” usando somente quatro tarefas.

Relatório deve mostrar `N`, denominador, CI/intervalos, p50/p95, taxa de resolução, custo total, falhas e casos em que Sidecar **piorou**. Sem repetições suficientes: `inconclusivo`.

### Instrumentação de feedback (não contamina tarefa)

- Ver `docs/agent-feedback.md`: `PostToolUse` e equivalentes observam operações sem texto do prompt; associar ferramentas a evidências por ID e validar com gold.
- Autorrelato parametrizado pelo modelo pode ser pedido **após** conclusão e fora da rodada principal, como sinal fraco. Não obrigar o modelo a dizer “Sidecar ajudou” durante a tarefa, nem atualizar ranking em tempo real com sua opinião.
- Critério forte: gold anotado/aceitação independente; arquivo acessado ≠ relevância demonstrada.
- Registro padronizado de cobertura de eventos (`observed`, `unobserved`), sem contabilizar como zero o que não foi monitorado.

## Qualidade de entrega e custo da V2 completa

O objetivo primário da V2 é **tempo e custo por tarefa corretamente concluída**, não score isolado de ranking. Adicionar ao protocolo:
- **Contract fidelity**: se implementação, teste e documentação divergem, identificar e **citar fontes reais**, sem inventar precedência. Medir precisão de conflitos, falso conflito, omissão de fonte e decisão humana desnecessária.
- **Repository discovery**: acertar localização de contrato descrito vagamente em PT-BR/inglês; distinguir repo, worktree, branch e documentação stale.
- **Loop inefficiency**: buscas repetidas sem evidência nova, arquivos reabertos sem motivo relevante, pesquisas que retornam ao mesmo conjunto e revisitas após compactação. Sinais são diagnóstico; confirmar com gold/traço anotado.
- **Implementation quality**: solução testada, conformidade aos contratos existentes, regressões, mudança de arquivos irrelevantes, necessidade de intervenção humana e qualidade da documentação atualizada.
- **Total task burden**: tokens do hook, skills e ferramentas, tempo index/IO/LSP, rondas de modelo, walltime, erro por timeout/rate-limit, custo observado e transições de contexto.
- **Selective delivery**: proporção de mensagens com zero injeção, falso positivo contextual, contextos recebidos e **usados** comprovadamente, não apenas textos apresentados.
- **Ablação por módulo**: OFF / Sidecar v1 / V2 completo; V2 sem IntentRouter, sem ContractResolver, sem SessionObserver, sem LSP, sem memória. Controlar capacidades próprias do agente para comparação justa.

A suíte própria deve cobrir 10–15 cenários reproducíveis de uso pessoal (não usar repositórios privados sem consentimento para publicar resultados): prompt implícito, localização de regra, docs vs código, contrato/teste, busca circular, mudança multiarquivo, continuação de sessão, repo não convencional, PT/EN, ausência de evidência. Cada cenário precisa de gabarito, commit e oracle independente **antes** das execuções. Benchmark público permanece a base de generalização.

**Orçamento:** 4 tarefas × 3 modelos baratos × 2 braços = 24 execuções é somente qualificação do harness, **não meta de V2 nem amostra conclusiva**. Ao medir OFF / v1 / V2 são 4 × 3 × 3 = **36 execuções por repetição**. Repetir somente após testar instrumentação sem modelos, fazer orçamento e registrar IDs exatos de Haiku, Luna e Flash; limite por tarefa de tempo, turnos e recursos. Nenhuma execução paga automática no CI.

**Critério de decisão:** ganhos de TTFE, rounds ou tokens que reduzam a taxa de tarefas corretas são regressões; não reivindicar determinismo do modelo; usar bootstrap pareado por tarefa, incerteza, efeitos negativos e status `inconclusivo`. Metas de ≥15% TTFE utilizado e ≥20% buscas até evidência são **hipóteses pré-registráveis**, não resultados já demonstrados.

## Estrutura do harness agentic (parcialmente implementada, sem execuções)

**Estado (2026-11-06):** `bench/agentic/manifest.jsonl` (6 tarefas `verified` em spring-petclinic e go-chi/chi, 4 `draft`), `internal/benchagent` e `sidecar-eval --agentic-report`. O avaliador lê `runs.jsonl`, rejeita campos desconhecidos, pareia repetições e emite veredito só para tarefa verificada com ao menos 3 pares. **Nenhum agente real foi executado:** os 90 registros vêm de uma simulação instrumentada (`bench/agentic/instrumented_sim.py`), com política de busca fixa, descrita em [real-repos-ab-score.md](../bench/results/real-repos-ab-score.md). O runner real, o isolamento por worktree e a coleta de traces por cliente continuam pendentes. Ver [benchmark.md](benchmark.md), seção 4.

Desenho original, mantido como referência:

`bench/agentic/`: `manifest.jsonl` (tarefa + versão + gold), `run-manifest.json` (modelo/CLI/effort, orçamento, braço, seed/ordem), `traces/` privados locais, `report.json` e `report.md` derivados. Início com um runner de replay simples; **não** introduzir framework pesado nem abstrair três CLIs antes de observar seus eventos reais. Salvar metadados agregados por padrão; conteúdo bruto somente com opt-in em diretório privado e retenção definida.

## Critérios de aceitação do primeiro benchmark agentic

- Pelo menos 4 tarefas verificadas em 2+ repositórios, replicáveis em OFF/ON no mesmo commit.
- Trace auditável de início, hook injetado ou não, ferramentas, localização gold, sucesso/falha, tempos e uso reportado.
- Filtro de conteúdo sensível e replay com estado limpo.
- Métricas com denominador, n, condições, versões de modelo, limites e falhas; não chamar piloto de comprovação.
- Relatório explicita `Sidecar ajudou`, `Sidecar prejudicou` ou `inconclusivo` por tarefa.
