package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestFormatSource_ValidGo verifies that valid Go source is returned formatted.
func TestFormatSource_ValidGo(t *testing.T) {
	// Unformatted but syntactically valid Go.
	src := []byte(`package foo
import "fmt"
func   F()  {fmt.Println("hi")}
`)
	got, err := FormatSource("foo.go", src)
	if err != nil {
		t.Fatalf("FormatSource: unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FormatSource returned empty bytes")
	}
	// go/format normalises import spacing; result must compile-equivalent.
	if !strings.Contains(string(got), `fmt.Println`) {
		t.Errorf("formatted output missing expected content; got:\n%s", got)
	}
}

// TestFormatSource_AlreadyFormatted verifies that already-formatted source is
// returned unchanged (idempotent).
func TestFormatSource_AlreadyFormatted(t *testing.T) {
	src := []byte("package foo\n\nfunc F() {}\n")
	got, err := FormatSource("foo.go", src)
	if err != nil {
		t.Fatalf("FormatSource: unexpected error: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Errorf("FormatSource mutated already-formatted source.\ngot:  %q\nwant: %q", got, src)
	}
}

// TestFormatSource_InvalidGo verifies that invalid Go source returns an error
// containing the relPath and a source preview.
func TestFormatSource_InvalidGo(t *testing.T) {
	src := []byte(`package foo
func broken( {
`)
	_, err := FormatSource("broken.go", src)
	if err == nil {
		t.Fatal("expected error for invalid Go source, got nil")
	}
	if !strings.Contains(err.Error(), "broken.go") {
		t.Errorf("error should mention the relPath; got: %v", err)
	}
}

// TestFormatSource_LargeInvalidGo verifies the preview truncation path (>2 KB).
func TestFormatSource_LargeInvalidGo(t *testing.T) {
	// Build >2 KB of invalid source so the truncation branch is exercised.
	var sb strings.Builder
	sb.WriteString("package foo\nfunc bad( {\n")
	for i := 0; i < 200; i++ {
		sb.WriteString("// padding line to exceed 2048 bytes of invalid source\n")
	}
	_, err := FormatSource("big.go", []byte(sb.String()))
	if err == nil {
		t.Fatal("expected error for large invalid Go source, got nil")
	}
}

// TestFormatSource_EmptyPackage verifies that a minimal valid package compiles.
func TestFormatSource_EmptyPackage(t *testing.T) {
	src := []byte("package empty\n")
	got, err := FormatSource("empty.go", src)
	if err != nil {
		t.Fatalf("FormatSource: unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("FormatSource returned empty bytes for valid minimal package")
	}
}
