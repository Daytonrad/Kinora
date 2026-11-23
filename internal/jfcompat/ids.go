package jfcompat

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"strings"
)

// Jellyfin clients use 32-hex-digit (GUID-like) ids. Kinora's are small integers, so an id is one AES block
// holding a namespace byte and the integer, encrypted with a key kept in the database: the same object gets
// the same id after a restart, the id can be decoded without a lookup table, and ids cannot be guessed or
// enumerated (the artwork endpoint answers without a token, as Jellyfin clients expect).

// Namespaces of ids.
const (
	nsItem    byte = 1
	nsLibrary byte = 2
	nsUser    byte = 3 // a Kinora profile is a Jellyfin user
	nsServer  byte = 4
)

// keySetting holds the AES key (hex) in the settings table.
const keySetting = "jfcompat_id_key"

type ids struct{ block cipher.Block }

// loadKey returns the stored key, creating it on first use.
func loadKey(get func(string) string, set func(string, string) error) ([]byte, error) {
	if k, err := hex.DecodeString(get(keySetting)); err == nil && len(k) == 16 {
		return k, nil
	}
	k := make([]byte, 16)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, set(keySetting, hex.EncodeToString(k))
}

func newIDs(key []byte) (*ids, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return &ids{block: b}, nil
}

// encode makes the id of (namespace, n).
func (x *ids) encode(ns byte, n int64) string {
	var in, out [16]byte
	in[0] = ns
	binary.BigEndian.PutUint64(in[8:], uint64(n))
	x.block.Encrypt(out[:], in[:])
	return hex.EncodeToString(out[:])
}

// decode reads an id of namespace ns; ok is false for anything else (a guessed or foreign id fails the
// check of the seven zero bytes with probability 1 - 2^-56).
func (x *ids) decode(ns byte, s string) (int64, bool) {
	s = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "") // GUID form with dashes too
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 16 {
		return 0, false
	}
	var out [16]byte
	x.block.Decrypt(out[:], b)
	if out[0] != ns {
		return 0, false
	}
	for _, z := range out[1:8] {
		if z != 0 {
			return 0, false
		}
	}
	n := int64(binary.BigEndian.Uint64(out[8:]))
	return n, n > 0
}
