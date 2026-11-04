package apikey

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	k1, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	k2, _ := Generate()
	if k1.Plaintext == k2.Plaintext {
		t.Fatal("keys must be random")
	}
	if !strings.HasPrefix(k1.Plaintext, Prefix) {
		t.Fatalf("want %q prefix, got %q", Prefix, k1.Plaintext)
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(k1.Plaintext, Prefix))
	if err != nil || len(raw) != 32 {
		t.Fatalf("want 32 base64url bytes, got %d (%v)", len(raw), err)
	}
	if k1.Hash != Hash(k1.Plaintext) || len(k1.Hash) != 64 {
		t.Fatalf("hash mismatch: %q", k1.Hash)
	}
	if !strings.HasPrefix(k1.Plaintext, k1.Display) || len(k1.Display) != len(Prefix)+displayChars {
		t.Fatalf("bad display prefix %q", k1.Display)
	}
}

func TestLooksLikeKey(t *testing.T) {
	k, _ := Generate()
	tests := []struct {
		in   string
		want bool
	}{
		{k.Plaintext, true},
		{"", false},
		{"le_live_", false},
		{"le_live_short", false},
		{"sk_live_" + strings.TrimPrefix(k.Plaintext, Prefix), false},
		{k.Plaintext + "!", false},
		{strings.Repeat("a", 500), false},
	}
	for _, tt := range tests {
		if got := LooksLikeKey(tt.in); got != tt.want {
			t.Errorf("LooksLikeKey(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestBearer(t *testing.T) {
	tests := []struct{ header, want string }{
		{"Bearer le_live_abc", "le_live_abc"},
		{"bearer le_live_abc", "le_live_abc"},
		{"Bearer   le_live_abc  ", "le_live_abc"},
		{"Basic abc", ""},
		{"", ""},
		{"Bearer", ""},
	}
	for _, tt := range tests {
		if got := Bearer(tt.header); got != tt.want {
			t.Errorf("Bearer(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestNormalizeScopes(t *testing.T) {
	tests := []struct {
		in      []string
		want    []string
		wantErr bool
	}{
		{nil, []string{ScopeRead}, false},
		{[]string{"read"}, []string{ScopeRead}, false},
		{[]string{"write"}, []string{ScopeRead, ScopeWrite}, false},
		{[]string{"write", "read", "write"}, []string{ScopeRead, ScopeWrite}, false},
		{[]string{"admin"}, nil, true},
		{[]string{""}, nil, true},
	}
	for _, tt := range tests {
		got, err := NormalizeScopes(tt.in)
		if (err != nil) != tt.wantErr {
			t.Fatalf("NormalizeScopes(%v) err = %v", tt.in, err)
		}
		if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
			t.Errorf("NormalizeScopes(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestHasScope(t *testing.T) {
	if !HasScope([]string{"read", "write"}, "write") || HasScope([]string{"read"}, "write") || HasScope(nil, "read") {
		t.Fatal("HasScope wrong")
	}
}
