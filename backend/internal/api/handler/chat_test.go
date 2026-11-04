package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"leadecho/internal/ai"
	"leadecho/internal/api/middleware"
	"leadecho/internal/apikey"
	"leadecho/internal/auth"
	"leadecho/internal/chat"
	"leadecho/internal/database"
	"leadecho/internal/tools"
)

const (
	chatWS   = "11111111-1111-1111-1111-111111111111"
	chatUser = "22222222-2222-2222-2222-222222222222"
)

// memChatStore is an in-memory chatStore with the same scoping rules as SQL.
type memChatStore struct {
	mu       sync.Mutex
	n        int
	threads  map[string]database.ChatThread
	messages []database.ChatMessage
}

func newMemChatStore() *memChatStore { return &memChatStore{threads: map[string]database.ChatThread{}} }

func (s *memChatStore) id() string {
	s.n++
	return fmt.Sprintf("00000000-0000-0000-0000-%012d", s.n)
}

func (s *memChatStore) CreateChatThread(_ context.Context, a database.CreateChatThreadParams) (database.ChatThread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := database.ChatThread{ID: s.id(), WorkspaceID: a.WorkspaceID, UserID: a.UserID, Title: a.Title, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.threads[t.ID] = t
	return t, nil
}

func (s *memChatStore) GetChatThread(_ context.Context, a database.GetChatThreadParams) (database.ChatThread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.threads[a.ID]
	if !ok || t.WorkspaceID != a.WorkspaceID || t.UserID != a.UserID {
		return database.ChatThread{}, pgx.ErrNoRows
	}
	return t, nil
}

func (s *memChatStore) ListChatThreads(_ context.Context, a database.ListChatThreadsParams) ([]database.ChatThread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []database.ChatThread{}
	for _, t := range s.threads {
		if t.WorkspaceID == a.WorkspaceID && t.UserID == a.UserID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memChatStore) TouchChatThread(context.Context, database.TouchChatThreadParams) error {
	return nil
}

func (s *memChatStore) DeleteChatThread(_ context.Context, a database.DeleteChatThreadParams) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.threads[a.ID]
	if !ok || t.WorkspaceID != a.WorkspaceID || t.UserID != a.UserID {
		return 0, nil
	}
	delete(s.threads, a.ID)
	return 1, nil
}

func (s *memChatStore) CreateChatMessage(_ context.Context, a database.CreateChatMessageParams) (database.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := database.ChatMessage{ID: s.id(), ThreadID: a.ThreadID, WorkspaceID: a.WorkspaceID, Role: a.Role, Content: a.Content, ToolCalls: a.ToolCalls, CreatedAt: time.Now()}
	s.messages = append(s.messages, m)
	return m, nil
}

func (s *memChatStore) ListChatMessages(_ context.Context, a database.ListChatMessagesParams) ([]database.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []database.ChatMessage{}
	for _, m := range s.messages {
		if m.ThreadID == a.ThreadID && m.WorkspaceID == a.WorkspaceID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (s *memChatStore) GetChatMessageForUser(_ context.Context, a database.GetChatMessageForUserParams) (database.ChatMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.messages {
		if m.ID == a.ID && m.WorkspaceID == a.WorkspaceID && s.threads[m.ThreadID].UserID == a.UserID {
			return m, nil
		}
	}
	return database.ChatMessage{}, pgx.ErrNoRows
}

func (s *memChatStore) SwapChatMessageToolCalls(_ context.Context, a database.SwapChatMessageToolCallsParams) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, m := range s.messages {
		if m.ID == a.ID && m.WorkspaceID == a.WorkspaceID && bytes.Equal(m.ToolCalls, a.OldCalls) {
			s.messages[i].ToolCalls = a.NewCalls
			return 1, nil
		}
	}
	return 0, nil
}

type chatFakeTools struct {
	mu    sync.Mutex
	calls []string
}

func (f *chatFakeTools) List() []tools.Tool {
	return []tools.Tool{
		{Name: "search_mentions", Scope: apikey.ScopeRead, InputSchema: json.RawMessage(`{}`)},
		{Name: "list_leads", Scope: apikey.ScopeRead, InputSchema: json.RawMessage(`{}`)},
		{Name: "set_stage", Scope: apikey.ScopeWrite, InputSchema: json.RawMessage(`{}`)},
	}
}

func (f *chatFakeTools) Call(_ context.Context, ws string, _ []string, name string, _ json.RawMessage) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, ws+":"+name)
	return map[string]any{"mentions": []tools.MentionView{{ID: "m1", Platform: "reddit", Content: "need a tool"}}}, nil
}

type setStageModel struct{}

func (setStageModel) Chat(context.Context, string, []ai.ChatMessage) (string, error) {
	return `{"tool":"set_stage","args":{"lead_id":"l1","stage":"qualified"}}`, nil
}

func chatServer(store chatStore, ft *chatFakeTools, model chat.Model) http.Handler {
	return chatServerAs(store, ft, model, chatWS, chatUser)
}

func chatServerAs(store chatStore, ft chat.ToolSet, model chat.Model, ws, user string) http.Handler {
	h := &ChatHandler{store: store, tools: ft, model: func(context.Context, string) chat.Model { return model }}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(middleware.WithClaims(r.Context(), &auth.Claims{WorkspaceID: ws, UserID: user})))
		})
	})
	r.Post("/chat", h.Send)
	r.Get("/chat/threads", h.Threads)
	r.Get("/chat/threads/{id}/messages", h.Messages)
	r.Delete("/chat/threads/{id}", h.DeleteThread)
	r.Post("/chat/messages/{id}/confirm", h.Confirm)
	return r
}

