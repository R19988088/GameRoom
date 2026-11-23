//go:build windows

package core

import (
	"bytes"
	"testing"
)

// TestTrimStateCutsWhatTheCoreLeftZero: a core declares the block it could fill
// and fills less of it, because the block is cleared before the call; the state
// is the bytes up to the last one it wrote, and nothing else.
func TestTrimStateCutsWhatTheCoreLeftZero(t *testing.T) {
	buf := make([]byte, 1024)
	copy(buf, []byte{1, 2, 3, 4, 5})
	out := trimState(buf)
	if len(out) != 5 {
		t.Fatalf("a state that ends at byte 5 came out %d bytes", len(out))
	}
	if !bytes.Equal(out, []byte{1, 2, 3, 4, 5}) {
		t.Fatalf("the trimmed state is not the bytes the core wrote")
	}

	// A state whose own last bytes are zero keeps them: they are part of what was
	// written, only the untouched tail goes.
	sparse := make([]byte, 1024)
	sparse[10] = 7
	if out := trimState(sparse); len(out) != 11 {
		t.Fatalf("a state with a hole came out %d bytes, want 11 (up to the last byte written)", len(out))
	}

	if out := trimState(make([]byte, 64)); out != nil {
		t.Fatalf("an all-zero block came out as %d bytes, want no state at all", len(out))
	}
}
