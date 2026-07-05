// Package strutil provides small string helpers used across cttw.
package strutil

// ShortID returns an 8-character prefix of id, or id itself if it is shorter.
func ShortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}
