# Voz, feedback e limites da evidência: avaliação offline

Execução em 2026-11-08, Linux amd64, Go 1.27.0, Ryzen 7 7735HS, Sidecar `main` após `2fef6fa` (a linha de base "antes" é `5e31e03`). Suíte e setup: [ten_repos_cases.json](../ten_repos_cases.json) e [setup_10_repos.sh](../agentic/setup_10_repos.sh). Relatório completo do avaliador: [ten-repos-benchmark.md](ten-repos-benchmark.md).

## O que foi e o que não foi executado

| Item | Executado? | Observação |
|---|---|---|
| Recuperação offline, 10 repositórios reais, 50 casos | **Sim** | Binário do Sidecar sobre o índice SQLite real de cada repositório. Sem modelo, sem agente. |
| Simulação agêntica OFF × V1 × V2, 6 tarefas × 5 repetições (90 runs) | **Sim, simulada** | Política fixa de `grep` + leitura (`instrumented_sim.py`, modelo `instrumented-sim`). Mede ferramentas, **não** um agente de linguagem. Resultados em [agentic-ab-report.md](agentic-ab-report.md) e [real-repos-ab-score.md](real-repos-ab-score.md). |
| Agente real (Claude Code Haiku, Codex, Antigravity CLI) em ON/OFF | **Não** | Nunca foi executado nesta suíte. Não há score, tempo de parede nem custo por cliente. |

> [!WARNING]
> Uma versão anterior deste arquivo trazia uma matriz por agente com scores 90–92 e reduções de tempo de parede. **Esses números não foram medidos e foram removidos.** Para produzir a matriz, siga [evaluation-plan.md](../../docs/evaluation-plan.md): versões e pesos pré-registrados, mesmos prompts e oráculos, ON/OFF balanceado por cliente, medição de tokens, tempo e custo reais.

Limites da suíte de 10 repositórios: os 50 casos têm um único autor, o gold é em nível de arquivo, os commits são fixos e o split é de desenvolvimento (não é holdout). Os dez prompts de voz têm 200–203 palavras e o mesmo molde de preenchimento; não são transcrições naturais independentes. Uma anotação ausente não prova que não existam outras respostas legítimas. Hit@K alto aqui indica que a regressão está controlada, não que o agente fica mais rápido.

## Recuperação offline por categoria

Método `v2 (intent + contract + packer)`; entre parênteses, o `v2.1 (intent router)`. P50/P95 são do avaliador (busca e renderização; sem indexação, hook ou modelo). Uma medição por caso. Tokens são estimativas (`runes/4`).

| Categoria | N | Hit@1 | Hit@5 | MRR | P50 ms | Máx ms |
|---|---:|---:|---:|---:|---:|---:|
| Direto EN/PT | 20 | 10/20 | 20/20 | 0,678 | 18,9 (5,8) | 37,9 (10,7) |
| PT imperfeito | 10 | 5/10 | 9/10 | 0,638 | 11,4 (4,8) | 43,7 (18,8) |
| Voz (~200 palavras) | 10 | 5/10 | 10/10 | 0,708 | 16,1 (8,0) | 30,2 (11,3) |
| Negativos (abstenção) | 10 | n/a | falso positivo 0/10 | n/a | 4,3 (2,0) | 5,6 (2,7) |

Comparação com baselines no mesmo conjunto (Hit@5): `textual (substring)` 9/20, 0/10, 1/10; `bm25 (fts5, termos crus)` 13/20, 0/10, 5/10 em direto, PT imperfeito e voz; falso positivo em negativos 7/10 e 1/10. Os 50 cenários passam nos métodos `lexical + grafo`, `v2.1` e `v2` (e nas ablações); `bm25 + traducao/normalizacao` passa 39/50 e injeta contexto indevido em 9/10 negativos.

### Antes e depois (voz e demais categorias)

| Categoria | Hit@5 antes (`5e31e03`) | Hit@5 agora |
|---|---:|---:|
| Voz | 3/10 (P50 28,5 ms, P95 59,7 ms) | 10/10 (P50 16,1 ms, máx. 30,2 ms) |
| Direto EN/PT | 17/20 | 20/20 |
| PT imperfeito | 5/10 | 9/10 |
| Negativos (falso positivo) | 2/10 | 0/10 |

Latência de voz: a mediana caiu, mas o máximo ainda passa de 20 ms, então a meta de 15 ms vale para a mediana do `v2.1`, não para todo o `v2`. O teste `TestVoicePromptMatchesCleanPromptTop1WithinLatency` usa o melhor de 6 execuções em um fixture pequeno; não substitui esta medição.

### O que mudou no código (hipóteses mecânicas, não provadas por ablação isolada)

