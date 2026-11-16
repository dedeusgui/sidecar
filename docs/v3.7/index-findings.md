# V3.7: achados de índice e sincronização

O que o caminho de índice (`internal/index`, `internal/gitx`, `internal/app`, `internal/paths`) faz de caro ou de errado antes de qualquer motor rodar. Tudo aqui é compartilhado por V1, V3 e V3.7, então afeta o motor padrão. Complementa o [backlog de latência](optimization-backlog.md) e os [achados de qualidade](quality-findings.md). Nada foi corrigido: é diagnóstico para decidir o próximo trabalho.

## Como foi levantado

- `app.Hook` chamado em processo com `SIDECAR_HOME` isolado e o indexador de fundo substituído por uma função vazia, lendo o registro que o hook grava em `hook.jsonl` (`sync_ms`, `sync_parsed`, `sync_complete`, `files`). Motor V1 (padrão).
- Repositórios: os checkouts pinados de `bench/v3.7/fetch_external.py` (`fastapi` 3.240 arquivos, `hono` 584, `chi` 110 e os demais). Para o `fastapi` só o índice e um prompt próprio foram usados; nenhum caso da coorte E1 foi aberto.
- `git status` medido com as mesmas opções que `gitx.Status` usa, mínimo de 3 execuções. `strace -c` para contar leituras de arquivo.
- Máquina de 4 CPUs com carga variável. Os milissegundos ordenam os itens; não são número de gate.
- Cada item diz o que foi **medido**, o que é **lido no código** e o que é **hipótese**.

## Resumo

| # | Achado | Evidência | Onde |
|---|---|---|---|
| 1 | Em checkout recém-criado o `git status` do hook relê todos os arquivos a cada prompt, e o hook nunca corrige isso | `fastapi`: ~90 ms contra ~8 ms; `sync_ms` 104 a 116 contra 18 a 29 | `gitx.gitRun` (`GIT_OPTIONAL_LOCKS=0`) |
| 2 | Sem indexador de fundo o hook não avança além dos primeiros 300 arquivos | `fastapi`: `files=300` de 3.240 em 6 prompts seguidos | `index.parseAll`, `hookMaxFiles` |
| 3 | Os 300 primeiros são um prefixo alfabético; num repositório com muita doc não há código neles | `fastapi`: 268 de 300 são `docs/de` e `docs/en`, zero de `fastapi/` | `cleanPaths`, `parseAll` |
| 4 | Um lock deixado por um indexador morto bloqueia a indexação por 10 minutos, sem registro | reproduzido com um arquivo `.lock` criado à mão | `maybeSpawn`, `acquireLock` |
| 5 | `FileRecords` roda duas vezes por prompt e cresce com o número de arquivos | 6,0 ms por chamada em 3.240 arquivos | `index.Sync` |
| 6 | Um índice por grafia da raiz, e nenhum índice é descartado | `--root .`, caminho absoluto e symlink: 3 diretórios para o mesmo repositório | `paths.IndexDB`, `cmdIndex` |
| 7 | Arquivo modificado e não commitado é relido, tem o hash refeito e gera um `UPDATE` em todo prompt | 40 arquivos sujos: `sync_ms` 2,8 → 5,2 | `index.parse`, `SetFileStat` |
| 8 | Hipótese do backlog verificada: o cache de `*sql.Stmt` funciona; o que ainda prepara SQL são as consultas diretas | lido no driver `modernc.org/sqlite` v1.45.0 | `store.queryCached` |

## 1. Índice do Git "racy": o hook relê o repositório inteiro a cada prompt

**Medido.** Logo depois do checkout, todas as entradas do índice do Git têm `mtime` igual ou posterior ao do próprio arquivo `.git/index` (`git ls-files --debug`: 3.093 de 3.240 no `fastapi` e 100% em cada um dos outros 14 repositórios pinados). Para uma entrada assim o Git não confia no `stat` e compara o conteúdo. O `strace -c` de um `git status` no `fastapi` conta 3.556 chamadas `openat` e 2.774 `read`.

Um `git status` comum regrava o índice e resolve na primeira execução. O hook não: `gitx.gitRun` define `GIT_OPTIONAL_LOCKS=0`, que proíbe essa regravação. Depois de 5 execuções com as opções do hook o `axum` continuava com 508 de 508 entradas nesse estado.

