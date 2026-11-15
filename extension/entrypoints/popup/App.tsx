import React, { useEffect, useState } from "react";
import {
  getSettings,
  saveSettings,
  getCaptureEnabled,
  setCaptureEnabled,
} from "../../lib/storage";
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
  const [capturing, setCapturing] = useState(true);

  useEffect(() => {
    getSettings().then((s) => {
      setApiKey(s.apiKey);
      setApiUrl(s.apiUrl);
    });

    getCaptureEnabled().then(setCapturing);

    getStatus()
      .then((resp) => {
        setDailyCount(resp.payload.dailyCount);
        setStatus(resp.payload.configured ? "connected" : "unknown");
      })
      .catch(() => setStatus("unknown"));
  }, []);

  async function handleToggleCapture() {
    const next = !capturing;
    setCapturing(next);
    await setCaptureEnabled(next);
  }

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
        <span className="popup-logo"><svg className="popup-mark" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2.4} strokeLinecap="round" aria-hidden="true"><circle cx="12" cy="12" r="3.6" fill="currentColor" stroke="none" /><path d="M4.12 13.39A8 8 0 1 1 10.61 19.88" /><circle cx="4.9" cy="19.1" r="1.7" fill="currentColor" stroke="none" /></svg>LeadEcho</span>
        <span className="popup-status" style={{ color: statusColor[status] }}>
          {statusLabel[status]}
        </span>
      </header>

      <section className="popup-stat">
        <span className="popup-count">{dailyCount}</span>
        <span className="popup-label">signals captured today</span>
      </section>

      <button
        type="button"
        className={`popup-toggle ${capturing ? "popup-toggle--on" : ""}`}
        onClick={handleToggleCapture}
        role="switch"
        aria-checked={capturing}
      >
        <span className="popup-toggle-label">
          {capturing ? "Capturing while you browse" : "Capture paused"}
        </span>
        <span className="popup-toggle-track">
          <span className="popup-toggle-thumb" />
        </span>
      </button>

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
