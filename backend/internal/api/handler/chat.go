package handler

// chat (P2 #15): "Ask LeadEcho" — SSE agent loop over the tool registry, with
// write tools held for an explicit confirm.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/chat"
	"leadecho/internal/database"
	"leadecho/internal/llm"
)

const (
	maxChatMessageLen = 4000
	chatHistoryLimit  = 20
	chatTitleLen      = 80
	chatStreamWindow  = 5 * time.Minute
	chatStepTimeout   = 30 * time.Second
)

type chatStore interface {
	CreateChatThread(ctx context.Context, arg database.CreateChatThreadParams) (database.ChatThread, error)
	GetChatThread(ctx context.Context, arg database.GetChatThreadParams) (database.ChatThread, error)
	ListChatThreads(ctx context.Context, arg database.ListChatThreadsParams) ([]database.ChatThread, error)
	TouchChatThread(ctx context.Context, arg database.TouchChatThreadParams) error
	DeleteChatThread(ctx context.Context, arg database.DeleteChatThreadParams) (int64, error)
	CreateChatMessage(ctx context.Context, arg database.CreateChatMessageParams) (database.ChatMessage, error)
	ListChatMessages(ctx context.Context, arg database.ListChatMessagesParams) ([]database.ChatMessage, error)
	GetChatMessageForUser(ctx context.Context, arg database.GetChatMessageForUserParams) (database.ChatMessage, error)
	SwapChatMessageToolCalls(ctx context.Context, arg database.SwapChatMessageToolCallsParams) (int64, error)
}

type ChatHandler struct {
	store  chatStore
	tools  chat.ToolSet
	model  func(ctx context.Context, ws string) chat.Model
	logger zerolog.Logger
}

// NewChatHandler uses the LLM router when the workspace has a chat provider
// key and the deterministic keyless StubModel otherwise.
func NewChatHandler(q chatStore, ts chat.ToolSet, router *llm.Router, logger zerolog.Logger) *ChatHandler {
	return &ChatHandler{store: q, tools: ts, logger: logger, model: func(ctx context.Context, ws string) chat.Model {
		if router != nil && router.ChatAvailable(ctx, ws) {
			return router
		}
		return chat.StubModel{}
	}}
}

