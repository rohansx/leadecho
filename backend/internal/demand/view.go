package demand

import "time"

// ThemeView is a theme as the dashboard shows it.
type ThemeView struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Coverage    string    `json:"coverage"`
	Why         string    `json:"why,omitempty"`
	Status      string    `json:"status"`
	Competitors []string  `json:"competitors"`
	People      int       `json:"people"`
	Buyers      int       `json:"buyers"`
	NewThisWeek int       `json:"new_this_week"`
	ShippedAt   time.Time `json:"shipped_at,omitzero"`
	ShippedNote string    `json:"shipped_note,omitempty"`
	Drafted     int       `json:"drafted"`
	UpdatedAt   time.Time `json:"updated_at"`
	Asks        []Ask     `json:"asks"` // newest first, capped
}

const maxAsksShown = 50

// View shapes the stored themes for display: ranked, with counts, and each
// theme's asks newest first.
func (s State) View(now time.Time) []ThemeView {
	ranked := s.Ranked()
	out := make([]ThemeView, 0, len(ranked))
	weekAgo := now.Add(-7 * 24 * time.Hour)
	for _, t := range ranked {
		asks := append([]Ask(nil), t.Asks...)
		for i, j := 0, len(asks)-1; i < j; i, j = i+1, j-1 { // stored oldest first
			asks[i], asks[j] = asks[j], asks[i]
		}
		if len(asks) > maxAsksShown {
			asks = asks[:maxAsksShown]
		}
		competitors := t.Competitors
		if competitors == nil {
			competitors = []string{}
		}
		out = append(out, ThemeView{
			ID: t.ID, Title: t.Title, Summary: t.Summary, Coverage: t.Coverage, Why: t.Why, Status: t.Status, Competitors: competitors,
			People: t.People(), Buyers: t.Buyers(), NewThisWeek: t.NewSince(weekAgo),
			ShippedAt: t.ShippedAt, ShippedNote: t.ShippedNote, Drafted: t.Drafted, UpdatedAt: t.UpdatedAt, Asks: asks,
		})
	}
	return out
}
