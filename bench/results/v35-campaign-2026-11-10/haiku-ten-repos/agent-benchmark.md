# Benchmark de agentes reais: Sidecar ON × OFF

Fonte: `/tmp/sidecar-full-haiku/runs.jsonl` · execuções: 100 · comparáveis: 100 · não comparáveis (fora das médias): 0. O oráculo é o gold de arquivo do manifesto (o agente não o vê). Tokens, custo e chamadas vêm do próprio cliente e podem faltar (`n/d`). Transporte do braço ON neste arquivo — none: sem contexto (braço OFF); prompt_prefix: prefixo de prompt (o runner chama `sidecar hook` e prefixa o contexto ao prompt; não é o hook do cliente).

## Resumo por agente

| Agente / modelo | Braço | N | Sucesso | Hit@1 | Tempo médio (s) | Tokens in | Tokens out | Custo médio (US$) | Tool calls | Resposta malformada | Editou arquivo |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| claude / claude-haiku-5-5 | OFF | 50 | 98% | 95% | 12.1 | 64399 | 1472 | 0.0038 | n/d | 0 | 0 |
| claude / claude-haiku-5-5 | ON | 50 | 98% | 98% | 11.2 | 56879 | 1352 | 0.0035 | n/d | 0 | 0 |

## Por categoria de prompt (sucesso)

| Agente / modelo | Categoria | OFF | ON |
|---|---|---:|---:|
| claude / claude-haiku-5-5 | direct | 20/20 | 20/20 |
| claude / claude-haiku-5-5 | messy_pt | 10/10 | 10/10 |
| claude / claude-haiku-5-5 | voice_mode | 10/10 | 10/10 |
| claude / claude-haiku-5-5 | negative | 9/10 | 9/10 |

## Diferença pareada ON − OFF

Cada tarefa tem o mesmo peso: calculamos a média das repetições pareadas por tarefa e bootstrap sobre tarefas (IC 95% apenas com 3+ tarefas). Pares incompletos são reportados, não imputados.

| Agente / modelo | Pares completos | Tarefas | Sem par | Δ tempo (s) | Δ tokens in | Δ custo (US$) | Δ sucesso (pontos) |
|---|---:|---:|---:|---|---|---|---|
| claude / claude-haiku-5-5 | 50 | 50 | 0 | -0.92 (n=50 tarefas) [-1.89, +0.05] | -7520.02 (n=50 tarefas) [-14185.22, -1159.66] | -0.00 (n=50 tarefas) [-0.00, -0.00] | +0.00 (n=50 tarefas) [-6.00, +6.00] |

## Coincidências entre resposta e contexto (braço ON)

| Agente / modelo | N ON | Contexto vazio | Hook disparou | Gold no contexto | Gold ausente da resposta | Outros caminhos em comum | Caminhos inexistentes | Resposta malformada | Transporte |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| claude / claude-haiku-5-5 | 50 | 24/50 | n/a (prefixo) | 0/40 | n/d | 5/6 | 0 | 0 | prompt_prefix |

Contexto vazio significa que nenhum prefixo foi entregue; diferenças entre braços ainda podem ocorrer por variabilidade. Estas coincidências não demonstram uso, atenção ou causalidade; o gold de arquivo pode estar incompleto. Caminhos inexistentes são contados no checkout.


## Limites

- Tarefa de localização somente leitura; não mede edição de código nem testes.
- Poucos casos por categoria e repetições: sem amostra adequada não há conclusão.
- Os prompts de voz do manifesto compartilham um molde de preenchimento; não são transcrições independentes.
- Tempo de parede inclui a latência do modelo e variação de rede. No prefixo, o tempo do hook entra no tempo de parede (`sidecar_ms`). No hook real o hook roda dentro do cliente e não é separável; `hook_probe_ms` é a mediana do mesmo payload medida fora do agente.
- Preços e versões dos clientes mudam; registre-os junto do relatório.
