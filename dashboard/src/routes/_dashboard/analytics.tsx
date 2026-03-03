import { createFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Text } from "@/components/ui/text";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  analyticsOverview,
  mentionsPerPlatform,
  mentionsPerIntent,
  conversionFunnel,
  topKeywords,
  listUTMLinks,
  createUTMLink,
  deleteUTMLink,
} from "@/lib/api";
import {
  TrendingUp,
  MessageSquare,
  Users,
  Zap,
  Send,
  Target,
  Link2,
  Copy,
  Trash2,
  Plus,
} from "lucide-react";

export const Route = createFileRoute("/_dashboard/analytics")({
  component: AnalyticsPage,
});

const platformColors: Record<string, string> = {
  reddit: "bg-orange-100 text-orange-800",
  hackernews: "bg-amber-100 text-amber-800",
  twitter: "bg-sky-100 text-sky-800",
  linkedin: "bg-blue-100 text-blue-800",
};

const intentLabels: Record<string, string> = {
  buy_signal: "Buy Signal",
  complaint: "Complaint",
  recommendation_ask: "Recommendation",
  comparison: "Comparison",
  general: "General",
};

const stageColors: Record<string, string> = {
  prospect: "bg-slate-200 text-slate-800",
  qualified: "bg-blue-200 text-blue-800",
  engaged: "bg-purple-200 text-purple-800",
  converted: "bg-green-200 text-green-800",
  lost: "bg-red-200 text-red-800",
};

