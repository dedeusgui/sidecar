# Benchmark de agentes reais: Sidecar ON × OFF

Fonte: `bench/results/real-hook-pilot-2026-11-08/runs.jsonl` · execuções: 20. O oráculo é o gold de arquivo do manifesto (o agente não o vê). Tokens, custo e chamadas vêm do próprio cliente e podem faltar (`n/d`). ON injeta o contexto do binário `sidecar hook` por prefixo de prompt quando há contexto; não usa o hook instalado do cliente.

## Resumo por agente

| Agente / modelo | Braço | N | Sucesso | Hit@1 | Tempo médio (s) | Tokens in | Tokens out | Custo médio (US$) | Tool calls | Resposta malformada | Editou arquivo |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| claude / claude-haiku-5-5 | OFF | 10 | 90% | 88% | 17.5 | 189152 | 1938 | 0.0060 | 3.2 | 0 | 0 |
| claude / claude-haiku-5-5 | ON | 10 | 100% | 75% | 16.5 | 144477 | 1759 | 0.0048 | 2.6 | 0 | 0 |

## Por categoria de prompt (sucesso)

| Agente / modelo | Categoria | OFF | ON |
|---|---|---:|---:|
| claude / claude-haiku-5-5 | direct | 4/4 | 4/4 |
| claude / claude-haiku-5-5 | messy_pt | 3/4 | 4/4 |
| claude / claude-haiku-5-5 | negative | 2/2 | 2/2 |

## Diferença pareada ON − OFF

Cada tarefa tem o mesmo peso: calculamos a média das repetições pareadas por tarefa e bootstrap sobre tarefas (IC 95% apenas com 3+ tarefas). Pares incompletos são reportados, não imputados.

| Agente / modelo | Pares completos | Tarefas | Sem par | Δ tempo (s) | Δ tokens in | Δ custo (US$) | Δ sucesso (pontos) |
|---|---:|---:|---:|---|---|---|---|
| claude / claude-haiku-5-5 | 10 | 5 | 0 | -0.92 (n=5 tarefas) [-2.98, +1.14] | -44675.80 (n=5 tarefas) [-69809.70, -16288.50] | -0.00 (n=5 tarefas) [-0.00, +0.00] | +10.00 (n=5 tarefas) [+0.00, +30.00] |

## Coincidências entre resposta e contexto (braço ON)

| Agente / modelo | N ON | Contexto vazio | Hook disparou | Gold no contexto | Gold ausente da resposta | Outros caminhos em comum | Caminhos inexistentes | Resposta malformada | Transporte |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| claude / claude-haiku-5-5 | 10 | 0/10 | 10/10 | 6/8 | 0/6 | 8/10 | 0 | 0 | real_hook |

Contexto vazio significa que nenhum prefixo foi entregue; diferenças entre braços ainda podem ocorrer por variabilidade. Estas coincidências não demonstram uso, atenção ou causalidade; o gold de arquivo pode estar incompleto. Caminhos inexistentes são contados no checkout.


## Limites

- Tarefa de localização somente leitura; não mede edição de código nem testes.
- Poucos casos por categoria e repetições: sem amostra adequada não há conclusão.
- Os prompts de voz do manifesto compartilham um molde de preenchimento; não são transcrições independentes.
- Tempo de parede inclui a latência do modelo e variação de rede; ON inclui o tempo do `sidecar hook` (campo `sidecar_ms`).
- Preços e versões dos clientes mudam; registre-os junto do relatório.
