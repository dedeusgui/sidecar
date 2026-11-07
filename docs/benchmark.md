# Benchmark de recuperação

Este documento registra como o Sidecar foi medido, o que os números mostram e o que eles **não** provam.
Todos os valores abaixo foram produzidos por `cmd/sidecar-eval` e estão em `bench/results/final-state.json`.

## Ambiente

- CPU: Intel(R) Xeon(R) Processor @ 2.10GHz · 4 núcleos · linux/amd64 · go1.24.7
- Data da medição: 2026-11-05T02:35:47Z
- Build: `CGO_ENABLED=0`, sem otimizações de release.

## Metodologia

**Repositórios de fixture.** Dois projetos pequenos (`bench/testdata/fixtures/tune` e `bench/testdata/fixtures/eval`) com
15 arquivos cada, em Go, Python, TypeScript, Java, C#, SQL e Markdown, com identificadores em inglês.
Cada fixture é montada como repositório Git real com histórico (pares de arquivos co-alterados em
commits separados) e uma alteração não commitada, para exercitar os sinais de Git.

**Casos.** `bench/cases.json` contém consultas em português com os símbolos relevantes definidos
*antes* de qualquer medição, e consultas negativas (saudações, perguntas gerais) que não devem gerar contexto.

- **tune** (26 casos): usado para ajustar o pipeline.
- **eval** (24 casos): usado para diagnóstico durante o desenvolvimento. Os erros desse split influenciaram
  alterações do pipeline, então **não é uma estimativa limpa**.
- **holdout** (21 casos): escrito depois de os ajustes principais, mas **medido várias vezes** durante a
  correção de defeitos encontrados em um repositório real (petclinic). Também **não é uma estimativa limpa**.

Não existe, nesta rodada, um split realmente nunca visto. A próxima medição precisa de casos novos.

**Métricas.** Hit@5 (algum símbolo relevante no top 5), Recall@10 (fração dos relevantes no top 10),
MRR (1 / posição do primeiro relevante), Precisão (relevantes / itens retornados, média sobre
consultas com resultado), taxa sem evidência (positivos sem nenhum item), falso positivo (negativos
com algum item), latência P50/P95 da chamada de recuperação em processo, e tokens médios do bloco de contexto
(aproximação de 1 token por 4 caracteres).

**Métodos.**

- `textual (substring)`: palavras do prompt com 4+ letras, sem tradução nem stemming, buscadas como substring do nome e do caminho.
- `bm25 (fts5, raw terms)`: ranking FTS5/BM25 dos termos analisados, sem tradução.
- `bm25 + traducao/normalizacao`: igual ao anterior, com tradução PT→EN presente no repositório, sem grafo nem Git.
- `lexical + grafo`: pipeline completo sem sinais de Git (sem co-alteração e sem alteração não commitada).
- `lexical + grafo + git`: pipeline completo (padrão do produto).

## Resultados

### Split: tune

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 15.0% | 15.0% | 0.117 | 13.2% | 40.0% | 0.0% | 0.21 | 0.35 | 12 |
| bm25 (fts5, raw terms) | 20.0% | 20.0% | 0.085 | 7.0% | 35.0% | 0.0% | 0.56 | 0.95 | 70 |
| bm25 + traducao/normalizacao | 70.0% | 82.5% | 0.557 | 13.0% | 0.0% | 0.0% | 1.28 | 2.23 | 248 |
| lexical + grafo | 80.0% | 80.0% | 0.604 | 12.6% | 0.0% | 0.0% | 7.54 | 9.68 | 304 |
| lexical + grafo + git | 80.0% | 80.0% | 0.604 | 12.6% | 0.0% | 0.0% | 8.09 | 9.46 | 354 |

### Split: eval

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 10.0% | 10.0% | 0.075 | 25.0% | 80.0% | 0.0% | 0.20 | 0.35 | 6 |
| bm25 (fts5, raw terms) | 20.0% | 20.0% | 0.163 | 37.5% | 70.0% | 0.0% | 0.41 | 0.75 | 26 |
| bm25 + traducao/normalizacao | 85.0% | 85.0% | 0.572 | 23.4% | 10.0% | 0.0% | 0.95 | 1.34 | 175 |
| lexical + grafo | 75.0% | 75.0% | 0.567 | 23.1% | 10.0% | 0.0% | 4.37 | 9.65 | 185 |
| lexical + grafo + git | 75.0% | 75.0% | 0.567 | 23.1% | 10.0% | 0.0% | 6.39 | 11.20 | 214 |

