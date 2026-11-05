import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Play, Trash2 } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Text } from "@/components/ui/text";
import { Badge } from "@/components/ui/badge";
import {
  deleteAutomation,
  listAutomationRuns,
  runAutomation,
  updateAutomation,
  type Automation,
  type AutomationConfig,
  type AutomationInput,
  type AutomationKind,
  type AutomationRun,
} from "@/lib/automations-api";

// automations (P2 #17): one automation — schedule, options, run now, history.

export const AUTOMATIONS_KEY = ["automations"];
const WEEKDAYS = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"];
const TIMEZONES: string[] =
  typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : ["UTC"];
const selectClass = "h-9 px-2 text-sm rounded-lg border border-border bg-background text-foreground";

export const KIND_INFO: Record<AutomationKind, { label: string; help: string }> = {
  daily_triage: {
    label: "Daily triage",
    help: "Ranks new mentions since the last run and lists the best ones. Can archive obvious noise.",
  },
  lead_sweep: {
    label: "Lead sweep",
    help: "Turns buying and seeking-alternative mentions that are not leads yet into leads. Can push them to your CRM.",
  },
  attribution_review: {
    label: "Attribution review",
    help: "Shows which keywords drove clicks, signups and revenue, and suggests muting the ones that never do.",
  },
  custom: {
    label: "Custom",
    help: "Runs a fixed list of read-only agent tools and stores their results.",
  },
};

interface NumberFieldProps {
  label: string;
  value: number | undefined;
  onChange: (v: number) => void;
  step?: number;
}

function NumberField({ label, value, onChange, step = 1 }: NumberFieldProps) {
  return (
    <label className="space-y-1">
      <Text as="span" className="text-sm font-medium">{label}</Text>
      <Input type="number" step={step} min={0} value={value ?? ""} onChange={(e) => onChange(Number(e.target.value))} />
    </label>
  );
}

interface CheckProps {
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}

function Check({ label, checked, onChange }: CheckProps) {
  return (
    <label className="flex items-center gap-2 text-sm cursor-pointer">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="w-4 h-4 accent-primary" />
      {label}
    </label>
  );
}

interface KindOptionsProps {
  kind: AutomationKind;
  config: AutomationConfig;
  setConfig: (c: AutomationConfig) => void;
  steps: string;
  setSteps: (s: string) => void;
}

function KindOptions({ kind, config, setConfig, steps, setSteps }: KindOptionsProps) {
  const set = <K extends keyof AutomationConfig>(k: K, v: AutomationConfig[K]) => setConfig({ ...config, [k]: v });
  switch (kind) {
    case "daily_triage":
      return (
        <div className="grid gap-3 sm:grid-cols-3 items-end">
          <NumberField label="List size" value={config.top_n} onChange={(v) => set("top_n", v)} />
          <NumberField label="Noise below score" step={0.5} value={config.noise_below} onChange={(v) => set("noise_below", v)} />
          <Check label="Archive noise automatically" checked={!!config.mark_noise} onChange={(v) => set("mark_noise", v)} />
        </div>
      );
    case "lead_sweep":
      return (
        <div className="grid gap-3 sm:grid-cols-3 items-end">
          <NumberField label="Minimum score" step={0.5} value={config.min_score} onChange={(v) => set("min_score", v)} />
          <NumberField label="Max leads per run" value={config.max_leads} onChange={(v) => set("max_leads", v)} />
          <Check label="Push new leads to CRM" checked={!!config.push_to_crm} onChange={(v) => set("push_to_crm", v)} />
        </div>
      );
    case "attribution_review":
      return (
        <div className="grid gap-3 sm:grid-cols-3">
          <NumberField label="Window (days)" value={config.days} onChange={(v) => set("days", v)} />
          <NumberField label="Suggest muting at mentions" value={config.min_mentions} onChange={(v) => set("min_mentions", v)} />
        </div>
      );
    case "custom":
      return (
        <label className="space-y-1 block">
          <Text as="span" className="text-sm font-medium">Steps (JSON)</Text>
          <Textarea rows={4} className="font-mono text-xs" value={steps} onChange={(e) => setSteps(e.target.value)} />
          <Text as="p" className="text-xs text-muted-foreground">
            Up to 5 read-only tools, e.g. [{"{"}"tool":"weekly_summary","args":{"{"}"days":7{"}"}{"}"}]
          </Text>
        </label>
      );
  }
}

