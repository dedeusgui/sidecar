# Benchmark real em spring-petclinic e go-chi/chi: OFF × Sidecar V1 × Sidecar V2

**Leia antes dos números.** Três limites definem o que este relatório pode afirmar:

1. **Nenhum agente de linguagem foi executado.** As 6 tarefas agênticas usam uma **simulação instrumentada** (`bench/agentic/instrumented_sim.py`): uma política fixa de grep e leitura, igual nos três braços. Só a fonte de candidatos muda. Tempo, chamadas e tokens vêm de execuções reais de `grep`, leitura de arquivo e `sidecar explain`. A simulação mede o valor do contexto entregue sob uma política fixa, não a produtividade de um agente real.
2. **O tempo medido exclui a latência do modelo.** Cada rodada real de um agente custa uma ida e volta ao modelo, que não está medida aqui. Ver a seção 6.
3. **O corpus é pequeno.** São 20 casos de recuperação (18 positivos e 2 negativos) e 6 tarefas, com 5 repetições por braço. Diferenças menores que cerca de 0,03 de MRR não são evidência (ver `docs/benchmark.md`, "Variância").

---

## 1. Resumo

- **Recuperação em código real (18 positivos):** o pipeline V1 de produção acerta o arquivo ou símbolo certo no top 5 em **61,1%** dos casos. O V2 acerta em **50,0%**. O baseline `bm25 + tradução` acerta em **72,2%**. Neste corpus, o V2 não superou o V1, e o V1 não superou o baseline léxico em Hit@5 nem em Recall@10.
- **Tarefas (6, simulação):** nas tarefas em que o bloco do Sidecar traz o gabarito, o agente chega à evidência com **1 chamada** contra 16 a 20 do braço OFF. Nas seis tarefas, o uso de tokens cai **44%** (21,1k → 11,8k) e as chamadas até o gabarito caem **43%** (19,3 → 11,0). A taxa de resolução fica igual nos três braços (83,3%).
- **Tempo:** o TTFE medido é **maior** com o Sidecar (55 ms contra 39 ms), porque `sidecar explain` custa cerca de 40 ms por chamada, e um `grep` local leva de 1 a 3 ms. Pela medição, o OFF é mais rápido em tempo de ferramenta. Com latência de modelo, a conta pode inverter (seção 6).
- **Nota final (0 a 100):** OFF **81,5**, Sidecar V1 **87,3**, Sidecar V2 **87,3**. A diferença vem quase toda de custo (tokens e chamadas), parcialmente compensada pela velocidade. V1 e V2 empatam.
- **Falhas que o relatório não esconde:** o caso negativo `PC10` passou a injetar contexto depois da inclusão de termos PT-BR no dicionário. A tarefa `R02` falhou em todos os braços. A causa provável é a falta de tradução para `validador` no dicionário. A hipótese não foi testada e não foi corrigida nesta rodada.

---

## 2. O que cada métrica significa

### Recuperação (corpus externo, `--external-cases`)

| Métrica | Significado | Como ler |
|---|---|---|
| **Hit@1** | O primeiro resultado é o item do gabarito. | Quanto maior, melhor. |
| **Hit@5** | Algum dos 5 primeiros resultados é o item do gabarito. | Quanto maior, melhor. |
| **Recall@10** | Fração dos itens do gabarito encontrados no top 10. | Quanto maior, melhor. |
| **MRR** | Média do inverso da posição do primeiro acerto. | Quanto maior, melhor. Oscila cerca de ±0,03 entre execuções. |
| **Falso positivo (negativos)** | Em casos sem resposta, o método injetou contexto mesmo assim. | Quanto menor, melhor. Um negativo que injeta contexto é erro. |

### Agêntico (`--agentic-report` e simulação)

