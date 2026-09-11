package steam

import (
	"fmt"
	"strings"
	"testing"
)

func TestTailWriter_KeepsOnlyTail(t *testing.T) {
	tw := &tailWriter{cap: 10}
	// Write more than the cap across several writes.
	for i := 0; i < 5; i++ {
		if _, err := tw.Write([]byte("abcdef")); err != nil {
			t.Fatal(err)
		}
	}
	got := tw.String()
	if len(got) != 10 {
		t.Fatalf("tail length = %d, want cap 10", len(got))
	}
	// 30 bytes of "abcdef" repeated; the last 10 are "efabcdefab".
	if want := "abcdefabcdefabcdefabcdefabcdef"[20:]; got != want {
		t.Fatalf("tail = %q, want %q", got, want)
	}
}

func TestTailWriter_ShortInputKept(t *testing.T) {
	tw := &tailWriter{cap: 1 << 20}
	msg := "Success! App '896660' fully installed."
	if _, err := fmt.Fprint(tw, msg); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tw.String(), "Success!") {
		t.Fatalf("short output must be retained in full, got %q", tw.String())
	}
}