function RunHistory({ id }: { id: string }) {
  const { data, isError } = useQuery({ queryKey: [...AUTOMATIONS_KEY, id, "runs"], queryFn: () => listAutomationRuns(id) });
  if (isError) return <Text as="p" className="text-sm text-destructive">Could not load run history.</Text>;
  if (!data) return <Text as="p" className="text-sm text-muted-foreground">Loading...</Text>;
  if (data.length === 0) return <Text as="p" className="text-sm text-muted-foreground">No runs yet.</Text>;
  return (
    <div className="space-y-2">
      {data.map((r) => <RunRow key={r.id} run={r} />)}
    </div>
  );
}

function RunRow({ run }: { run: AutomationRun }) {
  const o = run.output ?? {};
  return (
    <div className="rounded-lg border border-border p-3 space-y-1.5">
      <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <Badge variant={run.status === "failed" ? "solid" : run.status === "succeeded" ? "surface" : "default"} size="sm">
          {run.status}
        </Badge>
        <span>{run.trigger === "manual" ? "Run now" : "Scheduled"}</span>
        <span>{new Date(run.started_at).toLocaleString()}</span>
      </div>
      <Text as="p" className="text-sm">{o.summary || run.error || "No output."}</Text>
      {o.items && o.items.length > 0 && (
        <ul className="text-sm list-disc pl-5 space-y-0.5">
          {o.items.map((it, i) => (
            <li key={i}>
              {it.url ? <a href={it.url} target="_blank" rel="noreferrer noopener" className="underline underline-offset-2">{it.title}</a> : it.title}
              {typeof it.score === "number" && <span className="text-muted-foreground"> · {it.score.toFixed(1)}</span>}
              {it.note && <span className="text-muted-foreground"> · {it.note}</span>}
            </li>
          ))}
        </ul>
      )}
      {o.suggestions?.map((s, i) => <Text key={i} as="p" className="text-sm text-amber-700 dark:text-amber-400">{s}</Text>)}
      {o.notes?.map((n, i) => <Text key={i} as="p" className="text-xs text-muted-foreground">{n}</Text>)}
    </div>
  );
}

