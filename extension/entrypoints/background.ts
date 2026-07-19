import { defineBackground } from "wxt/sandbox";
import type { RawSignal, ExtensionMessage } from "../lib/messages";
import {
  getSettings,
  incrementDailyCount,
  getDailyCount,
  getCaptureEnabled,
  CAPTURE_KEY,
} from "../lib/storage";
import { postSignals, markReplyPosted } from "../lib/api";

export default defineBackground(() => {
  const buffer: RawSignal[] = [];
  const FLUSH_THRESHOLD = 20;
  // Hard cap so the buffer can't grow without bound while offline/unconfigured
  // (flush() early-returns without draining in that case). Drop oldest first.
  const MAX_BUFFER = 500;

  // Toolbar icon → side panel (Leads / Queue / Settings), not the tiny popup.
  chrome.sidePanel
    .setPanelBehavior({ openPanelOnActionClick: true })
    .catch(() => {});

  // Cache the capture toggle so the hot SIGNAL path stays synchronous.
  let captureEnabled = true;
  getCaptureEnabled().then((v) => (captureEnabled = v));
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === "local" && changes[CAPTURE_KEY]) {
      captureEnabled = changes[CAPTURE_KEY].newValue !== false;
    }
  });

  // Alarm-based flush every 30 seconds (chrome.alarms survives worker sleep).
  chrome.alarms.create("flush-signals", { periodInMinutes: 0.5 });

  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === "flush-signals") flush();
  });

  chrome.runtime.onMessage.addListener(
    (message: ExtensionMessage, sender, sendResponse) => {
      if (message.type === "SIGNAL") {
        if (!captureEnabled) return;
        buffer.push(message.payload);
        if (buffer.length > MAX_BUFFER) {
          buffer.splice(0, buffer.length - MAX_BUFFER);
        }
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
          // Normalize bare reddit.com → www so the content script match pattern fires.
          const url = normalizeTargetUrl(targetUrl);
          // Create the tab blank first, stash pending, THEN navigate. Otherwise the
          // content script can race ahead of chrome.storage.session.set and exit
          // silently — which surfaces in the side panel as "timed out".
          const tab = await chrome.tabs.create({ url: "about:blank" });
          if (tab.id == null) return;
          await chrome.storage.session.set({
            [`pending_reply_${tab.id}`]: { replyId, content },
          });
          await chrome.tabs.update(tab.id, { url });
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

/** Ensure platform URLs hit hosts our content scripts actually match. */
function normalizeTargetUrl(raw: string): string {
  try {
    const u = new URL(raw);
    if (u.hostname === "reddit.com" || u.hostname === "old.reddit.com" || u.hostname === "new.reddit.com") {
      u.hostname = "www.reddit.com";
      return u.toString();
    }
  } catch {
    /* keep original */
  }
  return raw;
}
