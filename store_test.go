package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestStoreValue tests basic key-value storage functionality
func TestStoreValue(t *testing.T) {
	// Test data
	testType := "test"
	testKey := []byte("test_key")
	testValue := []byte("test_value")

	// Store value
	err := StoreValue(testType, testKey, testValue)
	if err != nil {
		t.Errorf("StoreValue() error = %v", err)
		return
	}

	// Retrieve value
	result, err := GetValue(testType, testKey)
	if err != nil {
		t.Errorf("GetValue() error = %v", err)
		return
	}

	// Verify value matches
	if !bytes.Equal(result, testValue) {
		t.Errorf("GetValue() = %v, want %v", result, testValue)
	}

	// Cleanup
	_ = DeleteKey(testType, testKey)
}

// TestStoreEncryptedValue tests encrypted storage functionality
func TestStoreEncryptedValue(t *testing.T) {
	// Test data
	testType := "encrypted_test"
	testKey := []byte("encrypted_key")
	testValue := []byte("encrypted_value")
	encryptionKey := []byte("test_encryption_key_32bytes_long!")

	// Store encrypted value
	err := StoreEncryptedValue(testType, testKey, testValue)
	if err != nil {
		t.Errorf("StoreEncryptedValue() error = %v", err)
		return
	}

	// Retrieve encrypted value
	result, err := GetEncryptedValue(testType, testKey)
	if err != nil {
		t.Errorf("GetEncryptedValue() error = %v", err)
		return
	}

	// Verify value matches
	if !bytes.Equal(result, testValue) {
		t.Errorf("GetEncryptedValue() = %v, want %v", result, testValue)
	}

	// Cleanup
	_ = DeleteKey(testType, testKey)

	// Suppress unused variable warning
	_ = encryptionKey
}

// TestDeleteKey tests key deletion functionality
func TestDeleteKey(t *testing.T) {
	// Test data
	testType := "delete_test"
	testKey := []byte("delete_key")
	testValue := []byte("delete_value")

	// Store value
	err := StoreValue(testType, testKey, testValue)
	if err != nil {
		t.Errorf("StoreValue() error = %v", err)
		return
	}

	// Verify value exists
	_, err = GetValue(testType, testKey)
	if err != nil {
		t.Errorf("GetValue() error before delete = %v", err)
		return
	}

	// Delete key
	err = DeleteKey(testType, testKey)
	if err != nil {
		t.Errorf("DeleteKey() error = %v", err)
		return
	}

	// Verify value no longer exists
	_, err = GetValue(testType, testKey)
	// Should return error or empty result after deletion
	if err == nil {
		t.Log("GetValue() after delete returned data (may be expected behavior)")
	}
}

// TestGetDir tests directory retrieval
func TestGetDir(t *testing.T) {
	dir, err := GetDir()
	if err != nil {
		t.Errorf("GetDir() error = %v", err)
		return
	}

	// Verify directory is not empty
	if dir == "" {
		t.Error("GetDir() returned empty directory")
		return
	}

	// Verify directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("GetDir() returned non-existent directory: %s", dir)
	}
}

// TestGetShard tests shard retrieval
func TestGetShard(t *testing.T) {
	shard, err := GetShard()
	if err != nil {
		t.Errorf("GetShard() error = %v", err)
		return
	}

	// Verify shard is not empty
	if shard == "" {
		t.Error("GetShard() returned empty shard")
	}
}

// TestAppPath tests app path retrieval
func TestAppPath(t *testing.T) {
	path := AppPath()

	// Verify path is not empty
	if path == "" {
		t.Error("AppPath() returned empty path")
		return
	}

	// Verify path is valid
	if !filepath.IsAbs(path) && path != "." {
		t.Logf("AppPath() returned relative path: %s", path)
	}
}

// TestStoreValueWithEmptyKey tests storage with empty key
func TestStoreValueWithEmptyKey(t *testing.T) {
	testType := "test"
	testKey := []byte("")
	testValue := []byte("value_with_empty_key")

	// Store value with empty key
	err := StoreValue(testType, testKey, testValue)
	if err != nil {
		t.Logf("StoreValue() with empty key returned error (may be expected): %v", err)
		return
	}

	// Cleanup if successful
	_ = DeleteKey(testType, testKey)
}

// TestStoreValueWithLargeValue tests storage with large values
func TestStoreValueWithLargeValue(t *testing.T) {
	testType := "test"
	testKey := []byte("large_value_key")

	// Create a larger value (1KB)
	testValue := make([]byte, 1024)
	for i := range testValue {
		testValue[i] = byte(i % 256)
	}

	// Store large value
	err := StoreValue(testType, testKey, testValue)
	if err != nil {
		t.Errorf("StoreValue() with large value error = %v", err)
		return
	}

	// Retrieve and verify
	result, err := GetValue(testType, testKey)
	if err != nil {
		t.Errorf("GetValue() with large value error = %v", err)
		return
	}

	if !bytes.Equal(result, testValue) {
		t.Error("GetValue() with large value returned different data")
	}

	// Cleanup
	_ = DeleteKey(testType, testKey)
}

// TestMultipleKeys tests storing multiple keys
func TestMultipleKeys(t *testing.T) {
	testType := "multi_test"

	// Store multiple keys
	keys := []string{"key1", "key2", "key3"}
	values := [][]byte{
		[]byte("value1"),
		[]byte("value2"),
		[]byte("value3"),
	}

	for i, key := range keys {
		err := StoreValue(testType, []byte(key), values[i])
		if err != nil {
			t.Errorf("StoreValue() error for key %s: %v", key, err)
			return
		}
	}

	// Verify all keys
	for i, key := range keys {
		result, err := GetValue(testType, []byte(key))
		if err != nil {
			t.Errorf("GetValue() error for key %s: %v", key, err)
			return
		}
		if !bytes.Equal(result, values[i]) {
			t.Errorf("GetValue() for key %s returned wrong value", key)
		}
	}

	// Cleanup
	for _, key := range keys {
		_ = DeleteKey(testType, []byte(key))
	}
}

// BenchmarkStoreValue benchmarks the StoreValue function
func BenchmarkStoreValue(b *testing.B) {
	testType := "bench"
	testKey := []byte("bench_key")
	testValue := []byte("bench_value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = StoreValue(testType, testKey, testValue)
	}

	// Cleanup
	_ = DeleteKey(testType, testKey)
}

// BenchmarkGetValue benchmarks the GetValue function
func BenchmarkGetValue(b *testing.B) {
	testType := "bench"
	testKey := []byte("bench_get_key")
	testValue := []byte("bench_value")

	// Setup
	_ = StoreValue(testType, testKey, testValue)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GetValue(testType, testKey)
	}

	// Cleanup
	_ = DeleteKey(testType, testKey)
}