func do(t *testing.T, srv http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func TestChatSendValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `{`, http.StatusBadRequest},
		{"empty message", `{"message":"   "}`, http.StatusBadRequest},
		{"message too long", `{"message":"` + strings.Repeat("a", 4001) + `"}`, http.StatusBadRequest},
		{"bad thread id", `{"message":"hi","thread_id":"nope"}`, http.StatusBadRequest},
		{"unknown thread", `{"message":"hi","thread_id":"99999999-9999-9999-9999-999999999999"}`, http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, chatServer(newMemChatStore(), &chatFakeTools{}, chat.StubModel{}), "POST", "/chat", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}

func TestChatSendStreamsAndPersists(t *testing.T) {
	store, ft := newMemChatStore(), &chatFakeTools{}
	srv := chatServer(store, ft, chat.StubModel{})
	rec := do(t, srv, "POST", "/chat", `{"message":"today's best leads"}`)
	body := rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status %d type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	for _, ev := range []string{"event: thread", "event: tool_call", "event: tool_result", "event: answer", "event: done"} {
		if !strings.Contains(body, ev) {
			t.Errorf("stream lacks %q:\n%s", ev, body)
		}
	}
	if len(ft.calls) != 2 || ft.calls[0] != chatWS+":search_mentions" {
		t.Fatalf("tool calls %v", ft.calls)
	}
	if len(store.messages) != 2 || store.messages[0].Role != "user" || store.messages[1].Role != "assistant" {
		t.Fatalf("messages %+v", store.messages)
	}
	var calls []chat.ToolCall
	if err := json.Unmarshal(store.messages[1].ToolCalls, &calls); err != nil || len(calls) != 2 || calls[0].Cards == nil {
		t.Fatalf("tool calls stored: %s (%v)", store.messages[1].ToolCalls, err)
	}

	// The thread lists and its messages load for the same user.
	threadID := store.messages[0].ThreadID
	if rec := do(t, srv, "GET", "/chat/threads", ""); !strings.Contains(rec.Body.String(), threadID) {
		t.Fatalf("threads: %s", rec.Body)
	}
	rec = do(t, srv, "GET", "/chat/threads/"+threadID+"/messages", "")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"cards"`) {
		t.Fatalf("messages %d: %s", rec.Code, rec.Body)
	}
	// Follow-up in the same thread.
	rec = do(t, srv, "POST", "/chat", `{"message":"and revenue?","thread_id":"`+threadID+`"}`)
	if rec.Code != 200 || len(store.messages) != 4 {
		t.Fatalf("follow-up %d, %d messages", rec.Code, len(store.messages))
	}
}

func TestChatConfirmRunsWriteToolOnce(t *testing.T) {
	tests := []struct {
		name       string
		approve    bool
		wantStatus string
		wantCalls  int
	}{
		{"approve executes", true, chat.StatusDone, 1},
		{"cancel does not execute", false, chat.StatusCancelled, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, ft := newMemChatStore(), &chatFakeTools{}
			srv := chatServer(store, ft, setStageModel{})
			if rec := do(t, srv, "POST", "/chat", `{"message":"qualify lead l1"}`); !strings.Contains(rec.Body.String(), "event: confirm") {
				t.Fatalf("no confirm event: %s", rec.Body)
			}
			if len(ft.calls) != 0 {
				t.Fatal("write tool ran before confirmation")
			}
			msgID := store.messages[1].ID
			body := fmt.Sprintf(`{"call_id":"c1","approve":%v}`, tc.approve)
			rec := do(t, srv, "POST", "/chat/messages/"+msgID+"/confirm", body)
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"status":"`+tc.wantStatus+`"`) {
				t.Fatalf("confirm %d: %s", rec.Code, rec.Body)
			}
			if rec := do(t, srv, "POST", "/chat/messages/"+msgID+"/confirm", body); rec.Code != http.StatusConflict {
				t.Fatalf("second confirm %d, want 409", rec.Code)
			}
			if len(ft.calls) != tc.wantCalls {
				t.Fatalf("calls %v", ft.calls)
			}
		})
	}
}

func TestChatConfirmValidation(t *testing.T) {
	store := newMemChatStore()
	srv := chatServer(store, &chatFakeTools{}, setStageModel{})
	do(t, srv, "POST", "/chat", `{"message":"qualify"}`)
	msgID := store.messages[1].ID
	tests := []struct {
		name, path, body string
		want             int
	}{
		{"bad id", "/chat/messages/x/confirm", `{"call_id":"c1","approve":true}`, http.StatusBadRequest},
		{"unknown message", "/chat/messages/99999999-9999-9999-9999-999999999999/confirm", `{"call_id":"c1","approve":true}`, http.StatusNotFound},
		{"unknown call", "/chat/messages/" + msgID + "/confirm", `{"call_id":"c9","approve":true}`, http.StatusNotFound},
		{"missing call id", "/chat/messages/" + msgID + "/confirm", `{"approve":true}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := do(t, srv, "POST", tc.path, tc.body); rec.Code != tc.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.want, rec.Body)
			}
		})
	}
}
