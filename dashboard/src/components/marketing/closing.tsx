import { Link } from "@tanstack/react-router";
import { Logo } from "./logo";
import { GITHUB_URL } from "@/lib/constants";
import { btnPrimary, btnSecondary } from "./buttons";

export function FinalCta() {
  return (
    <section className="border-t border-border bg-card">
      <div className="mx-auto max-w-6xl px-4 py-28 sm:px-6">
        <h2 className="max-w-3xl text-[clamp(2.5rem,5vw,4rem)] leading-[1] font-bold tracking-[-0.035em] text-balance">
          Someone is describing your problem in public right now.
        </h2>
        <p className="mt-6 max-w-xl text-lg text-foreground-soft">
          Somebody will answer them today. With the right context and the right reply, it can be you.
        </p>
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
    <footer className="border-t border-border">
      <div className="mx-auto max-w-6xl px-4 py-14 sm:px-6">
        <div className="grid gap-10 sm:grid-cols-2 lg:grid-cols-[2fr_1fr_1fr_1fr]">
          <div>
            <Logo />
            <p className="mt-3 max-w-[16rem] text-[15px] text-muted-foreground">
              Find the people asking for what you sell, across the communities where they talk.
            </p>
          </div>
          {columns.map((c) => (
            <div key={c.title}>
              <h3 className="text-[15px] font-semibold">{c.title}</h3>
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
      </div>
    </footer>
  );
}
