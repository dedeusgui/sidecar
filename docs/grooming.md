# Task Grooming / Clarification Policy (R4)

Extensão pequena da V2, **opt-in** (`SIDECAR_FEATURES=groom`, só com `SIDECAR_ENGINE=v2`). Não é um agente nem um workflow: é uma função determinística sobre o prompt e a evidência que a recuperação já produziu. Referências estudadas (SHA, licença, arquivos, decisão de reuso: implementação independente, nada copiado): [grooming-references.md](grooming-references.md).

## 1. O que detecta (`internal/groom`)

| Estado | Evidência que o levanta | Alternativas | Pergunta? |
|---|---|---|---|
| `contract_conflict` | `ConflictCandidate` do `ContractEvidence` sobre uma definição entre as 4 primeiras evidências, com verbo de edição no pedido | seguir o código / seguir o documento, com a consequência de cada um (arquivos, testes) | sim, impacto 1,0; **não** se um valor numérico dado no pedido dispensa a escolha (ambos os lados passam a ele) |
| `ambiguous` | sem identificador exato; dois candidatos lexicais de arquivos diferentes com score ≥ 0,85× e confiança ≥ 0,5 | os candidatos, com `path:line` | sim (impacto 0,8) |
| `underspecified` | pedido introduz/altera um limite (palavra de limite, inclusive em identificadores) sem valor | "você dá o valor" / "default conservador como constante" | sim (impacto 0,7). **Não** se o repositório já tem a constante e o pedido só *adiciona* um limite (o repo responde); mudar um limite existente cita o valor atual e ainda pergunta o novo |
| `contradictory` | duas cláusulas com polaridade oposta (manter/adicionar × remover/sem) sobre um termo de conteúdo comum | — (cita as duas cláusulas) | sim (impacto 0,9) |
| `unknown` | pedido de edição sem alvo com evidência | — | **não**: informação insuficiente não vira entrevista |
| `clear` | sem lacunas, ou pedido de leitura/localização/explicação | — | não |

Vários estados podem coexistir; só a lacuna de maior valor é considerada.

## 2. Política pergunta × assume (`Decide`)

`valor = impacto × incerteza × (0,5 + 0,5·retrabalho)`; `custo = 0,30 (+0,25 se o prompt declara urgência)`; pergunta se `valor ≥ custo + 0,10`. Nunca pergunta o que o repositório responde. **Uma** pergunta por prompt, **até 2 por sessão**, e vira premissa explícita (`assume`) quando: o usuário diz "sem perguntar", a sessão não é interativa (`SIDECAR_ASK=never` ou `CI=true`) ou a cota acabou. Parâmetros **não calibrados**: são o ponto de partida do benchmark.

## 3. Injeção por cliente

Um hook não chama ferramentas nem interrompe o turno; só adiciona texto. A nota `TASK CHECK` vem **primeiro** no bloco (decisão aberta, opções com consequência, evidência `path:line`, instrução), tem limite próprio de 560 runas (a sanitização e os delimitadores do bloco continuam valendo) e trata o texto do repositório como dado.

| Cliente | Ferramenta nativa de pergunta | O que o Sidecar faz | Verificação |
|---|---|---|---|
| Claude Code | `AskUserQuestion` (existe em sessões interativas; ausente em `claude -p`) | nomeia a ferramenta, com fallback textual e "se ninguém puder responder, declare a opção assumida" | nome documentado e presente na sessão que desenvolveu isto; **E2E interativo não executado** |
| Codex | nenhuma verificada | texto: uma pergunta curta | sem ferramenta prometida |
| Antigravity CLI | nenhuma verificada | texto: uma pergunta curta | sem ferramenta prometida; E2E do cliente continua pendente (ver integrations.md) |

## 4. Respostas como decisões

O Sidecar guarda a pergunta pendente (decisão, opções, arquivo principal; sem texto do prompt) no estado da sessão por 3 turnos. Uma resposta só vira decisão se **casar sem ambiguidade com uma opção** ("B", "opção b", ou o rótulo exato; "A ou B", respostas longas ou sem opção não casam). A decisão (`origin = user_answer`, sessão, hash do arquivo, TTL de 48 h) entra na `DecisionMemory` existente: arquivo editado ⇒ `stale`, removido ⇒ `invalidated`. Nada inferido pelo agente é gravado como verdade. Respostas de uma letra não são descartadas pelo filtro de prompts curtos enquanto há pergunta pendente.

## 5. Benchmark offline (`bench/groom_cases.json`, `sidecar-eval --groom`)

28 casos no fixture interno `v2`, rótulos escritos lendo o fixture **antes** de executar o detector: 17 em que ninguém deve ser interrompido (especificados, só leitura, saudação, valor dado, repo responde, "sem perguntar", urgência com tarefa clara) e 11 com decisão aberta.

| Medida | Primeira execução (verbo "aumente" ausente do vocabulário de edição) | Após corrigir o vocabulário de verbos de alteração |
|---|---|---|
| Perguntas desnecessárias | 0/17 | **0/17** |
| Perguntas corretas | 4/11 | **8/11** (recall 73%) |
| Concordância de estado | 4/10 | 8/10 |

Erros restantes, **mantidos e não ajustados**: A06 `reduza LateFeePercent` (nenhuma palavra de limite; lacuna conhecida), A07 `deixe o frete mais barato` (sem sinal lexical), A09 `altere o cálculo do valor…` (candidatos não ficaram comparáveis). **Limites:** fixture interna escrita pelo autor do detector (regressão, não prova de utilidade); N=28; sem prompts reais; sem medida de retrabalho evitado, de tempo de espera ou de sucesso de patch.

## 6. O que NÃO foi medido (gate R4 não satisfeito)

Benefício líquido por tarefa correta (retrabalho evitado × custo de espera) exige usuário respondendo; `claude -p` não tem canal interativo e há **uma** tarefa `ambiguous` no corpus do lab. O recurso permanece **opt-in e não promovido**.
