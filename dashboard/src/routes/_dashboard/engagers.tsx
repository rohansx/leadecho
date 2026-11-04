// enrich (P2 #16): LinkedIn engagers -> leads, and tracked LinkedIn/X profiles.
import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Send, Trash2, UserPlus, Users } from "lucide-react";
import { Text } from "@/components/ui/text";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
  createTrackedProfile,
  deleteTrackedProfile,
  engagersToCSV,
  listTrackedProfiles,
  pullEngagers,
  pushEngagersToCRM,
  type EngagerKind,
  type EngagerRow,
} from "@/lib/enrich-api";

export const Route = createFileRoute("/_dashboard/engagers")({
  component: EngagersPage,
});

function downloadCSV(rows: EngagerRow[]) {
  const blob = new Blob([engagersToCSV(rows)], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = "linkedin-engagers.csv";
  a.click();
  URL.revokeObjectURL(url);
}

function KindToggle({ kind, kinds, onChange }: { kind: EngagerKind; kinds: EngagerKind[]; onChange: (k: EngagerKind[]) => void }) {
  const on = kinds.includes(kind);
  return (
    <label className="inline-flex items-center gap-1.5 text-sm">
      <input
        type="checkbox"
        checked={on}
        onChange={() => onChange(on ? kinds.filter((k) => k !== kind) : [...kinds, kind])}
      />
      <span className="capitalize">{kind}</span>
    </label>
  );
}

function EngagersCard() {
  const [postURL, setPostURL] = useState("");
  const [kinds, setKinds] = useState<EngagerKind[]>(["comments", "reactions"]);
  const [enrich, setEnrich] = useState(false);
  const [rows, setRows] = useState<EngagerRow[]>([]);
  const [notice, setNotice] = useState<string | null>(null);

  const pull = useMutation({
    mutationFn: () => pullEngagers({ post_url: postURL.trim(), kinds, enrich }),
    onSuccess: (res) => {
      setRows(res.data);
      setNotice(`${res.total} engagers imported as leads${enrich ? " — enrichment running in the background" : ""}.`);
    },
    onError: (e: Error) => setNotice(e.message),
  });
  const push = useMutation({
    mutationFn: () => pushEngagersToCRM(rows.map((r) => r.lead_id)),
    onSuccess: (res) => setNotice(`${res.queued} leads queued for CRM sync.`),
    onError: (e: Error) => setNotice(e.message),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>LinkedIn engagers</CardTitle>
        <CardDescription>
          Paste a LinkedIn post URL. Its commenters and reactors become leads (tagged engager), scored against your ICP.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            setNotice(null);
            pull.mutate();
          }}
        >
          <Input
            placeholder="https://www.linkedin.com/feed/update/urn:li:activity:…"
            value={postURL}
            onChange={(e) => setPostURL(e.target.value)}
          />
          <Button type="submit" size="sm" disabled={!postURL.trim() || kinds.length === 0 || pull.isPending}>
            <Users className="h-3.5 w-3.5 mr-1" />
            {pull.isPending ? "Pulling…" : "Pull engagers"}
          </Button>
        </form>
        <div className="flex flex-wrap items-center gap-4">
          <KindToggle kind="comments" kinds={kinds} onChange={setKinds} />
          <KindToggle kind="reactions" kinds={kinds} onChange={setKinds} />
          <label className="inline-flex items-center gap-1.5 text-sm">
            <input type="checkbox" checked={enrich} onChange={(e) => setEnrich(e.target.checked)} />
            Enrich (work email + company, uses data credits)
          </label>
        </div>
        {notice && <Text as="p" className="text-xs text-muted-foreground">{notice}</Text>}

        {rows.length > 0 && (
          <>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={() => downloadCSV(rows)}>
                <Download className="h-3.5 w-3.5 mr-1" /> Export CSV
              </Button>
              <Button variant="outline" size="sm" onClick={() => push.mutate()} disabled={push.isPending}>
                <Send className="h-3.5 w-3.5 mr-1" /> Push to CRM
              </Button>
            </div>
            <div className="overflow-x-auto rounded-lg border border-border">
              <table className="w-full text-sm">
                <thead className="bg-muted/40 text-left text-xs text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 font-medium">Name</th>
                    <th className="px-3 py-2 font-medium">Headline</th>
                    <th className="px-3 py-2 font-medium">Engagement</th>
                    <th className="px-3 py-2 font-medium text-right">ICP fit</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((r) => (
                    <tr key={r.lead_id} className="border-t border-border align-top">
                      <td className="px-3 py-2">
                        {r.profile_url ? (
                          <a href={r.profile_url} target="_blank" rel="noreferrer" className="font-medium hover:text-primary">
                            {r.name}
                          </a>
                        ) : (
                          <span className="font-medium">{r.name}</span>
                        )}
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">{r.headline}</td>
                      <td className="px-3 py-2">
                        <Badge variant="outline" size="sm" className="text-xs">
                          {r.kind === "comment" ? "Commented" : `Reacted${r.reaction ? ` · ${r.reaction.toLowerCase()}` : ""}`}
                        </Badge>
                        {r.comment && <p className="mt-1 text-xs text-muted-foreground line-clamp-2">{r.comment}</p>}
                      </td>
                      <td className="px-3 py-2 text-right tabular-nums">
                        {r.icp_fit_score != null ? `${Math.round(r.icp_fit_score * 100)}%` : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}

function TrackedProfilesCard() {
  const qc = useQueryClient();
  const [platform, setPlatform] = useState<"linkedin" | "x">("linkedin");
  const [handle, setHandle] = useState("");
  const [error, setError] = useState<string | null>(null);
  const { data, isLoading } = useQuery({ queryKey: ["tracked-profiles"], queryFn: listTrackedProfiles });
  const refresh = () => qc.invalidateQueries({ queryKey: ["tracked-profiles"] });

  const add = useMutation({
    mutationFn: () => createTrackedProfile({ platform, handle: handle.trim() }),
    onSuccess: () => {
      setHandle("");
      setError(null);
      refresh();
    },
    onError: (e: Error) => setError(e.message),
  });
  const remove = useMutation({ mutationFn: deleteTrackedProfile, onSuccess: refresh });
  const profiles = data?.data ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Tracked profiles</CardTitle>
        <CardDescription>New posts from these LinkedIn / X accounts land in your inbox as mentions.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            add.mutate();
          }}
        >
          <select
            value={platform}
            onChange={(e) => setPlatform(e.target.value as "linkedin" | "x")}
            className="h-9 rounded-md border border-border bg-background px-2 text-sm"
            aria-label="Platform"
          >
            <option value="linkedin">LinkedIn</option>
            <option value="x">X</option>
          </select>
          <Input
            placeholder={platform === "linkedin" ? "linkedin.com/in/… or handle" : "@handle or x.com/…"}
            value={handle}
            onChange={(e) => setHandle(e.target.value)}
          />
          <Button type="submit" size="sm" disabled={!handle.trim() || add.isPending}>
            <UserPlus className="h-3.5 w-3.5 mr-1" /> Track
          </Button>
        </form>
        {error && <Text as="p" className="text-xs text-red-600">{error}</Text>}
        {isLoading ? (
          <Text as="p" className="text-sm text-muted-foreground">Loading…</Text>
        ) : profiles.length === 0 ? (
          <Text as="p" className="text-sm text-muted-foreground">No tracked profiles yet.</Text>
        ) : (
          <div className="space-y-2">
            {profiles.map((p) => (
              <div key={p.id} className="flex items-center justify-between rounded-lg border border-border px-3 py-2 text-sm">
                <div className="min-w-0">
                  <span className="font-medium">{p.platform === "x" ? "X" : "LinkedIn"}</span>
                  <a href={p.url} target="_blank" rel="noreferrer" className="ml-2 text-muted-foreground hover:text-primary">
                    @{p.handle}
                  </a>
                  <span className="ml-2 text-xs text-muted-foreground">
                    {p.last_polled_at ? `checked ${new Date(p.last_polled_at).toLocaleString()}` : "not checked yet"}
                  </span>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={`Stop tracking ${p.handle}`}
                  onClick={() => remove.mutate(p.id)}
                  className="text-destructive hover:text-destructive"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function EngagersPage() {
  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div>
        <Text as="h2">LinkedIn engagers</Text>
        <Text as="p" className="text-muted-foreground mt-1">
          Turn the people engaging with a post into leads, and follow accounts whose posts signal buying intent.
        </Text>
      </div>
      <EngagersCard />
      <TrackedProfilesCard />
    </div>
  );
}
