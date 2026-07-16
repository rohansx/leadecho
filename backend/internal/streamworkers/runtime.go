package streamworkers

import (
	"context"

	"github.com/rs/zerolog"

	"leadecho/internal/config"
	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/consumers"
	streamredis "leadecho/internal/events/redis"
	"leadecho/internal/monitor"
	"leadecho/internal/reply"
	"leadecho/internal/workflow"
)

type Runtime struct {
	cfg              *config.Config
	logger           zerolog.Logger
	queries          *database.Queries
	streamClient     *streamredis.Client
	mon              *monitor.Monitor
	replyDrafter     *reply.Drafter
	workflowEngine   *workflow.Engine
	mentionWorkers   *consumers.MentionWorkers
	replyWorkers     *consumers.ReplyWorkers
	workflowWorkers  *consumers.WorkflowWorkers
}

type Options struct {
	Config         *config.Config
	Logger         zerolog.Logger
	Queries        *database.Queries
	StreamClient   *streamredis.Client
	Monitor        *monitor.Monitor
	ReplyDrafter   *reply.Drafter
	WorkflowEngine *workflow.Engine
}

func New(opts Options) *Runtime {
	mentionWorkers := consumers.NewMentionWorkers(
		opts.Queries,
		opts.StreamClient,
		opts.Monitor,
		opts.Logger,
		opts.Config.StreamsConsumerName,
		opts.Config.StreamsBatchSize,
		opts.Config.StreamsBlockMS,
		opts.Config.StreamsClaimIdleMS,
		opts.Config.StreamsMaxAttempts,
	)
	replyWorkers := consumers.NewReplyWorkers(
		opts.Queries,
		opts.StreamClient,
		opts.ReplyDrafter,
		opts.Logger,
		opts.Config.StreamsConsumerName,
		opts.Config.StreamsBatchSize,
		opts.Config.StreamsBlockMS,
		opts.Config.StreamsClaimIdleMS,
		opts.Config.StreamsMaxAttempts,
	)
	workflowWorkers := consumers.NewWorkflowWorkers(
		opts.Queries,
		opts.StreamClient,
		opts.WorkflowEngine,
		opts.Logger,
		opts.Config.StreamsConsumerName,
		opts.Config.StreamsBatchSize,
		opts.Config.StreamsBlockMS,
		opts.Config.StreamsClaimIdleMS,
		opts.Config.StreamsMaxAttempts,
	)

	return &Runtime{
		cfg:             opts.Config,
		logger:          opts.Logger.With().Str("component", "stream_workers").Logger(),
		queries:         opts.Queries,
		streamClient:    opts.StreamClient,
		mon:             opts.Monitor,
		replyDrafter:    opts.ReplyDrafter,
		workflowEngine:  opts.WorkflowEngine,
		mentionWorkers:  mentionWorkers,
		replyWorkers:    replyWorkers,
		workflowWorkers: workflowWorkers,
	}
}

func (r *Runtime) Start(ctx context.Context) {
	if !r.cfg.StreamsEnabled {
		r.logger.Info().Msg("streams disabled; workers not started")
		return
	}

	if err := consumers.EnsureAllGroups(ctx, r.streamClient); err != nil {
		r.logger.Error().Err(err).Msg("ensure stream consumer groups")
	}

	if r.cfg.StreamsScorerConsumerEnabled {
		go r.run(ctx, "mention_scorer", func(ctx context.Context) error {
			return r.mentionWorkers.StartScorer(ctx)
		})
	}
	if r.cfg.StreamsNotifierConsumerEnabled {
		go r.run(ctx, "mention_notifier", func(ctx context.Context) error {
			return r.mentionWorkers.StartNotifier(ctx)
		})
	}
	if r.cfg.StreamsQualifierConsumerEnabled {
		go r.run(ctx, "mention_qualifier", func(ctx context.Context) error {
			return r.mentionWorkers.StartQualifier(ctx)
		})
	}
	if r.cfg.StreamsReplyDrafterConsumerEnabled {
		go r.run(ctx, "reply_drafter", func(ctx context.Context) error {
			return r.replyWorkers.StartDrafter(ctx)
		})
	}
	if r.cfg.StreamsWorkflowConsumerEnabled {
		go r.run(ctx, "workflow_executor", func(ctx context.Context) error {
			return r.workflowWorkers.StartExecutor(ctx)
		})
	}
	if r.cfg.StreamsRetryConsumerEnabled {
		retryMgr := consumers.NewRetryManager(
			r.queries,
			r.streamClient,
			r.logger,
			r.cfg.StreamsConsumerName,
			r.cfg.StreamsBatchSize,
			r.cfg.StreamsBlockMS,
			r.cfg.StreamsClaimIdleMS,
			r.cfg.StreamsMaxAttempts,
		)
		retryMgr.Register(events.StreamMentionEvents, events.GroupMentionScorers, r.mentionWorkers.Handler(events.GroupMentionScorers))
		retryMgr.Register(events.StreamMentionEvents, events.GroupMentionNotifiers, r.mentionWorkers.Handler(events.GroupMentionNotifiers))
		retryMgr.Register(events.StreamMentionEvents, events.GroupMentionQualifiers, r.mentionWorkers.Handler(events.GroupMentionQualifiers))
		retryMgr.Register(events.StreamReplyEvents, events.GroupReplyDrafters, r.replyWorkers.Handler())
		retryMgr.Register(events.StreamWorkflowEvents, events.GroupWorkflowExecutors, r.workflowWorkers.Handler())
		go r.run(ctx, "retry_manager", retryMgr.Start)
	}
}

func (r *Runtime) run(ctx context.Context, name string, fn func(context.Context) error) {
	if err := fn(ctx); err != nil && err != context.Canceled {
		r.logger.Error().Err(err).Str("worker", name).Msg("stream worker stopped")
	}
}
