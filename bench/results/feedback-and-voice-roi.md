# Voice Mode, Feedback Estruturado e Matriz Multi-Agente: Avaliação Empírica

Execução em 2026-11-07 UTC em Linux amd64 (Go 1.27.0, AMD Ryzen 7 7735HS). A suíte multi-linguagem com 10 repositórios públicos reais e 50 cenários está em [`bench/ten_repos_cases.json`](../ten_repos_cases.json) e [`bench/agentic/setup_10_repos.sh`](../agentic/setup_10_repos.sh); o relatório completo de recuperação offline está em [`bench/results/ten-repos-benchmark.md`](ten-repos-benchmark.md) e o benchmark agentic de 90 execuções reais está em [`bench/results/real-external-benchmark.md`](real-external-benchmark.md).

---

## 1. Voice Mode (Prompts Falados de 200+ Palavras) vs. Perguntas Curtas

### Diagnóstico da falha anterior (`5e31e03`) e solução determinística implementada

Na linha de base `5e31e03`, prompts de Voice Mode em português (~200 palavras com hesitações como *"Cara, bom dia, deixa eu te explicar..."*) acertavam apenas **3/10 (`30,0%` Hit@5)** com latência **P50 de `28,48 ms` (P95 `59,68 ms`)**. A análise de causa raiz revelou três gargalos determinísticos:
1. **Empate de IDF em consultas cross-lingual (PT-BR → EN)**: como as palavras em português não aparecem literalmente nos identificadores em inglês (`freq[term].Symbol == 0`), todas recebiam o mesmo IDF máximo (`idf(0, total)`). O `sort.SliceStable` truncava `concepts[:10]` pegando as **10 primeiras palavras de saudação/preenchimento** do prompt falado e descartava a pergunta técnica na palavra 90+.
2. **Ausência de destilação de cláusula interrogativa**: prompts de voz concentram a intenção real nas cláusulas interrogativas (`onde`, `qual`, `como`, `?`) e nos termos técnicos/`codeLike`.
3. **Consultas individuais por termo no SQLite (`TermFreq`)**: para prompts longos, 40+ chamadas `SELECT` sequenciais no `fts5vocab` inflavam a latência.

### O que foi implementado no código

- **Destilação focal para prompts longos (`focalTerms` + `conceptSalience` em [`internal/retrieve/retrieve.go`](../../internal/retrieve/retrieve.go))**: quando `len(terms) > 12`, extrai janelas em torno de cláusulas interrogativas e identificadores `codeLike` e calcula a saliência pelo **DF efetivo considerando as traduções/cognatos (`c.alts`)**, impedindo que muletas conversacionais no início do áudio ocupem o orçamento de 10 conceitos.
- **Batching de vocabulário SQLite (`TermFreqs` em [`internal/store/store.go`](../../internal/store/store.go))**: substitui N queries unitárias por uma única consulta em lote `WHERE term IN (...)`.
- **Paridade cross-lingual de símbolo composto (`c.nameComplete` em [`internal/retrieve/retrieve.go`](../../internal/retrieve/retrieve.go))**: quando 100% dos tokens do identificador de um símbolo multi-token são cobertos pela tradução/cognato (ex.: `"fuso horário"` → `TimeZone`), aplica o mesmo impulso RRF de nome completo que uma busca em inglês recebe.
- **Extração de docstrings Python (`docstringBelow` em [`internal/extract/extract.go`](../../internal/extract/extract.go))**: indexa docstrings PEP 257 (`"""..."""`) logo abaixo de `def`/`class`.

### Comparação Antes (`5e31e03`) vs. Depois (`main`) nos 10 Repositórios Reais

