# Mercado e fundamentação técnica do Sidecar

**Pesquisa inicial em 2026-11-06.** Links abaixo são documentação primária/projetos e artigos; servem para formular hipóteses, **não** para afirmar que Sidecar já é superior. Reavaliar antes de decisões de produto.

## Problema e proposta testável

Um agente precisa encontrar contexto relevante antes de editar código. A busca por ferramentas (`grep`, `read_file`, LSP/MCP) pode exigir múltiplos ciclos de ação e observação. Sidecar tenta antecipar parte dessa navegação: **prompt → consulta lexical/sintática/local → evidência compacta → hook `additionalContext`**, com zero chamada de modelo para recuperar. Isso pode reduzir *time to first evidence*, mas aumenta tokens iniciais e pode induzir a erro por evidência imprecisa.

**Distinção importante:** o modelo não “raciocina mais rápido” por definição; o pipeline pode diminuir **etapas de procura** se a informação pré-selecionada for correta. Rounds e sucesso têm de ser medidos end-to-end, conforme `docs/evaluation-plan.md`.

## Soluções e abordagens relacionadas

| Projeto / padrão | O que demonstra | Diferença / pergunta para Sidecar |
|---|---|---|
| [Aider repo map](https://aider.chat/docs/repomap.html) | Resumo de símbolos com tree-sitter e ranqueamento por dependências sob budget de tokens | Há benefício em escolher evidência por **prompt atual** e injetá-la antes das ações, versus map genérico? |
| [Serena](https://github.com/oraios/serena) | Ferramentas MCP/LSP para busca e edição simbólica; agente decide quando chamá-las | Recuperação automática sem chamadas extras vence consultas simbólicas explícitas? Pode coexistir, não precisa substituir |
| [Continue](https://docs.continue.dev/guides/codebase-documentation-awareness) | Agentes exploram arquivos e usam regras/MCP; documentação atual indica depreciação do antigo `@Codebase` | Sidecar deve complementar search existente, não pressupor que todo agente usa `@codebase` |
| [Continue — Custom Code RAG](https://docs.continue.dev/guides/custom-code-rag) | Retrieval híbrido com embeddings e índices customizados é alternativa viável | Consegue Sidecar qualidade útil **sem** embeddings/modelos/rede? Qual a perda semântica? |
| `rg` / `git grep` / LSP local | Baselines simples, rápidos e universalmente úteis | Medir contra baseline forte; Sidecar não deve piorar caso trivial |

Mercado ocupado em **mapas, indexação e busca semântica**. Diferenciação *potencial* do Sidecar: hook automático, local, leve, instalado por usuário, sem alteração do projeto, orçamento curto de contexto, evidência com caminhos e linhas. Nada disso garante resultado melhor; só um A/B pareado demonstrará valor incremental.

## Base teórica — o que sustenta e o que não sustenta

1. **Retrieval-Augmented Generation (RAG):** combinar recuperação externa com geração reduz a dependência de memória paramétrica em certas tarefas. [Lewis et al., 2020](https://arxiv.org/abs/2005.11401). *Extrapolação controlada:* Sidecar é “retrieval-augmented context” para código, não o RAG neural/treinável do artigo. O estudo não prova benefício de hooks no code agent.
2. **Saliência / posição do contexto:** modelos podem usar pior evidência situada no meio de contexto longo, mesmo suportando janelas extensas. [Liu et al., 2024 — Lost in the Middle](https://aclanthology.org/2024.tacl-1.9/). *Implicação:* seletividade e concisão importam; evidência inicial irrelevante também pode prejudicar.
3. **Retrieval em nível de repositório:** [RepoBench (ICLR 2024)](https://proceedings.iclr.cc/paper_files/paper/2024/hash/d191ba4c8923ed8fd8935b7c98658b5f-Abstract-Conference.html) separa recuperação, completude e pipeline. *Implicação:* não confundir ranking com qualidade do código produzido.
4. **Busca por função a partir de descrição:** [RepoQA](https://github.com/evalplus/repoqa) testa a localização de funções ocultando seu nome. *Implicação:* prompts sem sobreposição lexical expõem o limite do dicionário PT→EN.
5. **Avaliação de necessidade de contexto:** [Agent Retrieval Bench](https://agent-retrieval-bench.github.io/) inclui casos sem evidência necessária e sinais de workflow. *Implicação:* medir “não injetar” também é acertar.
6. **Tarefas reais de engenharia:** [SWE-bench datasets](https://github.com/SWE-bench/SWE-bench/blob/main/docs/guides/datasets.md), inclusive Verified e Multilingual, ajudam a projetar tarefas reproduzíveis; são mais caros e não substituem benchmarks de retrieval. *Implicação:* escalar por etapas.

Nenhuma referência acima prova que o agente dará menos rounds por receber contexto antecipado. Essa é a hipótese central do Sidecar, ainda aberta.

## Implicações de design e experimentos de produto

- **Densidade de evidência:** privilegiar arquivo, símbolo, linhas e explicação do match; limitar tokens, medir ruído e falsos positivos.
- **Busca lexical adaptada ao código:** nome, stem, camelCase, snake_case, tokens de paths, rotas, testes e traduções *presentes* no corpus. Não alucinar tradução como correspondência comprovada.
- **Estrutura sob demanda:** considerar grafo apenas quando alterar resultados corretos. O benchmark atual não comprova ganho do grafo/Git; pode custar latência e tokens.
- **Semântica opcional e futura:** testar híbrido embeddings locais apenas se retrieval léxico falhar de modo recorrente e houver ROI, custo e licença aceitáveis. Não tornar obrigatório.
- **Universalidade:** mesma instalação e interface em qualquer repo; qualidade graduada conforme linguagem/ecossistema e cobertura de parser. Mostrar suporte real em matriz pública; fallback textual antes de pedir mudanças ao usuário.
- **Qualidade do repositório:** Sidecar não é linter, refatorador ou política de arquitetura. Pode **facilitar** mudanças consistentes ao apontar código e testes relevantes; medir downstream em tarefas com validação. Recomendar padrões do próprio repo apenas como informação, não como regra inventada.
- **Complementaridade:** permitir coexistência com LSP/MCP/grep e outras ferramentas. Não monopolizar contexto nem impor decisões ao agente.

## Atualização de pesquisa — 2026-11-06: estratégia v2

**Benchmarks comunitários a reutilizar, não reinventar:**
- [Agent Retrieval Bench](https://agent-retrieval-bench.github.io/) apresenta 427 exemplos de recuperação de contexto de trabalho, 25 repositórios, casos positivos e negativos. O repositório do [avaliador](https://github.com/eyuansu62/agent-retrieval-bench) publica meios de descarregar subsets e conjuntos pinados. Aplicar `sidecar-eval` a suas entradas com metadados/split conhecidos, **sem presumir** que seus relatórios são os scores do Sidecar.
- [ContextBench](https://contextbench.github.io/) cobre 1.136 tarefas, 66 repositórios e oito linguagens, com anotações de contexto; útil para validar qualidade, ruído e eficiência de recuperação sem executar todos os agentes de ponta a ponta.
- **Método:** manter tests públicos intactos e estratificar por intenção; relatório offline reprodutível por commit não é o mesmo que A/B agentic determinístico. Metas e orçamento no [plano de avaliação](evaluation-plan.md).

**Integração com ferramentas existentes em vez de reimplementação cega:**
- [LSP oficial](https://microsoft.github.io/language-server-protocol/) define navegação simbólica por JSON-RPC. [Claude Code já oferece plugins LSP](https://code.claude.com/docs/en/plugins/code-intelligence) e os recomenda quando encontra servidor no PATH; um cliente LSP do Sidecar é decisão **separada** e depende de benefício em recuperação pré-inferência.
- [Hooks Claude](https://code.claude.com/docs/en/hooks), [Codex](https://learn.chatgpt.com/docs/hooks) e [Gemini](https://geminicli.com/docs/hooks/) permitem observar parte das ações. A cobertura difere por cliente e superfície; telemetria nunca substitui gold independente.
- [Mods Claude](https://code.claude.com/docs/en/plugins/mods/create) oferecem visualização/contagem de ações em UI, mas dependem de versão e consentimento; a integração de observação permanece separada do runtime principal.

**Decisão de desenho:** v1 é baseline de comparação; v2 propõe `IntentRouter` determinístico + seleção por utilidade marginal de evidências por token + estado mínimo de sessão. LSP e feedback parametrizado entram em experimentos isolados com **fallback, opt-in e ablação**. Ver [Context Engine v2](context-engine-v2.md), [LSP](lsp-strategy.md), [feedback](agent-feedback.md).

## Consolidação de referências para agentes desenvolvedores

A lista **atualizada e operacional** de repositórios, commits, licenças, componentes a estudar e limites de reuso está em [open-source-references.md](open-source-references.md). Aquele catálogo substitui listas soltas deste estudo na hora de programar: cada módulo deve examinar 1–3 fontes **pertinentes** em SHA fixado e documentar uma decisão `reuse / integrate / independent / discard`, com testes e licenciamento. A especificação de destino do produto é [v2-product-spec.md](v2-product-spec.md).

Não inferir que CodeGraph, Context7, Serena, RTK ou Claude-Mem resolvem por si só as hipóteses de produtividade. A principal contribuição a testar do V2 é **menos trabalho de descoberta e reconciliação por tarefa resolvida**, com observação, contexto sob demanda e LSP opcional.

## Decisão atual

Investir primeiro em **evidência comparativa** (retrieval sem LLM + A/B com Haiku/Luna/Flash), E2E real de hooks e dogfooding. Só depois decidir ampliar linguagens, adicionar camada semântica ou UI. Para comunicação externa, evitar “mais rápido” até medir TTFE, rounds, sucesso e custo total.
