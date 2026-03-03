import { defineBackground } from "wxt/sandbox";
import type { RawSignal, ExtensionMessage } from "../lib/messages";
import { getSettings, incrementDailyCount, getDailyCount } from "../lib/storage";
import { postSignals, markReplyPosted } from "../lib/api";

export default defineBackground(() => {
  const buffer: RawSignal[] = [];
  const FLUSH_THRESHOLD = 20;

  // Alarm-based flush every 30 seconds (chrome.alarms survives worker sleep).
  chrome.alarms.create("flush-signals", { periodInMinutes: 0.5 });

  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === "flush-signals") flush();
  });

  chrome.runtime.onMessage.addListener(
    (message: ExtensionMessage, sender, sendResponse) => {
      if (message.type === "SIGNAL") {
        buffer.push(message.payload);
        if (buffer.length >= FLUSH_THRESHOLD) flush();
        return;
      }

      if (message.type === "GET_TAB_ID") {
        sendResponse({ tabId: sender.tab?.id ?? null });
        return true;
      }

      if (message.type === "GET_STATUS") {
        (async () => {
          const { apiKey, apiUrl } = await getSettings();
          const dailyCount = await getDailyCount();
          sendResponse({
            type: "STATUS",
            payload: { configured: !!(apiKey && apiUrl), dailyCount },
          });
        })();
        return true; // keep channel open for async response
      }

      if (message.type === "POST_REPLY") {
        (async () => {
          const { replyId, targetUrl, content } = message.payload;
          const tab = await chrome.tabs.create({ url: targetUrl });
          if (tab.id != null) {
            await chrome.storage.session.set({
              [`pending_reply_${tab.id}`]: { replyId, content },
            });
          }
        })();
        return;
      }

      if (message.type === "REPLY_POSTED") {
        (async () => {
          const { replyId, success } = message.payload;
          if (success) {
            const { apiKey, apiUrl } = await getSettings();
            if (apiKey && apiUrl) {
              await markReplyPosted(apiUrl, apiKey, replyId).catch(() => {});
            }
          }
        })();
        return;
      }
    },
  );

  async function flush() {
    if (buffer.length === 0) return;
    const { apiKey, apiUrl } = await getSettings();
    if (!apiKey || !apiUrl) return;

    // Drain atomically — restore on failure.
    const batch = buffer.splice(0, buffer.length);
    try {
      const result = await postSignals(apiUrl, apiKey, batch);
      if (result.inserted > 0) await incrementDailyCount(result.inserted);
    } catch {
      buffer.unshift(...batch);
    }
  }
});
