# Placar Sidecar

Ordem: 1) execuções reais de agentes, por rodada; 2) retrieval offline (secundário).

## 1. Agentes reais

### Rodada `bench/results/agent-pilot-20261108.jsonl`

| Agente | Modelo | Braço | N | Sucesso | Sem veredito | Timeouts | Tempo médio (s) | Tempo por correta (s) | Custo por correta (US$) | Custo informado (n) | Custo médio informado (US$) | Custo desconhecido (n) |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| claude | haiku | OFF | 4 | 4/4 (100%) | 0 | 0 | 9.4 | 9.4 | 0.0323 | 4 | 0.0323 | 0 |
| claude | haiku | ON | 4 | 4/4 (100%) | 0 | 0 | 9.0 | 9.0 | 0.0426 | 4 | 0.0426 | 0 |
| codex | gpt-6-luna | OFF | 4 | 4/4 (100%) | 0 | 0 | 14.0 | 14.0 | n/d | 0 | n/d | 4 |
| codex | gpt-6-luna | ON | 4 | 3/4 (75%) | 0 | 0 | 12.4 | 16.5 | n/d | 0 | n/d | 4 |

Transporte(s) registrado(s): `none`, `prompt_prefix`. Tempo/custo por correta = soma das execuções dividida pelo número de acertos; custo desconhecido ou veredito ausente torna a respectiva razão indisponível. A média de custo usa só valores informados. Descritivo: sem inferência estatística ou causal.

### Rodada `bench/results/agent-edit-infra-20261108.jsonl`

| Agente | Modelo | Braço | N | Sucesso | Sem veredito | Timeouts | Tempo médio (s) | Tempo por correta (s) | Custo por correta (US$) | Custo informado (n) | Custo médio informado (US$) | Custo desconhecido (n) |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| codex | gpt-6-luna | OFF | 1 | n/d | 1 | 0 | 173.6 | n/d | n/d | 0 | n/d | 1 |

Transporte(s) registrado(s): `none`. Tempo/custo por correta = soma das execuções dividida pelo número de acertos; custo desconhecido ou veredito ausente torna a respectiva razão indisponível. A média de custo usa só valores informados. Descritivo: sem inferência estatística ou causal.

### Rodada `bench/results/agent-edit-pilot-20261108.jsonl`

| Agente | Modelo | Braço | N | Sucesso | Sem veredito | Timeouts | Tempo médio (s) | Tempo por correta (s) | Custo por correta (US$) | Custo informado (n) | Custo médio informado (US$) | Custo desconhecido (n) |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| codex | gpt-6-luna | OFF | 1 | 1/1 (100%) | 0 | 0 | 19.7 | 19.7 | n/d | 0 | n/d | 1 |
| codex | gpt-6-luna | ON | 1 | 1/1 (100%) | 0 | 0 | 19.1 | 19.1 | n/d | 0 | n/d | 1 |

Transporte(s) registrado(s): `none`, `prompt_prefix`. Tempo/custo por correta = soma das execuções dividida pelo número de acertos; custo desconhecido ou veredito ausente torna a respectiva razão indisponível. A média de custo usa só valores informados. Descritivo: sem inferência estatística ou causal.

### Rodada `bench/results/agent-edit-v2-20261108.jsonl`

| Agente | Modelo | Braço | N | Sucesso | Sem veredito | Timeouts | Tempo médio (s) | Tempo por correta (s) | Custo por correta (US$) | Custo informado (n) | Custo médio informado (US$) | Custo desconhecido (n) |
|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| codex | gpt-6-luna | OFF | 1 | 0/1 (0%) | 0 | 0 | 11.6 | n/d | n/d | 0 | n/d | 1 |
| codex | gpt-6-luna | ON | 1 | 1/1 (100%) | 0 | 0 | 16.8 | 16.8 | n/d | 0 | n/d | 1 |

Transporte(s) registrado(s): `none`, `prompt_prefix`. Tempo/custo por correta = soma das execuções dividida pelo número de acertos; custo desconhecido ou veredito ausente torna a respectiva razão indisponível. A média de custo usa só valores informados. Descritivo: sem inferência estatística ou causal.

## 2. Retrieval offline (secundário)

Medição offline do sidecar-eval, não é resultado end-to-end com agentes.

