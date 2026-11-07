package store

import (
	"context"
	"encoding/json"

	"github.com/dedeusgui/sidecar/internal/repo"
)

// Meta keys: the JSON of the last fingerprint, and the QuickHash state it was read under.
const (
	fingerprintKey      = "repo_fingerprint"
	fingerprintStateKey = "repo_fingerprint_state"
)

// DiscoverCached returns repo.Discover(root), reusing the copy stored in the index while HEAD
// and the recorded manifests are unchanged. Anything unreadable is recomputed, never an error.
func DiscoverCached(ctx context.Context, root string, st *Store) repo.Fingerprint {
	raw, _ := st.GetMeta(fingerprintKey)
	var prev repo.Fingerprint
	if raw != "" && json.Unmarshal([]byte(raw), &prev) == nil {
		if state, ok := repo.QuickHash(ctx, root, prev); ok {
			if saved, _ := st.GetMeta(fingerprintStateKey); saved == state {
				return prev
			}
		}
	}
	fp := repo.Discover(ctx, root)
	// The state must come from the manifests this walk recorded, so it is taken after the walk.
	// shortcut: a manifest edited during the walk is cached with its new mtime and old content until its next edit, upgrade by re-reading the state after the walk when that is reported.
	if state, ok := repo.QuickHash(ctx, root, fp); ok {
		if b, err := json.Marshal(fp); err == nil {
			_ = st.SetMeta(fingerprintKey, string(b))
			_ = st.SetMeta(fingerprintStateKey, state)
		}
	}
	return fp
}
