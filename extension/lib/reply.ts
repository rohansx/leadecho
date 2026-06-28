/**
 * Shared "post a pending reply" runner used by every platform content script.
 *
 * Centralising this guarantees consistent, HONEST success reporting: a reply is
 * only reported as posted when the composer was found, text was actually
 * injected, and an enabled submit control was clicked. Any earlier failure
 * reports `success: false` with a reason instead of silently marking the reply
 * as posted.
 */

import { simulateTyping } from "./human-mimicry";

interface PendingReply {
  replyId: string;
  content: string;
}

export interface ReplyConfig {
  /** ms to wait for the page's JS to settle before touching the DOM. */
  settleMs?: number;
  /**
   * Optional step to reveal the composer (e.g. click "Reply"/"Comment").
   * Return false to abort and report failure.
   */
  openComposer?: () => boolean | void | Promise<boolean | void>;
  /** Locates the editable reply box. Polled until found or timeout. */
  findReplyBox: () => HTMLElement | null;
  /** Locates the submit control (may be a <button> or a role=button div). */
  findSubmit: () => HTMLElement | null;
  /**
   * Set for classic form submits that trigger a full-page navigation (e.g. HN).
   * The success report is sent just before clicking, since the content script is
   * torn down by the navigation and couldn't report afterwards.
   */
  submitNavigates?: boolean;
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

function isDisabled(el: HTMLElement): boolean {
  if (el instanceof HTMLButtonElement && el.disabled) return true;
  return el.getAttribute("aria-disabled") === "true";
}

async function waitForElement<T extends HTMLElement>(
  getter: () => T | null,
  timeoutMs: number,
): Promise<T | null> {
  const start = Date.now();
  for (;;) {
    const el = getter();
    if (el) return el;
    if (Date.now() - start >= timeoutMs) return null;
    await sleep(200);
  }
}

async function getTabId(): Promise<number | null> {
  try {
    const resp = await chrome.runtime.sendMessage({ type: "GET_TAB_ID" });
    return resp?.tabId ?? null;
  } catch {
    return null;
  }
}

/**
 * Reads the pending reply (if any) for this tab and attempts to post it.
 * Safe to call unconditionally on every content-script load.
 */
export async function runPendingReply(cfg: ReplyConfig): Promise<void> {
  const tabId = await getTabId();
  if (tabId == null) return;

  const key = `pending_reply_${tabId}`;
  const stored = await chrome.storage.session.get(key);
  const pending = stored[key] as PendingReply | undefined;
  if (!pending) return;

  // Consume immediately so a reload can't double-post.
  await chrome.storage.session.remove(key);

  const report = (success: boolean, reason?: string): void => {
    chrome.runtime
      .sendMessage({
        type: "REPLY_POSTED",
        payload: { replyId: pending.replyId, success, reason },
      })
      .catch(() => {});
  };

  try {
    await sleep(cfg.settleMs ?? 2500);

    if (cfg.openComposer) {
      const opened = await cfg.openComposer();
      if (opened === false) {
        report(false, "could not open composer");
        return;
      }
      await sleep(900);
    }

    const box = await waitForElement(cfg.findReplyBox, 8000);
    if (!box) {
      report(false, "reply box not found");
      return;
    }

    const typed = await simulateTyping(box, pending.content);
    if (!typed) {
      report(false, "failed to enter reply text");
      return;
    }

    // Give the editor a moment to enable its submit control.
    await sleep(500);
    const submit = await waitForElement(cfg.findSubmit, 4000);
    if (!submit || isDisabled(submit)) {
      report(false, "submit control unavailable");
      return;
    }

    // A navigating form submit destroys this content script, so report first.
    if (cfg.submitNavigates) {
      report(true);
      submit.click();
      return;
    }

    submit.click();
    await sleep(1500);
    report(true);
  } catch (err) {
    report(false, err instanceof Error ? err.message : "unexpected error");
  }
}
