package chat

import (
	"strings"
	"testing"
)

func TestParseStep(t *testing.T) {
	known := func(n string) bool { return n == "list_leads" || n == "set_stage" }
	tests := []struct {
		name     string
		raw      string
		wantTool string
		wantArgs string
		wantAns  string
		wantErr  string
	}{
		{name: "answer", raw: `{"answer":"Here are your leads."}`, wantAns: "Here are your leads."},
		{name: "tool with args", raw: `{"tool":"list_leads","args":{"limit":5}}`, wantTool: "list_leads", wantArgs: `{"limit":5}`},
		{name: "tool without args gets empty object", raw: `{"tool":"list_leads"}`, wantTool: "list_leads", wantArgs: `{}`},
		{name: "code fence is tolerated", raw: "```json\n{\"answer\":\"ok\"}\n```", wantAns: "ok"},
		{name: "surrounding whitespace", raw: "  \n{\"answer\":\"ok\"}\n ", wantAns: "ok"},
		{name: "unknown tool", raw: `{"tool":"drop_db","args":{}}`, wantErr: "unknown tool"},
		{name: "both tool and answer", raw: `{"tool":"list_leads","answer":"x"}`, wantErr: "exactly one"},
		{name: "neither", raw: `{}`, wantErr: "exactly one"},
		{name: "blank answer", raw: `{"answer":"   "}`, wantErr: "exactly one"},
		{name: "unknown field", raw: `{"answer":"x","thought":"y"}`, wantErr: "unknown field"},
		{name: "trailing object", raw: `{"answer":"x"} {"answer":"y"}`, wantErr: "trailing"},
		{name: "plain text", raw: `Sure! Here are your leads.`, wantErr: "JSON"},
		{name: "args must be an object", raw: `{"tool":"list_leads","args":[1]}`, wantErr: "args"},
		{name: "args null becomes empty", raw: `{"tool":"list_leads","args":null}`, wantTool: "list_leads", wantArgs: `{}`},
		{name: "empty reply", raw: ``, wantErr: "empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			st, err := ParseStep(tc.raw, known)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if st.Tool != tc.wantTool || st.Answer != tc.wantAns || (tc.wantTool != "" && string(st.Args) != tc.wantArgs) {
				t.Fatalf("got %+v (args %s)", st, st.Args)
			}
		})
	}
}
