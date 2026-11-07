# Instalação, distribuição e atualização

**Estado em 2026-11-06:** há `.goreleaser.yaml` e workflow de release em tag `v*`, mas **nenhuma release publicada foi encontrada**. As instruções abaixo separam implementação existente, experimento local e design futuro; não são afirmação de instalação ponta a ponta em todas as plataformas.

## Fluxo atual

1. Um executável `sidecar` / `sidecar.exe` é compilado a partir de Go (`cmd/sidecar`).
2. `sidecar install` mostra as alterações, pede consentimento (ou `--yes`), copia o executável para `~/.local/bin` ou `%LOCALAPPDATA%\Programs\sidecar`.
3. Instala `UserPromptSubmit` em settings de Claude Code e/ou Codex detectados; faz backup e mantém manifest de ownership.
4. `sidecar doctor` verifica binário, SQLite/FTS5, Git, hooks e integridade; `sidecar uninstall` remove apenas entradas que instalou (com `--purge` limpa cache/config do Sidecar).

Código de referência: `internal/install/`, `internal/app/diagnostics.go`, `cmd/sidecar/` e `internal/paths/`.

## Build para desenvolvimento (não é release)

```bash
go test ./...
go build -o sidecar ./cmd/sidecar
./sidecar version
./sidecar install
./sidecar doctor
```

No PowerShell, usar `go build -o sidecar.exe ./cmd/sidecar`, `.\sidecar.exe install`, etc. Não executar os comandos de instalação na máquina pessoal/CI sem aceitar os efeitos nos arquivos de configuração; usar usuário/ambiente de teste. O binário copiado pode não estar no `PATH`: usar caminho completo ou configurar `PATH` manualmente. Esse requisito ainda deve melhorar antes do quickstart oficial.

## Contrato que precisamos validar

- **Detectar agente:** sem agente, erro claro; com apenas um, não escrever config de outro; múltiplos hooks não duplicam ou sobrescrevem.
- **Consentimento:** preview e `--yes` explícito; backups criados sem sobrescrever originais; JSON inválido deixado intacto.
- **Idempotência / update:** testar mesma versão, versão nova, fonte/destino coincidentes, manifest ausente/corrupto, binário externo no caminho de destino.
- **Uninstall / rollback:** remover somente hooks/binários registrados; preservar configurações alheias; detectar e reportar falha de remoção, incluindo `.exe` em uso no Windows. Não assumir sucesso se o SO reteve binário.
- **Segurança:** caminhos Unicode/espaços e permissões; não baixar/rodar remote scripts sem verificação; checksum SHA-256; não executar código de repo do usuário.
- **Compatibilidade:** smoke test end-to-end do hook para Claude e Codex, incluindo consentimento de hooks no Codex.

## Estratégia escolhida

**Primeiro:** binários nativos Linux, macOS e Windows (amd64/arm64 onde suportado), GitHub Releases + GoReleaser, `checksums.txt`, notas e assinatura/proveniência em etapa posterior. Tag `v0.1.0` somente depois de CI, licença definida, smoke test e revisão. Não há dependência de Node.js no runtime.

**Depois:** instalador shell/PowerShell ou gerenciador de pacotes (Homebrew, Scoop/WinGet). Evitar npm global só por familiaridade: criaria dependência do Node para instalar um binário Go, sem benefício garantido.

**Separar responsabilidades:** gerenciador de pacote instala/atualiza o binário; `sidecar install` registra a integração do agente. O instalador atual **copia** o próprio executável; antes de adicionar pacote, projetar modo “referência ao binário gerenciado” versus “cópia autogerenciada”, incluindo rollback, version e manuseio de `PATH`. Não misturar dois proprietários do mesmo executável.

## Checklist de primeira release

- [ ] Branch padrão do GitHub é `main` (verificada em 2026-11-06 com `gh repo view`); commit e tag pinados ainda pendentes.
- [ ] Licença MIT em `LICENSE` (Copyright (c) 2026 Guilherme de Deus) criada em 2026-11-06, ainda não commitada; confirmar que o arquivo entra nos artefatos do GoReleaser antes da tag.
- [ ] Build/release provado em matriz e checksums verificados após download.
- [ ] Smoke test isolado por SO (instalação, hooks reais, `doctor`, desinstalação, rollback).
- [ ] README mostra comandos reais e limitações; nenhum link de download inexistente.
- [ ] Planos de atualização compatíveis com dados e configurações anteriores.
- [ ] Nenhum teste de pagamento/modelo caro embutido em release.

## Observabilidade para suporte

Documentar `sidecar version`, `doctor`, `status` e `explain --json`. Logs permanecem locais, sem texto do prompt. Não solicitar que usuário publique índices, tokens de acesso, configuração privada ou transcrições. Para bugs de instalação registrar SO, versão, modo de instalação, erro redigido e comportamento de outros hooks.