| Métrica | Significado | Como ler |
|---|---|---|
| **Resolução** | Fração das execuções em que todo o gabarito foi identificado. | Quanto maior, melhor. |
| **TTFE** (tempo até a evidência correta) | Tempo de ferramenta até o gabarito completo estar identificado. Se nunca chegou, usa-se o tempo de parede (cota inferior). | Quanto menor, melhor. Exclui latência do modelo. |
| **Chamadas até o gabarito** | Número de buscas (`grep`, `explain`) e leituras de arquivo até identificar o gabarito. | Quanto menor, menos trabalho. Um proxy de custo. |
| **Rodadas até o gabarito** | Quantas vezes o agente precisou "pensar" e emitir ferramentas, contando buscas em lote como uma rodada. | Proxy das idas e voltas ao modelo, que são o custo caro. |
| **Tokens da tarefa** | Tokens estimados de toda a saída de ferramentas consumida até o fim. Regra `(caracteres + 3) / 4`, igual a `ctxbuild.EstimateTokens`. | Não inclui o prompt do sistema nem a resposta do modelo. Compare braços, não valores absolutos. |
| **Fidelidade do contrato** | Fração dos símbolos citados pelo bloco que aparecem nas linhas citadas do arquivo. | 1,0 significa que nenhuma citação do Sidecar estava errada. Só existe para braços com Sidecar. |

### Nota composta (0 a 100)

A spec define `Score = 0,45 × Qualidade + 0,30 × Velocidade + 0,25 × Economia`. Como a spec não fixa a escala de Velocidade e Economia, usei estas definições, aplicadas **por tarefa** e comparando os três braços:

- **Qualidade (0 a 100):** percentual dos itens do gabarito (arquivo e símbolo) identificados, média das 5 repetições.
- **Velocidade (0 a 100):** `100 × melhor TTFE da tarefa / TTFE do braço`. O braço mais rápido da tarefa vale 100.
- **Economia (0 a 100):** `100 × [0,5 × menor tokens / tokens do braço + 0,5 × menor chamadas / chamadas do braço]`. O braço mais barato da tarefa vale 100 em cada metade.
- **Nota final:** média das 6 tarefas.

Consequência: a nota é **relativa aos braços desta execução**. Não mede produtividade absoluta. O braço que é mais rápido em tempo de ferramenta ganha pontos de velocidade, e isso favorece o OFF (seção 6).

---

## 3. Como foi medido

### 3.1 Repositórios

| Repositório | Commit | Licença | Arquivos indexados | Tamanho informado na spec |
|---|---|---|---|---|
| `spring-petclinic` | `500158f732419217507c7656904b8e6aa1bcc0d6` | Apache-2.0 | 132 | 4.822 linhas (Java, SQL, Markdown) |
| `go-chi/chi` | `9f8adb1aabaa8a91180a821111d3f8e668d1c0bf` | MIT | 110 | 14.316 linhas (Go, Markdown) |

Os dois checkouts estavam limpos e no commit fixado. O avaliador recusa checkout sujo ou fora do pin.

### 3.2 Casos de recuperação (`bench/results/external-real-repos.md`)

Os 20 casos estão em `/tmp/sidecar-real-bench/external-manifest.json`. Cada gabarito foi conferido linha a linha no código do commit fixado (arquivo, símbolo ou tabela).

- **spring-petclinic (PC01 a PC10):** rota, controller, validação, SQL (`visits` e `vet_specialties`), teste e um negativo. Prompts em PT-BR e EN.
- **go-chi/chi (CH01 a CH10):** rota, roteador (`Mount`), rate limit e timeout, autenticação, cabeçalho, compressão, limite de corpo, teste e um negativo. Prompts em PT-BR e EN.
- **Negativos:** `PC10` (Kafka, que não existe no projeto) e `CH10` (gRPC, que não existe no projeto). Os termos foram verificados com `grep` antes de serem usados.

### 3.3 Tarefas agênticas (`bench/agentic/manifest.jsonl`)

Seis tarefas `verified`, três por repositório. Cada uma tem commit fixado, gabarito de arquivo e símbolo, e um oráculo somente leitura (`git grep -w`) que confirma o gabarito no commit. Os seis oráculos foram executados e saíram com código 0 antes da coleta.

| Tarefa | Repositório | Prompt (resumido) | Gabarito |
|---|---|---|---|
| R01 | spring-petclinic | Método que trata o POST de visita e teste do caminho feliz | `VisitController.processNewVisitForm`, `VisitControllerTests.processNewVisitFormSuccess` |
| R02 | spring-petclinic | Validador que rejeita pet sem nome (PT-BR) e teste | `PetValidator.validate`, `PetValidatorTests.validateWithInvalidPetName` |
| R03 | spring-petclinic | Tabela de vínculo vet-especialidade e classe que mapeia `specialties` | `vet_specialties` (h2), `Specialty` |
| R04 | go-chi/chi | Onde `Mux.Mount` registra sub-roteador e teste de montagem existente | `Mount`, `TestMountingExistingPath` |
| R05 | go-chi/chi | Middleware de concorrência com fila e timeout (PT-BR) e teste | `ThrottleBacklog`, `ThrottleWithOpts`, `TestThrottleClientTimeout` |
| R06 | go-chi/chi | Função que escolhe o encoder pelo `Accept-Encoding` (PT-BR) e teste | `selectEncoder`, `TestCompressor` |

