# Catálogo de referências open source para implementação do Sidecar V2

**Revisado em 2026-11-06.** Este documento é uma **lista de trabalho para agentes desenvolvedores do Sidecar**, não uma lista de dependências instaladas ou promessas de funcionalidade. Os SHAs abaixo são fotografias verificadas de branches no discovery; a licença deve ser **revalidada no arquivo e subdiretório exatos antes de qualquer reaproveitamento**.

## Regra operacional para agentes (obrigatória quando houver trabalho correspondente)

1. Leia `AGENTS.md`, `docs/v2-product-spec.md` e a seção correspondente desta matriz **somente para a tarefa atual**; não carregue todos os repositórios ou manuais no contexto de cada prompt.
2. Use GitHub (API/CLI, em read-only) para consultar README, arquitetura, testes, interfaces e os arquivos **relacionados diretamente** ao módulo; fixe o SHA abaixo ou declare explicitamente uma nova revisão.
3. Produza uma nota de comparação curta: `problema / soluções examinadas / trade-offs / licença por arquivo / decisão (reusar, integrar, reimplementar, descartar) / tests / benchmark`. A nota entra no PR/ADR, não no prompt do usuário final.
4. Preferir **interoperabilidade ou dependência de componente maduro** a recriar uma infraestrutura grande. Para incluir código de terceiros, exigir revisão de licença/compatibilidade com a licença do Sidecar (ainda não definida), avisos de copyright/NOTICE e transitive deps; não copiar arquivo/algoritmo proprietário. Quando for clean-room, documentar comportamento público/spec/testes sem transplantar implementação.
5. Não assumir que MIT/Apache do README cobre todos os diretórios ou serviço remoto. Software público sem licença válida para o trecho não autoriza redistribuição. Licenças copyleft (GPL) exigem decisão explícita e não podem ser misturadas indiscriminadamente.
6. Não executem scripts de instalação, builds, testes ou código remoto de repositórios externos em máquinas do usuário: estudar em clone isolado e apenas com consentimento/ambiente seguro.
7. Após implementar, vincular referência upstream exata (URL+SHA), benchmark de baseline, resultados, riscos e rollback; atribuir corretamente. Não afirmar performance do Sidecar a partir de score de terceiros.

## Matriz: referência → função do V2

