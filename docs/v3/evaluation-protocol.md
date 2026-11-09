# Sidecar V3 — contrato de avaliação e promoção

**Estado:** protocolo prospectivo, **não** resultado. Baseline rastreável: `main@31279c3c`, [índice B01–B16](../benchmarks.md), [relatório B12/B15](../benchmarks-2026-11-08.md). **Obrigatório** antes de atribuir qualquer porcentagem de melhoria da V3. Não substituir [evaluation-plan.md](../evaluation-plan.md): este documento explicita gates para V3.

## 0. Regras anti-gaming e integridade

- **Oráculo separado do agente.** Código e testes ocultos não podem ser lidos nem por tools nem pelo código executado durante a tarefa. O próprio corpus público contém patches de referência: bloqueio de Read/Grep **não é sandbox**. Executar agente e processos filhos dentro de contêiner/usuário isolado com oracle fora do namespace/FS visível; o runner aplica o oracle depois da saída, em ambiente limpo. Validar isolamento com uma sonda adversarial que tente ler o gabarito via `go test`, `python`, `Bash` e caminhos construídos em runtime. Sem isso, marcar **inconclusivo/contaminável**, não gastar holdout.
- **Não selecionar resultado.** Registrar timeout, indisponibilidade, recusa, custo desconhecido, erro de ferramenta, uso indevido de recurso, run sem modelo servido ou configuração inconsistente. Separar inexequível por infraestrutura de falha do agente; não apagar retrospectivamente runs ruins.
- **Benchmark pré-registrado.** Fixar commits, seleção de tarefas, dificuldade proposta, oráculo, permissões, alocação de orçamento, ordem de braços, repetições, teste primário, intervalo de confiança, critérios de exclusão **antes** de rodar. Mudança após ver resultado = versão nova do corpus e reteste; antigos resultados preservados.
- **Sem tuning no holdout.** Não executar `--final` até freeze de engine+parâmetros+clientes, isolamento forte e orçamento. O atual split `holdout=9` tem arquivos em repo, portanto “selado” é regra de procedimento, não segredo criptográfico: preferir **novo holdout realmente externo** (avaliador independente, gold fora do alcance do workspace).
- **Definir capacidade do controle.** OFF sem Sidecar deve bloquear hooks, PATH, MCP e qualquer binário do Sidecar; controle CodeGraph OFF sem CodeGraph também. Verificação automática em transcript, shell e env. O benchmark CodeGraph documenta contaminação via binário acessível no PATH.
- **Não mentir com cache.** Mesmo init e versões, capturar input_uncached/cache_read/cache_create/output quando servidor reportar; tokens processados ≠ tokens faturados ≠ janela residente. Ausência de um contador = `unknown`, não zero.
- **Não gastar tokens pagos no CI**. CI executa lint, unit, oracle-validation, determinismo, simulados etiquetados como `infra`. Experimento agentic pago exige autorização explícita, orçamento e teto; esgotado orçamento -> continuar trabalho offline e entregar pendências.

## 1. Unidade de análise e métricas

**Unidade principal** = tarefa identificada por ID+SHA de commit base+oráculo versionado, agrupada por repositório e classe. Repetições da mesma tarefa não são amostras independentes. **Resultado primário de edição**: sucesso de comportamento (testes ocultos independentes + invariantes de contrato verificáveis); escopo e testes extras reportados **separadamente** e combinados num gate de qualidade transparente.

