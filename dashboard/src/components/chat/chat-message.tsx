// chat (P2 #15): one message bubble with tool activity, lead cards and confirm buttons.
import { Link } from "@tanstack/react-router";
import { Check, ExternalLink, Loader2, Wrench, X } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { ChatMessage, LeadCard, MentionCard, ToolCall } from "@/lib/chat";

/** Renders **bold** spans; everything else stays plain text. */
function RichText({ text }: { text: string }) {
  const parts = text.split(/(\*\*[^*]+\*\*)/g);
  return (
    <p className="whitespace-pre-wrap text-sm leading-6">
      {parts.map((p, i) =>
        p.startsWith("**") && p.endsWith("**") ? <strong key={i}>{p.slice(2, -2)}</strong> : p,
      )}
    </p>
  );
}

function MentionCardView({ m }: { m: MentionCard }) {
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="mb-1 flex items-center gap-1.5">
        <Badge size="sm" variant="outline">{m.platform}</Badge>
        {m.intent && <Badge size="sm">{m.intent.replace(/_/g, " ")}</Badge>}
        {m.relevance_score != null && (
          <Badge size="sm" variant="surface">{m.relevance_score.toFixed(1)}</Badge>
        )}
      </div>
      <p className="line-clamp-1 text-[13px] font-medium">{m.title || m.content}</p>
      {m.title && <p className="line-clamp-2 text-xs text-muted-foreground">{m.content}</p>}
      <div className="mt-2 flex gap-3 text-xs">
        <Link to="/inbox" search={{ id: m.id }} className="text-primary-ink hover:underline">
          Open in inbox
        </Link>
        {m.url && (
          <a href={m.url} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1 text-muted-foreground hover:text-foreground">
            Source <ExternalLink className="h-3 w-3" />
          </a>
        )}
      </div>
    </div>
  );
}

function LeadCardView({ l }: { l: LeadCard }) {
  const who = l.contact_name || l.username || "Unknown lead";
  return (
    <div className="rounded-lg border border-border bg-card p-3">
      <div className="flex items-center justify-between gap-2">
        <p className="truncate text-[13px] font-medium">{who}</p>
        <Badge size="sm" variant="surface">{l.stage}</Badge>
      </div>
      <p className="text-xs text-muted-foreground">{[l.company, l.platform].filter(Boolean).join(" · ") || "—"}</p>
      <div className="mt-2 text-xs">
        {l.mention_id ? (
          <Link to="/inbox" search={{ id: l.mention_id }} className="text-primary-ink hover:underline">Open in inbox</Link>
        ) : (
          <Link to="/pipeline" className="text-primary-ink hover:underline">Open pipeline</Link>
        )}
      </div>
    </div>
  );
}

interface ConfirmProps {
  call: ToolCall;
  busy: boolean;
  onConfirm: (approve: boolean) => void;
}

function ConfirmBox({ call, busy, onConfirm }: ConfirmProps) {
  return (
    <div className="rounded-lg border border-primary/40 bg-accent-soft/40 p-3">
      <p className="text-[13px] font-medium">Run {call.tool.replace(/_/g, " ")}?</p>
      <pre className="mt-1 overflow-x-auto text-[11px] text-muted-foreground">{JSON.stringify(call.args, null, 2)}</pre>
      {call.status === "pending" ? (
        <div className="mt-2 flex gap-2">
          <Button size="sm" variant="accent" disabled={busy} onClick={() => onConfirm(true)}>
            {busy ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />} Confirm
          </Button>
          <Button size="sm" variant="outline" disabled={busy} onClick={() => onConfirm(false)}>
            <X className="h-3.5 w-3.5" /> Cancel
          </Button>
        </div>
      ) : (
        <p className={cn("mt-2 text-xs", call.status === "error" ? "text-destructive" : "text-muted-foreground")}>
          {call.status === "done" && "Done."}
          {call.status === "cancelled" && "Cancelled."}
          {call.status === "running" && "Running…"}
          {call.status === "error" && (call.error || "Failed.")}
        </p>
      )}
    </div>
  );
}

interface ChatMessageViewProps {
  message: ChatMessage;
  streaming?: boolean;
  busyCall?: string;
  onConfirm?: (messageId: string, callId: string, approve: boolean) => void;
}

export function ChatMessageView({ message, streaming, busyCall, onConfirm }: ChatMessageViewProps) {
  if (message.role === "user") {
    return (
      <div className="flex justify-end">
        <div className="max-w-[80%] rounded-xl bg-foreground px-3.5 py-2 text-background">
          <p className="whitespace-pre-wrap text-sm leading-6">{message.content}</p>
        </div>
      </div>
    );
  }
  const mentions = message.tool_calls.flatMap((c) => c.cards?.mentions ?? []);
  const leads = message.tool_calls.flatMap((c) => c.cards?.leads ?? []);
  const confirms = message.tool_calls.filter((c) => c.confirm);
  return (
    <div className="space-y-2.5">
      {message.tool_calls.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {message.tool_calls.map((c) => (
            <span key={c.id} className="inline-flex items-center gap-1 rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
              <Wrench className="h-3 w-3" /> {c.tool}
              {c.status === "error" && <span className="text-destructive">· {c.error}</span>}
            </span>
          ))}
        </div>
      )}
      {message.content ? (
        <RichText text={message.content} />
      ) : (
        streaming && <p className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-3.5 w-3.5 animate-spin" /> Thinking…</p>
      )}
      {confirms.map((c) => (
        <ConfirmBox key={c.id} call={c} busy={busyCall === c.id} onConfirm={(ok) => onConfirm?.(message.id, c.id, ok)} />
      ))}
      {(mentions.length > 0 || leads.length > 0) && (
        <div className="grid gap-2 sm:grid-cols-2">
          {mentions.map((m, i) => <MentionCardView key={`m-${m.id}-${i}`} m={m} />)}
          {leads.map((l, i) => <LeadCardView key={`l-${l.id}-${i}`} l={l} />)}
        </div>
      )}
    </div>
  );
}
