import { motion } from "motion/react";
import { RefreshCw, Search } from "lucide-react";
import type { Mention, TierCount } from "@/lib/types";

const tierTabs = [
  { key: "", label: "All" },
  { key: "leads_ready", label: "Leads ready" },
  { key: "worth_watching", label: "Worth watching" },
  { key: "filtered", label: "Filtered" },
] as const;

const intentColors: Record<string, string> = {
  buy_signal: "text-primary-ink",
  recommendation_ask: "text-blue-600 dark:text-blue-400",
  comparison: "text-purple-600 dark:text-purple-400",
  complaint: "text-orange-600 dark:text-orange-400",
  general: "text-muted-foreground",
};

function scoreClass(score: number | null) {
  if (score == null) return "bg-muted text-muted-foreground";
  if (score >= 7) return "bg-primary/15 text-primary-ink";
  if (score >= 4) return "bg-blue-500/10 text-blue-700 dark:text-blue-400";
  return "bg-muted text-muted-foreground";
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / (1000 * 60));
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h`;
  return `${Math.floor(hours / 24)}d`;
}

export function LeadList({
  mentions,
  isLoading,
  selectedId,
  onSelect,
  tierFilter,
  onTierChange,
  tierCounts,
  platformFilter,
  onPlatformChange,
  platformOptions,
  search,
  onSearchChange,
  onRefresh,
}: {
  mentions: Mention[];
  isLoading: boolean;
  selectedId: string | null;
  onSelect: (id: string) => void;
  tierFilter: string;
  onTierChange: (tier: string) => void;
  tierCounts?: TierCount[];
  platformFilter: string;
  onPlatformChange: (platform: string) => void;
  platformOptions: { platform: string; count: number }[];
  search: string;
  onSearchChange: (v: string) => void;
  onRefresh: () => void;
}) {
  const getTierCount = (tier: string) =>
    tier ? (tierCounts?.find((c) => c.tier === tier)?.count ?? 0) : null;

  return (
    <section className="w-[380px] shrink-0 border-r border-border flex flex-col h-full bg-background">
      <div className="p-4 border-b border-border space-y-3">
        <div className="flex items-center justify-between">
          <h2 className="font-[family-name:var(--font-head)] font-medium">Inbox</h2>
          <button
            onClick={onRefresh}
            className="w-7 h-7 rounded-md border border-border hover:bg-accent flex items-center justify-center cursor-pointer"
            aria-label="Refresh"
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </button>
        </div>

        <div className="relative">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
          <input
            value={search}
            onChange={(e) => onSearchChange(e.target.value)}
            placeholder="Search this inbox…"
            className="w-full rounded-lg border border-border bg-card pl-8 pr-3 py-1.5 text-sm focus:outline-hidden focus:ring-2 focus:ring-ring/30"
          />
        </div>

        <div className="flex gap-1.5 flex-wrap">
          {tierTabs.map((t) => {
            const count = getTierCount(t.key);
            const active = tierFilter === t.key;
            return (
              <button
                key={t.key}
                onClick={() => onTierChange(t.key)}
                className={`px-2.5 py-1 rounded-full text-xs font-medium cursor-pointer transition-colors ${
                  active
                    ? "bg-primary text-primary-foreground"
                    : "bg-muted text-muted-foreground hover:bg-accent"
                }`}
              >
                {t.label}
                {count != null && <span className="ml-1 opacity-70">{count}</span>}
              </button>
            );
          })}
        </div>

        {platformOptions.length > 0 && (
          <select
            value={platformFilter}
            onChange={(e) => onPlatformChange(e.target.value)}
            className="w-full rounded-lg border border-border bg-card px-2.5 py-1.5 text-xs font-[family-name:var(--font-sans)]"
          >
            <option value="">All platforms</option>
            {platformOptions.map((p) => (
              <option key={p.platform} value={p.platform}>
                {p.platform} ({p.count})
              </option>
            ))}
          </select>
        )}
      </div>

      <div className="flex-1 overflow-y-auto">
        {isLoading && (
          <div className="p-6 text-center text-sm text-muted-foreground">Loading…</div>
        )}
        {!isLoading && mentions.length === 0 && (
          <div className="p-6 text-center text-sm text-muted-foreground">
            No mentions found. Adjust your filters.
          </div>
        )}
        {mentions.map((m, i) => (
          <motion.button
            key={m.id}
            onClick={() => onSelect(m.id)}
            initial={{ opacity: 0, y: 8 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.25, delay: Math.min(i, 8) * 0.03 }}
            className={`w-full text-left px-4 py-3.5 border-b border-border flex gap-3 transition-colors cursor-pointer ${
              selectedId === m.id ? "bg-accent-soft/60" : "hover:bg-accent/50"
            }`}
          >
            <div
              className={`shrink-0 h-9 w-9 rounded-lg flex flex-col items-center justify-center text-[13px] font-[family-name:var(--font-head)] font-medium ${scoreClass(m.relevance_score)}`}
            >
              {m.relevance_score != null ? m.relevance_score.toFixed(1) : "–"}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-1">
                <span className="font-medium text-foreground-soft">{m.platform}</span>
                {m.intent && (
                  <>
                    <span>·</span>
                    <span className={intentColors[m.intent] ?? ""}>{m.intent.replace("_", " ")}</span>
                  </>
                )}
                <span className="ml-auto shrink-0">{timeAgo(m.platform_created_at ?? m.created_at)}</span>
              </div>
              <p className="text-sm text-foreground line-clamp-2 leading-snug">{m.content}</p>
              <div className="mt-1 text-xs text-muted-foreground truncate">
                @{m.author_username ?? "unknown"}
              </div>
            </div>
          </motion.button>
        ))}
      </div>
    </section>
  );
}
