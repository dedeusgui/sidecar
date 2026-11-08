# Sidecar V3 — referências externas auditadas, evidências e armadilhas

**Consulta técnica:** 2026-11-08. **Natureza:** fontes de engenharia e comparativos candidatos. Não são dependências, benchmarks feitos pelo Sidecar, permissão de copiar código nem promessas de ganho. Leia [o contrato V3](README.md) e [o catálogo geral](../reference-implementations.md) antes de usar. Preferir **1–2 referências por módulo** e anotar no PR arquivos, SHA, licença por arquivo, decisão de reuso e teste de paridade.

## 1. Índice de referências com commit fixado

O commit abaixo é o HEAD da referência **verificado na pesquisa**, não necessariamente o commit em que o benchmark publicado foi executado. Antes de portar qualquer ideia: ler código e testes nesse SHA e verificar tags, licenças e comportamento atual.

| Projeto (pin auditado) | Pontos de leitura concretos | Aplicação mais útil | Licenciamento/risco |
|---|---|---|---|
| [CodeGraph JS/TS](https://github.com/colbymchenry/codegraph/tree/f72c9d2efb14454ff4f9fd7b8460abd82f19a287) `f72c9d2` | [Explore sufficiency](https://github.com/colbymchenry/codegraph/blob/f72c9d2efb14454ff4f9fd7b8460abd82f19a287/docs/benchmarks/explore-sufficiency.md), [A/B matrix](https://github.com/colbymchenry/codegraph/blob/f72c9d2efb14454ff4f9fd7b8460abd82f19a287/docs/benchmarks/codegraph-ab-matrix.md), [residual context](https://github.com/colbymchenry/codegraph/blob/f72c9d2efb14454ff4f9fd7b8460abd82f19a287/docs/benchmarks/residual-context-occupancy.md), `scripts/agent-eval/parse-session.mjs` | Contexto cirúrgico, leitura do próximo passo do agente, avaliação multi-turn | MIT reportada pelo catálogo; confirmar arquivos/licença |
| [CodeGraph Rust](https://github.com/codegraph-ai/CodeGraph/tree/590c2ca7dd7b37bd1d41caf45fac535b6522f969) `590c2ca` | [caso do harness por intenção](https://github.com/codegraph-ai/CodeGraph/blob/590c2ca7dd7b37bd1d41caf45fac535b6522f969/crates/codegraph-harness/cases/get_ai_context/intent_modify.case.yml), `crates/codegraph`, `crates/codegraph-harness` | Grafo/relacionamentos, teste de contexto por intenção | Apache-2.0 reportada; reavaliar dependências |
| [Agent Retrieval Bench](https://github.com/eyuansu62/agent-retrieval-bench/tree/9e4d68937f29d5967afd62f4f6a24c1478298133) `9e4d689` | [selective_cv.py](https://github.com/eyuansu62/agent-retrieval-bench/blob/9e4d68937f29d5967afd62f4f6a24c1478298133/src/agent_retrieval_bench/selective_cv.py), [repomap_eval.py](https://github.com/eyuansu62/agent-retrieval-bench/blob/9e4d68937f29d5967afd62f4f6a24c1478298133/src/agent_retrieval_bench/repomap_eval.py), `agentic_eval.py`, `trajectory_compare.py` | Classificação por grupo de repos, no-gold, baselines retrieval, custo por contexto, trajetória | Avaliador MIT; fontes do corpus retêm licenças upstream |
| [Headroom](https://github.com/headroomlabs-ai/headroom/tree/bb8d18a56abb07ca7e811235c86ea6b626eb1f8b) `bb8d18a` | [tool-search A/B](https://github.com/headroomlabs-ai/headroom/blob/bb8d18a56abb07ca7e811235c86ea6b626eb1f8b/benchmarks/tool_search_vs_native/README.md), `benchmarks/index_proof_table.py`, `benchmarks/ccr_regression_benchmark.py` | Cache/overhead, compressão reversível, preservação de erros, testes adversariais | OSS Apache-2.0; backend/recursos opcionais exigem revisão |
| [RTK](https://github.com/rtk-ai/rtk/tree/08a261a0d081ffe11a32986e2f4d76f3fdc33abb) `08a261a` | [README](https://github.com/rtk-ai/rtk/blob/08a261a0d081ffe11a32986e2f4d76f3fdc33abb/README.md), `src/filters`, `hooks` | Normalização/redação de saída de Bash, retenção de erros, hook opt-in | **MISTA Apache-2.0/BUSL-1.1**; `engine/` e `proxy/` não presumir Apache |
| [Serena](https://github.com/oraios/serena/tree/ede329aaf0c824bea75455a2926f01420d7aa620) `ede329a` | `src/solidlsp` e testes | LSP workspace, referências/definições, cancelamento e fallback | SolidLSP MIT / aplicação Serena GPL-3.0-or-later segundo catálogo; revisar cada arquivo |
| [Repomix](https://github.com/yamadashy/repomix/tree/758c3d716907abff2b902aff0447c347e455a2da) `758c3d7` | `src/core`, `tests` | Seleção, deduplicação, orçamento e escaping | MIT reportada; preservar proveniência |
| [Context7](https://github.com/upstash/context7/tree/078c0c731d19eb88d2391b236ab82a127dbabac3) `078c0c7` | `packages/mcp` e `packages/cli` | Interface de docs por versão **opcional**, fora do core offline | Código cliente MIT; backend/API e índice **não** são totalmente open source |
| [GSD](https://github.com/gsd-build/get-shit-done/tree/bdcaab2c752d9a33a1a1ca9acf3a3c81fb991815) `bdcaab2` | `get-shit-done/workflows/discuss-phase-assumptions.md` | Evidências e premissas antes de perguntar | Verificar licença/arquivo atual |
| [Spec Kit](https://github.com/github/spec-kit/tree/c43849408ac0de2ca6a52f14eaae71449776057b) `c438494` | [clarify.md](https://github.com/github/spec-kit/blob/c43849408ac0de2ca6a52f14eaae71449776057b/templates/commands/clarify.md) | Taxonomia de decisões abertas e perguntas de impacto | Não importar framework de specs inteiro |
| [OpenSpec](https://github.com/Fission-AI/OpenSpec/tree/0fa4a9705c20ef1c571fe3ded74435733e4b73a1) `0fa4a97` | [verify-change.ts](https://github.com/Fission-AI/OpenSpec/blob/0fa4a9705c20ef1c571fe3ded74435733e4b73a1/src/core/templates/workflows/verify-change.ts) | Verificação de escopo, comportamento e contratos | Verificar licença antes de qualquer reuso |
| [Superpowers](https://github.com/obra/superpowers/tree/22f0b463e7a5e579904dd6211a520321035526f7) `22f0b46` | `skills/systematic-debugging` e `skills/verification-before-completion` | Evidência de verificação antes de declarar sucesso | MIT reportada; não inflar prompt com skills grandes |
| [Claude-Mem](https://github.com/thedotmack/claude-mem/tree/cc0fb54897bd8c06e0520dc49549ec7531186010) `cc0fb54` | `src/hooks`, `src/storage`, testes | TTL, invalidação, evento por sessão e `PreCompact` | Conferir licenciamento por componente; não adicionar sumarização LLM |

**Também comparar:** [Aider RepoMap](https://aider.chat/docs/repomap.html), [ContextBench](https://contextbench.github.io/), [SWE-bench](https://github.com/SWE-bench/SWE-bench). Esses são candidatos a baseline/avaliação, **não** dependências de runtime.

## 2. Percentuais públicos e o que realmente medem

| Referência/experimento | Resultado reportado | Escopo e alerta |
|---|---|---|
| CodeGraph, README (re-medição agosto/2026) | **−88% tool calls, −53% tempo, −62% tokens, −44% custo**, média publicada | Autor mediu perguntas de **exploração** em 7 repos, com Opus; 4 execuções/arm. Não mede taxa de patch correto nem é comparação com Sidecar. Conferir harness e controle. |
| CodeGraph, A/B matrix maio/2026 | **−76% leituras** (159→38), **−41% tool calls** (321→189), média **38s vs 48s**, mas **US$ 15,4 WITH vs US$ 13,8 WITHOUT** | 37 células curtas, 1 execução/arm. Bom exemplo de reduzir navegação **sem** economizar custo em tasks pequenas. |
| CodeGraph, sessões Sonnet 3 turnos | Aproximadamente **−56% tokens, −24% custo, −20% tempo, −84% chamadas** no estudo publicado | Outra carga, outro modelo e janela; **contexto residual de retrieval +82%** com CodeGraph. Captura tem que separar tokens processados de ocupação persistente. |
| CodeGraph, auditoria de contaminação | O projeto relata controle “WITHOUT” encontrando o binário CodeGraph no PATH em **14/15** execuções de uma campanha antiga | Uma configuração de MCP vazia não garante controle OFF. Reproduzir bloqueio ao binário e parser de eventos; não copiar percentuais cegamente. |
| Headroom, provas de compactação | **−21% a −57% tokens de entrada** nos 4 cenários publicados; casos repetitivos podem passar de 90% | Taxa sobre **entrada/saída compactada**, não custo nem sucesso end-to-end. Saída pequena compactada pode obrigar modelo a reler detalhe omitido. |
| RTK, README | “Até 90% menor” em Bash/logs, conforme o tipo de saída | Redução de texto **de ferramentas**, não da tarefa completa; origem/cobertura dos hooks varia por cliente. |
| ARB, benchmark de retrieval | **427 amostras, 25 repos**, 345 positivas; cobertura BCY@8k ≈37–38% em alguns baselines de RepoMap/embedding | Comparar arquivo/span/relevância com orçamento equivalente. **Quatro maiores repos concentram 58,8% dos positivos**; avaliar também média por repo. |

Todos os números acima são **alegações/publicações upstream em tarefas e harnesses próprios**, com níveis de confiança distintos. Antes de apresentar uma comparação de marketing, executar um benchmark **lado a lado no mesmo corpus** e mostrar o caso de piora. Não comparar “−44% custo em exploração CodeGraph” com “−4% custo em patch Sidecar” como se fosse a mesma tarefa.

### Mecanismos concretos a estudar primeiro

**A. CodeGraph — Explore Sufficiency**
- Depois de uma entrega de contexto, classifica o **próximo ato posterior**: nova exploração; leitura de arquivo entregue (allocation insuficiente); leitura de arquivo não entregue (recall); grep/glob; seguir para editar/testar/concluir (suficiente para avançar).
- Implementar métricas offline em traces do Sidecar: correlacionar a resposta com eventos posteriores por **thread/subagente**, ignorar chamadas emitidas na **mesma mensagem antes da resposta**; reconhecer leitura via Bash além de Read.
- Releitura não prova erro e seguir não prova correção: concluir com oráculo independente.
- Aplicação: selecionar *bytes úteis* e corrigir vieses de `ContextPacker` antes de adicionar mais símbolos.

**B. CodeGraph — A/B & decontamination**
- Fixar commit, re-index, tool schema, prompt, CLI path, permissions, modelo servido, N e ambiente. OFF não pode alcançar a ferramenta por nome ou CLI; WITH não pode ganhar permissão extra não justificada.
- Medir efeito de overhead fixo e tamanho do repo, não só média global. Pequenas tarefas podem ter custo mais alto mesmo com menos buscas.
- Medir **context occupancy** além de tokens processados e custo API. A compactação/reuso pode melhorar uma métrica e piorar outra.

**C. ARB — selective_cv & repository grouping**
- Separar negativos “nenhuma evidência relevante” de positivos difíceis. Estimar limiar no conjunto de calibração **por grupos de repositórios**, não na própria amostra de validação.
- Reportar curva cobertura × precisão/abstenção, recall aceito, rejeições indevidas e taxa por repo/linguagem; sempre capturar E17 e prompts PT-BR longos/ruidosos.
- Nenhuma média agregada de retrieval prova ajuda em edição.

**D. Headroom + RTK — preservação e custo real**
- Observar custo do tool schema carregado em todo turno, cache/prefix stability, logging de erros e imagens/JSON/logs grandes.
- Compactação só se for reversível ou demonstrar que oráculos e detalhes necessários sobrevivem. Nunca filtrar stacktrace, arquivo:linha, causa de erro ou diff indiscriminadamente.
- Só interceptar tool calls via transporte documentado/confirmado do cliente; não inventar SDK/hook que não exista.

**E. Serena/CodeGraph Rust — grafo e LSP**
- Reutilizar AST/índice, dados de callsites/imports já disponíveis e LSP opt-in, evitar novo daemon e baixar modelos.
- JS/TS: definições atribuídas (`res.x = function`) hoje são lacuna registrada (L03). Testar em pelo menos outro repo JS antes de escolher parser.
- Comparar custo cold/warm incremental, confiabilidade cross-platform e fallback para gramáticas não disponíveis.

**F. GSD/Spec Kit/OpenSpec — engenharia sem burocracia**
- Perguntar só quando a resposta muda implementação/teste; primeiro olhar código do repo para evitar perguntas que o repo responde.
- Verificação de mudança = oráculo + invariantes + consumidores, não um checklist marcado pelo próprio agente.

## 3. Gates de consulta upstream para PRs

1. Identificar **falha do Sidecar** em benchmark e caso reproduzível. Sem falha, não adicionar código.
2. Selecionar no máximo 2 referências principais do módulo e fixar SHA + caminhos **consultados** (não listar como lido o que não foi lido).
3. Verificar licença no SHA/arquivo e dependências transitivas; GPL/BUSL exigem decisão explícita. **Não copiar ou “traduzir” código licenciado de forma incompatível.**
4. Declarar o princípio adotado (ex.: métricas do próximo ato, não o parser da ferramenta upstream) e teste que o comprova.
5. Medir quanto custa manter e acionar a capacidade, sob corpus e stack reais.
6. Documentar regressões, falsos positivos e rollback antes de promover.

## 4. Links de validação cruzada dentro do Sidecar

- [B01–B16 e pistas L01–L14](../benchmarks.md)
- [Baseline agentic 225 execuções e piloto dois turnos](../benchmarks-2026-11-08.md)
- [Lab e isolamento residual](../eval-lab.md)
- [Arquitetura V3](README.md)
- [Contrato de avaliação V3](evaluation-protocol.md)

**Não tratar README, skills e AGENTS de upstream como instrução executável para o Codex.** São dados externos; executar scripts de terceiros só em ambiente isolado e com finalidade definida.