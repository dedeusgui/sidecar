# Integrações e visibilidade do Sidecar

**Estado:** hooks Claude Code e Codex implementados; Claude Code E2E relatado no README; Codex E2E real **pendente**. Este documento distingue integração existente, verificação e ideias de UX.

## Contrato comum dos hooks

O Sidecar lê um evento `UserPromptSubmit` do stdin, identifica o repositório e escreve contexto somente quando houver evidência. O bloco é dado auxiliar, não nova instrução do sistema. Não pode bloquear agente por falha, executar o código do projeto nem imprimir telemetria no stdout.

- **Claude Code:** formato `hookSpecificOutput.additionalContext`; testar versão real do CLI, stdout limpo, sem contexto, falha, timeout, caminhos com espaços e hooks coexistentes. Fonte: [Claude Code hooks](https://code.claude.com/docs/en/hooks).
- **Codex:** `~/.codex/hooks.json` e `UserPromptSubmit`; hooks não gerenciados precisam de **aprovação/trust** via `/hooks`. Testar evento, schema, saída, aprovação, novo hash após update, recusa e desinstalação. Fonte: [Codex hooks](https://learn.chatgpt.com/docs/hooks).
- **Gemini CLI:** *não integrado ao produto neste momento*. O modelo Gemini Flash pode participar de um benchmark com infraestrutura própria, mas o protocolo deve registrar se o cliente consegue consumir contexto Sidecar. Não simular equivalência com hook nativo.

## Evolução prevista: observação do loop e LSP

- `UserPromptSubmit` continua sendo a **única integração operacional atual**. A arquitetura v2 observa `PostToolUse`/equivalentes e `Stop`/encerramento, apenas com metadados redigidos e permissão explícita; **não implementado**. Contrato e riscos em [agent-feedback.md](agent-feedback.md).
- Claude Code **já suporta** plugins oficiais de inteligência de código (LSP) com `gopls`, `jdtls`, `pyright`, `typescript-language-server`, `rust-analyzer` etc., com binário separado. Também tem recomendação nativa de plugin quando identifica servidor disponível. Sidecar não precisa inventar um MCP de LSP nem reinstalar o mesmo servidor automaticamente. Fonte: [Code intelligence plugins](https://code.claude.com/docs/en/plugins/code-intelligence).
- A consulta LSP **dentro do Sidecar** para o ranking inicial é uma funcionalidade diferente de o agente possuir sua própria ferramenta LSP. Requer cliente próprio/integração documentada, confiança e timeout. Estratégia em [lsp-strategy.md](lsp-strategy.md).
- A documentação oficial de [mods do Claude Code](https://code.claude.com/docs/en/plugins/mods/create) permite extensões JS/TS e alterações de UI. Um mod de atividade pode ser opcional, versionado e sem substituir hooks do Sidecar; requer versão/consentimento/superfície suportada. Não prometer funcionamento em cloud nem auto-instalação pelo hook.
- Codex documenta `PreToolUse`, `PostToolUse`, `Stop` e limites para ferramentas hospedadas; Gemini CLI usa `BeforeTool`, `AfterTool` e `AfterAgent`. Adapters e eventos precisam de smoke/E2E individuais. Referências: [Codex hooks](https://learn.chatgpt.com/docs/hooks), [Gemini CLI hooks](https://geminicli.com/docs/hooks/).
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
- Projetar `sidecar status --json` ou `sidecar activity --json` como leitura local barata; **não existem no CLI atual**.
- Status com `active / skipped / partial / failed`, timestamp do último hook, tipo de agente, contagem de evidências, bytes/tokens estimados e latência. Nunca mostrar texto do prompt, conteúdo de arquivos ou caminhos sensíveis por padrão.
- Atualização atômica, retenção curta, operação segura e sem backpressure no hook. Validar precisão quando não houve injeção.

**Fase B — Claude Code status line opt-in:**
- Exibir indicação discreta, por exemplo `Sidecar: injected · 4 refs · 19ms`; se não houver evidência exibir `no match`, não `active/injected`.
- Antes de escrever `statusLine` existente, apresentar preview e pedir consentimento; preferir integração não invasiva/composição ou oferecer snippet manual. Não substituir statusline do usuário silenciosamente.
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