| Projeto (fonte de consulta) | SHA de referência | Licença declarada (verificar trecho) | O que estudar / aplicar | O que NÃO fazer |
|---|---|---|---|---|
| [codegraph-ai/CodeGraph](https://github.com/codegraph-ai/CodeGraph) | `590c2ca7` | Apache-2.0 (metadados GitHub) | Grafo de dependências, descoberta de símbolos, relações e consultas; comparação de arquitetura/cobertura; baseline concorrente | Recriar um grande CodeGraph por imitação |
| [colbymchenry/codegraph](https://github.com/colbymchenry/codegraph) | `f72c9d2e` | MIT ([LICENSE](https://github.com/colbymchenry/codegraph/blob/main/LICENSE)) | Hook de contexto antecipado, instrumentação e comparação ON/OFF; benchmark concorrente com configuração equivalente | Alegar que auto-injeção é exclusiva do Sidecar |
| [oraios/serena](https://github.com/oraios/serena) | `ede329aa` | **Mista**: SolidLSP MIT, aplicação Serena GPL-3.0-or-later ([LICENSE](https://github.com/oraios/serena/blob/main/LICENSE)) | Conexão/capacidades de servidores, erros e gestão de workspaces; preferir estudar/reusar apenas SolidLSP MIT após revisão | Copiar `src/serena` GPL como se MIT |
| [yamadashy/repomix](https://github.com/yamadashy/repomix) | `41f052b8` | MIT | Mapas de repo, árvore de arquivos, compaction baseada em AST, proteção de tokens, filtros | Injetar repo inteiro em toda tarefa |
| [upstash/context7](https://github.com/upstash/context7) | `078c0c73` | MIT para este repositório | Descoberta de bibliotecas/versões, conectores de documentação e UX sob demanda | Supor que backend privado, parsing/crawler e índice do Context7 estão neste repo; forçar rede |
| [rtk-ai/rtk](https://github.com/rtk-ai/rtk) | `1cf7bc70` | Apache-2.0 | Seleção/compressão de saída de ferramentas, filtros de erros, preservação de escape hatch | Comprimir tudo: economia local de tokens pode piorar tarefas completas |
| [thedotmack/claude-mem](https://github.com/thedotmack/claude-mem) | `31ebffbb` | Apache-2.0 ([README](https://github.com/thedotmack/claude-mem/blob/main/README.md)); verificar subpastas e histórico | Persistência de observações, deduplicação, TTL, proveniência, recuperação/compactação | Registrar toda a conversa ou tornar LLM obrigatório no runtime |
| [gastownhall/beads](https://github.com/gastownhall/beads) | `8c3eb30f` | MIT | Estado de tarefas, relações, checkpoint entre sessões | Substituir gerenciador de tarefas do agente |
| [obra/superpowers](https://github.com/obra/superpowers) | `22f0b463` | MIT | Skills e processo de engenharia, disciplina de testes e validação | Injetar manual de 10k tokens por mensagem |
| [eyuansu62/agent-retrieval-bench](https://github.com/eyuansu62/agent-retrieval-bench) | `9e4d6893` | MIT para avaliador/metadados; corpus sob licenças upstream | Reusar parser de datasets, casos pinados e métricas; no-gold, BCY por budget | Publicar scores de terceiros como Sidecar ou redistribuir corpus sem licença |
| [aider repo map](https://aider.chat/docs/repomap.html) | documento público; pin ao escolher versão no [repo aider](https://github.com/Aider-AI/aider) | Verificar arquivo antes de incorporação | Seleção de símbolos importantes por tokens e relações | Assumir que um mapa global vence seleção por intenção |

**Opcional, sob pesquisa separada:** [microsoft/spec-kit](https://github.com/github/spec-kit) (especificações/processo), [Headroom](https://github.com/headroomlabs-ai/headroom) (compaction), [ContextBench](https://contextbench.github.io/) (avaliação), [LSP protocol](https://microsoft.github.io/language-server-protocol/) (protocolo; não código pronto). Verificar URL, licença e disponibilidade no momento de uso. Não transformar catálogos em dependências obrigatórias.

## Roteiro de investigação por módulo

| Módulo Sidecar | Primeiro consultar | Perguntas concretas |
|---|---|---|
| `IntentRouter` / score | ARB, CodeGraph (ambos), Aider | Como identificar relevância de fluxo (`trace2code`, `edit2ripple`)? O que é reproduzível sem LLM? |
| `ContextPacker` | Repomix, Aider, ARB, RTK | Como maximizar evidência útil / token sem eliminar tracebacks, erros e linhas? |
| `ContractResolver` | Serena (SolidLSP), CodeGraph, AST/test links | Que vínculos são fatos verificáveis? Quando declarar `potential_conflict`? |
| `SessionObserver` / memória | Claude-Mem, Beads, docs de hooks oficiais | Como diferenciar ação observada de validação? Como manter TTL, invalidação, no-prompt logging? |
| `LSP` | SolidLSP (MIT), LSP spec, plugins oficiais Claude | Reusar protocolo/biblioteca ou escrever adapter minimal? Qual o custo de startup? |
| `RepoEnvironment` | Repomix, CodeGraph, Context7 (manifest discovery) | Como reconhecer stack/versão sem executar código do projeto ou fazer download? |
| `Harness` | ARB, ContextBench, RTK claims, CodeGraph claims | Quais métricas distinguem retrieval, tool calls e sucesso? Dataset e licença pinados? |
| `Skills/Agent rules` | Superpowers, Beads e AGENTS.md existente | O que adicionar só como diagnóstico/opt-in sem sobrecarregar prompts? |

## Licenciamento e prova de origem

O Sidecar **ainda não tem licença escolhida**; isso bloqueia uma distribuição responsável de código incorporado de terceiros. Um agente pode estudar ou implementar **ideias públicas** sem copiar a expressão protegida, mas o uso de trechos concretos requer:
- `upstream url`, commit SHA, caminho, arquivo LICENSE/SPDX correspondente, notices e inventário de dependências;
- decisão de compatibilidade da licença do Sidecar com a referência (aprovação do mantenedor quando houver copyleft/dual licensing);
- testes, scanner de licença e artefatos `THIRD_PARTY_NOTICES` se aplicável;
- distinção explícita `reference_only` vs `dependency` vs `vendored` vs `clean_room`.

**Não chamar “clean room” de copiar e renomear código**. Uma implementação independente deve usar especificação observável, testes e separação deliberada quando necessária.

## Política de atualização deste catálogo

Os SHAs foram consultados no discovery de 2026-11-06. Não atualizar todos automaticamente. Antes de um PR de módulo, verificar se surgiram mudanças materiais no upstream e **fixar a versão de referência no PR**, sem trocar silenciosamente a versão testada. Se a funcionalidade já existir no Sidecar, modificar apenas após comparar custo, riscos e comportamento.
