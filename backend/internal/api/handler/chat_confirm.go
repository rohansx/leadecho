package handler

// chat (P2 #15): confirm or cancel a write tool the assistant proposed.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/chat"
	"leadecho/internal/database"
)

type chatConfirmRequest struct {
	CallID  string `json:"call_id"`
	Approve bool   `json:"approve"`
}

var errChatConflict = errors.New("this action was already handled")

// Confirm (POST /chat/messages/{id}/confirm) runs (approve) or cancels a
// pending write tool. The tool and args come from the stored message, never
// the request, and a compare-and-swap makes it run at most once.
func (h *ChatHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	ws, uid, ok := chatPrincipal(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		writeError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	var req chatConfirmRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || req.CallID == "" {
		writeError(w, http.StatusBadRequest, "call_id is required")
		return
	}
	ctx := r.Context()
	msg, err := h.store.GetChatMessageForUser(ctx, database.GetChatMessageForUserParams{ID: id, WorkspaceID: ws, UserID: uid})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	if err != nil {
		h.logger.Error().Err(err).Msg("chat: get message")
		writeError(w, http.StatusInternalServerError, "failed to load message")
		return
	}
	calls := toChatView(msg).ToolCalls
	i := -1
	for j, c := range calls {
		if c.ID == req.CallID {
			i = j
		}
	}
	if i < 0 {
		writeError(w, http.StatusNotFound, "action not found")
		return
	}
	if calls[i].Status != chat.StatusPending {
		writeError(w, http.StatusConflict, errChatConflict.Error())
		return
	}
	next := chat.StatusCancelled
	if req.Approve {
		next = chat.StatusRunning
	}
	old := msg.ToolCalls
	if old, err = h.swapCall(ctx, ws, id, old, calls, i, func(c chat.ToolCall) chat.ToolCall { c.Status = next; return c }); err != nil {
		h.confirmFailed(w, err)
		return
	}
	if req.Approve {
		// Finish even if the client disconnects: the tool may already have run.
		pctx := context.WithoutCancel(ctx)
		done := chat.Execute(pctx, h.tools, ws, calls[i])
		if _, err = h.swapCall(pctx, ws, id, old, calls, i, func(chat.ToolCall) chat.ToolCall { return done }); err != nil {
			h.confirmFailed(w, err)
			return
		}
	}
	msg.ToolCalls, _ = json.Marshal(calls)
	writeJSON(w, http.StatusOK, map[string]any{"call": calls[i], "message": toChatView(msg)})
}

// swapCall applies fn to calls[i] (in place) and CAS-writes the list; it
// returns the newly stored bytes.
func (h *ChatHandler) swapCall(ctx context.Context, ws, id string, old []byte, calls []chat.ToolCall, i int, fn func(chat.ToolCall) chat.ToolCall) ([]byte, error) {
	calls[i] = fn(calls[i])
	b, err := json.Marshal(calls)
	if err != nil {
		return nil, err
	}
	n, err := h.store.SwapChatMessageToolCalls(ctx, database.SwapChatMessageToolCallsParams{NewCalls: b, ID: id, WorkspaceID: ws, OldCalls: old})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, errChatConflict
	}
	return b, nil
}

func (h *ChatHandler) confirmFailed(w http.ResponseWriter, err error) {
	if errors.Is(err, errChatConflict) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	h.logger.Error().Err(err).Msg("chat: confirm")
	writeError(w, http.StatusInternalServerError, "failed to update the action")
}
