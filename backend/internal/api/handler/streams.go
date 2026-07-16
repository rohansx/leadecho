package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"leadecho/internal/api/middleware"
	"leadecho/internal/database"
	"leadecho/internal/events"
	"leadecho/internal/events/publishers"
)

type StreamsHandler struct {
	q         *database.Queries
	publisher *publishers.Publisher
}

func NewStreamsHandler(q *database.Queries, publisher *publishers.Publisher) *StreamsHandler {
	return &StreamsHandler{q: q, publisher: publisher}
}

func (h *StreamsHandler) Status(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListConsumerCheckpoints(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load stream status")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *StreamsHandler) DeadLetters(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListOpenDeadLetterEvents(r.Context(), database.ListOpenDeadLetterEventsParams{
		Lim: 100,
		Off: 0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load dead letters")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *StreamsHandler) Replays(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListEventReplays(r.Context(), database.ListEventReplaysParams{
		Lim: 100,
		Off: 0,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load replays")
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (h *StreamsHandler) TriggerReplay(w http.ResponseWriter, r *http.Request) {
	var body struct {
		StreamName     string `json:"stream_name"`
		EventType      string `json:"event_type"`
		WorkspaceID    string `json:"workspace_id"`
		AggregateType  string `json:"aggregate_type"`
		AggregateID    string `json:"aggregate_id"`
		FromOccurredAt string `json:"from_occurred_at"`
		ToOccurredAt   string `json:"to_occurred_at"`
		Mode           string `json:"mode"`
		Limit          int32  `json:"limit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.StreamName == "" {
		writeError(w, http.StatusBadRequest, "stream_name is required")
		return
	}
	if body.Mode == "" {
		body.Mode = "replay_from_event_log"
	}
	if body.Limit <= 0 || body.Limit > 500 {
		body.Limit = 100
	}

	var from, to pgtype.Timestamptz
	var fromTime, toTime time.Time
	if body.FromOccurredAt != "" {
		t, err := time.Parse(time.RFC3339, body.FromOccurredAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from_occurred_at")
			return
		}
		from = pgtype.Timestamptz{Time: t, Valid: true}
		fromTime = t
	}
	if body.ToOccurredAt != "" {
		t, err := time.Parse(time.RFC3339, body.ToOccurredAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to_occurred_at")
			return
		}
		to = pgtype.Timestamptz{Time: t, Valid: true}
		toTime = t
	}

	var requestedBy pgtype.UUID
	if claims := middleware.ClaimsFromContext(r.Context()); claims != nil && claims.UserID != "" {
		_ = requestedBy.Scan(claims.UserID)
	}
	replay, err := h.q.CreateEventReplay(r.Context(), database.CreateEventReplayParams{
		RequestedBy:    requestedBy,
		StreamName:     body.StreamName,
		ConsumerGroup:  pgtype.Text{},
		EventType:      pgtype.Text{String: body.EventType, Valid: body.EventType != ""},
		WorkspaceID:    uuidOrNullText(body.WorkspaceID),
		AggregateType:  pgtype.Text{String: body.AggregateType, Valid: body.AggregateType != ""},
		AggregateID:    pgtype.Text{String: body.AggregateID, Valid: body.AggregateID != ""},
		FromOccurredAt: from,
		ToOccurredAt:   to,
		ReplayMode:     body.Mode,
		Status:         "running",
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create replay record")
		return
	}

	eventsToReplay, err := h.q.ListReplayableEvents(r.Context(), database.ListReplayableEventsParams{
		StreamName:     body.StreamName,
		EventType:      emptyToNil(body.EventType),
		WorkspaceID:    body.WorkspaceID,
		AggregateType:  emptyToNil(body.AggregateType),
		AggregateID:    emptyToNil(body.AggregateID),
		FromOccurredAt: fromTime,
		ToOccurredAt:   toTime,
		Lim:            body.Limit,
	})
	if err != nil {
		_, _ = h.q.UpdateEventReplayStatus(r.Context(), database.UpdateEventReplayStatusParams{
			ID:            replay.ID,
			Status:        "failed",
			ReplayedCount: 0,
			LastError:     pgtype.Text{String: err.Error(), Valid: true},
		})
		writeError(w, http.StatusInternalServerError, "failed to list replayable events")
		return
	}

	count := 0
	for _, row := range eventsToReplay {
		env := events.Envelope{
			EventID:        row.EventID,
			EventType:      row.EventType,
			SchemaVersion:  row.SchemaVersion,
			WorkspaceID:    nullableUUIDToString(row.WorkspaceID),
			AggregateType:  row.AggregateType,
			AggregateID:    row.AggregateID,
			Producer:       "replay",
			Payload:        row.Payload,
			IdempotencyKey: row.IdempotencyKey + ":replay:" + replay.ID,
			CorrelationID:  nullableText(row.CorrelationID),
			CausationID:    row.EventID,
			TraceID:        nullableText(row.TraceID),
			OccurredAt:     time.Now().UTC(),
		}
		if _, err := h.publisher.Publish(r.Context(), env); err != nil {
			_, _ = h.q.UpdateEventReplayStatus(r.Context(), database.UpdateEventReplayStatusParams{
				ID:            replay.ID,
				Status:        "failed",
				ReplayedCount: int32(count),
				LastError:     pgtype.Text{String: err.Error(), Valid: true},
			})
			writeError(w, http.StatusInternalServerError, "failed to publish replay events")
			return
		}
		count++
	}

	updated, _ := h.q.UpdateEventReplayStatus(r.Context(), database.UpdateEventReplayStatusParams{
		ID:            replay.ID,
		Status:        "completed",
		ReplayedCount: int32(count),
		LastError:     pgtype.Text{},
	})
	writeJSON(w, http.StatusOK, updated)
}

func nullableUUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	const hex = "0123456789abcdef"
	var buf [36]byte
	pos := 0
	for i, v := range u.Bytes {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			buf[pos] = '-'
			pos++
		}
		buf[pos] = hex[v>>4]
		buf[pos+1] = hex[v&0x0f]
		pos += 2
	}
	return string(buf[:])
}

func uuidOrNullText(v string) pgtype.UUID {
	var u pgtype.UUID
	if v == "" {
		return u
	}
	_ = u.Scan(v)
	return u
}

func nullableText(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

func emptyToNil(s string) any {
	if s == "" {
		return ""
	}
	return s
}