| Categoria | N | Hit@1 (Antes → Depois) | Hit@5 (Antes → Depois) | Recall@10 (Antes → Depois) | MRR (Antes → Depois) | P50 `v2` (Antes → Depois) | P50 `v2.1 router` (Depois) | P95 `v2.1 router` (Depois) |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| **Voice Mode (~200 palavras PT-BR)** | 10 | 1/10 (`10%`) → **5/10 (`50%`)** | 3/10 (`30%`) → **10/10 (`100%`)** | `30,0%` → **`100,0%`** | `0,200` → **`0,708`** | `28,48 ms` → **`18,22 ms`** | **`9,61 ms`** | **`11,84 ms`** |
| **Direto (EN + PT curto)** | 20 | 8/20 (`40%`) → **10/20 (`50%`)** | 17/20 (`85%`) → **20/20 (`100%`)** | `85,0%` → **`100,0%`** | `0,561` → **`0,678`** | `18,28 ms` → **`18,45 ms`** | **`6,47 ms`** | **`11,99 ms`** |
| **PT Imperfeito / Ruído (`messy_pt`)** | 10 | 3/10 (`30%`) → **4/10 (`40%`)** | 5/10 (`50%`) → **9/10 (`90%`)** | `50,0%` → **`100,0%`** | `0,367` → **`0,604`** | `11,05 ms` → **`11,63 ms`** | **`5,79 ms`** | **`17,80 ms`** |
| **Negativos / Abstinência (`negative`)** | 10 | — | FP: 2/10 (`20%`) → **0/10 (`0,0%`)** | — | — | `4,80 ms` → **`4,61 ms`** | **`2,71 ms`** | **`3,02 ms`** |
| **Total Consolidado (50 cenários)** | **50** | `30,0%` → **`47,5%`** | `62,5%` → **`97,5%`** | `65,0%` → **`100,0%`** | `0,425` → **`0,668`** | `16,52 ms` → **`14,40 ms`** | **`5,79 ms`** | **`13,86 ms`** |

- **Resultado em Voice Mode**: mesmo em prompts gigantes de 200+ palavras faladas, o Sidecar atingiu **10/10 (`100%` Hit@5 e `100%` Recall@10)**, superando inclusive o baseline BM25 puro (`50%` Hit@5) e busca textual substring (`20%` Hit@5), com latência mediana de **`9,61 ms`** no `v2.1 (intent router)` e **`18,22 ms`** no `v2` completo.

---

## 2. ROI e Custo do `sidecar feedback` (Reconciliação Determinística no SQLite)

### Experimento controlado multi-turno em `go-chi/chi` ([`bench/agentic/feedback_chi.py`](../agentic/feedback_chi.py))

1. **Turno 1 (sem feedback prévio)**:
   - Prompt: `"Qual middleware aplica timeout a cada requisição?"`
   - Ranking inicial: `#1 middleware/throttle_test.go#constant:timeout@292` (vizinho ruidoso de teste) e `#2 middleware/timeout.go#function:Timeout@32` (implementação real).
2. **Registro de feedback pós-tarefa (`sidecar feedback record`)**:
   - Payload JSON compacto: `273 bytes` (**~69 tokens**).
   - Marca `middleware/throttle_test.go#constant:timeout@292` em `misleading_ids` (`weight = -0.25`) e `middleware/timeout.go#function:Timeout@32` em `helpful_ids` (`weight = +0.1625`), gravando o `file_hash` SHA-256 atual de cada arquivo na tabela `evidence_feedback` (Schema v5).
   - Tempo total do processo CLI `sidecar feedback record` (incluindo spawn do binário, abertura do SQLite, reconciliação e commit): **`14,87 ms`** (overhead de leitura `FeedbackMultipliers` dentro do `retrieve.Search`: **`< 0,4 ms`**).
3. **Turno 2 (nova consulta relacionada na mesma sessão/worktree)**:
   - Prompt: `"Onde fica o middleware de timeout para requisições?"`
   - Ranking reconciliado: `#1 middleware/timeout.go#function:Timeout@32` (**Hit@1 salta de `0` → `1`**, **MRR salta de `0,500` → `1,000`**).
4. **Invalidação automática por hash**: assim que o arquivo sofre edição e seu `file_hash` muda na indexação incremental, o peso reconciliado daquele símbolo é automaticamente ignorado (validado em [`internal/retrieve/feedback_retrieve_test.go`](../../internal/retrieve/feedback_retrieve_test.go) e [`internal/store/feedback_test.go`](../../internal/store/feedback_test.go)).

### Equação de Custo vs. Retorno (ROI)

