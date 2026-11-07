# Voz e feedback: avaliação exploratória offline

Execução em 2026-11-07 UTC, Sidecar `5e31e03`, Linux amd64, Go 1.27.0, Ryzen 7 7735HS. A suíte e o setup estão em [ten_repos_cases.json](../ten_repos_cases.json) e [setup_10_repos.sh](../agentic/setup_10_repos.sh); relatório do avaliador em [ten-repos-benchmark.md](ten-repos-benchmark.md). São dez repositórios reais, 50 casos anotados por um único autor, gold em nível de arquivo e commits fixos. Os casos são de desenvolvimento, não holdout; uma anotação ausente de gold não prova que não existem outras respostas legítimas. Nenhum modelo ou agente foi executado. O corpus usa dez prompts de voz de 200–203 palavras, com o mesmo molde de preenchimento, e não transcrições naturais independentes.

## Voz versus pergunta curta

Métricas da segunda execução offline, método `v2 (intent + contract + packer)`, índices já construídos pelo avaliador antes das consultas, uma medição por caso. P50/P95 são nearest-rank, incluem busca e renderização medidos pelo avaliador, mas não indexação, hook nem tempo de modelo. Tokens são estimativas de contexto emitido (`runes/4`), não tokens do prompt nem contagem de um tokenizer de modelo.

| Grupo | N | Hit@1 | Hit@5 | MRR | P50 ms | P95 ms | Contexto médio estimado |
|---|---:|---:|---:|---:|---:|---:|---:|
| Voz | 10 | 1/10 | 3/10 | 0,200 | 28,48 | 59,68 | 345 |
| Curto PT pareado por alvo (`-2`) | 10 | 4/10 | 7/10 | 0,537 | 12,41 | 32,04 | 368 |
| Direto EN e PT | 20 | 8/20 | 17/20 | 0,561 | 18,28 | 33,98 | 407 |
| PT imperfeito | 10 | 3/10 | 5/10 | 0,367 | 11,05 | 36,31 | 253 |

Voz falhou em 7/10 alvos no top 5 e as dez latências medidas excederam 20 ms. O limite de dez conceitos existe, mas esta amostra não confirma que mantenha a latência abaixo de 20 ms nem que evite diluição; não se mediu uma ablação sem limite, portanto o mecanismo da perda ainda é hipótese. O tempo varia com carga e corpus, e p95 com dez valores é o máximo. Negativos: 2/10 retornaram contexto indevido no v2; ver detalhe por caso no relatório. Nenhuma alteração de ranking foi feita com base nesta suíte.

## Feedback: um exemplo controlado, não ROI comprovado

Foi indexado `chi` no SHA da suíte em HOME/cache isolados. Turno 1: `Qual middleware aplica timeout a cada requisição?` trouxe `middleware/throttle_test.go#constant:timeout@292` em primeiro e `middleware/timeout.go#function:Timeout@32` em segundo. Antes do feedback, a consulta relacionada `Onde fica o middleware de timeout para requisições?` reproduziu essa ordem. O relatório marcou o primeiro ID como enganoso e o segundo como útil, com `task_completed=true`, `confidence=high`, `additional_search_needed=true`, `reason_code=noisy_neighbor`; os registros tiveram pesos -0,25 e +0,1625. Após gravar, a consulta relacionada trouxe o alvo em primeiro: Hit@1 0→1 e RR 0,5→1,0 para **este único par**, sem replicações independentes de tarefa ou oráculo de resolução.

O JSON compacto usado tem 273 bytes e 273 caracteres ASCII, portanto a estimativa do Sidecar é 69 tokens; não medimos tokens reais de um tokenizer. Um processo `sidecar feedback record` inteiro levou 15,129 ms; uma repetição isolada pelo script levou 14,869 ms. Isso inclui inicialização, abertura do índice, reconciliação, gravação e saída: **não mede a latência do SQLite isolada** e não valida <2 ms. Reproduzir com `go build -o /tmp/sidecar-real-bench/sidecar ./cmd/sidecar` e `python3 bench/agentic/feedback_chi.py /tmp/sidecar-real-bench/sidecar /tmp/sidecar-real-bench/chi <diretorio-vazio>`; o script gera seus dados locais e não altera o repositório externo.

O efeito é restrito ao mesmo worktree e aos alvos candidatos; o código compara hashes com o índice, então a invalidação depende de reindexação após alteração. Os testes de `internal/retrieve/feedback_retrieve_test.go` e `internal/store/feedback_test.go` cobrem invalidação, mas esta experiência não editou arquivos nem mediu isso. A condição de retorno econômico é: economia acumulada em tokens de buscas/leitura posteriores, convertida pelo preço efetivo, mais economia de tempo valorizada, deve exceder o custo do relatório e do processamento. Não se mediram os supostos 1.500–8.000 tokens economizados nem a frequência de reutilização; portanto não há ponto de equilíbrio observado.

## Matriz de agentes

| Cliente/modelo pretendido | Qualidade | Velocidade | Tokens e custo | Score 0–100 |
|---|---|---|---|---|
| Claude Code / Haiku | não medido | não medido | não medido | indisponível |
| Codex / Mini ou Standard | não medido | não medido | não medido | indisponível |
| Antigravity CLI / Flash | não medido | não medido | não medido | indisponível |

Os dados de recuperação são comuns ao algoritmo e não distinguem clientes ou modelos; preços, versões, instrumentos e pesos do score não foram fixados. Um placar ou custo monetário a partir deles seria inventado. Para produzir a matriz, pré-registrar versões e pesos, executar os mesmos prompts e oráculos em ON/OFF balanceado por cliente, medir TTFE usado, sucesso, tokens e custo efetivo, e incluir censuras e variância conforme [evaluation-plan.md](../../docs/evaluation-plan.md).

## Próximos experimentos determinísticos, em ordem de hipótese de ROI

1. **Seleção de termos de voz com preservação da intenção.** Comparar distilação atual com extração de frases relevantes, controle de vocabulário e fallback por abstinência. Medir pares curtos/voz independentes, recall, falsos positivos e latência; a referência de compressão reversível do Headroom é uma fonte para estudar fidelidade, não prova de ganho.
2. **Seleção e orçamento de evidência com expansão sob demanda.** Investigar fatiamento de corpo de símbolo (Serena), compactação e deduplicação (Repomix) e divulgação progressiva (Claude-Mem). Avaliar snippet versus arquivo e qualidade de tarefa, incluindo o custo de reabrir contexto omitido; cuidado com a licença GPL da aplicação Serena.
3. **Ranking estrutural seletivo e diagnóstico de saída.** Avaliar centralidade/in-degree inspirados em CodeGraph apenas com ablação de latência, vizinhos e precisão por linguagem; estudar filtragem RTK-OSS apenas como diagnóstico opt-in, preservando erros. Não assumir que centralidade ou compressão melhoram sucesso do agente. RTK tem componentes de licença distinta.

Estas são prioridades propostas a partir do catálogo local, não avaliação empírica de upstream nem plano de integração: antes de implementar qualquer módulo, consultar código e testes das até duas referências pertinentes, registrar SHA, licença por componente e comparar o efeito medido.
