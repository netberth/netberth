// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

// Package backupcrypto implements optional passphrase encryption for NetBerth
// database backups: AES-256-GCM with Argon2id key derivation, streamed in
// fixed-size chunks (format NBBK2).
//
// Integrity: every data chunk is authenticated together with its sequence
// number (AAD), and a final authenticated footer binds the total chunk count.
// Deleting, reordering, truncating, or modifying any byte therefore fails
// decryption with the same error as a wrong passphrase.
package backupcrypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	magic         = "NBBK"
	version       = 0x02
	saltSize      = 16
	nonceSize     = 12
	maxChunkSize  = 1 << 20 // 1 MiB plaintext per chunk
	argon2Time    = 3
	argon2Memory  = 64 * 1024 // KiB
	argon2Threads = 4
	keySize       = 32
	footerIndex   = ^uint64(0) // 0xFFFFFFFFFFFFFFFF, never a real chunk index
	headerSize    = len(magic) + 1 + saltSize
)

var (
	// ErrEmptyPassphrase is returned when no passphrase is supplied.
	ErrEmptyPassphrase = errors.New("backupcrypto: empty passphrase")
	// ErrInvalidFormat is returned for malformed, tampered, or wrongly
	// passphrased data. It intentionally does not distinguish these cases.
	ErrInvalidFormat = errors.New("backupcrypto: invalid encrypted backup")
	// ErrUnsupportedVersion is returned when the envelope uses a future
	// format version.
	ErrUnsupportedVersion = errors.New("backupcrypto: unsupported backup version")
)

// EncryptStream reads src in 1 MiB chunks and writes an NBBK2 envelope to dst.
// The passphrase must be non-empty; minimum-length policy lives at the API
// layer.
func EncryptStream(dst io.Writer, src io.Reader, passphrase string) error {
	if passphrase == "" {
		return ErrEmptyPassphrase
	}

	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("backupcrypto: salt: %w", err)
	}
	aead, err := newAEAD(passphrase, salt)
	if err != nil {
		return err
	}

	header := make([]byte, 0, headerSize)
	header = append(header, magic...)
	header = append(header, version)
	header = append(header, salt...)
	if _, err := dst.Write(header); err != nil {
		return fmt.Errorf("backupcrypto: header: %w", err)
	}

	buf := make([]byte, maxChunkSize)
	var index uint64
	for {
		n, readErr := io.ReadFull(src, buf)
		if n > 0 {
			if err := writeChunk(aead, dst, buf[:n], index); err != nil {
				return err
			}
			index++
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("backupcrypto: read: %w", readErr)
		}
	}

	return writeFooter(aead, dst, index)
}

// DecryptStream reads an NBBK2 envelope from src and writes the plaintext to
// dst. A wrong passphrase, truncated input, or any byte modification returns
// ErrInvalidFormat.
func DecryptStream(dst io.Writer, src io.Reader, passphrase string) error {
	if passphrase == "" {
		return ErrEmptyPassphrase
	}

	header := make([]byte, headerSize)
	if _, err := io.ReadFull(src, header); err != nil {
		return ErrInvalidFormat
	}
	if string(header[:len(magic)]) != magic {
		return ErrInvalidFormat
	}
	if header[len(magic)] != version {
		return ErrUnsupportedVersion
	}
	aead, err := newAEAD(passphrase, header[len(magic)+1:])
	if err != nil {
		return err
	}

	var index uint64
	var lenBuf [4]byte
	for {
		if _, err := io.ReadFull(src, lenBuf[:]); err != nil {
			return ErrInvalidFormat // stream ended without an authenticated footer
		}
		plainLen := binary.BigEndian.Uint32(lenBuf[:])
		if plainLen == 0 {
			return readFooter(aead, src, index)
		}
		if plainLen > maxChunkSize {
			return ErrInvalidFormat
		}

		nonce := make([]byte, nonceSize)
		if _, err := io.ReadFull(src, nonce); err != nil {
			return ErrInvalidFormat
		}
		sealed := make([]byte, int(plainLen)+aead.Overhead())
		if _, err := io.ReadFull(src, sealed); err != nil {
			return ErrInvalidFormat
		}
		plain, err := aead.Open(nil, nonce, sealed, chunkAAD(index))
		if err != nil {
			return ErrInvalidFormat
		}
		if _, err := dst.Write(plain); err != nil {
			return fmt.Errorf("backupcrypto: write: %w", err)
		}
		index++
	}
}

// Encrypt is the in-memory convenience wrapper around EncryptStream.
func Encrypt(plain []byte, passphrase string) ([]byte, error) {
	var out bytes.Buffer
	if err := EncryptStream(&out, bytes.NewReader(plain), passphrase); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Decrypt is the in-memory convenience wrapper around DecryptStream.
func Decrypt(envelope []byte, passphrase string) ([]byte, error) {
	var out bytes.Buffer
	if err := DecryptStream(&out, bytes.NewReader(envelope), passphrase); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func newAEAD(passphrase string, salt []byte) (cipher.AEAD, error) {
	key := argon2.IDKey([]byte(passphrase), salt, argon2Time, argon2Memory, argon2Threads, keySize)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("backupcrypto: gcm: %w", err)
	}
	return aead, nil
}

func chunkAAD(index uint64) []byte {
	aad := make([]byte, 0, len(magic)+1+8)
	aad = append(aad, magic...)
	aad = append(aad, version)
	var idx [8]byte
	binary.BigEndian.PutUint64(idx[:], index)
	aad = append(aad, idx[:]...)
	return aad
}

func footerAAD() []byte {
	return chunkAAD(footerIndex)
}

func writeChunk(aead cipher.AEAD, w io.Writer, plain []byte, index uint64) error {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("backupcrypto: nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, plain, chunkAAD(index))

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(plain)))
	if _, err := w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("backupcrypto: chunk length: %w", err)
	}
	if _, err := w.Write(nonce); err != nil {
		return fmt.Errorf("backupcrypto: chunk nonce: %w", err)
	}
	if _, err := w.Write(sealed); err != nil {
		return fmt.Errorf("backupcrypto: chunk data: %w", err)
	}
	return nil
}

func writeFooter(aead cipher.AEAD, w io.Writer, chunkCount uint64) error {
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("backupcrypto: footer nonce: %w", err)
	}
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], chunkCount)
	sealed := aead.Seal(nil, nonce, count[:], footerAAD())

	var lenBuf [4]byte // 0 marks the authenticated footer
	if _, err := w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("backupcrypto: footer marker: %w", err)
	}
	if _, err := w.Write(nonce); err != nil {
		return fmt.Errorf("backupcrypto: footer nonce write: %w", err)
	}
	if _, err := w.Write(sealed); err != nil {
		return fmt.Errorf("backupcrypto: footer data: %w", err)
	}
	return nil
}

func readFooter(aead cipher.AEAD, src io.Reader, chunkCount uint64) error {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(src, nonce); err != nil {
		return ErrInvalidFormat
	}
	sealed := make([]byte, 8+aead.Overhead())
	if _, err := io.ReadFull(src, sealed); err != nil {
		return ErrInvalidFormat
	}
	plain, err := aead.Open(nil, nonce, sealed, footerAAD())
	if err != nil {
		return ErrInvalidFormat
	}
	if binary.BigEndian.Uint64(plain) != chunkCount {
		return ErrInvalidFormat
	}
	return nil
}
