import { defineContentScript } from "wxt/sandbox";
import { sendSignal } from "../../lib/messages";

export default defineContentScript({
  matches: ["https://news.ycombinator.com/*"],
  runAt: "document_idle",
  main() {
    document.querySelectorAll("tr.athing").forEach((row) => {
      const id = row.getAttribute("id");
      if (!id) return;

      const titleEl = row.querySelector(".titleline > a") as HTMLAnchorElement | null;
      if (!titleEl) return;

      const title = titleEl.textContent?.trim() || "";
      const url = titleEl.href || `https://news.ycombinator.com/item?id=${id}`;

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
  },
});
