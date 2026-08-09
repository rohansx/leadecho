package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStripCodeFencesSanitizesLLMJSON(t *testing.T) {
	// The exact payload NVIDIA Nemotron returned for clipxd.com — a // comment
	// after "competitors": [] that made encoding/json fail.
	raw := "```json\n" + `{
  "product_name": "Veyo",
  "competitors": [], // None mentioned or implied in the provided text
  "suggested_keywords": ["Veyo", "Screen Recording"],
  "homepage": "https://clipxd.com/path",
  "suggested_platforms": ["reddit", "hackernews",]
}` + "\n```"

	got := stripCodeFences(raw)
	if !json.Valid([]byte(got)) {
		t.Fatalf("sanitized output is not valid JSON:\n%s", got)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	// The // inside the URL string must survive.
	if m["homepage"] != "https://clipxd.com/path" {
		t.Fatalf("URL corrupted: %v", m["homepage"])
	}
	if !strings.Contains(got, "https://clipxd.com/path") {
		t.Fatalf("expected URL preserved, got:\n%s", got)
	}
}

func TestSanitizeJSONBlockCommentAndClean(t *testing.T) {
	cases := map[string]string{
		`{"a":1 /* note */}`:       `{"a":1}`, // ws before closer is trimmed too
		`{"a":1,}`:                 `{"a":1}`,
		`[1,2,3,]`:                 `[1,2,3]`,
		`{"u":"a // b, c"}`:        `{"u":"a // b, c"}`, // untouched inside string
		`{"a":1} // trailing note`: `{"a":1} `,
	}
	for in, want := range cases {
		if got := sanitizeJSON(in); got != want {
			t.Errorf("sanitizeJSON(%q) = %q, want %q", in, got, want)
		}
	}
}