### 3.4 Braços e o que é real

| Braço | O que faz | Real ou simulado |
|---|---|---|
| **OFF** | Sem Sidecar: grep por termos do prompt (e suas traduções PT→EN do dicionário), ordenação por idf, leitura de até 12 arquivos em lotes de 3. | Os comandos e as leituras são reais. A política é simulada. |
| **ON-v1** | `sidecar explain` com `SIDECAR_ENGINE=v1` primeiro. Se o bloco trouxer o gabarito, não há mais buscas. Se não, segue a política do OFF. | `explain` real. Política simulada. |
| **ON-v2** | Igual ao ON-v1, com `SIDECAR_ENGINE=v2`. | `explain` real. Política simulada. |

Outros detalhes:

- **Índice isolado:** `SIDECAR_HOME` apontou para um diretório novo, fora do cache do usuário. O índice frio custou 267 ms (petclinic) e 669 ms (chi). Não entrou nas medições.
- **Repetições:** 5 por braço e tarefa (90 execuções). A ordem dos braços rotaciona entre as repetições (balanceamento). A política é determinística, então as repetições variam **só no tempo**. Os desvios-padrão de TTFE ficaram em 0 a 3 ms, exceto R01-V2 (±9 ms) e R04-V2 (±19 ms), por chamadas lentas isoladas de `explain`.
- **Não executado:** `claude -p` com Haiku ou outro modelo. Exige autorização de custo e tempo (ver `docs/benchmark.md`).

### 3.5 Mudanças de código nesta entrega

- **`internal/lexical/dictionary.go`:** dez termos PT-BR da spec (`veterinario`, `dono`, `proprietario`, `visita`, `animal`, `mascote`, `roteador`, `autenticacao`, `cabecalho`, `compressao`). Teste em `internal/lexical/lexical_test.go` (`TestRealCorpusTermsTranslate`).
- **Benchmark oficial (`bench/cases.json`):** comparado antes e depois com `--compare`. **Zero regressões e zero melhorias** em todos os splits. A mudança não afeta o benchmark anterior.
- **`cmd/sidecar-eval/main.go`:** `--agentic-report` ignorava `--report-md` e escrevia só no stdout. Agora grava no arquivo quando o flag é passado. O formato não mudou.
- **`internal/benchagent/benchagent_test.go`:** dois testes afirmavam que o manifesto distribuído não tinha tarefa `verified`, o que era o estado antes desta coleta. Agora afirmam o invariante real: `verified` exige commit fixado e oráculo independente, e rascunhos não recebem veredito.
- **Nome do arquivo de registros:** o avaliador lê `runs.jsonl`, não `real_runs.jsonl`. Usei `runs.jsonl` para o comando documentado funcionar sem mudar o contrato. `real_runs.jsonl` existe com os mesmos registros mais qualidade e estado dos itens, e `real_trace.jsonl` guarda cada chamada.

---

## 4. Recuperação em repositórios reais

### 4.1 Resultado geral (split `real`, 18 positivos e 2 negativos)

| Método | Hit@1 | Hit@5 | Recall@10 | MRR | Falso positivo (negativos) |
|---|---|---|---|---|---|
| textual (substring) | 16,7% | 44,4% | 50,0% | 0,290 | 50% |
| bm25 (FTS5, termos crus) | 33,3% | 61,1% | 61,1% | 0,423 | 50% |
| **bm25 + tradução/normalização** | **27,8%** | **72,2%** | **72,2%** | 0,415 | 100% |
| lexical + grafo | 33,3% | 61,1% | 61,1% | 0,423 | 100% |
| **Sidecar V1** (lexical + grafo + git) | 33,3% | 61,1% | 61,1% | 0,423 | 100% |
| v2.1 (router de intenção) | 33,3% | 61,1% | 61,1% | 0,428 | 100% |
| **Sidecar V2** (intent + contract + packer) | 27,8% | 50,0% | 50,0% | 0,354 | 100% |
| V2 sem intent | 27,8% | 50,0% | 50,0% | 0,354 | 100% |
| V2 sem contract | 33,3% | 61,1% | 61,1% | 0,428 | 100% |
| V2 sem packer | 33,3% | 61,1% | 61,1% | 0,428 | 100% |

