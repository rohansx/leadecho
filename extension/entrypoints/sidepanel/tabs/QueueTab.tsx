import React, { useState, useEffect, useCallback } from "react";
import { getSettings } from "../../../lib/storage";
import { getReplyQueue, type QueuedReply } from "../../../lib/api";
import { sendPostReply } from "../../../lib/messages";

const PLATFORM_LABELS: Record<string, string> = {
  reddit: "Reddit",
  twitter: "Twitter",
  linkedin: "LinkedIn",
  hackernews: "HN",
  devto: "Dev.to",
  lobsters: "Lobsters",
  indiehackers: "IH",
};

interface QueueTabProps {
  onCountChange: (n: number) => void;
}

export default function QueueTab({ onCountChange }: QueueTabProps) {
  const [queue, setQueue] = useState<QueuedReply[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [posting, setPosting] = useState<Record<string, "idle" | "posting" | "done" | "fail">>({});

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

  function handlePost(reply: QueuedReply) {
    setPosting((p) => ({ ...p, [reply.id]: "posting" }));
    sendPostReply({
      replyId: reply.id,
      platform: reply.platform,
      targetUrl: reply.url,
      content: reply.edited_content ?? reply.content,
    });
    // Optimistically show done after 2s; real confirmation comes via REPLY_POSTED
    setTimeout(() => {
      setPosting((p) => ({ ...p, [reply.id]: "done" }));
    }, 2000);
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
        const previewText = (reply.edited_content ?? reply.content).slice(0, 100);
        return (
          <div key={reply.id} className="sp-card">
            <div className="sp-card-header">
              <span className="sp-badge">{PLATFORM_LABELS[reply.platform] ?? reply.platform}</span>
              <a href={reply.url} target="_blank" rel="noreferrer" className="sp-open-link">
                Open ↗
              </a>
            </div>
            {reply.title && <div className="sp-card-title">{reply.title}</div>}
            <div className="sp-card-body">{previewText}{previewText.length >= 100 ? "…" : ""}</div>
            <div className="sp-card-actions">
              <button
                className={`sp-post-btn ${state !== "idle" ? "sp-post-btn--done" : ""}`}
                onClick={() => handlePost(reply)}
                disabled={state !== "idle"}
              >
                {state === "idle" && "Post Now"}
                {state === "posting" && "Posting…"}
                {state === "done" && "✓ Posted"}
                {state === "fail" && "✗ Failed"}
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
