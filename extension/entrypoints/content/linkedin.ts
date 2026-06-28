import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../../lib/messages";
import { runPendingReply } from "../../lib/reply";

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

    await runPendingReply({
      settleMs: 3000,
      openComposer: () => {
        // Reveal the comment editor by clicking the post's "Comment" action.
        const trigger =
          (document.querySelector(
            '.comments-comment-box__form-container [placeholder]',
          ) as HTMLElement | null) ??
          (document.querySelector(
            'button[aria-label^="Comment"]',
          ) as HTMLElement | null);
        trigger?.click();
      },
      findReplyBox: () =>
        (document.querySelector(
          ".comments-comment-box__form-container [contenteditable]",
        ) as HTMLElement | null) ??
        (document.querySelector(".ql-editor[contenteditable]") as HTMLElement | null),
      findSubmit: () =>
        document.querySelector(
          ".comments-comment-box__submit-button",
        ) as HTMLElement | null,
    });
  },
});
