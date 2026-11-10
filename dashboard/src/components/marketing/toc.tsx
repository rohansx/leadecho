import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";

const tocGroups = [
  {
    label: "Product",
    items: [
      { id: "how", title: "How it filters" },
      { id: "who", title: "Who is asking" },
      { id: "attribution", title: "What converts" },
      { id: "features", title: "Everything else" },
    ],
  },
  {
    label: "Plans",
    items: [
      { id: "pricing", title: "Pricing" },
      { id: "compare", title: "Comparison" },
      { id: "self-host", title: "Self-host" },
    ],
  },
  { label: "Help", items: [{ id: "faq", title: "Questions" }] },
];

/** Sticky table of contents; highlights the section currently in view. */
export function Toc() {
  const [active, setActive] = useState("how");

  useEffect(() => {
    const ids = tocGroups.flatMap((g) => g.items.map((i) => i.id));
    const els = ids.map((id) => document.getElementById(id)).filter((e): e is HTMLElement => !!e);
    const io = new IntersectionObserver(
      (entries) => {
        const hit = entries.filter((e) => e.isIntersecting).sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top)[0];
        if (hit) setActive(hit.target.id);
      },
      { rootMargin: "-20% 0px -70% 0px" },
    );
    els.forEach((el) => io.observe(el));
    return () => io.disconnect();
  }, []);

  return (
    <nav aria-label="On this page" className="sticky top-24 space-y-7">
      {tocGroups.map((g) => (
        <div key={g.label}>
          <p className="font-[family-name:var(--font-plexmono)] text-[12px] uppercase tracking-[0.08em] text-muted-foreground">
            {g.label}
          </p>
          <ul className="mt-3 border-l border-border">
            {g.items.map((i) => (
              <li key={i.id}>
                <a
                  href={`#${i.id}`}
                  aria-current={active === i.id ? "true" : undefined}
                  className={cn(
                    "-ml-px block border-l-2 py-1 pl-4 text-[14px] transition-colors",
                    active === i.id
                      ? "border-yellow-deep text-foreground"
                      : "border-transparent text-muted-foreground hover:text-foreground",
                  )}
                >
                  {i.title}
                </a>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </nav>
  );
}
