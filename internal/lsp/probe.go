package lsp

import (
	"context"
	"path/filepath"
	"time"
)

// Probe runs a real handshake for one discovered row. Only rows that are available are
// started: disabled, requires_consent and missing rows are returned unchanged, so a probe
// never starts a server the user did not allow. "ready" needs initialize/initialized and at
// least one announced capability; anything else is "failed" with the reason.
func Probe(ctx context.Context, root string, row Server) Server {
	if row.Status != StatusAvailable || len(row.RepoMarkers) == 0 {
		return row
	}
	ws := filepath.Join(root, filepath.Dir(filepath.FromSlash(row.RepoMarkers[0])))
	row.Status, row.Reason = StatusInitializing, "handshake em andamento"
	c, err := Start(ctx, root, ws, row.LanguageID, row.InstalledServerPath, Options{
		StartTimeout: 15 * time.Second, RequestTimeout: 5 * time.Second,
	})
	if err != nil {
		row.Status, row.Reason = StatusFailed, err.Error()
		return row
	}
	defer func() { _ = c.Close() }()
	row.Capabilities = c.Capabilities()
	if len(row.Capabilities) == 0 {
		row.Status, row.Reason = StatusFailed, "handshake concluído, mas o servidor não anunciou capacidades úteis"
		return row
	}
	row.Status, row.Reason = StatusReady, "initialize/initialized concluído; capacidades anunciadas"
	return row
}
