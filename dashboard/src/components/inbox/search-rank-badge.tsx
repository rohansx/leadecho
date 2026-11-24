// A lead thread that shows up in web search for one of the workspace's
// keywords (platform_metadata.search_rank, set weekly by the monitor). The
// results come from Exa, so the label says "web search", never "Google".

interface SearchRank {
  query: string;
  position: number;
}

function parseSearchRank(meta: Record<string, unknown> | null | undefined): SearchRank | null {
  const r = meta?.search_rank as Partial<SearchRank> | undefined;
  return r && typeof r.query === "string" && typeof r.position === "number" ? { query: r.query, position: r.position } : null;
}

export function SearchRankBadge({ meta }: { meta: Record<string, unknown> | null | undefined }) {
  const rank = parseSearchRank(meta);
  if (!rank) return null;
  return (
    <span
      title={`Shows up in web search for '${rank.query}' (#${rank.position})`}
      className="rounded-full border border-border px-1.5 py-px text-[10px] font-medium text-foreground-soft"
    >
      Ranks in search
    </span>
  );
}
