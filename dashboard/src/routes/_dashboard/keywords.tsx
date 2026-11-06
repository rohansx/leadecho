import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  listKeywords,
  createKeyword,
  updateKeyword,
  deleteKeyword,
} from "@/lib/api";
import type { Keyword } from "@/lib/types";
// sources: cadence + cost preview, mute list
import {
  BulkPaidCadenceButton,
  KeywordCadence,
  MonthlyCostSummary,
  useCostPreview,
} from "@/components/sources/keyword-cadence";
import { isPaid, truncate, type Rates } from "@/lib/keyword-schedule";
import { MuteListCard } from "@/components/sources/mute-list-card";
import { Plus, Trash2, Power, PowerOff, Search, X, DollarSign } from "lucide-react";

export const Route = createFileRoute("/_dashboard/keywords")({
  component: KeywordsPage,
});

const platformOptions = ["reddit", "hackernews", "devto", "googlenews", "lobsters", "indiehackers", "twitter", "linkedin", "exa", "bluesky"] as const;
const matchTypes = ["broad", "exact", "phrase"] as const;

function KeywordsPage() {
  const queryClient = useQueryClient();
  const [newTerm, setNewTerm] = useState("");
  const [selectedPlatforms, setSelectedPlatforms] = useState<string[]>([
    ...platformOptions,
  ]);
  const [matchType, setMatchType] = useState<string>("broad");
  const [negativeTerms, setNegativeTerms] = useState("");
  const [subreddits, setSubreddits] = useState("");

  const [filter, setFilter] = useState("");

  const { data: keywords, isLoading } = useQuery({
    queryKey: ["keywords"],
    queryFn: listKeywords,
  });
  const { data: costPreview } = useCostPreview();
  const pausedCount = keywords?.filter((k) => !k.is_active).length ?? 0;
  const q = filter.trim().toLowerCase();
  const visible = (keywords ?? []).filter(
    (k) => !q || [k.term, ...k.platforms, ...(k.subreddits ?? [])].some((v) => v.toLowerCase().includes(q)),
  );

  const addMutation = useMutation({
    mutationFn: () =>
      createKeyword({
        term: newTerm,
        platforms: selectedPlatforms,
        match_type: matchType,
        negative_terms: negativeTerms
          .split(",")
          .map((t) => t.trim())
          .filter(Boolean),
        subreddits: subreddits
          .split(",")
          .map((s) => s.trim().replace(/^r\//, ""))
          .filter(Boolean),
      }),
    onSuccess: () => {
      setNewTerm("");
      setNegativeTerms("");
      setSubreddits("");
      setMatchType("broad");
      queryClient.invalidateQueries({ queryKey: ["keywords"] });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, isActive }: { id: string; isActive: boolean }) =>
      updateKeyword(id, { is_active: isActive }),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["keywords"] }),
  });

  const deleteMutation = useMutation({
    mutationFn: deleteKeyword,
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ["keywords"] }),
  });

  const togglePlatform = (p: string) => {
    setSelectedPlatforms((prev) =>
      prev.includes(p) ? prev.filter((x) => x !== p) : [...prev, p],
    );
  };

  return (
    <div className="min-w-0 space-y-6">
      <div>
        <Text as="h2">Keywords</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Terms to monitor across social platforms. The system crawls Reddit, Hacker News, Twitter, and LinkedIn for these.
        </Text>
      </div>

      <div className="grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_22rem] lg:items-start">
      {/* Keyword list */}
      <Card className="min-w-0">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
              <Search className="h-4 w-4 text-foreground-soft" />
            </div>
            <div className="min-w-0">
              <CardTitle>Keywords</CardTitle>
              <CardDescription>
                {keywords?.length ?? 0} keyword{(keywords?.length ?? 0) !== 1 ? "s" : ""} configured
                {pausedCount > 0 && ` (${pausedCount} paused)`}
                {" · "}
                <MonthlyCostSummary />
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {isLoading ? (
            <Text as="p" className="text-sm text-muted-foreground text-center py-8">
              Loading...
            </Text>
          ) : !keywords?.length ? (
            <div className="text-center py-8">
              <Search className="h-8 w-8 text-muted-foreground mx-auto mb-2 opacity-50" />
              <Text as="p" className="text-muted-foreground text-sm">
                No keywords yet. Add one to start monitoring.
              </Text>
            </div>
          ) : (
            <>
              <BulkPaidCadenceButton keywords={keywords} />
              {keywords.length > 5 && (
                <Input
                  type="search"
                  placeholder="Filter keywords, platforms or subreddits..."
                  value={filter}
                  onChange={(e) => setFilter(e.target.value)}
                  aria-label="Filter keywords"
                />
              )}
              <div className="space-y-2">
                {visible.map((kw) => (
                  <KeywordRow
                    key={kw.id}
                    kw={kw}
                    rates={costPreview?.rates ?? {}}
                    onToggle={() => toggleMutation.mutate({ id: kw.id, isActive: !kw.is_active })}
                    onDelete={() => deleteMutation.mutate(kw.id)}
                  />
                ))}
                {!visible.length && (
                  <Text as="p" className="text-sm text-muted-foreground text-center py-4">
                    No keywords match "{filter}".
                  </Text>
                )}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <div className="min-w-0 space-y-6">
      {/* Add keyword */}
      <Card className="min-w-0">
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
              <Plus className="h-4 w-4 text-foreground-soft" />
            </div>
            <div>
              <CardTitle>Add Keyword</CardTitle>
              <CardDescription>
                Configure a new term to monitor across your selected platforms.
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex min-w-0 gap-2">
            <Input
              placeholder="Enter keyword or phrase..."
              value={newTerm}
              onChange={(e) => setNewTerm(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && newTerm.trim()) addMutation.mutate();
              }}
            />
            <Button
              onClick={() => addMutation.mutate()}
              disabled={!newTerm.trim() || addMutation.isPending}
            >
              <Plus className="h-3.5 w-3.5 mr-1" />
              Add
            </Button>
          </div>

          {/* Platforms */}
          <div className="space-y-1.5">
            <Text as="p" className="text-sm font-medium">Platforms</Text>
            <div className="flex flex-wrap gap-1.5">
              {platformOptions.map((p) => (
                <button
                  key={p}
                  type="button"
                  onClick={() => togglePlatform(p)}
                  className={`px-2.5 py-1 rounded-lg text-xs border cursor-pointer transition font-medium ${
                    selectedPlatforms.includes(p)
                      ? "bg-accent-soft text-primary-ink border-primary/40"
                      : "bg-muted text-muted-foreground border-transparent hover:border-border"
                  }`}
                >
                  {p}
                </button>
              ))}
            </div>
          </div>

          {/* Match type */}
          <div className="space-y-1.5">
            <Text as="p" className="text-sm font-medium">Match Type</Text>
            <div className="flex flex-wrap gap-1.5">
              {matchTypes.map((mt) => (
                <button
                  key={mt}
                  type="button"
                  onClick={() => setMatchType(mt)}
                  className={`px-2.5 py-1 rounded-lg text-xs border cursor-pointer transition font-medium ${
                    matchType === mt
                      ? "bg-accent-soft text-primary-ink border-primary/40"
                      : "bg-muted text-muted-foreground border-transparent hover:border-border"
                  }`}
                >
                  {mt}
                </button>
              ))}
            </div>
            <Text as="p" className="text-xs text-muted-foreground">
              {matchType === "broad" && "Matches any post containing the keyword (most results)."}
              {matchType === "exact" && "Only matches the exact keyword as a standalone word."}
              {matchType === "phrase" && "Matches the exact phrase in sequence."}
            </Text>
          </div>

          {/* Negative terms */}
          <div className="space-y-1.5">
            <Text as="p" className="text-sm font-medium">Negative Terms</Text>
            <Input
              placeholder="e.g. hiring, job, salary (comma-separated)"
              value={negativeTerms}
              onChange={(e) => setNegativeTerms(e.target.value)}
            />
            <Text as="p" className="text-xs text-muted-foreground">
              Exclude posts containing these terms. Helps reduce noise.
            </Text>
          </div>

          {/* Subreddits — only when reddit is selected */}
          {selectedPlatforms.includes("reddit") && (
            <div className="space-y-1.5">
              <Text as="p" className="text-sm font-medium">Subreddits</Text>
              <Input
                placeholder="e.g. SaaS, startups, smallbusiness (comma-separated)"
                value={subreddits}
                onChange={(e) => setSubreddits(e.target.value)}
              />
              <Text as="p" className="text-xs text-muted-foreground">
                Reddit monitors specific subreddits for your keyword. Leave empty to skip Reddit.
              </Text>
            </div>
          )}
        </CardContent>
      </Card>

      <MuteListCard />
      </div>
      </div>
    </div>
  );
}

