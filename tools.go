//go:build tools
// +build tools

package main

// This file pins build dependencies for the DERO miner so go mod tidy
// doesn't remove them. The miner is compiled separately from source.

import (
	_ "github.com/deroproject/derohe/cmd/dero-miner"
)
