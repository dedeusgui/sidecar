# V3.7: achados de extração de símbolos

O que o extrator (`internal/extract`) grava no índice de forma errada ou deixa de gravar, medido nos 15 repositórios pinados, e o que isso causa adiante. O índice é compartilhado, então os achados valem para V1, V3 e V3.7. Complementa os [achados de qualidade](quality-findings.md) e os [achados de índice](index-findings.md). Nada foi corrigido.

## Como foi levantado

- Índice completo (`sidecar index --root <abs> --history`) de cada checkout pinado, com `SIDECAR_HOME` isolado, e consultas SQL diretas nas tabelas `files`, `symbols` e `refs`.
- Contagens de declarações no código-fonte com `grep`, para comparar com o que o índice tem.
- `sidecar explain --json` com `--engine v1` e `--engine v3.7` para ver o efeito no contexto entregue. Os prompts são um caso do ten-repos (R04-1, split `development`) e três prompts próprios no `hono`. Nenhum caso da coorte E1 foi aberto.
- Cada item diz o que foi **medido**, o que é **lido no código** e o que **não foi medido**.

## Resumo

| # | Achado | Evidência | Onde |
|---|---|---|---|
| 1 | Em JS/TS todo `const` vira símbolo `constant`: funções-flecha e variáveis locais | `hono`: 1.709 `constant` contra 64 `function` no código; `res` indexado 1.219 vezes | `grammar.constant` |
| 2 | Por ser `constant`, a implementação vai para a seção de contratos, é penalizada e perde para variáveis locais | `hono`, 3 prompts próprios: 24 a 36 de 40 candidatos são `constant` | `sectionOf`, `packState.value`, `tierRank` |
| 3 | O docstring guarda as 8 últimas linhas do comentário e descarta o começo | resumo perdido em 13 a 36% dos símbolos documentados de 8 repositórios | `lineIndex.docAbove` |
| 4 | Em Rust, atributos (`#[inline]`, `#[derive]`) entram como docstring | `serde-json`: 433 de 666 docstrings; `axum`: 823 de 1.360 | `commentPrefixes` |
| 5 | Em Rust, métodos de `impl` não têm dono e `macro_rules!` não é indexado | `serde-json`: 92 de 1.094 funções com pai; macro `json!` ausente | consultas de tags |
| 6 | `type` do TypeScript não é indexado | `hono`: 359 declarações no código, 3 símbolos `type` | consulta de tags |
| 7 | A linha "no test file in the index references X" é emitida sem a busca de testes ter rodado | R04-1: o índice tem 16 referências; a V1 encontra o teste | `intentFamilies`, `brief/task.go` |
| 8 | `.mts` e `.cts` não são reconhecidos como TypeScript | `hono`: 27 arquivos | `langByExt` |

## 1. Todo `const` de JS/TS vira `constant`

**Lido no código.** `grammar.constant` aceita qualquer `variable_declarator` cujo pai é um `lexical_declaration` iniciado por `const`. O comentário diz "top-level const", mas não há teste de nível: declarações dentro de funções e de blocos de teste entram. O valor também não é examinado, então `const cors = (options) => { ... }` é gravado como `constant`. `assignedFunction`, que grava `function`, só trata `variable_declarator` quando o valor é uma cadeia de atribuição (`const x = exports.y = function ...`).

**Medido** (símbolos por tipo, só arquivos de código):

| Repositório | `constant` | `function` | `method` | `constant` cujo valor é função |
|---|---:|---:|---:|---:|
| hono (TypeScript) | 1.709 | 64 | 186 | pelo menos 421 |
| koa (JavaScript) | 126 | 11 | 100 | 7 |
| express (JavaScript, estilo `function`) | 4 | 111 | 4 | 0 |

A última coluna é um piso: conta assinaturas com `=>`, `= function` ou `= async`, e a assinatura guardada é truncada.

Contando também os arquivos de teste, o `hono` tem 8.782 símbolos `constant` em JS/TS. Os nomes mais repetidos são variáveis locais: `res` 1.219 vezes, `app` 782, `req` 334, `App` 205, `template` 190, `result` 153. Pelo menos 1.444 estão dentro de outra definição. No `koa`: `ctx` 163, `app` 126, `res` 116.

