import type { RawSignal } from "./messages";

export interface IngestResponse {
  inserted: number;
}

export interface QueuedReply {
  id: string;
  content: string;
  edited_content: string | null;
  mention_id: string;
  platform: string;
  url: string;
  title: string | null;
}

export interface ExtensionMention {
  id: string;
  platform: string;
  url: string;
  title: string | null;
  content: string;
  author_username: string | null;
  intent: string | null;
  relevance_score: number | null;
  created_at: string;
}

export async function postSignals(
  apiUrl: string,
  apiKey: string,
  signals: RawSignal[],
): Promise<IngestResponse> {
  const res = await fetch(`${apiUrl}/api/v1/extension/signals`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Extension-Key": apiKey,
    },
    body: JSON.stringify({ signals }),
  });
  if (!res.ok) throw new Error(`signal ingest failed: ${res.status}`);
  return res.json();
}

export async function getReplyQueue(apiUrl: string, apiKey: string): Promise<QueuedReply[]> {
  const res = await fetch(`${apiUrl}/api/v1/extension/reply-queue`, {
    headers: { "X-Extension-Key": apiKey },
  });
  if (!res.ok) throw new Error(`reply queue failed: ${res.status}`);
  return res.json();
}

export async function getMentions(apiUrl: string, apiKey: string): Promise<ExtensionMention[]> {
  const res = await fetch(`${apiUrl}/api/v1/extension/mentions`, {
    headers: { "X-Extension-Key": apiKey },
  });
  if (!res.ok) throw new Error(`mentions failed: ${res.status}`);
  return res.json();
}

export async function markReplyPosted(
  apiUrl: string,
  apiKey: string,
  replyId: string,
): Promise<void> {
  const res = await fetch(`${apiUrl}/api/v1/extension/replies/${replyId}/mark-posted`, {
    method: "PATCH",
    headers: { "X-Extension-Key": apiKey },
  });
  if (!res.ok) throw new Error(`mark posted failed: ${res.status}`);
}

export async function testApiKey(apiUrl: string, apiKey: string): Promise<boolean> {
  try {
    const res = await fetch(`${apiUrl}/api/v1/extension/signals`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Extension-Key": apiKey,
      },
      body: JSON.stringify({ signals: [] }),
    });
    return res.ok;
  } catch {
    return false;
  }
}
