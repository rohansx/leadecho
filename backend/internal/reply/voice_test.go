package reply

import (
	"strings"
	"testing"

	"leadecho/internal/database"
)

func TestVoiceInstructions(t *testing.T) {
	tests := []struct {
		name    string
		v       Voice
		want    []string
		notWant []string
	}{
		{
			name: "derived default",
			v:    DefaultVoice(),
			want: []string{"Goal: move the reader", "conversational", "2-4 sentences", "at most once"},
		},
		{
			name:    "casual short no plug",
			v:       Voice{Goal: "build_relationship", Tone: 5, Length: "short", Assertiveness: 10, BrandPlug: "none", CTAGoal: "book a demo"},
			want:    []string{"Goal: build goodwill", "casual", "1-2 sentences", "Suggest gently", "Do NOT mention the product"},
			notWant: []string{"call to action"},
		},
		{
			name: "formal long assertive high plug with cta",
			v:    Voice{Goal: "drive_conversion", Tone: 95, Length: "long", Assertiveness: 90, BrandPlug: "high", CTAGoal: "start a free trial"},
			want: []string{"formal", "two short paragraphs", "Be direct", "Recommend the product clearly", "call to action: start a free trial"},
		},
		{
			name:    "custom goal uses special instructions only",
			v:       Voice{Goal: "custom", Tone: 50, Length: "medium", Assertiveness: 50, BrandPlug: "medium", SpecialInstructions: "Always mention we are open source."},
			want:    []string{"Special instructions: Always mention we are open source.", "once by name"},
			notWant: []string{"Goal:"},
		},
		{
			name: "support goal",
			v:    Voice{Goal: "support", Tone: 50, Length: "medium", Assertiveness: 50, BrandPlug: "low"},
			want: []string{"Goal: solve the reader's problem"},
		},
		{
			name: "unknown values fall back to defaults",
			v:    Voice{Goal: "bogus", Tone: 50, Length: "huge", BrandPlug: "max"},
			want: []string{"Goal: move the reader", "2-4 sentences", "at most once"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VoiceInstructions(tt.v)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in:\n%s", w, got)
				}
			}
		})
	}
}

func TestExampleForPlatform(t *testing.T) {
	ex := map[string]string{"reddit": "r-sample", "x": "x-sample", "hackernews": "hn-sample"}
	tests := []struct{ platform, want string }{
		{"reddit", "r-sample"},
		{"twitter", "x-sample"},
		{"x", "x-sample"},
		{"hackernews", "hn-sample"},
		{"linkedin", ""},
		{"devto", ""},
	}
	for _, tt := range tests {
		if got := ExampleForPlatform(ex, tt.platform); got != tt.want {
			t.Errorf("ExampleForPlatform(%q) = %q, want %q", tt.platform, got, tt.want)
		}
	}
}

func TestValidateVoice(t *testing.T) {
	ok := DefaultVoice()
	tests := []struct {
		name    string
		mutate  func(v *Voice)
		wantErr string
	}{
		{"valid", func(v *Voice) {}, ""},
		{"bad goal", func(v *Voice) { v.Goal = "sell" }, "goal"},
		{"tone high", func(v *Voice) { v.Tone = 101 }, "tone"},
		{"assertiveness low", func(v *Voice) { v.Assertiveness = -1 }, "assertiveness"},
		{"bad length", func(v *Voice) { v.Length = "epic" }, "length"},
		{"bad plug", func(v *Voice) { v.BrandPlug = "max" }, "brand_plug"},
		{"bad platform key", func(v *Voice) { v.Examples = map[string]string{"myspace": "hi"} }, "examples"},
		{"example too long", func(v *Voice) { v.Examples = map[string]string{"reddit": strings.Repeat("a", 2001)} }, "examples"},
		{"bad url", func(v *Voice) { v.DestinationURL = "javascript:alert(1)" }, "destination_url"},
		{"good url", func(v *Voice) { v.DestinationURL = "https://acme.dev/pricing" }, ""},
		{"instructions too long", func(v *Voice) { v.SpecialInstructions = strings.Repeat("a", 2001) }, "special_instructions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := ok
			tt.mutate(&v)
			err := ValidateVoice(v)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want mention of %q", err, tt.wantErr)
			}
		})
	}
}

func TestVoiceFromProfile(t *testing.T) {
	p := database.ResponseProfile{
		Goal: "support", Tone: 70, Length: "short", Assertiveness: 20, BrandPlug: "none",
		CtaGoal: "reply", DestinationUrl: "https://a.dev", Examples: []byte(`{"reddit":"hey"}`),
	}
	v := VoiceFromProfile(p)
	if v.Goal != "support" || v.Tone != 70 || v.Length != "short" || v.Assertiveness != 20 ||
		v.BrandPlug != "none" || v.CTAGoal != "reply" || v.DestinationURL != "https://a.dev" || v.Examples["reddit"] != "hey" {
		t.Fatalf("unexpected voice %+v", v)
	}
	if got := VoiceFromProfile(database.ResponseProfile{Examples: []byte("not json")}); got.Examples == nil {
		t.Fatal("bad examples json should yield empty map, not nil")
	}
}

func TestLinkedReply(t *testing.T) {
	short := "https://le.ad/r/abc"
	tests := []struct {
		name, text, dest, want string
	}{
		{"replaces product url", "Try acme.dev for this.", "https://acme.dev", "Try " + short + " for this."},
		{"appends when brand named", "Acme handles this well.", "https://acme.dev", "Acme handles this well.\n\n" + short},
		{"appends even when product not named", "Use a queue.", "https://acme.dev", "Use a queue.\n\n" + short},
		{"bad destination still appends", "Use a queue.", "::nope", "Use a queue.\n\n" + short},
		{"already linked", "See " + short, "https://acme.dev", "See " + short},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LinkedReply(tt.text, tt.dest, short); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDerivedVoice(t *testing.T) {
	if got := DerivedVoice("Friendly CLI for indie hackers").Tone; got != DefaultVoice().Tone {
		t.Fatalf("casual brand tone = %d", got)
	}
	if got := DerivedVoice("SOC 2 compliance automation for enterprise security teams").Tone; got <= DefaultVoice().Tone {
		t.Fatalf("enterprise brand should be more formal, got %d", got)
	}
	v := DerivedVoice("")
	if v.Goal != "drive_conversion" || v.BrandPlug != "low" {
		t.Fatalf("derived default must be drive_conversion/low, got %+v", v)
	}
}
