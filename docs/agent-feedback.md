# ADR: observação do agente e feedback parametrizado

**Estado:** normalização schema v1 (`internal/adapter/tool.go`) e observador passivo (`sidecar observe`, não instalado por padrão) implementados com testes unitários em 2026-11-06; E2E de observação por cliente e medição de custo **pendentes**. **Separar telemetria de sessão, autoavaliação e gold independente.**

## Pergunta de produto

O Sidecar sugeriu contexto que o agente efetivamente usou? Evitou `grep` e leituras? O agente chegou antes ao trecho correto? Houve erro ou distração por contexto ruim? Aprender com isso **sem transformar o Sidecar em um agente autônomo** e sem coletar dados pessoais.

## Princípio de instrumentação

Preferir **traces de eventos determinísticos** dos hooks disponíveis, capturados de forma idêntica nas condições ON/OFF do benchmark:
- `prompt_received`, `context_candidate`, `context_injected`, `hook_skipped`, `tool_invoked`, `tool_completed`, `turn_stopped`, `session_end`.
- `tool_invoked` inclui classe da ferramenta (`search`, `read`, `edit`, `test`, `lsp`, `other`) e referencial **redigido** do path/símbolo quando permitido.
- Se o cliente não fornece todos os eventos (cloud/hosted tools), reportar `unobserved`; **não inferir ausência de busca a partir de ausência de hook**.
- Traces são locais, por sessão, TTL e tamanho limitados, IDs opacos, redaction por padrão. Nunca guardar prompt, tool output bruto ou códigos secretos.

## Hooks viáveis / gaps por cliente (precisam de E2E)

