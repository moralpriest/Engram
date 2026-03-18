// Copyright 2023-2024 DERO Foundation. All rights reserved.
//
// Use of this source code in any form is governed by RESEARCH license.
// license can be found in the LICENSE file.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND ANY
// EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
// MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL
// THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO,
// PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
// INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT,
// STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF
// THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/deroproject/graviton"
)

// Get Engram's working directory
func GetDir() (result string, err error) {
	result, err = os.Getwd()
	if err != nil {
		return
	}

	if runtime.GOOS == "darwin" {
		switch session.Network {
		case NETWORK_MAINNET:
			result = filepath.Join(AppPath(), "Contents", "Resources", "mainnet") + string(filepath.Separator)
		case NETWORK_SIMULATOR:
			result = filepath.Join(AppPath(), "Contents", "Resources", "testnet_simulator") + string(filepath.Separator)
		default:
			result = filepath.Join(AppPath(), "Contents", "Resources", "testnet") + string(filepath.Separator)
		}
	} else if runtime.GOOS == "android" {
		switch session.Network {
		case NETWORK_MAINNET:
			result = filepath.Join(AppPath(), "mainnet") + string(filepath.Separator)
		case NETWORK_SIMULATOR:
			result = filepath.Join(AppPath(), "testnet_simulator") + string(filepath.Separator)
		default:
			result = filepath.Join(AppPath(), "testnet") + string(filepath.Separator)
		}
	} else if runtime.GOOS == "ios" {
		switch session.Network {
		case NETWORK_MAINNET:
			result = filepath.Join(AppPath(), "mainnet") + string(filepath.Separator)
		case NETWORK_SIMULATOR:
			result = filepath.Join(AppPath(), "testnet_simulator") + string(filepath.Separator)
		default:
			result = filepath.Join(AppPath(), "testnet") + string(filepath.Separator)
		}
	} else {
		switch session.Network {
		case NETWORK_MAINNET:
			result = filepath.Join(AppPath(), "mainnet") + string(filepath.Separator)
		case NETWORK_SIMULATOR:
			result = filepath.Join(AppPath(), "testnet_simulator") + string(filepath.Separator)
		default:
			result = filepath.Join(AppPath(), "testnet") + string(filepath.Separator)
		}
	}

	return
}

// Get a datashard's path
// Get a datashard's path
func GetShard() (result string, err error) {
	// Always use the unified settings shard regardless of wallet state
	// This fixes the settings persistence issue where settings changed
	// when a wallet is open were saved to a wallet-specific shard
	// but read from the unified settings shard on restart
	result = filepath.Join(AppPath(), "datashards", "settings")
	return
}

// Encrypt a key-value and then store it in a Graviton tree
// Requires the user to have an active wallet open
func StoreEncryptedValue(t string, key []byte, value []byte) (err error) {
	if engram.Disk == nil {
		err = errors.New("error: no active account found")
		return
	}

	if t == "" {
		err = errors.New("error: missing graviton tree input")
		return
	} else if key == nil {
		err = errors.New("error: missing graviton key input")
		return
	}

	eValue, err := engram.Disk.Encrypt(value)
	if err != nil {
		return
	}

	shard, err := GetShard()
	if err != nil {
		return
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		return
	}

	ss, err := store.LoadSnapshot(0)
	if err != nil {
		return
	}

	tree, err := ss.GetTree(t)
	if err != nil {
		return
	}

	err = tree.Put(key, eValue)
	if err != nil {
		return
	}

	_, err = graviton.Commit(tree)
	if err != nil {
		return
	}

	return
}

// Store a key-value in a Graviton tree
func StoreValue(t string, key []byte, value []byte) (err error) {
	if t == "" {
		err = errors.New("error: missing graviton tree input")
		return
	} else if key == nil {
		err = errors.New("error: missing graviton key input")
		return
	}

	shard, err := GetShard()
	if err != nil {
		return
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		return
	}

	ss, err := store.LoadSnapshot(0)
	if err != nil {
		return
	}

	tree, err := ss.GetTree(t)
	if err != nil {
		return
	}

	err = tree.Put(key, value)
	if err != nil {
		return
	}

	_, err = graviton.Commit(tree)
	if err != nil {
		return
	}

	return
}

