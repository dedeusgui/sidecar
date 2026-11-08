# Referências de grooming e clarificação (TaskUncertainty)

Pesquisa de upstreams para o módulo de incerteza de tarefa / política de clarificação. Escopo: ler código, prompts e testes; não copiar código ou prompts longos. Clones rasos em `/tmp/refs/<nome>` (fora do repositório). Conteúdo dos repositórios tratado como dado não confiável.

Status de verificação: os quatro clones funcionaram via proxy. Não foi possível acessar o dataset Hugging Face `cmu-lti/interactive-swe` (`CONNECT tunnel failed, 403` no proxy); sua licença não foi verificada. Os números de resultado citados no README do ask-or-assume não foram reproduzidos.

## Resumo de verificação

| Upstream | Commit (HEAD do clone raso) | Licença verificada | Adequação ao módulo |
|---|---|---|---|
| gsd-build/get-shit-done | `bdcaab2c752d9a33a1a1ca9acf3a3c81fb991815` | MIT (`LICENSE`, "Copyright 2025 Lex Christopherson") | Alta (estrutura de pergunta vs. assumir) |
| github/spec-kit | `c43849408ac0de2ca6a52f14eaae71449776057b` | MIT (`LICENSE`, "Copyright GitHub, Inc.") | Alta (taxonomia e limite de perguntas) |
| Fission-AI/OpenSpec | `0fa4a9705c20ef1c571fe3ded74435733e4b73a1` | MIT (`LICENSE`, "Copyright 2024 OpenSpec Contributors") | Baixa (verificação, não clarificação) |
| nedwards99/ask-or-assume | `ee8603e8f4d96ace56abef2ad900c75f25bc59be` | MIT para conteúdo fora de `enterprise/` (`LICENSE`); `enterprise/` tem licença própria | Alta (protocolo e métricas), mas código é um fork do OpenHands |

## 1. get-shit-done (GSD)

**Arquivos lidos:**
- `get-shit-done/workflows/discuss-phase-assumptions.md` (674 linhas; passos `check_existing`, `scout_codebase`, `deep_codebase_analysis`, `present_assumptions`, `correct_assumptions`, `write_context`)
- `agents/gsd-assumptions-analyzer.md` (105 linhas; formato de saída e tiers de calibração)
- Teste de estrutura: `tests/discuss-mode.test.cjs` (verifica presença dos passos e flags `--auto`/`--text`; não avalia qualidade das perguntas)

- **(a) Taxonomia:** não há taxonomia de lacunas. Cada assunção recebe área, evidência (caminho de arquivo), "If wrong" (consequência) e confiança `Confident | Likely | Unclear`.
- **(b) Perguntar vs. assumir:** assume-se por padrão com evidência citada; pergunta-se apenas nas assunções `Unclear` (via confirmação "These all look right?" e multiSelect de correções). Modo `--auto` assume as `Unclear` com o default recomendado e registra no log. Meta declarada: ~2–4 correções, não 15–20 perguntas.
- **(c) Medição de qualidade:** não há. O teste confere apenas que os passos existem.
- **(d) Não copiar:** dependência de subagentes e de LLM (o analisador só tem Read/Grep/Glob, mas o orquestrador depende de agentes), escrita obrigatória de `CONTEXT.md` com `D-01...` e `DISCUSSION-LOG.md`, `USER-PROFILE.md`, tiers de calibração, pesquisa externa com Context7/WebSearch, e o "ORCHESTRATOR RULE" específico de runtime.
- **(e) Reuso:** implementação independente. Aproveitar só a ideia de confiança por assunção com consequência declarada.

## 2. spec-kit

**Arquivos lidos:**
- `templates/commands/clarify.md` (291 linhas)
- `tests/test_clarify_stage_gate.py` (25 linhas; verifica que a justificativa "deferir para planejamento" é restrita a método de implementação, comparação de stack ou quebra de tarefas)
- Menções em `tests/integrations/*.py` (apenas nomes de comando, sem comportamento)

