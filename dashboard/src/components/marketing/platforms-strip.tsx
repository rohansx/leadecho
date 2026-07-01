const platforms = [
  "Reddit",
  "Twitter / X",
  "LinkedIn",
  "Hacker News",
  "Dev.to",
  "Lobsters",
  "Indie Hackers",
];

export function PlatformsStrip() {
  const loop = [...platforms, ...platforms];
  return (
    <div
      aria-label="Monitored platforms"
      className="border-y border-border bg-card overflow-hidden py-4"
    >
      <div className="flex w-max animate-[marquee_28s_linear_infinite] gap-10 text-sm font-[family-name:var(--font-mono)] text-muted-foreground">
        {loop.map((p, i) => (
          <span key={`${p}-${i}`} className="shrink-0">
            {p}
          </span>
        ))}
      </div>
    </div>
  );
}
