# ADR: Context Engine v2 — recuperação orientada à intenção e à sessão

**Estado:** proposta algorítmica, não implementada · **Data:** 2026-11-06 · **Escopo:** subsistema de recuperação da **V2 completa**, conforme [product-v2.md](product-v2.md) e [roadmap.md](roadmap.md). Não exige novo agente.

## 1. Problema e objetivos

O Sidecar hoje utiliza FTS5/BM25, normalização PT/EN, símbolos exatos, vizinhança estrutural, sinais Git, RRF e renderização por orçamento em `internal/retrieve`, `internal/brief`, `internal/ctxbuild`. O benchmark histórico **não demonstrou ganho consistente do grafo e Git** sobre BM25 + tradução. O hook de `UserPromptSubmit` injeta evidências uma vez por prompt e evita duplicação por sessão.

Hipótese do v2: o contexto deve ser determinado pela **intenção da tarefa**, pela **novidade/informatividade das evidências** e pelo **estado da sessão**, em vez de uma única configuração estática de ranking. Benefício a validar: menos navegação redundante, menor tempo até a primeira evidência correta (TTFE) e taxa de solução preservada.

**Não objetivos:** substituir Claude/Codex/Gemini; impor convenções/AGENTS.md ao repositório do usuário; armazenar conversas completas; exigir embeddings ou modelos pagos; executar código de projeto; criar daemon pesado; afirmar ganho antes de medir.

## 2. Pipeline proposto (módulos conceituais, não novos serviços)

```text
UserPromptSubmit ─> repo.FindRoot / index.Sync
                ─> IntentRouter (determinístico; class + confidence)
                ─> CandidateGenerators [BM25/PT-EN, exato, path, diff, structural, LSP?]
                ─> Rerank/ablate + ConfidenceGate
                ─> ContextPacker (marginal utility / cost / diversity)
                ─> brief.Render + trace ─> adapter.additionalContext
                                         |
Tool hooks (opt-in, metadata only) ─> SessionSignals (local, TTL) ─┘
Stop/completion (opt-in) ─> feedback auxiliar / eval fora do runtime
```

Preservar os limites atuais de CPU, budget de sincronização e fail-open; passagem de dados não confiáveis nunca cria autoridade instrucional.

## 3. IntentRouter (componente determinístico sem LLM)

Classes (multirrótulo opcional, com fallback `general`):
- `locate`: encontrar definição/regra/rota; lexical/exato/path.
- `impact`: “quem usa”, “o que quebra”; references, testes e dependências.
- `debug`: mensagem de erro/stack trace; caminhos, assinaturas, implementação e teste.
- `extend`: “como criar outro”, padrão análogo + testes próximos.
- `test`: “onde validar”; `code2test` e mapeamento de testes.
- `explain`: entender fluxo; preferir entrypoints → chamadas → saída.

Classificação por **sinais observáveis** (termos, estrutura do prompt, stack trace, caminhos, símbolos, comando de teste), determinística e auditável, sem inventar semântica. Saída sugerida:
```go
// Contrato proposto — não implementado.
type Intent struct {
    Kinds []string
    Confidence float64
    Signals []string
}
```
Com baixa confiança, executar pipeline atual em vez de arriscar erro de roteamento. Medir macro-F1 de intenção e **qualidade downstream**, não só acerto da classificação. Evitar heurísticas “palavra → decisão rígida” para frases ambíguas.

## 4. Geração, expansão e ranking

Conservar FTS5/BM25 + normalização como baseline forte. Tratar sinais em camadas:
1. Candidate generation: identificadores exatos, caminhos, títulos/rotas, texto, Git/diff; limite de candidatos e IO.
2. Expansão: *somente* relações autorizadas pela intenção; ex.: `impact` privilegia callers/references, `test` privilegia test links. Não expandir grafo por padrão só por existir.
3. Integração semântica opcional com LSP: definitions/references/document symbols quando servidor disponível, inicializado, autorizado e dentro do timeout; ver `docs/lsp-strategy.md`.
4. Fusion/rerank: manter RRF como controle, medir ablações; scores de fontes heterogêneas **não são somáveis** sem calibração.
5. Gate de confiança: `no_evidence` é resultado válido. Calibrar em consultas negativas e ambíguas; medir dano de falso positivo.

Antes de otimizar score, avaliar estratos: sem correspondência lexical, nomes PT/EN, símbolos duplicados, monorepo, linguagem sem parser, sem Git, snippets desatualizados e mudanças locais.

## 5. ContextPacker — evidências por utilidade marginal

Objetivo: selecionar pequeno conjunto diversificado sob limite de tokens, não simplesmente top-K bruto.

Abordagem inicial: greedy/MMR com `utility(e | selected)` estimada por relevância calibrada, cobertura de conceitos novos, diversidade de arquivos/funções, complemento entre implementação/teste e **custo de tokens**. Estabilidade e ausência de vazamento são obrigatórias; não introduzir pesos de produção sem ablação.
- Reservar cabeçalho de proveniência (caminho relativo, símbolo, linhas, verdict VERIFIED/INFERRED).
- Penalizar trechos repetidos, vizinhos sem relação comprovada, contexto antigo ou já injetado na sessão.
- Limitar evidência por arquivo, tamanho de blocos e orçamento total; resultados de baixa confiança não preenchem orçamento só por estar disponível.
- Emitir trace com `reason_selected`, `reason_suppressed`, `token_estimate`, `confidence` e `source` sem armazenar texto do prompt.
- Se `token_estimate` divergir do contador real do cliente, registrar diferença quando possível; não prometer precisão de tokens usando a heurística 4 chars/token.