const SUBREDDITS_SHOWN = 3;

interface KeywordRowProps {
  kw: Keyword;
  rates: Rates;
  onToggle: () => void;
  onDelete: () => void;
}

function KeywordRow({ kw, rates, onToggle, onDelete }: KeywordRowProps) {
  const [showAllSubs, setShowAllSubs] = useState(false);
  const subs = truncate(kw.subreddits ?? [], SUBREDDITS_SHOWN, showAllSubs);

  return (
    <div
      className={`min-w-0 rounded-lg border border-border p-3 transition ${
        kw.is_active ? "bg-card" : "bg-muted opacity-60"
      }`}
    >
      <div className="flex items-center gap-2">
        <Text as="span" className="min-w-0 truncate text-sm font-medium" title={kw.term}>
          {kw.term}
        </Text>
        <Badge variant="outline" size="sm" className="shrink-0">
          {kw.match_type || "broad"}
        </Badge>
        {!kw.is_active && (
          <Badge size="sm" className="shrink-0">
            paused
          </Badge>
        )}
        <div className="ml-auto flex shrink-0 items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={onToggle}
            aria-label={kw.is_active ? `Pause ${kw.term}` : `Resume ${kw.term}`}
            title={kw.is_active ? "Pause" : "Resume"}
          >
            {kw.is_active ? (
              <Power className="h-3.5 w-3.5 text-green-600" />
            ) : (
              <PowerOff className="h-3.5 w-3.5 text-muted-foreground" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7"
            onClick={onDelete}
            aria-label={`Delete ${kw.term}`}
            title="Delete"
          >
            <Trash2 className="h-3.5 w-3.5 text-destructive" />
          </Button>
        </div>
      </div>

      <div className="mt-2 flex min-w-0 flex-wrap items-center gap-1">
        {kw.platforms.map((p) => (
          <Badge key={p} variant="surface" size="sm" title={isPaid(p, rates) ? "Paid data source" : undefined}>
            {p}
            {isPaid(p, rates) && <DollarSign className="h-2.5 w-2.5" aria-label="paid" />}
          </Badge>
        ))}
      </div>

      {subs.shown.length > 0 && (
        <div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-1">
          {subs.shown.map((sub) => (
            <Badge key={sub} variant="outline" size="sm">
              r/{sub}
            </Badge>
          ))}
          {(subs.hidden > 0 || showAllSubs) && (
            <button
              type="button"
              onClick={() => setShowAllSubs((v) => !v)}
              className="text-[11px] text-muted-foreground underline underline-offset-2 hover:text-foreground cursor-pointer"
            >
              {showAllSubs ? "show less" : `+${subs.hidden} more`}
            </button>
          )}
        </div>
      )}

      {kw.negative_terms?.length > 0 && (
        <div className="mt-1.5 flex min-w-0 items-start gap-1">
          <X className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground" />
          <Text as="span" className="min-w-0 break-words text-xs text-muted-foreground">
            {kw.negative_terms.join(", ")}
          </Text>
        </div>
      )}

      <KeywordCadence keyword={kw} />
    </div>
  );
}
