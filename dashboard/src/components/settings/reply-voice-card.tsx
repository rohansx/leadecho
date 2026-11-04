import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MessageSquareQuote, Plus, Star, Trash2 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Text } from "@/components/ui/text";
import {
  createResponseProfile,
  deleteResponseProfile,
  listResponseProfiles,
  setDefaultResponseProfile,
  updateResponseProfile,
  type ExamplePlatform,
  type ResponseProfile,
  type ResponseProfileInput,
} from "@/lib/api";
import { QUERY_KEYS } from "@/lib/constants";

const GOALS: { value: ResponseProfileInput["goal"]; label: string }[] = [
  { value: "drive_conversion", label: "Drive conversion" },
  { value: "build_relationship", label: "Build relationship" },
  { value: "ask_for_info", label: "Ask for info" },
  { value: "handle_feedback", label: "Handle feedback" },
  { value: "support", label: "Support" },
  { value: "custom", label: "Custom" },
];

const PLATFORMS: { key: ExamplePlatform; label: string }[] = [
  { key: "reddit", label: "Reddit" },
  { key: "x", label: "X" },
  { key: "linkedin", label: "LinkedIn" },
  { key: "bluesky", label: "Bluesky" },
  { key: "hackernews", label: "Hacker News" },
];

const PLUGS: ResponseProfileInput["brand_plug"][] = ["none", "low", "medium", "high"];
const LENGTHS: ResponseProfileInput["length"][] = ["short", "medium", "long"];

const EMPTY: ResponseProfileInput = {
  name: "",
  goal: "drive_conversion",
  special_instructions: "",
  tone: 40,
  length: "medium",
  assertiveness: 40,
  brand_plug: "low",
  cta_goal: "",
  destination_url: "",
  examples: {},
};

const selectClass =
  "h-9 w-full rounded-lg border border-border bg-card px-3 text-sm text-foreground shadow-xs";

interface SliderProps {
  label: string;
  left: string;
  right: string;
  value: number;
  onChange: (v: number) => void;
}

function Slider({ label, left, right, value, onChange }: SliderProps) {
  return (
    <label className="block space-y-1">
      <span className="text-sm font-medium">{label}</span>
      <input
        type="range"
        min={0}
        max={100}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="w-full accent-[var(--color-primary)]"
      />
      <span className="flex justify-between text-xs text-muted-foreground">
        <span>{left}</span>
        <span>{value}</span>
        <span>{right}</span>
      </span>
    </label>
  );
}

interface EditorProps {
  initial: ResponseProfileInput;
  pending: boolean;
  error: string | null;
  onSave: (p: ResponseProfileInput) => void;
  onCancel: () => void;
}

function ProfileEditor({ initial, pending, error, onSave, onCancel }: EditorProps) {
  const [p, setP] = useState<ResponseProfileInput>(initial);
  const set = <K extends keyof ResponseProfileInput>(k: K, v: ResponseProfileInput[K]) =>
    setP((prev) => ({ ...prev, [k]: v }));

  return (
    <div className="space-y-4 p-4 rounded-lg border border-border bg-background">
      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-sm font-medium">Name</span>
          <Input value={p.name} maxLength={120} onChange={(e) => set("name", e.target.value)} placeholder="Default voice" />
        </label>
        <label className="space-y-1">
          <span className="text-sm font-medium">Goal</span>
          <select className={selectClass} value={p.goal} onChange={(e) => set("goal", e.target.value as ResponseProfileInput["goal"])}>
            {GOALS.map((g) => (
              <option key={g.value} value={g.value}>{g.label}</option>
            ))}
          </select>
        </label>
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <Slider label="Tone" left="Casual" right="Formal" value={p.tone} onChange={(v) => set("tone", v)} />
        <Slider label="Assertiveness" left="Gentle" right="Direct" value={p.assertiveness} onChange={(v) => set("assertiveness", v)} />
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-sm font-medium">Length</span>
          <select className={selectClass} value={p.length} onChange={(e) => set("length", e.target.value as ResponseProfileInput["length"])}>
            {LENGTHS.map((l) => (
              <option key={l} value={l}>{l}</option>
            ))}
          </select>
        </label>
        <label className="space-y-1">
          <span className="text-sm font-medium">Brand plug</span>
          <select className={selectClass} value={p.brand_plug} onChange={(e) => set("brand_plug", e.target.value as ResponseProfileInput["brand_plug"])}>
            {PLUGS.map((b) => (
              <option key={b} value={b}>{b}</option>
            ))}
          </select>
        </label>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        <label className="space-y-1">
          <span className="text-sm font-medium">Call to action</span>
          <Input value={p.cta_goal} onChange={(e) => set("cta_goal", e.target.value)} placeholder="Try the free tier" />
        </label>
        <label className="space-y-1">
          <span className="text-sm font-medium">Tracked link destination</span>
          <Input value={p.destination_url} onChange={(e) => set("destination_url", e.target.value)} placeholder="https://yourproduct.com (defaults to onboarding URL)" />
        </label>
      </div>

      <label className="block space-y-1">
        <span className="text-sm font-medium">Special instructions</span>
        <Textarea value={p.special_instructions} maxLength={2000} onChange={(e) => set("special_instructions", e.target.value)} placeholder="Never mention pricing. Mention we're open source." />
      </label>

      <div className="space-y-2">
        <Text as="p" className="text-sm font-medium">Example replies (style samples)</Text>
        {PLATFORMS.map(({ key, label }) => (
          <label key={key} className="block space-y-1">
            <span className="text-xs text-muted-foreground">{label}</span>
            <Textarea
              className="min-h-14"
              maxLength={2000}
              value={p.examples[key] ?? ""}
              onChange={(e) => set("examples", { ...p.examples, [key]: e.target.value })}
            />
          </label>
        ))}
      </div>

      {error && <Text as="p" className="text-xs text-destructive">{error}</Text>}
      <div className="flex gap-2">
        <Button size="sm" disabled={pending || !p.name.trim()} onClick={() => onSave(p)}>
          {pending ? "Saving…" : "Save"}
        </Button>
        <Button size="sm" variant="ghost" onClick={onCancel}>Cancel</Button>
      </div>
    </div>
  );
}

