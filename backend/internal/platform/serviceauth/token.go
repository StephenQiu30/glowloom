// Package serviceauth validates private service bearer identities.
package serviceauth

// ValidToken accepts bounded visible ASCII without whitespace. In particular,
// gRPC metadata cannot carry control bytes or non-ASCII credential values.
func ValidToken(token string) bool {
	if len(token) < 32 || len(token) > 4096 {
		return false
	}
	for i := range len(token) {
		if token[i] < 0x21 || token[i] > 0x7e {
			return false
		}
	}
	return true
}
