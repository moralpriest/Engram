// Copyright 2023-2024 DERO Foundation. All rights reserved.
//
// Use of this source code in any form is governed by RESEARCH license.
// license can be found in the LICENSE file.

//go:build go1.18
// +build go1.18

package main

import (
	"testing"
)

// FuzzStoreValueInputs tests StoreValue with random inputs
// Looking for panics, nil pointer dereferences, or unexpected crashes
func FuzzStoreValueInputs(f *testing.F) {
	// Seed corpus with edge cases
	f.Add("", []byte{}, []byte{})
	f.Add("tree", []byte("key"), []byte("value"))
	f.Add("tree", []byte{0x00}, []byte{0xff, 0xfe})
	f.Add("a]b[c", []byte("key\x00with\x00nulls"), []byte{})
	f.Add(string(make([]byte, 1000)), []byte("k"), []byte("v"))

	f.Fuzz(func(t *testing.T, tree string, key []byte, value []byte) {
		// We're testing that the function doesn't panic on any input
		// Actual storage will fail without proper setup, but shouldn't crash
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on input: tree=%q, key=%v, value=%v: %v", tree, key, value, r)
			}
		}()

		// Test input validation logic (will return error, but shouldn't panic)
		_ = StoreValue(tree, key, value)
	})
}

// FuzzGetValueInputs tests GetValue with random inputs
func FuzzGetValueInputs(f *testing.F) {
	f.Add("", []byte{})
	f.Add("tree", []byte("key"))
	f.Add("tree", []byte{0x00, 0xff})
	f.Add(string(make([]byte, 10000)), []byte("k"))

	f.Fuzz(func(t *testing.T, tree string, key []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on input: tree=%q, key=%v: %v", tree, key, r)
			}
		}()

		_, _ = GetValue(tree, key)
	})
}

// FuzzDeleteKeyInputs tests DeleteKey with random inputs
func FuzzDeleteKeyInputs(f *testing.F) {
	f.Add("", []byte{})
	f.Add("tree", []byte("key"))
	f.Add("../../../etc/passwd", []byte("key"))
	f.Add("tree\x00inject", []byte{0x00})

	f.Fuzz(func(t *testing.T, tree string, key []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("panic on input: tree=%q, key=%v: %v", tree, key, r)
			}
		}()

		_ = DeleteKey(tree, key)
	})
}