export function AutomationCard({ automation }: { automation: Automation }) {
  const queryClient = useQueryClient();
  const [a, setA] = useState<AutomationInput>({
    name: automation.name, kind: automation.kind, enabled: automation.enabled,
    schedule: automation.schedule, config: automation.config,
  });
  const [steps, setSteps] = useState(JSON.stringify(automation.config.steps ?? [], null, 0));
  const [showRuns, setShowRuns] = useState(false);
  const [stepsError, setStepsError] = useState<string | null>(null);

  const invalidate = () => queryClient.invalidateQueries({ queryKey: AUTOMATIONS_KEY });
  const save = useMutation({
    mutationFn: (next: AutomationInput) => updateAutomation(automation.id, next),
    onSuccess: invalidate,
  });
  const run = useMutation({
    mutationFn: () => runAutomation(automation.id),
    onSuccess: () => {
      setShowRuns(true);
      invalidate();
    },
  });
  const remove = useMutation({ mutationFn: () => deleteAutomation(automation.id), onSuccess: invalidate });

  const withSteps = (next: AutomationInput): AutomationInput | null => {
    if (next.kind !== "custom") return next;
    try {
      setStepsError(null);
      return { ...next, config: { ...next.config, steps: JSON.parse(steps) } };
    } catch {
      setStepsError("Steps must be valid JSON.");
      return null;
    }
  };
  const submit = (next: AutomationInput) => {
    const body = withSteps(next);
    if (body) save.mutate(body);
  };
  const setSchedule = <K extends keyof AutomationInput["schedule"]>(k: K, v: AutomationInput["schedule"][K]) =>
    setA((prev) => ({ ...prev, schedule: { ...prev.schedule, [k]: v } }));
  const error = save.error ?? run.error ?? remove.error;

  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <CardTitle>{automation.name}</CardTitle>
            <CardDescription>{KIND_INFO[automation.kind].help}</CardDescription>
            <Text as="p" className="text-xs text-muted-foreground mt-1">
              {automation.last_run_at ? `Last run ${new Date(automation.last_run_at).toLocaleString()}` : "Never run"}
            </Text>
          </div>
          <label className="flex items-center gap-2 text-sm cursor-pointer shrink-0">
            <input
              type="checkbox"
              role="switch"
              aria-label={`Enable ${automation.name}`}
              checked={a.enabled}
              onChange={(e) => {
                const next = { ...a, enabled: e.target.checked };
                setA(next);
                submit(next);
              }}
              className="w-4 h-4 accent-primary"
            />
            {a.enabled ? "On" : "Off"}
          </label>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid gap-3 sm:grid-cols-4">
          <label className="space-y-1">
            <Text as="span" className="text-sm font-medium">Name</Text>
            <Input value={a.name} onChange={(e) => setA({ ...a, name: e.target.value })} />
          </label>
          <label className="space-y-1 flex flex-col">
            <Text as="span" className="text-sm font-medium">Repeat</Text>
            <select value={a.schedule.frequency} onChange={(e) => setSchedule("frequency", e.target.value as "daily" | "weekly")} className={selectClass}>
              <option value="daily">Daily</option>
              <option value="weekly">Weekly</option>
            </select>
          </label>
          {a.schedule.frequency === "weekly" && (
            <label className="space-y-1 flex flex-col">
              <Text as="span" className="text-sm font-medium">On</Text>
              <select value={a.schedule.weekday} onChange={(e) => setSchedule("weekday", e.target.value)} className={selectClass}>
                {WEEKDAYS.map((d) => <option key={d} value={d}>{d[0].toUpperCase() + d.slice(1)}</option>)}
              </select>
            </label>
          )}
          <label className="space-y-1">
            <Text as="span" className="text-sm font-medium">At</Text>
            <Input type="time" value={a.schedule.time} onChange={(e) => setSchedule("time", e.target.value)} />
          </label>
          <label className="space-y-1">
            <Text as="span" className="text-sm font-medium">Timezone</Text>
            <Input list={`tz-${automation.id}`} value={a.schedule.timezone} onChange={(e) => setSchedule("timezone", e.target.value)} />
            <datalist id={`tz-${automation.id}`}>
              {TIMEZONES.map((tz) => <option key={tz} value={tz} />)}
            </datalist>
          </label>
        </div>

        <KindOptions kind={a.kind} config={a.config} setConfig={(config) => setA({ ...a, config })} steps={steps} setSteps={setSteps} />
        <Check label="Also email the result to the workspace owner" checked={!!a.config.email} onChange={(v) => setA({ ...a, config: { ...a.config, email: v } })} />

        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" onClick={() => submit(a)} disabled={save.isPending}>
            {save.isPending ? "Saving..." : "Save"}
          </Button>
          <Button size="sm" variant="outline" onClick={() => run.mutate()} disabled={run.isPending}>
            <Play className="h-3.5 w-3.5 mr-1" /> {run.isPending ? "Running..." : "Run now"}
          </Button>
          <Button size="sm" variant="outline" onClick={() => setShowRuns((v) => !v)}>
            <History className="h-3.5 w-3.5 mr-1" /> {showRuns ? "Hide history" : "Run history"}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            onClick={() => window.confirm(`Delete "${automation.name}"?`) && remove.mutate()}
            disabled={remove.isPending}
            aria-label={`Delete ${automation.name}`}
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
          {save.isSuccess && <Text as="p" className="text-sm text-green-600">Saved.</Text>}
          {stepsError && <Text as="p" className="text-sm text-destructive">{stepsError}</Text>}
          {error && <Text as="p" className="text-sm text-destructive">{error.message}</Text>}
        </div>

        {showRuns && <RunHistory id={automation.id} />}
      </CardContent>
    </Card>
  );
}
