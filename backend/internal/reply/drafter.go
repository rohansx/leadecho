package reply

import (
	"context"
	"encoding/json"
	"math/rand/v2"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/ai"
	"leadecho/internal/billing"
	"leadecho/internal/browser"
	"leadecho/internal/database"
	"leadecho/internal/llm"
	"leadecho/internal/monitor"
	"leadecho/internal/subrules"
)

func (d *Drafter) markEscalation(ctx context.Context, wsID, mentionID string, existingMeta []byte, reason string) {
	meta := map[string]any{}
	if len(existingMeta) > 0 {
		_ = json.Unmarshal(existingMeta, &meta)
	}
	meta["needs_escalation"] = true
	meta["escalation_reason"] = reason
	b, _ := json.Marshal(meta)
	_, _ = d.q.PatchMentionScoringMetadata(ctx, database.PatchMentionScoringMetadataParams{
		ID:              mentionID,
		WorkspaceID:     wsID,
		ScoringMetadata: b,
	})
}

// KBRetriever supplies semantic knowledge-base context for reply drafting.
type KBRetriever interface {
	Retrieve(ctx context.Context, workspaceID, query string, topK int32) (string, error)
}

type Drafter struct {
	q         *database.Queries
	llmRouter llm.ReplyGenerator
	scrapling *browser.ScraplingClient
	kb        KBRetriever
	shortBase string            // voice: public base for /r/{code} tracking links
	credits   *billing.Credits  // billing: nil or unenforced never blocks
	rules     *subrules.Service // ban-safe: subreddit rules; nil = none
}

// SetSubredditRules makes Reddit drafts follow the subreddit's rules.
func (d *Drafter) SetSubredditRules(s *subrules.Service) { d.rules = s }

// SubredditRules is the rules service (nil when unset), shared with the API.
func (d *Drafter) SubredditRules() *subrules.Service {
	if d == nil {
		return nil
	}
	return d.rules
}

// RulesFor is the subreddit summary for a Reddit mention, else nil.
func RulesFor(ctx context.Context, s *subrules.Service, m database.Mention) *subrules.Summary {
	if s == nil || m.Platform != string(database.PlatformTypeReddit) {
		return nil
	}
	return s.Get(ctx, subrules.SubredditOf(m.Url, m.PlatformMetadata))
}

// SetCredits charges each draft (billing.CostDraft), refunded if drafting fails.
func (d *Drafter) SetCredits(c *billing.Credits) { d.credits = c }

// SetShortLinkBase sets the public base URL used for tracked short links (voice).
func (d *Drafter) SetShortLinkBase(base string) { d.shortBase = strings.TrimRight(base, "/") }

func NewDrafter(q *database.Queries, llmRouter llm.ReplyGenerator, scrapling *browser.ScraplingClient, kb KBRetriever) *Drafter {
	return &Drafter{q: q, llmRouter: llmRouter, scrapling: scrapling, kb: kb}
}

type DraftResult struct {
	Reply             database.Reply
	Tone              string
	TemplateStyle     string
	ShouldReply       bool
	Reason            string
	AwarenessLevel    string
	ThreadContextUsed bool
}

func (d *Drafter) DraftForMention(ctx context.Context, wsID, mentionID string) (*DraftResult, error) {
	return d.DraftWithProfile(ctx, wsID, mentionID, "")
}

// DraftWithProfile drafts using the given response profile ("" = workspace
// default, or a derived voice when the workspace has none).
func (d *Drafter) DraftWithProfile(ctx context.Context, wsID, mentionID, profileID string) (res *DraftResult, err error) {
	if err := d.credits.Spend(ctx, wsID, billing.CostDraft, billing.ReasonDraft); err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			d.credits.Refund(ctx, wsID, billing.CostDraft, billing.ReasonDraft)
		}
	}()
	mention, err := d.q.GetMention(ctx, database.GetMentionParams{
		ID:          mentionID,
		WorkspaceID: wsID,
	})
	if err != nil {
		return nil, err
	}
	return d.draft(ctx, mention, profileID, true)
}

// AutoDraft writes the reply for a lead nobody asked a draft for, so it is
// ready when the customer opens it. It costs the customer nothing, which is
// why it is capped: at most the workspace's daily number (draft settings,
// default 5), and never for a lead that already has a reply or is no longer
// new. It reports whether a draft was written.
func (d *Drafter) AutoDraft(ctx context.Context, wsID, mentionID string) (bool, error) {
	limit := d.q.GetDraftSettings(ctx, wsID).AutoDraftLimit()
	if limit == 0 {
		return false, nil
	}
	mention, err := d.q.GetMention(ctx, database.GetMentionParams{ID: mentionID, WorkspaceID: wsID})
	if err != nil {
		return false, err
	}
	if mention.Status != database.MentionStatusNew {
		return false, nil
	}
	replies, err := d.q.ListRepliesByMention(ctx, database.ListRepliesByMentionParams{MentionID: mentionID, WorkspaceID: wsID})
	if err != nil || len(replies) > 0 {
		return false, err
	}
	if ok, err := d.q.TakeAutoDraft(ctx, wsID, limit); err != nil || !ok {
		return false, err
	}
	// The verdict "not worth a reply" is not stored as a flag for review:
	// the customer did not ask, so nothing about the lead changes.
	res, err := d.draft(ctx, mention, "", false)
	if err != nil || !res.ShouldReply {
		d.q.ReturnAutoDraft(context.WithoutCancel(ctx), wsID) // nothing was written: the slot is free again
		return false, err
	}
	return true, nil
}

