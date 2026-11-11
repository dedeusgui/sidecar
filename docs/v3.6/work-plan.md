# V3.6 — plano de correções e validação

## Estado e contrato

A V3.6 é uma correção incremental e opt-in da engine unificada V3.5, não uma reescrita nem uma promoção do padrão. A implementação é experimental; as hipóteses e pendências abaixo preservam o plano de trabalho e devem ser conferidas contra o progresso e o código atuais. Preservar V1–V3.5, contratos dos hooks, execução local e offline, fail-open, limites globais, privacidade, escopo e determinismo. Não executar código de repositórios indexados, benchmarks pagos ou holdout. Não alterar configurações de hooks do usuário. O histórico e os limites dos resultados observados estão em [progress.md](progress.md).

## Sequência e hipóteses

1. **Entrega de evidência:** investigar se `snippetKeysAndFingerprints` e `v36Choices` descartam uma evidência antes de confirmar que o snippet correspondente foi alocado e entregue. Reproduzir com orçamento ou limite de itens que rejeite a proposta; verificar também snippet aceito, sessão vista, fonte stale e escopo divergente. Hipótese: uma proposta não entregue pode eliminar indevidamente a alternativa de evidência. Só corrigir se o teste demonstrar a falha; o fallback deve respeitar a ordem e o orçamento, sem duplicar evidência.
2. **Abstenção:** comparar decisões e saída V3.5/V3.6 por caso e componente no conjunto de desenvolvimento. Distinguir mudança do gate de mudança de empacotamento: saída vazia na V3.5 pode ser falha de entrega, não abstenção deliberada. A V3.6 produziu saída em 15/180 negativos difíceis versus 8/180 na V3.5; isso exige investigação e não autoriza regras específicas aos exemplos. Formular hipóteses gerais antes de criar novos testes positivos e negativos independentes; registrar regressões sem ocultá-las.
3. **Planejamento e packing:** testar alocação por seção, prioridade e ordem, limites global e por seção, contabilização real de texto e corpo, truncamento, propostas com múltiplos itens, deduplicação, razões de descarte e estabilidade. Testar que material adversarial não forja marcadores, rótulos ou instruções estruturais e que stale/escopo incorreto nunca vira evidência verificada. Confirmar que rótulos e proveniência correspondem à prova, não apenas que não se contradizem visualmente.
4. **Hooks e compatibilidade:** executar testes de contrato para Claude e Codex, abstinência, fail-open, sessões, invalidação e repetição determinística; comparar versões anteriores com baseline equivalente. Testes simulados não comprovam E2E de clientes reais: registrar o que de fato foi executado.
5. **Medição:** revisar o scorecard para evitar falsos positivos de caminho/símbolo, demonstrar como pontua linhas de evidência e erros e separar custo, saída vazia e latência de cada caso. Rodar avaliação offline não-holdout com controles V3/V3.5/V3.6, resultados por caso e ablações pertinentes. Registrar corpus, commits fixos, ambiente, comandos, critérios e limitações; não tratar manifesto temporário como reprodução garantida nem presença de caminho como sucesso de agente. Avaliar custo de tokens e latência juntamente com entrega e falsos positivos.
6. **Integração:** revisar cada diff e teste, executar checagens do projeto, documentar resultados negativos e pontos pendentes. Manter o motor opt-in até os gates da V3.5/V3.6 serem realmente cumpridos; uma suíte verde não prova ganho agentic. Só organizar commits e integrar à main após revisão, testes e verificação do estado remoto e das proteções.

## Critérios verificáveis

- Testes demonstram as falhas corrigidas e caminhos negativos, inclusive preservação de evidência quando uma proposta elegível não é entregue.
- Nenhuma regressão de compatibilidade demonstrada é ignorada; trade-offs de abstenção e recall são relatados por estrato, sem alegar não regressão quando dados a contradizem.
- `go test -count=1 ./...`, `go test -race -count=1 ./internal/store/ ./internal/index/ ./internal/app/ ./internal/gitx/`, lint configurado e avaliação offline não-holdout executam com resultados registrados. Para alteração de retrieval, comparar baseline e ablação e investigar pioras por caso.
- Revisão cobre código não confiável, custos/limites, dados sensíveis, escopo, compatibilidade e o diff inteiro. Nenhum commit inclui artefatos temporários ou dados de código recuperado.
- Holdout negativo já exposto nesta sessão é contaminado e não pode ser anunciado como cego. Um novo holdout independente só pode ser executado após freeze explícito e conforme a política do projeto; não é necessário para os testes de desenvolvimento.

## Divisão de trabalho

O coordenador mantém documentação, integração, revisão de todas as alterações e validação final. Um único worker Orca por vez recebe escopo estreito, arquivos permitidos e testes exigidos. O worker não faz commits, push, merge, instalação, abre holdout nem altera arquivos fora do escopo. Resultados devem identificar hipótese, reprodução, diff, comandos, resultados, riscos e lacunas; o coordenador verifica tudo antes de aceitar.