function toInput(p: ResponseProfile): ResponseProfileInput {
  const { id: _id, is_default: _d, created_at: _c, updated_at: _u, ...rest } = p;
  return rest;
}

export function ReplyVoiceCard() {
  const queryClient = useQueryClient();
  const queryKey = [QUERY_KEYS.responseProfiles];
  const { data: profiles = [] } = useQuery({ queryKey, queryFn: listResponseProfiles });
  const [editing, setEditing] = useState<string | "new" | null>(null);
  const [error, setError] = useState<string | null>(null);

  const done = () => {
    setEditing(null);
    setError(null);
    queryClient.invalidateQueries({ queryKey });
  };
  const onError = (e: Error) => setError(e.message);

  const save = useMutation({
    mutationFn: ({ id, data }: { id: string | null; data: ResponseProfileInput }) =>
      id ? updateResponseProfile(id, data) : createResponseProfile(data),
    onSuccess: done,
    onError,
  });
  const remove = useMutation({ mutationFn: deleteResponseProfile, onSuccess: done, onError });
  const makeDefault = useMutation({ mutationFn: setDefaultResponseProfile, onSuccess: done, onError });

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <MessageSquareQuote className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>Reply voice</CardTitle>
            <CardDescription>
              How drafted replies sound. Optional: without a profile, replies use a sensible default.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {profiles.length === 0 && editing === null && (
          <Text as="p" className="text-sm text-muted-foreground">
            Using the default voice: drive conversion, low brand plug, conversational tone.
          </Text>
        )}
        {profiles.map((p) =>
          editing === p.id ? (
            <ProfileEditor
              key={p.id}
              initial={toInput(p)}
              pending={save.isPending}
              error={error}
              onSave={(data) => save.mutate({ id: p.id, data })}
              onCancel={() => { setEditing(null); setError(null); }}
            />
          ) : (
            <div key={p.id} className="flex items-center gap-2 p-3 rounded-lg border border-border bg-background">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <Text as="p" className="text-sm font-medium truncate">{p.name}</Text>
                  {p.is_default && <Badge variant="surface" size="sm">Default</Badge>}
                </div>
                <Text as="p" className="text-xs text-muted-foreground">
                  {GOALS.find((g) => g.value === p.goal)?.label} · {p.length} · plug {p.brand_plug}
                </Text>
              </div>
              {!p.is_default && (
                <Button size="sm" variant="ghost" disabled={makeDefault.isPending} onClick={() => makeDefault.mutate(p.id)}>
                  <Star className="h-3.5 w-3.5" /> Make default
                </Button>
              )}
              <Button size="sm" variant="outline" onClick={() => { setEditing(p.id); setError(null); }}>Edit</Button>
              <Button
                size="sm"
                variant="ghost"
                aria-label={`Delete ${p.name}`}
                disabled={remove.isPending}
                onClick={() => remove.mutate(p.id)}
              >
                <Trash2 className="h-3.5 w-3.5" />
              </Button>
            </div>
          ),
        )}
        {editing === "new" ? (
          <ProfileEditor
            initial={EMPTY}
            pending={save.isPending}
            error={error}
            onSave={(data) => save.mutate({ id: null, data })}
            onCancel={() => { setEditing(null); setError(null); }}
          />
        ) : (
          <Button size="sm" variant="outline" onClick={() => { setEditing("new"); setError(null); }}>
            <Plus className="h-3.5 w-3.5" /> New voice
          </Button>
        )}
        {error && editing === null && <Text as="p" className="text-xs text-destructive">{error}</Text>}
      </CardContent>
    </Card>
  );
}
