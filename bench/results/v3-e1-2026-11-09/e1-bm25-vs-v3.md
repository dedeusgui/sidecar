# E1 — v3 (relevance gate + effort router) vs bm25 (fts5, raw terms)

Holdout incluído: não

| Suíte | Método | FP/neg | Hit@1 | Hit@5 | MRR | Positivos vazios | Tokens pos | Tokens neg | p50 ms | p95 ms |
|---|---|---|---|---|---|---|---|---|---|---|
| ten | bm25 (fts5, raw terms) | 1/10 | 35.0% | 50.0% | 0.424 | 8/40 | 259.1 | 36.5 | 2.56 | 7.28 |
| ten | v3 (relevance gate + effort router) | 0/10 | 52.5% | 97.5% | 0.7108 | 0/40 | 299.1 | 0 | 11.94 | 64.05 |
| hn | bm25 (fts5, raw terms) | 46/180 | — | — | — | 0/0 | None | 72.9 | 0.38 | 2.32 |
| hn | v3 (relevance gate + effort router) | 14/180 | — | — | — | 0/0 | None | 14.4 | 2.97 | 16.98 |
| long | bm25 (fts5, raw terms) | 3/6 | 50.0% | 81.2% | 0.6354 | 0/16 | 348.6 | 180.2 | 2.5 | 4.64 |
| long | v3 (relevance gate + effort router) | 1/6 | 56.2% | 87.5% | 0.6844 | 0/16 | 339.9 | 28.3 | 15.57 | 27.01 |
| ind | bm25 (fts5, raw terms) | 23/40 | 46.7% | 66.7% | 0.5486 | 1/30 | 314.1 | 190.1 | 2.11 | 4.5 |
| ind | v3 (relevance gate + effort router) | 9/40 | 56.7% | 83.3% | 0.6775 | 0/30 | 293.7 | 37.2 | 12.21 | 29.6 |

## ten: 23 melhorias, 6 regressões
- regressed `R02-1` (development, positive): hit5 True->True rr 1.000->0.200 empty False->False
- regressed `R02-2` (development, positive): hit5 True->True rr 1.000->0.500 empty False->False
- regressed `R02-4` (development, positive): hit5 True->True rr 1.000->0.500 empty False->False
- regressed `R05-1` (development, positive): hit5 True->True rr 0.500->0.333 empty False->False
- regressed `R05-2` (development, positive): hit5 True->True rr 1.000->0.500 empty False->False
- regressed `R05-4` (development, positive): hit5 True->True rr 1.000->0.500 empty False->False
- improved `R01-2` (development, positive): hit5 False->True rr 0.000->1.000 empty True->False
- improved `R01-3` (development, positive): hit5 False->True rr 0.000->1.000 empty True->False
- improved `R01-4` (development, positive): hit5 False->True rr 0.000->1.000 empty True->False
- improved `R02-3` (development, positive): hit5 False->True rr 0.000->1.000 empty True->False
- improved `R04-3` (development, positive): hit5 False->True rr 0.000->0.200 empty True->False
- improved `R05-3` (development, positive): hit5 False->True rr 0.000->0.500 empty False->False
- improved `R06-1` (development, positive): hit5 True->True rr 0.333->0.500 empty False->False
- improved `R06-2` (development, positive): hit5 False->True rr 0.000->0.500 empty False->False
- improved `R06-3` (development, positive): hit5 False->True rr 0.000->1.000 empty True->False
- improved `R06-4` (development, positive): hit5 False->True rr 0.000->0.500 empty False->False
- improved `R07-2` (development, positive): hit5 True->True rr 0.500->1.000 empty False->False
- improved `R07-3` (development, positive): hit5 False->True rr 0.000->1.000 empty False->False
- improved `R07-4` (development, positive): hit5 True->True rr 0.500->1.000 empty False->False
- improved `R08-1` (development, positive): hit5 False->True rr 0.125->0.200 empty False->False
- improved `R08-2` (development, positive): hit5 False->True rr 0.000->1.000 empty False->False
- improved `R08-3` (development, positive): hit5 False->True rr 0.000->0.500 empty False->False
- improved `R08-4` (development, positive): hit5 False->True rr 0.000->1.000 empty False->False
- improved `R09-1` (development, positive): hit5 False->True rr 0.000->0.333 empty False->False
- improved `R09-2` (development, positive): hit5 False->True rr 0.000->0.333 empty False->False
- improved `R09-3` (development, positive): hit5 False->True rr 0.000->1.000 empty True->False
- improved `R09-4` (development, positive): hit5 False->True rr 0.000->0.333 empty False->False
- improved `R09-5` (development, negative): FP True -> False
- improved `R10-3` (development, positive): hit5 False->True rr 0.000->1.000 empty True->False

