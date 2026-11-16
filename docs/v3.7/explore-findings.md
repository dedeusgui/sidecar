# V3.7: achados do `sidecar explore` no E2 de navegação

Por que o `sidecar explore` não cobre três fatos do E2 offline (`parse_args` em NAV-CLK-01; `deserializer|from_trait` e `deserialize_any|deserialize` em NAV-SER-01), e o que a investigação mostrou sobre o ranking que alimenta o `explore` e sobre a própria métrica. É o item que [handoff.md](handoff.md) §4.5 e [quality-findings.md](quality-findings.md) deixaram como não investigado. Nada foi corrigido.

## Como foi levantado

- `python3 bench/v3/e2_offline.py --sidecar <bin> --checkout <dir> --engines v1,v2,v3,v3.7`, sem CodeGraph, sobre os checkouts pinados. Reproduziu os números registrados: `explore` 23/26 fatos e 5/7 tarefas; hook V3.7 14/26.
- `sidecar explore --json` e `sidecar explain --json --engine v3.7` nas duas tarefas incompletas.
- Variações controladas do texto do prompt (tirar a instrução final, tirar o nome do projeto, deixar só o identificador), para isolar qual parte do prompt muda os arquivos mostrados. Nenhuma mudança de código.
- NAV-CLK-01 é do split `development`; NAV-SER-01 é do split `validation`. Depois desta análise NAV-SER-01 deixa de servir como validação não vista para qualquer ajuste feito a partir daqui.
- Cada item diz o que foi **medido**, o que é **lido no código** e o que **não foi testado**.

## Resumo

| # | Achado | Evidência | Onde |
|---|---|---|---|
| 1 | O arquivo do identificador citado no prompt fica fora dos arquivos mostrados | NAV-SER-01: `from_str` de `src/de.rs` é `VERIFIED` e aparece em "ranked below the shown files" | `core.Build` (ordem dos grupos) |
| 2 | A instrução final do prompt vira termo de busca | `name`, `function`, `modify`, `repository` (+ tradução `repo`) trazem `examples/repo/repo.py` | análise de termos |
| 3 | O nome do projeto e o qualificador do identificador viram termos | `serde_json::from_str` gera `serde`, `json`, `str`; constantes `TOKEN` ficam com cobertura 1 | análise de termos |
| 4 | Nomes iguais: a ligação por nome escolhe o símbolo errado | `CliRunner.invoke` no lugar de `Command.invoke`; stub de `@overload` mostrado como `invoke` | relações por nome, extração |
| 5 | `parse_args` nunca é candidato | sem docstring; só se chega a ele pela cadeia `main → make_context → parse_args` | sementes da expansão |
| 6 | A métrica conta substring em qualquer parte da saída | NAV-SER-01: os 2 fatos "cobertos" vêm de linhas de referência a um arquivo não mostrado | `e2_offline.covered` |
| 7 | `e2_offline.py` quebra se `--engines` não tiver `v2` e `v3` | `KeyError: 'v2'` ao montar o relatório | `bench/v3/e2_offline.py` |

## 1. O arquivo certo fica abaixo do corte de arquivos

**Medido em NAV-SER-01** (`How does serde_json::from_str parse a string into a typed Rust value? ...`). O `explore` mostrou `src/raw.rs`, `src/number.rs` e `src/value/from.rs`, os três com o motivo `matches: serde, json, str`. A definição pedida está em `src/de.rs`. No JSON ela aparece em `not_shown`:

```
function from_str  src/de.rs:96  VERIFIED  — ranked below the shown files
```

O hook marca o mesmo identificador como localizador verificado (`{"kind": "identifier", "text": "from_str", "verified": true}`). O `explore` tem o identificador exato, verificado, e não mostra o arquivo dele.

**Mecanismo (lido no código):**
- `explore.Run` busca com `NoExactDominance = true`, que mantém os casamentos parciais ao lado do exato. Sem essa opção eles seriam descartados.
- `core.Build` agrupa a evidência por arquivo na ordem do ranking e mostra os primeiros `maxFiles`. A única reordenação é código antes de teste.
- `anchoredPaths` (arquivos com item `VERIFIED` ou exato) é calculado, mas só decide o orçamento de caracteres por arquivo, não a ordem.
- `BudgetFor` dá 3 arquivos e 6.000 caracteres a repositórios com menos de 200 arquivos. O `serde-json` tem 93.

**Medido:** com `--max-files 5` a ordem é `src/raw.rs`, `src/number.rs`, `src/value/from.rs`, `README.md`, `src/de.rs`. O arquivo certo é o quinto.

**Variações do prompt:**

| Prompt | Fatos | Arquivos mostrados |
|---|---:|---|
| original | 2/4 | `src/raw.rs`, `src/number.rs`, `src/value/from.rs` |
| sem `Name the functions and types. Do not modify the repository.` | 3/4 | `README.md`, `src/read.rs`, `src/raw.rs` |
| sem o qualificador (`How does from_str parse ...?`) | 3/4 | `src/value/mod.rs`, `src/read.rs`, `README.md` |
| só `from_str` | 4/4 | `src/de.rs`, `src/value/de.rs`, `src/number.rs` |

