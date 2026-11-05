import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarClock, Eye } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Text } from "@/components/ui/text";
import {
  getDigestSettings,
  previewDigest,
  saveDigestSettings,
  type DigestSettings,
} from "@/lib/api";

// Digests (P1 #13): daily/weekly digest schedule + preview on the Alerts page.

const QUERY_KEY = ["digest-settings"];
const WEEKDAYS = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"];
const TIMEZONES: string[] =
  typeof Intl.supportedValuesOf === "function" ? Intl.supportedValuesOf("timeZone") : ["UTC"];
const selectClass = "h-9 px-2 text-sm rounded-lg border border-border bg-background text-foreground";

interface CheckRowProps {
  label: string;
  help: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}

function CheckRow({ label, help, checked, onChange }: CheckRowProps) {
  return (
    <label className="flex items-start gap-3 cursor-pointer p-3 rounded-lg border border-border hover:bg-accent transition">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => onChange(e.target.checked)}
        className="w-4 h-4 accent-primary mt-0.5"
      />
      <div>
        <Text as="span" className="text-sm font-medium">{label}</Text>
        <Text as="p" className="text-xs text-muted-foreground">{help}</Text>
      </div>
    </label>
  );
}

function DigestForm({ initial }: { initial: DigestSettings }) {
  const queryClient = useQueryClient();
  const [s, setS] = useState<DigestSettings>(initial);
  const [recipients, setRecipients] = useState(initial.recipients.join(", "));
  const [previewHtml, setPreviewHtml] = useState<string | null>(null);

  const set = <K extends keyof DigestSettings>(key: K, value: DigestSettings[K]) =>
    setS((prev) => ({ ...prev, [key]: value }));
  const toggleChannel = (c: "email" | "slack", on: boolean) =>
    set("channels", on ? [...s.channels.filter((x) => x !== c), c] : s.channels.filter((x) => x !== c));

  const save = useMutation({
    mutationFn: () =>
      saveDigestSettings({
        ...s,
        recipients: recipients.split(",").map((r) => r.trim()).filter(Boolean),
      }),
    onSuccess: (next) => queryClient.setQueryData(QUERY_KEY, next),
  });
  const preview = useMutation({
    mutationFn: previewDigest,
    onSuccess: setPreviewHtml,
  });

  return (
    <CardContent className="space-y-3">
      <CheckRow
        label="Daily digest"
        help="Top leads since yesterday, counts by platform and intent, noisy keywords."
        checked={s.daily_enabled}
        onChange={(v) => set("daily_enabled", v)}
      />
      <CheckRow
        label="Weekly digest"
        help="Adds an AI summary of the week, replies posted, clicks and revenue attributed."
        checked={s.weekly_enabled}
        onChange={(v) => set("weekly_enabled", v)}
      />

      <div className="grid gap-3 sm:grid-cols-3">
        <label className="space-y-1">
          <Text as="span" className="text-sm font-medium">Send at</Text>
          <Input type="time" value={s.daily_time} onChange={(e) => set("daily_time", e.target.value)} />
        </label>
        <label className="space-y-1 flex flex-col">
          <Text as="span" className="text-sm font-medium">Weekly on</Text>
          <select value={s.weekly_day} onChange={(e) => set("weekly_day", e.target.value)} className={selectClass}>
            {WEEKDAYS.map((d) => (
              <option key={d} value={d}>{d[0].toUpperCase() + d.slice(1)}</option>
            ))}
          </select>
        </label>
        <label className="space-y-1">
          <Text as="span" className="text-sm font-medium">Timezone</Text>
          <Input list="digest-timezones" value={s.timezone} onChange={(e) => set("timezone", e.target.value)} />
          <datalist id="digest-timezones">
            {TIMEZONES.map((tz) => <option key={tz} value={tz} />)}
          </datalist>
        </label>
      </div>

      <div className="space-y-1">
        <Text as="p" className="text-sm font-medium">Deliver via</Text>
        <div className="flex gap-4">
          {(["email", "slack"] as const).map((c) => (
            <label key={c} className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="checkbox"
                checked={s.channels.includes(c)}
                onChange={(e) => toggleChannel(c, e.target.checked)}
                className="w-4 h-4 accent-primary"
              />
              {c === "email" ? "Email" : "Slack (uses the webhook above)"}
            </label>
          ))}
        </div>
      </div>

      {s.channels.includes("email") && (
        <label className="space-y-1 block">
          <Text as="span" className="text-sm font-medium">Recipients</Text>
          <Input
            placeholder="you@company.com, team@company.com"
            value={recipients}
            onChange={(e) => setRecipients(e.target.value)}
          />
          <Text as="p" className="text-xs text-muted-foreground">
            Comma-separated, up to 10. Empty sends to the workspace owner.
          </Text>
        </label>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <Button size="sm" onClick={() => save.mutate()} disabled={save.isPending}>
          {save.isPending ? "Saving..." : "Save digest settings"}
        </Button>
        <Button size="sm" variant="outline" onClick={() => preview.mutate("daily")} disabled={preview.isPending}>
          <Eye className="h-3.5 w-3.5 mr-1" /> Preview daily
        </Button>
        <Button size="sm" variant="outline" onClick={() => preview.mutate("weekly")} disabled={preview.isPending}>
          <Eye className="h-3.5 w-3.5 mr-1" /> Preview weekly
        </Button>
        {save.isSuccess && <Text as="p" className="text-sm text-green-600">Saved.</Text>}
        {save.isError && <Text as="p" className="text-sm text-destructive">{save.error.message}</Text>}
        {preview.isError && <Text as="p" className="text-sm text-destructive">{preview.error.message}</Text>}
      </div>

      {previewHtml !== null && (
        <iframe
          title="Digest preview"
          sandbox=""
          srcDoc={previewHtml}
          className="w-full h-[520px] rounded-lg border border-border bg-white"
        />
      )}
    </CardContent>
  );
}

export function DigestsCard() {
  const { data, isError } = useQuery({ queryKey: QUERY_KEY, queryFn: getDigestSettings });

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center gap-3">
          <div className="size-9 shrink-0 rounded-lg border border-border bg-secondary flex items-center justify-center">
            <CalendarClock className="h-4 w-4 text-foreground-soft" />
          </div>
          <div>
            <CardTitle>Digests</CardTitle>
            <CardDescription>
              A daily and weekly roundup of buyers, noisy keywords and revenue attributed.
            </CardDescription>
          </div>
        </div>
      </CardHeader>
      {data ? (
        <DigestForm initial={data} />
      ) : (
        <CardContent>
          <Text as="p" className="text-sm text-muted-foreground">
            {isError ? "Could not load digest settings." : "Loading..."}
          </Text>
        </CardContent>
      )}
    </Card>
  );
}
