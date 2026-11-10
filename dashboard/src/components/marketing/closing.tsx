import { Link } from "@tanstack/react-router";
import { Logo } from "./logo";
import { GITHUB_URL } from "@/lib/constants";
import { btnPrimary, btnSecondary } from "./buttons";

export function FinalCta() {
  return (
    <section className="px-5 pb-16 sm:px-10">
      <div className="rounded-[20px] border border-border px-6 py-16 sm:px-12 sm:py-20" style={{ background: "var(--demo)" }}>
        <h2 className="max-w-3xl text-[clamp(2.4rem,5vw,3.6rem)] leading-[1.05] font-light tracking-[-0.04em] text-balance">
          Someone is describing your problem in public right now.{" "}
          <span className="text-foreground/50">It can be you who answers.</span>
        </h2>
        <div className="mt-9 flex flex-wrap gap-3">
          <Link to="/register" className={btnPrimary}>
            Start free
          </Link>
          <a href={GITHUB_URL} target="_blank" rel="noreferrer" className={btnSecondary}>
            Self-host from GitHub
          </a>
        </div>
      </div>
    </section>
  );
}

const columns = [
  {
    title: "Product",
    links: [
      { label: "How it works", href: "#how" },
      { label: "Pricing", href: "#pricing" },
      { label: "Self-host", href: "#self-host" },
    ],
  },
  {
    title: "Resources",
    links: [
      { label: "Setup guide", href: `${GITHUB_URL}#readme`, external: true },
      { label: "Discussions", href: `${GITHUB_URL}/discussions`, external: true },
      { label: "FAQ", href: "#faq" },
    ],
  },
  {
    title: "Company",
    links: [
      { label: "GitHub", href: GITHUB_URL, external: true },
      { label: "Contact", href: "mailto:hello@leadecho.app" },
    ],
  },
];

export function MarketingFooter() {
  return (
    <footer className="border-t border-border px-5 py-14 sm:px-10">
      <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-[2fr_1fr_1fr_1fr]">
        <div>
          <Logo />
          <p className="mt-3 max-w-[16rem] text-[15px] text-muted-foreground">
            Find the people asking for what you sell, across the communities where they talk.
          </p>
        </div>
        {columns.map((c) => (
          <div key={c.title}>
            <h3 className="font-[family-name:var(--font-plexmono)] text-[12px] uppercase tracking-[0.08em] text-muted-foreground">
              {c.title}
            </h3>
            <ul className="mt-3 space-y-2 text-[15px]">
              {c.links.map((l) => (
                <li key={l.label}>
                  <a
                    href={l.href}
                    {...(l.external ? { target: "_blank", rel: "noreferrer" } : {})}
                    className="text-foreground-soft transition-colors hover:text-foreground"
                  >
                    {l.label}
                  </a>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      <div className="mt-12 flex flex-wrap justify-between gap-4 border-t border-border pt-6 text-[13px] text-muted-foreground">
        <span>© 2026 LeadEcho. Released under the MIT License.</span>
        <Link to="/privacy" className="hover:text-foreground">
          Privacy
        </Link>
      </div>
    </footer>
  );
}
