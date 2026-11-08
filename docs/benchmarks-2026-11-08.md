# Benchmarks do Sidecar, 2026-11-08: resultado com agente real, por versão e componente

Relatório consolidado. Índice de todos os benchmarks (offline, infra e agentic) e pistas de melhoria: [benchmarks.md](benchmarks.md). Artefatos desta rodada: [bench/results/lab-bench-clean-2026-11-08](../bench/results/lab-bench-clean-2026-11-08/) (execuções, scorecard, análise de falhas, metadados).

## 1. Resumo

**Não há melhora comprovada.** Em 225 execuções reais de tarefas de edição, nenhum braço se separou de OFF nem de V1 nas métricas de qualidade e de custo de forma que sobreviva ao intervalo de confiança. A direção dos números é a esperada para o brief (menos tokens, menos tool calls, mais aprovação), mas o tamanho do efeito é pequeno e está dentro do ruído com 15 tarefas e um modelo.

| comparação | aprovação estrita | tool calls | tokens | custo | tempo* |
|---|---|---|---|---|---|
| V2+brief vs OFF | +8,6% | −4,8% | −8,8% | −4,0% | −0,5% |
| V2 vs OFF | +2,9% | −2,4% | −2,7% | −2,3% | +8,3% |
| V1 vs OFF | −5,7% | −1,0% | −1,3% | −0,4% | +20,3% |
| V2 vs V1 | +9,1% | −1,4% | −1,4% | −2,0% | −10,0% |
| V2+brief vs V2 | +5,6% | −2,5% | −6,3% | −1,7% | −8,1% |
| V2+adaptive vs V2 | −2,8% | +0,8% | +4,7% | +4,1% | −4,6% |

\* tempo de parede: três processos competiam por CPU. **Não usar como medida de velocidade** (seção 4). Todos os intervalos de confiança de 95% e leituras por métrica estão na seção 3.

## 2. Como foi medido

- **Tarefas:** 15 tarefas de edição reais (splits development e validation do corpus de 20; **holdout intocado**), em chi, click, express e sinatra, com commits fixados. Classes: locate 2, debug 4, edit 3, contract-change 3, multi-file 1, ambiguous 1, negative 1.
- **Braços:** OFF (sem Sidecar), V1, V2, V2+adaptive, V2+brief. Mesmo modelo, mesmas ferramentas, mesmo teto de custo.
- **Modelo:** `claude-haiku-5-5`, o ID efetivamente servido em todas as 225 execuções (verificado por execução).
- **Repetições:** 3 por tarefa e braço, em processos separados. Pareamento por tarefa e repetição; intervalo de confiança por bootstrap **sobre as tarefas** (repetições da mesma tarefa não são independentes).
- **Oráculo:** teste oculto rodado sem rede depois do agente, mais escopo do patch e invariantes de contrato. **Aprovação estrita** = teste + escopo + invariantes. **Testes ocultos** = só o comportamento.
- **Isolamento:** configuração do cliente nova por execução; agente sem permissão de ler o repositório de benchmark e o diretório de saída; execução invalidada se o agente tocar nesses dados. Todas as 225 foram comparáveis (nenhuma contaminada, nenhuma sem modelo servido). **Sem sandbox de SO:** código que o agente escreva e rode ainda poderia ler o gabarito (a varredura do workspace não achou nada).
- **Custo desta rodada:** US$ 1,75 (total da missão em modelos pagos ≈ US$ 2,5 de um teto de US$ 10).

## 3. Resultados

### Por braço (médias por execução, 45 execuções por braço)

| braço | aprovação estrita | testes ocultos | tool calls | tokens (mil) | custo US$ | tempo s* | contexto injetado (chars) |
|---|---:|---:|---:|---:|---:|---:|---:|
| OFF | 35/45 (78%) | 45/45 | 11.1 | 285 | 0.0078 | 33 | 0 |
| V1 | 33/45 (73%) | 45/45 | 11.0 | 281 | 0.0078 | 40 | 1268 |
| V2 | 36/45 (80%) | 45/45 | 10.8 | 277 | 0.0077 | 36 | 1541 |
| V2+adaptive | 35/45 (78%) | 45/45 | 10.9 | 290 | 0.0080 | 34 | 1527 |
| V2+brief | 38/45 (84%) | 45/45 | 10.5 | 260 | 0.0075 | 33 | 1369 |

\* tempo de parede com CPU compartilhada entre 3 processos.

### Δ% contra OFF e contra V1 (IC95 entre colchetes; ** = IC exclui zero com ≥ 10 tarefas)

Contra **OFF** (efeito de ter o Sidecar):