| Família | Métricas diagnósticas | Interpretação correta |
|---|---|---|
| Correção | oracle pass/fail, invariantes, escopo não-teste, regressões, revisões independentes | Não compensa perda de qualidade com tokens |
| Economia | custo total USD, custo por sucesso, tokens por fonte, cold/warm, indexação amortizada | Dividir custo total de **todas** as tentativas por total de sucessos; mostrar taxa de sucesso e IC |
| Agente | tool calls por tipo, file reads, grep, Bash, subagentes, primeira edição, rework | Menos calls sem patch correto **não** é vitória |
| Retrieval | Hit@1/5, Recall@K, MRR, BCY@budget, spans/lines, falsos positivos e falsos negativos | Não transpor para taxa de patches |
| Seletividade | precision/recall condicional, rejeição de positivo, cobertura × risco, calibração por repo | `no-gold` com verdade independente, não baseado no score da engine |
| Suficiência | próximo ato em thread após evidência, `read_returned`, `read_not_returned`, `search_again`, `moved_on`, erro | Proxy de alocação/recall, **não** prova de qualidade |
| Sessão | turnos, releitura após compactação, perda de evidência, latência de retomada, custo por decisão | A segunda volta do usuário é uma intervenção; registrar quando/por quê |
| Performance | p50/p95/p99, init/index/reindex e tempo de hook+cliente, uso RAM/CPU, contexto residente | Tempos de processos competindo por CPU não servem para claim |

**Métrica econômica:** `cost_per_success = SUM(cost_usd over all comparable attempts) / COUNT(correct successful attempts)`. Custo desconhecido em qualquer execução comparável -> agregado de custo **desconhecido** ou reportar limites conservadores claramente, nunca excluir em silêncio. Se uma das duas voltas tem `cost_usd=None`, o total da sessão **é desconhecido**, não a soma parcial. Adicionar regressão de teste em `bench/lab/lab.py::merge_turns`.

**Comparações primárias** pré-definidas em pares (sem cherry-picking):
- Qualidade/custo de edição: `OFF vs V2`, `OFF vs V3`, `V2 vs V3`; `V2+brief` em ablação.
- Competição de navegação: `OFF vs V3 vs CodeGraph`, com Aider RepoMap e lexical/BM25 como baselines offline.
- Feature attribution somente nas features candidatas que chegaram a um gate de regressão; não comparar 2^N braços em tasks pagas.

IC: bootstrap pareado **agrupado por repo/tarefa**, registrar variação por rep, classe, repo e modelo; validar escolha de unidade contra pseudorrepetição. Declarar efeitos e limites; controlar multiplicidade (um outcome primário por hipótese pré-registrado, secundários exploratórios). Reportar **regressões por caso**, não esconder em médias.

## 2. Famílias de eval independentes

### E0 — Integridade, harness, oracle e overhead (sem modelo)

**Motivação:** B12 ocultos 225/225, e escopo “só testes extras” gerou 33/48 reprovações estritas; B15 parcial; oráculo sem sandbox SO.

Tarefas:
- Revisar `bench/lab/lab.py`, `test_lab.py`, `bench/registry.json`, `consolidate.py`, splits/allowed_paths/require_output. Reparar veredictos confusos preservando histórico (gerar versão de corpus separada).
- Validar 32 tarefas existentes: base falha e referência passa (já feito B16), mas verificar **não vacuidade**, teste oculto por edge behavior, executabilidade por stack e se ferramentas permitem rodar suíte real.
- Adicionar oráculos de controle (`no-op`, patch errado, patch correto), sonda de acesso ao gabarito por subprocesso, invariante quebrado e timeout.
- Instrumentar **custo desconhecido em uma das voltas**, cache/proveniência, trabalho de instalação, e parsing de tools/subagentes sem contar Bash test como leitura.
- Testar no CI em Linux/macOS/Windows onde possível; sandbox forte em Linux controlado fora do CI quando disponível.

**Gate:** sem vazamento demonstrável, sem seleção silenciosa de runs, métricas auditáveis e comandos reprodutíveis. Enquanto isolamento forte indisponível, não rodar holdout.

### E1 — Retrieval & Relevance (offline, barato)

