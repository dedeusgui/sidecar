# E1 — v3 (relevance gate + effort router) vs lexical + grafo + git

Holdout incluído: não

| Suíte | Método | FP/neg | Hit@1 | Hit@5 | MRR | Positivos vazios | Tokens pos | Tokens neg | p50 ms | p95 ms |
|---|---|---|---|---|---|---|---|---|---|---|
| ten | lexical + grafo + git | 0/10 | 52.5% | 97.5% | 0.7144 | 0/40 | 373.4 | 0 | 10.88 | 21.59 |
| ten | v3 (relevance gate + effort router) | 0/10 | 52.5% | 97.5% | 0.7108 | 0/40 | 299.1 | 0 | 11.94 | 64.05 |
| hn | lexical + grafo + git | 72/180 | — | — | — | 0/0 | None | 126.2 | 0.55 | 10.49 |
| hn | v3 (relevance gate + effort router) | 14/180 | — | — | — | 0/0 | None | 14.4 | 2.97 | 16.98 |
| long | lexical + grafo + git | 4/6 | 56.2% | 87.5% | 0.6844 | 0/16 | 406.2 | 238.3 | 10.54 | 15.73 |
| long | v3 (relevance gate + effort router) | 1/6 | 56.2% | 87.5% | 0.6844 | 0/16 | 339.9 | 28.3 | 15.57 | 27.01 |
| ind | lexical + grafo + git | 24/40 | 56.7% | 83.3% | 0.6809 | 0/30 | 381.2 | 211.1 | 8.36 | 14.87 |
| ind | v3 (relevance gate + effort router) | 9/40 | 56.7% | 83.3% | 0.6775 | 0/30 | 293.7 | 37.2 | 12.21 | 29.6 |

## ten: 0 melhorias, 1 regressões
- regressed `R03-3` (development, positive): hit5 False->False rr 0.143->0.000 empty False->False

## hn: 58 melhorias, 0 regressões
- improved `HN-dev-02@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-03@hono` (dev, negative): FP True -> False
- improved `HN-dev-03@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-03@monolog` (dev, negative): FP True -> False
- improved `HN-dev-03@dapper` (dev, negative): FP True -> False
- improved `HN-dev-04@click` (dev, negative): FP True -> False
- improved `HN-dev-04@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-04@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-05@spring-petclinic` (dev, negative): FP True -> False
- improved `HN-dev-05@chi` (dev, negative): FP True -> False
- improved `HN-dev-05@click` (dev, negative): FP True -> False
- improved `HN-dev-05@hono` (dev, negative): FP True -> False
- improved `HN-dev-05@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-05@sinatra` (dev, negative): FP True -> False
- improved `HN-dev-05@monolog` (dev, negative): FP True -> False
- improved `HN-dev-05@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-05@dapper` (dev, negative): FP True -> False
- improved `HN-dev-06@spring-petclinic` (dev, negative): FP True -> False
- improved `HN-dev-06@chi` (dev, negative): FP True -> False
- improved `HN-dev-06@hono` (dev, negative): FP True -> False
- improved `HN-dev-06@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-06@dapper` (dev, negative): FP True -> False
- improved `HN-dev-06@express` (dev, negative): FP True -> False
- improved `HN-dev-07@spring-petclinic` (dev, negative): FP True -> False
- improved `HN-dev-07@chi` (dev, negative): FP True -> False
- improved `HN-dev-07@click` (dev, negative): FP True -> False
- improved `HN-dev-07@hono` (dev, negative): FP True -> False
- improved `HN-dev-07@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-07@sinatra` (dev, negative): FP True -> False
- improved `HN-dev-07@monolog` (dev, negative): FP True -> False
- improved `HN-dev-07@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-07@dapper` (dev, negative): FP True -> False
- improved `HN-dev-07@express` (dev, negative): FP True -> False
- improved `HN-dev-08@spring-petclinic` (dev, negative): FP True -> False
- improved `HN-dev-08@chi` (dev, negative): FP True -> False
- improved `HN-dev-08@click` (dev, negative): FP True -> False
- improved `HN-dev-08@hono` (dev, negative): FP True -> False
- improved `HN-dev-08@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-08@sinatra` (dev, negative): FP True -> False
- improved `HN-dev-08@monolog` (dev, negative): FP True -> False
- improved `HN-dev-08@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-08@dapper` (dev, negative): FP True -> False
- improved `HN-validation-02@chi` (validation, negative): FP True -> False
- improved `HN-validation-02@click` (validation, negative): FP True -> False
- improved `HN-validation-02@hono` (validation, negative): FP True -> False
- improved `HN-validation-02@sinatra` (validation, negative): FP True -> False
- improved `HN-validation-02@express` (validation, negative): FP True -> False
- improved `HN-validation-03@kotlinx-datetime` (validation, negative): FP True -> False
- improved `HN-validation-05@chi` (validation, negative): FP True -> False
- improved `HN-validation-05@click` (validation, negative): FP True -> False
- improved `HN-validation-05@hono` (validation, negative): FP True -> False
- improved `HN-validation-05@serde-json` (validation, negative): FP True -> False
- improved `HN-validation-05@sinatra` (validation, negative): FP True -> False
- improved `HN-validation-05@kotlinx-datetime` (validation, negative): FP True -> False
- improved `HN-validation-05@dapper` (validation, negative): FP True -> False
- improved `HN-validation-05@express` (validation, negative): FP True -> False
- improved `HN-validation-06@spring-petclinic` (validation, negative): FP True -> False
- improved `HN-validation-06@express` (validation, negative): FP True -> False

## long: 3 melhorias, 0 regressões
- improved `LN-01` (dev, negative): FP True -> False
- improved `LN-04` (validation, negative): FP True -> False
- improved `LN-06` (validation, negative): FP True -> False

## ind: 16 melhorias, 2 regressões
- regressed `V3I-P12` (independent, positive): hit5 False->False rr 0.125->0.000 empty False->False
- regressed `V3I-P21` (independent, positive): hit5 False->False rr 0.143->0.000 empty False->False
- improved `V3I-P19` (independent, positive): hit5 True->True rr 0.333->0.500 empty False->False
- improved `V3I-N02` (independent, negative): FP True -> False
- improved `V3I-N04` (independent, negative): FP True -> False
- improved `V3I-N05` (independent, negative): FP True -> False
- improved `V3I-N06` (independent, negative): FP True -> False
- improved `V3I-N09` (independent, negative): FP True -> False
- improved `V3I-N10` (independent, negative): FP True -> False
- improved `V3I-N16` (independent, negative): FP True -> False
- improved `V3I-N17` (independent, negative): FP True -> False
- improved `V3I-N26` (independent, negative): FP True -> False
- improved `V3I-N27` (independent, negative): FP True -> False
- improved `V3I-N30` (independent, negative): FP True -> False
- improved `V3I-N31` (independent, negative): FP True -> False
- improved `V3I-N33` (independent, negative): FP True -> False
- improved `V3I-N34` (independent, negative): FP True -> False
- improved `V3I-N36` (independent, negative): FP True -> False
