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
import { Plus, Trash2, Power, PowerOff, Search, X } from "lucide-react";

export const Route = createFileRoute("/_dashboard/keywords")({
  component: KeywordsPage,
});

const platformOptions = ["reddit", "hackernews", "devto", "lobsters", "indiehackers", "twitter", "linkedin"] as const;
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

  const { data: keywords, isLoading } = useQuery({
    queryKey: ["keywords"],
    queryFn: listKeywords,
  });

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
    <div className="space-y-6 max-w-3xl">
      <div>
        <Text as="h2">Keywords</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Terms to monitor across social platforms. The system crawls Reddit, Hacker News, Twitter, and LinkedIn for these.
        </Text>
      </div>

      {/* Add keyword */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Plus className="h-5 w-5 text-primary-foreground" />
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
          <div className="flex gap-2">
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
            <div className="flex gap-1.5">
              {platformOptions.map((p) => (
                <button
                  key={p}
                  type="button"
                  onClick={() => togglePlatform(p)}
                  className={`px-2.5 py-1 rounded text-xs border-2 cursor-pointer transition font-medium ${
                    selectedPlatforms.includes(p)
                      ? "bg-primary text-primary-foreground border-border shadow-xs"
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
            <div className="flex gap-1.5">
              {matchTypes.map((mt) => (
                <button
                  key={mt}
                  type="button"
                  onClick={() => setMatchType(mt)}
                  className={`px-2.5 py-1 rounded text-xs border-2 cursor-pointer transition font-medium ${
                    matchType === mt
                      ? "bg-primary text-primary-foreground border-border shadow-xs"
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

      {/* Keyword list */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary border-2 border-border shadow-xs flex items-center justify-center">
              <Search className="h-5 w-5 text-primary-foreground" />
            </div>
            <div>
              <CardTitle>Active Keywords</CardTitle>
              <CardDescription>
                {keywords?.length ?? 0} keyword{(keywords?.length ?? 0) !== 1 ? "s" : ""} configured
              </CardDescription>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <Text as="p" className="text-sm text-muted-foreground text-center py-8">
              Loading...
            </Text>
          ) : !keywords?.length ? (
            <div className="text-center py-8">
              <Search className="h-8 w-8 text-muted-foreground mx-auto mb-2 opacity-50" />
              <Text as="p" className="text-muted-foreground text-sm">
                No keywords yet. Add one above to start monitoring.
              </Text>
            </div>
          ) : (
            <div className="space-y-2">
              {keywords.map((kw: Keyword) => (
                <div
                  key={kw.id}
                  className={`p-3 rounded border-2 border-border transition ${
                    kw.is_active ? "bg-card" : "bg-muted opacity-60"
                  }`}
                >
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 min-w-0">
                      <Text as="span" className="font-medium text-sm truncate">
                        {kw.term}
                      </Text>
                      <Badge variant="outline" size="sm" className="text-xs shrink-0">
                        {kw.match_type || "broad"}
                      </Badge>
                    </div>
                    <div className="flex items-center gap-1 shrink-0">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7"
                        onClick={() =>
                          toggleMutation.mutate({
                            id: kw.id,
                            isActive: !kw.is_active,
                          })
                        }
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
                        onClick={() => deleteMutation.mutate(kw.id)}
                      >
                        <Trash2 className="h-3.5 w-3.5 text-destructive" />
                      </Button>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5 mt-2">
                    {kw.platforms.map((p) => (
                      <Badge key={p} variant="surface" size="sm" className="text-xs">
                        {p}
                      </Badge>
                    ))}
                    {kw.subreddits?.length > 0 && (
                      <div className="flex items-center gap-1 ml-2">
                        {kw.subreddits.map((sub) => (
                          <Badge key={sub} variant="outline" size="sm" className="text-xs">
                            r/{sub}
                          </Badge>
                        ))}
                      </div>
                    )}
                    {kw.negative_terms?.length > 0 && (
                      <div className="flex items-center gap-1 ml-2">
                        <X className="h-3 w-3 text-muted-foreground" />
                        <Text as="span" className="text-xs text-muted-foreground">
                          {kw.negative_terms.join(", ")}
                        </Text>
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
