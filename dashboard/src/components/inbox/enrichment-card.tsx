// enrich (P1 #11): treg enrichment stored on persons.metadata.enrichment.
import { ExternalLink } from "lucide-react";

interface Enrichment {
  full_name?: string;
  title?: string;
  company?: string;
  company_domain?: string;
  industry?: string;
  employees?: string;
  company_size?: number;
  email?: string;
  email_status?: string;
  linkedin_url?: string;
  location?: string;
  enriched_at?: string;
}

const statusTone: Record<string, string> = {
  verified: "text-green-600",
  invalid: "text-red-600",
};

function readEnrichment(meta: Record<string, unknown> | undefined): Enrichment | null {
  const e = meta?.enrichment;
  return e && typeof e === "object" ? (e as Enrichment) : null;
}

export function EnrichmentCard({ metadata }: { metadata?: Record<string, unknown> }) {
  const e = readEnrichment(metadata);
  if (!e) return null;
  const rows: [string, string | undefined][] = [
    ["Title", e.title],
    ["Company", [e.company, e.company_domain && `(${e.company_domain})`].filter(Boolean).join(" ") || undefined],
    ["Industry", e.industry],
    ["Company size", e.employees || (e.company_size ? `${e.company_size}+` : undefined)],
    ["Location", e.location],
  ];
  const shown = rows.filter(([, v]) => v);
  if (shown.length === 0 && !e.email && !e.linkedin_url) {
    return (
      <div className="rounded-xl border border-dashed border-border p-3 text-xs text-muted-foreground">
        Enrichment ran but found no contact data for this author.
      </div>
    );
  }
  return (
    <div className="rounded-xl border border-border bg-card p-4 space-y-2">
      <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Enrichment</h5>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
        {shown.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="truncate">{v}</dd>
          </div>
        ))}
        {e.email && (
          <div className="contents">
            <dt className="text-muted-foreground">Email</dt>
            <dd className="truncate">
              {e.email}
              {e.email_status && (
                <span className={`ml-2 text-xs ${statusTone[e.email_status] ?? "text-muted-foreground"}`}>{e.email_status}</span>
              )}
            </dd>
          </div>
        )}
        {e.linkedin_url && (
          <div className="contents">
            <dt className="text-muted-foreground">LinkedIn</dt>
            <dd>
              <a href={e.linkedin_url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 hover:text-primary">
                Profile <ExternalLink className="h-3 w-3" />
              </a>
            </dd>
          </div>
        )}
      </dl>
    </div>
  );
}
