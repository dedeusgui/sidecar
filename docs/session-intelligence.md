# Session Intelligence e Loop Guardian (R6): modo passivo

Evolui o `SessionObserver` existente. **Nada intervém**: não há transporte verificado para entregar uma mensagem no meio de um turno (só `UserPromptSubmit` injeta contexto) e nenhum hook cancela ou reescreve uma chamada. O que existe é classificação passiva, decisões de intervenção em **modo sombra** (contadas, nunca entregues) e análise offline de traces reais.

## Classificação (`internal/progress`, pura e determinística)

Eventos observados (`search`, `read`, `edit`, `test`, `other`, com chave hasheada no observador) recebem um rótulo dentro da época (uma edição inicia nova época: editar→ler, editar→testar e rodar de novo o teste após o conserto são legítimos):

| Rótulo | Significado | Redundante? |
|---|---|---|
| `new` | primeira ocorrência da chave na época | não |
| `exact_repeat` | mesma busca (mesmo conjunto de termos) na época | **sim** |
| `near_repeat` | busca com Jaccard de termos ≥ 0,8 (≥ 2 termos) | **sim** |
| `revisit` | mesmo arquivo relido na época | **sim** |
| `repeated_error` | mesmo erro normalizado sem edição entre as ocorrências | **sim** |
| `iterated_error` | mesmo erro depois de uma edição | não (iteração útil) |
| `progress` | edição ou teste | não |

Janela de 8 eventos: `iterating` (há edição/teste), `stagnating` (≥ 4 redundantes e nenhum progresso) ou `exploring`. Novidade = chaves novas / eventos exploratórios. **Evento ausente é "não observado"**, nunca prova de inatividade (Codex não é observado; Claude/Codex não documentam falha em `PostToolUse`).

## Observador

`SessionSignals` ganha `Trace` (≤ 64 eventos, **só hashes**: uma busca é o conjunto ordenado de hashes de seus termos) e `Shadows`. Sinais novos, passivos: `near_repeat_search` e `stagnation`. `stagnation` é a decisão de intervenção em sombra: espaçamento mínimo de 12 eventos, máximo de 3 por sessão, `deliver` sempre falso. `PreCompact` limpa o trace (o que o agente fez antes da compactação não prevê depois dela; a deduplicação de contexto já era reiniciada). O estado nunca guarda prompt, saída de ferramenta nem texto de caminho/consulta (teste de vazamento).

## Análise de traces reais

`sidecar progress` classifica um JSONL `{kind,key,err}` com o mesmo código do observador. O lab converte o stream-json do Claude Code (`Read→read`, `Grep/Glob→search`, `Edit/Write→edit`, `Bash→test|search|read|other` por prefixo; mapeamento aproximado) e grava `tool_events` e `progress` por execução; o scorecard reporta chamadas redundantes médias e execuções com estagnação por braço.

## O que falta para o gate R6 (não satisfeito)

Reduzir chamadas redundantes **sem** perder resolução exige (1) um transporte real de intervenção e (2) E2E por cliente. Sem isso, só há medição: taxa de redundância observada por braço e a contagem de decisões em sombra (custo da observação e falsos alarmes só podem ser medidos com intervenção entregue). Resultados de simulação não são generalizados para LLM. Ver os números reais na seção de progresso.
