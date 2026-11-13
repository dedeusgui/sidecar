# Sidecar 3.6 → 4.0 — milestones de produto e entregas

> Proposta de direção de produto (2026-11-10). Não representa novas releases ou promoção de engine. Estado verificado ao elaborar: `main@a503a164`; a V3.6 é opt-in experimental e V1 é o default. Conferir sempre o SHA de execução.

## Norte do produto

**Objetivo competitivo: superar todos os concorrentes relevantes nos benchmarks que conseguirmos executar de maneira econômica, reprodutível e justa.** Buscamos liderança em **tarefa corretamente concluída por tempo, custo e recursos consumidos**, não apenas em Hit@K, volume de contexto ou uma tabela favorável. Priorizar validação gratuita/offline e modelos econômicos onde a avaliação com agentes trouxer informação adicional. Registrar derrotas, incerteza, limitações, custos de infraestrutura e casos sem comparabilidade. Não afirmar vitória sem evidência independente; não ajustar engine ao holdout nem selecionar apenas benchmarks vantajosos.

O Sidecar deve instalar uma vez, entender o repositório localmente e entregar automaticamente **o menor contexto suficiente** ao agente; oferecer aprofundamento opcional, sem obrigação de aprender comandos proprietários. Core local-first, determinístico, sem LLM, rede ou serviço obrigatório em runtime; hooks fail-open, privacidade e rollback.

## Nomes e estados

- **Sidecar 3.x** é a versão de produto: motor, CLI, adaptadores, UX, instalação, testes, docs, release e métricas. A **Engine 3.x** é o comportamento selecionável dentro desse produto. Em cada marco, alinhar o nome da engine ao produto, mas não fingir que a versão de produto implica promoção da engine.
- **Milestone entregue** = critérios verificáveis concluídos e documentados. **Release publicada** = tag, binários identificáveis, changelog e install/upgrade/uninstall validados. **Engine promovida a default** = decisão separada após benchmarks e gates. Um milestone experimental pode terminar com engine opt-in.
- V3.5/V3.6 e seguintes permanecem disponíveis para A/B. V1 continua default até decisão formal de promoção. O número de versão não significa ganho automático.
- **Patch** (`3.x.y`) corrige defeitos do milestone sem reabrir o desenho. **Milestone** (`3.(x+1)`) entrega hipótese/funcionalidade nova com avaliação. Não rebatizar resultados históricos como releases.

## Plano de entrega — 3.6 a 4.0

| Milestone | Valor observável para o usuário | Entregáveis / hipóteses | Critérios para encerrar |
| --- | --- | --- | --- |
| **Sidecar 3.6 — Reliable Context** | Receber evidência correta e verificável em vez de orientação solta ou item omitido | Fechar pipeline seleção → planner → packing → hook; fallback após alocação e *entrega*, limites reais, deduplicação, confiança, stale; corrigir regressões de abstenção sem decorar corpus; scorecard de entrega offline | Testes que reproduzem falhas antes e passam depois; diferenciação de gate e packing; relatório por caso e estrato; Go/race/lint/contratos hook simulados; limitações e perdas registradas; não declarar ganho agentic |
| **Sidecar 3.7 — Context Intelligence** | Menos busca manual e contexto inútil, com cobertura útil preservada | Comparar V3 e V3.6; investigar recuperação, seletividade, snippets, QuickExpand automático versus Explore opcional; ablações por componente e língua; custo por contexto suficiente | Corpus externo independente e critérios congelados; comparação pareada offline com concorrentes permitidos; evidência de contribuição marginal; nenhuma regressão crítica escondida |
| **Sidecar 3.8 — Agent Workflow** | Reduzir releituras e recuperação repetitiva durante a tarefa | Continuidade de sessão verificável, observabilidade de eventos suportados, necessidades de contexto por fase, contratos reais Claude/Codex e aprofundamento acionável; recursos condicionados ao suporte real | Testes E2E por cliente e testes de sessão/compaction/fail-open; replay offline de comportamento; pilotos pagos pequenos só com autorização; medir ações após entrega e custo por tarefa correta |
| **Sidecar 3.9 — Integration & Release Candidate** | Instalação simples, confiabilidade, diagnósticos e comparação objetiva entre versões | Consolidar somente abordagens vencedoras; remover duplicação, compatibilidade, segurança, release cross-platform, upgrade/uninstall/rollback; painel HTML de benchmarks e relatório de produto automatizados | RC instalável e reversível; CI de regressão, performance e integração; comparativos reproduzíveis por SHA; dashboards gerados de dados rastreáveis; dívida crítica enumerada |
| **Sidecar 4.0 — Unified Sidecar** | Experiência completa e coesa: contexto automático, exploração, continuidade e avaliação confiáveis | Consolidar arquitetura a partir do que 3.6–3.9 demonstraram; um conjunto claro de interfaces e políticas, documentação de arquitetura e operação, release estável | Gates de correção/segurança/compatibilidade e ganho líquido pré-registrados; holdout novo independente e E2E real; benchmark competitivo honesto; release, migração e rollback documentados |

Os escopos de 3.7–4.0 são **hipóteses e entregáveis propostos**, não compromissos de implementar uma técnica que se mostre pior que alternativas. Nenhuma refatoração total é obrigatória: a V4 consolida o que os dados justificarem.

