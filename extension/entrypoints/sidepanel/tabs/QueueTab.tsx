import React, { useState, useEffect, useCallback } from "react";
import { getSettings } from "../../../lib/storage";
import { getReplyQueue, type QueuedReply } from "../../../lib/api";
import { sendPostReply } from "../../../lib/messages";
import { canAutoPost, platformLabel } from "../../../lib/platforms";

type PostState = "idle" | "posting" | "done" | "fail";

// Posting opens a tab, simulates reading (2–8s) then types char-by-char, so the
// round-trip can take a while. Fail the optimistic state only after this.
const POST_TIMEOUT_MS = 120_000;

interface QueueTabProps {
  onCountChange: (n: number) => void;
}

export default function QueueTab({ onCountChange }: QueueTabProps) {
  const [queue, setQueue] = useState<QueuedReply[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [posting, setPosting] = useState<Record<string, PostState>>({});
  const [failReason, setFailReason] = useState<Record<string, string>>({});

  const load = useCallback(async () => {
    const { apiKey, apiUrl } = await getSettings();
    if (!apiKey || !apiUrl) {
      setLoading(false);
      setError("Not configured — add your API key in Settings.");
      return;
    }
    try {
      const data = await getReplyQueue(apiUrl, apiKey);
      setQueue(data);
      onCountChange(data.length);
      setError("");
    } catch {
      setError("Failed to load reply queue.");
    } finally {
      setLoading(false);
    }
  }, [onCountChange]);

  useEffect(() => {
    load();
    const id = setInterval(load, 30_000);
    return () => clearInterval(id);
  }, [load]);

  // Listen for the REAL outcome reported by the content script after it attempts
  // to post. The content script broadcasts REPLY_POSTED to all extension pages.
  useEffect(() => {
    const listener = (msg: { type?: string; payload?: { replyId: string; success: boolean; reason?: string } }) => {
      if (msg?.type !== "REPLY_POSTED" || !msg.payload) return;
      const { replyId, success, reason } = msg.payload;
      setPosting((p) => ({ ...p, [replyId]: success ? "done" : "fail" }));
      if (!success && reason) {
        setFailReason((r) => ({ ...r, [replyId]: reason }));
      }
      // Drop successfully-posted replies on the next refresh (backend marks them posted).
      if (success) setTimeout(load, 2000);
    };
    chrome.runtime.onMessage.addListener(listener);
    return () => chrome.runtime.onMessage.removeListener(listener);
  }, [load]);

  function handlePost(reply: QueuedReply) {
    setPosting((p) => ({ ...p, [reply.id]: "posting" }));
    setFailReason((r) => {
      const next = { ...r };
      delete next[reply.id];
      return next;
    });
    sendPostReply({
      replyId: reply.id,
      platform: reply.platform,
      targetUrl: reply.url,
      content: reply.edited_content ?? reply.content,
    });
    // Safety net: if no REPLY_POSTED arrives (tab closed, no content script, …),
    // surface a failure instead of leaving the button stuck on "Posting…".
    window.setTimeout(() => {
      setPosting((p) => (p[reply.id] === "posting" ? { ...p, [reply.id]: "fail" } : p));
      setFailReason((r) => (r[reply.id] ? r : { ...r, [reply.id]: "timed out" }));
    }, POST_TIMEOUT_MS);
  }

  if (loading) return <div className="sp-empty">Loading…</div>;
  if (error) return <div className="sp-empty sp-error">{error}</div>;
  if (queue.length === 0)
    return (
      <div className="sp-empty">
        No approved replies. Approve replies in the LeadEcho dashboard.
      </div>
    );

  return (
    <div className="sp-list">
      {queue.map((reply) => {
        const state = posting[reply.id] ?? "idle";
        const autoPost = canAutoPost(reply.platform);
        const previewText = (reply.edited_content ?? reply.content).slice(0, 100);
        return (
          <div key={reply.id} className="sp-card">
            <div className="sp-card-header">
              <span className="sp-badge">{platformLabel(reply.platform)}</span>
              <a href={reply.url} target="_blank" rel="noreferrer" className="sp-open-link">
                Open ↗
              </a>
            </div>
            {reply.title && <div className="sp-card-title">{reply.title}</div>}
            <div className="sp-card-body">{previewText}{previewText.length >= 100 ? "…" : ""}</div>
            <div className="sp-card-actions">
              {autoPost ? (
                <button
                  className={`sp-post-btn ${state === "done" ? "sp-post-btn--done" : ""}`}
                  onClick={() => handlePost(reply)}
                  disabled={state === "posting" || state === "done"}
                  title={state === "fail" ? failReason[reply.id] : undefined}
                >
                  {state === "idle" && "Post Now"}
                  {state === "posting" && "Posting…"}
                  {state === "done" && "✓ Posted"}
                  {state === "fail" && "✗ Retry"}
                </button>
              ) : (
                <a
                  href={reply.url}
                  target="_blank"
                  rel="noreferrer"
                  className="sp-post-btn sp-post-btn--manual"
                  title={`Auto-posting isn't supported for ${platformLabel(reply.platform)} — open and paste manually.`}
                >
                  Post manually ↗
                </a>
              )}
              {state === "fail" && failReason[reply.id] && (
                <span className="sp-fail-reason">{failReason[reply.id]}</span>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
