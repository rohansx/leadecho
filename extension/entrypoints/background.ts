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

  // Inject the vanilla Reddit script on Reddit tabs. The WXT content script
  // crashes because the webextension-polyfill tries to access chrome.storage
  // during init, which Reddit blocks. This vanilla script has no polyfill and
  // no storage access — it just captures posts and sends signals.
  chrome.tabs.onUpdated.addListener((tabId, changeInfo, tab) => {
    if (changeInfo.status !== "complete") return;
    if (!tab.url) return;
    try {
      const u = new URL(tab.url);
      const isReddit = u.hostname === "www.reddit.com" || u.hostname === "reddit.com";
      if (!isReddit) return;
      chrome.scripting.executeScript({
        target: { tabId },
        files: ["vanilla/reddit.js"],
      }).catch(() => {});
    } catch {
      // Invalid URL — ignore.
    }
  });

  // Cache the capture toggle so the hot SIGNAL path stays synchronous.
  let captureEnabled = true;
  getCaptureEnabled().then((v) => (captureEnabled = v)).catch(() => {
    // Storage access may fail in some contexts — default to enabled.
    captureEnabled = true;
  });
  chrome.storage.onChanged.addListener((changes, area) => {
    if (area === "local" && changes[CAPTURE_KEY]) {
      captureEnabled = changes[CAPTURE_KEY].newValue !== false;
    }
  });

  // Alarm-based flush every 15 seconds (chrome.alarms survives worker sleep).
  // Reduced from 30s to 15s so signals reach the backend faster.
  chrome.alarms.create("flush-signals", { periodInMinutes: 0.25 });

  chrome.alarms.onAlarm.addListener((alarm) => {
    if (alarm.name === "flush-signals") flush().catch(() => {});
  });

  // Also flush when a SIGNAL message arrives and the buffer has enough.
  chrome.runtime.onMessage.addListener(
    (message: ExtensionMessage, sender, sendResponse) => {
      if (message.type === "SIGNAL") {
        if (!captureEnabled) return;
        buffer.push(message.payload);
        if (buffer.length > MAX_BUFFER) {
          buffer.splice(0, buffer.length - MAX_BUFFER);
        }
        // Flush immediately if we have any signals (don't wait for threshold
        // — Reddit posts may come in slowly one at a time).
        if (buffer.length >= 1) flush().catch(() => {});
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