| Papel | Rótulo | Método | Split | N pos | N neg | Checks aprovados | Hit@5 pos | Falsos positivos (neg) | Latência mediana (ms) | Tokens médios |
|---|---|---|---|---:|---:|---:|---:|---:|---:|---:|
| suite | v2 | textual (substring) | v2 | 13 | 2 | 4/15 (26.7%) | 5/13 (38.5%) | 0/2 (0.0%) | 0.2 | 17 |
| suite | v2 | bm25 (fts5, raw terms) | v2 | 13 | 2 | 5/15 (33.3%) | 6/13 (46.2%) | 0/2 (0.0%) | 0.8 | 105 |
| suite | v2 | bm25 + traducao/normalizacao | v2 | 13 | 2 | 11/15 (73.3%) | 13/13 (100.0%) | 0/2 (0.0%) | 1.2 | 198 |
| suite | v2 | lexical + grafo | v2 | 13 | 2 | 11/15 (73.3%) | 13/13 (100.0%) | 0/2 (0.0%) | 4.0 | 168 |
| suite | v2 | lexical + grafo + git | v2 | 13 | 2 | 11/15 (73.3%) | 13/13 (100.0%) | 0/2 (0.0%) | 4.9 | 172 |
| suite | v2 | v2.1 (intent router) | v2 | 13 | 2 | 11/15 (73.3%) | 13/13 (100.0%) | 0/2 (0.0%) | 3.6 | 169 |
| suite | v2 | v2 (intent + contract + packer) | v2 | 13 | 2 | 14/15 (93.3%) | 13/13 (100.0%) | 0/2 (0.0%) | 7.5 | 181 |
| suite | v2 | v2 sem intent | v2 | 13 | 2 | 14/15 (93.3%) | 13/13 (100.0%) | 0/2 (0.0%) | 8.1 | 184 |
| suite | v2 | v2 sem contract | v2 | 13 | 2 | 12/15 (80.0%) | 13/13 (100.0%) | 0/2 (0.0%) | 5.9 | 169 |
| suite | v2 | v2 sem packer | v2 | 13 | 2 | 13/15 (86.7%) | 13/13 (100.0%) | 0/2 (0.0%) | 6.5 | 181 |
| suite | dez-repos | textual (substring) | development | 40 | 10 | 16/50 (32.0%) | 11/40 (27.5%) | 7/10 (70.0%) | 7.5 | 57 |
| suite | dez-repos | bm25 (fts5, raw terms) | development | 40 | 10 | 30/50 (60.0%) | 18/40 (45.0%) | 1/10 (10.0%) | 2.3 | 214 |
| suite | dez-repos | bm25 + traducao/normalizacao | development | 40 | 10 | 39/50 (78.0%) | 36/40 (90.0%) | 9/10 (90.0%) | 3.5 | 363 |
| suite | dez-repos | lexical + grafo | development | 40 | 10 | 50/50 (100.0%) | 39/40 (97.5%) | 0/10 (0.0%) | 8.5 | 299 |
| suite | dez-repos | lexical + grafo + git | development | 40 | 10 | 50/50 (100.0%) | 39/40 (97.5%) | 0/10 (0.0%) | 8.9 | 299 |
| suite | dez-repos | v2.1 (intent router) | development | 40 | 10 | 50/50 (100.0%) | 39/40 (97.5%) | 0/10 (0.0%) | 5.5 | 296 |
| suite | dez-repos | v2 (intent + contract + packer) | development | 40 | 10 | 50/50 (100.0%) | 39/40 (97.5%) | 0/10 (0.0%) | 15.4 | 334 |
| suite | dez-repos | v2 sem intent | development | 40 | 10 | 50/50 (100.0%) | 39/40 (97.5%) | 0/10 (0.0%) | 20.9 | 337 |
| suite | dez-repos | v2 sem contract | development | 40 | 10 | 50/50 (100.0%) | 39/40 (97.5%) | 0/10 (0.0%) | 9.2 | 296 |
| suite | dez-repos | v2 sem packer | development | 40 | 10 | 50/50 (100.0%) | 39/40 (97.5%) | 0/10 (0.0%) | 13.9 | 334 |

## 3. Arquivos e hashes

| Papel | Rótulo | Tipo | Caminho | SHA256 |
|---|---|---|---|---|
| agente | - | JSONL | `bench/results/agent-pilot-20261108.jsonl` | `a00ed770a5258439292ca7ea70084e9da5e4a0b285edb40a37629630728c542b` |
| agente | - | JSONL | `bench/results/agent-edit-infra-20261108.jsonl` | `b4b20a147c0390808c3fbe13873ced07625cf98fdddad83d142d0cd067b44e71` |
| agente | - | JSONL | `bench/results/agent-edit-pilot-20261108.jsonl` | `ecf92eca011fa7c6112ae8e4d58f79af96b7ac86380ef10cd733bec3d5e15889` |
| agente | - | JSONL | `bench/results/agent-edit-v2-20261108.jsonl` | `21aa8c46e572be02c45136e3cb699f9ae1ee48b98c930d9b9a2e818dddaa6640` |
| suite | v2 | resultados | `bench/results/scorecard-v2-20261108.json` | `6648d645f75286ea2e4e23281c095a4a39e87f45a2bed5687023e8c55a65a471` |
| suite | v2 | CASES_JSON | `bench/v2_scenarios.json` | `5dafbbeb3a9742a56d14576fad232b81f9c4c4a83ac7c25bb00e016ede6a76a1` |
| suite | dez-repos | resultados | `bench/results/scorecard-ten-repos-20261108.json` | `7a5b4a9b734d25328a669e6438f85be4e240226741614c1e647ae3f09254a5de` |
| suite | dez-repos | CASES_JSON | `bench/ten_repos_cases.json` | `d1bcc0d8fc1484d3a10150a3bdacdc7245bda27f60c62ddd7c8a11f6b9d9fba9` |

CASES_JSON é fornecido pelo usuário: SHA256 igual indica o mesmo arquivo de casos, não prova de que o resultado foi gerado a partir dele.

## Limites

- CASES_JSON é fornecido pelo usuário; fixtures internas não são prova de utilidade externa.
- Deltas são diferenças observadas em uma execução: sem teste de significância, intervalo de confiança ou inferência causal.
- Checks são identificados pelo texto exato de `failed_checks`; mudança de contagem na mensagem (ex.: `gold: 0/1` → `gold: 1/2`) aparece como resolvido + novo.
- Execuções de agentes: N pequeno é descritivo; não há comparação estatística entre braços. Tempo médio inclui timeouts.
- Cada rodada pode usar tarefa, engine e transporte diferentes; consulte seu protocolo e metadados. Prefixo não é hook real.
- Tempo e custo por correta incluem apenas o que cada instrumento mediu; setup e avaliação podem estar ausentes.
- Nada é executado: sem agentes, modelos, rede ou comandos destrutivos.