`src/de.rs` só aparece quando o prompt é o identificador sozinho. `explore.Run` tem uma regra para consultas de até 3 palavras (o nome vira chave exata extra); numa frase, o mesmo identificador perde para os termos comuns da frase (`parse`, `string`, `value`, `str` casam com `parse_str`, `as_str`, `is_string`).

**Direção a testar:** em `core.Build`, pôr primeiro os grupos de `anchoredPaths` cujo item é exato e veio de um identificador escrito no prompt. Não muda o ranking do hook. Validar nas 7 tarefas com a métrica do achado 6 corrigida.

## 2. A instrução final do prompt vira termo de busca

As 7 tarefas do corpus terminam com `Name the ... Do not modify the repository.`

**Medido em NAV-CLK-01.** Termos extraídos: `click, turn, command, line, argument, keyword, passed, callback, name, function, modify, repository`, com a tradução `repository → repo`. Entre os candidatos estão `cli`, `delete`, `clone` e `commit` de `examples/repo/repo.py`, um exemplo cujo único vínculo com a pergunta é o nome do arquivo.

| Prompt | Símbolos de `src/click/core.py` mostrados |
|---|---|
| original | `forward`, `Command`, `result_callback`, `invoke` (stub, ver achado 4) |
| sem a instrução final | `forward`, `Command`, `result_callback`, `Argument`, `format_arguments`, **`make_context`**, **`main`**, `scope` |

Sem a instrução, dois dos métodos da cadeia pedida passam a ser mostrados. No `serde-json` a mesma retirada leva de 2/4 para 3/4 (tabela do achado 1).

Isso não é só do benchmark. Prompts reais trazem instruções do mesmo tipo ("não altere nada", "só me explique", "liste os arquivos"), e o prompt de voz do ten-repos traz "não precisa executar o projeto nem alterar arquivo nenhum".

**Lido no código:** `v37FillerTerms` remove palavras de estrutura em inglês (`which`, `into`, `about`...), mas não verbos de instrução nem os substantivos `function`, `file`, `repository`, `name`. O achado 4 de quality-findings registra o mesmo problema para palavras de estrutura em português.

**Direção a testar:** separar a parte do prompt que é instrução ao agente (frases imperativas sobre o que fazer ou não fazer com o repositório) e não extrair termos dela. Regras fixas, em PT e EN, como as de `ClassifyIntent`. Risco: descartar uma palavra que também é do domínio do repositório (`repository` num projeto que tem uma classe `Repository`); por isso a regra deve valer para a frase de instrução, não para a palavra.

## 3. O nome do projeto e o qualificador viram termos

**Medido.** `serde_json::from_str` gera os termos `serde`, `json` e `str` (`from` é descartado). No `serde-json`, `serde` e `json` aparecem em quase todo arquivo. As constantes `TOKEN` de `src/raw.rs` e `src/number.rs` têm a assinatura `TOKEN: &str = "$serde_json::private::RawValue"`, que contém os três termos, e ficam com cobertura 1. O `from_str` de `src/de.rs`, que é o identificador exato, fica com cobertura 0,62 e 0,30. O hook marca `serde_json` como localizador **não** verificado e mesmo assim usa suas partes como termos.

No `click`, o termo `click` entra no motivo dos três arquivos mostrados (`matches: keyword, passed, function, argument, click`).

**Direções a testar:** quando o prompt tem `a::b` ou `a.b` e `b` resolve como identificador exato, não usar as partes de `a` como termos léxicos; reduzir o peso de um termo que é o nome do repositório ou que casa com a maioria dos arquivos. A opção `RaritySupport` já existe no pool do `explore`; **não medido** por que ela não segura estes casos.

## 4. Nomes iguais

**Medido no `click`:**
- O primeiro arquivo mostrado é `src/click/testing.py`, pelo método `CliRunner.invoke` (625-782). Os métodos da pergunta são `Command.invoke` (1425) e `Context.invoke` (886) em `core.py`.
- Os pontos de entrada listados são funções de teste (`test_prefixes`, `test_counting`...) "top of the indexed call chain into invoke". Elas chamam `runner.invoke(...)`; a ligação é por nome.
- Em `core.py` o símbolo mostrado é `invoke :884-884`. É um stub de `@t.overload`, de uma linha. O índice tem `invoke` em 879-881 e 884-884 (stubs) e em 886-936 (implementação), e `main` em 1489-1496 e 1499-1506 (stubs) e em 1508-1642 (implementação), todos como métodos comuns.

**Medido no `serde-json`:** `src/de.rs` tem três `from_str` (linhas 96, 1299 e 2709). A função pública pedida é a de 2709; a referência usada é sempre a de 96, método de um bloco `impl`. Sem dono de método em Rust ([extraction-findings.md](extraction-findings.md), achado 5) o índice não distingue as três.

