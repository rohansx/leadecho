// Minimal Server-Sent Events over fetch (EventSource cannot POST a body).

export type SSEHandler = (event: string, data: unknown) => void;

/** Parses complete "event:/data:" blocks out of buf; returns the unparsed rest. */
export function parseSSE(buf: string, onEvent: SSEHandler): string {
  const blocks = buf.split("\n\n");
  const rest = blocks.pop() ?? "";
  for (const block of blocks) {
    let event = "message";
    const data: string[] = [];
    for (const line of block.split("\n")) {
      if (line.startsWith("event:")) event = line.slice(6).trim();
      else if (line.startsWith("data:")) data.push(line.slice(5).trim());
    }
    if (data.length === 0) continue;
    try {
      onEvent(event, JSON.parse(data.join("\n")));
    } catch {
      onEvent(event, data.join("\n"));
    }
  }
  return rest;
}

/** POSTs JSON and dispatches each SSE event until the stream ends. */
export async function streamSSE(
  path: string,
  body: unknown,
  onEvent: SSEHandler,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(`/api/v1${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
    body: JSON.stringify(body),
    signal,
  });
  if (!res.ok || !res.body) {
    let message = `Request failed (${res.status})`;
    try {
      const parsed = await res.json();
      if (parsed && typeof parsed.error === "string") message = parsed.error;
    } catch {
      // not JSON
    }
    throw new Error(message);
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader();
  let buf = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buf = parseSSE(buf + value, onEvent);
  }
  if (buf.trim()) parseSSE(buf + "\n\n", onEvent);
}
