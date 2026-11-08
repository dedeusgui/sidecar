# Sidecar Studio (R7)

`sidecar studio [--out sidecar-studio.html] entrada.json [entrada.jsonl ...]` gera **um arquivo HTML autocontido** a partir de saídas estruturadas que já existem. É um visualizador: sem servidor, sem rede, sem biblioteca externa, CSP `default-src 'none'`, e todo texto vindo dos dados entra no DOM como texto (nunca como HTML ou instrução). Não é injetado no prompt de nenhum agente.

| Entrada (detectada pelo conteúdo, não pelo nome) | Vista |
|---|---|
| `sidecar explain --json` | intenção, termos e traduções, **custo por fase**, candidatos **selecionados e descartados** com score/confiança/motivo, contratos, empacotamento e o **contexto realmente entregue** |
| `runs.jsonl` do lab (`lab-run/1`) | resumo por braço (aprovação, testes ocultos, custo, chamadas, chamadas redundantes), matriz tarefa × braço (veredito · custo · s), execuções excluídas/fora de escopo |
| `lab-scorecard/1` | seções por nível de evidência, comparação pareada com OFF, limites |
| `sidecar-eval --replay` (`replay/1`) | variantes, regressões contra a configuração ativa |
| `sidecar-eval --perf` (`perf/1`) | p50/p95/p99 por estágio, cold e warm |
| `sidecar-eval --groom` (`groom-eval/1`) | perguntas corretas/desnecessárias e cada caso |
| `sidecar-eval --impact-history` (`impact-history/1`) | taxas contra o baseline |
| qualquer outro JSON | visão bruta limitada |

Não cobertos nesta versão: linha do tempo do agente por sessão e respostas confirmadas (o estado do observador guarda só hashes, por privacidade; uma vista útil exige exportar um trace próprio) e comparação visual lado a lado de dois scorecards. Limites: 16 MiB por entrada, 48 MiB por página; JSON inválido é erro, nunca descartado em silêncio; um JSONL é lido linha a linha (um bug de truncar em silêncio no primeiro objeto foi pego por teste e corrigido).

Verificação: testes Go (`internal/studio`: escape de `</script>`, ausência de recursos externos e de `innerHTML`/`eval`, detecção por conteúdo, JSONL, limites) e execução em Chromium (Playwright): 7 abas sem erro de console nem requisição de rede, sem rolagem horizontal em 390 px, conteúdo hostil exibido como texto sem executar.