| braço | aprovação estrita | tool calls | tokens | custo | tempo* | chamadas redundantes |
|---|---:|---:|---:|---:|---:|---:|
| V1 | -5.7% [-29, +15] | -1.0% [-10, +9] | -1.3% [-12, +9] | -0.4% [-7, +7] | +20.3% [+1, +40] ** | -51.5% [-76, +0] |
| V2 | +2.9% [-19, +32] | -2.4% [-14, +8] | -2.7% [-14, +10] | -2.3% [-11, +8] | +8.3% [-5, +19] | -18.2% [-49, +70] |
| V2+adaptive | +0.0% [-18, +24] | -1.6% [-15, +13] | +1.9% [-10, +15] | +1.7% [-7, +11] | +3.3% [-6, +14] | -48.5% [-73, -0] |
| V2+brief | +8.6% [-5, +29] | -4.8% [-15, +5] | -8.8% [-20, +2] | -4.0% [-12, +4] | -0.5% [-10, +7] | -48.5% [-72, +6] |

Contra **V1** (efeito da V2 e dos componentes sobre a engine anterior):

| braço | aprovação estrita | tool calls | tokens | custo | tempo* | chamadas redundantes |
|---|---:|---:|---:|---:|---:|---:|
| V2 | +9.1% [-10, +42] | -1.4% [-9, +8] | -1.4% [-10, +11] | -2.0% [-8, +5] | -10.0% [-23, +1] | +68.7% [+8, +233] ** |
| V2+adaptive | +6.1% [-13, +38] | -0.6% [-9, +10] | +3.2% [-5, +14] | +2.1% [-3, +8] | -14.1% [-24, +2] | +6.3% [-33, +85] |
| V2+brief | +15.2% [+0, +46] | -3.9% [-13, +6] | -7.7% [-14, +0] | -3.7% [-9, +1] | -17.3% [-30, -5] ** | +6.3% [-54, +162] |

Contra **V2 puro** (efeito de cada componente), na seção correspondente de [scorecard.md](../bench/results/lab-bench-clean-2026-11-08/scorecard.md): brief −6,3% tokens [−16, +3], tempo −8,1% [−13, −3]**; adaptive +4,7% tokens [−3, +12], chamadas redundantes −37% [−65, −7]**.

### Por classe (aprovação estrita, execuções aprovadas / total)

| classe | tarefas | OFF | V1 | V2 | V2+adaptive | V2+brief |
|---|---:|---:|---:|---:|---:|---:|
| ambiguous | 1 | 0/3 | 0/3 | 2/3 | 1/3 | 0/3 |
| contract-change | 3 | 7/9 | 9/9 | 7/9 | 7/9 | 9/9 |
| debug | 4 | 12/12 | 9/12 | 10/12 | 10/12 | 11/12 |
| edit | 3 | 6/9 | 6/9 | 6/9 | 7/9 | 7/9 |
| locate | 2 | 6/6 | 6/6 | 6/6 | 6/6 | 6/6 |
| multi-file | 1 | 1/3 | 0/3 | 2/3 | 1/3 | 2/3 |
| negative | 1 | 3/3 | 3/3 | 3/3 | 3/3 | 3/3 |

Cada classe tem 1 a 4 tarefas: leituras por classe são anedotas, não estimativas.

## 4. O que esta rodada NÃO permite concluir

1. **Efeito de teto na capacidade.** Os testes ocultos passaram em **225 de 225** execuções, em todos os braços, inclusive OFF. O Haiku resolve o comportamento de todas as 15 tarefas; as diferenças de "aprovação estrita" vêm de **escopo e invariantes** (o agente adicionou testes ou tocou arquivos fora do permitido), não de resolver ou não o problema. **8 das 15 tarefas foram resolvidas por todos os braços em todas as repetições** e não carregam nenhum sinal. Este corpus mede eficiência e disciplina, **não ganho de capacidade**; para medir capacidade são necessárias tarefas mais difíceis.
2. **Comparações múltiplas.** São 9 pares × 7 métricas = 63 intervalos. Com 95% de confiança, ~3 resultados "significativos" aparecem por acaso. Foram 5 (V1 vs OFF tempo; V2 vs V1 redundantes; V2+brief vs V1 tempo; V2+adaptive vs V2 redundantes; V2+brief vs V2 tempo). **Isso é compatível com ruído.** Os dois efeitos de tempo estão ainda por cima contaminados por CPU compartilhada.
3. **Tempo de parede.** Três processos de benchmark competiam por 4 núcleos, mais instalação de dependências. O tempo não mede a velocidade do agente nem a do Sidecar. O custo do Sidecar em tempo vem do `sidecar perf` (hook p50 de poucos ms), não daqui.
4. **Redundância.** A média é de 0,4 a 0,8 chamadas redundantes por execução de ~11 chamadas: as diferenças percentuais saem de números absolutos minúsculos.
5. **Um modelo, 15 tarefas, 3 repetições, um fornecedor.** Nada aqui diz como um modelo mais fraco ou mais forte se comporta.
6. **Sem canal interativo.** Os braços de grooming (R4), impacto (R5) e intervenção (R6) **não foram exercidos**: precisam de um próximo prompt ou de resposta do usuário, e `claude -p` roda um turno. O braço `V2+groom`, `V2+impact` não existe nesta rodada.

