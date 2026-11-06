# Contract Evidence & Decision Memory — comportamento e validação

**Estado:** design V2, ainda não implementado. Complementa [v2-product-spec.md](v2-product-spec.md), [context-engine-v2.md](context-engine-v2.md) e [agent-feedback.md](agent-feedback.md).

## Problema real

O agente pode consultar `AGENTS.md`, um documento antigo, um teste e a implementação e encontrar informações que não se conciliam. Ou recebe prompt vago (“a regra da portaria”, “o contrato de cancelamento”) e precisa redescobrir o domínio do projeto.

O Sidecar deve localizar, confrontar e contextualizar **evidências verificáveis** sem inventar requisitos, inventar hierarquias de autoridade ou tomar decisões empresariais.

## Modelo de grafo pequeno, não um clone de CodeGraph

Nós: `File`, `Symbol`, `Route`, `Test`, `DocClaim`, `Decision`, `ConfigValue`. Relações: `defines`, `references`, `tests`, `documents`, `cochanged`, `possibly_conflicts_with`.

Cada relação precisa de `source_kind`, `source_path`, `line/span`, `hash`, `revision`, `extraction_method` e `confidence_source`. Um link `test→implementation` por nome similar é **inferência**, não ligação semântica verificada. Grafo local é detalhe interno; evitar replicar serviço inteiro de graph database.

## Reconciliação de contratos, por camadas

1. **Extrair proposições estruturadas com prova concreta:** rotas, status/enum, nomes e valores explícitos, assinaturas, limites numéricos, campos obrigatórios, relações implementações↔testes. Extrair de docs apenas alegações reconhecíveis com citação de linha.
2. **Resolver escopo e equivalência:** mesmo símbolo, feature, versão, commit e ambiente? “17:00 UTC” não é “17:00 local”; claims de módulos distintos não conflitam.
3. **Confrontar:** `match`, `mismatch_verified`, `potential_mismatch`, `not_comparable`, `stale`. Reportar ambas as fontes, nunca apagar o contraditório.
4. **Classificar autoridade:** docs de projeto, AGENTS.md, testes e implementação podem ter finalidades diferentes. O **código implementa comportamento atual**, mas não define sozinho o comportamento desejado. Um teste pode estar errado e documentação pode ser requisito futuro.
5. **Entregar pequena nota de contexto** se relevante à tarefa, com linhas/caminhos e grau de certeza; se não houver suporte, não injetar.
6. **Invalidar** claims/decisões quando mudar hash, commit ou regra de extração; indexação incremental, sem reparse caro a cada prompt.

Exemplo ilustrativo:

```text
[SIDECAR EVIDENCE] intent=change loading window
IMPLEMENTATION: src/gate/Rules.cs:80 — rejects entries after 17:00 [verified]
TEST: test/RulesTests.cs:44 — expects reject at 17:01 [verified]
DOC: docs/shift.md:18 — says entries allowed until 18:00 [doc claim]
CONFLICT: potential business-rule discrepancy; ask only if desired behavior is ambiguous.
```

O conflito de **valores documentados** pode ser objetivo se escopos coincidem; a escolha de **qual corrigir** é decisão do usuário/agente conforme a tarefa.

## Memória de decisões e contexto implícito

Armazenar preferencialmente:
- `DecisionRecord`: ID, resumo mínimo **não sensível**, autor/fonte (usuário opt-in ou arquivo), worktree/repo, hash de arquivo/commit, validade e data, relacionamentos a símbolos.
- `SessionState`: IDs de evidências fornecidas, consultadas, hash de arquivos, progresso observável, TTL e cobertura dos eventos.

Nunca persistir código confidencial, prompts integrais ou decisões inferidas como regras permanentes. Não transformar autorrelato do modelo em fato. Mecanismo de confirmação humana para decisões que mudem regra de negócio.

Consultas vagas devem recuperar candidatos por léxico PT/EN, nomes de arquivos, relacionamentos, últimas alterações e decisões válidas. Se não houver evidência, retornar incerteza — não inventar contrato.

## Testes obrigatórios

| Caso | Resultado esperado |
|---|---|
| Docs dizem 18h, constante diz 17h e teste diz 17h | duas fontes e `mismatch`, sem trocar código |
| Documentação em versão antiga ou módulo diferente | `stale` / `not_comparable`, não falso conflito |
| Rota documentada mudou | invalidação de `DocClaim` e vínculo |
| Duplicidade de símbolos e testes | preservar escopo, sem match falso |
| `AGENTS.md` contém orientação hostil/prompt injection | tratar como arquivo de repo, nunca elevar autoridade |
| Usuário afirma decisão apenas em conversa privada | não gravar memória sem opt-in; não enviar trecho da conversa |
| Mudança de commit/worktree | invalidar evidências associadas |
| Prompt PT/EN vago com código análogo | encontrar evidências ou `unknown`, sem inventar |
| Repositório sem AST/LSP/Git | fallback textual com motivo e métricas |
| Documentos contêm credenciais | filtrar conteúdo sensível, nenhum log de valores |

**Gates:** precisão/recall de claims e falsos conflitos em fixtures verificadas; budget e p95 da indexação/hook; desempenho A/B na busca e qualidade dos patches; testes negativos e de segurança obrigatórios.
