// Lead assistant: a chat scoped to the inbox item that is open. The server
// adds what LeadEcho knows about the lead to every turn, and the assistant
// can search the web and look up the company's recent job postings.
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowUp, Loader2, Sparkles } from "lucide-react";
import { Button } from "@/components/ui/button";
import { ChatMessageView } from "@/components/chat/chat-message";
import {
  getChatCustomers,
  listChatMessages,
  saveChatCustomers,
  sendChat,
  type ChatMessage,
  type ToolCall,
} from "@/lib/chat";
import type { Mention } from "@/lib/types";

const HIRING_PROMPTS = [
  "Do they hire often?",
  "Is this company like our customers?",
  "What should I know before I write to them?",
];
const POST_PROMPTS = ["Is this person a real buyer?", "What does their company do?", "How should I reply?"];

const threadKey = (mentionId: string) => `le-lead-thread:${mentionId}`;

function readThread(mentionId: string): string | undefined {
  try {
    return localStorage.getItem(threadKey(mentionId)) ?? undefined;
  } catch {
    return undefined; // private window or blocked storage: the chat still works, it just starts fresh
  }
}

function rememberThread(mentionId: string, threadId: string) {
  try {
    localStorage.setItem(threadKey(mentionId), threadId);
  } catch {
    /* see readThread */
  }
}

function draft(role: ChatMessage["role"], content: string): ChatMessage {
  return { id: `pending-${role}`, thread_id: "", role, content, tool_calls: [], created_at: new Date().toISOString() };
}

function upsertCall(calls: ToolCall[], call: ToolCall): ToolCall[] {
  return calls.some((c) => c.id === call.id) ? calls.map((c) => (c.id === call.id ? call : c)) : [...calls, call];
}

const CUSTOMERS_KEY = ["chat-customers"];

/** Lets the user record who their customers are, so "is this lead like our customers?" has an answer. */
function YourCustomers() {
  const qc = useQueryClient();
  const saved = useQuery({ queryKey: CUSTOMERS_KEY, queryFn: getChatCustomers });
  const [open, setOpen] = useState(false);
  const [text, setText] = useState<string | null>(null);
  const value = text ?? saved.data?.customers ?? "";
  const save = useMutation({
    mutationFn: () => saveChatCustomers(value),
    onSuccess: (d) => {
      qc.setQueryData(CUSTOMERS_KEY, d);
      setText(null);
      setOpen(false);
    },
  });
  const count = (saved.data?.customers ?? "").split("\n").filter((l) => l.trim()).length;

  return (
    <div className="mb-3 text-xs">
      <button type="button" onClick={() => setOpen((o) => !o)} className="text-muted-foreground underline underline-offset-2 hover:text-foreground">
        {count > 0 ? `Your customers (${count} listed)` : "Add your customers so I can compare leads with them"}
      </button>
      {open && (
        <div className="mt-2 space-y-2">
          <textarea
            value={value}
            onChange={(e) => setText(e.target.value)}
            rows={4}
            maxLength={3000}
            placeholder={"One per line, with a word on each if you like:\nAcme: 40-person fintech, hires 5 engineers a quarter"}
            aria-label="Your customers"
            className="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground"
          />
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={() => save.mutate()} disabled={save.isPending}>
              {save.isPending ? "Saving…" : "Save"}
            </Button>
            {save.isError && <span className="text-destructive">Could not save. Try again.</span>}
          </div>
        </div>
      )}
    </div>
  );
}

