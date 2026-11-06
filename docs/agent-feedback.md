# ADR: observação do agente e feedback parametrizado

**Estado:** proposta, não implementada · 2026-11-06. **Separar telemetria de sessão, autoavaliação e gold independente.**

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
| Gemini CLI | `BeforeAgent`, `BeforeTool`, `AfterTool`, `AfterAgent`, `SessionEnd` | outro schema; exige adaptador próprio, não converter eventos por nome |

Referências: [Claude hooks](https://code.claude.com/docs/en/hooks), [Codex hooks](https://learn.chatgpt.com/docs/hooks), [Gemini hooks](https://geminicli.com/docs/hooks/).

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
  "reason_code": "missing_test_reference",
  "confidence": "low"
}
```
Valores para `context_helpfulness`: `helpful`, `neutral`, `harmful`, `unknown`; `reason_code` pertence a enum documentada, nunca comentário arbitrário que injete instrução na indexação.

**Regras:**
1. Opt-in claro; não obrigar comentário a cada resposta do agente real, pois consome tokens e pode atrasar sua tarefa.
2. Não inserir pergunta avaliativa apenas no braço ON antes do fim da tarefa: isso contamina A/B. Se usado, mesmo protocolo de pós-tarefa em ambos os braços, com `unknown/not_applicable` quando OFF.
3. Não confiar cegamente em `helpful` ou `task_completed`; verificar automaticamente com gold/testes e revisões cegas.
4. Não ajustar pesos automaticamente durante a mesma execução a partir de autorrelato; registrar como hipótese para análise offline e próxima release.
5. Nunca misturar dados de prompts privados de usuários comuns com corpus público de benchmark sem consentimento.

## Feedback forte vs fraco

| Força | Exemplo | Uso |
|---|---|---|
| Forte | teste oracle passou, símbolo gold validado, anotação humana | avaliação e possível ajuste offline |
| Médio | evidência fornecida aparece em patch/arquivo alterado | correlato; revisão antes de usar como rótulo |
| Fraco | o agente abriu arquivo, mencionou símbolo, respondeu “ajudou” | diagnóstico e triagem; não label definitivo |

Observação ≠ causalidade. Comparação causal de utilidade exige A/B com estado equivalente.

## Experimento inicial

1. Gerar traces por adaptador sem texto sensível; validar correlação `session/turn/tool` com testes de contrato e smoke.
2. Executar replay com a mesma tarefa OFF/ON; instrumentar igualmente, medir `TTFE`, `calls_to_gold`, `rounds_to_gold`, sucesso e duração.
3. Separadamente solicitar autorrelato **depois** do encerramento e comparar concordância com verdade anotada.
4. Avaliar precisão dos sinais, memória ocupada e overhead; descartar a autoavaliação se não acrescentar informação útil.
5. Só usar feedback para futuras melhorias após revisão humana ou critério objetivo. Não compartilhar dados sem consentimento.

## Integração no produto V2 completo

Esta é a estratégia de observação/feedback da trilha B/F em [roadmap.md](roadmap.md). O observador é um módulo final do produto, embora implementado incrementalmente. Toda intervenção por hook precisa ser suportada pelo **schema real** do cliente/versionamento; eventos que não suportam adicionar contexto devem apenas observar. A autoavaliação parametrizada permanece pós-tarefa e opcional, jamais obrigação que custe tokens em todas as mensagens do usuário.

Para evitar reinventar telemetria/memória, consultar [open-source-references.md](open-source-references.md) e examinar Claude-Mem/Beads e contratos de hooks oficiais em SHA pinado, preservando privacidade. A trilha D integra esse estado a `ContractResolver` e memórias de decisões **verificadas e invalidadas**; não persistir toda conversa.

Métricas adicionais da trilha completa: `navigation_revisit_rate`, `tokens_injected`, `context_used_vs_presented`, `verified_contract_conflicts`, `human_interventions`, `tool_hook_observability_coverage`, `patch_success`. Todos devem explicitar denominador, cobertura e quando o sinal é apenas inferido.

## Fontes

- [Claude Code hooks](https://code.claude.com/docs/en/hooks)
- [Codex hooks](https://learn.chatgpt.com/docs/hooks)
- [Gemini CLI hooks](https://geminicli.com/docs/hooks/)