// Get a key-value from a Graviton tree
func GetValue(t string, key []byte) (result []byte, err error) {
	result = []byte("")

	if t == "" {
		err = errors.New("error: missing graviton tree input")
		return
	} else if key == nil {
		err = errors.New("error: missing graviton key input")
		return
	}

	shard, err := GetShard()
	if err != nil {
		return
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		return
	}

	ss, err := store.LoadSnapshot(0)
	if err != nil {
		return
	}

	tree, err := ss.GetTree(t)
	if err != nil {
		return
	}

	result, err = tree.Get(key)
	if err != nil {
		return
	}

	return
}

// Get an encrypted key-value from a Graviton tree and then decrypt it
// Requires the user to have an active wallet open
func GetEncryptedValue(t string, key []byte) (result []byte, err error) {
	result = []byte("")

	if engram.Disk == nil {
		err = errors.New("error: no active account found")
		return
	}

	if t == "" {
		err = errors.New("error: missing graviton tree input")
		return
	} else if key == nil {
		err = errors.New("error: missing graviton key input")
		return
	}

	shard, err := GetShard()
	if err != nil {
		return
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		return
	}

	ss, err := store.LoadSnapshot(0)

	if err != nil {
		return
	}

	tree, err := ss.GetTree(t)
	if err != nil {
		return
	}

	eValue, err := tree.Get(key)
	if err != nil {
		return
	}

	result, err = engram.Disk.Decrypt(eValue)
	if err != nil {
		return
	}

	return
}

// Delete a key-value in a Graviton tree
func DeleteKey(t string, key []byte) (err error) {
	if t == "" {
		err = errors.New("error: missing graviton tree input")
		return
	} else if key == nil {
		err = errors.New("error: missing graviton key input")
		return
	}

	shard, err := GetShard()
	if err != nil {
		return
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		return
	}

	ss, err := store.LoadSnapshot(0)
	if err != nil {
		return
	}

	tree, err := ss.GetTree(t)
	if err != nil {
		return
	}

	err = tree.Delete(key)
	if err != nil {
		return
	}

	_, err = graviton.Commit(tree)
	if err != nil {
		return
	}

	return
}

// TELA Favorites management
// Favorites are stored per-wallet using the wallet address as key