Antes da inclusão de termos PT-BR, o V1 estava em Hit@1 38,9%, Hit@5 50,0%, MRR 0,435 e falso positivo 50%. O V2 estava em Hit@5 44,4%.

### 4.2 Por repositório (9 positivos cada)

| Método | petclinic Hit@1 | petclinic Hit@5 | chi Hit@1 | chi Hit@5 |
|---|---|---|---|---|
| V1 | 33,3% | 55,6% | 33,3% | 66,7% |
| V2 | 33,3% | 55,6% | 22,2% | 44,4% |
| bm25 + tradução | 33,3% | 66,7% | 22,2% | 77,8% |

### 4.3 Caso a caso

| Caso | Categoria | V1 Hit@5 | V2 Hit@5 | bm25 + trad. Hit@5 | Observação |
|---|---|---|---|---|---|
| PC01 | rota | ✓ | ✓ | ✓ | |
| PC02 | controller | ✗ | ✗ | ✓ | O grafo tira o alvo do top 10 |
| PC03 | controller | ✗ | ✗ | ✗ | "visita de um animal" ainda falha após o dicionário (causa não investigada) |
| PC04 | validação | ✗ | ✗ | ✗ | "pet com nome em branco" (causa não investigada) |
| PC05 | sql | ✓ | ✓ | ✓ | |
| PC06 | sql | ✓ | ✓ | ✓ | "veterinários" só resolve com o termo novo do dicionário |
| PC07 | teste | ✓ | ✓ | ✓ | |
| PC08 | teste | ✗ | ✗ | ✗ | "cadastro com nome duplicado" (causa não investigada) |
| PC09 | controller | ✓ | ✓ | ✓ | Rank 3 no V1 |
| **PC10** | **negativo** | **falso positivo** | **falso positivo** | **falso positivo** | "eventos de **visita**" casa com `Visit`; o Kafka, ausente, não casa com nada. **Passou a falhar após o dicionário** |
| CH01 | rota | ✓ | ✗ | ✓ | |
| CH02 | roteador | ✗ | ✗ | ✗ | `Mount` fora do top 10 |
| CH03 | rate | ✓ | ✓ | ✓ | |
| CH04 | timeout | ✓ | ✓ | ✓ | Rank 2 no V1 |
| CH05 | autenticação | ✓ | ✗ | ✓ | Passou a acertar após o dicionário. O V2 perde |
| CH06 | cabeçalho | ✗ | ✗ | ✗ | `SetHeader` fora do top 10 |
| CH07 | compressão | ✗ | ✗ | ✓ | O grafo tira `selectEncoder` do top 10 |
| CH08 | validação | ✓ | ✓ | ✓ | |
| CH09 | teste | ✓ | ✓ | ✓ | Rank 3 no V1 |
| **CH10** | **negativo** | **falso positivo** | **falso positivo** | **falso positivo** | "gRPC" é ausente; o termo genérico `server` traz `FileServer` e `TestServerBaseContext` |

### 4.4 O que os números dizem

- **A expansão por grafo e estrutura da V1 custa recall neste corpus.** `bm25 + tradução` acerta PC02 e CH07; `lexical + grafo` não. A diferença entre essas duas ablações é justamente a expansão por grafo e estrutura, que tira esses dois alvos do top 10.
- **O V2 perde CH01 e CH05 em relação ao V1.** Esses são os dois casos em que a V2 difere de forma clara. Não investiguei a causa nesta rodada.
- **A abstenção piorou.** O negativo `PC10` passou a injetar contexto depois do termo `visita → visit`. A causa é cobertura parcial: o termo presente casa, e o conceito ausente (Kafka) não casa com nada. Isto é um problema de regra de abstenção, não do dicionário. Corrigir exige uma regra de cobertura (ex.: exigir os termos discriminativos do prompt), que é um novo ranking. Precisa de A/B próprio e não foi feito.
- **Sobre o dicionário:** a mudança fez o V1 subir de 50% para 61% em Hit@5 e Recall@10 (dois casos a mais), e fez o negativo `PC10` falhar. A queda de Hit@1 (38,9% → 33,3%, um caso) e de MRR (0,435 → 0,423) estão dentro da variância documentada. Não consigo separar esses ganhos e perdas de ruído com uma execução.

