# V3.7: o que ainda dá para otimizar

Lista do que foi encontrado ao perfilar o hook quente depois das três correções do PR #30 (ver [progress.md](progress.md#latência-do-hook-três-causas-isoladas-e-corrigidas-sem-mudança-de-saída)). Cada item diz o que foi **medido**, o que é só **lido no código** e o que é **hipótese**. Nada aqui foi implementado.

## Como foi medido

- `app.Hook` chamado em processo, índice completo (`sidecar index --root <abs> --history`), `SIDECAR_HOME` isolado, perfil de CPU do `go test -bench`.
- Três alvos: este repositório (617 arquivos, muito Markdown, 4 prompts), `hono` com o prompt de voz `R04-4` (~1.100 caracteres) e `sinatra` com os prompts `R06-*`.
- Os milissegundos abaixo são CPU por hook (amostras ÷ execuções), já com o PR #30 aplicado. Máquina de 4 CPUs com carga variável: servem para ordenar os itens, não como número de gate.
- Referência depois do PR #30: V1 ~16 ms e V3.7 ~42 ms por hook neste repositório; V1 ~22 ms e V3.7 ~67 ms no prompt de voz do `hono`; p50 do ten-repos V1 11 a 13 ms, V3.7 29 a 33 ms.

## Itens, do maior ganho provável ao menor

### 1. Evidência de contrato roda antes do gate de relevância

- **Onde:** `retrieve.v2Pipeline` chama `ContractEvidence` dentro de `retrieve.Search`; o gate da V3.7 (`relevanceGateV37`, `hasCodeAnchorV37`) só roda depois, em `engine.Run`.
- **Medido:** `ContractEvidence` é ~14 ms por hook neste repositório e ~18 ms no prompt de voz do `hono`, o maior bloco isolado da V3.7. Um prompt que termina em abstenção paga esse custo e descarta o resultado.
- **Não medido:** quanto do custo cai em prompts que abstêm (os fixtures negativos são repositórios de 15 arquivos, onde o custo é pequeno).
- **Ideia:** rodar contratos só depois do gate, ou só para as definições que sobrevivem à seleção. Hoje são até 12 definições (`maxSubjects`) de uma janela de 60 itens.
- **Risco:** muda a ordem dos estágios. Os conflitos de contrato entram no packer V2 como itens `CONCERNS` e os achados alimentam `brief.MicroBriefProposal` e `providersOf`. Precisa de P1 com comparação de contexto.
- **Observação:** a V3.7 não exibe as anotações de contrato que não são conflito. `v37ActionableRelations` só mantém `tested_by:`, `called_by:` e `calls:`, e as anotações de contrato têm a forma `test caminho:linha [STATUS]`. O trabalho de produzi-las só chega ao agente via TaskBrief e conflitos.

### 2. `DocstringOwners`: varredura completa por definição

- **Onde:** `store.DocstringOwners` (`instr(s.doc, ?) > 0` sem índice), chamada uma vez por definição em `ContractEvidence`.
- **Medido:** ~6,3 ms por hook neste repositório (até 12 varreduras da tabela `symbols`).
- **Ideia:** uma consulta só que traga os símbolos com docstring e filtre os 12 nomes em Go, ou reaproveitar o `symbol_fts` para pré-filtrar.
- **Risco:** baixo se o filtro em Go repetir o `instr` e a ordem `ORDER BY s.id LIMIT 5` por nome.

### 3. `lexical.Translations` recalculada por candidato × termo

- **Onde:** `retrieve.ownMatchesWithKind` (chamada para cada candidato em `Search`) → `lexical.Translations` → `CognateCandidates`.
- **Medido:** ~15,8 ms por hook no prompt de voz do `hono` (~21% do hook), ~2,5 ms neste repositório. O PR #30 já tirou a reconstrução do `strings.NewReplacer`; o que sobra é repetir `Fold`, `Stem` e `transduceSuffixes` para o mesmo termo.
- **Ideia:** calcular as traduções uma vez por termo no início de `Search` e passar o mapa.
- **Risco:** baixo; a função é determinística por palavra. Sem mudança de saída esperada.

### 4. Leitura de arquivos em `testLinks` e na varredura de docs

- **Onde:** `retrieve.fileReader.text` → `repo.ReadSource` e `repo.Contained` (`filepath.EvalSymlinks` por arquivo).
- **Medido:** ~4,7 ms por hook neste repositório e ~10 ms no prompt de voz do `hono`. Para uma definição do tipo `constant`, `testPathsForValues` lista até 300 arquivos de teste e todos são lidos.
- **Ideia:** resolver o symlink da raiz uma vez por chamada; para constantes, pré-filtrar os arquivos de teste pelo índice (`refs`/FTS) antes de ler do disco.
- **Risco:** o pré-filtro pelo índice pode perder uma constante usada como argumento, que o índice não registra (é o motivo declarado da varredura por texto). Precisa de teste.

### 5. `index.Sync` carrega `FileRecords` duas vezes por prompt

- **Onde:** `internal/index/index.go`, linhas 115 e 182 (a segunda só para contar `rep.Files`).
- **Medido:** `FileRecords` ~3,6 ms por hook neste repositório, de ~4,8 ms do `Sync`. Afeta todos os motores, inclusive a V1.
- **Ideia:** `SELECT count(*)` para a contagem, ou reaproveitar o mapa quando nada foi parseado nem removido.
- **Risco:** baixo.