| Repositório | Arquivos | `git status` do hook, índice racy | Depois de um `git status` comum |
|---|---:|---:|---:|
| fastapi | 3.240 | ~90 ms | ~8 ms |
| kotlinx-datetime | 903 | 21,7 ms | 4,3 ms |
| hono | 584 | 15,9 ms | 4,1 ms |
| axum | 508 | 10,6 ms | 4,9 ms |
| symfony-http-kernel | 399 | 8,7 ms | 3,4 ms |
| sinatra | 292 | 6,1 ms | 3,4 ms |
| dapper | 230 | 7,8 ms | 3,2 ms |
| chi | 110 | 4,7 ms | 3,1 ms |

No hook inteiro (`fastapi`, índice do Sidecar completo, `sync_parsed=0`): `sync_ms` 104 a 116 e `total_ms` 120 a 134 com o índice do Git racy; `sync_ms` 18 a 29 e `total_ms` 34 a 54 depois de um `git status` comum.

**Quando acontece em uso real.** Clone novo, `git checkout` ou `git pull` que reescreve arquivos, worktree nova, sandbox de CI ou sessão em nuvem. Dura até outro processo rodar um comando Git que regrave o índice (o próprio agente, a IDE ou o usuário). Num repositório grande só os arquivos escritos no último segundo do checkout ficam nesse estado; num pequeno ficam todos.

**Efeito nos números já publicados.** Os checkouts do benchmark saem de `fetch_external.py` com 100% das entradas nesse estado, e o harness não roda nenhum comando Git que as corrija. Os p50 e p95 do ten-repos e do E1 em [progress.md](progress.md) e [handoff.md](handoff.md) incluem esse custo fixo por repositório, igual para todos os motores: de ~2 ms (`chi`) a ~12 ms (`hono`) e ~17 ms (`kotlinx-datetime`). O gate de latência é uma razão (`1,5× + 5 ms`); um custo fixo dos dois lados aproxima a razão de 1. A V1 tem p50 de 11 a 13 ms no ten-repos, então no `hono` a maior parte do tempo medido da V1 é o Git relendo arquivos, não o motor. A diferença absoluta entre motores não muda; a razão real do tempo de motor é maior do que a medida.

**Direções a testar (não testadas):**
- No benchmark: rodar um `git status` comum em cada checkout depois do `fetch_external.py` e antes de medir, e registrar isso no relatório. Sem mudança de produto.
- No produto: decidir a mudança por arquivo com o `stat` que o Sidecar já guarda (`size`, `mtime_ns` em `files`), como `statPlan` faz fora do Git, e usar o Git só para HEAD e lista de arquivos. Custo esperado: um `lstat` por arquivo.
- No `doctor`: medir o `git status` e avisar quando ele está lendo conteúdo.
- **Não** remover `GIT_OPTIONAL_LOCKS=0` sem decisão explícita: ele existe para o hook não disputar o lock do índice com o Git do usuário e não escrever dentro do repositório.

## 2. Sem indexador de fundo, o hook não passa dos primeiros 300 arquivos

**Medido** (indexador de fundo desligado, 6 prompts seguidos, índice do Git já corrigido):

| Repositório | `files` | `sync_parsed` (1º → demais) | `sync_complete` | `sync_ms` (1º → demais) | `sync_ms` com índice completo |
|---|---:|---|---|---|---:|
| fastapi (3.240) | 300 em todos | 300 → 42 | false em todos | 218 → 39 a 48 | 18 a 29 |
| hono (584) | 292 em todos | 292 → 7 | false em todos | 362 → 22 a 30 | 5 a 6 |

**Mecanismo (lido no código):**
- Sem `indexed_head`, `gitPlan` faz varredura completa em todo prompt (`git ls-files` e todos os caminhos como `changed`).
- `parseAll` ordena os caminhos e corta em `changed[:maxFiles]` **antes** de saber quais já estão indexados. O comentário de `Options.MaxFiles` diz "stop after parsing this many files", mas o corte é sobre candidatos. No prompt seguinte os mesmos 300 voltam, são lidos e têm o hash conferido, e o lote nunca avança.
- Como a lista foi truncada, `complete` é falso, `indexed_head` não é gravado e o ciclo se repete.
- Arquivos pulados (binário, sensível, grande demais) não têm hash, então nunca caem no ramo `unchanged`: passam por `ReplaceFile` em todo prompt. São os 42 e os 7 de `sync_parsed`. Isso confirma, com medição, a leitura de código registrada no backlog.
- Cada arquivo inalterado ainda gera um `SetFileStat` (um `UPDATE`).

