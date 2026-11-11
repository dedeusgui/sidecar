# Tarefa Orca — investigar fallback de evidência V3.6

> Registro histórico da tarefa delegada; não é uma instrução operacional atual. Para o estado e as lacunas, consulte [progress.md](progress.md).

## Objetivo e escopo

Investigue o item 1 de [work-plan.md](work-plan.md) no workspace atual. Leia `AGENTS.md`, `internal/app/v35.go`, `internal/app/v36.go`, `internal/engine/planner.go`, `internal/ctxbuild/packer.go` e testes pertinentes. O workspace contém alterações anteriores não commitadas: preserve-as.

Hipótese a testar: uma escolha pode ser removida porque existe uma proposta de snippet, mesmo quando a proposta não é alocada ou o snippet não é entregue. Não assuma que a hipótese é verdadeira. Reproduza primeiro com um teste determinístico que falhe no código atual e respeite os contratos reais. Se confirmada, faça a menor correção que garanta entrega de uma alternativa elegível sem duplicação e sem exceder o orçamento ou escapar do escopo. Se não for possível reproduzir, não faça uma correção especulativa: relate a evidência e a lacuna.

Arquivos permitidos para edição: `internal/app/v35.go`, `internal/app/v36.go`, `internal/app/v36_test.go` e, somente se estritamente necessário, testes de `internal/app/v35_test.go`. Não altere engine, retrieval, scorecard, documentação ou outros arquivos. Não execute benchmarks pagos, não leia nem execute holdout, não instale hooks, não faça commits, merge ou push. Use fixtures sintéticas; não ajuste regras a prompts de benchmark.

## Critérios de aceite e entrega

- Teste que demonstra o problema antes da correção, se confirmado; teste de snippet entregue sem duplicação, além do fallback não entregue.
- Respeito a orçamento, determinismo, deduplicação, sessão e escopo; não enfraquecer fail-open ou os contratos V3.5.
- Execute testes focados apropriados e relate comandos e saída resumida; um comando com saída não zero é falha, não sucesso.
- Retorne `STATUS`, hipótese e reprodução, arquivos e linhas, explicação do diff, comandos/resultados, riscos e lacunas. Não declare a V3.6 concluída. Ao terminar, use o protocolo `worker_done` do Orca.
