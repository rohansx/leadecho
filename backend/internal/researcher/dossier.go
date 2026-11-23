package researcher

// dossier: after a lead's author is enriched (identity walk, research agent,
// paid lookup), one model call writes a profile a salesperson can act on.
// It is rebuilt only when what it is built from changes (a new post, a new
// link, a new lookup), so a prolific author costs one call per change.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"leadecho/internal/ai"
	"leadecho/internal/database"
	"leadecho/internal/enrich"
	"leadecho/internal/identity"
)

// DossierWriter makes the model call (*llm.Router satisfies it).
type DossierWriter interface {
	BuildPersonDossier(ctx context.Context, workspaceID string, in ai.DossierInput) (*ai.Dossier, error)
}

const (
	dossierTimeout  = time.Minute
	dossierPostsMax = 10
)

var (
	dossierSlots = make(chan struct{}, 2)
	dossierRuns  = &dayCounter{runs: map[string]int{}}
)

// SetDossierWriter enables dossiers; dailyCap is per workspace (0 = off).
func (s *Service) SetDossierWriter(w DossierWriter, dailyCap int) {
	if w == nil || dailyCap <= 0 {
		return
	}
	s.dossier, s.dossierCap = w, dailyCap
}

// dossierAsync rebuilds the person's dossier in the background when its
// inputs changed.
func (s *Service) dossierAsync(ws, personID, platform, handle string) {
	if s.dossier == nil || personID == "" {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error().Interface("panic", r).Str("person_id", personID).Msg("dossier: panicked")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), dossierTimeout+30*time.Second)
		defer cancel()
		select {
		case dossierSlots <- struct{}{}:
			defer func() { <-dossierSlots }()
		case <-ctx.Done():
			return
		}
		if err := s.buildDossier(ctx, ws, personID, platform, handle); err != nil {
			s.logger.Debug().Err(err).Str("person_id", personID).Msg("dossier: not built")
		}
	}()
}

func (s *Service) buildDossier(ctx context.Context, ws, personID, platform, handle string) error {
	person, err := s.q.GetPerson(ctx, database.GetPersonParams{ID: personID, WorkspaceID: ws})
	if err != nil {
		return fmt.Errorf("get person: %w", err)
	}
	posts, err := s.q.ListPersonMentions(ctx, ws, personID, dossierPostsMax)
	if err != nil {
		return fmt.Errorf("list posts: %w", err)
	}
	if len(posts) == 0 {
		return nil // not a lead's author
	}
	var meta struct {
		Identity   *identity.Profile `json:"identity"`
		Enrichment *enrich.Result    `json:"enrichment"`
		Dossier    *ai.Dossier       `json:"dossier"`
	}
	_ = json.Unmarshal(person.Metadata, &meta)
	in := dossierInput(platform, handle, meta.Identity, meta.Enrichment, posts)
	if bp, err := s.q.GetBrandProfile(ctx, ws); err == nil {
		in.Product = strings.TrimSpace(bp.Product + ". " + bp.Overview)
		in.Audience = bp.TargetAudience
	}
	hash := inputHash(in)
	if meta.Dossier != nil && meta.Dossier.InputHash == hash {
		return nil // nothing new since the last one
	}
	if !dossierRuns.take(ws, s.dossierCap) {
		return fmt.Errorf("daily dossier cap reached")
	}
	callCtx, cancel := context.WithTimeout(ctx, dossierTimeout)
	defer cancel()
	d, err := s.dossier.BuildPersonDossier(callCtx, ws, in)
	if err != nil {
		return err
	}
	d.InputHash = hash
	fresh, err := s.q.GetPerson(ctx, database.GetPersonParams{ID: personID, WorkspaceID: ws}) // metadata may have moved on
	if err != nil {
		return fmt.Errorf("get person: %w", err)
	}
	_, err = s.q.UpdatePerson(ctx, database.UpdatePersonParams{
		ID: personID, WorkspaceID: ws, Confidence: fresh.Confidence,
		Metadata: mergeMeta(fresh.Metadata, "dossier", d),
	})
	return err
}

// dossierInput turns what is stored into the model's input. Only links at
// Shown count as channels; everything is text the prompt bounds.
func dossierInput(platform, handle string, prof *identity.Profile, ent *enrich.Result, posts []database.PersonMention) ai.DossierInput {
	in := ai.DossierInput{Handle: handle, Platform: platform}
	channels := map[string]bool{}
	if prof != nil {
		var b strings.Builder
		for _, kv := range [][2]string{{"Name", prof.Name}, {"Role", prof.Role}, {"Company", prof.Company}, {"Location", prof.Location}, {"Bio", prof.Bio}, {"Summary", prof.Summary}} {
			if strings.TrimSpace(kv[1]) != "" {
				fmt.Fprintf(&b, "%s: %s\n", kv[0], kv[1])
			}
		}
		if len(prof.Topics) > 0 {
			fmt.Fprintf(&b, "Topics they post about: %s\n", strings.Join(prof.Topics, ", "))
		}
		for _, l := range prof.Links {
			if l.Method == "origin" || l.Confidence < 0.5 {
				continue
			}
			fmt.Fprintf(&b, "%s: %s [%.2f]\n", l.Kind, l.URL, l.Confidence)
			if l.Confidence >= identity.Shown {
				switch l.Kind {
				case identity.KindEmail, identity.KindLinkedIn, identity.KindX, identity.KindBluesky, identity.KindMastodon, identity.KindBooking, identity.KindGitHub:
					channels[l.Kind] = true
				}
			}
		}
		for _, n := range prof.Notes {
			fmt.Fprintf(&b, "They said: %s\n", n)
		}
		for _, p := range prof.RecentPosts {
			fmt.Fprintf(&b, "Recent post elsewhere: %s\n", p.Title)
		}
		in.Identity = b.String()
	}
	if ent != nil && ent.Found() {
		var b strings.Builder
		for _, kv := range [][2]string{{"Name", ent.FullName}, {"Title", ent.Title}, {"Company", ent.Company}, {"Company domain", ent.CompanyDomain},
			{"Industry", ent.Industry}, {"Employees", ent.Employees}, {"Location", ent.Location}} {
			if strings.TrimSpace(kv[1]) != "" {
				fmt.Fprintf(&b, "%s: %s\n", kv[0], kv[1])
			}
		}
		if ent.Email != "" && ent.EmailStatus != "invalid" {
			channels[identity.KindEmail] = true
			fmt.Fprintf(&b, "Work email found (%s)\n", firstNonEmpty(ent.EmailStatus, "unverified"))
		}
		if ent.LinkedInURL != "" {
			channels[identity.KindLinkedIn] = true
		}
		in.Enrichment = b.String()
	}
	for c := range channels {
		in.Channels = append(in.Channels, c)
	}
	sort.Strings(in.Channels)
	for _, p := range posts {
		in.Posts = append(in.Posts, ai.DossierPost{
			ID: p.MentionID, Platform: p.Platform, Date: p.PostedAt.Format("2006-01-02"),
			Title: p.Title, Excerpt: p.Excerpt, Intent: p.Intent,
		})
	}
	return in
}

func inputHash(in ai.DossierInput) string {
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}