// draft writes and stores the reply. flag marks a lead the pre-filter says
// should not be replied to as needing review (the manual path).
func (d *Drafter) draft(ctx context.Context, mention database.Mention, profileID string, flag bool) (*DraftResult, error) {
	wsID, mentionID := mention.WorkspaceID, mention.ID

	title := ""
	if mention.Title.Valid {
		title = mention.Title.String
	}
	intent := "general"
	if mention.Intent.Valid {
		intent = string(mention.Intent.IntentType)
	}

	productName, brandDesc := brandOf(ctx, d.q, wsID)
	productDesc, positioning := brandDesc, ""
	if bp, err := d.q.GetBrandProfile(ctx, wsID); err == nil {
		positioning = bp.Positioning
		if bp.Overview != "" {
			productDesc = bp.Overview
		}
	}
	voice, err := d.resolveVoice(ctx, wsID, profileID, brandDesc)
	if err != nil {
		return nil, err
	}

	preFilter, err := d.llmRouter.PreFilterForReply(ctx, wsID, title, mention.Content, string(mention.Platform), intent)
	if err != nil {
		return nil, err
	}

	if preFilter.AwarenessLevel != "" {
		_ = d.q.UpdateMentionAwarenessLevel(ctx, database.UpdateMentionAwarenessLevelParams{
			AwarenessLevel: pgtype.Text{String: preFilter.AwarenessLevel, Valid: true},
			ID:             mentionID,
			WorkspaceID:    wsID,
		})
	}

	if !preFilter.ShouldReply {
		if flag {
			d.markEscalation(ctx, wsID, mentionID, mention.ScoringMetadata, preFilter.Reason)
		}
		return &DraftResult{
			ShouldReply:    false,
			Reason:         preFilter.Reason,
			AwarenessLevel: preFilter.AwarenessLevel,
		}, nil
	}

	threadCtx, _ := monitor.FetchThreadContext(ctx, d.q, d.scrapling, mention)

	kbContext := ""
	if d.kb != nil {
		query := mention.Content
		if title != "" {
			query = title + "\n\n" + query
		}
		if ctxText, err := d.kb.Retrieve(ctx, wsID, query, 5); err == nil {
			kbContext = ctxText
		}
	}

	templateStyle := selectTemplateStyle(intent, preFilter.AwarenessLevel)
	rules := RulesFor(ctx, d.rules, mention)

	result, err := d.llmRouter.DraftReplyEnhanced(ctx, wsID, ai.DraftReplyOptions{
		Title:          title,
		Content:        mention.Content,
		Platform:       string(mention.Platform),
		Intent:         intent,
		AwarenessLevel: preFilter.AwarenessLevel,
		ThreadContext:  threadCtx,
		KBContext:      kbContext,
		TemplateStyle:  templateStyle,
		ProductName:    productName,
		// The reply speaks for the product: it is told what the product is
		// and what it replaces, so it does not argue the other side.
		ProductDescription: productDesc,
		Positioning:        positioning,
		Voice:              VoiceInstructions(voice),
		StyleExample:       ExampleForPlatform(voice.Examples, string(mention.Platform)),
		CommunityRules:     subrules.PromptSection(rules),
	})
	if err != nil {
		return nil, err
	}

	reply, err := d.q.CreateReply(ctx, database.CreateReplyParams{
		MentionID:         mentionID,
		WorkspaceID:       wsID,
		Content:           result.Reply,
		Status:            database.ReplyStatusDraft,
		TemplateStyle:     pgtype.Text{String: result.TemplateStyle, Valid: true},
		ThreadContextUsed: threadCtx != "",
	})
	if err != nil {
		return nil, err
	}
	if !rules.BlocksLinks() { // a link would break the subreddit's rules
		reply = d.attachTrackingLink(ctx, reply, voice)
	}

	return &DraftResult{
		Reply:             reply,
		Tone:              result.Tone,
		TemplateStyle:     result.TemplateStyle,
		ShouldReply:       true,
		AwarenessLevel:    preFilter.AwarenessLevel,
		ThreadContextUsed: threadCtx != "",
	}, nil
}

func selectTemplateStyle(intent, awareness string) string {
	styles := []string{"value_first"}

	switch intent {
	case "recommendation_ask":
		styles = []string{"value_first", "value_first", "storytelling"}
	case "complaint":
		styles = []string{"storytelling", "storytelling", "value_first"}
	case "comparison":
		styles = []string{"technical_deep_dive", "contrarian", "value_first"}
	case "general":
		styles = []string{"casual_helpful", "casual_helpful", "value_first"}
	case "buy_signal":
		styles = []string{"value_first", "technical_deep_dive", "value_first"}
	}

	if awareness == "problem_aware" {
		styles = []string{"casual_helpful", "storytelling", "casual_helpful"}
	}

	return styles[rand.IntN(len(styles))]
}
