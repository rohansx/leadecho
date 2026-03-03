import React, { useState, useEffect } from "react";
import { getSettings, saveSettings } from "../../../lib/storage";
import { testApiKey } from "../../../lib/api";

interface SettingsTabProps {
  onSave: () => void;
}

export default function SettingsTab({ onSave }: SettingsTabProps) {
  const [apiUrl, setApiUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [status, setStatus] = useState<"idle" | "saving" | "ok" | "err">("idle");

  useEffect(() => {
    getSettings().then(({ apiUrl: u, apiKey: k }) => {
      setApiUrl(u ?? "");
      setApiKey(k ?? "");
    });
  }, []);

  async function handleSave() {
    setStatus("saving");
    await saveSettings({ apiUrl, apiKey });
    const ok = await testApiKey(apiUrl, apiKey);
    setStatus(ok ? "ok" : "err");
    if (ok) onSave();
    setTimeout(() => setStatus("idle"), 3000);
  }

  return (
    <div className="sp-settings">
      <div className="sp-field-label">API URL</div>
      <input
        className="sp-input"
        type="url"
        placeholder="https://yourserver.com"
        value={apiUrl}
        onChange={(e) => setApiUrl(e.target.value)}
      />
      <div className="sp-field-label">Extension Key</div>
      <input
        className="sp-input"
        type="password"
        placeholder="Paste your extension key"
        value={apiKey}
        onChange={(e) => setApiKey(e.target.value)}
      />
      <button className="sp-btn" onClick={handleSave} disabled={status === "saving"}>
        {status === "saving" ? "Testing…" : status === "ok" ? "✓ Connected" : status === "err" ? "✗ Failed" : "Save & Test"}
      </button>
      <div className="sp-settings-hint">
        Generate your extension key in the LeadEcho dashboard → Settings → Chrome Extension.
      </div>
    </div>
  );
}
