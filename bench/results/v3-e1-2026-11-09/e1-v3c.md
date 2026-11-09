# E1 — v3 (relevance gate + effort router) vs v2 (intent + contract + packer)

Holdout incluído: não

| Suíte | Método | FP/neg | Hit@1 | Hit@5 | MRR | Positivos vazios | Tokens pos | Tokens neg | p50 ms | p95 ms |
|---|---|---|---|---|---|---|---|---|---|---|
| ten | v2 (intent + contract + packer) | 0/10 | 52.5% | 97.5% | 0.7102 | 0/40 | 418.3 | 0 | 19.03 | 63.68 |
| ten | v3 (relevance gate + effort router) | 0/10 | 52.5% | 97.5% | 0.7067 | 0/40 | 299.1 | 0 | 13.04 | 75.34 |
| hn | v2 (intent + contract + packer) | 30/180 | — | — | — | 0/0 | None | 60.2 | 3.17 | 28.12 |
| hn | v3 (relevance gate + effort router) | 14/180 | — | — | — | 0/0 | None | 14.4 | 3.1 | 20.41 |
| long | v2 (intent + contract + packer) | 3/6 | 56.2% | 87.5% | 0.6844 | 0/16 | 465.2 | 224.2 | 18.9 | 33.4 |
| long | v3 (relevance gate + effort router) | 1/6 | 56.2% | 87.5% | 0.6844 | 0/16 | 339.9 | 24.3 | 19.72 | 38.34 |
| ind | v2 (intent + contract + packer) | 13/40 | 56.7% | 83.3% | 0.6864 | 0/30 | 425.8 | 119.5 | 19.21 | 42.26 |
| ind | v3 (relevance gate + effort router) | 9/40 | 56.7% | 83.3% | 0.6775 | 0/30 | 293.7 | 37.2 | 12.18 | 31.97 |
| cases | v2 (intent + contract + packer) | 0/10 | 45.0% | 67.5% | 0.5354 | 5/40 | 276.4 | 0 | 7.32 | 13.68 |
| cases | v3 (relevance gate + effort router) | 0/10 | 42.5% | 57.5% | 0.4833 | 13/40 | 146.4 | 0 | 6.15 | 12.35 |
| scen | v2 (intent + contract + packer) | 0/2 | 76.9% | 100.0% | 0.8654 | 0/13 | 208.6 | 0 | 5.4 | 13.2 |
| scen | v3 (relevance gate + effort router) | 0/2 | 76.9% | 100.0% | 0.8654 | 0/13 | 171.6 | 0 | 4.7 | 9.06 |

## ten: 0 melhorias, 1 regressões
- regressed `R03-3` (development, positive): hit5 False->False rr 0.143->0.000 empty False->False

## hn: 16 melhorias, 0 regressões
- improved `HN-dev-02@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-03@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-03@dapper` (dev, negative): FP True -> False
- improved `HN-dev-04@click` (dev, negative): FP True -> False
- improved `HN-dev-04@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-04@kotlinx-datetime` (dev, negative): FP True -> False
- improved `HN-dev-05@chi` (dev, negative): FP True -> False
- improved `HN-dev-05@serde-json` (dev, negative): FP True -> False
- improved `HN-dev-06@express` (dev, negative): FP True -> False
- improved `HN-dev-07@click` (dev, negative): FP True -> False
- improved `HN-dev-07@monolog` (dev, negative): FP True -> False
- improved `HN-dev-07@express` (dev, negative): FP True -> False
- improved `HN-dev-08@serde-json` (dev, negative): FP True -> False
- improved `HN-validation-05@chi` (validation, negative): FP True -> False
- improved `HN-validation-05@kotlinx-datetime` (validation, negative): FP True -> False
- improved `HN-validation-05@dapper` (validation, negative): FP True -> False

## long: 2 melhorias, 0 regressões
- improved `LN-01` (dev, negative): FP True -> False
- improved `LN-04` (validation, negative): FP True -> False

## ind: 4 melhorias, 2 regressões
- regressed `V3I-P12` (independent, positive): hit5 False->False rr 0.125->0.000 empty False->False
- regressed `V3I-P21` (independent, positive): hit5 False->False rr 0.143->0.000 empty False->False
- improved `V3I-N02` (independent, negative): FP True -> False
- improved `V3I-N09` (independent, negative): FP True -> False
- improved `V3I-N30` (independent, negative): FP True -> False
- improved `V3I-N33` (independent, negative): FP True -> False

## cases: 0 melhorias, 9 regressões
- regressed `T02` (tune, positive): hit5 False->False rr 0.000->0.000 empty False->True
- regressed `T06` (tune, positive): hit5 False->False rr 0.000->0.000 empty False->True
- regressed `T10` (tune, positive): hit5 True->False rr 0.333->0.000 empty False->True
- regressed `E05` (eval, positive): hit5 False->False rr 0.000->0.000 empty False->True
- regressed `E06` (eval, positive): hit5 False->False rr 0.000->0.000 empty False->True
- regressed `E10` (eval, positive): hit5 True->False rr 1.000->0.000 empty False->True
- regressed `E12` (eval, positive): hit5 False->False rr 0.000->0.000 empty False->True
- regressed `E13` (eval, positive): hit5 True->False rr 0.500->0.000 empty False->True
- regressed `E20` (eval, positive): hit5 True->False rr 0.250->0.000 empty False->False

## scen: 0 melhorias, 0 regressões
