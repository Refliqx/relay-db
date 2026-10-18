// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package bcryptdet provides bcrypt with a DETERMINISTIC SALT.
//
// # WHY THIS PACKAGE EXISTS
//
// `golang.org/x/crypto/bcrypt` always generates the salt from crypto/rand, so
// the hash for the same password DIFFERS in every process, and therefore the
// seed file differs too. RelayDB promises byte-identical output for the same
// `seed` (see package mask and docs/ARCHITECTURE.md), so the salt must be
// derived from the seed, not from an RNG.
//
// # CODE ORIGIN
//
// This implementation is a faithful copy of `golang.org/x/crypto/bcrypt`
// v0.58.0 (`bcrypt.go` + `base64.go`), with one change: the salt is no longer
// generated inside, but accepted as an argument.
//
// Hash verification still uses the upstream package
// (`bcrypt.CompareHashAndPassword`), and that interoperability is cross-tested
// in bcryptdet_test.go. The output format stays `$2a$<cost>$<salt22><hash31>`,
// so any application using standard bcrypt can consume it.
package bcryptdet

import (
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/blowfish"
)

const (
	// MinCost is the same as bcrypt.MinCost.
	MinCost = 4
	// MaxCost is the same as bcrypt.MaxCost.
	MaxCost = 31
	// SaltSize is the expected length of the raw (pre-encoded) salt.
	SaltSize = 16

	majorVersion       = '2'
	minorVersion       = 'a'
	maxCryptedHashSize = 23
	encodedSaltSize    = 22
	// hashLen is the total length of the produced bcrypt hash ("$2a$NN$" +
	// 22 salt characters + 31 hash characters).
	hashLen = 60
)

// magicCipherData is the IV for the 64 Blowfish encryption rounds in hash().
// Its contents are the string "OrpheanBeholderScryDoubt" in big-endian.
var magicCipherData = []byte{
	0x4f, 0x72, 0x70, 0x68,
	0x65, 0x61, 0x6e, 0x42,
	0x65, 0x68, 0x6f, 0x6c,
	0x64, 0x65, 0x72, 0x53,
	0x63, 0x72, 0x79, 0x44,
	0x6f, 0x75, 0x62, 0x74,
}

// GenerateFromPasswordSalt produces a bcrypt hash of `password` with a raw
// `salt` of SaltSize bytes.
//
// The salt is the ONLY source of randomness, so this function's output is
// fully deterministic for (password, cost, salt); an invariant the
// reproducible-export guarantee relies on.
func GenerateFromPasswordSalt(password []byte, cost int, salt []byte) ([]byte, error) {
	if len(password) > 72 {
		return nil, fmt.Errorf("bcrypt: password exceeds 72 bytes")
	}
	if len(salt) != SaltSize {
		return nil, fmt.Errorf("bcrypt: salt must be %d bytes, got %d", SaltSize, len(salt))
	}
	if cost < MinCost || cost > MaxCost {
		return nil, fmt.Errorf("bcrypt: cost %d out of range %d..%d", cost, MinCost, MaxCost)
	}

	hash, err := hashWithSalt(password, cost, base64Encode(salt))
	if err != nil {
		return nil, err
	}

	out := make([]byte, 0, hashLen)
	out = append(out, '$', majorVersion, minorVersion, '$')
	out = append(out, fmt.Sprintf("%02d", cost)...)
	out = append(out, '$')
	out = append(out, base64Encode(salt)...)
	out = append(out, hash...)
	return out, nil
}

// hashWithSalt is the core of bcrypt(): the expensive Blowfish setup, then 64
// rounds of IV encryption. `salt` here is already in encoded form.
func hashWithSalt(password []byte, cost int, salt []byte) ([]byte, error) {
	cipherData := make([]byte, len(magicCipherData))
	copy(cipherData, magicCipherData)

	c, err := expensiveBlowfishSetup(password, uint32(cost), salt)
	if err != nil {
		return nil, err
	}

	for i := 0; i < 24; i += 8 {
		for j := 0; j < 64; j++ {
			c.Encrypt(cipherData[i:i+8], cipherData[i:i+8])
		}
	}

	// Bug compatibility with the C bcrypt implementation: only 23 of the 24
	// bytes are encoded.
	return base64Encode(cipherData[:maxCryptedHashSize]), nil
}

func expensiveBlowfishSetup(key []byte, cost uint32, salt []byte) (*blowfish.Cipher, error) {
	csalt, err := base64Decode(salt)
	if err != nil {
		return nil, err
	}

	// Bug compatibility with the C bcrypt implementation: the trailing NULL
	// byte of the key is used during expansion too. The key is copied so the
	// caller's array is not modified.
	ckey := append(key[:len(key):len(key)], 0)

	c, err := blowfish.NewSaltedCipher(ckey, csalt)
	if err != nil {
		return nil, err
	}

	rounds := uint64(1) << cost
	for i := uint64(0); i < rounds; i++ {
		blowfish.ExpandKey(ckey, c)
		blowfish.ExpandKey(csalt, c)
	}

	return c, nil
}

// bcrypt's own base64 (its alphabet differs from standard base64)

const alphabet = "./ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

var bcEncoding = base64.NewEncoding(alphabet)

func base64Encode(src []byte) []byte {
	n := bcEncoding.EncodedLen(len(src))
	dst := make([]byte, n)
	bcEncoding.Encode(dst, src)
	for dst[n-1] == '=' {
		n--
	}
	return dst[:n]
}

func base64Decode(src []byte) ([]byte, error) {
	numOfEquals := 4 - (len(src) % 4)
	for i := 0; i < numOfEquals; i++ {
		src = append(src, '=')
	}

	dst := make([]byte, bcEncoding.DecodedLen(len(src)))
	n, err := bcEncoding.Decode(dst, src)
	if err != nil {
		return nil, err
	}
	return dst[:n], nil
}

// EncodedSaltSize is useful for callers that want to assemble/parse a hash
// themselves; it is exported so the internal constant need not be.
const EncodedSaltSize = encodedSaltSize
