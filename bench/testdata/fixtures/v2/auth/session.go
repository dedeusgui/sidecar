package auth

import "time"

// SessionTTL is how long a login stays valid without a refresh.
const SessionTTL = 30 * time.Minute

// RefreshSession extends a session that is still valid at now.
func RefreshSession(expiresAt, now time.Time) (time.Time, bool) {
	if now.After(expiresAt) {
		return now, false
	}
	return now.Add(SessionTTL), true
}
