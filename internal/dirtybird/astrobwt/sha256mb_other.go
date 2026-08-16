//go:build !amd64 && !arm64

package astrobwt

// No multi-buffer SHA on this arch: pair hashing degrades to two singles.

const pairHashPossible = false

func pairHashAvailable() bool { return false }

func sha256Sum256Pair(a, b []byte) ([32]byte, [32]byte) {
	return sha256Fallback(a), sha256Fallback(b)
}
