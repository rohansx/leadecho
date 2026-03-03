import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../../lib/messages";
import { simulateTyping } from "../../lib/human-mimicry";

export default defineContentScript({
  matches: ["https://www.linkedin.com/feed*", "https://www.linkedin.com/in/*"],
  async main() {
    const seen = new Set<string>();

    function processPost(el: Element) {
      const urn =
        el.getAttribute("data-urn") ||
        el.closest("[data-urn]")?.getAttribute("data-urn");
      if (!urn || seen.has(urn)) return;
      seen.add(urn);

      const content =
        el.querySelector(".feed-shared-update-v2__description-wrapper")?.textContent?.trim() ||
        el.querySelector(".feed-shared-text")?.textContent?.trim() ||
        el.querySelector(".update-components-text")?.textContent?.trim() ||
        "";
      if (!content || content.length < 30) return;

      const author =
        el.querySelector(".update-components-actor__name")?.textContent?.trim() ||
        el.querySelector(".feed-shared-actor__name")?.textContent?.trim() ||
        "";
      const authorHrefEl = el.querySelector(".update-components-actor__meta-link") as HTMLAnchorElement | null;
      const authorHref = authorHrefEl?.href || "";
      const title =
        el.querySelector(".feed-shared-article__title")?.textContent?.trim() || "";

      sendSignal({
        platform: "linkedin",
        platform_id: urn,
        url: `https://www.linkedin.com/feed/update/${urn}`,
        title,
        content,
        author,
        author_url: authorHref,
      });
    }

    // Scan already-rendered posts on load.
    document.querySelectorAll("[data-urn]").forEach(processPost);

    // Watch for new posts as the user scrolls.
    const observer = new MutationObserver((mutations) => {
      for (const mutation of mutations) {
        for (const node of mutation.addedNodes) {
          if (!(node instanceof HTMLElement)) continue;
          if (node.hasAttribute("data-urn")) processPost(node);
          node.querySelectorAll("[data-urn]").forEach(processPost);
        }
      }
    });

    const feed = document.querySelector(".scaffold-finite-scroll__content") ?? document.body;
    observer.observe(feed, { childList: true, subtree: true });

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

  // Wait for LinkedIn's JS to render comment boxes
  await new Promise((r) => setTimeout(r, 3000));

  // Click the "Add a comment..." prompt to open the comment editor
  const commentPrompt = document.querySelector(
    ".comments-comment-box__form-container [placeholder]",
  ) as HTMLElement | null;
  commentPrompt?.click();

  await new Promise((r) => setTimeout(r, 800));

  const replyBox = document.querySelector(
    ".comments-comment-box__form-container [contenteditable]",
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
    ".comments-comment-box__submit-button",
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