---

## 5. Tabela comparativa: OFF × Sidecar V1 × Sidecar V2

### 5.1 Resultado geral (6 tarefas, 5 repetições)

| Métrica | OFF (sem Sidecar) | Sidecar V1 | Sidecar V2 |
|---|---|---|---|
| **Qualidade (0 a 100)** | 91,7 | 91,7 | 91,7 |
| Resolução (gabarito completo) | 83,3% | 83,3% | 83,3% |
| **Tempo (TTFE, ms de ferramenta)** | **38,7** | **55,0** | **55,0** |
| **Buscas e leituras até o gabarito** | 19,3 | **11,0** | **11,0** |
| Rodadas até o gabarito | 3,2 | 2,2 | 2,2 |
| **Tokens da tarefa (estimados)** | 21.062 | **11.816** | **11.772** |
| Fidelidade do contrato | — | 1,0 | 1,0 |
| Velocidade (0 a 100) | **93,9** | 72,3 | 71,4 |
| Economia de custo e buscas (0 a 100) | 48,3 | **97,4** | **98,4** |
| **Nota final (0 a 100)** | **81,5** | **87,3** | **87,3** |

### 5.2 Por repositório

| Métrica | OFF | V1 | V2 |
|---|---|---|---|
| **spring-petclinic** (4.822 linhas): nota | 72,8 | 86,1 | **87,0** |
| spring-petclinic: qualidade | 83,3 | 83,3 | 83,3 |
| spring-petclinic: tokens / chamadas | 18.308 / 20,7 | 8.643 / 9,0 | 8.584 / 9,0 |
| spring-petclinic: TTFE (ms) | 42,7 | 49,6 | 49,7 |
| **go-chi/chi** (14.316 linhas): nota | **90,2** | 88,5 | 87,5 |
| go-chi/chi: qualidade | 100,0 | 100,0 | 100,0 |
| go-chi/chi: tokens / chamadas | 23.816 / 18,0 | 14.989 / 13,0 | 14.960 / 13,0 |
| go-chi/chi: TTFE (ms) | 34,6 | 60,3 | 60,2 |

A qualidade do petclinic cai para 83,3 porque `R02` falhou em todos os braços (1 de 2 itens).

### 5.3 Nota por tarefa

| Tarefa | Repo | Braço | Q | TTFE (ms) | Chamadas | Tokens | V | E | Nota |
|---|---|---|---|---|---|---|---|---|---|
| R01 | petclinic | OFF | 100 | 45 | 18 | 19.087 | 84 | 4 | 71,3 |
| R01 | petclinic | V1 | 100 | 38 | 1 | 536 | 100 | 88 | **97,1** |
| R01 | petclinic | V2 | 100 | 39 | 1 | 410 | 98 | 100 | **99,5** |
| R02 | petclinic | OFF | 50 | 41 | 24 | 24.518 | 100 | 100 | **77,5** |
| R02 | petclinic | V1 | 50 | 75 | 25 | 25.088 | 54 | 97 | 62,9 |
| R02 | petclinic | V2 | 50 | 77 | 25 | 25.012 | 53 | 97 | 62,6 |
| R03 | petclinic | OFF | 100 | 42 | 20 | 11.319 | 79 | 4 | 69,7 |
| R03 | petclinic | V1 | 100 | 35 | 1 | 305 | 95 | 100 | **98,4** |
| R03 | petclinic | V2 | 100 | 34 | 1 | 331 | 100 | 96 | **99,0** |
| R04 | chi | OFF | 100 | 36 | 16 | 22.923 | 100 | 4 | 76,0 |
| R04 | chi | V1 | 100 | 40 | 1 | 456 | 88 | 100 | **96,5** |
| R04 | chi | V2 | 100 | 48 | 1 | 484 | 75 | 97 | 91,6 |
| R05 | chi | OFF | 100 | 35 | 23 | 42.013 | 100 | 98 | **99,5** |
| R05 | chi | V1 | 100 | 70 | 23 | 40.555 | 50 | 100 | 85,1 |
| R05 | chi | V2 | 100 | 66 | 23 | 40.500 | 54 | 100 | 86,1 |
| R06 | chi | OFF | 100 | 33 | 15 | 6.511 | 100 | 80 | 95,0 |
| R06 | chi | V1 | 100 | 71 | 15 | 3.957 | 47 | 99 | 83,8 |
| R06 | chi | V2 | 100 | 67 | 15 | 3.895 | 49 | 100 | 84,7 |

