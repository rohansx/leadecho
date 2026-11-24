// Lead facts: what a post states about its author, read by the classifier for
// the workspace's product archetype (backend ai/facts.go).

export interface LeadFact {
  name: string;
  value: string;
}

// scoring_metadata.facts, keeping only well-formed entries.
export function leadFacts(meta: Record<string, unknown> | null | undefined): LeadFact[] {
  const raw = meta?.facts;
  if (!Array.isArray(raw)) return [];
  return raw.filter(
    (f): f is LeadFact => !!f && typeof f.name === "string" && typeof f.value === "string" && f.name !== "" && f.value !== "",
  );
}

export type Archetype = "" | "devtool" | "hiring" | "b2b_saas" | "vertical_smb" | "prosumer";

export const ARCHETYPES: { value: Archetype; label: string }[] = [
  { value: "", label: "None (no lead facts)" },
  { value: "devtool", label: "Developer tool: stack, scale, trigger, role" },
  { value: "hiring", label: "Hiring: company, roles, volume, assessment method" },
  { value: "b2b_saas", label: "B2B software: role, team size, current tool, timeline" },
  { value: "vertical_smb", label: "Small-business software: business type, region, size, deadline" },
  { value: "prosumer", label: "App for individuals: platform, use case, tool replaced, budget" },
];
