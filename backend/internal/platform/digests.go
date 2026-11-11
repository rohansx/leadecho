package platform

import (
	"context"
	"time"

	"leadecho/internal/digest"
)

// DigestCheckInterval is how often due digests are checked (P1 #13).
const DigestCheckInterval = 5 * time.Minute

// StartDigests runs the digest scheduler in whichever process runs the
// monitor. Running it in several processes is safe: digest_runs claims each
// (workspace, kind, period) once.
func (s *Shared) StartDigests(ctx context.Context) {
	if !s.Config.DigestsEnabled || !s.Config.ShouldRunMonitor() {
		s.Logger.Info().Msg("digest scheduler disabled")
		return
	}
	svc := digest.NewService(s.Queries, s.LLMRouter,
		s.Config.ResendAPIKey, s.Config.DigestFromEmail,
		s.Config.FrontendURL, s.Logger)
	if s.ReplyDrafter != nil {
		svc.SetDrafter(func(ctx context.Context, wsID, mentionID string) (string, error) {
			res, err := s.ReplyDrafter.DraftForMention(ctx, wsID, mentionID)
			if err != nil || res == nil || !res.ShouldReply {
				return "", err
			}
			return res.Reply.Content, nil
		})
	}
	go svc.Run(ctx, DigestCheckInterval)
}
