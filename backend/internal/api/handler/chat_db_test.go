package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"leadecho/internal/chat"
	"leadecho/internal/database"
	"leadecho/internal/tools"
)

// DB-backed: runs only when LEADECHO_TEST_DATABASE_URL points at a migrated
// database. Drives the chat handler against the real chat queries, including
// user/workspace isolation and the confirm compare-and-swap.
func TestChatHandlerDB(t *testing.T) {
	dsn := os.Getenv("LEADECHO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("LEADECHO_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := database.New(pool)

	n := time.Now().UnixNano()
	newUser := func(i int) (string, string) {
		w, err := q.CreateWorkspace(ctx, database.CreateWorkspaceParams{ClerkOrgID: fmt.Sprint("org_chat_", n, i), Name: "chat test", Slug: fmt.Sprint("chat-", n, i)})
		if err != nil {
			t.Fatal(err)
		}
		u, err := q.CreateUser(ctx, database.CreateUserParams{ClerkUserID: fmt.Sprint("chat_", n, i), WorkspaceID: w.ID, Email: fmt.Sprintf("chat%d_%d@example.com", n, i), Name: "c", Role: database.UserRoleAdmin})
		if err != nil {
			t.Fatal(err)
		}
		return w.ID, u.ID
	}
	wsA, userA := newUser(1)
	wsB, userB := newUser(2)

	ft := &chatFakeTools{}
	srvA := chatServerAs(q, ft, setStageModel{}, wsA, userA)
	srvB := chatServerAs(q, ft, setStageModel{}, wsB, userB)

	rec := do(t, srvA, "POST", "/chat", `{"message":"qualify lead l1"}`)
	if !strings.Contains(rec.Body.String(), "event: confirm") || !strings.Contains(rec.Body.String(), "event: done") {
		t.Fatalf("stream: %s", rec.Body)
	}
	threads, err := q.ListChatThreads(ctx, database.ListChatThreadsParams{WorkspaceID: wsA, UserID: userA})
	if err != nil || len(threads) != 1 {
		t.Fatalf("threads %v %+v", err, threads)
	}
	msgs, err := q.ListChatMessages(ctx, database.ListChatMessagesParams{ThreadID: threads[0].ID, WorkspaceID: wsA, Lim: 10})
	if err != nil || len(msgs) != 2 || msgs[0].Role != "user" {
		t.Fatalf("messages %v %+v", err, msgs)
	}
	asst := msgs[1].ID

	// Another workspace cannot see or confirm it.
	if rec := do(t, srvB, "GET", "/chat/threads/"+threads[0].ID+"/messages", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace read %d", rec.Code)
	}
	if rec := do(t, srvB, "POST", "/chat/messages/"+asst+"/confirm", `{"call_id":"c1","approve":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace confirm %d", rec.Code)
	}

	// Confirm runs once; the stored call is done.
	if rec := do(t, srvA, "POST", "/chat/messages/"+asst+"/confirm", `{"call_id":"c1","approve":true}`); rec.Code != 200 {
		t.Fatalf("confirm %d: %s", rec.Code, rec.Body)
	}
	if rec := do(t, srvA, "POST", "/chat/messages/"+asst+"/confirm", `{"call_id":"c1","approve":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("second confirm %d", rec.Code)
	}
	if len(ft.calls) != 1 || ft.calls[0] != wsA+":set_stage" {
		t.Fatalf("calls %v", ft.calls)
	}
	got, err := q.GetChatMessageForUser(ctx, database.GetChatMessageForUserParams{ID: asst, WorkspaceID: wsA, UserID: userA})
	if err != nil {
		t.Fatal(err)
	}
	var calls []chat.ToolCall
	if err := json.Unmarshal(got.ToolCalls, &calls); err != nil || calls[0].Status != chat.StatusDone {
		t.Fatalf("stored calls %s %v", got.ToolCalls, err)
	}

	if rec := do(t, srvB, "DELETE", "/chat/threads/"+threads[0].ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace delete %d", rec.Code)
	}
	if rec := do(t, srvA, "DELETE", "/chat/threads/"+threads[0].ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete %d", rec.Code)
	}

	// Keyless: the stub's tool plan is accepted by the real registry.
	real := tools.New(tools.Deps{Q: q})
	for _, question := range []string{"today's best leads", "why did this keyword make money?"} {
		rec := do(t, chatServerAs(q, real, chat.StubModel{}, wsA, userA), "POST", "/chat", fmt.Sprintf(`{"message":%q}`, question))
		body := rec.Body.String()
		if !strings.Contains(body, "event: done") || strings.Contains(body, `"status":"error"`) {
			t.Fatalf("keyless %q: %s", question, body)
		}
	}
}