- Para prompts com mais de 12 termos, `focalTerms` extrai a cláusula interrogativa e identificadores com cara de código; `conceptSalience` ordena por frequência efetiva entre termo e traduções. Antes, todas as palavras PT sem tradução presente empatavam no IDF máximo e o corte em 10 conceitos ficava com a saudação.
- `TermFreqs` consulta o vocabulário em lote; `gitx.Status` dobra o `HEAD` no `git status`.
- `c.nameComplete` dá a um símbolo composto traduzido (`"fuso horário"` → `TimeZone`) o mesmo reforço de um identificador exato em inglês.
- `docstringBelow` indexa docstrings Python (`"""..."""`).
- `expand` só pede membros a contêineres e deixa de consultar chamadas/testes para arquivos e seções; `pickSeeds` ignora sementes parciais quando existe um identificador exato. Isso reduz a fase estrutural, mas não foi isolado em A/B próprio.
- O dicionário em Go virou transdutor de cognatos (`internal/lexical/cognates.go`) mais léxico TSV embutido e `$SIDECAR_HOME/lexicon.json`. Cada candidato derivado só entra se existir no vocabulário FTS5 do repositório.

## Simulação agêntica (6 tarefas, 90 runs, sem modelo)

Tarefas `R01`–`R06` de [manifest.jsonl](../agentic/manifest.jsonl) em `spring-petclinic` e `go-chi/chi` (detalhes em [real-repos-ab-score.md](real-repos-ab-score.md)). Execução de 2026-11-08, médias por braço:

| Braço | Resolução | Chamadas até o gabarito | Tokens de ferramenta | TTFE (ms, sem modelo) | Nota 0–100 |
|---|---:|---:|---:|---:|---:|
| OFF | 83,3% | 19,2 | 17.772 | 96 | 85,5 |
| Sidecar V1 | 83,3% | 13,5 | 11.523 | 144 | 86,5 |
| Sidecar V2 | 83,3% | 13,5 | 11.517 | 142 | 86,5 |

Leitura honesta: a resolução é igual; o Sidecar reduz chamadas e tokens (cerca de −30% e −35% nesta rodada) e **aumenta** o tempo de ferramenta, porque `sidecar explain` custa dezenas de ms contra 1–3 ms de um `grep`. O relatório pareado marca o Sidecar como "prejudicou" em tempo de ferramenta em 4 de 6 tarefas e "ajudou" em 1. O ganho de tempo total depende de quantas idas e voltas ao modelo ele evita, o que **só um agente real mede**. Nas tarefas `R01`, `R03` e `R04`, o bloco traz o gabarito em 1 chamada; em `R02` todos os braços falham (1 de 2 itens). As tarefas são simples e poucas; isso limita qualquer conclusão sobre tarefas complexas.

## Feedback: um exemplo controlado, não ROI comprovado

Foi indexado `chi` no SHA da suíte com HOME e cache isolados ([feedback_chi.py](../agentic/feedback_chi.py)). Turno 1: `Qual middleware aplica timeout a cada requisição?` trouxe `middleware/throttle_test.go#constant:timeout@292` em primeiro e `middleware/timeout.go#function:Timeout@32` em segundo. A consulta relacionada `Onde fica o middleware de timeout para requisições?` reproduziu essa ordem. O relatório marcou o primeiro ID como enganoso e o segundo como útil (`task_completed=true`, `confidence=high`, `reason_code=noisy_neighbor`), com pesos −0,25 e +0,1625. Depois de gravar, a consulta relacionada trouxe o alvo em primeiro: Hit@1 0→1 e RR 0,5→1,0 **para este único par**, sem replicação independente.

O JSON compacto tem 273 bytes (estimativa de 69 tokens, não tokenizer real). Um processo `sidecar feedback record` inteiro levou cerca de 15 ms, incluindo inicialização, abertura do índice, gravação e saída; isso não mede o SQLite isolado. O efeito vale para o mesmo worktree e depende de `file_hash`: editar o arquivo e reindexar invalida o peso. **Não foi medida** a economia em tokens ou rodadas de um agente real, nem a frequência de reutilização; portanto não existe ponto de equilíbrio observado. Condição de retorno: a economia acumulada em buscas e leituras posteriores deve superar o custo do relatório e do processamento.

## Próximos experimentos (por hipótese de ROI)

1. **Agente real em ON/OFF.** Rodar `claude`, `codex` e `agy` nos modelos mais baratos de cada cliente, com as mesmas tarefas, oráculo somente leitura, ordem balanceada e orçamento, medindo tempo de parede, tokens, custo e sucesso. Incluir tarefas maiores que as seis atuais e prompts de voz de fontes independentes.
2. **Ablação isolada** da poda de `expand`/`pickSeeds` e de `focalTerms` nos 10 repositórios, com repetição para intervalo de incerteza de latência.
3. **Holdout cego** de repositórios e prompts escritos por outra pessoa, congelado antes de olhar resultados.
4. **Referências a estudar antes de implementar** (sem copiar GPL/BUSL; SHA, licença por componente e decisão em cada PR): Headroom (compressão reversível), Serena (corpo de símbolo), Repomix (empacotamento), CodeGraph (centralidade), Claude-Mem (divulgação progressiva) e RTK-OSS (diagnóstico de saída). São prioridades propostas, não avaliação empírica de upstream.