## Duas trilhas em paralelo

1. **Motor e produto (slices curtas):** problema observado → teste offline que falha → correção mínima → ablação → documentação de decisão → PR pequeno. Evitar a implementação de uma versão inteira em um único PR. Executar tarefas independentes em paralelo somente quando isolamento e interfaces permitirem.
2. **Avaliação e laboratório (plataforma contínua):** schema de evidências, corpus versionado, replay determinístico, scorecard automático e painel HTML; melhorar a infraestrutura uma vez e reutilizar de 3.6 até 4.0. Quando a avaliação encontra falha, registrar issue e caso reproduzível.

## Gates progressivos e velocidade de entrega

- **Por alteração (rápido, sem custo de modelo):** unit tests, propriedade/invariantes, snapshot de saída, determinismo, lint, contratos dos adaptadores, orçamento de tokens e privacidade; falhas interrompem merge.
- **Por PR de engine (offline):** casos sintéticos novos, fixtures e repositórios externos pinados, ablações contra base, negativos/abstenção, cold/warm quando pertinente, por arquivo e por classe. Preservar artefatos e reportar regressões.
- **Por milestone (integração):** E2E instalado por cliente suportado, reversibilidade, matriz SO e performance controlada; revisão de riscos e documentação; preparar RC/patch sem bloquear desenvolvimento paralelo do marco seguinte.
- **Por reivindicação competitiva ou mudança de default:** benchmark com regras e oráculos independentes, controles justos, repetição e incerteza; agentes pagos somente após qualificação do harness e autorização explícita de orçamento; decidir com base em custo/tempo **por tarefa correta**, não só métricas internas. Não usar holdout já exposto como cego.

**Gate não é veto absoluto a todo release experimental:** uma funcionalidade pode ser entregue opt-in com limitações documentadas. Já qualidade mínima, segurança e integridade do instrumento são obrigatórias para todo merge.

## Linha do tempo da arquitetura V4

Cada milestone atualiza `docs/v4/architecture-decision-log.md` (ou registro equivalente) com: problema, alternativa testada, decisão, evidência favorável/desfavorável, custo e compatibilidade, interface afetada, reversibilidade e hipótese ainda aberta. A V4 deve ser o resultado desse histórico, não um diagrama especulativo escrito só no final.

## Entregáveis de documentação por marco

- `docs/engines.md`: capacidades **implementadas**, **exercidas**, **não verificadas**, versão e cliente.
- `docs/v3.x/progress.md`: entregas concluídas e pendentes, riscos, links para código e dados, desempenho, limitações.
- Changelog de produto e notas de release por tag; link para PRs/commits.
- `bench/registry.json` e documentação gerada: proveniência, corpus, versão de engine, custo, critérios, e negativas.
- Dashboard HTML gerado do registro de resultados, seguindo `docs/benchmark-observatory.md`.

## Dependências e próximos passos imediatos

1. Encerrar V3.6: teste de proposta alocada mas não entregue pelo packer; falsos positivos V3.6, falsos negativos e cobertura; corrigir somente defeitos reproduzidos.
2. Tornar o scorecard offline de entrega replicável fora da máquina original (manifesto/checkout pinado e checksums) e incorporar ao CI econômico.
3. Construir o primeiro Observatory HTML gerado a partir dos resultados existentes e marcados por nível de evidência; evitar cópia manual de números.
4. Congelar plano experimental V3.7, incluindo concorrentes em condições comparáveis (ex.: CodeGraph e Aider RepoMap com caveats), e iniciar slices curtas.
5. Avaliar o resultado e manter V3.8–V4 ajustáveis às evidências. Não gastar limites do Codex/Claude em campanhas de qualificação.

## Progresso implementado desta campanha (3.6 → 3.7 infra)

- **Pipeline P0–P3 e Observatory HTML (`$0`):** `bench/fast_gate.py` executa P0 (`go test`, `unittest`, opcional `-race`) + P1 (`sidecar-eval --delivery`, E1/E2 opcionais e gate sem regressão), `internal/studio` renderiza os esquemas `delivery/1` e `e2-offline/1` em HTML único, `bench/v3/e2_offline.py` aceita `--engines` e `bench/lab/lab.py` suporta os braços `V3.6`/`V3.7` e detecção nvm do `@colbymchenry/codegraph`.
- **Engine V3.7 opt-in (`SIDECAR_ENGINE=v3.7`):** implementada em `internal/app/v37.go`, `internal/engine`, `internal/retrieve`, `internal/extract`, `internal/index`, `internal/store/query.go` e `internal/explore`, passando nos portões offline P0 e P1 (ver [docs/v3.7/progress.md](v3.7/progress.md) e [docs/v3.7/architecture.md](v3.7/architecture.md)). **V1 continua default**; o encerramento do marco exige fechar os gaps de `explore` (`23/26` vs CodeGraph `24/26`), `E1` negativos (`16/40`), `voice_mode` top-3 (`6/10`), os 3 itens `staticcheck` do `golangci-lint` e executar a validação **P3** autorizada.

Referências: [motores e diferenças](engines.md), [progresso 3.6](v3.6/progress.md), [pesquisa 3.7](v3.7/research-plan.md), [progresso e promoção](engine-progression.md), [metodologia](evaluation-plan.md).