É o mesmo padrão do achado 8 de quality-findings (sobrecargas no `dapper`), agora em Python e Rust.

**Direções a testar:** marcar stubs de `@overload` na extração e preferir a definição com corpo; entre definições de mesmo nome, preferir a que não está em arquivo de teste e a de nível de módulo quando o prompt usa o nome qualificado pelo módulo.

## 5. `parse_args` nunca é candidato

**Medido.** `Command.parse_args` (`core.py:1392`) não aparece entre os 40 candidatos do hook para o prompt original, nem entre os símbolos mostrados pelo `explore` em nenhuma das três variações. O método não tem docstring, e o prompt diz `arguments`, não `args`. `make_context` é candidato com cobertura 0,30 e não é mantido.

**Lido no código do `click`:** a cadeia é `main` (1584) → `make_context` → `parse_args` (1389) → `invoke` (1585). O `explore` expande cadeias de chamada a partir das sementes, mas as sementes foram `CliRunner.invoke`, `secho` e `forward`, então a expansão foi gasta em `testing.py` e `termui.py`.

Este fato depende dos achados 2 e 4: com sementes certas (`main`, `make_context`), `parse_args` está a uma chamada de distância. **Não testado** se a expansão chega a ele quando a semente é `make_context`.

## 6. A métrica conta substring em qualquer parte da saída

**Lido no código.** `e2_offline.covered` marca um fato como coberto quando a string aparece, sem distinção de maiúsculas, em qualquer ponto do texto do `explore`. Isso inclui as linhas "ranked below the shown files", as linhas de contrato e os pontos de entrada.

**Medido:**
- NAV-SER-01, 2/4. `de.rs` e `from_str` são contados pelas linhas de contrato e por `[VERIFIED] function from_str (src/de.rs:96) — ranked below the shown files`. Nenhum trecho de `src/de.rs` foi mostrado. `from_str` também casa com `crate::from_str` dentro de um trecho de `src/raw.rs`.
- NAV-CLK-01, 3/4. `make_context|main` é contado por `function main (tests/test_options.py:2019)` e por um docstring de `testing.py` que cita `Command.main`; nem `make_context` nem `Command.main` foram mostrados no prompt original. `invoke|ctx.invoke` casa primeiro com `CliRunner.invoke`; de `core.py` só o stub de uma linha foi mostrado.
- O oráculo tem alternativas curtas (`main`, `deserialize`, `write(`, `handle `) que casam com qualquer ocorrência. Só `main` foi conferida.

Os 23/26 são um teto. Nas duas tarefas abertas, pelo menos 3 fatos contados não correspondem a código mostrado. As outras 5 tarefas não foram auditadas, e o mesmo vale para os 24/26 do CodeGraph, medidos com a mesma função. A comparação entre os dois braços pode continuar válida, mas nenhum dos dois números mede "o agente recebeu a função".

**Direção a testar:** contar o fato só quando ele aparece no cabeçalho de símbolo ou no trecho de um arquivo mostrado, e exigir o arquivo junto do nome quando o oráculo dá os dois (`core.py` + `parse_args`). Reportar as duas contagens lado a lado por uma rodada, para ver quanto muda.

## 7. `e2_offline.py` exige `v2` e `v3` em `--engines`

`--engines v1,v3,v3.7` termina em `KeyError: 'v2'`: o relatório em Markdown usa as colunas `v2` e `v3` fixas. Com `--engines v1,v2,v3,v3.7` roda. O texto de ajuda sugere listas livres.

## O que isso muda nas conclusões anteriores

- A hipótese registrada em extraction-findings (os fatos de NAV-SER-01 dependeriam de ligar método a `impl`) **não é a causa principal**. O arquivo nem é mostrado. O dono do método só passa a importar depois dos achados 1 a 3.
- "O `explore` cobre 23 fatos e o CodeGraph 24" deve ser lido como teto, pelos motivos do achado 6.
- Os achados 2 e 3 são do ranking compartilhado (`retrieve.Search`), não do `explore`. Valem para o hook em qualquer prompt longo com instrução ou com o nome do projeto.

## Ordem sugerida

1. **Achado 6:** corrigir a métrica antes de qualquer ajuste, pelo mesmo motivo do achado 2 de quality-findings: sem medida confiável não há como provar ganho. Junto, o achado 7.
2. **Achado 1:** reordenar grupos ancorados no `explore`. Mudança local, não toca o hook.
3. **Achado 2:** frases de instrução fora da extração de termos. Afeta o hook; exige P1 completo e os negativos.
4. **Achado 3:** qualificador e nome do projeto.
5. **Achado 4:** stubs de `@overload` e escolha entre nomes iguais, junto com as mudanças de extração.
6. **Achado 5:** verificar de novo depois dos anteriores; pode se resolver sozinho.
