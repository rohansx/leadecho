package llm

import "testing"

// voyage-4-large: best retrieval quality, 1024 dims (matches vector(1024)),
// and the voyage-4 family includes 200M free tokens (voyage-3 has none).
func TestDefaultEmbeddingModel(t *testing.T) {
	if got := DefaultConfig().Models[TierEmbedding]; got.Provider != "voyage" || got.Model != "voyage-4-large" {
		t.Fatalf("default embedding = %+v, want voyage/voyage-4-large", got)
	}
}
