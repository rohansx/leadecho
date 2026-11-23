// identity: who is behind a forum handle (persons.metadata.identity).
// Every link names the page it was found on, so a reader can check it.
import { ExternalLink } from "lucide-react";

interface IdentityLink {
  kind: string;
  url: string;
  handle?: string;
  confidence: number;
  evidence?: string;
  method?: string;
}

interface IdentityPost {
  title: string;
  url?: string;
}

interface IdentityProfile {
  name?: string;
  role?: string;
  company?: string;
  summary?: string;
  topics?: string[];
  links?: IdentityLink[];
  recent_posts?: IdentityPost[];
  tier?: string;
}

const SHOWN = 0.8;

const KIND_LABEL: Record<string, string> = {
  email: "Email",
  booking: "Book a call",
  linkedin: "LinkedIn",
  website: "Website",
  github: "GitHub",
  x: "X",
  bluesky: "Bluesky",
  mastodon: "Mastodon",
  youtube: "YouTube",
  devto: "Dev.to",
  hackernews: "Hacker News",
  reddit: "Reddit",
};

// Contact routes first, then the rest.
const KIND_ORDER = ["email", "booking", "linkedin", "website", "x", "github", "bluesky", "mastodon", "youtube", "devto", "hackernews", "reddit"];

/** Only http(s) and mailto links are rendered: the data comes from public pages. */
function safeHref(u: string | undefined): string | undefined {
  if (!u) return undefined;
  try {
    const p = new URL(u);
    return p.protocol === "https:" || p.protocol === "http:" || p.protocol === "mailto:" ? p.toString() : undefined;
  } catch {
    return undefined;
  }
}

function hostOf(u: string | undefined): string {
  try {
    return u ? new URL(u).hostname.replace(/^www\./, "") : "";
  } catch {
    return "";
  }
}

function readIdentity(meta: Record<string, unknown> | undefined): IdentityProfile | null {
  const v = meta?.identity;
  return v && typeof v === "object" ? (v as IdentityProfile) : null;
}

function byKind(a: IdentityLink, b: IdentityLink): number {
  const ia = KIND_ORDER.indexOf(a.kind);
  const ib = KIND_ORDER.indexOf(b.kind);
  return (ia < 0 ? 99 : ia) - (ib < 0 ? 99 : ib) || b.confidence - a.confidence;
}

function LinkRow({ link, muted }: { link: IdentityLink; muted?: boolean }) {
  const href = safeHref(link.url);
  const evidence = safeHref(link.evidence);
  const text = link.kind === "email" ? link.handle : link.url.replace(/^https?:\/\/(www\.)?/, "");
  return (
    <div className={`flex items-center justify-between gap-3 text-sm ${muted ? "text-muted-foreground" : ""}`}>
      <div className="min-w-0 flex items-baseline gap-2">
        <span className="shrink-0 text-xs text-muted-foreground w-20">{KIND_LABEL[link.kind] ?? link.kind}</span>
        {href ? (
          <a href={href} target="_blank" rel="noreferrer noopener" className="truncate underline-offset-2 hover:underline">
            {text}
          </a>
        ) : (
          <span className="truncate">{text}</span>
        )}
      </div>
      {evidence && (
        <a href={evidence} target="_blank" rel="noreferrer noopener" className="shrink-0 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground" title="Where this was found">
          {hostOf(evidence)} <ExternalLink className="h-3 w-3" />
        </a>
      )}
    </div>
  );
}

export function IdentityCard({ metadata }: { metadata?: Record<string, unknown> }) {
  const p = readIdentity(metadata);
  if (!p) return null;
  const links = (p.links ?? []).filter((l) => l.method !== "origin");
  const shown = links.filter((l) => l.confidence >= SHOWN).sort(byKind);
  const possible = links.filter((l) => l.confidence >= 0.5 && l.confidence < SHOWN).sort(byKind);
  const headline = [p.role, p.company].filter(Boolean).join(" at ");
  const empty = shown.length === 0 && !p.summary && !headline;

  if (empty && possible.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border p-3 text-xs text-muted-foreground">
        {p.tier === "b"
          ? "Researched: this author keeps no public profile that links to who they are."
          : "Their public profiles link nowhere else yet. Research runs next for leads worth it."}
      </div>
    );
  }

  return (
    <div className="rounded-xl border border-border bg-card p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Who they are</h5>
        <span className="text-[11px] text-muted-foreground">{p.tier === "b" ? "researched" : "from public profiles"}</span>
      </div>
      {headline && <p className="text-sm font-medium">{headline}</p>}
      {p.summary && <p className="text-sm leading-relaxed text-foreground-soft">{p.summary}</p>}
      {p.topics && p.topics.length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {p.topics.map((t) => (
            <span key={t} className="text-xs rounded-full bg-secondary px-2 py-0.5">
              {t}
            </span>
          ))}
        </div>
      )}
      {shown.length > 0 && (
        <div className="space-y-1.5">
          {shown.map((l) => (
            <LinkRow key={l.url} link={l} />
          ))}
        </div>
      )}
      {possible.length > 0 && (
        <details className="text-sm">
          <summary className="cursor-pointer text-xs text-muted-foreground">
            {possible.length} possible, not confirmed
          </summary>
          <div className="mt-2 space-y-1.5">
            {possible.map((l) => (
              <LinkRow key={l.url} link={l} muted />
            ))}
          </div>
        </details>
      )}
      {p.recent_posts && p.recent_posts.length > 0 && (
        <details className="text-sm">
          <summary className="cursor-pointer text-xs text-muted-foreground">What they post</summary>
          <ul className="mt-2 space-y-1">
            {p.recent_posts.slice(0, 8).map((post) => {
              const href = safeHref(post.url);
              return (
                <li key={post.title + (post.url ?? "")} className="truncate">
                  {href ? (
                    <a href={href} target="_blank" rel="noreferrer noopener" className="hover:underline">
                      {post.title}
                    </a>
                  ) : (
                    post.title
                  )}
                </li>
              );
            })}
          </ul>
        </details>
      )}
    </div>
  );
}
