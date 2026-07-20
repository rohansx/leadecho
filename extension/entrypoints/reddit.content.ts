import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../lib/messages";
import { runPendingReply } from "../lib/reply";

export default defineContentScript({
  matches: ["https://www.reddit.com/*", "https://reddit.com/*"],
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

    // Reddit is a high-churn SPA; coalesce bursts of mutations into one scan
    // every ~500ms instead of re-scanning the whole page on every mutation.
    let scanScheduled = false;
    const scheduleScan = () => {
      if (scanScheduled) return;
      scanScheduled = true;
      setTimeout(() => {
        scanScheduled = false;
        scanPage();
      }, 500);
    };

    const observer = new MutationObserver(scheduleScan);
    observer.observe(document.body, { childList: true, subtree: true });

    await runPendingReply({
      settleMs: 2500,
      openComposer: openRedditComposer,
      findReplyBox: findRedditReplyBox,
      findSubmit: findRedditSubmit,
    });
  },
});

/** Reveal the top-level comment composer (often collapsed until clicked). */
async function openRedditComposer(): Promise<boolean> {
  if (findRedditReplyBox()) return true;

  const triggers: Array<() => HTMLElement | null> = [
    () =>
      document.querySelector(
        'shreddit-composer [contenteditable="true"]',
      ) as HTMLElement | null,
    () =>
      document.querySelector(
        '[placeholder*="Add a comment" i], [aria-placeholder*="Add a comment" i]',
      ) as HTMLElement | null,
    () =>
      document.querySelector(
        'div[data-testid="comment-submission-form-richtext"], faceplate-textarea-input',
      ) as HTMLElement | null,
    () => {
      const buttons = Array.from(document.querySelectorAll("button, div[role='button']"));
      const hit = buttons.find((el) => {
        const t = (el.textContent || "").trim().toLowerCase();
        return t === "add a comment" || t === "comment" || t.startsWith("add a comment");
      });
      return (hit as HTMLElement | undefined) ?? null;
    },
  ];

  for (const get of triggers) {
    const el = get();
    if (!el) continue;
    el.click();
    await new Promise((r) => setTimeout(r, 600));
    if (findRedditReplyBox()) return true;
  }

  // Composer may already be in the DOM but not focused — treat as opened so
  // the poller can still find it.
  return true;
}

function findRedditReplyBox(): HTMLElement | null {
  // Prefer the shreddit comment composer only — a bare [contenteditable] match
  // can hit unrelated editors (search, chat) and never enable Comment submit.
  return (
    (document.querySelector(
      'shreddit-composer[slot="comment-composer"] [contenteditable="true"]',
    ) as HTMLElement | null) ??
    (document.querySelector(
      'shreddit-composer [contenteditable="true"]',
    ) as HTMLElement | null) ??
    (document.querySelector(
      '[data-test-id="comment-submission-form-richtext"] [contenteditable="true"]',
    ) as HTMLElement | null) ??
    (document.querySelector(".public-DraftEditor-content") as HTMLElement | null)
  );
}

function findRedditSubmit(): HTMLElement | null {
  return (
    (document.querySelector(
      'shreddit-composer button[slot="submit-button"]',
    ) as HTMLElement | null) ??
    (document.querySelector(
      'shreddit-composer button[type="submit"]',
    ) as HTMLElement | null) ??
    (document.querySelector(
      'shreddit-composer button:not([disabled])',
    ) as HTMLElement | null)
  );
}
