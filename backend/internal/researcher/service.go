package researcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/database"
)

var githubHandleRE = regexp.MustCompile(`(?i)github\.com/([A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?)`)

// GitHubProfile is a subset of the GitHub users API response.
type GitHubProfile struct {
	Login       string `json:"login"`
	Name        string `json:"name"`
	Bio         string `json:"bio"`
	Company     string `json:"company"`
	Location    string `json:"location"`
	Blog        string `json:"blog"`
	PublicRepos int    `json:"public_repos"`
}

type githubClient struct {
	http *http.Client
}

func newGitHubClient() *githubClient {
	return &githubClient{http: &http.Client{Timeout: 10 * time.Second}}
}

func (c *githubClient) FetchUser(ctx context.Context, handle string) (*GitHubProfile, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return nil, fmt.Errorf("empty github handle")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/users/"+handle, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "LeadEcho-Researcher/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("github user not found: %s", handle)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api status %d", resp.StatusCode)
	}

	var profile GitHubProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, err
	}
	return &profile, nil
}

// Service performs Person360 enrichment for qualified leads.
type Service struct {
	q      *database.Queries
	github *githubClient
	logger zerolog.Logger
}

func NewService(q *database.Queries, logger zerolog.Logger) *Service {
	return &Service{
		q:      q,
		github: newGitHubClient(),
		logger: logger.With().Str("component", "researcher").Logger(),
	}
}

// EnrichLeadAsync runs enrichment in the background; safe to call from hot paths.
func (s *Service) EnrichLeadAsync(leadID, workspaceID string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.EnrichLead(ctx, leadID, workspaceID); err != nil {
			s.logger.Warn().Err(err).Str("lead_id", leadID).Msg("enrichment failed")
		}
	}()
}

// EnrichLead builds or updates Person360 for a lead from mention + GitHub L1 data.
func (s *Service) EnrichLead(ctx context.Context, leadID, workspaceID string) error {
	lead, err := s.q.GetLead(ctx, database.GetLeadParams{ID: leadID, WorkspaceID: workspaceID})
	if err != nil {
		return fmt.Errorf("get lead: %w", err)
	}

	var mention database.Mention
	if lead.MentionID.Valid {
		mention, err = s.q.GetMention(ctx, database.GetMentionParams{
			ID:          uuidToString(lead.MentionID),
			WorkspaceID: workspaceID,
		})
		if err != nil {
			return fmt.Errorf("get mention: %w", err)
		}
	}

	platform := ""
	handle := ""
	profileURL := ""
	if lead.Platform.Valid {
		platform = string(lead.Platform.PlatformType)
	}
	if lead.Username.Valid {
		handle = lead.Username.String
	}
	if lead.ProfileUrl.Valid {
		profileURL = lead.ProfileUrl.String
	}
	if handle == "" && mention.AuthorUsername.Valid {
		handle = mention.AuthorUsername.String
	}
	if profileURL == "" && mention.AuthorProfileUrl.Valid {
		profileURL = mention.AuthorProfileUrl.String
	}

	person, err := s.ensurePerson(ctx, workspaceID, lead, platform, handle, profileURL, mention.Content)
	if err != nil {
		return err
	}

	if err := s.q.LinkLeadToPerson(ctx, database.LinkLeadToPersonParams{
		PersonID:    parseUUID(person.ID),
		LeadID:      leadID,
		WorkspaceID: workspaceID,
	}); err != nil {
		return fmt.Errorf("link lead to person: %w", err)
	}

	githubHandle := extractGitHubHandle(profileURL, mention.Content, handle)
	if githubHandle != "" {
		if err := s.enrichFromGitHub(ctx, workspaceID, person.ID, githubHandle); err != nil {
			s.logger.Debug().Err(err).Str("handle", githubHandle).Msg("github enrichment skipped")
		}
	}
	return nil
}

func (s *Service) ensurePerson(ctx context.Context, workspaceID string, lead database.Lead, platform, handle, profileURL, content string) (database.Person, error) {
	if lead.PersonID.Valid {
		p, err := s.q.GetPerson(ctx, database.GetPersonParams{
			ID:          uuidToString(lead.PersonID),
			WorkspaceID: workspaceID,
		})
		if err == nil {
			return p, nil
		}
	}

	displayName := handle
	if displayName == "" {
		displayName = "Unknown"
	}

	person, err := s.q.CreatePerson(ctx, database.CreatePersonParams{
		WorkspaceID: workspaceID,
		DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""},
		Confidence:  0.5,
		Metadata:    []byte(`{"source":"mention_author"}`),
	})
	if err != nil {
		return database.Person{}, err
	}

	if platform != "" && handle != "" {
		_, _ = s.q.UpsertPersonIdentity(ctx, database.UpsertPersonIdentityParams{
			PersonID:    person.ID,
			WorkspaceID: workspaceID,
			Platform:    platform,
			Handle:      handle,
			ProfileUrl:  pgtype.Text{String: profileURL, Valid: profileURL != ""},
			Confidence:  0.55,
			Source:      "mention_author",
			Metadata:    []byte("{}"),
		})
	}

	_ = content // reserved for Layer 2 bio cross-links
	return person, nil
}

