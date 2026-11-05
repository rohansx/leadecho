package platform

import (
	"context"
	"time"

	"leadecho/internal/automation"
	"leadecho/internal/crmsync"
	"leadecho/internal/crypto"
	"leadecho/internal/tools"
)

// AutomationCheckInterval is how often due automations are checked (P2 #17).
const AutomationCheckInterval = 5 * time.Minute

// StartAutomations runs the automation scheduler wherever the monitor runs.
// Several processes are safe: automation_runs claims each period once.
func (s *Shared) StartAutomations(ctx context.Context) {
	if !s.Config.AutomationsEnabled || !s.Config.ShouldRunMonitor() {
		s.Logger.Info().Msg("automation scheduler disabled")
		return
	}
	deps := tools.Deps{Q: s.Queries, CRM: crmsync.NewDispatcher(s.Queries, crypto.DeriveKey(s.Config.EncryptionKeyOrDefault()), s.Logger)}
	if s.Monitor != nil && s.Monitor.Mutes() != nil {
		deps.Mutes = s.Monitor.Mutes()
	}
	if s.ReplyDrafter != nil {
		deps.Drafter = s.ReplyDrafter
	}
	svc := automation.NewService(s.Queries, tools.New(deps), s.Config.ResendAPIKey, s.Config.DigestFromEmail, s.Config.FrontendURL, s.Logger)
	go svc.Run(ctx, AutomationCheckInterval)
}
