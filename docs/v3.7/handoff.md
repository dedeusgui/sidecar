# Passagem da V3.7: estado, bloqueio do P3 e como retomar

Documento para outro agente retomar a V3.7 sem depender do histórico desta sessão. Tudo o que está aqui foi medido ou verificado no código; o que não foi medido está marcado como tal. Números completos e racional de cada decisão: [progress.md](progress.md).

## 1. Estado

- **Na `main`:** `bdfba3e` (PR #28, CI verde). A V3.7 segue **opt-in** (`SIDECAR_ENGINE=v3.7`); a V1 continua padrão.
- **Gate P1 da V3.7 contra V3 e V3.6:** zero regressões nos fixtures internos e no corte E1; no ten-repos, duas (ver §3).
- **Gate P1 da V3.7 contra V1:** **reprova**. Nada disso foi maquiado: o gate é o mesmo do repositório (`bench/fast_gate.py`).
- **P0** (`go test`, `-race`, `golangci-lint`, testes Python, registro, freeze): verde.
- **P3 pago (agentic):** **não executado**. Bloqueado por autenticação (§2). Nenhum gasto registrado.

## 2. Bloqueio do P3: o que é, por que, e o que falta

### Sintoma
`bench/lab/lab.py run --agent claude` para no preflight `verify_agent_auth`: o CLI Claude dentro do sandbox reporta `loggedIn: false`, `authMethod: none`, e o harness recusa a execução antes de gastar qualquer coisa.

### Causa verificada
- O harness copia `--credentials` (padrão `~/.claude/.credentials.json`) para `CLAUDE_CONFIG_DIR` do sandbox. **Esse arquivo não contém o login da conta.** Só tem a chave `mcpOAuth` (tokens de servidores MCP). Verificado sem imprimir valores.
- O login da conta chega ao CLI do host pela **sessão** (variáveis de ambiente e/ou descritor de arquivo; o binário aceita `CLAUDE_CODE_OAUTH_TOKEN` e `CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR`).
- O sandbox usa `--clearenv` e não repassa essas variáveis. Por isso o CLI dentro dele não tem como se autenticar.
- Confirmado por `strace` (só caminhos, sem conteúdo): o CLI abre `/agent-config/.credentials.json` com sucesso e depois segue como não logado. Ou seja, o arquivo não traz a credencial que ele precisa.
- Descartado, com teste: versão do CLI (idêntica no host e no sandbox), rede/proxy (encaminhados corretamente), `--unshare-pid`, `--die-with-parent`, montagem em outro caminho, mascaramento de `/run` e `/etc`.

### O que foi tentado e não foi feito
- Passar o token OAuth da conta pelo ambiente do sandbox para um teste de status. **O classificador de permissões negou** essa ação (acesso não autorizado a credencial). Não foi contornada por outro caminho. **Não repetir sem permissão explícita do usuário.**
- Chave de API fictícia no ambiente, esperando que o proxy a substitua. Foi recusada pelo usuário.
- Pedido de credencial para `api.anthropic.com` pela ferramenta do ambiente: recusado pelo sistema, que trata os hosts da Anthropic como autenticados pela sessão.

### O que é preciso decidir
Uma destas, **com aprovação explícita do usuário**:
1. **Login da sessão no sandbox do piloto:** o host passa a credencial para o CLI do sandbox (variável de ambiente ou descritor), por um mecanismo aprovado e com regra de permissão para esse comando.
2. **Chave de API** fornecida pelo usuário ao ambiente do harness (o `lab.py` já repassa `ANTHROPIC_API_KEY` ao agente quando ela existe no ambiente; ver a lista de variáveis perto da linha 1174).

Depois de resolvido, o `verify_agent_auth` deve retornar `loggedIn: true` antes de qualquer execução paga.

### Plano de gasto já discutido com o usuário
- Modelo: `claude-haiku-5-5` (ID explícito; o harness rejeita aliases).
- Teto: **US$ 0,50 por execução** (`--max-budget 0.5`) e **US$ 10 no total** (`--max-total-usd 10`), com `--yes-spend`.
- Matriz: tarefas `CHI-LOC-01` (chi, split development) e `CLK-LOC-01` (click, split validation); braços `OFF`, `V1`, `V3.7`, `CG` (CodeGraph); 1 repetição. 8 execuções.

### Comando de referência (só depois de §2 resolvido)
```bash
python3 bench/lab/lab.py run --agent claude --model claude-haiku-5-5 \
  --sidecar <binário sidecar> --arms OFF,V1,V3.7,CG --codegraph /home/user/bench-tools \
  --tasks CHI-LOC-01,CLK-LOC-01 --splits development,validation --reps 1 \
  --out-dir <dir> --max-budget 0.5 --max-total-usd 10 --yes-spend
```
Antes, validar o harness sem custo: `--agent scripted` (nesta sessão, a tarefa `CHI-LOC-01` passou nos 4 braços com custo zero; `CLK-LOC-01` não entrou porque está no split validation e o corte foi development).

Repositórios das tarefas: o harness busca o commit fixado para `~/.cache/sidecar-lab`. Isso **não foi verificado de ponta a ponta** nesta sessão; o que foi verificado é a leitura anônima de repositórios públicos pelo `fetch_external.py`. Não precisa dos checkouts de `bench-external`.

## 3. Resultados medidos (commit `bdfba3e`)

Fixtures internos (40 positivos, 10 negativos simples, 36 negativos difíceis dev+validation; holdout nunca lido):

| Motor | Gold (40) | Top-3 | Tokens/gold | Negativos difíceis com saída |
|---|---:|---:|---:|---:|
| V1 (padrão) | 39 | 34 | 288 | 7 de 36 |
| V3 | 35 | 33 | 192 | 0 |
| V3.6 | 33 | 32 | 243 | 0 |
| **V3.7** | **35** | **33** | **193** | **0** |

Cortes externos (checkouts públicos no commit fixado; `fast_gate.py --external-manifest ... --checkout ...`):

| Corte | Motor | Gold | Top-3 | Tokens/gold | p95 ms |
|---|---|---:|---:|---:|---:|
| ten-repos (40 pos.) | V1 | 37 | 32 | 436 | 102 |
| | V3 | 35 | 35 | 367 | 180 |
| | V3.6 | 36 | 34 | 422 | 179 |
| | **V3.7** | **39** | **37** | **320** | 205 |
| E1 (30 pos. / 40 neg.) | V1 | 20 | 17 | 572 | 47 |
| | V3 | 20 | 17 | 407 | 66 |
| | V3.6 | 22 | 21 | 397 | 96 |
| | **V3.7** | **23** | **22** | **350** | 92 |

E1, negativos com saída: V3.7 **16/40**, V1 28, V3 19, V3.6 18.

Navegação (E2, corpus-nav, 7 tarefas / 26 fatos, CodeGraph 1.6.0): hook V3.7 cobre 14 fatos (V1 11, V3 10); `sidecar explore` 23; CodeGraph 24. Faltam `parse_args` (NAV-CLK-01) e `deserializer|from_trait` (NAV-SER-01) no `explore`.

### Gate P1 completo (último, sobre `bdfba3e`)
- Contra V3: **2 regressões** no ten-repos: `messy_pt` p95 (41 contra limite de 38 ms, 10 casos) e `voice_mode` top-3 (**9 contra 10**).
- Contra V3.6: zero.
- Contra V1: reprova em latência (p95 ~2× nos dois cortes externos e em `tune` interno) e em gold interno (35 contra 39) e no `voice_mode` do ten-repos.

## 4. Itens abertos, com diagnóstico

1. **Recall interno (35 contra 39 da V1).** Perdas: E06 e E12 (abstenção: a confiança de bloco fica abaixo de 0,5 com termos sem tradução), T08 (nenhuma evidência chega aos gates V2: `best_support 0`), E20 (contexto errado, modo LEAN), e E05 (falha em todos os motores). E03 e E13 foram corrigidas com entradas de léxico. Nota: o léxico é **ajuste em amostra** (palavras dos próprios fixtures).
2. **`voice_mode` do ten-repos, top-3 9 contra 10 da V3.** A causa de maior efeito já foi revertida: compactar trechos da V3.6 ao lado dos trechos inteiros gastava o orçamento do gold (gold 10 → 7). A expansão automática (`QuickExpand`) também piorava o top-3 (6 → 9 quando desligada). Hoje ela é opt-in. Falta uma causa para o 9 contra 10 remanescente.
3. **Latência da V3.7 ~2× a V1 nos cortes externos.** Perfil de CPU do hook quente (um prompt): V1 ~4,8 ms, V3.7 ~8,9 ms; cerca de 37% da V3.7 era `QuickExpand` (agora opt-in, mas a ablação externa não mostrou redução clara de p95). **Recompilação de SQLite (`sqlite3Reprepare`) ~26–30% do tempo nos dois motores**, inclusive a V1; o `schema_version` ficou constante, então não é mudança de schema. Hipótese de FTS5 **descartada** em microbenchmark isolado (FTS5 cacheado ~18,7 µs contra ~20,7 µs sem cache; tabela comum ~2,4 µs). **Causa não identificada.** Mexer nisso altera o caminho do motor padrão: exige PR próprio com benchmark antes e depois.
4. **Negativos E1 com saída (16/40).** Ainda é o maior vazamento da V3.7, menor que o da V1 (28).
5. **Fatos do E2:** `parse_args` e `deserializer|from_trait` no `explore`.

## 5. Decisões já tomadas (e revertidas), para não repetir

- **Compactar trechos que a V3.6 pulou por orçamento**, ao lado dos inteiros: revertida (piorou `voice_mode`, gold 10 → 7). Tinha teste unitário; o teste foi removido junto.
- **Ordenar navegação antes dos trechos de código** (flag só na V3.7): revertida. Trocou ganho de top-3 em `voice_mode` (6 → 7) por perda em `direct` (18 → 17), sem ganho líquido.
- **Ignorar palavras de estrutura em português no denominador da confiança**: revertida. +1 gold, mas custo por gold subiu para 218 e reprovou o gate.
- **Deduplicar relações repetidas no bloco**: não feito (~1% dos tokens).
- **Relações da V3.7:** no máximo uma por item, na ordem `tested_by` > `called_by` > `calls`. Contenção (`member_of`, `contains`) e `cochange` **não são exibidas**. Efeito no agente **não medido** (só o P3 responde).
- **Expansão automática (`QuickExpand`) da V3.7:** opt-in (`SIDECAR_FEATURES=quickexpand`). V3.5 e V3.6 inalteradas.
- **Léxico PT-BR→EN:** +47 entradas gerais desde `56088bf`. Ganho nos fixtures é em amostra.

## 6. Ferramentas e artefatos

- **Repositórios externos (pinados):** `python3 bench/v3.7/fetch_external.py --root <dir>` baixa os 15 repositórios no commit de cada manifesto (leitura anônima, rasa). `--verify-only` confere os commits sem rede. Nesta máquina, eles estão em `/home/user/bench-external` (todos conferidos).
- **Manifestos:** `bench/ten_repos_cases.json` (ten-repos) e `bench/v3.5/e1_cases.json` (E1). Para rodar, o `ten-repos` precisa de caminhos absolutos: gerar uma cópia do manifesto com `path` apontando para o checkout (foi feito assim nesta sessão).
- **CodeGraph 1.6.0:** instalado com `npm install --prefix <dir> @colbymchenry/codegraph@1.6.0`; nesta máquina em `/home/user/bench-tools`.
- **bwrap:** necessário ao sandbox do harness. Instalado por `apt-get install bubblewrap` (após `apt-get update`).

### Comandos de reprodução
```bash
# P0 + P1 + Studio, com cortes externos e CodeGraph (ver fast_gate --help)
python3 bench/fast_gate.py --race --engines v1,v2,v3,v3.5,v3.6,v3.7 \
  --external-manifest /home/user/bench-external/ten-manifest.json \
  --checkout /home/user/bench-external --codegraph /home/user/bench-tools/node_modules/.bin/codegraph \
  --candidate v3.7 --baseline v3 --out-dir <dir>

# E2 offline (navegação)
python3 bench/v3/e2_offline.py --sidecar <binário> --checkout /home/user/bench-external \
  --engines v1,v2,v3,v3.6,v3.7 --codegraph /home/user/bench-tools/node_modules/.bin/codegraph \
  --json <out.json> --md <out.md>
```
O `ten-manifest.json` é uma cópia de `bench/ten_repos_cases.json` com `path` absoluto (ver acima).

## 7. Armadilhas observadas (leia antes de agir)

- **Gate agregado engana.** O gate é por estrato. Em uma rodada, o agregado passou e `tune` reprovou. Sempre inspecionar `check_delivery_gate` por estrato, não só `ALL POSITIVES`.
- **Teste de caminho em `bwrap --ro-bind /`:** o diretório de montagem precisa existir; senão o erro (`Can't mkdir ... Read-only file system`) parece falha de autenticação. Sempre capturar o stderr.
- **`sleep` em segundo plano nem sempre espera o tempo real:** para aguardar CI ou pipelines, usar as ferramentas do GitHub ou um monitor com condição de saída.
- **Repositórios externos:** o ambiente da sessão restringe escrita no GitHub a `dedeusgui/sidecar`. Leitura anônima de repositórios públicos funcionou.
- **Não expor credenciais:** nunca imprimir tokens; verificar só nomes de chaves e datas.
- **O `validate-corpus` do harness reescreve `validation.json` das tarefas**: reverter com `git checkout -- bench/lab/corpus/tasks/` se rodar.
- **Léxico e ajustes por fixture são em amostra.** Qualquer ganho nos fixtures internos precisa de validação nos cortes externos.

## 8. Próximos passos, em ordem

1. Resolver o login do P3 (§2) com aprovação do usuário; validar com `--agent scripted` antes; depois rodar o piloto de 8 execuções com o teto acordado.
2. Investigar a latência contra V1 (§4.3), começando pelo armazenamento, com benchmark antes e depois em PR próprio.
3. Investigar `voice_mode` top-3 contra V3 (§4.2).
4. Investigar E06, E12, T08 e E20 (§4.1), sem ajuste em amostra.
5. Só então decidir sobre promover a V3.7 (ver critérios em `docs/engine-progression.md`).
