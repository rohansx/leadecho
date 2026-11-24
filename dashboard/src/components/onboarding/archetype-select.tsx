import { ARCHETYPES, type Archetype } from "@/lib/lead-facts";

/** The kind of product, which picks the lead facts read from each post. */
export function ArchetypeSelect({ value, onChange }: { value: string | undefined; onChange: (a: Archetype) => void }) {
  return (
    <select
      value={value ?? ""}
      onChange={(e) => onChange(e.target.value as Archetype)}
      aria-label="Product type"
      className="w-full rounded-lg border border-border bg-card px-2.5 py-1.5 text-sm"
    >
      {ARCHETYPES.map((a) => (
        <option key={a.value} value={a.value}>
          {a.label}
        </option>
      ))}
    </select>
  );
}
