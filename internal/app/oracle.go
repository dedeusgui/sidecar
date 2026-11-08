package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// OracleArtifact is the record an independent harness writes after it runs an oracle (tests or a
// build) outside the agent's workspace. Only an artifact whose digest verifies makes feedback strong.
// The digest is an HMAC-SHA256 over the identity fields with a key the agent does not hold, so
// editing the JSON of an agent report cannot promote its provenance to "harness" or "human".
type OracleArtifact struct {
	SchemaVersion int    `json:"schema_version"`
	RunID         string `json:"run_id"`
	SessionID     string `json:"session_id"`
	TaskID        string `json:"task_id"`
	Commit        string `json:"commit"`
	OracleSHA256  string `json:"oracle_sha256"` // hash of the oracle definition the harness executed
	Status        string `json:"status"`        // passed | failed
	Digest        string `json:"digest"`        // OracleDigest(key, artifact)
}

// minHarnessKey is the shortest key accepted for signing; a weak key would make the digest guessable.
const minHarnessKey = 16

// canonicalOracle is the exact byte string the digest covers. Fields are joined with the ASCII unit
// separator, so no field value can move bytes into a neighbouring field.
func canonicalOracle(a OracleArtifact) string {
	return strings.Join([]string{"sidecar-oracle-v1", fmt.Sprint(a.SchemaVersion), a.RunID, a.SessionID,
		a.TaskID, a.Commit, a.OracleSHA256, a.Status}, "\x1f")
}

// OracleDigest returns the HMAC-SHA256 digest of the artifact's identity fields under key.
func OracleDigest(key []byte, a OracleArtifact) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(canonicalOracle(a)))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyOracleArtifact checks that the artifact was signed with key and is well formed. The session
// and commit checks are separate, because they depend on the report and the worktree.
func verifyOracleArtifact(key []byte, a OracleArtifact) error {
	if len(key) < minHarnessKey {
		return fmt.Errorf("harness key must be at least %d bytes", minHarnessKey)
	}
	if a.SchemaVersion != 1 {
		return fmt.Errorf("oracle artifact schema_version must be 1, got %d", a.SchemaVersion)
	}
	if !hmac.Equal([]byte(OracleDigest(key, a)), []byte(a.Digest)) {
		return errors.New("oracle artifact digest does not verify: it was not written by the harness holding this key")
	}
	if a.Status != "passed" && a.Status != "failed" {
		return fmt.Errorf("oracle artifact status must be passed or failed, got %q", a.Status)
	}
	if a.RunID == "" || a.SessionID == "" || a.Commit == "" {
		return errors.New("oracle artifact must name its run, session and commit")
	}
	if len(a.OracleSHA256) != sha256.Size*2 {
		return errors.New("oracle artifact must carry the sha256 of the oracle definition")
	}
	if _, err := hex.DecodeString(a.OracleSHA256); err != nil {
		return errors.New("oracle_sha256 is not hexadecimal")
	}
	return nil
}
