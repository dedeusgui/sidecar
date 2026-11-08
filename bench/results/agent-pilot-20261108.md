# Benchmark de agentes reais: Sidecar ON × OFF

Fonte: `bench/results/agent-pilot-20261108.jsonl` · execuções: 16. O oráculo é o gold de arquivo do manifesto (o agente não o vê). Tokens, custo e chamadas vêm do próprio cliente e podem faltar (`n/d`). ON injeta o contexto do binário `sidecar hook` por prefixo de prompt quando há contexto; não usa o hook instalado do cliente.

## Resumo por agente

| Agente / modelo | Braço | N | Sucesso | Hit@1 | Tempo médio (s) | Tokens in | Tokens out | Custo médio (US$) | Tool calls | Resposta malformada | Editou arquivo |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| claude / haiku | OFF | 4 | 100% | 100% | 9.4 | 37100 | 497 | 0.0323 | n/d | 0 | 0 |
| claude / haiku | ON | 4 | 100% | 100% | 9.0 | 31650 | 562 | 0.0426 | n/d | 0 | 0 |
| codex / gpt-6-luna | OFF | 4 | 100% | 100% | 14.0 | 82815 | 390 | n/d | 2.5 | 0 | 0 |
| codex / gpt-6-luna | ON | 4 | 75% | 100% | 12.4 | 62071 | 248 | n/d | 1.8 | 0 | 0 |

## Por categoria de prompt (sucesso)

| Agente / modelo | Categoria | OFF | ON |
|---|---|---:|---:|
| claude / haiku | direct | 1/1 | 1/1 |
| claude / haiku | messy_pt | 1/1 | 1/1 |
| claude / haiku | voice_mode | 1/1 | 1/1 |
| claude / haiku | negative | 1/1 | 1/1 |
| codex / gpt-6-luna | direct | 1/1 | 1/1 |
| codex / gpt-6-luna | messy_pt | 1/1 | 1/1 |
| codex / gpt-6-luna | voice_mode | 1/1 | 1/1 |
| codex / gpt-6-luna | negative | 1/1 | 0/1 |

## Diferença pareada ON − OFF

Cada tarefa tem o mesmo peso: calculamos a média das repetições pareadas por tarefa e bootstrap sobre tarefas (IC 95% apenas com 3+ tarefas). Pares incompletos são reportados, não imputados.

| Agente / modelo | Pares completos | Tarefas | Sem par | Δ tempo (s) | Δ tokens in | Δ custo (US$) | Δ sucesso (pontos) |
|---|---:|---:|---:|---|---|---|---|
| claude / haiku | 4 | 4 | 0 | -0.37 (n=4 tarefas) [-2.33, +2.44] | -5450.25 (n=4 tarefas) [-12671.50, +8182.75] | +0.01 (n=4 tarefas) [+0.00, +0.02] | +0.00 (n=4 tarefas) [+0.00, +0.00] |
| codex / gpt-6-luna | 4 | 4 | 0 | -1.67 (n=4 tarefas) [-6.57, +5.72] | -20744.00 (n=4 tarefas) [-62644.00, +27752.50] | n/d | -25.00 (n=4 tarefas) [-75.00, +0.00] |

## Coincidências entre resposta e contexto (braço ON)

| Agente / modelo | N ON | Contexto vazio | Hook disparou | Gold no contexto | Gold ausente da resposta | Outros caminhos em comum | Caminhos inexistentes | Resposta malformada | Transporte |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| claude / haiku | 4 | 1/4 | n/a (prefixo) | 3/3 | 0/3 | 3/3 | 0 | 0 | prompt_prefix |
| codex / gpt-6-luna | 4 | 1/4 | n/a (prefixo) | 3/3 | 0/3 | 1/3 | 0 | 0 | prompt_prefix |

Contexto vazio significa que nenhum prefixo foi entregue; diferenças entre braços ainda podem ocorrer por variabilidade. Estas coincidências não demonstram uso, atenção ou causalidade; o gold de arquivo pode estar incompleto. Caminhos inexistentes são contados no checkout.


## Limites

- Tarefa de localização somente leitura; não mede edição de código nem testes.
- Poucos casos por categoria e repetições: sem amostra adequada não há conclusão.
- Os prompts de voz do manifesto compartilham um molde de preenchimento; não são transcrições independentes.
- Tempo de parede inclui a latência do modelo e variação de rede; ON inclui o tempo do `sidecar hook` (campo `sidecar_ms`).
- Preços e versões dos clientes mudam; registre-os junto do relatório.
