# Task Evidence Brief (R3)

Opt-in (`SIDECAR_FEATURES=brief`, só V2). **Sem motor novo**: `brief.Build` converte o que a recuperação já produziu — `Evidence`, `Trace.Contracts` (`ContractEvidence`), relações estruturais, `Trace.Intent` — em linhas rastreáveis dentro do mesmo bloco `[SIDECAR CONTEXT]` (orçamento, sanitização e deduplicação por sessão de `ctxbuild`). Se a evidência não bastar, o hook entrega o bloco padrão: ligar a feature nunca remove contexto.

## Campos e status

| Campo | Conteúdo | Status |
|---|---|---|
| `goal` | só a leitura de intenção do prompt (`extend+impact`, score), nunca uma paráfrase | `CANDIDATE` (o agente é dono do objetivo) |
| `entrypoint` | símbolo, `path:line`, assinatura do índice (≤110 runas). Símbolos de teste **não** são entrypoints | `CONFIRMED` só com identificador exato/`VERIFIED`; senão `CANDIDATE` |
| `contract` | doc concordante (`DOCUMENTED`) → `CONFIRMED`; divergência código×doc/teste → **`CONFLICT`** com as duas fontes, sem escolher autoridade | |
| `consumer` | chamadores/dependências; ligação resolvida por import → `CONFIRMED`, nominal (`unique_name`/`same_file`) → `CANDIDATE` | |
| `test` | arquivo de teste que referencia o símbolo **pelo nome** (fato; não afirma que verifica o comportamento) | `CONFIRMED` |
| `surface` | nº e lista de arquivos da evidência (entrada + consumidores + testes) | `CANDIDATE` (estimativa) |
| `unknown` | "nenhum teste do índice referencia X" (lacuna explícita); menção não verificada de valor | `UNKNOWN` / `CANDIDATE` |
| `verify` | arquivo de teste existente; para Go, comando candidato `go test ./dir` — **nunca executado** | `CANDIDATE` |

Limites por campo (4 entradas, 4 consumidores, 3 testes, 3 contratos, 3 lacunas, 2 verificações) e o orçamento de tokens do bloco. Menções `INFERRED` de docstring não viram linhas (ruído). Falha fechada: precisa de uma entrada `CONFIRMED` **e** de mais um fato (consumidor, teste, contrato, conflito ou lacuna); senão volta ao bloco padrão.

## Evidência offline (não é resultado agentic)

Conjuntos externos fixados (chi/click/express/…): Hit@1/Hit@5/MRR/FP **idênticos** à V2 (o brief só muda a renderização). Tokens médios do contexto injetado: positivos externos 417 → **340**, positivos longos 454 → 385. Menos tokens **não** prova economia: o bloco padrão traz assinatura e doc que o brief resume; o efeito sobre o agente é o que o piloto mede.

Bugs encontrados inspecionando saídas reais (corrigidos e cobertos por teste): símbolos de teste apareciam como entrypoint; docstrings viravam "unknown"; o brief perdia a assinatura que o bloco padrão traz.

## Piloto agentic

Ver `docs/mission-progress.md` (seção "Piloto R3"): V2 / V2+adaptive / V2+brief / OFF no corpus do lab, Haiku, N pequeno. Qualquer leitura abaixo de ~20 pares pareados é **inconclusiva**.

## Limites

- Só linguagens com símbolos/relações no índice; sem gramática, o brief cai para o bloco padrão.
- `go test` só para Go; outros ecossistemas recebem a dica genérica (arquivo de teste).
- Brief e política `adaptive` são independentes e combináveis.
