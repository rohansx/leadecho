// workflows: When / If / Then form with a live "would have matched" preview.
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { FlaskConical, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import { cn } from "@/lib/utils";
import { PLATFORMS } from "@/lib/constants";
import {
  ACTION_INFO,
  ACTION_ORDER,
  INTENT_LABELS,
  splitList,
  validateDefinition,
  type ActionType,
  type WorkflowDefinition,
} from "@/lib/workflow-templates";
import { previewWorkflow } from "@/lib/workflows-api";

const PLATFORM_CHOICES = PLATFORMS.filter((p) => p !== "quora");

function Chip({ on, onClick, children }: { on: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button
      type="button"
      aria-pressed={on}
      onClick={onClick}
      className={cn(
        "rounded-full border px-2.5 py-1 text-xs transition-colors",
        on ? "border-primary bg-primary/10 text-foreground" : "border-border text-muted-foreground hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

function Section({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <div>
        <Text as="p" className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{label}</Text>
        {hint && <Text as="p" className="text-xs text-muted-foreground">{hint}</Text>}
      </div>
      {children}
    </div>
  );
}

const toggle = (list: string[] | undefined, v: string) =>
  list?.includes(v) ? list.filter((x) => x !== v) : [...(list ?? []), v];

interface Props {
  initial: WorkflowDefinition;
  saving: boolean;
  error?: string;
  onSave: (d: WorkflowDefinition) => void;
  onCancel: () => void;
}

export function WorkflowEditor({ initial, saving, error, onSave, onCancel }: Props) {
  const [d, setD] = useState<WorkflowDefinition>(initial);
  const [keywords, setKeywords] = useState((initial.trigger.keywords ?? []).join(", "));
  const [excludes, setExcludes] = useState((initial.trigger.exclude_keywords ?? []).join(", "));
  const trigger = { ...d.trigger, keywords: splitList(keywords), exclude_keywords: splitList(excludes) };
  const def: WorkflowDefinition = { ...d, trigger };
  const problem = validateDefinition(def);
  const preview = useMutation({ mutationFn: () => previewWorkflow(trigger) });

  const setTrigger = (patch: Partial<WorkflowDefinition["trigger"]>) => setD((p) => ({ ...p, trigger: { ...p.trigger, ...patch } }));
  const has = (t: ActionType) => d.actions.some((a) => a.type === t);
  const toggleAction = (t: ActionType) =>
    setD((p) => ({ ...p, actions: has(t) ? p.actions.filter((a) => a.type !== t) : [...p.actions, { type: t }] }));
  const setConfig = (t: ActionType, key: "url" | "tag", value: string) =>
    setD((p) => ({ ...p, actions: p.actions.map((a) => (a.type === t ? { ...a, config: { ...a.config, [key]: value } } : a)) }));
  const configOf = (t: ActionType, key: "url" | "tag") => d.actions.find((a) => a.type === t)?.config?.[key] ?? "";

  return (
    <Card>
      <CardContent className="space-y-6 p-5">
        <Input
          aria-label="Workflow name"
          placeholder="Name, e.g. Hot buyer alert"
          value={d.name}
          maxLength={120}
          onChange={(e) => setD({ ...d, name: e.target.value })}
          className="text-base font-medium"
        />

        <Section label="When" hint="A new post is found and scored by AI.">
          <div className="flex flex-wrap items-center gap-3 text-sm">
            <label className="flex items-center gap-2">
              Score at least
              <Input
                type="number" min={0} max={10} step={0.5} className="h-8 w-20"
                value={d.trigger.min_score}
                onChange={(e) => setTrigger({ min_score: Math.min(10, Math.max(0, Number(e.target.value) || 0)) })}
              />
              <span className="text-xs text-muted-foreground">/ 10</span>
            </label>
          </div>
        </Section>

        <Section label="If the post is…" hint="Pick none to match any.">
          <div className="flex flex-wrap gap-1.5">
            {Object.entries(INTENT_LABELS).map(([tag, label]) => (
              <Chip key={tag} on={!!d.trigger.intent_tags?.includes(tag)} onClick={() => setTrigger({ intent_tags: toggle(d.trigger.intent_tags, tag) })}>
                {label}
              </Chip>
            ))}
          </div>
        </Section>

        <Section label="On" hint="Pick none for every platform you monitor.">
          <div className="flex flex-wrap gap-1.5">
            {PLATFORM_CHOICES.map((p) => (
              <Chip key={p} on={!!d.trigger.platforms?.includes(p)} onClick={() => setTrigger({ platforms: toggle(d.trigger.platforms, p) })}>
                {p}
              </Chip>
            ))}
          </div>
        </Section>

        <div className="grid gap-3 sm:grid-cols-2">
          <Section label="Mentions any of" hint="Comma-separated, optional.">
            <Input placeholder="pricing, alternative to" value={keywords} onChange={(e) => setKeywords(e.target.value)} />
          </Section>
          <Section label="But not" hint="Comma-separated, optional.">
            <Input placeholder="hiring, giveaway" value={excludes} onChange={(e) => setExcludes(e.target.value)} />
          </Section>
        </div>

        <Section label="Then">
          <div className="grid gap-2 sm:grid-cols-2">
            {ACTION_ORDER.map((t) => (
              <div key={t} className={cn("rounded-lg border p-3", has(t) ? "border-primary/60 bg-primary/5" : "border-border")}>
                <label className="flex cursor-pointer items-start gap-2.5">
                  <input type="checkbox" className="mt-0.5 accent-[var(--primary)]" checked={has(t)} onChange={() => toggleAction(t)} />
                  <span>
                    <span className="block text-sm font-medium">{ACTION_INFO[t].label}</span>
                    <span className="block text-xs text-muted-foreground">{ACTION_INFO[t].hint}</span>
                  </span>
                </label>
                {has(t) && t === "webhook" && (
                  <Input className="mt-2 h-8 text-xs" placeholder="https://hooks.zapier.com/…" value={configOf(t, "url")} onChange={(e) => setConfig(t, "url", e.target.value)} />
                )}
                {has(t) && t === "create_lead" && (
                  <Input className="mt-2 h-8 text-xs" placeholder="Tag (optional), e.g. hot" maxLength={40} value={configOf(t, "tag")} onChange={(e) => setConfig(t, "tag", e.target.value)} />
                )}
              </div>
            ))}
          </div>
        </Section>

        <div className="space-y-2 rounded-lg bg-muted/50 p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <Text as="p" className="text-sm">
              {preview.data
                ? `Would have caught ${preview.data.matched} of your last ${preview.data.checked} posts.`
                : "See what this would catch before you turn it on."}
            </Text>
            <Button type="button" size="sm" variant="outline" onClick={() => preview.mutate()} disabled={preview.isPending}>
              {preview.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <FlaskConical className="h-3.5 w-3.5" />} Test on recent posts
            </Button>
          </div>
          {preview.error && <Text as="p" className="text-xs text-destructive">{preview.error.message}</Text>}
          {preview.data?.samples.map((s) => (
            <a key={s.mention_id} href={s.url} target="_blank" rel="noreferrer noopener" className="block rounded-md border border-border bg-card p-2 hover:bg-accent">
              <span className="text-xs text-muted-foreground">{s.platform} · score {s.score.toFixed(1)}</span>
              <span className="line-clamp-1 block text-[13px]">{s.title || s.content}</span>
            </a>
          ))}
        </div>

        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border pt-4">
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" className="accent-[var(--primary)]" checked={d.enabled} onChange={(e) => setD({ ...d, enabled: e.target.checked })} />
            Turn on after saving
          </label>
          <div className="flex items-center gap-2">
            {(error || problem) && <Text as="p" className="text-xs text-destructive">{error || problem}</Text>}
            <Button type="button" variant="ghost" size="sm" onClick={onCancel}>Cancel</Button>
            <Button type="button" variant="accent" size="sm" disabled={saving || !!problem} onClick={() => onSave(def)}>
              {saving && <Loader2 className="h-3.5 w-3.5 animate-spin" />} Save workflow
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