### 6. Três empacotamentos sobre uma janela de 60 itens

- **Onde:** packer V2 (`retrieve.Pack`), `engine.pack` e `ctxbuild.PackWith` em `runV35`. A V3.7 sobe `MaxItems` para 60.
- **Medido:** `retrieve.Pack` ~4,5 ms por hook neste repositório (`baseValue`, `costOf`, `Describe`, `complement`).
- **Ideia:** cachear `Describe`/custo por item dentro de `Pack`; avaliar se o packer V2 precisa rodar completo quando o planner da V3.7 vai escolher de novo.
- **Risco:** o segundo ponto muda seleção. Só com ablação e P1.

### 7. `SearchSymbols`

- **Medido:** ~4,2 ms por hook neste repositório, ~7,7 ms no prompt de voz (a expressão FTS cresce com o número de termos; `_fts5ApiColumnSize` aparece no perfil).
- **Não investigado.** Candidato: limitar termos de baixa saliência antes de montar o `MATCH`. Mexe em recall.

### 8. Subprocesso `git rev-parse HEAD` repetido

- **Onde:** `runV35` chama `gitx.Head`; o `index.Sync` do mesmo hook já obteve o HEAD por `gitx.Status` (`Report.Head`).
- **Medido:** só CPU do pai (~0,3 ms). O custo de relógio de criar o processo **não foi medido** e não aparece em perfil de CPU.
- **Ideia:** passar `sync.Head` para `runV35`.
- **Risco:** baixo.

### 9. Índice `refs(to_name, file_id)`

- **Medido (SQLite em C, cópia do índice do `sinatra`):** com o índice composto a consulta já reescrita de `TestFilesReferencing` cai de ~0,31 ms para ~0,16 ms em nomes muito referenciados.
- **Custo:** migração de schema e índice maior. Ganho pequeno depois do PR #30; só vale junto com outra migração.

### 10. Detalhes pequenos

- `paramCount` (`internal/retrieve/contract.go`) compila uma regex por chamada (`regexp.MustCompile` com o nome da função). Lido no código; não apareceu no topo do perfil.
- `registerV35Evidence` faz hash do arquivo em disco e uma consulta `FileRecordOf` por arquivo de evidência (~0,5 ms por hook).
- Coletor de lixo: ~16% das amostras antes do PR #30, puxado por `strings.Split` de arquivos inteiros. Não remedido depois.

## Hipóteses a verificar (não medidas)

- **`queryCached` pode não economizar o parse.** `_sqlite3Prepare` ainda é 3 a 5% do hook. Se o driver `modernc.org/sqlite` preparar o SQL a cada execução, o cache de `*sql.Stmt` em `store.go` não evita o parse. Verificar no código do driver antes de mexer.
- **Recompilação (`sqlite3Reprepare`).** Apareceu com 1 a 2% nestes perfis, contra 26 a 30% no registro anterior. Não foi reproduzida nem explicada; a diferença pode ser do repositório ou do prompt usados naquela medição.

## Comportamentos observados fora do caminho quente

- **Índice incompleto reescreve arquivos pulados a cada prompt.** Com o indexador de fundo desligado no benchmark, o hook reportou `sync_parsed: 3` e `sync_complete: false` em todas as execuções, com `sync_ms` ~27 em vez de ~8. Leitura do código: arquivos pulados (binário, sensível, grande demais) não têm hash, então nunca caem no ramo `unchanged` de `index.parse` e passam por `ReplaceFile` toda vez; e, sem `indexed_head`, cada prompt refaz o `ListFiles` e trunca nos mesmos 300 primeiros arquivos (`hookMaxFiles`). Em uso real o indexador de fundo resolve; o efeito existe na janela até ele terminar. Causa inferida do código, não isolada em teste.
- **`sidecar index --root .` e o hook criaram dois diretórios de índice** para o mesmo repositório (raiz relativa contra absoluta). Observado uma vez; não investigado.
- **A cauda do ten-repos vinha de um repositório só.** Antes do PR #30, 3 dos 40 casos (`sinatra`) definiam o p95. Um gate de p95 sobre 40 casos reage a um único repositório; vale olhar a latência por caso antes de concluir sobre o motor.
- **`gofmt -l internal/`** lista `internal/retrieve/retrieve.go` e `internal/studio/studio_test.go` (já estava assim na `main`).

## Qualidade (do handoff, não investigado aqui)

Continuam abertos e sem diagnóstico novo: recall interno 35 contra 39 da V1 (E06, E12, T08, E20), `voice_mode` top-3 9 contra 10 da V3 no ten-repos, negativos do E1 com saída (16/40) e os fatos faltantes do E2. Ver [handoff.md](handoff.md) §4.

## Ordem sugerida

1. Itens 3, 5 e 8: sem mudança de saída esperada, validação por comparação de contexto como no PR #30.
2. Item 2 e a parte de symlink do item 4: mesma validação, um pouco mais de código.
3. Item 1: maior ganho, mas muda a ordem dos estágios; PR próprio com `fast_gate.py` completo.
4. Itens 6 e 7: só com ablação, porque mexem em seleção e recall.
