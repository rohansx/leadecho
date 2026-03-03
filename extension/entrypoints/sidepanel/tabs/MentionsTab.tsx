import React, { useState, useEffect, useCallback } from "react";
import { getSettings } from "../../../lib/storage";
import { getMentions, type ExtensionMention } from "../../../lib/api";

const PLATFORM_LABELS: Record<string, string> = {
  reddit: "Reddit",
  twitter: "Twitter",
  linkedin: "LinkedIn",
  hackernews: "HN",
  devto: "Dev.to",
  lobsters: "Lobsters",
  indiehackers: "IH",
};

export default function MentionsTab() {
  const [mentions, setMentions] = useState<ExtensionMention[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    const { apiKey, apiUrl } = await getSettings();
    if (!apiKey || !apiUrl) {
      setLoading(false);
      setError("Not configured — add your API key in Settings.");
      return;
    }
    try {
      const data = await getMentions(apiUrl, apiKey);
      setMentions(data);
      setError("");
    } catch {
      setError("Failed to load leads.");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
    const id = setInterval(load, 60_000);
    return () => clearInterval(id);
  }, [load]);

  if (loading) return <div className="sp-empty">Loading…</div>;
  if (error) return <div className="sp-empty sp-error">{error}</div>;
  if (mentions.length === 0)
    return (
      <div className="sp-empty">
        No leads yet — browsing collects signals automatically.
      </div>
    );

  return (
    <div className="sp-list">
      {mentions.map((m) => (
        <div key={m.id} className="sp-card">
          <div className="sp-card-header">
            <span className="sp-badge">{PLATFORM_LABELS[m.platform] ?? m.platform}</span>
            {m.intent && <span className="sp-intent">{m.intent.replace(/_/g, " ")}</span>}
            <a href={m.url} target="_blank" rel="noreferrer" className="sp-open-link">
              Open ↗
            </a>
          </div>
          {m.title && <div className="sp-card-title">{m.title}</div>}
          <div className="sp-card-body">{m.content.slice(0, 120)}{m.content.length > 120 ? "…" : ""}</div>
          {m.author_username && (
            <div className="sp-card-author">by {m.author_username}</div>
          )}
        </div>
      ))}
    </div>
  );
}