func (s *Service) enrichFromGitHub(ctx context.Context, workspaceID, personID, handle string) error {
	profile, err := s.github.FetchUser(ctx, handle)
	if err != nil {
		return err
	}

	meta, _ := json.Marshal(map[string]any{
		"public_repos": profile.PublicRepos,
		"blog":         profile.Blog,
	})
	_, err = s.q.UpsertPersonIdentity(ctx, database.UpsertPersonIdentityParams{
		PersonID:    personID,
		WorkspaceID: workspaceID,
		Platform:    "github",
		Handle:      profile.Login,
		ProfileUrl:  pgtype.Text{String: "https://github.com/" + profile.Login, Valid: true},
		Confidence:  0.85,
		Source:      "github_api",
		Metadata:    meta,
	})
	if err != nil {
		return err
	}

	confidence := float32(0.75)
	if profile.PublicRepos > 0 {
		confidence = 0.82
	}

	_, err = s.q.UpdatePerson(ctx, database.UpdatePersonParams{
		ID:          personID,
		WorkspaceID: workspaceID,
		DisplayName: pgtype.Text{String: firstNonEmpty(profile.Name, profile.Login), Valid: true},
		Bio:         pgtype.Text{String: profile.Bio, Valid: profile.Bio != ""},
		Company:     pgtype.Text{String: strings.TrimSpace(profile.Company), Valid: profile.Company != ""},
		Location:    pgtype.Text{String: profile.Location, Valid: profile.Location != ""},
		Confidence:  confidence,
		Metadata:    meta,
	})
	return err
}

// Person360View is the API response for mention/lead enrichment.
type Person360View struct {
	Person      *database.Person             `json:"person,omitempty"`
	Identities  []database.PersonIdentity    `json:"identities"`
	Enriched    bool                         `json:"enriched"`
}

func (s *Service) GetPerson360ByMention(ctx context.Context, workspaceID, mentionID string) (*Person360View, error) {
	lead, err := s.q.GetLeadByMention(ctx, database.GetLeadByMentionParams{
		MentionID:   parseUUID(mentionID),
		WorkspaceID: workspaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &Person360View{Identities: []database.PersonIdentity{}}, nil
		}
		return nil, err
	}
	if !lead.PersonID.Valid {
		s.EnrichLeadAsync(lead.ID, workspaceID)
		return &Person360View{Identities: []database.PersonIdentity{}, Enriched: false}, nil
	}

	person, err := s.q.GetPerson(ctx, database.GetPersonParams{
		ID:          uuidToString(lead.PersonID),
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}

	identities, err := s.q.ListPersonIdentities(ctx, database.ListPersonIdentitiesParams{
		PersonID:    person.ID,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		return nil, err
	}
	if identities == nil {
		identities = []database.PersonIdentity{}
	}

	return &Person360View{
		Person:     &person,
		Identities: identities,
		Enriched:   true,
	}, nil
}

func extractGitHubHandle(profileURL, content, author string) string {
	if m := githubHandleRE.FindStringSubmatch(profileURL); len(m) == 2 {
		return m[1]
	}
	if m := githubHandleRE.FindStringSubmatch(content); len(m) == 2 {
		return m[1]
	}
	// HN authors often match GitHub usernames — try as a low-confidence lookup candidate.
	if author != "" && !strings.Contains(author, " ") && len(author) <= 39 {
		return author
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func uuidToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func parseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if len(s) != 36 {
		return u
	}
	hexVal := func(c byte) (byte, bool) {
		switch {
		case '0' <= c && c <= '9':
			return c - '0', true
		case 'a' <= c && c <= 'f':
			return c - 'a' + 10, true
		case 'A' <= c && c <= 'F':
			return c - 'A' + 10, true
		}
		return 0, false
	}
	dst := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		hi, ok1 := hexVal(s[i])
		lo, ok2 := hexVal(s[i+1])
		if !ok1 || !ok2 {
			return pgtype.UUID{}
		}
		u.Bytes[dst] = hi<<4 | lo
		dst++
		i++
	}
	u.Valid = true
	return u
}