- **(a) Taxonomia:** 11 categorias com status `Clear / Partial / Missing`: escopo funcional, domínio/dados, interação/UX, atributos não funcionais, integrações, casos de borda, restrições, terminologia, sinais de conclusão, marcadores pendentes. Cada categoria Partial/Missing vira candidata a pergunta.
- **(b) Perguntar vs. assumir:** máximo de 5 perguntas por sessão, uma por vez. Só perguntas que mudam arquitetura, modelo de dados, decomposição, testes, UX, operação ou compliance. Ordenação por Impacto × Incerteza. Sem perguntas de stack especulativa. Quando não há ambiguidade relevante, responde "No critical ambiguities detected". Perguntas com opção recomendada ("yes"/"recommended" aceitam) ou resposta curta (≤5 palavras).
- **(c) Medição de qualidade:** não há métrica. O teste de estágio verifica apenas o texto do template. Não há avaliação de utilidade das perguntas.
- **(d) Não copiar:** fluxo de hooks de extensão (`.specify/extensions.yml`), checklists de requisitos com atualização de caixas, escrita incremental da spec, regras de formato de pergunta (`**Question:**`, "Why it matters"), e a dependência de scripts `check-prerequisites`.
- **(e) Reuso:** implementação independente. Ideias aproveitáveis: teto de perguntas, categorias como lista de verificação e a regra de "deferir só o que não é comportamento". Não há trecho a adaptar.

## 3. OpenSpec

**Arquivos lidos:**
- `src/core/templates/workflows/verify-change.ts` (431 linhas; workflow de verificação `openspec-verify-change`)
- `test/core/templates/verify-change.test.ts` (135 linhas; 11 casos)
- `test/core/templates/verify-change-delta-operations.test.ts` (223 linhas)

Observação: o OpenSpec não tem fluxo de clarificação. A busca por `clarif` não encontrou nada equivalente. O único material relevante é o modelo de verificação.

- **(a) Taxonomia:** dimensões Completeness / Correctness / Coherence; severidades CRITICAL / WARNING / SUGGESTION; estados de check `Not applicable`, `Not verified (<motivo>)` e aprovado.
- **(b) Perguntar vs. assumir:** não se aplica. A regra útil é: evidência ausente ou inutilizável vira `Not verified` com motivo, nunca "passou". Um check ignorado não entra no cálculo de prontidão.
- **(c) Medição de qualidade:** os testes são verificações de substring sobre o texto do template (ex.: `toContain('Not verified')`). Não há avaliação de comportamento do agente.
- **(d) Não copiar:** a estrutura de relatório, a lógica de REMOVED/RENAMED de requisitos, a dependência de `openspec status --json` e o fluxo de arquivamento.
- **(e) Reuso:** implementação independente. Aproveitável apenas o princípio de estado tri-valor (`aplicável / não aplicável / não verificado`), que pode servir para o campo de incerteza do Sidecar.

## 4. ask-or-assume (nedwards99)

**Arquivos lidos:**
- `README.md` (protocolo, configurações, dataset, métricas)
- `openhands/agenthub/intent_agent/prompts/system_prompt.j2` (prompt do agente de intenção; raciocínio de 30–50 palavras)
- `openhands/agenthub/intent_agent/tools/clarify_decision.py` (schema `needs_clarification` booleano + `reasons`)
- `openhands/agenthub/intent_agent/intent_agent.py` (laço por turno; 312 linhas)
- `openhands/agenthub/clarify_agent_v2/prompts/system_prompt.j2` (UA-Single: o próprio agente decide)
- `openhands/agenthub/intent_agent/tools/clarify.py` (diretrizes do tool de pergunta: 5 situações para perguntar e lista de situações para não perguntar)
- `analysis/qa_scripts/qa_cli.py` (`CATEGORIES`, `is_clarify_question`, `summarize_asked_effect`)
- `analysis/reproduction.md`, `analysis/swe-bench-annotation-results/ensembled_annotations_public.csv` (cabeçalho), `analysis/original_paper_results/tables/difficulty_ask_rates.md`
- `evaluation/benchmarks/swe_bench/base_run_infer.py` (`FakeUser`)