// AddTELAFavorite adds a SCID to the wallet's favorites with cached metadata
func AddTELAFavorite(walletAddress, scid, name, description, iconURL string, rating float64) error {
	if walletAddress == "" || scid == "" {
		return errors.New("wallet address and SCID required")
	}

	data := TELAFavoriteData{
		Name:        name,
		Description: description,
		IconURL:     iconURL,
		Rating:      rating,
		LastUpdated: time.Now().Unix(),
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	key := []byte("fav_" + walletAddress + "_" + scid)
	return StoreEncryptedValue("TELA Favorites", key, jsonData)
}

// RemoveTELAFavorite removes a SCID from the wallet's favorites
func RemoveTELAFavorite(walletAddress string, scid string) error {
	if walletAddress == "" || scid == "" {
		return errors.New("wallet address and SCID required")
	}

	key := []byte("fav_" + walletAddress + "_" + scid)

	return DeleteKey("TELA Favorites", key)
}

// IsTELAFavorite checks if a SCID is in the wallet's favorites
func IsTELAFavorite(walletAddress string, scid string) bool {
	if walletAddress == "" || scid == "" {
		return false
	}

	key := []byte("fav_" + walletAddress + "_" + scid)
	_, err := GetEncryptedValue("TELA Favorites", key)

	return err == nil
}

// GetTELAFavoriteData retrieves cached metadata for a favorited SCID
func GetTELAFavoriteData(walletAddress, scid string) (*TELAFavoriteData, error) {
	if walletAddress == "" || scid == "" {
		return nil, errors.New("wallet address and SCID required")
	}

	key := []byte("fav_" + walletAddress + "_" + scid)
	data, err := GetEncryptedValue("TELA Favorites", key)
	if err != nil {
		return nil, err
	}

	var favData TELAFavoriteData
	if err := json.Unmarshal(data, &favData); err != nil {
		return nil, err
	}

	return &favData, nil
}

// GetTELAFavorites returns all favorite SCIDs with their cached data
func GetTELAFavorites(walletAddress string) (map[string]*TELAFavoriteData, error) {
	result := make(map[string]*TELAFavoriteData)

	if walletAddress == "" {
		return result, nil
	}

	prefix := "fav_" + walletAddress + "_"

	shard, err := GetShard()
	if err != nil {
		return result, err
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		return result, err
	}

	ss, err := store.LoadSnapshot(0)
	if err != nil {
		return result, err
	}

	tree, err := ss.GetTree("TELA Favorites")
	if err != nil {
		return result, err
	}

	c := tree.Cursor()
	for k, v, err := c.First(); err == nil; k, v, err = c.Next() {
		keyStr := string(k)
		if len(keyStr) > len(prefix) && keyStr[:len(prefix)] == prefix {
			scid := keyStr[len(prefix):]
			decrypted, decryptErr := engram.Disk.Decrypt(v)
			if decryptErr == nil {
				var favData TELAFavoriteData
				if jsonErr := json.Unmarshal(decrypted, &favData); jsonErr == nil {
					result[scid] = &favData
				} else {
					result[scid] = &TELAFavoriteData{Name: string(decrypted)}
				}
			}
		}
	}

	return result, nil
}

// AddContact stores a contact for the current wallet.
func AddContact(walletAddress, contact string) error {
	if walletAddress == "" || contact == "" {
		return errors.New("wallet address and contact required")
	}

	key := []byte("contact_" + walletAddress + "_" + contact)
	return StoreEncryptedValue("Contacts", key, []byte(contact))
}

// RemoveContact deletes a stored contact for the current wallet.
func RemoveContact(walletAddress, contact string) error {
	if walletAddress == "" || contact == "" {
		return errors.New("wallet address and contact required")
	}

	key := []byte("contact_" + walletAddress + "_" + contact)
	return DeleteKey("Contacts", key)
}

// IsContact checks if a contact is already stored for the current wallet.
func IsContact(walletAddress, contact string) bool {
	if walletAddress == "" || contact == "" {
		return false
	}

	key := []byte("contact_" + walletAddress + "_" + contact)
	_, err := GetEncryptedValue("Contacts", key)
	return err == nil
}

// GetContacts returns all stored contacts for the current wallet.
func GetContacts(walletAddress string) ([]string, error) {
	result := []string{}
	if walletAddress == "" {
		return result, nil
	}

	prefix := "contact_" + walletAddress + "_"

	shard, err := GetShard()
	if err != nil {
		return result, err
	}

	store, err := graviton.NewDiskStore(shard)
	if err != nil {
		return result, err
	}

	ss, err := store.LoadSnapshot(0)
	if err != nil {
		return result, err
	}

	tree, err := ss.GetTree("Contacts")
	if err != nil {
		return result, nil
	}

	c := tree.Cursor()
	for k, v, err := c.First(); err == nil; k, v, err = c.Next() {
		keyStr := string(k)
		if !strings.HasPrefix(keyStr, prefix) {
			continue
		}

		decrypted, decryptErr := engram.Disk.Decrypt(v)
		if decryptErr != nil {
			continue
		}

		contact := strings.TrimSpace(string(decrypted))
		if contact == "" {
			continue
		}

		result = append(result, contact)
	}

	sort.Strings(result)
	return result, nil
}