Em uso normal o indexador de fundo termina (`fastapi`: ~9,9 s para o índice completo, 20 MB; `hono` ~2,1 s; este repositório ~1,5 s) e o problema dura só essa janela. Ele vira permanente quando o indexador não roda: binário movido, falha ao criar o processo, ou o caso do achado 4.

**Direções a testar:** cortar em `maxFiles` depois de descartar os arquivos cujo `stat` não mudou, para o lote avançar a cada prompt; gravar hash vazio ou marca própria para arquivo pulado, para ele cair no ramo `unchanged`.

## 3. O primeiro lote é um prefixo alfabético

**Medido.** No `fastapi` os 300 primeiros caminhos em ordem de bytes são 142 de `docs/en`, 126 de `docs/de`, 26 de `.github` e 6 arquivos da raiz. O pacote `fastapi/` (62 arquivos, o código da biblioteca) não entra. Com esse índice parcial, o prompt `where is the request body validated?` recebeu 8 itens, todos seções de documentação, 6 deles em alemão (`docs/de/docs/tutorial/...`), 7 com rótulo `[VERIFIED]`.

O contexto entregue não diz que o índice está incompleto. O estado `partial` só vai para o `hook.jsonl`.

**Direções a testar:** ordenar o primeiro lote por tipo de arquivo (código antes de doc e de configuração) ou por casamento do caminho com os termos do prompt; quando `sync_complete` é falso, dizer isso em uma linha do contexto ou abster. A segunda muda texto entregue e precisa de P1.

## 4. Lock de indexador morto bloqueia por 10 minutos

**Reproduzido.** Com um arquivo `index.db.lock` criado à mão (simulando um indexador morto sem liberar o lock), o hook respondeu com `files=300`, `sync_complete=false` e **sem** `spawn_error`: `maybeSpawn` vê um lock com menos de 10 minutos e retorna sem iniciar nada e sem registrar o motivo.

**Lido no código:**
- O lock guarda o PID, mas ninguém confere se o processo existe. A única saída é a idade de 10 minutos.
- O `mtime` do lock não é renovado durante a execução. `RunIndexer` tem limite de 15 minutos. Uma indexação que passe de 10 minutos tem o lock removido por `acquireLock` e um segundo indexador começa sobre o mesmo banco. Não medido: nenhum repositório do corpus chega perto (a ~3 ms por arquivo, seriam ~200 mil arquivos).

**Direções a testar:** conferir o PID do lock antes de respeitá-lo; renovar o `mtime` durante a execução; registrar no `hook.jsonl` quando a indexação foi adiada por lock.

## 5. `FileRecords` duas vezes por prompt

O backlog já registra o item (~3,6 ms por hook neste repositório, 617 arquivos). Medida nova para ver a escala: no `fastapi` (3.240 arquivos) cada chamada custa **6,0 ms**, média de 20. São ~12 ms por prompt, a maior parte do `sync_ms` de 18 a 29 que sobra depois do achado 1. `StaleFilePaths` custa 0,46 ms e `DirtyPaths` 0,01 ms no mesmo índice. O custo cresce de forma aproximadamente linear com o número de arquivos, e a segunda chamada só serve para `len(files)`.

## 6. Um índice por grafia da raiz, sem descarte

**Reproduzido** no `chi`, com `SIDECAR_HOME` vazio:

| Comando | Diretórios em `repos/` |
|---|---:|
| `sidecar index --root .` (dentro do repositório) | 1 |
| `sidecar index --root /caminho/absoluto/chi` | 2 |
| `sidecar index --root /caminho/absoluto/chi/` | 2 |
| `sidecar index --root <symlink para o chi>` | 3 |
| hook com `cwd` em `chi/middleware` | 3 (reusa o do caminho absoluto) |

