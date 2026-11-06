# Sidecar V2 — especificação do produto completo

**Estado:** contrato de produto e arquitetura propostos em 2026-11-06; **nenhuma funcionalidade V2 foi implementada por este documento**. Autoridade: este arquivo define o destino do V2; [roadmap.md](roadmap.md) define a sequência e os gates; ADRs existentes detalham decisões. O Sidecar v1 permanece como baseline testável e rollback.

## Proposta de valor e público

**Usuário principal:** o próprio desenvolvedor que utiliza Claude Code, Codex e Gemini CLI com modelos econômicos, orçamento limitado e repositórios reais, nem sempre bem documentados. A instalação deve ser simples; qualquer outro usuário é secundário.

**Promessa condicional, a validar:** reduzir tempo, gasto de tokens, round trips, buscas redundantes e necessidade de intervenção humana **por tarefa corretamente concluída**. O produto não melhora os pesos internos do modelo nem garante raciocínio, mas pode reduzir trabalho de descoberta e perda de contexto.

**Diferença para CodeGraph:** grafo de símbolos, busca e LSP são infraestrutura de evidências. O produto é a *eficiência operacional da interação agente↔repositório*: preparar contexto preciso antes da inferência, observar a navegação, antecipar contratos e intervir com parcimônia. CodeGraphs já possuem algumas dessas funções; superioridade é hipótese experimental, não marketing estabelecido.

## Jornada completa do usuário

1. **Instalar uma vez:** `sidecar install` registra hooks dos agentes selecionados com consentimento, backup, uninstall e diagnóstico. Sem config obrigatória por projeto, daemon, chave, MCP ou LLM.
2. **Entrar em qualquer diretório:** Sidecar resolve root/worktree, detecta linguagens, manifests, índice, ferramentas locais e convenções; em repo sem Git ou parser, usa fallback textual e reporta cobertura real.
3. **Enviar prompt PT/EN, inclusive vago:** o `IntentRouter` identifica objetivos e expande candidatos do corpus: lexical BM25, identificador/path, diff, relações sintáticas, testes, docs e LSP opcional.
4. **Receber evidências precisas:** `EvidenceEngine` valida proveniência e `ContextPacker` seleciona por ganho marginal por token. Se não houver evidência confiável, não injeta nada.
5. **Agente navega/edita:** `SessionObserver` recebe apenas eventos suportados pelos hooks, redige metadados e acompanha arquivos/evidências consultados. A cobertura observável varia por agente e superfície.
6. **Intervir apenas quando útil:** reemitir contexto **somente se o evento permitir contexto adicional** e houver evidência nova comprovada, economia estimada e orçamento; do contrário ficar silencioso. Nunca impedir tool calls.
7. **Detectar contratos/conflitos:** conectar implementação, testes, documentação e decisões técnicas referenciadas. Distinguir divergência objetiva de possível conflito; **não escolher automaticamente a verdade** nem emitir instrução autoritativa a partir de docs ou código.
8. **Concluir/continuar:** estado de sessão invalida com hashes/commits; memórias de decisões opt-in incluem fonte, escopo, validade e opção de apagar. Feedback do modelo é diagnóstico opcional, separado do sucesso validado por teste/gold.
9. **Inspecionar:** `doctor` / `explain` / atividade local mostram o que foi e não foi injetado, custo de tokens/latência e motivo; UI de agente opcional, nenhuma telemetria remota padrão.
10. **Atualizar/desinstalar:** ownership de hooks e binário, versão e migração estáveis, rollback sem corromper config de outros programas.

## Componentes lógicos — mesmo executável Go, sem microserviços

```text
Root/Environment Discovery  -> Incremental Index (existing)
              |                      |
              +----------------------v
Prompt -> IntentRouter -> CandidateSources (FTS5/exact/path/AST/Git/docs/test/LSP?)
                       -> Evidence & Contract Resolver
                       -> Confidence Gate / ContextPacker
                       -> Hook Adapter -> Agent
Tool Events ---------> SessionObserver -> Session Signals -> next relevant retrieval
Stop/Compaction -----> Session checkpoint + optional diagnostic feedback
Offline Bench --------> corpus pinado + traces + A/B scorecard
```

| Responsabilidade | Código atual de partida | Implementação final requerida |
|---|---|---|
| Repo discovery | `internal/repo`, `paths`, `gitx` | workspace/monorepo, linguagem/servidor, proteção de paths |
| Índice/evidências | `index`, `extract`, `store` | docs, testes, contracts, hashes, provenance e invalidação |
| Intenção/ranking | `lexical`, `retrieve`, `fusion` | IntentRouter com fallback; ablação v1 vs v2 |
| Render/presupuesto | `brief`, `ctxbuild` | ContextPacker, tipos e token budgeting |
| Execução/agentes | `adapter`, `app`, `install` | hooks de sessão/ações por capacidade, sem sobrescrever hooks existentes |
| Memória mínima | cache/estado atual em `app` | SessionSignals com TTL; decisões verificáveis apenas opt-in |
| LSP | hoje sem cliente LSP próprio | descoberta, cliente JSON-RPC opcional, setup com consentimento e rollback |
| Testes/bench | `cmd/sidecar-eval`, `bench/` | adapter de ARB/ContextBench, harness agentic e relatório de qualidade/custo |

