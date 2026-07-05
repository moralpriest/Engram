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

func hashToBig(buf crypto.Hash) *big.Int {
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}
	return new(big.Int).SetBytes(buf[:])
}

func convertDifficultyToBig(difficultyi uint64) *big.Int {
	if difficultyi == 0 {
		panic("difficulty can never be zero")
	}
	difficulty := new(big.Int).SetUint64(difficultyi)
	return new(big.Int).Div(oneLsh256, difficulty)
}

func convertIntegerDifficultyToBig(difficultyi *big.Int) *big.Int {
	if difficultyi.Cmp(bigZero) == 0 {
		panic("difficulty can never be zero")
	}
	return new(big.Int).Div(oneLsh256, difficultyi)
}

func checkPowHashBig(pow_hash crypto.Hash, big_difficulty_integer *big.Int) bool {
	big_pow_hash := hashToBig(pow_hash)
	big_difficulty := convertIntegerDifficultyToBig(big_difficulty_integer)
	return big_pow_hash.Cmp(big_difficulty) <= 0
}
