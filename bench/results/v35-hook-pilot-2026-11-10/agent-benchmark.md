# Benchmark de agentes reais: Sidecar ON × OFF

Fonte: `/tmp/sidecar-v35-pilot-isolated/runs.jsonl` · execuções: 10 · comparáveis: 10 · não comparáveis (fora das médias): 0. O oráculo é o gold de arquivo do manifesto (o agente não o vê). Tokens, custo e chamadas vêm do próprio cliente e podem faltar (`n/d`). Transporte do braço ON neste arquivo — none: sem contexto (braço OFF); real_hook: hook real do cliente (UserPromptSubmit registrado por `--settings`; entrega conferida no stream).

## Resumo por agente

| Agente / modelo | Braço | N | Sucesso | Hit@1 | Tempo médio (s) | Tokens in | Tokens out | Custo médio (US$) | Tool calls | Resposta malformada | Editou arquivo |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| claude / claude-haiku-5-5 | OFF | 5 | 80% | 75% | 16.7 | 83522 | 2258 | 0.0054 | 3.0 | 0 | 0 |
| claude / claude-haiku-5-5 | ON | 5 | 100% | 75% | 15.7 | 110575 | 1894 | 0.0062 | 3.6 | 0 | 0 |

## Por categoria de prompt (sucesso)

| Agente / modelo | Categoria | OFF | ON |
|---|---|---:|---:|
| claude / claude-haiku-5-5 | direct | 2/2 | 2/2 |
| claude / claude-haiku-5-5 | messy_pt | 2/2 | 2/2 |
| claude / claude-haiku-5-5 | negative | 0/1 | 1/1 |

## Diferença pareada ON − OFF

Cada tarefa tem o mesmo peso: calculamos a média das repetições pareadas por tarefa e bootstrap sobre tarefas (IC 95% apenas com 3+ tarefas). Pares incompletos são reportados, não imputados.

| Agente / modelo | Pares completos | Tarefas | Sem par | Δ tempo (s) | Δ tokens in | Δ custo (US$) | Δ sucesso (pontos) |
|---|---:|---:|---:|---|---|---|---|
| claude / claude-haiku-5-5 | 5 | 5 | 0 | -0.96 (n=5 tarefas) [-4.14, +2.23] | +27052.60 (n=5 tarefas) [-2947.40, +55459.40] | +0.00 (n=5 tarefas) [-0.00, +0.00] | +20.00 (n=5 tarefas) [+0.00, +60.00] |

## Coincidências entre resposta e contexto (braço ON)

| Agente / modelo | N ON | Contexto vazio | Hook disparou | Gold no contexto | Gold ausente da resposta | Outros caminhos em comum | Caminhos inexistentes | Resposta malformada | Transporte |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| claude / claude-haiku-5-5 | 5 | 0/5 | 5/5 | 2/4 | 0/2 | 2/3 | 0 | 0 | real_hook |

Contexto vazio significa que nenhum prefixo foi entregue; diferenças entre braços ainda podem ocorrer por variabilidade. Estas coincidências não demonstram uso, atenção ou causalidade; o gold de arquivo pode estar incompleto. Caminhos inexistentes são contados no checkout.


## Limites

- Tarefa de localização somente leitura; não mede edição de código nem testes.
- Poucos casos por categoria e repetições: sem amostra adequada não há conclusão.
- Os prompts de voz do manifesto compartilham um molde de preenchimento; não são transcrições independentes.
- Tempo de parede inclui a latência do modelo e variação de rede. No prefixo, o tempo do hook entra no tempo de parede (`sidecar_ms`). No hook real o hook roda dentro do cliente e não é separável; `hook_probe_ms` é a mediana do mesmo payload medida fora do agente.
- Preços e versões dos clientes mudam; registre-os junto do relatório.
