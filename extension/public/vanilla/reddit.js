// LeadEcho Reddit signal capture — vanilla script, no polyfill, no storage access.
// Runs as a plain <script> via chrome.scripting to avoid the WXT polyfill crash
// ("Access to storage is not allowed from this context") that kills the content
// script before main() can run.
(function () {
  "use strict";
  if (window.__leadechoRedditInjected) return;
  window.__leadechoRedditInjected = true;

  var seen = new Set();

  function sendSignal(signal) {
    try {
      chrome.runtime.sendMessage({ type: "SIGNAL", payload: signal }).catch(function () {});
    } catch (e) {
      console.warn("[LeadEcho] Failed to send signal:", e);
    }
  }

  function processShredditPost(el) {
    var postId = el.getAttribute("id");
    if (!postId || seen.has(postId)) return;
    seen.add(postId);

    var title =
      (el.getAttribute("post-title") || "").trim() ||
      (el.querySelector('[slot="title"]') || {}).textContent &&
        el.querySelector('[slot="title"]').textContent.trim() ||
      "";

    var bodyEl = el.querySelector('[slot="text-body"]');
    var bodyContent = bodyEl ? (bodyEl.textContent || "").trim() : "";
    var content = bodyContent || title;

    console.log("[LeadEcho] Reddit post found:", postId, "title:", title.slice(0, 50), "content length:", content.length);

    if (!content || content.length < 20) {
      console.log("[LeadEcho] Skipping post (content too short):", postId);
      return;
    }

    var author = (el.getAttribute("author") || "").trim();
    var permalink = el.getAttribute("permalink") || "";
    var url = permalink ? "https://www.reddit.com" + permalink : window.location.href;

    console.log("[LeadEcho] Sending signal for post:", postId);
    sendSignal({
      platform: "reddit",
      platform_id: postId,
      url: url,
      title: title,
      content: content,
      author: author,
      author_url: author ? "https://www.reddit.com/user/" + author : "",
    });
  }

  function processLegacyPost(el) {
    var postId =
      el.getAttribute("data-fullname") ||
      (el.closest("[data-fullname]") && el.closest("[data-fullname]").getAttribute("data-fullname"));
    if (!postId || seen.has(postId)) return;
    seen.add(postId);

    var titleEl = el.querySelector("h3");
    var title = titleEl ? (titleEl.textContent || "").trim() : "";

    var bodyEl =
      el.querySelector(".RichTextJSON-root") ||
      el.querySelector('[data-click-id="text"] div');
    var bodyContent = bodyEl ? (bodyEl.textContent || "").trim() : "";
    var content = bodyContent || title;

    if (!content || content.length < 20) return;

    var authorEl =
      el.querySelector('[data-testid="post_author_link"]') ||
      el.querySelector('a[href^="/user/"]');
    var author = authorEl ? (authorEl.textContent || "").trim() : "";

    var linkEl = el.querySelector('a[data-click-id="body"]');
    var url = linkEl ? linkEl.href : window.location.href;

    sendSignal({
      platform: "reddit",
      platform_id: postId,
      url: url,
      title: title,
      content: content,
      author: author,
      author_url: author
        ? "https://www.reddit.com/user/" + author.replace(/^u\//, "")
        : "",
    });
  }

  function scanPage() {
    var shredditPosts = document.querySelectorAll("shreddit-post");
    var legacyPosts = document.querySelectorAll("[data-fullname]");
    if (shredditPosts.length === 0 && legacyPosts.length === 0) return;
    console.log("[LeadEcho] Reddit scan:", shredditPosts.length, "shreddit posts,", legacyPosts.length, "legacy posts");
    shredditPosts.forEach(processShredditPost);
    legacyPosts.forEach(processLegacyPost);
  }

  scanPage();

  var scanScheduled = false;
  function scheduleScan() {
    if (scanScheduled) return;
    scanScheduled = true;
    setTimeout(function () {
      scanScheduled = false;
      scanPage();
    }, 500);
  }

  var observer = new MutationObserver(scheduleScan);
  observer.observe(document.body, { childList: true, subtree: true });

  console.log("[LeadEcho] Reddit content script loaded (vanilla)");
})();