### Split: holdout

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 11.8% | 11.8% | 0.088 | 25.0% | 76.5% | 0.0% | 0.19 | 0.24 | 7 |
| bm25 (fts5, raw terms) | 17.6% | 17.6% | 0.176 | 29.2% | 64.7% | 0.0% | 0.53 | 0.87 | 29 |
| bm25 + traducao/normalizacao | 70.6% | 76.5% | 0.487 | 20.8% | 17.6% | 0.0% | 0.88 | 1.39 | 165 |
| lexical + grafo | 64.7% | 64.7% | 0.490 | 20.2% | 17.6% | 0.0% | 3.89 | 7.93 | 185 |
| lexical + grafo + git | 64.7% | 64.7% | 0.490 | 20.2% | 17.6% | 0.0% | 4.89 | 9.35 | 215 |

## Leitura honesta

1. **A tradução PT→EN é o componente que mais contribui.** Em holdout, o BM25 cru acerta o top 5 em 17,6%
   das consultas; com tradução e normalização, sobe para 70,6%.
2. **O grafo estrutural não mostrou ganho confiável sobre tradução pura.** Em tune, grafo + léxico ficou
   10 pontos acima; em eval e holdout, ficou 6 a 10 pontos abaixo. Com 17 a 26 consultas por split, uma
   diferença de 2 casos não é estatisticamente sólida. A evidência atual **não sustenta** a afirmação de que
   o grafo melhora a recuperação.
3. **Os sinais de Git não mudaram os resultados.** Co-alteração e alteração não commitada aparecem como
   anotações, mas nenhum caso mediu ganho. Esta fixture tem pouca relação de co-alteração útil para as
   consultas, então isso é um resultado negativo fraco, não uma conclusão.
4. **Negativos:** nenhum falso positivo em nenhum método do pipeline. O pipeline completo também não
   injeta contexto em saudações. Em holdout, a taxa sem evidência de 17,6% é o custo dessa precisão.
5. **Falhas recorrentes em holdout** (todas as variantes com tradução): palavras que o dicionário não
   cobre, como `encomenda`, `pacote`, `rastreio` e `cupom`, e intenções sem palavra correspondente no
   código, como `tentar cobrar novamente` → `MAX_RETRY_ATTEMPTS`. Cobertura de vocabulário é o limite
   principal hoje.

## Rodada B3–B5: contrato e empacotamento (v2)

Medido em 2026-11-06 com `go run ./cmd/sidecar-eval --cases bench/cases.json` (retrieval offline, sem agente). Hit@5, Recall@10, MRR e precisão em %, tokens = média do bloco renderizado.

| Split | Método | Hit@5 | Recall@10 | MRR | Precisão | Falso positivo (negativos) | Tokens |
|---|---|---|---|---|---|---|---|
| tune | v1 (`lexical + grafo + git`) | 80,0 | 80,0 | 0,629 | 12,6 | 0,0 | 354 |
| tune | v2 (intent + contract + packer) | 80,0 | 80,0 | 0,629 | 12,7 | 0,0 | 365 |
| tune | v2 sem intent | 80,0 | 80,0 | 0,629 | 12,7 | 0,0 | 373 |
| tune | v2 sem contract | 80,0 | 80,0 | 0,629 | 12,6 | 0,0 | 345 |
| tune | v2 sem packer | 80,0 | 80,0 | 0,629 | 12,6 | 0,0 | 369 |
| eval | v1 (`lexical + grafo + git`) | 75,0 | 75,0 | 0,567 | 23,1 | 0,0 | 214 |
| eval | v2 (intent + contract + packer) | 75,0 | 75,0 | 0,567 | 23,3 | 0,0 | 215 |
| eval | v2 sem intent | 75,0 | 75,0 | 0,567 | 23,3 | 0,0 | 218 |
| eval | v2 sem contract | 75,0 | 75,0 | 0,567 | 23,1 | 0,0 | 210 |
| eval | v2 sem packer | 75,0 | 75,0 | 0,567 | 23,1 | 0,0 | 219 |
| holdout | v1 (`lexical + grafo + git`) | 64,7 | 64,7 | 0,490 | 20,2 | 0,0 | 215 |
| holdout | v2 (intent + contract + packer) | 64,7 | 64,7 | 0,490 | 20,2 | 0,0 | 216 |
| holdout | v2 sem intent | 64,7 | 64,7 | 0,490 | 20,2 | 0,0 | 223 |
| holdout | v2 sem contract | 64,7 | 64,7 | 0,490 | 20,2 | 0,0 | 206 |
| holdout | v2 sem packer | 64,7 | 64,7 | 0,490 | 20,2 | 0,0 | 216 |

Leitura:

