# Sidecar V3 — progresso de implementação

Atualizado em 2026-11-09. Branch `codex/v3-p0-lab`, baseada em `24323a8ef4f93f51c8bb5a7d305ed1574d8bbfe4` (PR #19). Não houve merge.

## Checkpoint P0-A

- Auditoria e leitura integral dos documentos solicitados concluídas. Baseline: 54 testes Python e todos os pacotes Go passaram; Go 1.24.0 oficial foi instalado em `/tmp/sidecar-tools/go` e seu checksum conferido.
- O laboratório preserva valores desconhecidos nas voltas e nos totais, responde tarefas ambíguas apenas após pergunta relevante anterior à edição, registra permissões por linguagem e correlaciona traces por chamada, ordem e thread. Contadores de cache e tempos observados no log local são separados; tokens de resultados de ferramenta e contexto residente continuam desconhecidos quando não há fonte.
- O agente real exige sandbox `bwrap` com checkout e estado isolados. Um teste com arquivo externo e executáveis falsos verificou a barreira de filesystem e o PATH filtrado. O preflight é obrigatório, mas a integração real ainda não foi exercida: `claude` não está instalado neste ambiente.
- O manifesto normal reutiliza metadados selados do holdout, sem abrir seus arquivos ou oráculos; inventário novo ou tarefa alterada bloqueia a leitura automática. `validate-corpus` exclui o holdout por padrão. Um teste verifica a ausência de leitura do holdout sintético.
- O reconhecimento de edição ignora instruções citadas e mantém comandos positivos em cláusulas separadas; os testes de `retrieve` e `groom` passaram.
- Verificação mais recente: 72 testes do laboratório, 17 testes do parser agentic, todos os pacotes Go (`go test -count=1 ./...`) e `bench/consolidate.py --check` passaram. O manifesto de development/validation foi gerado em `/tmp/sidecar-v3-manifest.json` sem abrir o holdout. Os testes que exercitam o ledger usam exclusivamente fixtures sintéticas temporárias.

## Limites e próximos passos

- O holdout real exige `--final`, `--yes-spend`, revisão limpa identificada por `--frozen-revision`, SHA-256 do binário e preflight do sandbox antes de abrir tarefas. A compatibilidade do sandbox com a instalação real do Claude e todos os runtimes de teste continua sem prova neste ambiente; P0-A ainda precisa de revisão final.
- Implementar a política seletiva V3 opt-in, `sidecar explore`, calibração de Brief/Groom/Impact/Session Intelligence e controles competitivos; não há evidência de superioridade end-to-end nesta sessão.
- Próximo passo: implementar o gate V3 opt-in e medir E1 em development/validation, sem usar o holdout.
- Nenhuma chamada paga e nenhuma avaliação final do holdout foram executadas. Custo pago: US$ 0. O holdout final exige autorização, isolamento validado e versão congelada.
