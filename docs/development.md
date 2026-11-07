# Desenvolvimento e dogfooding do Sidecar

## Pré-requisitos e estado

- Go 1.24+ e Git para desenvolvimento. Os pacotes do projeto estão em `go.mod`; **não** se exige Node.js, serviço de embeddings ou modelo para rodar o núcleo.
- CI existente: testes Go em Linux/macOS/Windows, race detector seletivo em Linux, lint, cross-build e benchmark offline.
- O produto usa **hooks por usuário**. Não editar automaticamente um repositório de terceiros para acomodar Sidecar.
- Os resultados e relatos de `docs/benchmark.md` são históricos; não representam benchmark agentic já validado.

## Mapa de leitura por problema

| Se mexer em… | Comece por… | Verifique também |
|---|---|---|
| Eventos Claude/Codex | `internal/adapter`, `internal/install` | `internal/app/diagnostics.go`, testes E2E |
| Busca, ranking e contexto | `internal/retrieve`, `lexical`, `fusion` | `brief`, `ctxbuild`, `bench/` |
| Indexação e Git | `internal/index`, `gitx`, `extract` | `store`, segurança de paths |
| SQLite, corrupção, concorrência | `internal/store` | `internal/index`, `app`, race tests |
| Instalação ou release | `internal/install`, `cmd/sidecar` | `.goreleaser.yaml`, `.github/workflows/` |

## Desenvolvimento rápido

```bash
go test -count=1 ./...
go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/
go run ./cmd/sidecar-eval --cases bench/cases.json --out bench/results/local.json
go run ./cmd/sidecar-eval --cases bench/cases.json --compare bench/results/final-state.json --report-md <fora-do-repo>/relatorio.md
go run ./cmd/sidecar-eval --cases bench/v2_scenarios.json --report-md bench/results/v2-scenarios.md
go run ./cmd/sidecar-eval --external-cases <manifesto-pinado.json> --out <fora-do-repo>/externo.json
go run ./cmd/sidecar-eval --agentic-report <diretorio-com-runs.jsonl> --agentic-manifest bench/agentic/manifest.jsonl
go run ./cmd/sidecar doctor
```

`--external-cases` exige que cada repositório esteja limpo e no commit fixado no manifesto; não clona nem faz checkout. `--agentic-report` só lê registros; não executa agentes, e com `--report-md` grava o relatório no arquivo indicado. Tarefas `draft` do manifesto não recebem veredito. Os experimentos rejeitados (peso RRF dos vizinhos e marcação de vizinhos) estão em [benchmark.md](benchmark.md), seção Rodada E.

O método `v2.1 (intent router)` roteia só prompts com confiança de intenção ≥ 0,55 e limita a expansão estrutural às famílias da intenção; os demais produzem os mesmos itens do v1 (coberto por `TestRoutingFallsBackToV1BelowThreshold`). Ele não tem baseline em `final-state.json`, então aparece como não comparado até uma nova rodada congelada. Para ver quantos prompts roteiam, use `sidecar explain --json` e leia `intent` e `intent_routed`.

Não commitar `bench/results/local.json` ou dados de diagnóstico gerados sem intenção; preferir caminho fora do repo.

## Loop mínimo de dogfooding (manual, barato, reproduzível)

1. Identificar SHA atual, Go/SO, commit alvo e um prompt **pré-definido**. Usar worktree/ambiente isolado quando possível.
2. Rodar `sidecar explain --root . --json` com o prompt em stdin. Guardar localmente trace com `evidence` (cada item traz `id`, `source`, `confidence` e `token_estimate`), `candidates` (cada um traz `reason_selected` ou `reason_suppressed`), `intent` (`kinds`, `confidence`, `signals`), `repo_fingerprint`, `translations`, `timings_ms`, `budget_tokens` e `context`. Não publicar texto sensível.
3. Conferir manualmente se caminho/símbolo/linhas apontam para a implementação verdadeira. Classificar `hit`, `partial`, `miss`, `wrong`, `no_evidence_correct`.
4. Repetir após a mudança; rodar as mesmas queries, mesmas fixtures e comparação com baseline. Se houve ganho de retrieval, registrar também o custo incremental de tokens e latência.
5. Para validar auxílio ao **agente**, executar protocolo separado em `docs/evaluation-plan.md`; `explain` sozinho não mede rounds de modelo.
6. Para o pipeline v2 (contrato e empacotamento), use `sidecar explain --engine v2 --json`. O hook usa `SIDECAR_ENGINE=v2` (qualquer outro valor, ou ausência, mantém v1) e `SIDECAR_PROFILE=minimal|balanced|deep` para fixar o orçamento; sem perfil o orçamento segue o tamanho da evidência, como na v1. O trace v2 traz `contracts` (cada vínculo com status) e `packing` (orçamento, tokens usados e motivo da abstenção).
7. Registrar falhas reproduzíveis como casos de regressão sem inserir os resultados da avaliação cega no conjunto de tuning.

Consultas pinadas em `bench/dogfood_cases.json`, verificadas por `TestDogfoodQueries` (`go test ./internal/app/ -run TestDogfoodQueries`) contra um snapshot congelado (`dogfoodSnapshot`: arquivos de produção de `internal/app`, `internal/gitx` e `internal/install`), não contra o repositório vivo. Os positivos exigem o par arquivo+símbolo no top-5 do `explain`; os negativos não podem injetar contexto. Se um símbolo for renomeado ou um arquivo movido, atualize o arquivo e a lista no mesmo PR. Não são gabarito de tuning e não justificam mudar ranking sem A/B em `bench/cases.json`.