type chatMessageView struct {
	ID        string          `json:"id"`
	ThreadID  string          `json:"thread_id"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls []chat.ToolCall `json:"tool_calls"`
	CreatedAt time.Time       `json:"created_at"`
}

func toChatView(m database.ChatMessage) chatMessageView {
	calls := []chat.ToolCall{}
	_ = json.Unmarshal(m.ToolCalls, &calls) // written by us; empty on corruption
	return chatMessageView{ID: m.ID, ThreadID: m.ThreadID, Role: m.Role, Content: m.Content, ToolCalls: calls, CreatedAt: m.CreatedAt}
}

func validUUID(s string) bool {
	var u pgtype.UUID
	return u.Scan(s) == nil && u.Valid
}

// chatPrincipal returns workspace and user; false (401 written) without a user.
func chatPrincipal(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	uid := middleware.UserID(r.Context())
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "sign in required")
		return "", "", false
	}
	return middleware.WorkspaceID(r.Context()), uid, true
}

type chatSendRequest struct {
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
}

// Send (POST /chat) streams one assistant turn as SSE events:
// thread, tool_call, tool_result, confirm, answer, done | error.
func (h *ChatHandler) Send(w http.ResponseWriter, r *http.Request) {
	ws, uid, ok := chatPrincipal(w, r)
	if !ok {
		return
	}
	var req chatSendRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Message = strings.TrimSpace(req.Message)
	if req.Message == "" || utf8.RuneCountInString(req.Message) > maxChatMessageLen {
		writeError(w, http.StatusBadRequest, "message must be 1-4000 characters")
		return
	}
	ctx := r.Context()
	thread, status, err := h.thread(ctx, ws, uid, req)
	if err != nil {
		writeError(w, status, err.Error())
		return
	}
	if _, err := h.store.CreateChatMessage(ctx, database.CreateChatMessageParams{
		ThreadID: thread.ID, WorkspaceID: ws, Role: "user", Content: req.Message, ToolCalls: []byte("[]"),
	}); err != nil {
		h.logger.Error().Err(err).Msg("chat: save user message")
		writeError(w, http.StatusInternalServerError, "failed to save message")
		return
	}
	history, err := h.history(ctx, ws, thread.ID)
	if err != nil {
		h.logger.Error().Err(err).Msg("chat: load history")
		writeError(w, http.StatusInternalServerError, "failed to load thread")
		return
	}
	s, ok := startSSE(w)
	if !ok {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(chatStreamWindow))
	s.send("thread", map[string]string{"thread_id": thread.ID, "title": thread.Title})
	h.runTurn(ctx, s, ws, thread.ID, history)
}

func (h *ChatHandler) runTurn(ctx context.Context, s *sse, ws, threadID string, history []ai.ChatMessage) {
	agent := &chat.Agent{Model: h.model(ctx, ws), Tools: h.tools, MaxSteps: chat.DefaultMaxSteps, StepTimeout: chatStepTimeout, Logger: h.logger}
	turn, err := agent.Run(ctx, ws, history, s.send)
	if err != nil {
		h.logger.Warn().Err(err).Str("workspace_id", ws).Msg("chat: agent failed")
		turn.Answer = "Sorry, the assistant could not answer right now. Please try again."
		s.send("error", map[string]string{"error": turn.Answer})
	}
	calls := turn.Calls
	if calls == nil {
		calls = []chat.ToolCall{}
	}
	b, _ := json.Marshal(calls)
	// Persist even if the client went away mid-stream.
	pctx := context.WithoutCancel(ctx)
	msg, err := h.store.CreateChatMessage(pctx, database.CreateChatMessageParams{
		ThreadID: threadID, WorkspaceID: ws, Role: "assistant", Content: turn.Answer, ToolCalls: b,
	})
	if err != nil {
		h.logger.Error().Err(err).Msg("chat: save assistant message")
		s.send("error", map[string]string{"error": "failed to save the answer"})
		return
	}
	_ = h.store.TouchChatThread(pctx, database.TouchChatThreadParams{ID: threadID, WorkspaceID: ws})
	s.send("done", map[string]any{"message": toChatView(msg)})
}

// thread loads the requested thread or starts a new one titled by the message.
func (h *ChatHandler) thread(ctx context.Context, ws, uid string, req chatSendRequest) (database.ChatThread, int, error) {
	if req.ThreadID == "" {
		title := req.Message
		if utf8.RuneCountInString(title) > chatTitleLen {
			title = string([]rune(title)[:chatTitleLen]) + "…"
		}
		t, err := h.store.CreateChatThread(ctx, database.CreateChatThreadParams{WorkspaceID: ws, UserID: uid, Title: title})
		if err != nil {
			h.logger.Error().Err(err).Msg("chat: create thread")
			return t, http.StatusInternalServerError, errors.New("failed to create thread")
		}
		return t, 0, nil
	}
	if !validUUID(req.ThreadID) {
		return database.ChatThread{}, http.StatusBadRequest, errors.New("thread_id must be a UUID")
	}
	t, err := h.store.GetChatThread(ctx, database.GetChatThreadParams{ID: req.ThreadID, WorkspaceID: ws, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		return t, http.StatusNotFound, errors.New("thread not found")
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("chat: get thread")
		return t, http.StatusInternalServerError, errors.New("failed to load thread")
	}
	return t, 0, nil
}

func (h *ChatHandler) history(ctx context.Context, ws, threadID string) ([]ai.ChatMessage, error) {
	ms, err := h.store.ListChatMessages(ctx, database.ListChatMessagesParams{ThreadID: threadID, WorkspaceID: ws, Lim: chatHistoryLimit})
	if err != nil {
		return nil, err
	}
	out := make([]ai.ChatMessage, 0, len(ms))
	for _, m := range ms {
		out = append(out, ai.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out, nil
}

// Threads (GET /chat/threads) lists the user's recent threads.
func (h *ChatHandler) Threads(w http.ResponseWriter, r *http.Request) {
	ws, uid, ok := chatPrincipal(w, r)
	if !ok {
		return
	}
	ts, err := h.store.ListChatThreads(r.Context(), database.ListChatThreadsParams{WorkspaceID: ws, UserID: uid})
	if err != nil {
		h.logger.Error().Err(err).Msg("chat: list threads")
		writeError(w, http.StatusInternalServerError, "failed to list threads")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": ts})
}

// Messages (GET /chat/threads/{id}/messages) returns a thread's messages.
func (h *ChatHandler) Messages(w http.ResponseWriter, r *http.Request) {
	ws, uid, ok := chatPrincipal(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if _, status, err := h.thread(r.Context(), ws, uid, chatSendRequest{ThreadID: id}); err != nil {
		writeError(w, status, err.Error())
		return
	}
	ms, err := h.store.ListChatMessages(r.Context(), database.ListChatMessagesParams{ThreadID: id, WorkspaceID: ws, Lim: 200})
	if err != nil {
		h.logger.Error().Err(err).Msg("chat: list messages")
		writeError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}
	out := make([]chatMessageView, len(ms))
	for i, m := range ms {
		out[i] = toChatView(m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// DeleteThread (DELETE /chat/threads/{id}).
func (h *ChatHandler) DeleteThread(w http.ResponseWriter, r *http.Request) {
	ws, uid, ok := chatPrincipal(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	n, err := h.store.DeleteChatThread(r.Context(), database.DeleteChatThreadParams{ID: id, WorkspaceID: ws, UserID: uid})
	if err != nil {
		h.logger.Error().Err(err).Msg("chat: delete thread")
		writeError(w, http.StatusInternalServerError, "failed to delete thread")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "thread not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
