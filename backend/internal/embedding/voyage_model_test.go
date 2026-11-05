package embedding

import "testing"

func TestVoyageModel(t *testing.T) {
	if got := New("k").model; got != "voyage-4-large" {
		t.Errorf("default model = %q, want voyage-4-large", got)
	}
	if got := New("k").WithModel("").model; got != "voyage-4-large" {
		t.Errorf("empty override = %q, want the default kept", got)
	}
	if got := New("k").WithModel("voyage-4").model; got != "voyage-4" {
		t.Errorf("override = %q, want voyage-4 (a workspace's chosen model must be used)", got)
	}
}
