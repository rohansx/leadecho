// chat (P2 #15): "Ask LeadEcho" — in-app assistant over the agent tools.
import { createFileRoute, useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageSquarePlus, Send, Sparkles, Trash2 } from "lucide-react";
import { Text } from "@/components/ui/text";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { ChatMessageView } from "@/components/chat/chat-message";
import { cn } from "@/lib/utils";
import {
  SUGGESTED_PROMPTS,
  confirmChatAction,
  deleteChatThread,
  listChatMessages,
  listChatThreads,
  sendChat,
  type ChatMessage,
  type ToolCall,
} from "@/lib/chat";

interface ChatSearch {
  thread?: string;
}

export const Route = createFileRoute("/_dashboard/chat")({
  validateSearch: (s: Record<string, unknown>): ChatSearch => ({
    thread: typeof s.thread === "string" ? s.thread : undefined,
  }),
  component: ChatPage,
});

const THREADS_KEY = ["chat-threads"];
const messagesKey = (id?: string) => ["chat-messages", id ?? "new"];

function draftMessage(role: ChatMessage["role"], content: string): ChatMessage {
  return { id: `local-${role}-${Date.now()}`, thread_id: "", role, content, tool_calls: [], created_at: new Date().toISOString() };
}

function upsertCall(calls: ToolCall[], call: ToolCall): ToolCall[] {
  return calls.some((c) => c.id === call.id) ? calls.map((c) => (c.id === call.id ? call : c)) : [...calls, call];
}