## hn: 33 melhorias, 1 regressões
- regressed `HN-validation-02@monolog` (validation, negative): FP False -> True
- improved `HN-dev-02@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-03@hono` (dev, negative): FP True -> False
- improved `HN-dev-03@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-03@monolog` (dev, negative): FP True -> False
- improved `HN-dev-03@dapper` (dev, negative): FP True -> False
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

## long: 6 melhorias, 4 regressões
- regressed `LP-02` (dev, positive): hit5 True->True rr 1.000->0.200 empty False->False
- regressed `LP-06` (dev, positive): hit5 True->True rr 1.000->0.500 empty False->False
- regressed `LP-07` (dev, positive): hit5 True->True rr 0.333->0.250 empty False->False
- regressed `LP-13` (validation, positive): hit5 True->True rr 1.000->0.500 empty False->False
- improved `LP-05` (dev, positive): hit5 True->True rr 0.500->1.000 empty False->False
- improved `LP-10` (validation, positive): hit5 True->True rr 0.500->1.000 empty False->False
- improved `LP-14` (validation, positive): hit5 True->True rr 0.333->1.000 empty False->False
- improved `LP-16` (validation, positive): hit5 False->True rr 0.000->1.000 empty False->False
- improved `LN-04` (validation, negative): FP True -> False
- improved `LN-06` (validation, negative): FP True -> False

## ind: 24 melhorias, 1 regressões
- regressed `V3I-P12` (independent, positive): hit5 False->False rr 0.125->0.000 empty False->False
- improved `V3I-P02` (independent, positive): hit5 False->True rr 0.000->0.200 empty False->False
- improved `V3I-P06` (independent, positive): hit5 False->False rr 0.000->0.125 empty True->False
- improved `V3I-P07` (independent, positive): hit5 True->True rr 0.333->1.000 empty False->False
- improved `V3I-P09` (independent, positive): hit5 False->True rr 0.000->1.000 empty False->False
- improved `V3I-P13` (independent, positive): hit5 False->True rr 0.000->0.333 empty False->False
- improved `V3I-P14` (independent, positive): hit5 False->True rr 0.000->0.333 empty False->False
- improved `V3I-P17` (independent, positive): hit5 True->True rr 0.250->0.333 empty False->False
- improved `V3I-P23` (independent, positive): hit5 True->True rr 0.500->1.000 empty False->False
- improved `V3I-P27` (independent, positive): hit5 False->True rr 0.000->0.500 empty False->False
- improved `V3I-P30` (independent, positive): hit5 True->True rr 0.250->0.500 empty False->False
- improved `V3I-N02` (independent, negative): FP True -> False
- improved `V3I-N04` (independent, negative): FP True -> False
- improved `V3I-N06` (independent, negative): FP True -> False
- improved `V3I-N10` (independent, negative): FP True -> False
- improved `V3I-N11` (independent, negative): FP True -> False
- improved `V3I-N16` (independent, negative): FP True -> False
- improved `V3I-N17` (independent, negative): FP True -> False
- improved `V3I-N20` (independent, negative): FP True -> False
- improved `V3I-N27` (independent, negative): FP True -> False
- improved `V3I-N29` (independent, negative): FP True -> False
- improved `V3I-N30` (independent, negative): FP True -> False
- improved `V3I-N33` (independent, negative): FP True -> False
- improved `V3I-N35` (independent, negative): FP True -> False
- improved `V3I-N36` (independent, negative): FP True -> False
