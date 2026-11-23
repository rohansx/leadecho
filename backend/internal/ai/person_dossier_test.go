package ai

import (
	"strings"
	"testing"
)

func TestParseDossierKeepsOnlyCitedAndAvailable(t *testing.T) {
	in := DossierInput{
		Channels: []string{"email", "linkedin"},
		Posts:    []DossierPost{{ID: "m1"}, {ID: "m2"}},
	}
	raw := "```json\n" + `{"headline":"CTO at Acme building data tools","about":"Runs a small team.","seniority":"Executive",
	"pains":[{"text":"slow CI","post_id":"m1"},{"text":"made up","post_id":"m9"}],
	"buying_signals":[{"text":"comparing vendors","post_id":"m2"}],
	"best_channel":"x","channel_reason":"active on X","opener":"Saw your CI post.",
	"expertise":["Go","Postgres"],"cautions":["identity partly confirmed"]}` + "\n```"
	d, err := parseDossier(raw, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Pains) != 1 || d.Pains[0].PostID != "m1" {
		t.Errorf("a pain citing a post we didn't give must be dropped: %+v", d.Pains)
	}
	if len(d.BuyingSignals) != 1 {
		t.Errorf("buying signal with a real citation kept: %+v", d.BuyingSignals)
	}
	if d.BestChannel != "" || d.ChannelReason != "" {
		t.Errorf("x is not an available channel, got %q", d.BestChannel)
	}
	if d.Seniority != "executive" {
		t.Errorf("seniority normalised, got %q", d.Seniority)
	}
}

func TestParseDossierRejectsEmptyAndUnknownSeniority(t *testing.T) {
	if _, err := parseDossier(`{"headline":""}`, DossierInput{}); err == nil {
		t.Error("an empty headline is not a dossier")
	}
	d, err := parseDossier(`{"headline":"Dev","seniority":"wizard","best_channel":"reply"}`, DossierInput{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Seniority != "unknown" || d.BestChannel != "reply" {
		t.Errorf("got seniority %q channel %q", d.Seniority, d.BestChannel)
	}
}

func TestDossierPromptCarriesPostsAndChannels(t *testing.T) {
	p := buildDossierPrompt(DossierInput{Product: "CI cache", Handle: "janed", Platform: "hackernews",
		Channels: []string{"email"}, Posts: []DossierPost{{ID: "m1", Title: "CI is slow", Excerpt: "our builds take 40 min"}}})
	for _, want := range []string{"CI cache", "janed", "AVAILABLE CHANNELS: email", "id=m1", "our builds take 40 min"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}
