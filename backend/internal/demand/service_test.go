package demand

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
)

type fakeStore struct {
	state    State
	asks     []Ask
	product  Product
	saved    int
	drafts   map[string]string
	draftErr error
}

func (f *fakeStore) LoadState(context.Context, string) (State, error) { return f.state, nil }
func (f *fakeStore) SaveState(_ context.Context, _ string, s State) error {
	f.state, f.saved = s, f.saved+1
	return nil
}
func (f *fakeStore) RecentAsks(context.Context, string) ([]Ask, error) { return f.asks, nil }
func (f *fakeStore) Product(context.Context, string) (Product, error)  { return f.product, nil }
func (f *fakeStore) CreateDraft(_ context.Context, _, mentionID, text string) error {
	if f.draftErr != nil {
		return f.draftErr
	}
	if f.drafts == nil {
		f.drafts = map[string]string{}
	}
	f.drafts[mentionID] = text
	return nil
}

type fakeModel struct {
	replies []string
	err     error
	seen    [][]ai.ChatMessage
}

func (m *fakeModel) Chat(_ context.Context, _ string, msgs []ai.ChatMessage) (string, error) {
	m.seen = append(m.seen, msgs)
	if m.err != nil {
		return "", m.err
	}
	return m.replies[min(len(m.seen)-1, len(m.replies)-1)], nil
}

type fakeCredits struct {
	spent, refunded int
	err             error
}

func (c *fakeCredits) Spend(_ context.Context, _ string, n int, _ string) error {
	if c.err != nil {
		return c.err
	}
	c.spent += n
	return nil
}
func (c *fakeCredits) Refund(_ context.Context, _ string, n int, _ string) { c.refunded += n }

func newService(st *fakeStore, m *fakeModel, c *fakeCredits) *Service {
	return &Service{Store: st, Model: m, Credits: c, Logger: zerolog.Nop()}
}

func TestRefreshGroupsNewAsks(t *testing.T) {
	st := &fakeStore{
		product: Product{Name: "Utkrusht"},
		asks:    []Ask{ask("m1", "screen for judgment", true), ask("m2", "AI in live interviews", false), ask("m3", "judgment not syntax", true)},
	}
	m := &fakeModel{replies: []string{`{"themes":[{"id":"","title":"Screening for judgment","summary":"s","coverage":"covered","asks":[0,2]},{"id":"","title":"AI in live interviews","summary":"s","coverage":"partial","asks":[1]}]}`}}
	n, err := newService(st, m, &fakeCredits{}).Refresh(context.Background(), "ws", t0)
	if err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(st.state.Themes) != 2 || st.state.Themes[0].People() != 2 || !st.state.LastRun.Equal(t0) || st.saved != 1 {
		t.Fatalf("state = %+v", st.state)
	}
	// Nothing new: no model call, nothing saved.
	n, err = newService(st, m, &fakeCredits{}).Refresh(context.Background(), "ws", t0)
	if err != nil || n != 0 || len(m.seen) != 1 || st.saved != 1 {
		t.Fatalf("second run: n=%d err=%v calls=%d saved=%d", n, err, len(m.seen), st.saved)
	}
}

func TestRefreshWaitsForEnoughAsksAndSurvivesBadOutput(t *testing.T) {
	st := &fakeStore{asks: []Ask{ask("m1", "one ask", true)}}
	m := &fakeModel{replies: []string{"not json"}}
	svc := newService(st, m, &fakeCredits{})
	if n, err := svc.Refresh(context.Background(), "ws", t0); err != nil || n != 0 || len(m.seen) != 0 {
		t.Fatalf("too few asks must not call the model: n=%d err=%v calls=%d", n, err, len(m.seen))
	}
	st.asks = []Ask{ask("m1", "a", true), ask("m2", "b", true), ask("m3", "c", true)}
	if _, err := svc.Refresh(context.Background(), "ws", t0); err == nil {
		t.Fatal("unparseable model output must be an error")
	}
	if st.saved != 0 || len(st.state.Themes) != 0 {
		t.Fatal("a failed run must not change the stored themes")
	}
}

