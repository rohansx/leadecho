// Person360 dossier: the model-written profile in persons.metadata.dossier.
// Pains and buying signals link to the post they came from.
import { Copy, Check } from "lucide-react";
import { useState } from "react";
import type { PersonPost } from "@/lib/types";

interface Cited {
  text: string;
  post_id: string;
}

interface Dossier {
  headline: string;
  about?: string;
  role?: string;
  company?: string;
  seniority?: string;
  location?: string;
  expertise?: string[];
  interests?: string[];
  pains?: Cited[];
  buying_signals?: Cited[];
  tools?: string[];
  best_channel?: string;
  channel_reason?: string;
  opener?: string;
  cautions?: string[];
  generated_at?: string;
}

const CHANNEL_LABEL: Record<string, string> = {
  email: "Email",
  linkedin: "LinkedIn",
  x: "X",
  bluesky: "Bluesky",
  mastodon: "Mastodon",
  booking: "Book a call",
  github: "GitHub",
  reply: "Reply where they posted",
};

function readDossier(meta: Record<string, unknown> | undefined): Dossier | null {
  const d = meta?.dossier;
  return d && typeof d === "object" && typeof (d as Dossier).headline === "string" ? (d as Dossier) : null;
}

function safeHref(u: string | undefined): string | undefined {
  try {
    const p = u ? new URL(u) : null;
    return p && (p.protocol === "https:" || p.protocol === "http:") ? p.toString() : undefined;
  } catch {
    return undefined;
  }
}

function Chips({ items }: { items?: string[] }) {
  if (!items?.length) return null;
  return (
    <div className="flex flex-wrap gap-1.5">
      {items.map((t) => (
        <span key={t} className="text-xs rounded-full bg-secondary px-2 py-0.5">
          {t}
        </span>
      ))}
    </div>
  );
}

function CitedList({ title, items, posts }: { title: string; items?: Cited[]; posts: Map<string, PersonPost> }) {
  if (!items?.length) return null;
  return (
    <div>
      <h6 className="text-xs font-medium text-muted-foreground mb-1">{title}</h6>
      <ul className="space-y-1 text-sm">
        {items.map((c) => {
          const post = posts.get(c.post_id);
          const href = safeHref(post?.url);
          return (
            <li key={c.post_id + c.text}>
              {c.text}
              {href && (
                <a href={href} target="_blank" rel="noreferrer noopener" className="ml-1.5 text-xs text-muted-foreground hover:text-foreground hover:underline">
                  {post?.platform}
                </a>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

export function DossierCard({ metadata, posts }: { metadata?: Record<string, unknown>; posts: PersonPost[] }) {
  const d = readDossier(metadata);
  const [copied, setCopied] = useState(false);
  if (!d) {
    return (
      <div className="rounded-xl border border-dashed border-border p-3 text-xs text-muted-foreground">
        The profile is written once we know who they are; it appears here within a minute or two.
      </div>
    );
  }
  const byId = new Map(posts.map((p) => [p.mention_id, p]));
  const facts = [d.role && d.company ? `${d.role} at ${d.company}` : d.role || d.company, d.seniority !== "unknown" ? d.seniority : "", d.location]
    .filter(Boolean)
    .join(" · ");
  const copyOpener = async () => {
    if (!d.opener) return;
    try {
      await navigator.clipboard.writeText(d.opener);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // clipboard blocked: the text is on screen to copy by hand
    }
  };

  return (
    <div className="rounded-xl border border-border bg-card p-4 space-y-3">
      <div className="flex items-center justify-between gap-2">
        <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Profile</h5>
        {d.generated_at && (
          <span className="text-[11px] text-muted-foreground">updated {new Date(d.generated_at).toLocaleDateString()}</span>
        )}
      </div>
      <div>
        <p className="font-medium">{d.headline}</p>
        {facts && <p className="text-xs text-muted-foreground mt-0.5">{facts}</p>}
      </div>
      {d.about && <p className="text-sm leading-relaxed text-foreground-soft">{d.about}</p>}
      <Chips items={[...(d.expertise ?? []), ...(d.interests ?? [])]} />
      <CitedList title="Pains they described" items={d.pains} posts={byId} />
      <CitedList title="Buying signals" items={d.buying_signals} posts={byId} />
      {d.tools?.length ? (
        <p className="text-sm">
          <span className="text-xs text-muted-foreground mr-1.5">Tools they mention</span>
          {d.tools.join(", ")}
        </p>
      ) : null}
      {(d.best_channel || d.opener) && (
        <div className="rounded-lg bg-secondary/60 p-3 space-y-1.5">
          {d.best_channel && (
            <p className="text-sm">
              <span className="font-medium">{CHANNEL_LABEL[d.best_channel] ?? d.best_channel}</span>
              {d.channel_reason && <span className="text-muted-foreground"> · {d.channel_reason}</span>}
            </p>
          )}
          {d.opener && (
            <div className="flex items-start gap-2">
              <p className="text-sm italic flex-1">“{d.opener}”</p>
              <button type="button" onClick={copyOpener} className="shrink-0 text-muted-foreground hover:text-foreground" aria-label="Copy opener">
                {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
              </button>
            </div>
          )}
        </div>
      )}
      {d.cautions?.length ? (
        <ul className="text-xs text-amber-700 dark:text-amber-400 space-y-0.5">
          {d.cautions.map((c) => (
            <li key={c}>⚠ {c}</li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function PersonPosts({ posts, current }: { posts: PersonPost[]; current?: string }) {
  if (posts.length <= 1) return null;
  return (
    <div>
      <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-2">Their posts in your inbox ({posts.length})</h5>
      <ul className="space-y-1.5">
        {posts.map((p) => {
          const href = safeHref(p.url);
          return (
            <li key={p.mention_id} className={`text-sm rounded-lg border border-border px-3 py-2 ${p.mention_id === current ? "bg-secondary/60" : ""}`}>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span>{p.platform}</span>
                <span>·</span>
                <span>{new Date(p.posted_at).toLocaleDateString()}</span>
                {p.intent && <span>· {p.intent.replace(/_/g, " ")}</span>}
                {href && (
                  <a href={href} target="_blank" rel="noreferrer noopener" className="ml-auto hover:text-foreground hover:underline">
                    open
                  </a>
                )}
              </div>
              <p className="truncate">{p.title || p.excerpt}</p>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
