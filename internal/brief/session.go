package brief

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SessionTTL is how long session signals survive without activity.
const SessionTTL = 6 * time.Hour

// MaxSignalEntries bounds every per-session set, so a very long session cannot grow its
// state file without limit. Entries past the bound are not recorded.
const MaxSignalEntries = 1000

// Task stages, inferred from tool use. Stage changes come only from edit and test calls.
const (
	StageLocate = "locate"
	StageEdit   = "edit"
	StageTest   = "test"
	StageVerify = "verify"
)

// SessionSignals is what Sidecar remembers about one agent session in one worktree. It
// holds counts and hashes only: never prompts, tool output, or raw paths. Version (the HEAD
// commit) and Expires invalidate it when the code changes or the session goes idle.
// Reads, Searches and Errors describe the current epoch, which starts at the last edit.
type SessionSignals struct {
	Version      string          `json:"version"`
	Expires      int64           `json:"expires"`
	Stage        string          `json:"stage"`
	EvidenceSent int             `json:"evidence_sent"`
	Symbols      map[string]bool `json:"symbols_visited"`
	Files        map[string]bool `json:"files_seen"`
	Edits        map[string]int  `json:"file_edits"`
	Reads        map[string]int  `json:"reads"`
	Searches     map[string]bool `json:"searches"`
	Errors       map[string]int  `json:"errors"`
	Signals      map[string]int  `json:"signals"`
}

// NewSignals returns empty signals for version, valid until now plus SessionTTL.
func NewSignals(version string, now time.Time) SessionSignals {
	s := SessionSignals{Version: version, Expires: now.Add(SessionTTL).Unix(), Stage: StageLocate}
	s.initMaps()
	return s
}

// LoadSignals returns the stored signals when they still apply to version and have not
// expired. Missing, corrupt, outdated or expired state yields fresh signals, never an error.
func LoadSignals(path, version string, now time.Time) SessionSignals {
	fresh := NewSignals(version, now)
	data, err := os.ReadFile(path)
	if err != nil {
		return fresh
	}
	var s SessionSignals
	if json.Unmarshal(data, &s) != nil || s.Version != version || now.Unix() >= s.Expires {
		return fresh
	}
	s.initMaps()
	return s
}

// SaveSignals writes the signals atomically. Callers treat failure as non-fatal.
func SaveSignals(path string, s SessionSignals) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ClearEpoch forgets reads, searches and errors. An edit calls it: what was read or tried
// before a change no longer predicts what will happen after it.
func (s *SessionSignals) ClearEpoch() {
	s.Reads = map[string]int{}
	s.Searches = map[string]bool{}
	s.Errors = map[string]int{}
}

func (s *SessionSignals) initMaps() {
	if s.Symbols == nil {
		s.Symbols = map[string]bool{}
	}
	if s.Files == nil {
		s.Files = map[string]bool{}
	}
	if s.Edits == nil {
		s.Edits = map[string]int{}
	}
	if s.Signals == nil {
		s.Signals = map[string]int{}
	}
	if s.Reads == nil {
		s.Reads = map[string]int{}
	}
	if s.Searches == nil {
		s.Searches = map[string]bool{}
	}
	if s.Errors == nil {
		s.Errors = map[string]int{}
	}
}
