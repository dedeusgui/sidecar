# Benchmark de recuperação

Este documento registra como o Sidecar foi medido, o que os números mostram e o que eles **não** provam.
Todos os valores abaixo foram produzidos por `cmd/sidecar-eval` e estão em `bench/results/final-state.json`.

## Ambiente

- CPU: Intel(R) Xeon(R) Processor @ 2.10GHz · 4 núcleos · linux/amd64 · go1.24.7
- Data da medição: 2026-11-05T02:35:47Z
- Build: `CGO_ENABLED=0`, sem otimizações de release.

## Metodologia

**Repositórios de fixture.** Dois projetos pequenos (`bench/fixtures/tune` e `bench/fixtures/eval`) com
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
| textual (substring) | 15.0% | 15.0% | 0.117 | 13.2% | 40.0% | 0.0% | 0.36 | 0.41 | 12 |
| bm25 (fts5, raw terms) | 20.0% | 20.0% | 0.085 | 7.0% | 35.0% | 0.0% | 0.60 | 1.35 | 70 |
| bm25 + traducao/normalizacao | 70.0% | 82.5% | 0.557 | 13.0% | 0.0% | 0.0% | 1.87 | 3.05 | 248 |
| lexical + grafo | 80.0% | 80.0% | 0.604 | 12.6% | 0.0% | 0.0% | 9.16 | 11.90 | 304 |
| lexical + grafo + git | 80.0% | 80.0% | 0.604 | 12.6% | 0.0% | 0.0% | 8.17 | 9.53 | 354 |

### Split: eval

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 10.0% | 10.0% | 0.075 | 25.0% | 80.0% | 0.0% | 0.32 | 0.43 | 6 |
| bm25 (fts5, raw terms) | 20.0% | 20.0% | 0.163 | 37.5% | 70.0% | 0.0% | 0.50 | 0.86 | 26 |
| bm25 + traducao/normalizacao | 85.0% | 85.0% | 0.572 | 23.4% | 10.0% | 0.0% | 0.92 | 1.41 | 175 |
| lexical + grafo | 75.0% | 75.0% | 0.567 | 23.1% | 10.0% | 0.0% | 4.12 | 7.45 | 185 |
| lexical + grafo + git | 75.0% | 75.0% | 0.567 | 23.1% | 10.0% | 0.0% | 4.76 | 9.27 | 214 |

### Split: holdout

| Método | Hit@5 | Recall@10 | MRR | Precisão | Sem evidência (positivos) | Falso positivo (negativos) | P50 ms | P95 ms | Tokens médios |
|---|---|---|---|---|---|---|---|---|---|
| textual (substring) | 11.8% | 11.8% | 0.088 | 25.0% | 76.5% | 0.0% | 0.21 | 0.27 | 7 |
| bm25 (fts5, raw terms) | 17.6% | 17.6% | 0.176 | 29.2% | 64.7% | 0.0% | 0.49 | 0.87 | 29 |
| bm25 + traducao/normalizacao | 70.6% | 76.5% | 0.487 | 20.8% | 17.6% | 0.0% | 0.93 | 1.46 | 165 |
| lexical + grafo | 64.7% | 64.7% | 0.490 | 20.2% | 17.6% | 0.0% | 3.97 | 7.63 | 185 |
| lexical + grafo + git | 64.7% | 64.7% | 0.490 | 20.2% | 17.6% | 0.0% | 7.36 | 15.23 | 215 |

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
```

O harness constrói os repositórios de fixture em um diretório temporário, indexa, roda os métodos e imprime as tabelas.
