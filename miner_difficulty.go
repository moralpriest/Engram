package main

import (
	"math/big"

	"github.com/deroproject/derohe/cryptography/crypto"
)

var (
	bigZero   = big.NewInt(0)
	bigOne    = big.NewInt(1)
	oneLsh256 = new(big.Int).Lsh(bigOne, 256)
)

// CachedTarget pre-computes target = 2^256 / difficulty and provides
// zero-extra-allocation hash checking. All big.Int values are stored as
// struct fields to avoid heap-allocating new *big.Int on every hash check.
type CachedTarget struct {
	target big.Int
	diff   string  // source difficulty hex string — used to detect changes
	hash   big.Int // reusable buffer for hash-to-big-endian conversion
}

// UpdateTarget recomputes the cached difficulty target.
// It is a no-op when the difficulty string has not changed.
// This should be called from the mining loop when a new job arrives.
func (ct *CachedTarget) UpdateTarget(difficulty string) {
	if ct.diff == difficulty {
		return
	}
	d := new(big.Int)
	d.SetString(difficulty, 10)
	if d.Cmp(bigZero) == 0 {
		panic("difficulty can never be zero")
	}
	ct.target.Div(oneLsh256, d)
	ct.diff = difficulty
}

// CheckHash checks whether a hash meets the cached difficulty target.
// The hash is reversed in-place (little-endian → big-endian) and compared
// as an unsigned big integer.  Unlike the original checkPowHashBig this
// does not allocate new *big.Int on each call.
func (ct *CachedTarget) CheckHash(pow_hash crypto.Hash) bool {
	blen := len(pow_hash)
	for i := 0; i < blen/2; i++ {
		pow_hash[i], pow_hash[blen-1-i] = pow_hash[blen-1-i], pow_hash[i]
	}
	ct.hash.SetBytes(pow_hash[:])
	return ct.hash.Cmp(&ct.target) <= 0
}

// hashToBig converts a crypto.Hash to a big.Int (allocates).
// Kept for backward compatibility.
func hashToBig(buf crypto.Hash) *big.Int {
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}
	return new(big.Int).SetBytes(buf[:])
}

// convertDifficultyToBig converts a uint64 difficulty to a big.Int target (allocates).
func convertDifficultyToBig(difficultyi uint64) *big.Int {
	if difficultyi == 0 {
		panic("difficulty can never be zero")
	}
	difficulty := new(big.Int).SetUint64(difficultyi)
	return new(big.Int).Div(oneLsh256, difficulty)
}

// convertIntegerDifficultyToBig converts a big.Int difficulty to a big.Int target (allocates).
func convertIntegerDifficultyToBig(difficultyi *big.Int) *big.Int {
	if difficultyi.Cmp(bigZero) == 0 {
		panic("difficulty can never be zero")
	}
	return new(big.Int).Div(oneLsh256, difficultyi)
}

// checkPowHashBig checks if a hash meets a difficulty target (allocates).
// Kept for backward compatibility.
func checkPowHashBig(pow_hash crypto.Hash, big_difficulty_integer *big.Int) bool {
	big_pow_hash := hashToBig(pow_hash)
	big_difficulty := convertIntegerDifficultyToBig(big_difficulty_integer)
	return big_pow_hash.Cmp(big_difficulty) <= 0
}
