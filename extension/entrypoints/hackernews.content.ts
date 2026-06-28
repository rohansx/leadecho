import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../lib/messages";
import { runPendingReply } from "../lib/reply";

export default defineContentScript({
  matches: ["https://news.ycombinator.com/*"],
  runAt: "document_idle",
  async main() {
    document.querySelectorAll("tr.athing").forEach((row) => {
      const id = row.getAttribute("id");
      if (!id) return;

      const titleEl = row.querySelector(".titleline > a") as HTMLAnchorElement | null;
      if (!titleEl) return;

      const title = titleEl.textContent?.trim() || "";

      const subRow = row.nextElementSibling;
      const author = subRow?.querySelector(".hnuser")?.textContent?.trim() || "";

      sendSignal({
        platform: "hackernews",
        platform_id: id,
        url: `https://news.ycombinator.com/item?id=${id}`,
        title,
        content: title, // body text requires a separate item fetch; title is sufficient
        author,
        author_url: author ? `https://news.ycombinator.com/user?id=${author}` : "",
      });
    });

    // HN's comment box is a plain server-rendered form, so auto-posting is highly
    // reliable when the user is logged in.
    await runPendingReply({
      settleMs: 1000,
      submitNavigates: true,
      findReplyBox: () =>
        document.querySelector('textarea[name="text"]') as HTMLElement | null,
      findSubmit: () =>
        (document.querySelector(
          'form[action="comment"] input[type="submit"]',
        ) as HTMLElement | null) ??
        (document.querySelector('input[type="submit"]') as HTMLElement | null),
    });
  },
});
