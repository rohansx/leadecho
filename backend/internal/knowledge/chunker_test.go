package knowledge

import (
	"strings"
	"testing"
)

func TestChunk_paragraphs(t *testing.T) {
	content := "First paragraph about CRM pain points.\n\nSecond paragraph with more detail about team workflows and budget constraints."
	chunks := Chunk(content, 200, 40)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0] != content {
		t.Errorf("short doc should be single chunk, got %d chunks", len(chunks))
	}
}

func TestChunk_oversizedParagraph(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("word ")
	}
	chunks := Chunk(b.String(), 100, 20)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple hard-split chunks, got %d", len(chunks))
	}
}

func TestChunk_empty(t *testing.T) {
	if got := Chunk("   ", 800, 100); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
}