func TestShipMarksThemeAndDraftsFollowUps(t *testing.T) {
	st := &fakeStore{product: Product{Name: "Utkrusht"}, state: State{Themes: []Theme{
		{ID: "t1", Title: "AI in live interviews", Status: StatusOpen, Asks: []Ask{ask("m1", "cannot tell who uses AI", true), ask("m2", "AI cheating", false)}},
	}}}
	m := &fakeModel{replies: []string{"You asked how to tell who is using AI. We now record it."}}
	c := &fakeCredits{}
	th, drafted, err := newService(st, m, c).Ship(context.Background(), "ws", "t1", "Live AI-usage tracking", true, t0)
	if err != nil {
		t.Fatal(err)
	}
	if th.Status != StatusShipped || th.ShippedNote != "Live AI-usage tracking" || !th.ShippedAt.Equal(t0) {
		t.Fatalf("theme = %+v", th)
	}
	if drafted != 2 || len(st.drafts) != 2 || c.spent != 2*FollowUpCost || c.refunded != 0 {
		t.Fatalf("drafted=%d drafts=%d spent=%d refunded=%d", drafted, len(st.drafts), c.spent, c.refunded)
	}
	prompt := m.seen[0][0].Content + m.seen[0][1].Content
	for _, want := range []string{"Live AI-usage tracking", "cannot tell who uses AI", "untrusted"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("follow-up prompt is missing %q", want)
		}
	}
	if st.state.Themes[0].Status != StatusShipped {
		t.Fatal("the shipped status must be saved")
	}
}

func TestShipWithoutDraftsAndErrors(t *testing.T) {
	st := &fakeStore{state: State{Themes: []Theme{{ID: "t1", Status: StatusOpen, Asks: []Ask{ask("m1", "x", true)}}}}}
	m := &fakeModel{replies: []string{"hi"}}
	c := &fakeCredits{}
	svc := newService(st, m, c)
	if _, n, err := svc.Ship(context.Background(), "ws", "t1", "note", false, t0); err != nil || n != 0 || len(m.seen) != 0 || c.spent != 0 {
		t.Fatalf("no drafts asked for: n=%d err=%v calls=%d spent=%d", n, err, len(m.seen), c.spent)
	}
	if _, _, err := svc.Ship(context.Background(), "ws", "nope", "note", true, t0); !errors.Is(err, ErrNoTheme) {
		t.Fatalf("unknown theme: %v", err)
	}
	// A draft that fails is refunded and does not stop the rest.
	st2 := &fakeStore{draftErr: errors.New("db down"), state: State{Themes: []Theme{{ID: "t1", Status: StatusOpen, Asks: []Ask{ask("m1", "x", true)}}}}}
	c2 := &fakeCredits{}
	if _, n, err := newService(st2, m, c2).Ship(context.Background(), "ws", "t1", "note", true, t0); err != nil || n != 0 || c2.spent != FollowUpCost || c2.refunded != FollowUpCost {
		t.Fatalf("failed draft: n=%d err=%v spent=%d refunded=%d", n, err, c2.spent, c2.refunded)
	}
	// Out of credits: the theme still ships, nothing is drafted.
	st3 := &fakeStore{state: State{Themes: []Theme{{ID: "t1", Status: StatusOpen, Asks: []Ask{ask("m1", "x", true)}}}}}
	if th, n, err := newService(st3, m, &fakeCredits{err: errors.New("no credits")}).Ship(context.Background(), "ws", "t1", "note", true, t0); err != nil || n != 0 || th.Status != StatusShipped {
		t.Fatalf("out of credits: n=%d err=%v status=%s", n, err, th.Status)
	}
}

// The daily scheduler asks Due before it claims a workspace's run, so a
// workspace with nothing to group is not marked as grouped.
func TestDueOnlyWhenEnoughAsksWait(t *testing.T) {
	st := &fakeStore{asks: []Ask{ask("m1", "a", true), ask("m2", "b", true)}}
	svc := newService(st, &fakeModel{}, &fakeCredits{})
	if n, err := svc.Waiting(context.Background(), "ws"); err != nil || n != 2 {
		t.Fatalf("waiting = %d, %v", n, err)
	}
	if due, err := svc.Due(context.Background(), "ws"); err != nil || due {
		t.Fatalf("two asks must not be due: %v %v", due, err)
	}
	st.asks = append(st.asks, ask("m3", "c", false))
	if due, err := svc.Due(context.Background(), "ws"); err != nil || !due {
		t.Fatalf("three asks must be due: %v %v", due, err)
	}
}
