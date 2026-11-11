package demand

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"leadecho/internal/ai"
)

type fakeStore struct {
	mu       sync.Mutex
	state    State
	asks     []Ask
	product  Product
	saved    int
	drafts   map[string]string
	draftErr error
}

func (f *fakeStore) LoadState(context.Context, string) (State, error) { return f.state, nil }
func (f *fakeStore) UpdateState(_ context.Context, _ string, fn func(State) (State, error)) (State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	next, err := fn(f.state)
	if err != nil {
		return State{}, err
	}
	f.state, f.saved = next, f.saved+1
	return next, nil
}
func (f *fakeStore) RecentAsks(context.Context, string) ([]Ask, error) { return f.asks, nil }
func (f *fakeStore) Product(context.Context, string) (Product, error)  { return f.product, nil }
func (f *fakeStore) CreateDraft(_ context.Context, _, mentionID, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	mu      sync.Mutex
	replies []string
	err     error
	seen    [][]ai.ChatMessage
	onChat  func() // runs during the call: something else changing the state meanwhile
}

func (m *fakeModel) Chat(_ context.Context, _ string, msgs []ai.ChatMessage) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.onChat != nil {
		m.onChat()
	}
	m.seen = append(m.seen, msgs)
	if m.err != nil {
		return "", m.err
	}
	return m.replies[min(len(m.seen)-1, len(m.replies)-1)], nil
}

type fakeCredits struct {
	mu              sync.Mutex
	spent, refunded int
	err             error
}

func (c *fakeCredits) Spend(_ context.Context, _ string, n int, _ string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	c.spent += n
	return nil
}
func (c *fakeCredits) Refund(_ context.Context, _ string, n int, _ string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refunded += n
}

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
		{ID: "t1", Title: "AI in live interviews", Status: StatusOpen, Asks: []Ask{ask("m1", "cannot tell who uses AI", true), ask("m2", "AI cheating", true), ask("m3", "role unknown", false)}},
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
	prompt := ""
	for _, msgs := range m.seen {
		prompt += msgs[0].Content + msgs[1].Content
	}
	if _, ok := st.drafts["m3"]; ok {
		t.Fatal("only buyers get a follow-up")
	}
	if st.state.Themes[0].Drafted != 2 {
		t.Fatalf("the draft count must be saved on the theme, got %d", st.state.Themes[0].Drafted)
	}
	for _, want := range []string{"Live AI-usage tracking", "cannot tell who uses AI", "untrusted", "first person", "SKIP"} {
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

// A second click, a retry after a timeout, or a script: the theme is already
// shipped, so nothing is charged or drafted again.
func TestShipTwiceIsRefused(t *testing.T) {
	st := &fakeStore{state: State{Themes: []Theme{{ID: "t1", Status: StatusOpen, Asks: []Ask{ask("m1", "x", true)}}}}}
	m := &fakeModel{replies: []string{"You asked about x. It exists now."}}
	c := &fakeCredits{}
	svc := newService(st, m, c)
	if _, n, err := svc.Ship(context.Background(), "ws", "t1", "first note", true, t0); err != nil || n != 1 {
		t.Fatalf("first ship: n=%d err=%v", n, err)
	}
	if _, _, err := svc.Ship(context.Background(), "ws", "t1", "second note", true, t0); !errors.Is(err, ErrShipped) {
		t.Fatalf("second ship must be refused, got %v", err)
	}
	if c.spent != FollowUpCost || len(m.seen) != 1 || st.state.Themes[0].ShippedNote != "first note" {
		t.Fatalf("second ship changed something: spent=%d calls=%d note=%q", c.spent, len(m.seen), st.state.Themes[0].ShippedNote)
	}
}

// The grouping call takes seconds. A theme shipped during it must stay shipped.
func TestRefreshKeepsAShipMadeMeanwhile(t *testing.T) {
	st := &fakeStore{
		state: State{Themes: []Theme{{ID: "t1", Title: "Old", Status: StatusOpen, Asks: []Ask{ask("m0", "old", true)}}}},
		asks:  []Ask{ask("m1", "a", true), ask("m2", "b", true), ask("m3", "c", true)},
	}
	m := &fakeModel{replies: []string{`{"themes":[{"id":"t1","asks":[0]},{"id":"","title":"New need","summary":"s","coverage":"gap","asks":[1,2]}]}`}}
	m.onChat = func() {
		st.state.Themes = []Theme{{ID: "t1", Title: "Old", Status: StatusShipped, ShippedNote: "done", Asks: []Ask{ask("m0", "old", true)}}}
	}
	if n, err := newService(st, m, &fakeCredits{}).Refresh(context.Background(), "ws", t0); err != nil || n != 3 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if got := st.state.Themes[0]; got.Status != StatusShipped || got.ShippedNote != "done" || got.People() != 2 {
		t.Fatalf("the ship was lost: %+v", got)
	}
	if len(st.state.Themes) != 2 {
		t.Fatalf("want the new theme too, got %d themes", len(st.state.Themes))
	}
}

// Asks the model leaves out (not a need) are remembered, so they are not
// sent, and paid for, again on every run.
func TestLeftOutAsksAreNotSentAgain(t *testing.T) {
	st := &fakeStore{asks: []Ask{ask("m1", "a", true), ask("m2", "b", true), ask("m3", "joke", false), ask("m4", "news", false), ask("m5", "promo", false)}}
	m := &fakeModel{replies: []string{`{"themes":[{"id":"","title":"A need","summary":"s","coverage":"gap","asks":[0,1]}]}`}}
	svc := newService(st, m, &fakeCredits{})
	if n, err := svc.Refresh(context.Background(), "ws", t0); err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if n, _ := svc.Waiting(context.Background(), "ws"); n != 0 {
		t.Fatalf("left-out asks must not keep waiting, got %d", n)
	}
	if n, err := svc.Refresh(context.Background(), "ws", t0); err != nil || n != 0 || len(m.seen) != 1 {
		t.Fatalf("second run must not call the model: n=%d err=%v calls=%d", n, err, len(m.seen))
	}
}

// A draft is only saved when it is usable: the model may decline (the ask
// does not match what shipped), and a post cannot smuggle a link into it.
func TestUnusableDraftsAreNotSavedOrCharged(t *testing.T) {
	for _, reply := range []string{"SKIP", "Great news, see https://evil.example/x for details", "visit www.evil.example now"} {
		st := &fakeStore{state: State{Themes: []Theme{{ID: "t1", Status: StatusOpen, Asks: []Ask{ask("m1", "x", true)}}}}}
		c := &fakeCredits{}
		_, n, err := newService(st, &fakeModel{replies: []string{reply}}, c).Ship(context.Background(), "ws", "t1", "note", true, t0)
		if err != nil || n != 0 || len(st.drafts) != 0 || c.spent != c.refunded {
			t.Errorf("%q: n=%d err=%v drafts=%d spent=%d refunded=%d", reply, n, err, len(st.drafts), c.spent, c.refunded)
		}
	}
}
