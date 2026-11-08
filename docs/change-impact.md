# Change Impact, Invariantes e Verificação (R5)

Duas superfícies, ambas **opt-in** e sem executar código do repositório:

- `sidecar impact [--root .] [--files a,b] [--json]`: analisa as edições não commitadas (ou `--files`) contra o índice.
- `SIDECAR_FEATURES=impact` (só V2): o hook acrescenta uma nota `Change check` ao próximo prompt quando há edições não commitadas e algo que o agente provavelmente perdeu. Limitada a 295 runas; o texto é deduplicado por sessão (hash do texto).

## O que `internal/impact` relaciona (diff-aware, limitado, determinístico)

| Achado | Origem | Status |
|---|---|---|
| `consumer`: chamador do símbolo alterado, em arquivo **não alterado** | `CallersOf` (relação por import → `CONFIRMED`; por nome único → `CANDIDATE`) | |
| `importer`: arquivo que importa o arquivo alterado e não foi alterado | imports resolvidos | `CONFIRMED` |
| `cochange`: arquivo alterado junto com o arquivo editado em ≥ 2 commits passados e não alterado agora | histórico do índice | `CANDIDATE` |
| `test`: arquivo de teste que **referencia o símbolo pelo nome**; diz se também foi alterado | `TestFilesReferencing` | `CONFIRMED` (referência, não cobertura) |
| `contract`: documento/teste afirma um valor que o código contradiz | `ContractEvidence` | **`CONFLICT`** (sem escolher autoridade) |
| `doc`: documento que nomeia o símbolo (atualizar se o comportamento mudou) | `ContractEvidence` | `CONFIRMED` |
| `gap`: símbolo exportado sem nenhum teste que o referencie (≤ 3 por arquivo) | índice | `UNKNOWN` |
| `verify`: arquivo de teste existente a rodar; para Go, `go test ./dir` como **candidato** | achados de teste | `CANDIDATE`, nunca executado |

Limites: 12 arquivos de código, 40 símbolos/arquivo, 40 achados. Arquivos removidos, sem símbolos ou que não são código vão para `skipped`. Mudança nos testes sozinha não gera achados.

## Evidência offline em histórico real (não é resultado agentic)

`sidecar-eval --impact-history REPO`: para commits reais de edição de 2–6 arquivos de código (só modificações), todos menos um arquivo representam "as edições até agora"; a pergunta é se a análise nomeia o arquivo que o mantenedor editou junto. O índice é construído num `git worktree` do commit **pai** com o histórico até ali, sem o commit que se tenta prever. A verdade vem do que os mantenedores mudaram, não de rótulos nossos. Baseline: arquivos de código nos mesmos diretórios dos editados.

| Repositório (commits / arquivos retidos) | estrutural | co-mudança | estrutural + co-mudança (mediana de arquivos sinalizados) | baseline mesmo diretório (mediana) |
|---|---|---|---|---|
| chi (40 / 101) | 19% | 39% | **46%** (4) | 71% (17) |
| click (40 / 99) | 38% | 57% | **63%** (6) | 96% (15) |
| express (19 / 43) | **0%** | 49% | **49%** (3) | 42% (5) |

**Leitura honesta:** a análise sinaliza 3–6 arquivos e perde muitos arquivos co-editados; em repositórios pequenos e planos o baseline por diretório acerta mais, mas aponta 15–17 arquivos (não é um filtro). Em express (JavaScript), o grafo de chamadas estrutural **não resolve** o padrão `res.x = function` (0%); só a co-mudança funciona. Não há comparação de precisão com intervalos nem teste de significância. Isto mede "qual arquivo o autor editou depois", não "o patch preservou o contrato" (esse é o gate de R5).

## O que NÃO foi medido (gate R5 não satisfeito)

Preservação de contratos e menos regressões fora do escopo exigem tarefas de edição com agente. A nota do hook só aparece em um **próximo prompt**; em execuções `claude -p` de um turno ela nunca dispara. A única forma de medir agora seria o agente chamar `sidecar impact` como ferramenta (braço dedicado no lab). Sem esse experimento, o recurso fica opt-in e não promovido.
