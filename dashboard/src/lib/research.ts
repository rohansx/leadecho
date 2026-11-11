// research: what a web search found about a lead's company, stored on the
// mention as platform_metadata.research. Every fact carries its source page.

export interface ResearchFact {
  text: string;
  url: string;
}

export interface Research {
  whatTheyDo: string;
  whyNow: string;
  hook: string;
  facts: ResearchFact[];
}

const str = (v: unknown): string => (typeof v === "string" ? v.trim() : "");

/** Reads the stored brief. Null when nothing usable is there. */
export function parseResearch(meta: unknown): Research | null {
  const raw = (meta as { research?: unknown } | null)?.research;
  if (!raw || typeof raw !== "object") return null;
  const r = raw as Record<string, unknown>;
  const facts = (Array.isArray(r.facts) ? r.facts : [])
    .map((f): ResearchFact => ({
      text: str((f as Record<string, unknown> | null)?.text),
      url: str((f as Record<string, unknown> | null)?.url),
    }))
    .filter((f) => f.text !== "" && /^https?:\/\//i.test(f.url));
  const out: Research = {
    whatTheyDo: str(r.what_they_do),
    whyNow: str(r.why_now),
    hook: str(r.hook),
    facts,
  };
  return out.whatTheyDo || out.whyNow || out.hook || facts.length > 0
    ? out
    : null;
}

/** "finance.yahoo.com" for a source link. */
export function sourceHost(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return "source";
  }
}
