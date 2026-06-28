import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../lib/messages";
import { runPendingReply } from "../lib/reply";

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

    await runPendingReply({
      settleMs: 2000,
      openComposer: () => {
        const replyBtn = document.querySelector('[data-testid="reply"]') as HTMLElement | null;
        replyBtn?.click();
      },
      findReplyBox: () =>
        document.querySelector('[data-testid="tweetTextarea_0"]') as HTMLElement | null,
      findSubmit: () =>
        (document.querySelector('[data-testid="tweetButtonInline"]') as HTMLElement | null) ??
        (document.querySelector('[data-testid="tweetButton"]') as HTMLElement | null),
    });
  },
});
