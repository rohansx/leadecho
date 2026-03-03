import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../../lib/messages";
import { simulateTyping } from "../../lib/human-mimicry";

export default defineContentScript({
  matches: ["https://www.reddit.com/*"],
  async main() {
    const seen = new Set<string>();

    function processPost(el: Element) {
      const postId =
        el.getAttribute("data-fullname") ||
        el.closest("[data-fullname]")?.getAttribute("data-fullname");
      if (!postId || seen.has(postId)) return;
      seen.add(postId);

      const title =
        el.querySelector("h3")?.textContent?.trim() ||
        el.querySelector('[data-adclicklocation="title"] h1')?.textContent?.trim() ||
        "";
      const bodyContent =
        el.querySelector(".RichTextJSON-root")?.textContent?.trim() ||
        el.querySelector('[data-click-id="text"] div')?.textContent?.trim() ||
        "";
      const content = bodyContent || title;
      if (!content || content.length < 20) return;

      const author =
        el.querySelector('[data-testid="post_author_link"]')?.textContent?.trim() ||
        el.querySelector('a[href^="/user/"]')?.textContent?.trim() ||
        "";
      const linkEl = el.querySelector('a[data-click-id="body"]') as HTMLAnchorElement | null;
      const url = linkEl?.href || window.location.href;

      sendSignal({
        platform: "reddit",
        platform_id: postId,
        url,
        title,
        content,
        author,
        author_url: author
          ? `https://www.reddit.com/user/${author.replace(/^u\//, "")}`
          : "",
      });
    }

    function scanPage() {
      document.querySelectorAll("[data-fullname]").forEach(processPost);
      document.querySelectorAll("article").forEach(processPost);
    }

    scanPage();

    const observer = new MutationObserver(scanPage);
    observer.observe(document.body, { childList: true, subtree: true });

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

  // Wait for page to settle then find the comment box
  await new Promise((r) => setTimeout(r, 3000));

  const replyBox = findRedditReplyBox();
  if (!replyBox) {
    chrome.runtime.sendMessage({
      type: "REPLY_POSTED",
      payload: { replyId: pending.replyId, success: false },
    });
    return;
  }

  await simulateTyping(replyBox, pending.content);

  const submitBtn = document.querySelector(
    'button[type="submit"]:not([disabled])',
  ) as HTMLButtonElement | null;
  submitBtn?.click();

  chrome.runtime.sendMessage({
    type: "REPLY_POSTED",
    payload: { replyId: pending.replyId, success: true },
  });
}

function findRedditReplyBox(): HTMLElement | null {
  return (
    (document.querySelector('[contenteditable="true"]') as HTMLElement | null) ??
    (document.querySelector(".public-DraftEditor-content") as HTMLElement | null)
  );
}

async function getTabId(): Promise<number | null> {
  try {
    const resp = await chrome.runtime.sendMessage({ type: "GET_TAB_ID" });
    return resp?.tabId ?? null;
  } catch {
    return null;
  }
}
