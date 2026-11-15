package monitor

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
)

func TestWithSignalFacts(t *testing.T) {
	posted := pgtype.Timestamptz{Time: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC), Valid: true}
	// What the caller passes for the place is ignored: the stored signal decides.
	o := withSignalFacts(ai.Outreach{JobTitle: "Senior Staff Software Engineer", JobLocation: "Palo Alto"},
		[]byte(`{"signal":"hiring","job_title":"Senior Staff Software Engineer","job_location":"Rio de Janeiro, Brazil (On-site)"}`), posted)
	if o.JobLocation != "Rio de Janeiro, Brazil (On-site)" || !o.JobPosted.Equal(posted.Time) || o.Funding != "" {
		t.Fatalf("stored facts not used: %+v", o)
	}

	// Stored before the location was kept, and with no posting date: neither is stated.
	o = withSignalFacts(ai.Outreach{JobTitle: "Engineer"}, []byte(`{"signal":"hiring"}`), pgtype.Timestamptz{})
	if o.JobLocation != "" || !o.JobPosted.IsZero() {
		t.Fatalf("unknown facts must stay empty: %+v", o)
	}

	o = withSignalFacts(ai.Outreach{}, []byte(`{"funding_round":"Series B","funding_amount":"$40M","funding_date":" October 2026 "}`), posted)
	if o.Funding != "Series B, $40M, October 2026" || !o.JobPosted.IsZero() {
		t.Fatalf("funding: %+v", o)
	}
	if o = withSignalFacts(ai.Outreach{JobTitle: "Engineer"}, nil, pgtype.Timestamptz{}); o.Funding != "" || o.JobLocation != "" {
		t.Fatalf("no metadata: %+v", o)
	}
}