## 5. Achados da análise de falhas

- **O efeito do contexto é heterogêneo por tarefa, em ambas as direções.** Por tarefa (3 repetições): CHI-CC-01 sobe de 0,33 (OFF) para 1,0 com V1/V2/brief; CHI-DBG-01 cai de 1,0 (OFF) para 0 (V1) e 0,33 (V2); CLK-CC-01 cai de 1,0 (OFF) para 0,33 (V2 e adaptive) e volta a 1,0 com o brief. Com 3 repetições essas taxas são ruidosas, mas o padrão é consistente com "contexto ajuda algumas tarefas e atrapalha outras", exatamente o que uma política seletiva (V3) precisa tratar.
- **O contexto foi injetado em prompts que não pediam código.** No prompt negativo de express ("Oi! Tudo bem? Me dá três ideias de nome… grupo de estudos sobre Node.js"), V1 e V2 injetaram contexto em **12 de 12 execuções** (886 a 1.103 caracteres), sem relação com o pedido. O agente ignorou e passou, mas pagou os tokens. É o mesmo falso positivo medido offline (B03/B04), agora com agente real.
- **A maior parte das reprovações estritas é escopo.** `escopo_so_testes_extras` responde por 33 das 48 reprovações estritas; 7 envolvem arquivo fora do código permitido (ex.: `CHANGES.md`) e 8 são invariantes quebrados. Uma política de corpus que permita testes (L05) mudaria o placar absoluto, não a comparação entre braços.
- **O contexto injetado é uma fração minúscula do consumo de tokens.** ~280 mil tokens por execução (a entrada inclui leitura de cache, então serve para comparar braços, não como custo real) contra ~1,5 mil caracteres injetados (~400 tokens). O melhor braço economiza ~25 mil tokens (≈9%). De onde vêm os outros ~280 mil não foi decomposto: é a próxima medição a fazer.

## 6. Insumos para o V3

O que os dados sustentam (e o que não):

1. **Seletividade antes de sofisticação.** Se o contexto ajuda algumas tarefas e prejudica outras, o ganho está em saber **quando injetar** (e quanto), por intenção e especificidade. A política adaptativa atual só pula contratos em pedidos de localização e não resolveu isso (adaptive vs V2: +4,7% tokens).
2. **Abster-se melhor em prompts fora de assunto**, incluindo longos e conversacionais (B03, B04 e o run real acima).
3. **Brief como base do contexto operacional**: é o único componente com direção consistente contra OFF, V1 e V2 em tokens (−6% a −9%), tool calls (−2% a −5%), custo (−2% a −4%) e aprovação estrita (+6% a +15%), mas todos os intervalos cruzam zero.
4. **Investigar onde o custo está:** o contexto injetado é ~400 tokens de ~280 mil por execução; a decomposição do resto (leituras de arquivo, histórico em cache, saída do modelo) **não foi medida por origem**. Medir isso antes de decidir, por exemplo se deduplicar leitura de arquivo já entregue ou resumir saída de teste reduz tokens.
5. **Corpus v2 mais difícil e com mais tarefas por classe** (8 de 15 tarefas não discriminam), com testes permitidos por padrão e repetições suficientes para IC útil por classe.
6. **Canal interativo real** (pergunta nativa e entrega no meio do turno) como pré-requisito para qualquer ganho de R4/R5/R6 ser medido, e sandbox de SO antes de gastar o holdout.

As pistas L01–L12 com origem, hipótese e como confirmar estão em [benchmarks.md](benchmarks.md#pistas-de-melhoria-insumos-do-v3).

## 7. Reprodução

```bash
go build -o /tmp/sidecar-bench ./cmd/sidecar
for off in 0 1 2; do python3 bench/lab/lab.py run --agent claude --model claude-haiku-5-5 --sidecar /tmp/sidecar-bench \
  --arms OFF,V1,V2,V2+adaptive,V2+brief --splits development,validation --reps 1 --rep-offset $off --seed 21 \
  --out-dir /tmp/lab-bench-r$off --max-budget 0.4 --max-total-usd 3.0 --yes-spend; done
python3 bench/lab/lab.py report /tmp/lab-bench-r*/runs.jsonl --md scorecard.md --json scorecard.json
python3 bench/lab/analyze.py /tmp/lab-bench-r*/runs.jsonl --md analysis.md
```