**Baselines:** lexical/FTS5, V1, V2, V2+adaptive, V3, Aider RepoMap, ARB quando pinado. 40 positivos externos de B02, 180 negativos de B03, 22 longos de B04 = regressões **não cegas**. Introduzir novos repos, novos negativos de PT-BR/EN/voz e positivos longos em validação independente; não ajustar thresholds no mesmo corpus em que vai alegar vitória.

**Metodologia:** base commits fixos, gold arquivos e spans por revisor independente, casos `no-gold` verdadeiros, sem leak de path no prompt, tarefas de localizar e editar. Calibrar regras pelo método de grupos por repo do [ARB](upstream-references.md), não por tokens de nome comuns. Médias macro por repo e micro por caso. Acompanhar E17 ('alíquota') e negativas longas.

**Diagnósticos:** Recall@K, Hit@5, BCY@4k/8k, contexto útil por token, FP+FN, precisão em `abstain`, custo/p95. Nenhum recall médio compensa regressão grave de snippet de contrato.

**Meta exploratória:** FP abaixo de 5% em negativos novos, sem piorar recall externo; não promover o valor como ganho até confirmação fora da amostra.

### E2 — Navigation A/B competitivo (agente real)

**Questão:** a evidência do Sidecar reduz buscas, leituras e tempo para uma resposta de fluxo de código **correta** comparada a um concorrente estrutural? Criar tarefas equivalentes ao corpus CodeGraph com caminhos de call, import, estado, efeito e teste, em JS/Go/Python/Rust/Java e repo pequeno/médio/grande.

**Braços:** `OFF` (Read/Grep/Glob/Bash), `V2` hook, `V3` hook + `sidecar explore` disponível, `CodeGraph` MCP em versão fixada; se o cliente não suporta determinado transporte, marcar par não comparável e não inventar equivalência. Controles extras `BM25`/`Aider RepoMap` offline. Garantir **mesma capacidade**: nenhum acesso diferencial à internet/arquivos/testes, mesmo modelo e teto de custo; fixar overhead de ferramentas/config e provar que OFF não encontra binário do outro braço.

**Correto:** resposta contém conjunto independente de fatos/fluxo/pontos de entrada, com referências de arquivo/linha verificadas. “O agente parou de procurar” não é oracle. Segundo plano: eventos pós-`Explore` classificados como recall vs alocação insuficiente vs suficiente, com parser por thread/subagent e leitura via Bash.

**Artefatos:** traces, prompt, commits, ID servido, tools allowlisted, token/custo real (incl. cache), primeira evidência útil, read counts, saída por caso, custo do índice amortizado. Repetições e CI por repo.

### E3 — Real Patch / Hard Tasks (agente real)

O corpus v2 contém 12 novas tarefas “difíceis” **apenas projetadas**; nenhuma execução agentic de v2 foi reportada. Antes de dimensionar a campanha, rodar piloto controlado no development para ver se ainda existe efeito de teto. Se todos os ocultos passam, ampliar com bugs de comportamento multiarquivo/contratos realmente difíceis, novos repos e revisores externos; não adulterar gold após ver respostas.

**Braços:** OFF, V2, V2+brief, V3; ablações só para módulos com hipótese. **Mesma permissão para rodar testes do projeto por linguagem**: hoje `--tools-allow` limita principalmente a `go test`/`ls`; para Click/Express/Sinatra são necessárias allowlists específicas e seguras para `pytest`/`npm test`/`bundle exec` etc. Permissões mudadas apenas para um braço invalidam o A/B.

**Métrica primária:** taxa de patch comportamentalmente correto, com contratos preservados e escopo não-teste válido. Reportar testes extras permitidos; invariantes, custo por sucesso, total cost, revisões e regressões.

### E4 — Grooming, Impact e duas voltas (agente real)

**Bug metodológico atual:** `followup_for(task)` envia `underspec.canned_answer` na segunda volta mesmo quando o agente não pergunta. Assim mede continuidade de sessão, **não** ROI da pergunta.