**Lido no código:**
- `paths.IndexDB` faz hash de `filepath.Clean(raiz)`, sem `filepath.Abs` e sem resolver symlink.
- `cmdIndex` passa `--root` como veio. Os outros comandos que aceitam `--root` (`status`, `explain`, `explore`, `memory`, `feedback`, `impact`) passam por `filepath.Abs` e `repo.FindRoot`, então só o `index` cria o índice de raiz relativa. É o comando que o backlog registrou como "dois diretórios de índice".
- Nenhum comando resolve symlink da raiz. Um repositório aberto por dois caminhos (por exemplo `/tmp` e `/private/tmp` no macOS, ou um atalho na pasta do usuário) tem dois índices e duas indexações frias.
- Nada remove diretórios de `repos/`. A única limpeza é `uninstall --purge`, que apaga tudo. Cada worktree tem índice próprio por desenho; worktrees temporárias de agentes deixam um índice cada (4,5 MB neste repositório, 20 MB no `fastapi`) e cada uma paga a indexação fria inteira, mesmo com conteúdo quase idêntico ao da worktree principal.

**Direções a testar:** `filepath.Abs` e `EvalSymlinks` em `cmdIndex` (ou dentro de `paths.IndexDB`; isso muda a chave e faz os índices atuais serem recriados uma vez); descartar índices cuja raiz não existe mais ou sem uso há N dias; semear o índice de uma worktree nova a partir do índice de outra worktree do mesmo repositório (os hashes de arquivo já estão guardados). A última é a de maior ganho para fluxos com worktrees e a de maior risco.

## 7. Arquivos modificados custam leitura, hash e escrita por prompt

**Medido** (`chi`, clone limpo, índice completo): `sync_ms` de 2,6 a 3,2 sem arquivos modificados e de 4,4 a 6,0 com 40 arquivos `.go` modificados e não commitados, com `sync_parsed=0` em todos os prompts. São ~60 µs por arquivo sujo.

**Lido no código:** todo caminho que o `git status` lista entra em `changed`. `parse` lê o arquivo, calcula SHA-256, vê que o hash é o mesmo e devolve `unchanged`; `parseAll` grava `SetFileStat` mesmo assim. O comentário de `Sync` diz que as escritas são a parte cara e disputada do prompt e devem ser puladas quando nada mudou, mas esse caminho escreve uma vez por arquivo sujo por prompt.

**Direção a testar:** comparar `size` e `mtime_ns` com o registro antes de ler o arquivo, e não gravar `SetFileStat` quando os dois já são iguais. Ganho pequeno em milissegundos; o que importa é não escrever no banco em prompt sem mudança.

## 8. Hipótese do backlog: `queryCached` economiza o parse?

**Verificado no código do driver** (`modernc.org/sqlite` v1.45.0, `stmt.go`): `newStmt` prepara o comando uma vez e guarda o handle compilado (`pstmt`) quando o SQL tem um único comando; `stmt.query` reusa esse handle no caminho que o driver chama de otimizado. Então o cache de `*sql.Stmt` em `store.queryCached` evita o parse. A hipótese contrária do backlog está descartada.

O que ainda prepara SQL a cada chamada são as consultas que não passam por `queryCached`. Em `internal/store`, `query.go` tem 7 chamadas com cache, 6 `s.db.Query` e 5 `s.db.QueryRow` diretas; `store.go` tem 2 com cache, 8 e 7 diretas. `SearchSymbols` (a consulta FTS5 do caminho quente) é uma das diretas. É a origem provável dos 3 a 5% de `_sqlite3Prepare` do perfil. **Não medido** quanto cada uma pesa.

## Ordem sugerida

1. **Achado 1, lado do benchmark:** um `git status` comum por checkout antes de medir, e refazer a tabela de latência por motor. Sem isso a razão contra a V1 continua contaminada por um custo que não é do motor.
2. **Achados 2 e 4:** são falhas de progresso, não de latência. O lote que não avança e o lock sem conferência de PID deixam o hook respondendo de 300 arquivos sem aviso. Teste de regressão simples para cada um.
3. **Achado 6, `cmdIndex`:** uma linha, sem mudança de chave para quem usa caminho absoluto.
4. **Achados 5 e 7:** sem mudança de saída; validação por comparação de contexto como no PR #30.
5. **Achado 1, lado do produto, e achado 3:** mexem em como a mudança é detectada e em qual lote entra primeiro. PR próprio, com teste de arquivo alterado sem mudar `size` nem `mtime`.
6. **Semear worktree (achado 6):** só depois de medir quanto tempo de indexação fria os fluxos com worktree gastam hoje.