Limite conhecido: a consulta D01 apontava para `adapter.Format`. Depois dos testes novos em `internal/app/app_test.go`, `Format` caiu para o 6.º lugar (0,0323, contra 0,0325 da seção de `CLAUDE.md`), por causa de identificadores com “hook” no código de teste. O gabarito passou a ser `app.Hook`, que monta e emite o contexto do hook; **o ranking não foi alterado**. A margem é fina: outra mudança no repositório pode reordenar esses resultados.

Perguntas pinadas (positivas):
- “Onde é montado o contexto adicional do hook no Claude Code?”
- “Onde o índice lida com arquivo renomeado no Git?”
- “Como um hook com erro evita interromper o agente?”
- “Onde o instalador preserva outros UserPromptSubmit hooks?”

## Autodogfooding e feedback pós-tarefa

Quem desenvolve o Sidecar deve trabalhar com ele instalado, para que o próprio uso gere evidência:

1. Instalar com consentimento explícito (`--yes` autoriza a alteração dos hooks; `sidecar uninstall` desfaz): `sidecar install --yes --statusline --observe-tools`.
2. Trabalhar normalmente. A injeção acontece no prompt. Com `SIDECAR_ENGINE=v2`, o feedback abaixo também passa a afetar o ranking.
3. Ao terminar a tarefa, registrar quais evidências ajudaram ou enganaram, usando os IDs exatos de `sidecar explain --json` (`evidence[].id`):
   ```bash
   sidecar feedback record --root . --completed=true --confidence high \
     --helpful 'internal/app/feedback.go#function:RecordFeedback@NN' \
     --misleading 'internal/x.go#Y' --reason wrong_file
   ```
   Ou envie o relatório completo pela entrada padrão, com `schema_version: 1` (ver [agent-feedback.md](agent-feedback.md)).
4. Conferir com `sidecar feedback list --json`. Remover os obsoletos com `sidecar feedback purge`; `--all` remove tudo do worktree.

Fórmula, em resumo (detalhes e limites em [agent-feedback.md](agent-feedback.md)): pesos 0,45 para o oráculo (`oracle_passed`, por padrão `task_completed`), 0,35 para o rastro do `SessionObserver` (o agente leu ou editou o arquivo?) e 0,20 para o autorrelato, escalado pela confiança. Quando o rastro contradiz um elogio, o peso do autorrelato vale `0,25 ×`. O resultado é limitado a ±0,25 por alvo e decai `0,85^n` com registros mais antigos. Cada registro guarda o hash do arquivo: ao mudar e reindexar o arquivo, ele vira `stale` e deixa de valer. Feedback não gera ganho comprovado: avaliação end-to-end continua em [evaluation-plan.md](evaluation-plan.md).

## Contrato de alteração

- Definir antes a hipótese mensurável. Preferir mudanças pequenas e reversíveis.
- Regra de teste: fix de bug ⇒ regressão; ranking ⇒ casos por estratégia e ablação; instalação ⇒ install/reinstall/upgrade/uninstall; hook ⇒ contrato e E2E em cliente real.
- Revisar diferenças em Windows e macOS; não tratar cross-build como prova de funcionamento da integração.
- Atualizar documentação impactada **antes de concluir o PR**, incluindo números e limitações reais.
- No PR: `Motivo / Escopo / Testes realmente executados / Comparação antes-depois / Riscos-rollback / Docs atualizadas`. Não afirmar que um teste rodou se não rodou.

## Testes de risco e limites conhecidos

- Smoke E2E de instalação e hooks: `go test ./internal/app/ -run 'Smoke|Shell|Unicode|Concurrent|Damaged'`. O teste de shell compila o binário real e é pulado com `-short` e no Windows.
- Corrupção: o hook move o índice para `<db>.corrupt` quando o SQLite acusa dano na abertura ou numa consulta (`malformed`, `not a database`, `no such fts5 table`) e responde uma vez com índice reconstruído. `database is locked` não causa quarentena.
- Limite: dano silencioso que nenhuma consulta toca não é detectado pelo hook, que responde normalmente. `sidecar doctor` detecta esse caso pelo `quick_check`.
- `sidecar-eval --compare` marca regressão por caso só para Hit@5 e falso positivo, conforme o critério definido; mudanças apenas de posição aparecem no Δ MRR agregado, não na lista de casos.

## Aprovação de mudanças de maior risco

Mudanças em schema externo, hook JSON, caminhos de instalação, remoção de binário/cache, log de prompts, execução de comandos do repositório e acesso à rede exigem revisão explícita. Evitar testes pagos e alterações em configurações pessoais de agentes sem opt-in e ambientes temporários.

## Implementação da V2 completa

Antes de mexer no núcleo, ler [product-v2.md](product-v2.md), [roadmap.md](roadmap.md) e **apenas as linhas pertinentes** do [catálogo de implementações](reference-implementations.md). Investigar upstreams e testes do problema para não reinventar componentes maduros; licenças devem ser validadas no SHA/arquivo. Registrar escolhas e custo de runtime no PR.

Ordem operacional: corrigir setup/release/hooks, congelar baseline e traces, corpus externo offline, IntentRouter/ContractEvidence/EvidencePacker, SessionObserver/DecisionMemory, LSP/providers/adapters, benchmarks agentic e hardening final. Cada integração passa por unit/contract tests, E2E real quando aplicável e benchmark antes/depois. Nenhum módulo que só compila é considerado validado.

Para alteração de **contratos** adicionar caso em que docs e código estão alinhados, caso em que divergem e caso ambíguo; exigir referência de arquivo/linha, falsos conflitos medidos e ausência de instrução inventada. Para observador testar eventos indisponíveis, redaction, loops legítimos e injeção silenciosa por padrão. Para install/LSP testar consentimento e rollback.