function AnalyticsPage() {
  const queryClient = useQueryClient();
  const { data: overview } = useQuery({
    queryKey: ["analytics-overview"],
    queryFn: analyticsOverview,
  });
  const { data: platforms } = useQuery({
    queryKey: ["analytics-platforms"],
    queryFn: mentionsPerPlatform,
  });
  const { data: intents } = useQuery({
    queryKey: ["analytics-intents"],
    queryFn: mentionsPerIntent,
  });
  const { data: funnel } = useQuery({
    queryKey: ["analytics-funnel"],
    queryFn: conversionFunnel,
  });
  const { data: keywords } = useQuery({
    queryKey: ["analytics-keywords"],
    queryFn: topKeywords,
  });
  const { data: utmLinks = [] } = useQuery({
    queryKey: ["utm-links"],
    queryFn: listUTMLinks,
  });

  const [showNewLink, setShowNewLink] = useState(false);
  const [newLink, setNewLink] = useState({ destination_url: "", utm_source: "", utm_campaign: "" });

  const createMutation = useMutation({
    mutationFn: createUTMLink,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["utm-links"] });
      setShowNewLink(false);
      setNewLink({ destination_url: "", utm_source: "", utm_campaign: "" });
    },
  });
  const deleteMutation = useMutation({
    mutationFn: deleteUTMLink,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["utm-links"] }),
  });

  const stats = [
    { label: "Mentions (30d)", value: overview?.mentions_30d ?? 0, icon: MessageSquare, color: "text-blue-600" },
    { label: "New / Unread", value: overview?.mentions_new ?? 0, icon: Zap, color: "text-amber-600" },
    { label: "Total Leads", value: overview?.total_leads ?? 0, icon: Users, color: "text-purple-600" },
    { label: "Converted", value: overview?.converted_leads ?? 0, icon: TrendingUp, color: "text-green-600" },
    { label: "Replies Posted", value: overview?.replies_posted ?? 0, icon: Send, color: "text-indigo-600" },
    { label: "Active Keywords", value: overview?.active_keywords ?? 0, icon: Target, color: "text-rose-600" },
  ];

  const conversionRate =
    overview && overview.total_leads > 0
      ? ((overview.converted_leads / overview.total_leads) * 100).toFixed(1)
      : "0";

  return (
    <div className="space-y-6 max-w-5xl">
      <Text as="h2">Analytics</Text>

      {/* KPI Cards */}
      <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3">
        {stats.map((s) => {
          const Icon = s.icon;
          return (
            <Card key={s.label}>
              <CardContent className="p-4 text-center">
                <Icon className={`h-5 w-5 mx-auto mb-1 ${s.color}`} />
                <Text as="p" className="text-2xl font-[family-name:var(--font-head)]">{s.value}</Text>
                <Text as="p" className="text-xs text-muted-foreground">{s.label}</Text>
              </CardContent>
            </Card>
          );
        })}
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        {/* Platform Breakdown */}
        <Card>
          <CardHeader><CardTitle>Mentions by Platform</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            {!(platforms ?? []).length && <Text as="p" className="text-sm text-muted-foreground">No data yet.</Text>}
            {(platforms ?? []).map((p) => {
              const total = (platforms ?? []).reduce((a, x) => a + x.count, 0);
              const pct = total > 0 ? (p.count / total) * 100 : 0;
              return (
                <div key={p.platform} className="space-y-1">
                  <div className="flex items-center justify-between">
                    <Badge className={`${platformColors[p.platform] ?? ""} border border-border`} size="sm">{p.platform}</Badge>
                    <Text as="span" className="text-sm font-medium">{p.count}</Text>
                  </div>
                  <div className="h-2 bg-muted rounded-full overflow-hidden border border-border">
                    <div className="h-full bg-primary rounded-full" style={{ width: `${pct}%` }} />
                  </div>
                </div>
              );
            })}
          </CardContent>
        </Card>

        {/* Intent Breakdown */}
        <Card>
          <CardHeader><CardTitle>Mentions by Intent</CardTitle></CardHeader>
          <CardContent className="space-y-3">
            {!(intents ?? []).length && <Text as="p" className="text-sm text-muted-foreground">No data yet.</Text>}
            {(intents ?? []).map((item) => {
              const total = (intents ?? []).reduce((a, x) => a + x.count, 0);
              const pct = total > 0 ? (item.count / total) * 100 : 0;
              return (
                <div key={item.intent} className="space-y-1">
                  <div className="flex items-center justify-between">
                    <Text as="span" className="text-sm">{intentLabels[item.intent] ?? item.intent}</Text>
                    <Text as="span" className="text-sm font-medium">{item.count}</Text>
                  </div>
                  <div className="h-2 bg-muted rounded-full overflow-hidden border border-border">
                    <div className="h-full bg-green-500 rounded-full" style={{ width: `${pct}%` }} />
                  </div>
                </div>
              );
            })}
          </CardContent>
        </Card>

        {/* Conversion Funnel */}
        <Card>
          <CardHeader>
            <CardTitle>Conversion Funnel <Badge variant="surface" size="sm" className="ml-2">{conversionRate}% rate</Badge></CardTitle>
          </CardHeader>
          <CardContent className="space-y-2">
            {!(funnel ?? []).length && <Text as="p" className="text-sm text-muted-foreground">No leads yet.</Text>}
            {["prospect", "qualified", "engaged", "converted", "lost"].map((stage) => {
              const item = (funnel ?? []).find((f) => f.stage === stage);
              const count = item?.count ?? 0;
              const max = Math.max(...(funnel ?? []).map((f) => f.count), 1);
              const pct = (count / max) * 100;
              return (
                <div key={stage} className="flex items-center gap-3">
                  <Badge className={`${stageColors[stage] ?? ""} border border-border w-24 justify-center`} size="sm">{stage}</Badge>
                  <div className="flex-1 h-6 bg-muted rounded border border-border overflow-hidden">
                    <div className="h-full bg-primary rounded flex items-center justify-end pr-2" style={{ width: `${Math.max(pct, 8)}%` }}>
                      <span className="text-xs font-medium text-primary-foreground">{count}</span>
                    </div>
                  </div>
                </div>
              );
            })}
          </CardContent>
        </Card>

        {/* Top Keywords */}
        <Card>
          <CardHeader><CardTitle>Top Keywords</CardTitle></CardHeader>
          <CardContent>
            {!(keywords ?? []).length && <Text as="p" className="text-sm text-muted-foreground">No keywords tracked yet.</Text>}
            <div className="space-y-2">
              {(keywords ?? []).map((kw, i) => (
                <div key={kw.term} className="flex items-center justify-between py-1">
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground w-5">{i + 1}.</span>
                    <Text as="span" className="text-sm font-medium">{kw.term}</Text>
                  </div>
                  <Badge variant="outline" size="sm">{kw.mention_count} mentions</Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* UTM Tracking Links */}
      <Card id="utm-links">
        <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
          <CardTitle className="flex items-center gap-2">
            <Link2 className="h-4 w-4" /> UTM Tracking Links
          </CardTitle>
          <Button size="sm" variant="outline" onClick={() => setShowNewLink((v) => !v)}>
            <Plus className="h-3.5 w-3.5 mr-1" /> New Link
          </Button>
        </CardHeader>
        <CardContent>
          {showNewLink && (
            <div className="mb-4 p-3 border-2 border-border rounded space-y-2">
              <Input
                placeholder="Destination URL (e.g. https://example.com/page)"
                value={newLink.destination_url}
                onChange={(e) => setNewLink((p) => ({ ...p, destination_url: e.target.value }))}
              />
              <div className="flex gap-2">
                <Input
                  placeholder="Source (e.g. reddit)"
                  value={newLink.utm_source}
                  onChange={(e) => setNewLink((p) => ({ ...p, utm_source: e.target.value }))}
                />
                <Input
                  placeholder="Campaign (optional)"
                  value={newLink.utm_campaign}
                  onChange={(e) => setNewLink((p) => ({ ...p, utm_campaign: e.target.value }))}
                />
              </div>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  onClick={() => createMutation.mutate(newLink)}
                  disabled={!newLink.destination_url || !newLink.utm_source || createMutation.isPending}
                >
                  {createMutation.isPending ? "Creating…" : "Create"}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setShowNewLink(false)}>
                  Cancel
                </Button>
              </div>
            </div>
          )}
          {utmLinks.length === 0 && !showNewLink && (
            <Text as="p" className="text-sm text-muted-foreground">No tracking links yet. Create one to start attributing conversions.</Text>
          )}
          <div className="space-y-1">
            {utmLinks.map((link) => (
              <div key={link.id} className="flex items-center gap-3 py-2 border-b border-border last:border-0">
                <code className="text-xs bg-muted px-2 py-1 rounded border border-border font-mono shrink-0">
                  /r/{link.code}
                </code>
                <div className="flex-1 min-w-0">
                  <Text as="p" className="text-sm truncate">{link.destination_url}</Text>
                  <Text as="p" className="text-xs text-muted-foreground">
                    {link.utm_source}{link.utm_campaign ? ` · ${link.utm_campaign}` : ""}
                  </Text>
                </div>
                <span className="text-sm text-muted-foreground shrink-0">{link.click_count} clicks</span>
                <button
                  onClick={() => navigator.clipboard.writeText(`${window.location.origin}/r/${link.code}`)}
                  title="Copy short link"
                  className="text-muted-foreground hover:text-foreground transition-colors"
                >
                  <Copy className="h-3.5 w-3.5" />
                </button>
                <button
                  onClick={() => deleteMutation.mutate(link.id)}
                  title="Delete"
                  className="text-muted-foreground hover:text-destructive transition-colors"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
