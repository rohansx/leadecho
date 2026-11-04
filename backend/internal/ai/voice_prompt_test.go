package ai

import (
	"strings"
	"testing"
)

func TestVoicePromptSection(t *testing.T) {
	tests := []struct {
		name    string
		opts    DraftReplyOptions
		want    []string
		wantNil bool
	}{
		{name: "no voice", opts: DraftReplyOptions{}, wantNil: true},
		{name: "voice only", opts: DraftReplyOptions{Voice: "- Tone: casual."}, want: []string{"VOICE PROFILE", "takes precedence", "- Tone: casual."}},
		{name: "voice and example", opts: DraftReplyOptions{Voice: "- Tone: casual.", StyleExample: "lol same, try X"},
			want: []string{"STYLE SAMPLE", "lol same, try X", "do not copy"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := voicePromptSection(tt.opts)
			if tt.wantNil {
				if got != "" {
					t.Fatalf("want empty, got %q", got)
				}
				return
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %q", w, got)
				}
			}
		})
	}
}
