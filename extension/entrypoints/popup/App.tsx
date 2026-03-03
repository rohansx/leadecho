import React, { useEffect, useState } from "react";
import { getSettings, saveSettings } from "../../lib/storage";
import { testApiKey } from "../../lib/api";
import { getStatus } from "../../lib/messages";
import "./popup.css";

type ConnectionStatus = "unknown" | "checking" | "connected" | "error";

export function App() {
  const [apiKey, setApiKey] = useState("");
  const [apiUrl, setApiUrl] = useState("");
  const [status, setStatus] = useState<ConnectionStatus>("unknown");
  const [dailyCount, setDailyCount] = useState(0);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    getSettings().then((s) => {
      setApiKey(s.apiKey);
      setApiUrl(s.apiUrl);
    });

    getStatus()
      .then((resp) => {
        setDailyCount(resp.payload.dailyCount);
        setStatus(resp.payload.configured ? "connected" : "unknown");
      })
      .catch(() => setStatus("unknown"));
  }, []);

  async function handleSave(e: React.FormEvent) {
    e.preventDefault();
    await saveSettings({ apiKey, apiUrl });
    setSaved(true);
    setStatus("checking");
    const ok = await testApiKey(apiUrl, apiKey);
    setStatus(ok ? "connected" : "error");
    setTimeout(() => setSaved(false), 2000);
  }

  const statusLabel: Record<ConnectionStatus, string> = {
    unknown: "Not configured",
    checking: "Checking…",
    connected: "Connected",
    error: "Connection failed",
  };

  const statusColor: Record<ConnectionStatus, string> = {
    unknown: "#888",
    checking: "#f5a623",
    connected: "#27c17b",
    error: "#e0444b",
  };

  return (
    <div className="popup-root">
      <header className="popup-header">
        <span className="popup-logo">LeadEcho</span>
        <span className="popup-status" style={{ color: statusColor[status] }}>
          {statusLabel[status]}
        </span>
      </header>

      <section className="popup-stat">
        <span className="popup-count">{dailyCount}</span>
        <span className="popup-label">signals captured today</span>
      </section>

      <form className="popup-form" onSubmit={handleSave}>
        <label className="popup-field-label">Backend URL</label>
        <input
          className="popup-input"
          type="url"
          placeholder="https://your-leadecho.com"
          value={apiUrl}
          onChange={(e) => setApiUrl(e.target.value)}
        />

        <label className="popup-field-label">Extension API Key</label>
        <input
          className="popup-input"
          type="password"
          placeholder="Paste key from LeadEcho Settings"
          value={apiKey}
          onChange={(e) => setApiKey(e.target.value)}
        />

        <button className="popup-btn" type="submit">
          {saved ? "Saved!" : "Save & Test"}
        </button>
      </form>
    </div>
  );
}