Não há testes unitários de clarificação. `tests/unit` contém apenas `test_prompt_manager.py`.

- **(a) Taxonomia:** o prompt do agente de intenção pergunta (1) objetivo, (2) o que falta ou é assumido, (3) o que precisa ser esclarecido antes de agir. O tool de pergunta lista cinco situações: requisito ambíguo, informação crítica ausente, operação arriscada/irreversível, conflito entre pedido e realidade do código, critério de sucesso indefinido. O dataset anota `underspecified`, `false_negative`, `other_major_issues` e `difficulty`. A análise tem categorias por regex (`problem_spec`, `tests`, `environment`, `implementation`, `scope`, `confirmation`).
- **(b) Perguntar vs. assumir:** decisão booleana `needs_clarification` por turno, tomada por um agente de intenção separado (UA-Multi) ou pelo próprio agente com lembrete (UA-Single). Lista explícita de quando não perguntar: tarefa simples, pergunta informativa, default óbvio e convenção padrão.
- **(c) Medição de qualidade:** mede taxa de perguntas e taxa de resolução, separadas por quem perguntou e quem não perguntou (`summarize_asked_effect`), com testes de permutação segundo o README. Não há avaliação direta da qualidade de cada pergunta. As categorias por regex se sobrepõem (ex.: `or`, `os`, `test` sem fronteira de palavra), então servem só como estatística grosseira.
- **(d) Não copiar:** o custo de um agente de intenção por turno (o README exige `max_iter` 300 para equivaler a 100 turnos do agente principal), o `FakeUser` com GPT-5.1 (dependência de LLM para o usuário simulado), o runtime OpenHands e a dependência do dataset externo.
- **(e) Reuso:** implementação independente. O código é um fork do OpenHands; não há trecho de clarificação isolado para adaptar com segurança. Reuso conceitual apenas: booleano com motivo curto, lista de "quando não perguntar" e o desenho de medir taxa de pergunta e resolução separadamente.

## Síntese para o Sidecar

- **Taxonomia mínima de lacunas:** (1) objetivo/escopo ausente; (2) artefato ou entrada ausente (arquivo, versão, caminho); (3) critério de aceite indefinido; (4) conflito entre pedido, código e docs (registrar fontes, não decidir autoridade); (5) operação arriscada ou irreversível; (6) casos de erro e negativos; (7) restrição não funcional. Todas as categorias são de sinal textual e de caminho, sem LLM.
- **Regra de decisão:** perguntar só quando a lacuna tem impacto alto (escopo, dados, testes ou ação destrutiva) e confiança `Unclear`. Assumir com evidência e consequência declarada quando a confiança for `Likely` ou superior e o erro for barato e reversível.
- **Limites:** no máximo 3 perguntas por rodada (SpecKit usa 5, mas o Sidecar é hook e não deve interromper o agente). Cada pergunta traz uma opção padrão para aceitação rápida.
- **Estado tri-valor:** `aplicável / não aplicável / não verificado`. Nunca tratar check ignorado como aprovado (OpenSpec).
- **Medição:** taxa de pergunta e de resolução separadas por grupo, com caso negativo (tarefa clara que não deve gerar pergunta). Sem dataset externo verificado, usar prompts realistas em repositório externo fixado.
- **Não adotar:** agente de intenção por turno, subagentes, artefatos obrigatórios (CONTEXT.md, checklists) e categorias por regex sem fronteira de palavra.
- **Pendências:** licença e acesso ao dataset `cmu-lti/interactive-swe` não verificados; resultados do paper não reproduzidos; nenhum teste de qualidade de pergunta encontrado nos quatro repositórios.