function ChatPage() {
  const { thread } = Route.useSearch();
  const navigate = useNavigate({ from: "/chat" });
  const qc = useQueryClient();
  const [input, setInput] = useState("");
  const [pending, setPending] = useState<ChatMessage[]>([]);
  const [streaming, setStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busyCall, setBusyCall] = useState<string>();
  const bottomRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);

  const threads = useQuery({ queryKey: THREADS_KEY, queryFn: listChatThreads });
  const messages = useQuery({
    queryKey: messagesKey(thread),
    queryFn: () => listChatMessages(thread as string),
    enabled: !!thread,
  });
  const shown = [...(thread ? (messages.data?.data ?? []) : []), ...pending];

  useEffect(() => bottomRef.current?.scrollIntoView({ behavior: "smooth" }), [shown.length, pending]);
  useEffect(() => () => abortRef.current?.abort(), []);

  const patchAssistant = (fn: (m: ChatMessage) => ChatMessage) =>
    setPending((p) => (p.length === 2 ? [p[0], fn(p[1])] : p));

  async function send(text: string) {
    const message = text.trim();
    if (!message || streaming) return;
    setInput("");
    setError(null);
    setStreaming(true);
    setPending([draftMessage("user", message), draftMessage("assistant", "")]);
    const ctrl = new AbortController();
    abortRef.current = ctrl;
    let threadId = thread;
    try {
      await sendChat(message, thread, (event, data) => {
        const d = data as Record<string, unknown>;
        if (event === "thread") threadId = d.thread_id as string;
        else if (event === "tool_call" || event === "tool_result" || event === "confirm")
          patchAssistant((m) => ({ ...m, tool_calls: upsertCall(m.tool_calls, d as unknown as ToolCall) }));
        else if (event === "answer") patchAssistant((m) => ({ ...m, content: d.content as string }));
        else if (event === "error") setError(d.error as string);
      }, ctrl.signal);
    } catch (e) {
      if (!ctrl.signal.aborted) setError(e instanceof Error ? e.message : "Chat failed");
    } finally {
      setStreaming(false);
      // Unmounted/aborted mid-stream: don't refetch or navigate (would yank the user back to /chat).
      if (ctrl.signal.aborted) return;
      if (threadId) {
        await qc.invalidateQueries({ queryKey: messagesKey(threadId) });
        await qc.refetchQueries({ queryKey: messagesKey(threadId) });
        qc.invalidateQueries({ queryKey: THREADS_KEY });
        if (threadId !== thread) navigate({ search: { thread: threadId } });
      }
      setPending([]);
    }
  }

  const confirm = useMutation({
    mutationFn: (v: { messageId: string; callId: string; approve: boolean }) => {
      setBusyCall(v.callId);
      return confirmChatAction(v.messageId, v.callId, v.approve);
    },
    onSettled: () => {
      setBusyCall(undefined);
      qc.invalidateQueries({ queryKey: messagesKey(thread) });
    },
    onError: (e) => setError(e instanceof Error ? e.message : "Action failed"),
  });

  const remove = useMutation({
    mutationFn: deleteChatThread,
    onSuccess: (_, id) => {
      qc.invalidateQueries({ queryKey: THREADS_KEY });
      if (id === thread) navigate({ search: {} });
    },
  });

  return (
    <div className="mx-auto flex h-[calc(100vh-var(--header-height)-3rem)] max-w-6xl gap-5">
      <aside className="hidden w-60 shrink-0 flex-col md:flex">
        <Button variant="outline" size="sm" onClick={() => navigate({ search: {} })} disabled={streaming}>
          <MessageSquarePlus className="h-4 w-4" /> New chat
        </Button>
        <div className="mt-3 flex-1 space-y-px overflow-y-auto">
          {(threads.data?.data ?? []).map((t) => (
            <div key={t.id} className={cn("group flex items-center rounded-md", t.id === thread ? "bg-accent" : "hover:bg-accent")}>
              <button
                className="flex-1 truncate px-2.5 py-1.5 text-left text-[13px]"
                onClick={() => !streaming && navigate({ search: { thread: t.id } })}
                title={t.title}
              >
                {t.title || "Untitled"}
              </button>
              <button
                aria-label="Delete thread"
                className="px-2 text-muted-foreground opacity-0 hover:text-destructive group-hover:opacity-100"
                onClick={() => remove.mutate(t.id)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            </div>
          ))}
        </div>
      </aside>

      <section className="flex min-w-0 flex-1 flex-col">
        <div className="flex-1 space-y-5 overflow-y-auto pb-4">
          {shown.length === 0 ? (
            <div className="mx-auto max-w-xl pt-10 text-center">
              <Sparkles className="mx-auto h-6 w-6 text-primary-ink" />
              <Text as="h2" className="mt-2">Ask LeadEcho</Text>
              <Text as="p" className="mt-1 text-muted-foreground">
                Ask about your leads, mentions and revenue. Actions like moving a lead or queueing a reply wait for your confirmation.
              </Text>
              <div className="mt-6 grid gap-2 sm:grid-cols-2">
                {SUGGESTED_PROMPTS.map((p) => (
                  <button key={p} onClick={() => send(p)} className="rounded-lg border border-border bg-card px-3 py-2.5 text-left text-[13px] hover:bg-accent">
                    {p}
                  </button>
                ))}
              </div>
            </div>
          ) : (
            shown.map((m, i) => (
              <ChatMessageView
                key={m.id}
                message={m}
                streaming={streaming && i === shown.length - 1}
                busyCall={busyCall}
                onConfirm={(messageId, callId, approve) => confirm.mutate({ messageId, callId, approve })}
              />
            ))
          )}
          <div ref={bottomRef} />
        </div>
        {error && <p className="mb-2 text-xs text-destructive">{error}</p>}
        <form
          className="flex items-end gap-2 border-t border-border pt-3"
          onSubmit={(e) => {
            e.preventDefault();
            send(input);
          }}
        >
          <Textarea
            value={input}
            maxLength={4000}
            rows={2}
            placeholder="Ask about your leads…"
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                send(input);
              }
            }}
          />
          <Button type="submit" variant="accent" size="icon" disabled={streaming || !input.trim()} aria-label="Send">
            <Send className="h-4 w-4" />
          </Button>
        </form>
      </section>
    </div>
  );
}