**Protocolo válido proposto:** simular usuário **condicional**. Se o agente fizer pergunta relevante antes de editar, simulador entrega a opção verdadeira; se não perguntar, **não revelar a decisão**, e avaliar a implementação contra o requisito oculto ao final. Registrar pergunta útil, redundante, falsa, demora, revisões e resultado; respostas padronizadas por tarefa, não por braço. O `claude -p` headless pode não dispor de ferramenta nativa de pergunta; registrar como transporte limitado e nunca fingir interatividade.

**Impacto:** nota precisa chegar após diff válido, com testes e invariantes conhecidos; distinguir “nota entregue” de “agente leu/usou/alterou”. Comparar duas voltas no mesmo modelo e cenário com E2/E3; não usar piloto B15 N=6 como prova.

### E5 — Long Sessions / Session Intelligence (agente real)

Tarefas de 3+ interações e repositórios com navegação difícil; observar repetição útil após edição versus loop redundante. Variar compactação, branch/diff, retomada, test failures. Persistir metadados mínimos, respeitar cobertura de hooks `unobserved`; intervenção ativa só se transporte real for confirmado e ensaio mostrar melhora **sem** queda de patch pass. Medir cache invalidation e custo de contexto residente.

## 3. Matriz de modelos e preço

Começar com 1 modelo econômico (Haiku ou equivalente) para validar discriminação; repetir candidatas com 1 intermediário (Sonnet ou equivalente) e 1 mais forte. Não usar pesos atribuídos por marketing para “inteligência”: perfis são hipóteses medidas. Modelo servido = modelo pedido (verificação real); versão/limites de cliente, branch, SO, hardware e preços/counters documentados.

**Conservação:** deterministic eval e oráculos gratuitos primeiro; pilotos pagos curtos sob teto **autorizado** depois; expansão só com ROI e poder estatístico. Sem autorização de orçamento, **não** consumir API, assinatura ou crédito do usuário.

## 4. Critérios de promoção e rollback

- **E0 íntegro** (contaminação/timeout/contadores); E1 sem regressão material de recall/negativos; E2 melhora de navegação com resposta certa; E3 melhora ou não inferioridade da qualidade; E4/E5 somente nas capacidades ativadas e testadas.
- Não inferir que CI verde = ganho agentic. Uma feature pode permanecer experimental ou ser removida se piorar custo, tempo ou correção; manter V1/V2 acessíveis.
- Para grandes claims (p.ex. “Sidecar vence CodeGraph”) exigir **mesmos prompts, repos, commits, modelos, versões, permissão, orçamento, ferramenta e métrica**, significância/IC, relatório público com perdas e custos, e reprodutibilidade. Caso contrário reportar “hipótese não verificada”.
- Para cada PR: hipótese, baseline, código, teste de regressão, ablações, novos resultados, limite/rollback; atualizar `bench/registry.json` e gerar `docs/benchmarks.md` com `python3 bench/consolidate.py`.
- **Freeze** do candidato antes de rodar holdout; um único passe do holdout, sem nova tentativa para tunar o mesmo conjunto. Retestar em outro holdout independente após mudanças.

## 5. Comandos conhecidos e execução segura

```bash
go test -count=1 ./...
go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/
go run ./cmd/sidecar-eval --cases bench/cases.json --out /tmp/sidecar-eval.json
python3 -m unittest bench.lab.test_lab
python3 bench/consolidate.py --check
go run ./cmd/sidecar doctor
```

Os comandos completos de `lab.py run`/`report` e `--turns 2` estão em [eval-lab.md](../eval-lab.md) e [benchmarks-2026-11-08.md](../benchmarks-2026-11-08.md). **Não executar** `--final` nem `--yes-spend` sem freeze/autorização específica. Teste de harness `scripted` só valida infraestrutura, sempre `evidence=simulated`, nunca conta como resultado de LLM.

**Próxima execução Codex:** [CODEX-HANDOFF.md](CODEX-HANDOFF.md).