// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package backupcrypto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

const testPass = "correct horse battery staple"

type chunkRef struct{ off, n int }

// chunkOffsets parses the record layout of an NBBK2 envelope so tests can
// surgically delete, swap, or truncate real records.
func chunkOffsets(t *testing.T, env []byte) []chunkRef {
	t.Helper()
	pos := headerSize
	var refs []chunkRef
	for pos+4 <= len(env) {
		plainLen := int(binary.BigEndian.Uint32(env[pos : pos+4]))
		recStart := pos
		if plainLen == 0 {
			refs = append(refs, chunkRef{recStart, len(env) - recStart})
			return refs
		}
		recLen := 4 + nonceSize + plainLen + 16
		if pos+recLen > len(env) {
			t.Fatalf("malformed envelope at %d", pos)
		}
		refs = append(refs, chunkRef{recStart, recLen})
		pos += recLen
	}
	t.Fatal("missing footer")
	return nil
}

func TestRoundTrip(t *testing.T) {
	cases := map[string][]byte{
		"empty":       {},
		"one byte":    {0x42},
		"exact chunk": bytes.Repeat([]byte("a"), maxChunkSize),
		"multi chunk": bytes.Repeat([]byte("0123456789abcdef"), 230000), // ~3.7 MiB
	}
	for name, plain := range cases {
		t.Run(name, func(t *testing.T) {
			env, err := Encrypt(plain, testPass)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			got, err := Decrypt(env, testPass)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(got, plain) {
				t.Fatalf("round trip mismatch: got %d bytes, want %d", len(got), len(plain))
			}
		})
	}
}

func TestWrongPassphrase(t *testing.T) {
	env, err := Encrypt([]byte("secret payload"), testPass)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := Decrypt(env, "wrong passphrase"); !errors.Is(err, ErrInvalidFormat) {
		t.Fatalf("expected ErrInvalidFormat, got %v", err)
	}
}

func TestTamperDetection(t *testing.T) {
	plain := bytes.Repeat([]byte("0123456789abcdef"), 200000) // 5 chunks
	env, err := Encrypt(plain, testPass)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	refs := chunkOffsets(t, env)
	if len(refs) != 5 { // 4 data chunks + footer
		t.Fatalf("expected 4 chunks + footer, got %d records", len(refs))
	}

	t.Run("flip byte in last chunk", func(t *testing.T) {
		bad := append([]byte(nil), env...)
		lastData := refs[len(refs)-2]
		bad[lastData.off+lastData.n-1] ^= 0x01
		if _, err := Decrypt(bad, testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("flip byte in footer", func(t *testing.T) {
		bad := append([]byte(nil), env...)
		bad[len(bad)-1] ^= 0x01
		if _, err := Decrypt(bad, testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("delete middle chunk", func(t *testing.T) {
		bad := append([]byte(nil), env[:refs[2].off]...)
		bad = append(bad, env[refs[2].off+refs[2].n:]...)
		if _, err := Decrypt(bad, testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("delete last data chunk", func(t *testing.T) {
		last := refs[len(refs)-2]
		bad := append([]byte(nil), env[:last.off]...)
		bad = append(bad, env[last.off+last.n:]...)
		if _, err := Decrypt(bad, testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("swap two chunks", func(t *testing.T) {
		a, b := refs[1], refs[3]
		bad := make([]byte, 0, len(env))
		bad = append(bad, env[:a.off]...)
		bad = append(bad, env[b.off:b.off+b.n]...)
		bad = append(bad, env[a.off+a.n:b.off]...)
		bad = append(bad, env[a.off:a.off+a.n]...)
		bad = append(bad, env[b.off+b.n:]...)
		if _, err := Decrypt(bad, testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("drop footer", func(t *testing.T) {
		footer := refs[len(refs)-1]
		if _, err := Decrypt(env[:footer.off], testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("truncate mid header", func(t *testing.T) {
		if _, err := Decrypt(env[:headerSize-1], testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})

	t.Run("truncate mid chunk", func(t *testing.T) {
		mid := refs[1]
		if _, err := Decrypt(env[:mid.off+mid.n-1], testPass); !errors.Is(err, ErrInvalidFormat) {
			t.Fatalf("expected ErrInvalidFormat, got %v", err)
		}
	})
}

func TestEmptyPassphrase(t *testing.T) {
	if _, err := Encrypt([]byte("x"), ""); !errors.Is(err, ErrEmptyPassphrase) {
		t.Fatalf("expected ErrEmptyPassphrase, got %v", err)
	}
	if _, err := Decrypt([]byte("x"), ""); !errors.Is(err, ErrEmptyPassphrase) {
		t.Fatalf("expected ErrEmptyPassphrase, got %v", err)
	}
}

func TestUnsupportedVersion(t *testing.T) {
	env, err := Encrypt([]byte("x"), testPass)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	bad := append([]byte(nil), env...)
	bad[len(magic)] = 0x01
	if _, err := Decrypt(bad, testPass); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestStreamingWriterError(t *testing.T) {
	plain := bytes.Repeat([]byte("a"), maxChunkSize+10)
	limited := &limitedWriter{max: headerSize + 32}
	err := EncryptStream(limited, bytes.NewReader(plain), testPass)
	if err == nil {
		t.Fatal("expected writer error to surface")
	}
	if !strings.Contains(err.Error(), "write") {
		t.Fatalf("expected write error, got %v", err)
	}
}

func TestStreamingReaderError(t *testing.T) {
	src := &failingReader{data: bytes.Repeat([]byte("a"), 100), failAfter: 50}
	err := EncryptStream(io.Discard, src, testPass)
	if err == nil {
		t.Fatal("expected reader error to surface")
	}
}

type limitedWriter struct {
	max int
	n   int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	room := w.max - w.n
	if room <= 0 {
		return 0, errors.New("write limit reached")
	}
	if len(p) > room {
		p = p[:room]
	}
	w.n += len(p)
	return len(p), nil
}

type failingReader struct {
	data      []byte
	failAfter int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.failAfter <= 0 {
		return 0, errors.New("read failure")
	}
	if len(p) > r.failAfter {
		p = p[:r.failAfter]
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	r.failAfter -= n
	return n, nil
}
