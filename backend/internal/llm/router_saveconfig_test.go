package llm

import "testing"

// A config save must never clear stored provider API keys. Callers build the
// providers map from the public config, which omits api_key, so an overwrite
// used to wipe every credential — saving the routing table alone signed the
// workspace out of its own providers.
func TestSaveConfigProviderMergePreservesKeys(t *testing.T) {
	enabled := true
	existing := Config{
		Providers: map[string]ProviderSettings{
			"openai": {APIKey: "enc-openai", Enabled: &enabled},
			"voyage": {APIKey: "enc-voyage", Enabled: &enabled},
		},
	}
	// What the dashboard sends when saving routing: providers present, keys absent.
	incoming := Config{
		Providers: map[string]ProviderSettings{
			"openai": {Enabled: &enabled},
		},
	}

	merged := mergeProviderKeys(incoming, existing)

	if got := merged.Providers["openai"].APIKey; got != "enc-openai" {
		t.Fatalf("openai key was clobbered: got %q, want %q", got, "enc-openai")
	}
	if got := merged.Providers["voyage"].APIKey; got != "enc-voyage" {
		t.Fatalf("unmentioned provider was dropped: got %q, want %q", got, "enc-voyage")
	}
}

// An explicit new key must still win over the stored one.
func TestSaveConfigProviderMergeAcceptsNewKey(t *testing.T) {
	existing := Config{
		Providers: map[string]ProviderSettings{"openai": {APIKey: "old"}},
	}
	incoming := Config{
		Providers: map[string]ProviderSettings{"openai": {APIKey: "new"}},
	}

	merged := mergeProviderKeys(incoming, existing)

	if got := merged.Providers["openai"].APIKey; got != "new" {
		t.Fatalf("explicit key not applied: got %q, want %q", got, "new")
	}
}