## 6. Estado da sessão, sem “memória mágica”

Camada mínima e privativa, isolada por `repo + worktree + agent + session`:
- `evidence_sent` (IDs+hash+versão), `files_seen` opcional, `symbols_visited` opcional, `last_retrieval`, contagem de navegação e IDs de mudança.
- Receber de hooks apenas **metadados normalizados**: operação de busca/leitura/edição, paths **relativos** permitidos, hash ou IDs de evidência, timestamps monotônicos quando disponíveis, outcome e um identificador opaco da chamada. Nunca persistir output completo de ferramenta/segredos.
- TTL e limite de tamanho; criar opt-in separado para observação de ações. Sessões OFF e ON em benchmark precisam de telemetria **simétrica**, sem dar dicas extras à condição OFF.
- Observação não é julgamento: arquivo lido ≠ arquivo correto; evidência ignorada ≠ erro. Não aprender pesos diretamente da telemetria sem gold/revisão.

O estado deve invalidar resultados quando `commit`, `hash do arquivo`, `extractVersion`, worktree ou configuração mudarem. Coexistir com memory/AGENTS do agente, sem copiá-los para outra memória de conversação.

## 7. Loop de engenharia sugerido

`hypothesis → fixture pública pinada → baseline → implementação mínima → ablação → replay OFF/ON → análise estratificada → aceitar/reverter → docs`.

Plano de mudança incremental:
- V2.0: acrescentar **trace** de estratégias, IDs, supressão e seleção (sem mudar ranking).
- V2.1: IntentRouter por feature flag + matriz de intents; default ao ranking legado.
- V2.2: ContextPacker novo por feature flag; gate e objetivo por token.
- V2.3: memória de sessão com sinais não invasivos e TTL.
- V2.4: LSP opcional em repositórios com ganho comprovado.
- V2.5: feedback auxiliar e relatório externo (sem retreinamento silencioso).

Cada etapa requer benchmark offline independente e verificação de latência/segurança. O algoritmo deve poder ser desativado ou revertido sem migrar dados do usuário irreversivelmente.

## 8. RepoFingerprint, contratos e conflitos integrados

A V2 **não é apenas IntentRouter + contexto**. Ela deve ligar recuperação a [ContractEvidence/ConflictResolver](product-v2.md) e às fontes observáveis de projeto:
- `RepoFingerprint` descobre linguagens, entrypoints, manifests, testes, convenções e estruturas sem impor padrões; sinais têm proveniência e hash de invalidação.
- `ContractEvidence` vincula entidades do código, expectations de testes, documentos e decisões verificadas. Classificar `IMPLEMENTED`, `TEST_EXPECTED`, `DOCUMENTED`, `INFERRED` e `CONFLICT_CANDIDATE` sem escolher automaticamente um vencedor.
- Dar prioridade a contradições que podem ser **verificadas deterministicamente** (endpoint, assinatura, enum, constante, assertion, tipo/rota) e tratar contradições em linguagem natural como possíveis conflitos.
- `EvidencePacker` não pode transformar frase de documentação desatualizada em instrução autoritativa. Deve preservar ambas as evidências e sinalizar incerteza.
- Os testes precisam de docs corretas/incorretas, alterações de branch, contratos divergentes, falsos conflitos e patches que passam no oracle. Retornar vazio se não houver prova suficiente.

Fontes para estudar antes de implementar: [catálogo OSS](reference-implementations.md) (CodeGraph, SolidLSP/Serena, Superpowers), com licença por componente.

## 9. Sistema completo sem aumento obrigatório de contexto

O [plano da V2 completa](product-v2.md) adiciona `SessionObserver`, `LoopDiagnostics`, `DecisionMemory`, `LSP provider`, docs externas opt-in, adapters/UX e testes agentic. Todos devem usar os mesmos IDs/provenance, hashes e políticas de confiança da recuperação. Estar documentado **não significa implementado**.

Nenhuma skill grande nem MCP obrigatório no fluxo cotidiano. Observar ferramenta não implica injetar; cada nova injeção exige informação nova, confiança adequada e budget. Não introduzir prompts “opine sobre Sidecar” durante tarefa principal.

## 10. Riscos e questões abertas

- Ganho em Hit@5 não garante queda de rounds nem correção de patch.
- Adicionar informação a cada ferramenta pode piorar o loop por ruído; preferir observação silenciosa primeiro.
- Traces nem sempre representam todas as chamadas (ferramentas hospedadas, modo cloud, subsistemas internos); ausência de sinal significa **não observável**.
- Um LSP pode executar componentes do projeto/abrir rede; isolamento e consentimento são necessários.
- Classificador pode superajustar benchmark; manter teste cego intocado até versão congelada.
- Se o v2 não vencer BM25 + tradução e Sidecar v1 no teste cego, preferir reduzir complexidade.

**Leitura obrigatória complementar:** `docs/evaluation-plan.md`, `docs/lsp-strategy.md`, `docs/agent-feedback.md`, `docs/benchmark.md`.
