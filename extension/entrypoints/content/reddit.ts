import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../../lib/messages";
import { runPendingReply } from "../../lib/reply";

export default defineContentScript({
  matches: ["https://www.reddit.com/*"],
  async main() {
    const seen = new Set<string>();

    // ── Current Reddit (shreddit web components) ──────────────────────────────
    function processShredditPost(el: Element) {
      const postId = el.getAttribute("id"); // e.g. "t3_abc123"
      if (!postId || seen.has(postId)) return;
      seen.add(postId);

      const title =
        el.getAttribute("post-title")?.trim() ||
        el.querySelector('[slot="title"]')?.textContent?.trim() ||
        "";

      const bodyContent =
        el.querySelector('[slot="text-body"]')?.textContent?.trim() || "";
      const content = bodyContent || title;
      if (!content || content.length < 20) return;

      const author = (el.getAttribute("author") || "").trim();
      const permalink = el.getAttribute("permalink") || "";
      const url = permalink
        ? `https://www.reddit.com${permalink}`
        : window.location.href;

      sendSignal({
        platform: "reddit",
        platform_id: postId,
        url,
        title,
        content,
        author,
        author_url: author ? `https://www.reddit.com/user/${author}` : "",
      });
    }

    // ── Legacy / old.reddit markup fallback ───────────────────────────────────
    function processLegacyPost(el: Element) {
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
      document.querySelectorAll("shreddit-post").forEach(processShredditPost);
      document.querySelectorAll("[data-fullname]").forEach(processLegacyPost);
    }

    scanPage();

    const observer = new MutationObserver(scanPage);
    observer.observe(document.body, { childList: true, subtree: true });

    await runPendingReply({
      settleMs: 3000,
      findReplyBox: findRedditReplyBox,
      findSubmit: findRedditSubmit,
    });
  },
});

function findRedditReplyBox(): HTMLElement | null {
  return (
    (document.querySelector('shreddit-composer [contenteditable="true"]') as HTMLElement | null) ??
    (document.querySelector('[contenteditable="true"]') as HTMLElement | null) ??
    (document.querySelector(".public-DraftEditor-content") as HTMLElement | null)
  );
}

function findRedditSubmit(): HTMLElement | null {
  return (
    (document.querySelector('shreddit-composer button[slot="submit-button"]') as HTMLElement | null) ??
    (document.querySelector('shreddit-composer button[type="submit"]') as HTMLElement | null) ??
    (document.querySelector('button[type="submit"]:not([disabled])') as HTMLElement | null)
  );
}