| Métrica | Valor Medido |
|---|---|
| Custo de emissão do JSON `sidecar feedback record` ao final da tarefa | **~69 tokens** (`273 bytes`) |
| Overhead de leitura na próxima consulta (`st.FeedbackMultipliers`) | **0 tokens** no prompt / **`< 0,4 ms`** no SQLite |
| Economia média por round-trip de busca/leitura evitado no turno seguinte | **1.500 a 4.200 tokens** (1 chamada `grep`/`read_file` evitada em arquivo errado) |
| **Break-even (ponto de equilíbrio)** | **1 única reutilização na sessão já gera retorno líquido de 20× a 60× em tokens** |

> **Recomendação operacional**: manter o `sidecar feedback record` **habilitado em sessões multi-turno de desenvolvimento no mesmo worktree** e no autodogfooding (`AGENTS.md`), pois custa apenas ~69 tokens no encerramento e elimina falsos vizinhos recorrentes (como mocks/testes que disputam topo com a implementação). Para execuções isoladas *one-shot* sem segundo turno, o comando é opcional.

---

## 3. O Que Foi Executado nos Benchmarks vs. Matriz Multi-Agente

Para total transparência metodológica (conforme `AGENTS.md` e [`docs/evaluation-plan.md`](../../docs/evaluation-plan.md)), separamos estritamente o que foi **executado e medido deterministicamente** do que depende de execução com modelos de linguagem ao vivo:

### 3.1 O que foi executado de forma real e determinística nesta suíte

1. **Benchmark de Recuperação Offline em 10 Repositórios Públicos Reais ([`bench/ten_repos_cases.json`](../ten_repos_cases.json) / [`bench/results/ten-repos-benchmark.md`](ten-repos-benchmark.md))**:
   - **50 cenários reais** (20 diretos EN/PT, 10 PT imperfeito, **10 Voice Mode de ~200 palavras**, 10 negativos de abstenção) executados contra checkouts reais com commit fixado de 10 projetos open-source (`spring-petclinic`, `chi`, `click`, `hono`, `serde-json`, `sinatra`, `monolog`, `kotlinx-datetime`, `dapper`, `express`).
   - Mede **diretamente o binário Go do Sidecar** sobre o índice SQLite/FTS5 real de cada projeto (por isso cada consulta leva **5,8 ms a 18,2 ms**, sem chamar LLM).
   - **Resultado medido**: **50/50 cenários aprovados (`100%`)**, **`97,5%` Hit@5**, **`100,0%` Recall@10**, **`0,0%` falsos positivos**, e **Voice Mode `10/10` (`100%` Hit@5)**.
2. **Benchmark Agêntico Instrumentado A/B em 2 Repositórios Reais ([`bench/agentic/instrumented_sim.py`](../agentic/instrumented_sim.py) / [`bench/results/real-repos-ab-score.md`](real-repos-ab-score.md))**:
   - **6 tarefas verificadas** (`R01` a `R06` em [`bench/agentic/manifest.jsonl`](../agentic/manifest.jsonl)) × 3 braços (`OFF`, `ON-v1`, `ON-v2`) × 5 repetições = **90 execuções reais de ferramentas** (`grep`, leitura de arquivos e `sidecar explain --json`) sobre `spring-petclinic` e `go-chi/chi`.
   - Usa uma política determinística fixa (`model = "instrumented-sim"`) para isolar a economia de buscas e tokens sem variância de rede/LLM:

| Braço Medido (`instrumented_sim.py`, 90 execuções) | Taxa de Resolução | Chamadas de Ferramenta até o Gabarito | Rodadas até o Gabarito | Tokens Consumidos de Ferramentas | Tempo de Ferramenta (TTFE, sem LLM) | Score Composto (0–100) |
|---|---:|---:|---:|---:|---:|---:|
| **OFF (Sem Sidecar — `grep` + leitura)** | `83,3%` | `18,0` a `19,3` calls | `2,8` a `3,2` rounds | `17.772` a `21.062` tok | `38,7 ms` a `96,1 ms` | **81,5 a 85,5 / 100** |
| **Sidecar V1 (`ON-v1`)** | `83,3%` | **`11,0` a `13,5` calls (`-35%` a `-43%`)** | **`2,2` a `2,4` rounds** | **`11.523` a `11.816` tok (`-35%` a `-44%`)** | `55,0 ms` a `144,4 ms` | **86,5 a 87,3 / 100** |
| **Sidecar V2 (`ON-v2`)** | `83,3%` | **`11,0` a `13,5` calls (`-35%` a `-43%`)** | **`2,2` a `2,4` rounds** | **`11.517` a `11.772` tok (`-35%` a `-44%`)** | `55,0 ms` a `142,2 ms` | **86,5 a 87,3 / 100** |

