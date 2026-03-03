import React, { useState, useEffect } from "react";
import { getSettings } from "../../lib/storage";
import { getStatus } from "../../lib/messages";
import MentionsTab from "./tabs/MentionsTab";
import QueueTab from "./tabs/QueueTab";
import SettingsTab from "./tabs/SettingsTab";

type Tab = "leads" | "queue" | "settings";

export default function App() {
  const [activeTab, setActiveTab] = useState<Tab>("leads");
  const [connected, setConnected] = useState<boolean | null>(null);
  const [queueCount, setQueueCount] = useState(0);

  useEffect(() => {
    getStatus()
      .then((resp) => setConnected(resp?.payload?.configured ?? false))
      .catch(() => setConnected(false));
  }, []);

  const dotColor =
    connected === null ? "#888" : connected ? "#27c17b" : "#ef4444";

  return (
    <div className="sp-root">
      <div className="sp-header">
        <span className="sp-logo">LeadEcho</span>
        <span className="sp-dot" style={{ background: dotColor }} title={
          connected === null ? "Checking…" : connected ? "Connected" : "Not connected"
        } />
      </div>

      <div className="sp-tabs">
        <button
          className={`sp-tab ${activeTab === "leads" ? "active" : ""}`}
          onClick={() => setActiveTab("leads")}
        >
          Leads
        </button>
        <button
          className={`sp-tab ${activeTab === "queue" ? "active" : ""}`}
          onClick={() => setActiveTab("queue")}
        >
          Queue {queueCount > 0 ? `(${queueCount})` : ""}
        </button>
        <button
          className={`sp-tab ${activeTab === "settings" ? "active" : ""}`}
          onClick={() => setActiveTab("settings")}
        >
          Settings
        </button>
      </div>

      <div className="sp-content">
        {activeTab === "leads" && <MentionsTab />}
        {activeTab === "queue" && <QueueTab onCountChange={setQueueCount} />}
        {activeTab === "settings" && <SettingsTab onSave={() => setConnected(null)} />}
      </div>
    </div>
  );
}