1. **Sem regressão de recuperação, sem economia de tokens.** A v2 completa iguala a v1 em Hit@5, recall e MRR nos três splits. Os tokens sobem 0,5% a 3%: o custo das anotações de contrato. O packer não reduz tokens sem perder acertos: testei pool largo (até 3× o top-8), limiares de valor por token de 0,001 a 0,005 e um teto de itens. No tune, todas as variantes que reduziram tokens perderam Hit@5: 5 pontos com teto de 8 itens (320 tokens) e até 35 pontos com limiar 0,005 e pool largo (148 tokens). A configuração defendida só remove ou reordena dentro do top-8 da v1, e o limiar 0,001 foi escolhido no split tune.
2. **O MRR da própria v1 oscila entre execuções** (0,604 e 0,629 no mesmo código do `HEAD`). A ordenação de símbolos depende da indexação paralela, como já registrado acima. Diferenças de MRR menores que ~0,03 não são evidência.
3. **Nenhum conflito no bench.** Nos 71 prompts, os vínculos foram `IMPLEMENTED` 103, `TEST_EXPECTED` 59, `INFERRED` 16 e `DOCUMENTED` 6, com 0 `UNVERIFIED` e 0 `CONFLICT_CANDIDATE`. O zero de falsos positivos é verdadeiro, mas não mede a detecção de conflito: essa só é exercida pelos testes sintéticos de `internal/retrieve/contract_test.go`. Um conjunto com divergências reais precisa ser criado antes de afirmar precisão ou recall de conflito.
4. **Não é medição agêntica.** Estes números não dizem se o agente chega à tarefa correta com menos tempo ou tokens. Isso exige o protocolo de `evaluation-plan.md` com agentes reais.

## Latência em repositório real

Medição com o binário de release (`-trimpath`, grammars de subconjunto, 16,2 MB) no Spring PetClinic
(132 arquivos, 50 Java, 1,7 MB de código-fonte, histórico Git real):

| Medida | Valor |
|---|---|
| Primeiro prompt a frio (indexa 132 arquivos) | 311 ms |
| Hook em regime, processo completo (30 execuções) | p50 24 ms · p95 26 ms · máx 29 ms |
| Tempo interno do hook (log) | p50 17,8 ms · p95 22,6 ms |

As metas de projeto (p50 ≤ 50 ms, p95 ≤ 150 ms) são atendidas **nesse repositório e nesta máquina**.
Repositórios muito maiores não foram medidos; o limite de 300 arquivos por prompt existe para proteger a latência,
e o restante vai para o indexador em segundo plano.

## Reproduzir

```sh
go run ./cmd/sidecar-eval --cases bench/cases.json --out bench/results/run.json
go run ./cmd/sidecar-eval --cases bench/cases.json --compare bench/results/final-state.json --report-md <fora-do-repo>/relatorio.md
```

O harness constrói os repositórios de fixture em um diretório temporário, indexa, roda os métodos e imprime as tabelas.

Com `--compare`, o avaliador imprime o delta de Hit@5, Recall@10, MRR, precisão, falso positivo, P50 e tokens por split e método, e lista nominalmente os casos que regrediram (Hit@5 perdido ou falso positivo novo em caso negativo) ou melhoraram. Ele sai com código 0 mesmo com regressões: a decisão fica com quem lê o relatório. Latência varia entre máquinas; compare-a só com o cabeçalho de hardware do baseline.

## Verificação de regressão — Rodada D (2026-11-06)

Mudança: `.rs`, `.php`, `.rb` e `.kt` passam a ser indexados com as gramáticas embutidas, e `extractVersion` sobe para 2. O corpus `bench/testdata` não tem arquivos dessas extensões, então a comparação mede só a ausência de regressão, não ganho.

- Comando: `go run ./cmd/sidecar-eval --cases bench/cases.json --out <tmp>`, antes e depois da mudança.
- Hit@5, Recall@10, precisão e falso positivo ficaram **idênticos** em todos os métodos, em ambos os splits.
- BM25, léxico e grafo: MRR idêntico.
- `textual (substring)`: MRR variou entre execuções **do mesmo binário** (0,075, 0,100 e 0,117 no split tune). A variação vem do desempate do `sort.SliceStable` sobre a ordem de `AllSymbols()`, que não é determinística. Isso já existia antes da mudança e não foi corrigido nesta rodada. Até corrigir, não compare MRR do baseline textual entre execuções.
- Limite: o corpus de avaliação não cobre a mudança de linguagens. Os casos de Rust, PHP, Ruby e Kotlin foram verificados à parte, com um repositório temporário, pelo `sidecar explain`.