No estilo do `hono` (função-flecha exportada), a definição principal de cada módulo fica com o tipo errado, e cada variável local concorre com ela no ranking.

## 2. O que o tipo `constant` causa adiante

**Lido no código:**
- `retrieve.sectionOf` manda todo `constant` para a seção `EXISTING CONTRACTS`, que vem depois de `RELEVANT CODE` e de relações na ordem de seções.
- `packState.value` (V3.7, `PreferImplTieBreak`) multiplica por 0,35 um `constant` que não é match exato. É a penalidade do achado 3 de [quality-findings.md](quality-findings.md), pensada para constantes de valor.
- `engine.tierRank` dá `m*10 + 3` a um `constant` não exato; uma implementação fica com `m*10 + 5` ou `m*10 + 12`, mais 18 quando verificada.
- `retrieve.testLinks` trata `constant` pelo caminho de valores (varre o texto dos arquivos de teste).
- O rótulo entregue ao agente é `constant cors`.

**Medido no caso R04-1** (`Where is the middleware that configures CORS?`, gold `src/middleware/cors/index.ts`), motor V1:

```
RELEVANT CODE
- function set (src/middleware/cors/index.ts:101-103) [INFERRED] — function set(key: string, value: string) | called_by:cors(same_file)
EXISTING CONTRACTS
- file src/middleware/cors/index.test.ts [INFERRED] · import { Hono } from '../../hono' | tested_by:...
- constant cors (src/middleware/cors/index.ts:63-164) [VERIFIED] — cors = (options?: CORSOptions): MiddlewareHandler => · app.all('/api/abc', ...
```

O middleware pedido é a última linha, depois de uma função auxiliar interna e do arquivo de teste. O caso conta como gold entregue e como top-3, então nenhuma métrica atual vê o problema.

**Medido com três prompts próprios no `hono`** (40 candidatos por prompt):

| Prompt | Candidatos `constant` | O que foi entregue |
|---|---:|---|
| `where is the response body compressed?` | 36 | V1: `interface CompressionOptions`, duas variáveis `res` do teste, `stream`, e a implementação `compress` na 5ª linha. V3.7: `interface Response` de `src/client/types.ts`, `stream`, `nullBodyResponses`, `newResponse`, `res`; **`compress` não é entregue** |
| `how does the app handle a request error?` | 24 | V1: 4 rotas de `src/hono.test.ts` e 4 variáveis `res` do mesmo teste. V3.7: rotas e variáveis `request` e `handler` de arquivos de teste |
| `where is the request result cached?` | 34 | V1: `cachedBody`, `resUsingCachedRouter`, `req`, `cachedRes`, três `result`. V3.7: `interface CloudFrontResult` duas vezes, `cachedBody`, `result` |

Em todos, linhas como `constant res — res = await app.request(...)` saem com rótulo `[VERIFIED]`, `[INFERRED]` ou `[CANDIDATE]` na seção `EXISTING CONTRACTS`. São três prompts escritos por mim, sem gold: mostram o mecanismo, não uma taxa. Os casos do ten-repos no `hono` nomeiam o diretório do middleware (`CORS`, `autenticação básica`) e não expõem o problema.

**Direções a testar (não testadas):**
- Gravar como `function` o `const` cujo valor é função-flecha ou `function`. Muda o tipo de centenas de símbolos e exige subir `extractVersion`.
- Não indexar `const` declarado dentro de função ou de bloco de teste, ou gravá-lo com um tipo que não entra no ranking.
- As duas mudam o índice de todo repositório JS/TS. Validar no P1 com símbolo gold medido e nos casos do `hono`, `express` e `koa`.

## 3. O docstring guarda o fim do comentário, não o começo

**Lido no código.** `docAbove` sobe a partir da linha da definição, junta linhas de comentário e para ao chegar a 8. Num comentário mais longo ficam as 8 linhas mais próximas da definição: `@param`, `@returns`, `@example`, `</param>`. A primeira frase, que diz o que o símbolo faz, é a que se perde. Depois o texto é cortado em 400 caracteres.

