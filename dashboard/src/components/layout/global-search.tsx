// Global search: ⌘K / Ctrl+K from anywhere, or the header box. Finds pages,
// people (name, company, any username or email, their Person360 headline)
// and posts (their text, title or author) in the workspace.
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { FileText, Search, User, X } from "lucide-react";
import { globalSearch } from "@/lib/api";

type Hit =
  | { kind: "page"; key: string; label: string; to: string }
  | { kind: "person"; key: string; label: string; sub: string; mentionId: string }
  | { kind: "post"; key: string; label: string; sub: string; mentionId: string };

// The sidebar's pages (kept here, not imported: the sidebar file may only
// export components). Add a line when a page is added to the nav.
const PAGES: { to: string; label: string }[] = [
  { to: "/inbox", label: "Inbox" },
  { to: "/pipeline", label: "Pipeline" },
  { to: "/analytics", label: "Analytics" },
  { to: "/keywords", label: "Keywords" },
  { to: "/profiles", label: "Profiles" },
  { to: "/knowledge-base", label: "Knowledge Base" },
  { to: "/settings", label: "Settings" },
  { to: "/demand", label: "Demand" },
  { to: "/competitors", label: "Competitors" },
  { to: "/chat", label: "Ask LeadEcho" },
  { to: "/workflows", label: "Workflows" },
  { to: "/engagers", label: "LinkedIn engagers" },
  { to: "/alerts", label: "Alerts" },
];

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

export function GlobalSearch() {
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [active, setActive] = useState(0);
  const navigate = useNavigate();
  const q = useDebounced(text.trim(), 200);

  // Opening always starts clean; state resets here, in the event, not in an effect.
  const show = () => {
    setText("");
    setActive(0);
    setOpen(true);
  };
  const hide = () => setOpen(false);

  // ⌘K / Ctrl+K toggles it from anywhere (clearing the text either way);
  // Escape closes.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setText("");
        setActive(0);
        setOpen((o) => !o);
      } else if (e.key === "Escape") {
        setOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const { data, isFetching, isError } = useQuery({
    queryKey: ["global-search", q],
    queryFn: () => globalSearch(q),
    enabled: open && q.length >= 2,
    staleTime: 30_000,
  });

  const hits: Hit[] = useMemo(() => {
    const lower = q.toLowerCase();
    const pages: Hit[] = lower
      ? PAGES.filter((p) => p.label.toLowerCase().includes(lower)).map((p) => ({ kind: "page", key: `page:${p.to}`, label: p.label, to: p.to }))
      : [];
    const people: Hit[] = (data?.people ?? [])
      .filter((p) => p.mention_id)
      .map((p) => ({
        kind: "person",
        key: `person:${p.person_id}`,
        label: p.name || p.handles || "Unknown",
        sub: [p.headline, p.company].filter(Boolean).join(" · ") || p.handles,
        mentionId: p.mention_id,
      }));
    const posts: Hit[] = (data?.posts ?? []).map((p) => ({
      kind: "post",
      key: `post:${p.mention_id}`,
      label: p.title || p.excerpt,
      sub: [p.platform, p.author && `@${p.author}`, new Date(p.posted_at).toLocaleDateString()].filter(Boolean).join(" · "),
      mentionId: p.mention_id,
    }));
    return [...pages, ...people, ...posts];
  }, [data, q]);

  const go = (h: Hit) => {
    hide();
    if (h.kind === "page") void navigate({ to: h.to });
    else if (h.kind === "person") void navigate({ to: "/inbox", search: { id: h.mentionId, tab: "person360" } });
    else void navigate({ to: "/inbox", search: { id: h.mentionId } });
  };

  const onInputKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => Math.min(a + 1, hits.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(a - 1, 0));
    } else if (e.key === "Enter" && hits[active]) {
      e.preventDefault();
      go(hits[active]);
    }
  };

  const isMac = typeof navigator !== "undefined" && /Mac|iPhone|iPad/.test(navigator.platform);

  return (
    <>
      <button
        type="button"
        onClick={show}
        className="flex h-8 w-full max-w-sm cursor-pointer items-center gap-2 rounded-lg border border-border bg-background/60 px-2.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        aria-label="Search"
      >
        <Search className="h-4 w-4 shrink-0" />
        <span className="flex-1 truncate text-left">Search people, posts, pages…</span>
        <kbd className="hidden rounded border border-border px-1.5 text-[10px] sm:inline">{isMac ? "⌘K" : "Ctrl K"}</kbd>
      </button>

      {open && (
        <div className="fixed inset-0 z-50 flex items-start justify-center bg-black/30 px-4 pt-[12vh] backdrop-blur-sm" onMouseDown={hide}>
          <div
            role="dialog"
            aria-label="Search"
            className="w-full max-w-xl overflow-hidden rounded-xl border border-border bg-card shadow-2xl"
            onMouseDown={(e) => e.stopPropagation()}
          >
            <div className="flex items-center gap-2 border-b border-border px-3">
              <Search className="h-4 w-4 text-muted-foreground" />
              <input
                autoFocus
                value={text}
                onChange={(e) => {
                  setText(e.target.value);
                  setActive(0);
                }}
                onKeyDown={onInputKey}
                maxLength={100}
                placeholder="Search a name, company, username, email or any text…"
                className="h-12 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
                aria-controls="global-search-results"
              />
              {text && (
                <button type="button" onClick={() => setText("")} aria-label="Clear" className="text-muted-foreground hover:text-foreground">
                  <X className="h-4 w-4" />
                </button>
              )}
            </div>
            <ul id="global-search-results" role="listbox" className="max-h-[60vh] overflow-y-auto p-1.5">
              {q.length < 2 && <li className="px-3 py-6 text-center text-sm text-muted-foreground">Type at least 2 characters.</li>}
              {q.length >= 2 && isError && <li className="px-3 py-6 text-center text-sm text-muted-foreground">Search failed. Try again.</li>}
              {q.length >= 2 && !isError && !isFetching && hits.length === 0 && (
                <li className="px-3 py-6 text-center text-sm text-muted-foreground">Nothing found for “{q}”.</li>
              )}
              {hits.map((h, i) => (
                <li
                  key={h.key}
                  role="option"
                  aria-selected={i === active}
                  onMouseEnter={() => setActive(i)}
                  onClick={() => go(h)}
                  className={`flex cursor-pointer items-start gap-2.5 rounded-lg px-2.5 py-2 ${i === active ? "bg-accent" : ""}`}
                >
                  <span className="mt-0.5 text-muted-foreground">
                    {h.kind === "person" ? <User className="h-4 w-4" /> : h.kind === "post" ? <FileText className="h-4 w-4" /> : <Search className="h-4 w-4" />}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm">{h.label}</span>
                    {"sub" in h && h.sub && <span className="block truncate text-xs text-muted-foreground">{h.sub}</span>}
                  </span>
                  <span className="shrink-0 text-[11px] text-muted-foreground">
                    {h.kind === "person" ? "Person360" : h.kind === "post" ? "Post" : "Page"}
                  </span>
                </li>
              ))}
              {isFetching && <li className="px-3 py-2 text-xs text-muted-foreground">Searching…</li>}
            </ul>
          </div>
        </div>
      )}
    </>
  );
}