Leitura da tabela:

- **R01, R03 e R04:** o bloco do Sidecar trouxe o gabarito completo em 1 chamada. O Sidecar vence em custo (tokens e chamadas). Em tempo de ferramenta, perde por causa do `explain`.
- **R05 e R06:** o bloco trouxe só parte do gabarito. O Sidecar pagou o `explain` e depois seguiu a política do OFF. Perde em tempo e ganha em tokens (R06: 6.511 → 3.957).
- **R02:** falha em todos os braços. Hipótese não testada: falta a tradução de `validador` no dicionário (ver seção 7). O OFF leu o arquivo de teste e identificou 1 de 2 itens.
- **Fallback:** em 30 das 60 execuções com Sidecar, o bloco não trouxe o gabarito completo e o agente precisou buscar.

---

## 6. Interpretação

### 6.1 Onde o Sidecar ajuda

Quando o bloco traz o gabarito, o ganho é grande e consistente: 1 chamada contra 16 a 20, cerca de 97% menos tokens, e a mesma taxa de resolução. Isso aconteceu em 3 de 6 tarefas (R01, R03, R04), com fidelidade 1,0 nas citações.

### 6.2 Onde não ajuda

- **Tempo de ferramenta:** `sidecar explain` leva cerca de 40 ms por chamada, dos quais cerca de 28 ms são sincronização do índice (`sync.ms`, mesmo com índice aquecido e sem arquivos novos). Um `grep` local leva de 1 a 3 ms. Pela medição, o OFF é mais rápido.
  - **Correção posterior (medição revisada):** o `sync.ms` antigo também contava a recuperação (~13 ms) e a fingerprint (~6 ms), e não só a sincronização. Medida isolada no chi aquecido, a sincronização leva ~5 ms, ou ~3 ms desde que um subprocesso `git` redundante foi removido (mediana de 40 execuções). Com o código atual, `sidecar explain` no chi leva ~32 a 37 ms de ponta a ponta (5 execuções), contra 1 a 3 ms do `grep`. A conclusão sobre o tempo de ferramenta continua valendo, mas o custo atribuído à sincronização estava superestimado.
- **Quando o bloco falha:** o Sidecar paga o `explain` e ainda faz a busca do OFF. Nas tarefas R05 e R06, a nota do Sidecar é menor que a do OFF por isso.
- **Bloco parcial (R05 e R06):** o bloco não trouxe o gabarito completo. O OFF precisou de 15 a 23 chamadas, e o `explain` foi custo extra pago antes da mesma sequência de buscas.

### 6.3 V2 contra V1

Não há diferença que se sustente. Em tokens, V2 é 0,4% menor (11.772 contra 11.816). Em nota, empata (87,26 contra 87,29). Em recuperação real, **V2 é pior que V1** (Hit@5 50,0% contra 61,1%). O resultado que o relatório pode afirmar é: **V2 não melhora V1 neste corpus**.

### 6.4 Por que o tempo pode inverter em um agente real (inferência, não medida)

Pela medição, a diferença de tempo de ferramenta entre ON-v1 e OFF é de cerca de 16 ms a mais no Sidecar. A diferença de rodadas é de 1,0 a menos no Sidecar (3,2 contra 2,2). Em um agente real, o tempo total é aproximadamente `rodadas × latência do modelo + tempo de ferramenta`. Por isso o Sidecar fica mais rápido em tempo total se a latência de cada rodada for maior que cerca de 16 ms. Uma ida e volta ao modelo custa bem mais que isso. **Esta conclusão depende de medir um agente real**, que não foi feito.

### 6.5 Limites do baseline OFF

A política do OFF é uma heurística simples de grep (idf e bônus para o nome do arquivo), sem julgamento semântico. Um agente real pode ser melhor ou pior. Se for melhor, a vantagem do Sidecar em chamadas e tokens cai. Se for pior, cresce. O relatório mede o contexto sob uma política fixa, não o agente.

