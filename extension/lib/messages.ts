export type Platform = "linkedin" | "reddit" | "twitter" | "hackernews";

export interface RawSignal {
  platform: Platform;
  platform_id: string;
  url: string;
  title: string;
  content: string;
  author: string;
  author_url: string;
}

export interface PostReplyPayload {
  replyId: string;
  platform: string;
  targetUrl: string;
  content: string;
}

export type ExtensionMessage =
  | { type: "SIGNAL"; payload: RawSignal }
  | { type: "GET_STATUS"; payload?: undefined }
  | { type: "GET_TAB_ID"; payload?: undefined }
  | { type: "POST_REPLY"; payload: PostReplyPayload }
  | { type: "REPLY_INJECTED"; payload: { replyId: string; tabId: number } }
  | { type: "REPLY_POSTED"; payload: { replyId: string; success: boolean } };

export interface StatusPayload {
  configured: boolean;
  dailyCount: number;
}

// Fire-and-forget — service worker may be sleeping, that's fine.
export function sendSignal(signal: RawSignal): void {
  chrome.runtime.sendMessage({ type: "SIGNAL", payload: signal } as ExtensionMessage).catch(() => {
    // Intentionally swallowed — seen-post dedup prevents resend.
  });
}

export function getStatus(): Promise<{ type: "STATUS"; payload: StatusPayload }> {
  return chrome.runtime.sendMessage({ type: "GET_STATUS" });
}

export function sendPostReply(payload: PostReplyPayload): void {
  chrome.runtime
    .sendMessage({ type: "POST_REPLY", payload } as ExtensionMessage)
    .catch(() => {});
}
