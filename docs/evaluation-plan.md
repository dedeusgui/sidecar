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
- Anthropic: **Haiku**; OpenAI/Codex: **Luna** (nome/alias usado pelo operador; fixar ID real e esforço); Google/Gemini: **Flash**. Esses nomes não garantem o mesmo limite, preço, versão nem grau de esforço. Registrar versão exata, CLI, modo de autenticação, reasoning/effort e ferramentas efetivamente usadas. Não pressupor permissão de automação ou disponibilidade de API a partir de assinatura.
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

## Estrutura proposta para o harness futuro (não implementada)

`bench/agentic/`: `manifest.jsonl` (tarefa + versão + gold), `run-manifest.json` (modelo/CLI/effort, orçamento, braço, seed/ordem), `traces/` privados locais, `report.json` e `report.md` derivados. Início com um runner de replay simples; **não** introduzir framework pesado nem abstrair três CLIs antes de observar seus eventos reais. Salvar metadados agregados por padrão; conteúdo bruto somente com opt-in em diretório privado e retenção definida.

## Critérios de aceitação do primeiro benchmark agentic

- Pelo menos 4 tarefas verificadas em 2+ repositórios, replicáveis em OFF/ON no mesmo commit.
- Trace auditável de início, hook injetado ou não, ferramentas, localização gold, sucesso/falha, tempos e uso reportado.
- Filtro de conteúdo sensível e replay com estado limpo.
- Métricas com denominador, n, condições, versões de modelo, limites e falhas; não chamar piloto de comprovação.
- Relatório explicita `Sidecar ajudou`, `Sidecar prejudicou` ou `inconclusivo` por tarefa.