---

## 7. Pendências

1. **Agente real:** rodar `claude -p` (Haiku ou outro) nas 6 tarefas, com e sem o bloco, com autorização de custo e tempo. Só então haverá medição de latência de modelo e de tokens reais.
2. **Dicionário:** provável falta de `validador → validator` para `R02` (hipótese não testada). Não adicionei agora para não ajustar o conjunto de tarefas depois de vê-lo falhar. Entra em uma próxima rodada, com A/B próprio.
3. **Regra de abstenção:** `PC10` mostra que termos parciais geram contexto falso. Precisa de uma regra de cobertura e de um A/B com ablação.
4. **Grafo da V1:** a ablação `lexical + grafo` perde PC02 e CH07 contra `bm25 + tradução`. Vale testar a camada de grafo com ablação antes de mantê-la no caminho padrão.
5. **Custo de `explain`:** cerca de 28 ms por chamada em sincronização do índice. Isto afeta o hook também. Vale investigar antes de medir latência do hook.
6. **Causas não investigadas:** PC03, PC04, PC08 e CH06 falham nos três métodos. A causa não foi verificada nesta rodada.
7. **Relatório externo:** os splits `tune`, `eval` e `holdout` aparecem vazios porque o manifesto só tem `real`. A tabela mostra zeros para eles.
8. **Variância de empates:** o `textual` mudou de Hit@1 22,2% para 16,7% entre execuções do mesmo código (já documentado em `docs/benchmark.md`). Isso limita comparações finas.

---

## 8. Reprodução

```sh
# Recuperação externa (manifesto em /tmp/sidecar-real-bench, repositórios no commit fixado)
go run ./cmd/sidecar-eval --external-cases /tmp/sidecar-real-bench/external-manifest.json \
  --out /tmp/sidecar-real-bench/external-results.json --report-md bench/results/external-real-repos.md

# Benchmark oficial (regressão)
go run ./cmd/sidecar-eval --cases bench/cases.json --out <fora-do-repo>/cases.json --compare <linha-de-base.json>

# Simulação instrumentada (binário construído a partir da árvore atual; diretório de índice novo)
go build -o <fora-do-repo>/sidecar ./cmd/sidecar
python3 -I bench/agentic/instrumented_sim.py --sidecar <fora-do-repo>/sidecar --home <fora-do-repo>/home \
  --dict internal/lexical/dictionary.go --manifest bench/agentic/manifest.jsonl \
  --repo spring-petclinic=/tmp/sidecar-real-bench/spring-petclinic --repo chi=/tmp/sidecar-real-bench/chi \
  --reps 5 --out-dir bench/agentic --scores-out bench/results/real-repos-scores.json

# Relatório agêntico oficial
go run ./cmd/sidecar-eval --agentic-report bench/agentic --agentic-manifest bench/agentic/manifest.jsonl \
  --report-md bench/results/agentic-ab-report.md
```

---

## 9. Arquivos desta entrega

| Arquivo | Conteúdo |
|---|---|
| `bench/results/real-repos-ab-score.md` | Este relatório |
| `bench/results/external-real-repos.md` | Relatório de recuperação dos 20 casos reais (com comparação ao dicionário anterior) |
| `bench/results/agentic-ab-report.md` | Relatório agêntico oficial (`--agentic-report`) |
| `bench/results/real-repos-scores.json` | Notas por tarefa e braço, com pesos e normalização |
| `bench/agentic/runs.jsonl` | 90 registros lidos pelo avaliador |
| `bench/agentic/real_runs.jsonl` | Os mesmos 90 registros, com qualidade, chamadas totais e itens identificados |
| `bench/agentic/real_trace.jsonl` | Rastro de cada chamada (alvo, tokens, se trouxe arquivo novo) |
| `bench/agentic/instrumented_sim.py` | Simulador usado para coletar os registros |
| `bench/agentic/manifest.jsonl` | 6 tarefas `verified` (R01 a R06) e 4 `draft` |
| `/tmp/sidecar-real-bench/external-manifest.json` | Manifesto dos 20 casos (fora do repositório, como pede a spec) |
| `/tmp/sidecar-real-bench/external-results.json` | Resultados brutos da recuperação externa |