**Exemplos medidos:**
- `cors` no `hono`: o comentário começa com `CORS Middleware for Hono.` e tem 39 linhas. O docstring gravado é `app.all('/api/abc', (c) => { return c.json({ success: true }) }) app.all('/api2/abc', ...` (o fim do bloco `@example`). É o texto que aparece no contexto entregue acima.
- `QueryAsync` no `dapper` (`Dapper/SqlMapper.Async.cs:26`): o docstring gravado começa em `</summary> <param name="cnn">...`. A frase `Execute a query asynchronously using Task.` ficou de fora. Isso explica parte do achado 10 de quality-findings (tags XML cruas): além das tags, o que sobra é a parte menos informativa.

**Medido** (símbolos de código com docstring cujo bloco de comentário tem mais de 8 linhas):

| Repositório | Com docstring | Começo descartado | % |
|---|---:|---:|---:|
| kotlinx-datetime | 599 | 218 | 36% |
| dapper | 495 | 105 | 21% |
| sinatra | 247 | 49 | 20% |
| hono | 358 | 68 | 19% |
| spring-petclinic | 42 | 8 | 19% |
| serde-json | 627 | 95 | 15% |
| axum | 1.185 | 149 | 13% |
| monolog | 719 | 72 | 10% |

Nos outros sete fica entre 0 e 9%. O docstring alimenta a coluna `doc_terms` do FTS (peso 2,0 no `bm25`), `DocstringOwners` e o texto renderizado na V3.7.

**Não medido:** o efeito em recall. Nenhum corte atual tem gold que dependa da primeira frase do comentário.

**Direção a testar:** guardar as primeiras linhas do bloco (até a primeira linha vazia ou a primeira tag `@`/`<param`), não as últimas. Exige subir `extractVersion`.

## 4. Atributos de Rust entram como docstring

**Lido no código.** `commentPrefixes` inclui `#` para todas as linguagens. Em Rust uma linha `#[inline]` ou `#[derive(Debug)]` começa com `#` e é tratada como comentário. Como os atributos ficam entre o comentário `///` e a definição, eles ocupam as primeiras posições da janela de 8 linhas do achado 3.

**Medido:**

| Repositório | Docstrings em Rust | Contêm texto de atributo | Começam por atributo |
|---|---:|---:|---:|
| serde-json | 666 | 433 | 234 |
| axum | 1.360 | 823 | 631 |

Exemplos gravados: `write` com docstring `[inline]`; `State` com `[derive(Debug, Default, Clone, Copy)]`; `extract_without_state` com `[tokio::test]`.

Fora de Rust o efeito é pequeno: 11 de 597 docstrings em C# no `dapper` começam por diretiva (`#if NET7_0_OR_GREATER`, `#pragma warning disable`).

**Direção a testar:** prefixos de comentário por linguagem; em Rust, pular linhas `#[...]` sem contá-las na janela.

## 5. Rust: métodos sem dono e macros ausentes

**Medido** (funções, métodos e construtores em arquivos de código, e quantos têm `parent_id`):

| Repositório | Linguagem | Total | Com pai |
|---|---|---:|---:|
| serde-json | Rust | 1.094 | 92 |
| axum | Rust | 1.812 | 56 |
| monolog | PHP | 715 | 715 |
| dapper | C# | 1.230 | 1.218 |
| sinatra | Ruby | 673 | 610 |
| click | Python | 675 | 424 |

Em `serde-json/src/de.rs` há 109 símbolos `function` e 8 `type`, todos sem pai. `new` aparece pelo menos 5 vezes, `from_str` 3 e `deserialize_any` 2, cada um num bloco `impl` diferente, e o índice não diz de qual tipo ou trait. O agente recebe `function new (src/de.rs:1925)`. As relações `member_of` e `contains` e qualquer navegação por tipo não funcionam em Rust. Os únicos tipos de símbolo gravados em Rust são `function`, `constant` e `type`: não há `trait` nem `impl`.

`macro_rules!`: 53 definições em `serde-json/src` e `axum/axum/src`, nenhuma no índice. `src/macros.rs` do `serde-json` (10 KB, onde está a macro `json!`, a API mais usada da biblioteca) tem zero símbolos.