export function LeadAssistant({ mention }: { mention: Mention }) {
  const qc = useQueryClient();
  const [thread, setThread] = useState<string | undefined>(() => readThread(mention.id));
  const [input, setInput] = useState("");
  const [pending, setPending] = useState<ChatMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const messagesKey = ["lead-chat", thread];

  const messages = useQuery({
    queryKey: messagesKey,
    queryFn: () => listChatMessages(thread as string),
    // Not while streaming: the stored user message would show next to the pending one.
    enabled: !!thread && !streaming,
    retry: false,
  });
  // A thread deleted elsewhere (or from another account): start a new one.
  useEffect(() => {
    if (messages.isError && thread) {
      setThread(undefined);
      try {
        localStorage.removeItem(threadKey(mention.id));
      } catch {
        /* see readThread */
      }
    }
  }, [messages.isError, thread, mention.id]);
  useEffect(() => () => abortRef.current?.abort(), []);

  const isHiring = (mention.platform_metadata as { signal?: string } | null)?.signal === "hiring";
  const shown = [...(messages.data?.data ?? []), ...pending];

  const patchAssistant = (fn: (m: ChatMessage) => ChatMessage) =>
    setPending((p) => (p.length === 2 ? [p[0], fn(p[1])] : p));

  async function send(text: string) {
    const message = text.trim();
    if (!message || streaming) return;
    setInput("");
    setError(null);
    setStreaming(true);
    setPending([draft("user", message), draft("assistant", "")]);
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    let threadId = thread;
    try {
      await sendChat(
        message,
        thread,
        (event, data) => {
          const d = data as Record<string, unknown>;
          if (event === "thread") {
            threadId = d.thread_id as string;
            rememberThread(mention.id, threadId);
          } else if (event === "tool_call" || event === "tool_result" || event === "confirm")
            patchAssistant((m) => ({ ...m, tool_calls: upsertCall(m.tool_calls, d as unknown as ToolCall) }));
          else if (event === "answer") patchAssistant((m) => ({ ...m, content: d.content as string }));
          else if (event === "error") setError(d.error as string);
        },
        ctrl.signal,
        mention.id,
      );
    } catch (e) {
      if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : "The assistant could not answer");
    } finally {
      if (!ctrl.signal.aborted) {
        if (threadId) {
          const id = threadId;
          await qc.fetchQuery({ queryKey: ["lead-chat", id], queryFn: () => listChatMessages(id), staleTime: 0 });
          setThread(id);
        }
        setPending([]);
        setStreaming(false);
      }
    }
  }

  return (
    <section className="rounded-xl border border-border bg-card p-4" aria-label="Ask about this lead">
      <div className="mb-3 flex items-center gap-2">
        <Sparkles className="h-4 w-4 text-primary-ink" />
        <h4 className="text-sm font-medium">Ask about this lead</h4>
        <span className="text-xs text-muted-foreground">It knows this lead and your product, and can search the web.</span>
      </div>

      <YourCustomers />

      {shown.length > 0 && (
        <div className="mb-3 max-h-[28rem] space-y-4 overflow-y-auto pr-1">
          {shown.map((m, i) => (
            <ChatMessageView key={m.id} message={m} streaming={streaming && i === shown.length - 1} />
          ))}
        </div>
      )}

      {shown.length === 0 && (
        <div className="mb-3 flex flex-wrap gap-2">
          {(isHiring ? HIRING_PROMPTS : POST_PROMPTS).map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => send(p)}
              disabled={streaming}
              className="rounded-full border border-border bg-background px-3 py-1.5 text-[13px] text-foreground hover:bg-muted disabled:opacity-50"
            >
              {p}
            </button>
          ))}
        </div>
      )}

      {error && <p className="mb-2 text-xs text-destructive">{error}</p>}

      <form
        className="flex items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          void send(input);
        }}
      >
        <textarea
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey) {
              e.preventDefault();
              void send(input);
            }
          }}
          rows={1}
          maxLength={4000}
          placeholder={isHiring ? "Ask about this company or contact…" : "Ask about this person or post…"}
          aria-label="Question about this lead"
          className="min-h-[38px] flex-1 resize-y rounded-lg border border-border bg-background px-3 py-2 text-sm text-foreground"
        />
        <Button type="submit" size="sm" disabled={streaming || !input.trim()} aria-label="Send question">
          {streaming ? <Loader2 className="h-4 w-4 animate-spin" /> : <ArrowUp className="h-4 w-4" />}
        </Button>
      </form>
      <p className="mt-2 text-xs text-muted-foreground">Each question uses credits; each web or job lookup uses one more.</p>
    </section>
  );
}