> **Nota sobre tarefas completas com agentes reais (Claude Code Haiku, Codex, Antigravity CLI)**: na simulação instrumentada acima, o tempo medido é apenas o tempo local de execução de `grep`/`read`/`sidecar explain` (dezenas de milissegundos), sem o tempo de inferência do modelo em cada rodada (~2 a 5 segundos por round-trip de LLM). Nas tarefas em que o bloco do Sidecar entrega o gabarito logo no turno 0 (`R01`, `R03`, `R04`), o número de chamadas cai de **16–20 chamadas (`~19.000 tokens`) para 1 única chamada (`~330–480 tokens`, `-97%` de tokens)**. Para medir o tempo de parede real (em segundos) com agentes caixa-preta editando código e rodando testes, o manifesto [`bench/agentic/manifest.jsonl`](../agentic/manifest.jsonl) deve ser executado invocando os CLIs reais (`claude`, `codex`, `agy`) em pares ON/OFF.

---

## 4. O Que Aprendemos e Incorporamos das Implementações de Referência ([`docs/reference-implementations.md`](../../docs/reference-implementations.md))

| Referência Analisada | Ideia Central Aproveitada (Sem copiar código GPL/BUSL) | Status no Sidecar V2 | Impacto Medido |
|---|---|---|---|
| **Headroom** (`chopratejas/headroom`, Apache-2.0) | Destilação reversível de contexto e preservação de âncoras interrogativas/identificadores em prompts longos | **Implementado** em `focalTerms` + `conceptSalience` ([`internal/retrieve/retrieve.go`](../../internal/retrieve/retrieve.go)) | Voice Mode saltou de **`30%` → `100%` Hit@5** e latência P50 caiu de `28,48 ms` para `9,61 ms` (`v2.1`) |
| **Serena SolidLSP** (`oraios/serena`, MIT na camada simbólica) | Extração estrutural de símbolos + docstrings no corpo (`docstringBelow`) + enriquecimento LSP sob demanda (`internal/lsp`) | **Implementado** em [`internal/extract/extract.go`](../../internal/extract/extract.go) e [`internal/lsp/`](../../internal/lsp/) | Resolveu falha de docstrings PEP 257 em Python (`click` `R03-2`: `0%` → `100%` Hit@5) |
| **Repomix** (`yamadashy/repomix`, MIT) | Empacotamento sob orçamento estrito de tokens com deduplicação e supressão de ruído (`internal/ctxbuild`) | **Implementado** no Evidence Packer V2 ([`internal/ctxbuild/packer.go`](../../internal/ctxbuild/packer.go)) | Mantém injeção média em **~380–425 tokens** com `0%` de estouro de budget |
| **CodeGraph** (`colbymchenry/codegraph`, MIT) | Expansão por arestas de chamada/importação/teste e completude de identificadores compostos (`c.nameComplete`) | **Implementado** em [`internal/retrieve/retrieve.go`](../../internal/retrieve/retrieve.go) | Paridade cross-lingual PT-BR ↔ EN em símbolos compostos (`TimeZone`, `SqlMapper`: **100% Hit@5**) |
| **Claude-Mem** (`thedotmack/claude-mem`, AGPL — estudado apenas como arquitetura conceitual, implementação 100% independente) | Memória local com proveniência por hash de arquivo (`file_hash`) e reconciliação de pesos (`evidence_feedback`) | **Implementado** em [`internal/store/feedback.go`](../../internal/store/feedback.go) | **Hit@1 `0` → `1` (MRR `0,5` → `1,0`)** em consultas subsequentes com invalidação automática por SHA-256 |
| **RTK-OSS** (`rtk-ai/rtk`, MIT/Apache-2.0) | Telemetria local de economia de comandos e observação passiva de ferramentas (`PostToolUse`) | **Implementado** em [`internal/observe/`](../../internal/observe/) | Observação passiva fail-open em `< 2 ms` sem interceptar execução |
