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
go run ./cmd/sidecar doctor
```

Não commitar `bench/results/local.json` ou dados de diagnóstico gerados sem intenção; preferir caminho fora do repo.

## Loop mínimo de dogfooding (manual, barato, reproduzível)

1. Identificar SHA atual, Go/SO, commit alvo e um prompt **pré-definido**. Usar worktree/ambiente isolado quando possível.
2. Rodar `sidecar explain --root . --json` com o prompt em stdin. Guardar localmente trace com `evidence`, `translations`, `timings_ms`, `budget_tokens` e `context`. Não publicar texto sensível.
3. Conferir manualmente se caminho/símbolo/linhas apontam para a implementação verdadeira. Classificar `hit`, `partial`, `miss`, `wrong`, `no_evidence_correct`.
4. Repetir após a mudança; rodar as mesmas queries, mesmas fixtures e comparação com baseline. Se houve ganho de retrieval, registrar também o custo incremental de tokens e latência.
5. Para validar auxílio ao **agente**, executar protocolo separado em `docs/evaluation-plan.md`; `explain` sozinho não mede rounds de modelo.
6. Registrar falhas reproduzíveis como casos de regressão sem inserir os resultados da avaliação cega no conjunto de tuning.

Exemplos de perguntas de dogfooding (não são gabaritos):
- “Onde é montado o contexto adicional do hook no Claude Code?”
- “Onde o índice lida com arquivo renomeado no Git?”
- “Como um hook com erro evita interromper o agente?”
- “Onde o instalador preserva outros UserPromptSubmit hooks?”

## Executando entregas do produto V2

1. Ler `docs/v2-product-spec.md` e a trilha de `docs/roadmap.md` antes de modificar código. V2 é um produto completo; PRs são incrementos para viabilizar integração e rollback, não um MVP.
2. Abrir `docs/open-source-references.md` **para o módulo da tarefa**, inspecionar os repositórios pertinentes (URLs/SHAs e licenças), e registrar alternativas `integrate/reuse/independent/discard` com custos/testes. Não executar scripts upstream em ambiente do usuário.
3. Escolher baseline v1, fixtures públicas pinadas e caso de uso privado PT/EN; definir primeiro o teste para o gargalo específico: round trips, buscas circulares, discrepâncias docs/código, contexto desperdiçado ou contrato ignorado.
4. Implementar com feature flag/rollback e testes de componente, integração real e segurança; verificar hooks dos clientes antes de prometer intervenção ou instalação de LSP.
5. Rodar comparações offline sem modelos pagos quando possível. Apenas testar agente com ON/OFF em sessão isolada e orçamento explícito. Registrar resultado adverso.
6. Atualizar ADR, README, README/roadmap e evidências no mesmo PR. Nunca declarar tarefa concluída só porque a feature compila.

**Checklist completo de testes pendentes:** `docs/roadmap.md#testes-pendentes-check-list-consolidado`; anexar evidências dos ambientes realmente usados, e apontar `not-run` em vez de `pass` para outros.

## Contrato de alteração

- Definir antes a hipótese mensurável. Preferir mudanças pequenas e reversíveis.
- Regra de teste: fix de bug ⇒ regressão; ranking ⇒ casos por estratégia e ablação; instalação ⇒ install/reinstall/upgrade/uninstall; hook ⇒ contrato e E2E em cliente real.
- Revisar diferenças em Windows e macOS; não tratar cross-build como prova de funcionamento da integração.
- Atualizar documentação impactada **antes de concluir o PR**, incluindo números e limitações reais.
- No PR: `Motivo / Escopo / Testes realmente executados / Comparação antes-depois / Riscos-rollback / Docs atualizadas`. Não afirmar que um teste rodou se não rodou.

## Aprovação de mudanças de maior risco

Mudanças em schema externo, hook JSON, caminhos de instalação, remoção de binário/cache, log de prompts, execução de comandos do repositório e acesso à rede exigem revisão explícita. Evitar testes pagos e alterações em configurações pessoais de agentes sem opt-in e ambientes temporários.
