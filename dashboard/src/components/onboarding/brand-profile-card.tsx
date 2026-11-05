import { useState } from "react";
import { Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { ChipList } from "./chip-list";
import type { BrandProfile, KeywordGroups } from "./types";

const TABS = ["Overview", "Customers", "Competitors", "Problems", "Keywords"] as const;
type Tab = (typeof TABS)[number];

const KEYWORD_GROUPS: { key: keyof KeywordGroups; label: string }[] = [
  { key: "brand", label: "Brand" },
  { key: "product", label: "Product" },
  { key: "category", label: "Category" },
  { key: "problem", label: "Problem" },
];

interface BrandProfileCardProps {
  profile: BrandProfile;
  onChange: (p: BrandProfile) => void;
}

const Label = ({ children }: { children: React.ReactNode }) => (
  <label className="block text-xs text-muted-foreground uppercase tracking-wider mb-1.5 font-medium">{children}</label>
);

/** Editable brand profile with the five onboarding tabs. */
export function BrandProfileCard({ profile, onChange }: BrandProfileCardProps) {
  const [tab, setTab] = useState<Tab>("Overview");
  const set = (patch: Partial<BrandProfile>) => onChange({ ...profile, ...patch });

  return (
    <div className="border border-border rounded-lg">
      <div className="flex border-b border-border overflow-x-auto">
        {TABS.map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={`px-3 py-2 text-sm cursor-pointer whitespace-nowrap ${
              tab === t ? "border-b-2 border-primary font-semibold text-foreground" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      <div className="p-4 space-y-3">
        {tab === "Overview" && (
          <>
            <Label>Product</Label>
            <Input value={profile.product} onChange={(e) => set({ product: e.target.value })} />
            <Label>What you do</Label>
            <Textarea value={profile.overview} onChange={(e) => set({ overview: e.target.value })} />
            <Label>Positioning</Label>
            <Input value={profile.positioning} onChange={(e) => set({ positioning: e.target.value })} placeholder="How you differ from alternatives" />
            <Label>Features</Label>
            <ChipList items={profile.features} onChange={(features) => set({ features })} placeholder="Add feature..." />
          </>
        )}
        {tab === "Customers" && <SegmentsEditor profile={profile} onChange={onChange} />}
        {tab === "Competitors" && (
          <>
            <Label>Competitors</Label>
            <ChipList items={profile.competitors} onChange={(competitors) => set({ competitors })} placeholder="Add competitor..." />
            <p className="text-xs text-muted-foreground">Used for the "Competitor switchers" listener.</p>
          </>
        )}
        {tab === "Problems" && <ProblemsEditor profile={profile} onChange={onChange} />}
        {tab === "Keywords" &&
          KEYWORD_GROUPS.map((g) => (
            <div key={g.key}>
              <Label>{g.label}</Label>
              <ChipList
                items={profile.keywords[g.key] ?? []}
                onChange={(items) => set({ keywords: { ...profile.keywords, [g.key]: items } })}
                placeholder={`Add ${g.label.toLowerCase()} keyword...`}
              />
            </div>
          ))}
      </div>
    </div>
  );
}

function SegmentsEditor({ profile, onChange }: BrandProfileCardProps) {
  const segs = profile.customer_segments;
  const update = (i: number, patch: Partial<(typeof segs)[number]>) =>
    onChange({ ...profile, customer_segments: segs.map((s, j) => (j === i ? { ...s, ...patch } : s)) });
  return (
    <div className="space-y-2">
      <Label>Customer segments</Label>
      {segs.map((s, i) => (
        <div key={i} className="flex gap-2 items-start">
          <div className="flex-1 space-y-1">
            <Input value={s.name} placeholder="Segment" onChange={(e) => update(i, { name: e.target.value })} />
            <Input value={s.why_they_buy} placeholder="Why they buy" onChange={(e) => update(i, { why_they_buy: e.target.value })} />
          </div>
          <Button variant="ghost" size="icon" aria-label="Remove segment" onClick={() => onChange({ ...profile, customer_segments: segs.filter((_, j) => j !== i) })}>
            <X className="size-4" />
          </Button>
        </div>
      ))}
      <Button variant="outline" size="sm" onClick={() => onChange({ ...profile, customer_segments: [...segs, { name: "", why_they_buy: "" }] })}>
        <Plus className="size-3.5" /> Add segment
      </Button>
    </div>
  );
}

function ProblemsEditor({ profile, onChange }: BrandProfileCardProps) {
  const pps = profile.pain_points;
  const update = (i: number, patch: Partial<(typeof pps)[number]>) =>
    onChange({ ...profile, pain_points: pps.map((p, j) => (j === i ? { ...p, ...patch } : p)) });
  return (
    <div className="space-y-2">
      <Label>Pain points</Label>
      {pps.map((p, i) => (
        <div key={i} className="flex gap-2 items-start">
          <div className="flex-1 space-y-1">
            <Input value={p.problem} placeholder="Problem" onChange={(e) => update(i, { problem: e.target.value })} />
            <Input value={p.explanation} placeholder="Why it hurts" onChange={(e) => update(i, { explanation: e.target.value })} />
          </div>
          <Button variant="ghost" size="icon" aria-label="Remove problem" onClick={() => onChange({ ...profile, pain_points: pps.filter((_, j) => j !== i) })}>
            <X className="size-4" />
          </Button>
        </div>
      ))}
      <Button variant="outline" size="sm" onClick={() => onChange({ ...profile, pain_points: [...pps, { problem: "", explanation: "" }] })}>
        <Plus className="size-3.5" /> Add problem
      </Button>
    </div>
  );
}
