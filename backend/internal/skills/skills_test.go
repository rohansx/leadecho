package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	md := "# Skill: daily-triage\n\nPick today's best threads.\n\n**Tools:** `search_mentions`, `mute`\n\n**Schedulable:** daily.\n\n## Prompt\n\n```\nTriage my inbox.\n1. step\n```\n"
	tests := []struct {
		name string
		got  func(Skill) string
		want string
	}{
		{"title", func(s Skill) string { return s.Title }, "daily-triage"},
		{"summary", func(s Skill) string { return s.Summary }, "Pick today's best threads."},
		{"tools", func(s Skill) string { return strings.Join(s.Tools, ",") }, "search_mentions,mute"},
		{"schedulable", func(s Skill) string { return s.Schedulable }, "daily."},
		{"prompt", func(s Skill) string { return s.Prompt }, "Triage my inbox.\n1. step"},
		{"content", func(s Skill) string { return s.Content }, md},
	}
	s := parse("daily-triage.md", md)
	if s.Name != "daily-triage" {
		t.Fatalf("name = %q", s.Name)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.got(s); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListShipsEverySkill(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 7 {
		t.Fatalf("want >= 7 skills, got %d", len(list))
	}
	for _, s := range list {
		if s.Name == "README" || s.Title == "" || s.Prompt == "" {
			t.Fatalf("incomplete skill %+v", s.Name)
		}
	}
}

// The embedded copies must match docs/skills (the source of truth).
func TestEmbeddedMatchesDocs(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "docs", "skills")
	docs, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil || len(docs) == 0 {
		t.Skip("docs/skills not present")
	}
	for _, p := range docs {
		want, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := files.ReadFile("files/" + filepath.Base(p))
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%s: embedded copy is stale; run: cp docs/skills/*.md backend/internal/skills/files/", filepath.Base(p))
		}
	}
}