| Cliente | Eventos documentados úteis | Limites |
|---|---|---|
| Claude Code | `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `SessionEnd` | filtros/event payload variam; não tratar Stop como garantia de tarefa válida |
| Codex | `UserPromptSubmit`, `PreToolUse`, `PostToolUse`, `Stop`, `SessionEnd` | hooks requerem trust, algumas ferramentas hosted não geram hooks locais; cloud diferenciado |
| Antigravity CLI | `SessionStart`, `PreInvocation`, `PostInvocation`, `PreToolUse`, `PostToolUse`, `Stop` | outro schema; `PreInvocation` suporta `injectSteps`, mas injeção e observação precisam de E2E próprio; não converter eventos por nome |

Referências: [Claude hooks](https://code.claude.com/docs/en/hooks), [Codex hooks](https://learn.chatgpt.com/docs/hooks), [Antigravity CLI hooks](https://antigravity.google/docs/hooks).

## Esquema de evento proposto (não vigente)

```json
{
  "schema_version": 1,
  "event_id": "opaque",
  "session_id_hash": "opaque",
  "task_id_hash": "opaque",
  "repo_id_hash": "opaque",
  "client": "codex",
  "client_version": "pinned",
  "event_type": "tool_completed",
  "tool_kind": "search",
  "tool_call_id_hash": "opaque",
  "path_id_hash": "opaque",
  "monotonic_elapsed_ms": 1234,
  "context_injected_ids": ["symbol-hash"],
  "result": "ok",
  "coverage": "observed"
}
```

Campos opcionais só quando observáveis; não exigir `tool_call_id` ou precisão de tokens em todos os clientes. IDs e hashes devem ser deriváveis localmente sem expor caminho bruto. Não confundir timestamp da máquina entre sessões com monotonicidade entre processos.

## Autoavaliação do modelo — **sinal secundário, não ground truth**

A ideia de pedir ao próprio modelo “o que do Sidecar ajudou?” é interessante como instrumento **qualitativo/diagnóstico**, mas a resposta é vulnerável a pós-racionalização, concordância, confusão de fonte e incentivo à autopromoção. O modelo pode não observar completamente o hook ou o contrafactual OFF.

Proposta de formulário estruturado opcional **após** a tarefa, fora do contexto da tarefa medida, preferivelmente em avaliação/dogfooding:
```json
{
  "schema_version": 1,
  "task_completed": true,
  "context_helpfulness": "helpful",
  "helpful_evidence_ids": ["symbol-hash"],
  "misleading_evidence_ids": [],
  "additional_search_needed": true,
  "reason_code": "missing_caller",
  "confidence": "low"
}
```
Valores para `context_helpfulness`: `helpful`, `neutral`, `harmful`, `unknown`; `reason_code` pertence a enum documentada, nunca comentário arbitrário que injete instrução na indexação.

**Regras:**
1. Opt-in claro; não obrigar comentário a cada resposta do agente real, pois consome tokens e pode atrasar sua tarefa.
2. Não inserir pergunta avaliativa apenas no braço ON antes do fim da tarefa: isso contamina A/B. Se usado, mesmo protocolo de pós-tarefa em ambos os braços, com `unknown/not_applicable` quando OFF.
3. Não confiar cegamente em `helpful` ou `task_completed`; verificar automaticamente com gold/testes e revisões cegas.
4. Não ajustar pesos durante a mesma execução. A reconciliação implementada (seção abaixo) grava o feedback ao fim da tarefa e o aplica só a consultas futuras do mesmo worktree, com pesos fixos (não aprendidos) e limite de ±0,25. **Decisão em aberto:** ela aplica o feedback no ranking mesmo quando o oráculo é só autorrelato (`oracle_passed` ausente). Até existir oráculo verificado por teste ou build, trate o efeito como hipótese e avalie com `Ablation.NoFeedback` (existe na API; `sidecar-eval` ainda não expõe esse braço).
5. Nunca misturar dados de prompts privados de usuários comuns com corpus público de benchmark sem consentimento.

## Feedback forte vs fraco

| Força | Exemplo | Uso |
|---|---|---|
| Forte | teste oracle passou, símbolo gold validado, anotação humana | avaliação e possível ajuste offline |
| Médio | evidência fornecida aparece em patch/arquivo alterado | correlato; revisão antes de usar como rótulo |
| Fraco | o agente abriu arquivo, mencionou símbolo, respondeu “ajudou” | diagnóstico e triagem; não label definitivo |

Observação ≠ causalidade. Comparação causal de utilidade exige A/B com estado equivalente.

## Reconciliação determinística do feedback (implementada)

`sidecar feedback record|list|purge` grava um registro por item de evidência citado no relatório, na tabela `evidence_feedback` (schema v5 do índice). Não há LLM, rede nem aprendizado: a fórmula é fixa e reproduzível.

**Sinais e pesos** (`store.ComputeReconciledWeight`):

| Sinal | Peso | Fonte | Valores |
|---|---|---|---|
| `S_agent` | 0,20 | autorrelato, escalado pela confiança (`high` 1,0 · `medium` 0,7 · `low` 0,4) | +confiança se útil; −confiança se enganoso |
| `S_trace` | 0,35 | `SessionObserver`: o agente leu ou editou o arquivo? | útil: +1 confirmado, −0,5 contradito, 0 sem rastro. Enganoso: −1 se o rastro não confirma uso, −0,25 se leu e mesmo assim marcou |
| `S_oracle` | 0,45 | resultado da tarefa (`oracle_passed`; padrão = `task_completed`) | útil: +1 passou, −0,5 falhou. Enganoso: −1 se falhou ou exigiu nova busca, −0,6 caso contrário |

- Anti-viés: se o item é útil, mas o rastro mostra que o arquivo nunca foi lido ou editado, `S_agent` vale `0,25 ×`. Elogio sem rastro não eleva o peso.
- `raw = 0,20·S_agent + 0,35·S_trace + 0,45·S_oracle` ∈ [−1, +1]
- `delta = clamp(0,25 · raw, −0,25, +0,25)`

**Agregação por alvo.** O alvo é `path#símbolo`, ou `path` quando não há símbolo. Os registros válidos do alvo, do mais novo para o mais antigo, somam `delta_n · 0,85^n`. O total é limitado a ±0,25 e vira o multiplicador `1 + total`, que fica em [0,75; 1,25].

**Invalidação automática por hash.** Cada registro guarda o `file_hash` que o índice tinha no momento do registro. Ele só vale enquanto o arquivo continua indexado e o hash é igual ao atual, ou se foi registrado sem hash. Arquivo alterado e reindexado, ou removido, deixa o registro `stale`, e ele deixa de valer já na consulta seguinte. `purge` apaga os obsoletos; `purge --all` apaga todos do worktree.

**Aplicação no ranking.** Só com `SIDECAR_ENGINE=v2`. O multiplicador escala o score RRF do item antes da ordenação. Feedback de símbolo não rebaixa vizinhos do mesmo arquivo; feedback de arquivo vale para os símbolos sem feedback próprio. O trace marca `feedback_boost` (> 1,01) ou `feedback_penalty` (< 0,99).

**Rastro da sessão** (`trace_corroborated`). `+1` se o estado do observador registra leitura ou edição do arquivo; `−1` se a sessão tem leituras, edições ou buscas e nenhuma tocou o arquivo; `0` sem rastro (estado ausente, expirado ou de outro HEAD). Limites conhecidos: leituras são zeradas a cada edição, então um arquivo lido antes da última edição e não editado conta como `−1`; um commit durante a tarefa muda o HEAD e zera o rastro.

