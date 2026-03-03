import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../../lib/messages";
import { simulateTyping } from "../../lib/human-mimicry";

export default defineContentScript({
  matches: ["https://x.com/*", "https://twitter.com/*"],
  async main() {
    const seen = new Set<string>();

    function processTweet(el: Element) {
      const linkEl = el.querySelector('a[href*="/status/"]') as HTMLAnchorElement | null;
      if (!linkEl) return;
      const match = linkEl.href.match(/\/status\/(\d+)/);
      if (!match) return;
      const tweetId = match[1];
      if (seen.has(tweetId)) return;
      seen.add(tweetId);

      const content =
        el.querySelector('[data-testid="tweetText"]')?.textContent?.trim() || "";
      if (!content || content.length < 20) return;

      const nameEl = el.querySelector('[data-testid="User-Name"]');
      const spans = nameEl?.querySelectorAll("span") ?? [];
      const author = spans[0]?.textContent?.trim() || "";
      const handle = (spans[1]?.textContent?.trim() || "").replace("@", "");

      sendSignal({
        platform: "twitter",
        platform_id: tweetId,
        url: linkEl.href,
        title: "",
        content,
        author,
        author_url: handle ? `https://x.com/${handle}` : "",
      });
    }

    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const node of mutation.addedNodes) {
          if (!(node instanceof HTMLElement)) continue;
          if (node.matches('article[data-testid="tweet"]')) processTweet(node);
          node.querySelectorAll('article[data-testid="tweet"]').forEach(processTweet);
        }
      }
    });

    observer.observe(document.body, { childList: true, subtree: true });
    document.querySelectorAll('article[data-testid="tweet"]').forEach(processTweet);

    await checkPendingReply();
  },
});

async function checkPendingReply() {
  const tabId = await getTabId();
  if (tabId == null) return;

  const key = `pending_reply_${tabId}`;
  const result = await chrome.storage.session.get(key);
  const pending = result[key] as { replyId: string; content: string } | undefined;
  if (!pending) return;

  await chrome.storage.session.remove(key);

  // Wait for Twitter's React to settle
  await new Promise((r) => setTimeout(r, 2000));

  // Click the reply button on the tweet if we're on a status page
  const replyBtn = document.querySelector('[data-testid="reply"]') as HTMLElement | null;
  replyBtn?.click();

  await new Promise((r) => setTimeout(r, 1000));

  const replyBox = document.querySelector(
    '[data-testid="tweetTextarea_0"]',
  ) as HTMLElement | null;

  if (!replyBox) {
    chrome.runtime.sendMessage({
      type: "REPLY_POSTED",
      payload: { replyId: pending.replyId, success: false },
    });
    return;
  }

  await simulateTyping(replyBox, pending.content);

  const submitBtn = document.querySelector(
    '[data-testid="tweetButtonInline"]',
  ) as HTMLButtonElement | null;
  submitBtn?.click();

  chrome.runtime.sendMessage({
    type: "REPLY_POSTED",
    payload: { replyId: pending.replyId, success: true },
  });
}

async function getTabId(): Promise<number | null> {
  try {
    const resp = await chrome.runtime.sendMessage({ type: "GET_TAB_ID" });
    return resp?.tabId ?? null;
  } catch {
    return null;
  }
}
