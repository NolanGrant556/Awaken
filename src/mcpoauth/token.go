package mcpoauth

import "time"

func (t TokenSet) AccessTokenExpired(clock Clock) bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	return !clock.Now().Before(t.ExpiresAt)
}

func (t TokenSet) AccessTokenExpiresWithin(clock Clock, threshold time.Duration) bool {
	if t.ExpiresAt.IsZero() {
		return false
	}
	if threshold < 0 {
		threshold = 0
	}
	return !clock.Now().Add(threshold).Before(t.ExpiresAt)
}

func (t TokenSet) RefreshNeeded(clock Clock, threshold time.Duration) bool {
	if t.RefreshToken.IsZero() {
		return false
	}
	if t.AccessToken.IsZero() {
		return true
	}
	return t.AccessTokenExpiresWithin(clock, threshold)
}