**Limite principal: o oráculo é autorrelato por padrão.** Sem `oracle_passed` explícito, o peso de 0,45 usa `task_completed`, que o próprio agente informou. Até o resultado de teste ou build ser informado por quem o executou, o oráculo não é independente. A fórmula só ordena evidências; ela não prova ganho de tempo, qualidade ou custo.

**CLI**
```bash
sidecar feedback record --helpful ID [--misleading ID] --reason wrong_file --confidence high --completed=true --additional-search=false
sidecar feedback record < relatorio.json        # relatório JSON com schema_version 1, sem flags de ID
sidecar feedback list --json
sidecar feedback purge [--all]
```
- `--completed` padrão é `false`; `--confidence` padrão é `medium`; `--reason` padrão é `other`.
- Valores aceitos: `reason_code` ∈ `exact_match`, `wrong_file`, `stale_context`, `missing_caller`, `noisy_neighbor`, `other`. `context_helpfulness` ∈ `helpful`, `neutral`, `harmful`, `unknown`. `confidence` ∈ `high`, `medium`, `low`. Outros valores são recusados, e nada é gravado.
- IDs de evidência são os de `sidecar explain --json` (`evidence[].id`). O stdin é limitado a 64 KiB.

**Cobertura.** Testes unitários em `internal/store/feedback_test.go`, `internal/retrieve/feedback_retrieve_test.go`, `internal/app/feedback_test.go` e `cmd/sidecar/feedback_test.go`. **Não há execução E2E com agente real.** Esta reconciliação está não comprovada quanto a tempo, qualidade ou custo.

## Experimento inicial

1. Gerar traces por adaptador sem texto sensível; validar correlação `session/turn/tool` com testes de contrato e smoke.
2. Executar replay com a mesma tarefa OFF/ON; instrumentar igualmente, medir `TTFE`, `calls_to_gold`, `rounds_to_gold`, sucesso e duração.
3. Separadamente solicitar autorrelato **depois** do encerramento e comparar concordância com verdade anotada.
4. Avaliar precisão dos sinais, memória ocupada e overhead; descartar a autoavaliação se não acrescentar informação útil.
5. Só usar feedback para futuras melhorias após revisão humana ou critério objetivo. A reconciliação automática cumpre apenas parte disso (critério objetivo fraco, ver a seção abaixo). Não compartilhar dados sem consentimento.

## Fontes

- [Claude Code hooks](https://code.claude.com/docs/en/hooks)
- [Codex hooks](https://learn.chatgpt.com/docs/hooks)
- [Antigravity CLI hooks](https://antigravity.google/docs/hooks/)

## Consolidação para a V2 completa

- Este arquivo especifica **um subsistema** do produto integrado ([product-v2.md](product-v2.md)); não é uma feature autônoma que exija outro daemon.
- `SessionObserver` deve ser **passivo** até validar, com rastreamento de `new_evidence`, `previous_evidence`, `repeated_search`, `unobserved`, `time_to_correct_used_evidence` e custo de observação.
- `LoopDiagnostics` exige anti-falsos-positivos: reler um arquivo após edição, depurar falha nova e reexecutar um teste não são necessariamente loops ruins.
- A futura `InjectionPolicy` pode sugerir evidência **nova** em evento de ferramenta somente quando o cliente comprovadamente permitir aquele retorno de hook, sem duplicar o conteúdo e sem bloquear. Caso contrário, usar próximo prompt do usuário.
- O feedback parametrizado por modelo permanece **opt-in e pós-tarefa**; o modelo não é juiz da própria eficiência e não gera rótulo definitivo para ranking.
- Referências do módulo: [Claude-Mem](https://github.com/thedotmack/claude-mem/tree/main/src/hooks), [Beads](https://github.com/gastownhall/beads), [headroom](https://github.com/headroomlabs-ai/headroom) e [catálogo de licenças](reference-implementations.md).
- Gates e cronograma: [roadmap.md](roadmap.md), rodadas C e E; nenhuma dessas funcionalidades é considerada implementada por esta documentação.
- **Contrato de registro para avaliação (2026-11-07):** `internal/benchagent` lê um `runs.jsonl` por execução com `task`, `arm`, `rep`, `resolved`, `used_ms` e `presented_ms` (ausentes = censurados, contados com o tempo de parede, nunca como zero), `tool_calls_to_gold`, `rounds_to_gold`, `contract_fidelity` e `loop_inefficiency` em [0,1], e `total_task_burden_tokens`. Campos desconhecidos são erro. O coletor por cliente que produz esses registros continua **pendente de E2E**; "evento ausente" permanece `unobserved`, nunca zero.