**Os nomes acima são módulos/limites lógicos**: criar pacotes novos apenas quando reduzir acoplamento. Reaproveitar abstrações existentes. Nenhum hook executa código arbitrário do repositório.

## Contrato de evidência — proposta para implementação

```text
Evidence {
  evidence_id, repo_id, worktree_id, source_revision, path, span, symbol?
  source_kind: implementation|test|documentation|git|lsp|decision
  relationship: defines|calls|tests|documents|contradicts? |related
  assertion: observed_fact|inference|candidate
  observed_at, content_hash, parser_or_source_version
  intent, reason_selected, confidence_calibration_bucket?, token_estimate
}
```

`VERIFIED` só significa que o arquivo/linha/valor foi verificado no conteúdo indexado **e atual**. **Não significa** que a regra de negócio seja correta. `contradicts` requer duas proposições comparáveis extraídas com evidência objetiva; possíveis conflitos continuam como `potential_conflict`. Não divulgar schema final antes de testes de compatibilidade e migração.

## Contract & Decision Resolver: primeiro no determinístico

Prioridade de relações: definição↔teste; rota↔handler; DTO↔validação; status↔transição explícita; configuração↔uso; docs↔elementos correspondentes. Confrontar valores/assinaturas/rotas/nomes de estados quando verificáveis.

Exemplo: documentação declara janela `08:00–18:00`, constante de implementação explicita `08:00–17:00` e teste verifica `17:00`. Emitir `IMPLEMENTATION vs DOCS CONFLICT` com linhas e classificação, **sem decidir o comportamento desejado**. Sem correspondência objetiva, apontar `possible mismatch` e evidências, sem acusar inconsistência sem prova.

Para `“aquela regra da portaria”`, usar termos do prompt, símbolos existentes, sessão, diffs, relações, documentos e histórico; nunca fabricar referência sem match. Decisões de sessão/perfil de projeto são registradas somente com evento autorizado e referência verificável, respeitando escopo worktree/commit.

## Política de intervenções e orçamento de contexto

- **Injeção normal automática** não carrega uma skill de milhares de tokens nem recomenda MCP por padrão; uma skill para diagnóstico pode existir separadamente.
- `0` tokens para casos sem evidência/saudação; budget inicial experimental `100–500` tokens conforme tarefa, configurável e medido contra tokens reais quando disponível. Valores são hipóteses, não promessas.
- `PostToolUse` pode ser usado para observar; só pode **intervir** quando o schema oficial do cliente/versionamento suportar resposta que chegue ao modelo. Nunca assumir que todo evento suporta injeção.
- Cooldown e deduplicação por evidência/hash. Após compactação, reintroduzir somente decisões/evidências ainda válidas, de modo mínimo e com gate.
- Medir `net_tokens = tokens_extra_injected + tokens_extra_tool_feedback - tokens_saved_downstream`; sem leitura dos tokens reais, estimar com erro explicitado.
- Qualidade, segurança e latência de hook são guardrails: não comprar redução de buscas com respostas falsas ou decisões de negócio alteradas.

## Configuração e UX — personal use first

**Defaults sem configuração por repo:** instalação por usuário; perfil `balanced` default quando implementado; logs locais redigidos; hook fail-open; LSP desativado se não confiável/sem suporte; benchmark pago só por ação explícita.

Comandos futuros (NÃO existentes): `sidecar activity`, `sidecar bench ...`, `sidecar lsp doctor/setup`, `sidecar config ...`. Manter API documentada somente quando implementada. Nenhum prompt do usuário deve disparar instalação de dependência automaticamente sem autorização.

## Definition of Done do **produto V2 completo**

O V2 só é `complete` quando TODOS os grupos estiverem integrados e verificados:
1. Engine por intenção, seleção de evidências e abstention; rollback v1.
2. Contract/Decision Resolver (incluindo falsos conflitos), SessionObserver e memória de sessão com invalidação.
3. LSP discovery + cliente/setup opt-in com fallback, segurança e teardown; compatibilidade com hooks de agentes realmente suportados.
4. Benchmark público offline reprodutível (ARB + ContextBench conforme acessibilidade/licença) e comparação agentic OFF/v1/v2, custo, tempo e qualidade, incluindo PT/EN e tasks negativas.
5. E2E real de instalação e hooks em plataformas/clientes declarados, concorrência, segurança/privacidade, backup/rollback.
6. Docs, `doctor`, distribuição e UX sem configuração obrigatória, mais teste de dogfooding no próprio Sidecar.

Entregas por PR verticais, feature flags e validação contínua **não reduzem o escopo final**. Podemos publicar versões intermediárias de desenvolvimento, mas não declarar `V2 complete` antes da matriz integral. Resultado negativo de benchmark não pode ser escondido; se uma técnica falhar, substituir por uma alternativa fundamentada ou remover a obrigação da técnica em ADR revisada com evidência, sem falsear conclusão.

## Fontes normativas

- [roadmap.md](roadmap.md): plano de execução e testes por entrega.
- [open-source-references.md](open-source-references.md): matriz de repositórios e licença/trechos a consultar antes de implementar.
- [contract-evidence.md](contract-evidence.md): divergência entre docs/código e decisões.
- [context-engine-v2.md](context-engine-v2.md): recuperação por intenção, packing e sessão (ADR).
- [lsp-strategy.md](lsp-strategy.md), [agent-feedback.md](agent-feedback.md), [evaluation-plan.md](evaluation-plan.md): contratos especializados.
