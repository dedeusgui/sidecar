# Integrações e visibilidade do Sidecar

**Estado:** hooks Claude Code e Codex implementados; Claude Code E2E relatado no README; Codex E2E real **pendente**. Este documento distingue integração existente, verificação e ideias de UX.

## Contrato comum dos hooks

O Sidecar lê um evento `UserPromptSubmit` do stdin, identifica o repositório e escreve contexto somente quando houver evidência. O bloco é dado auxiliar, não nova instrução do sistema. Não pode bloquear agente por falha, executar o código do projeto nem imprimir telemetria no stdout.

- **Claude Code:** formato `hookSpecificOutput.additionalContext`; testar versão real do CLI, stdout limpo, sem contexto, falha, timeout, caminhos com espaços e hooks coexistentes. Fonte: [Claude Code hooks](https://code.claude.com/docs/en/hooks).
- **Codex:** `~/.codex/hooks.json` e `UserPromptSubmit`; hooks não gerenciados precisam de **aprovação/trust** via `/hooks`. Testar evento, schema, saída, aprovação, novo hash após update, recusa e desinstalação. Fonte: [Codex hooks](https://learn.chatgpt.com/docs/hooks).
- **Antigravity CLI:** adaptador e instalador implementados (`sidecar install --agent antigravity`, evento `PreInvocation`, contexto por `injectSteps`), com teste unitário; **E2E com o CLI real pendente**. Contrato usado, da documentação oficial de 2026-11-06: prompt em `lastUserInput`, sessão em `conversationId`, `workspacePaths`, `invocationNum`; hooks em grupos nomeados no topo do `hooks.json`, e o Sidecar usa o grupo `sidecar`. `invocationNum` 0 como primeira chamada do turno é uma suposição não verificada. O modelo Gemini Flash pode ser usado no benchmark **via Antigravity CLI**, fixando versão do CLI e do modelo. A CLI possui eventos `SessionStart`, `PreInvocation`, `PostInvocation`, `PreToolUse`, `PostToolUse` e `Stop`, mas o Sidecar ainda precisa validar em E2E sua injeção e telemetria. Hooks globais: `~/.gemini/config/hooks.json`; por workspace: `.agents/hooks.json`. A CLI **não carrega** hooks de `~/.gemini/antigravity-cli/settings.json`. Ver [hooks oficiais do Antigravity CLI](https://antigravity.google/docs/hooks).

## Evolução prevista: observação do loop e LSP

- `UserPromptSubmit` continua sendo a **única integração que injeta contexto**. Observação de ferramentas é opt-in (`sidecar install --observe-tools`): `PreToolUse`/`PostToolUse` no Claude Code e `PreTool`/`PostTool` no Antigravity CLI, somente metadados, sem alterar a chamada. Não há observação de `Stop`/encerramento nem de Codex. Reinstalar sem a flag não remove observadores já instalados; `sidecar uninstall` remove todos. O evento `PreCompact` é aceito por `sidecar observe`, mas nenhum instalador o registra ainda. Custo de hook de ferramenta com agente real **não medido**. Contrato e riscos em [agent-feedback.md](agent-feedback.md).
- Claude Code **já suporta** plugins oficiais de inteligência de código (LSP) com `gopls`, `jdtls`, `pyright`, `typescript-language-server`, `rust-analyzer` etc., com binário separado. Também tem recomendação nativa de plugin quando identifica servidor disponível. Sidecar não precisa inventar um MCP de LSP nem reinstalar o mesmo servidor automaticamente. Fonte: [Code intelligence plugins](https://code.claude.com/docs/en/plugins/code-intelligence).
- A consulta LSP **dentro do Sidecar** para o ranking inicial é uma funcionalidade diferente de o agente possuir sua própria ferramenta LSP. Requer cliente próprio/integração documentada, confiança e timeout. Estratégia em [lsp-strategy.md](lsp-strategy.md).
- A documentação oficial de [mods do Claude Code](https://code.claude.com/docs/en/plugins/mods/create) permite extensões JS/TS e alterações de UI. Um mod de atividade pode ser opcional, versionado e sem substituir hooks do Sidecar; requer versão/consentimento/superfície suportada. Não prometer funcionamento em cloud nem auto-instalação pelo hook.
- Codex documenta `PreToolUse`, `PostToolUse`, `Stop` e limites para ferramentas hospedadas; **Antigravity CLI** usa `SessionStart`, `PreInvocation`, `PostInvocation`, `PreToolUse`, `PostToolUse` e `Stop`, com schema específico e possibilidade documentada de `injectSteps` em `PreInvocation`. Usar o contrato oficial específico da Antigravity CLI, sem presumir equivalência com outros clientes. Adapters e injeção precisam de smoke/E2E individuais. Referências: [Codex hooks](https://learn.chatgpt.com/docs/hooks), [Antigravity CLI hooks](https://antigravity.google/docs/hooks).
- Uma instalação **nova** do Sidecar não pode ser oferecida por hook de um Sidecar ausente. O primeiro prompt só pode detectar linguagens, sugerir LSP ou adaptar contexto **depois** que o Sidecar já está instalado; qualquer download/instalação requer ação autorizada fora do caminho crítico.

## E2E mínimo antes de prometer compatibilidade

1. Preparar usuário/configuração temporária sem hooks preexistentes; depois repetir **com** hooks do usuário.
2. Instalar Sidecar, verificar mudanças previstas, backup e manifest. Iniciar CLI real e conferir execução do hook.
3. Enviar consulta cujo `gold` é conhecido; capturar evidência de que o contexto alcançou a entrada do modelo (não apenas `hook.jsonl`).
4. Repetir com consulta sem evidência, falha do indexador, caminho Unicode/espaços e repositório sem parser.
5. No Codex, cobrir `/hooks` e confiança antes/depois de upgrade. Testar reinstalação, exclusão apenas de entradas próprias e isolamento de config.
6. Registrar cliente/versão/SO/branch/SHA, comandos e logs redigidos. Não guardar transcript sensível no repositório.

## Visibilidade opcional: primeira interface recomendada

Problema: hoje `sidecar doctor` e `sidecar explain` dão diagnóstico, mas o usuário não vê facilmente se o prompt corrente **recebeu** evidência. Proposta incremental:

**Fase A — diagnóstico local primeiro (sem UI):**
- **Implementado:** `sidecar status --json` e `sidecar activity [--json] [--limit N]`, leitura do log local do hook. Nenhum prompt, erro ou stack aparece na saída; o log guarda só um digest da sessão. Estados: `injected`, `no_evidence`, `skipped`, `partial`, `failed`, com p50/p95 e contagem de evidências.
- Status com `active / skipped / partial / failed`, timestamp do último hook, tipo de agente, contagem de evidências, bytes/tokens estimados e latência. Nunca mostrar texto do prompt, conteúdo de arquivos ou caminhos sensíveis por padrão.
- Atualização atômica, retenção curta, operação segura e sem backpressure no hook. Validar precisão quando não houve injeção.

**Fase B — Claude Code status line opt-in:**
- **Implementado:** `sidecar statusline` imprime `Sidecar: injected · N refs · Xms` ou `Sidecar: no match`. Quando o Claude Code envia `session_id` no stdin, mostra a última execução dessa sessão. O comando nunca falha e não bloqueia: a leitura de stdin tem limite de 300 ms.
- **Opt-in:** `sidecar install --statusline` grava `statusLine` `{"type":"command","command":"<binário> statusline"}` em `settings.json`, com backup e registro no manifesto. Uma linha de status de outra ferramenta **nunca** é substituída: o instalador a mantém e imprime o trecho manual. `sidecar uninstall` remove a linha só se ela ainda for exatamente a do Sidecar. Schema conferido na [documentação oficial da status line](https://code.claude.com/docs/en/statusline) em 2026-11-06.
- Limite: a status line de Claude Code atualiza por eventos. Não testamos ainda uma sessão real do Claude Code exibindo a linha; o teste cobre a saída do comando e a escrita em `settings.json`.
- Antes de escrever `statusLine`, o instalador mostra o plano e pede consentimento (`--yes` ou resposta interativa). Não substitui statusline existente, em nenhum caso.
- Uninstall reverte somente mudanças do Sidecar. Fallback somente CLI quando statusline indisponível.
- Referência oficial: [Claude Code custom status line](https://code.claude.com/docs/en/statusline). O suporte a “mods”/UI in-process varia por versão e documentação de terceiros; **pesquisar e provar em teste E2E** antes de apostar em API específica. A status line documentada é o caminho mais estável no momento.

**Fase C — plugin/mod, somente depois de validação:**
- Plugin opcional pode empacotar status line/skills e facilitar descoberta; não impor Node.js nem duplicar hooks. Compatibilidade de versões, comportamento em desktop/terminal, conflitos de statusline e rollback são gates.
- Não usar UI para atribuir sucesso de task, apenas atividade do Sidecar e transparência de contexto.

## Perguntas em aberto

- Eventos de hook/client API mantêm compatibilidade nos modos cloud, desktop e CLI?
- Como expor estado por **sessão**, sem vazar prompt e sem ambiguidade entre múltiplos agentes simultâneos?
- O usuário prefere atividade discreta ou nenhum UI? Medir o uso.
- Se um gerenciador de pacotes controla o binário, quem faz update dos paths do hook?

**Rodada E (2026-11-07):** nenhum hook, adaptador ou instalador foi alterado. Os estados acima permanecem como estavam; o E2E de Codex e Antigravity CLI continua pendente.


## Matriz de suporte verificada (2026-11-08)

| Cliente | `UserPromptSubmit` (contexto) | Observação de ferramenta | `PreCompact` | Pergunta nativa (R4) | Evidência |
|---|---|---|---|---|---|
| Claude Code 2.1.319 | **E2E real**: hook instalado, disparado, contexto entregue | **E2E real**: `PreToolUse`/`PostToolUse` com metadados hasheados; falha de ferramenta não documentada (`result: unknown`) | não registrado pelo instalador; **não verificado** | `AskUserQuestion` (interativo; ausente em `-p`); **E2E interativo não executado** | [r7-install-e2e](../bench/results/r7-install-e2e-2026-11-08.md) |
| Codex | só testes de contrato | não observado | — | nenhuma ferramenta verificada (texto) | CLI não disponível neste ambiente |
| Antigravity CLI | só testes de contrato (`PreInvocation`, `injectSteps`) | `PreTool`/`PostTool` por contrato | — | nenhuma ferramenta verificada (texto) | CLI não disponível neste ambiente |

Cloud × local: o ambiente de nuvem usado aqui carrega hooks do `CLAUDE_CONFIG_DIR` e não tem `bwrap`; permissões (`--allowedTools`, `acceptEdits`) e sandbox de SO continuam responsabilidade do cliente. Fallback: qualquer falha do Sidecar deixa o prompt seguir sem contexto (stdout limpo, log local sem prompt).
