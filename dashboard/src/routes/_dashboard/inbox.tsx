import { createFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Card,
  CardHeader,
  CardTitle,
  CardContent,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import {
  Archive,
  Sparkles,
  ExternalLink,
  RefreshCw,
  Brain,
  Loader2,
  Copy,
  Check,
} from "lucide-react";
import {
  listMentions,
  updateMentionStatus,
  mentionCounts,
  mentionTierCounts,
  classifyMention,
  draftReply,
} from "@/lib/api";
import type { Mention, Reply } from "@/lib/types";
import { useState } from "react";

export const Route = createFileRoute("/_dashboard/inbox")({
  component: InboxPage,
});

const intentColors: Record<string, string> = {
  buy_signal: "bg-green-200 text-green-900",
  recommendation_ask: "bg-blue-200 text-blue-900",
  comparison: "bg-purple-200 text-purple-900",
  complaint: "bg-orange-200 text-orange-900",
  general: "bg-muted text-muted-foreground",
};

const platformColors: Record<string, string> = {
  reddit: "bg-orange-100 text-orange-800",
  hackernews: "bg-amber-100 text-amber-800",
  twitter: "bg-sky-100 text-sky-800",
  linkedin: "bg-blue-100 text-blue-800",
  devto: "bg-indigo-100 text-indigo-800",
  lobsters: "bg-red-100 text-red-800",
  indiehackers: "bg-teal-100 text-teal-800",
};

const tierTabs = [
  { key: "", label: "All", color: "" },
  { key: "leads_ready", label: "Leads Ready", color: "bg-green-500" },
  { key: "worth_watching", label: "Worth Watching", color: "bg-yellow-500" },
  { key: "filtered", label: "Filtered", color: "bg-gray-400" },
] as const;

function InboxPage() {
  const queryClient = useQueryClient();
  const [tierFilter, setTierFilter] = useState<string>("");
  const [statusFilter, setStatusFilter] = useState<string>("");
  const [platformFilter, setPlatformFilter] = useState<string>("");
  const [search, setSearch] = useState("");
  const [expandedDraft, setExpandedDraft] = useState<string | null>(null);
  const [draftContent, setDraftContent] = useState<Record<string, { reply: Reply; tone: string }>>({});
  const [copied, setCopied] = useState<string | null>(null);

  const { data, isLoading, refetch } = useQuery({
    queryKey: ["mentions", tierFilter, statusFilter, platformFilter, search],
    queryFn: () =>
      listMentions({
        tier: tierFilter || undefined,
        status: statusFilter || undefined,
        platform: platformFilter || undefined,
        search: search || undefined,
        limit: 20,
      }),
  });

  const { data: counts } = useQuery({
    queryKey: ["mentionCounts"],
    queryFn: mentionCounts,
  });

  const { data: tierCounts } = useQuery({
    queryKey: ["mentionTierCounts"],
    queryFn: mentionTierCounts,
  });

  const archiveMutation = useMutation({
    mutationFn: (id: string) => updateMentionStatus(id, "archived"),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mentions"] });
      queryClient.invalidateQueries({ queryKey: ["mentionCounts"] });
      queryClient.invalidateQueries({ queryKey: ["mentionTierCounts"] });
    },
  });

  const classifyMutation = useMutation({
    mutationFn: (id: string) => classifyMention(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["mentions"] });
      queryClient.invalidateQueries({ queryKey: ["mentionTierCounts"] });
    },
  });

  const draftMutation = useMutation({
    mutationFn: (id: string) => draftReply(id),
    onSuccess: (data, id) => {
      setDraftContent((prev) => ({ ...prev, [id]: data }));
      setExpandedDraft(id);
      queryClient.invalidateQueries({ queryKey: ["mentions"] });
    },
  });

  const mentions = data?.data ?? [];
  const newCount = counts?.find((c) => c.status === "new")?.count ?? 0;

  const getTierCount = (tier: string) => {
    if (!tier) return null;
    return tierCounts?.find((c) => c.tier === tier)?.count ?? 0;
  };

  const isAutoScored = (mention: Mention) =>
    mention.scoring_metadata && (mention.scoring_metadata as Record<string, unknown>).auto_scored === true;

  const isLeadsReady = (mention: Mention) =>
    mention.relevance_score != null &&
    mention.relevance_score >= 7.0 &&
    mention.intent &&
    ["buy_signal", "recommendation_ask", "complaint"].includes(mention.intent);

  const handleCopy = (mentionId: string, text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(mentionId);
    setTimeout(() => setCopied(null), 2000);
  };

  return (
    <div className="space-y-6 max-w-4xl">
      {/* Header */}
      <div className="flex items-center justify-between">
        <Text as="h2">Smart Inbox</Text>
        <div className="flex gap-2">
          <Badge variant="surface">{newCount} new</Badge>
          <Button size="sm" variant="outline" onClick={() => refetch()}>
            <RefreshCw className="h-3.5 w-3.5 mr-1.5" />
            Refresh
          </Button>
        </div>
      </div>

      {/* Tier Tabs */}
      <div className="flex gap-1.5">
        {tierTabs.map((tab) => {
          const count = getTierCount(tab.key);
          return (
            <button
              key={tab.key}
              type="button"
              onClick={() => setTierFilter(tab.key)}
              className={`px-3 py-1.5 rounded text-sm border-2 cursor-pointer transition font-medium flex items-center gap-2 ${
                tierFilter === tab.key
                  ? "bg-primary text-primary-foreground border-border shadow-xs"
                  : "bg-muted text-muted-foreground border-transparent hover:border-border"
              }`}
            >
              {tab.color && (
                <span className={`w-2 h-2 rounded-full ${tab.color}`} />
              )}
              {tab.label}
              {count != null && (
                <span className="text-xs opacity-75">({count})</span>
              )}
            </button>
          );
        })}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <Input
          placeholder="Search mentions..."
          className="max-w-sm"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") refetch();
          }}
        />
        <select
          className="border-2 border-border rounded px-3 py-1.5 text-sm bg-background font-[family-name:var(--font-sans)] shadow-xs"
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
        >
          <option value="">All statuses</option>
          <option value="new">New</option>
          <option value="reviewed">Reviewed</option>
          <option value="replied">Replied</option>
          <option value="archived">Archived</option>
        </select>
        <select
          className="border-2 border-border rounded px-3 py-1.5 text-sm bg-background font-[family-name:var(--font-sans)] shadow-xs"
          value={platformFilter}
          onChange={(e) => setPlatformFilter(e.target.value)}
        >
          <option value="">All platforms</option>
          <option value="reddit">Reddit</option>
          <option value="hackernews">Hacker News</option>
          <option value="devto">Dev.to</option>
          <option value="lobsters">Lobsters</option>
          <option value="indiehackers">IndieHackers</option>
          <option value="twitter">Twitter</option>
          <option value="linkedin">LinkedIn</option>
        </select>
      </div>

      {/* Loading state */}
      {isLoading && (
        <Card className="w-full">
          <CardContent className="p-8 text-center">
            <Text as="p" className="text-muted-foreground">
              Loading mentions...
            </Text>
          </CardContent>
        </Card>
      )}

      {/* Empty state */}
      {!isLoading && mentions.length === 0 && (
        <Card className="w-full">
          <CardContent className="p-8 text-center">
            <Text as="p" className="text-muted-foreground">
              No mentions found. Adjust your filters or check back later.
            </Text>
          </CardContent>
        </Card>
      )}

      {/* Mention Cards */}
      <div className="grid gap-4">
        {mentions.map((mention: Mention) => (
          <Card
            key={mention.id}
            className={`w-full hover:shadow-sm ${isLeadsReady(mention) ? "border-l-4 border-l-green-500" : ""}`}
          >
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <Badge
                    className={`${platformColors[mention.platform] ?? ""} border border-border`}
                    size="sm"
                  >
                    {mention.platform}
                  </Badge>
                  {!!(
                    mention.platform_metadata &&
                    (mention.platform_metadata as Record<string, unknown>)
                      .subreddit
                  ) && (
                    <Badge variant="outline" size="sm">
                      {String(
                        (
                          mention.platform_metadata as Record<string, string>
                        ).subreddit,
                      )}
                    </Badge>
                  )}
                  {isAutoScored(mention) && (
                    <Badge variant="outline" size="sm" className="text-xs border-green-300 text-green-700">
                      Auto-scored
                    </Badge>
                  )}
                  <span className="font-[family-name:var(--font-sans)] text-sm text-muted-foreground">
                    @{mention.author_username ?? "unknown"} &middot;{" "}
                    {timeAgo(mention.platform_created_at ?? mention.created_at)}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  {mention.intent && (
                    <span
                      className={`px-2 py-0.5 rounded text-xs font-medium ${intentColors[mention.intent] ?? ""}`}
                    >
                      {mention.intent.replace("_", " ")}
                    </span>
                  )}
                  {mention.relevance_score != null && (
                    <Badge variant="surface" size="sm">
                      {mention.relevance_score.toFixed(1)}/10
                    </Badge>
                  )}
                </div>
              </div>
              {mention.title && (
                <CardTitle className="text-lg mt-2">
                  {mention.title}
                </CardTitle>
              )}
            </CardHeader>
            <CardContent className="pt-0">
              <Text
                as="p"
                className="text-muted-foreground text-sm leading-relaxed"
              >
                {mention.content}
              </Text>

              {/* Action buttons */}
              <div className="flex gap-2 mt-4">
                {!mention.intent && (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => classifyMutation.mutate(mention.id)}
                    disabled={classifyMutation.isPending}
                  >
                    {classifyMutation.isPending && classifyMutation.variables === mention.id ? (
                      <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                    ) : (
                      <Brain className="h-3.5 w-3.5 mr-1.5" />
                    )}
                    Classify
                  </Button>
                )}
                <Button
                  size="sm"
                  onClick={() => draftMutation.mutate(mention.id)}
                  disabled={draftMutation.isPending}
                >
                  {draftMutation.isPending && draftMutation.variables === mention.id ? (
                    <Loader2 className="h-3.5 w-3.5 mr-1.5 animate-spin" />
                  ) : (
                    <Sparkles className="h-3.5 w-3.5 mr-1.5" />
                  )}
                  Draft Reply
                </Button>
                <a
                  href={mention.url}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  <Button size="sm" variant="ghost">
                    <ExternalLink className="h-3.5 w-3.5 mr-1.5" />
                    Open
                  </Button>
                </a>
                <Button
                  size="sm"
                  variant="ghost"
                  className="ml-auto"
                  onClick={() => archiveMutation.mutate(mention.id)}
                  disabled={archiveMutation.isPending}
                >
                  <Archive className="h-3.5 w-3.5 mr-1.5" />
                  Archive
                </Button>
              </div>

              {/* Draft reply panel */}
              {expandedDraft === mention.id && draftContent[mention.id] && (
                <div className="mt-4 p-4 rounded-lg border-2 border-border bg-muted/30">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      <Text as="p" className="font-medium text-sm">
                        AI Draft
                      </Text>
                      <Badge variant="outline" size="sm">
                        {draftContent[mention.id].tone}
                      </Badge>
                    </div>
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() =>
                        handleCopy(
                          mention.id,
                          draftContent[mention.id].reply.content,
                        )
                      }
                    >
                      {copied === mention.id ? (
                        <Check className="h-3.5 w-3.5 mr-1.5" />
                      ) : (
                        <Copy className="h-3.5 w-3.5 mr-1.5" />
                      )}
                      {copied === mention.id ? "Copied" : "Copy"}
                    </Button>
                  </div>
                  <Text
                    as="p"
                    className="text-sm leading-relaxed whitespace-pre-wrap"
                  >
                    {draftContent[mention.id].reply.content}
                  </Text>
                </div>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const hours = Math.floor(diff / (1000 * 60 * 60));
  if (hours < 1) return "just now";
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}
