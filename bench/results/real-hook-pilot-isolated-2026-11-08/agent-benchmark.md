# Benchmark de agentes reais: Sidecar ON × OFF

Fonte: `bench/results/real-hook-pilot-isolated-2026-11-08/runs.jsonl` · execuções: 10 · comparáveis: 10 · não comparáveis (fora das médias): 0. O oráculo é o gold de arquivo do manifesto (o agente não o vê). Tokens, custo e chamadas vêm do próprio cliente e podem faltar (`n/d`). Transporte do braço ON neste arquivo — none: sem contexto (braço OFF); real_hook: hook real do cliente (UserPromptSubmit registrado por `--settings`; entrega conferida no stream).

## Resumo por agente

| Agente / modelo | Braço | N | Sucesso | Hit@1 | Tempo médio (s) | Tokens in | Tokens out | Custo médio (US$) | Tool calls | Resposta malformada | Editou arquivo |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| claude / claude-haiku-5-5 | OFF | 5 | 100% | 100% | 18.7 | 186377 | 1982 | 0.0059 | 3.6 | 0 | 0 |
| claude / claude-haiku-5-5 | ON | 5 | 80% | 100% | 18.2 | 160786 | 2060 | 0.0057 | 2.6 | 0 | 0 |

## Por categoria de prompt (sucesso)

| Agente / modelo | Categoria | OFF | ON |
|---|---|---:|---:|
| claude / claude-haiku-5-5 | direct | 2/2 | 2/2 |
| claude / claude-haiku-5-5 | messy_pt | 2/2 | 2/2 |
| claude / claude-haiku-5-5 | negative | 1/1 | 0/1 |

## Diferença pareada ON − OFF

Cada tarefa tem o mesmo peso: calculamos a média das repetições pareadas por tarefa e bootstrap sobre tarefas (IC 95% apenas com 3+ tarefas). Pares incompletos são reportados, não imputados.

| Agente / modelo | Pares completos | Tarefas | Sem par | Δ tempo (s) | Δ tokens in | Δ custo (US$) | Δ sucesso (pontos) |
|---|---:|---:|---:|---|---|---|---|
| claude / claude-haiku-5-5 | 5 | 5 | 0 | -0.53 (n=5 tarefas) [-3.02, +1.94] | -25591.60 (n=5 tarefas) [-65503.40, +10221.60] | -0.00 (n=5 tarefas) [-0.00, +0.00] | -20.00 (n=5 tarefas) [-60.00, +0.00] |

## Coincidências entre resposta e contexto (braço ON)

| Agente / modelo | N ON | Contexto vazio | Hook disparou | Gold no contexto | Gold ausente da resposta | Outros caminhos em comum | Caminhos inexistentes | Resposta malformada | Transporte |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| claude / claude-haiku-5-5 | 5 | 0/5 | 5/5 | 3/4 | 0/3 | 4/5 | 0 | 0 | real_hook |

Contexto vazio significa que nenhum prefixo foi entregue; diferenças entre braços ainda podem ocorrer por variabilidade. Estas coincidências não demonstram uso, atenção ou causalidade; o gold de arquivo pode estar incompleto. Caminhos inexistentes são contados no checkout.


## Limites

- Tarefa de localização somente leitura; não mede edição de código nem testes.
- Poucos casos por categoria e repetições: sem amostra adequada não há conclusão.
- Os prompts de voz do manifesto compartilham um molde de preenchimento; não são transcrições independentes.
- Tempo de parede inclui a latência do modelo e variação de rede. No prefixo, o tempo do hook entra no tempo de parede (`sidecar_ms`). No hook real o hook roda dentro do cliente e não é separável; `hook_probe_ms` é a mediana do mesmo payload medida fora do agente.
- Preços e versões dos clientes mudam; registre-os junto do relatório.
