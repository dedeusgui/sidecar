# Rodadas intermediárias

Cada arquivo é uma execução de `cmd/sidecar-eval` feita durante o desenvolvimento, em um estado do pipeline
que já foi alterado. Não use estes números para avaliar o produto atual: eles existem para auditoria do caminho
percorrido. A rodada que vale é `../final-state.json`, com o código congelado.

- `run1`: primeira medição, antes das correções de co-alteração e de identificadores literais.
- `run2`–`run4`: correções de co-alteração (anotação, não item), de literal (tradução não conta) e de supressão de arquivo.
- `final`–`final5`: correções de precisão e de vocabulário (identificador exato, IDF, frequência por coluna),
  e a rodada que levou ao `final-state`. A partir de `final`, o split holdout já tinha sido medido.