**Hipótese (não verificada):** os dois fatos que o `sidecar explore` não cobre no E2 do `serde-json` (`deserializer|from_trait` e `deserialize_any|deserialize`, NAV-SER-01) dependem de ligar um método ao seu `impl`. Fica para a investigação do `explore`.

## 6. `type` do TypeScript não é indexado

**Medido.** `grep` encontra 359 declarações `type X` em `hono/src` fora dos arquivos de teste; o índice tem 3 símbolos `type` em arquivos de código (`LogPrefix`, `AlgorithmTypes`, `CryptoKeyUsage`), e os três são `enum`. Nenhum alias de tipo é indexado. Sessenta arquivos de código TypeScript do `hono` não têm nenhum símbolo além da linha do arquivo, entre eles `src/utils/headers.ts` (7,4 KB, só tipos exportados) e `src/utils/types.ts` (22 tipos). Uma pergunta sobre um desses tipos só casa pelo nome do arquivo.

`interface` é indexado (225 no `hono`).

## 7. A afirmação "no test file in the index references X"

O achado 7 de quality-findings registrou a linha como errada em R04-1 e deixou a causa em aberto, com a suspeita na extração de referências do TypeScript. **A extração está correta:** o índice tem 16 referências `call` a `cors` em `src/middleware/cors/index.test.ts`, o arquivo está classificado como `test` e `store.TestFilesReferencing("cors")` devolve esse arquivo quando executada direto no banco.

**Causa (lida no código e confirmada no trace):**
- O prompt é classificado como `locate`. `intentFamilies["locate"]` só expande `parent` e `members`; a família `tests` não é consultada.
- A política adaptativa pula contratos: o trace mostra `skipped_providers: ["contracts", "docs"]`.
- `brief/task.go` emite a linha quando há ponto de entrada confirmado e a lista de testes está vazia. A lista está vazia porque ninguém procurou.
- Com a V1, que expande todas as famílias, o mesmo prompt traz `tested_by:src/middleware/cors/index.test.ts`.

A linha afirma algo sobre o índice ("nenhum arquivo de teste referencia") quando o fato é "a busca de testes não rodou". Vale para todo prompt de localização com ponto de entrada confirmado, em qualquer linguagem.

**Direção a testar:** só emitir a linha quando a família `tests` ou o provedor de contratos rodou naquele prompt. É uma condição em `brief/task.go`, com teste de regressão no caso R04-1.

## 8. Extensões `.mts` e `.cts`

`langByExt` não tem `.mts` nem `.cts`. No `hono` são 27 arquivos, todos em `benchmarks/`, indexados só pelo nome. Baixa prioridade neste corpus; em projetos ESM mais novos a extensão é comum.

## O que não é problema

- Arquivos de teste sem símbolos em `express` (47) e `sinatra`: são blocos `describe`/`it` sem definições nomeadas. Esperado.
- `fastapi`: 217 arquivos Python sem símbolos são `__init__.py` e exemplos de uma linha.
- Templates (`.erb`, `.ejs`, `.haml`, `.html`) e `.kts` não são analisados e entram pelo nome. Coerente com a degradação declarada.

## Ordem sugerida

1. **Achado 7:** uma condição, corrige uma afirmação falsa na primeira linha do contexto e devolve o gold à linha 1 nos 6 casos do ten-repos listados em quality-findings.
2. **Achados 1 e 2:** maior efeito esperado em repositórios TypeScript. Começar por não indexar `const` local, que só remove ruído; depois o tipo da função-flecha. Antes, incluir no P1 casos do `hono` cujo gold é uma função-flecha sem o nome do diretório no prompt, porque os casos atuais não medem isso.
3. **Achados 3 e 4:** mesma função (`docAbove`), mesmo aumento de `extractVersion`. Fazer juntos.
4. **Achado 5:** dono de método em Rust; avaliar junto com os fatos faltantes do E2.
5. **Achados 6 e 8:** cobertura; baixo risco.

Os itens 2 a 5 mudam linhas do índice. Cada aumento de `extractVersion` refaz o índice de todo usuário, então vale agrupar as mudanças de extração em uma versão só.
