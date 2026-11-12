import { useState, type CSSProperties } from "react";
import { Cabinet } from "@lucasmarkes/hairline/react";
import { GITHUB_URL } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { container } from "./buttons";
import { Reveal } from "./fx";

const commands = {
  docker: [
    "git clone https://github.com/rohansx/leadecho.git",
    "cd leadecho",
    "cp .env.example .env    # add your AI keys",
    "docker compose up -d",
  ],
  source: [
    "git clone https://github.com/rohansx/leadecho.git",
    "cd leadecho/backend && go run ./cmd/api",
    "cd ../dashboard && pnpm install && pnpm dev",
  ],
};
type Tab = keyof typeof commands;

// Always dark, in both themes: the one place the bright yellow carries the page.
const band = {
  "--hairline-plate": "var(--band)",
  "--hairline-hi": "#f2f2f0",
  "--hairline-edge": "#7a7a82",
  "--hairline-mid": "#46464c",
  "--hairline-lo": "#2a2a2f",
} as CSSProperties;

export function TerminalInstall() {
  const [tab, setTab] = useState<Tab>("docker");

  return (
    <section id="self-host" className="scroll-mt-20 bg-band py-16 text-[#f2f2f0] sm:py-20" style={band}>
      <div className={cn(container, "grid gap-14 lg:grid-cols-[1fr_1.1fr] lg:items-center lg:gap-20 [&>*]:min-w-0")}>
        <Reveal>
          <p className="inline-flex items-center gap-2 font-[family-name:var(--font-mono)] text-[12px] text-white/55">
            <span className="size-1.5 rounded-full bg-[#ffd43b]" aria-hidden="true" />
            Self-host
          </p>
          <h2 className="mt-4 text-[clamp(2.1rem,4vw,3.1rem)] leading-[1.04] font-semibold tracking-[-0.035em] text-balance [font-stretch:92%]">
            Run the whole thing on your own servers.
          </h2>
          <p className="mt-5 max-w-md text-white/65">
            MIT licensed, one repository, one compose file. The full product with no feature gates. You pay only for
            your own AI usage, and your data never leaves your infrastructure.
          </p>
          <Cabinet theme="dark" play className="mt-10 max-w-[340px]" intensity={0.6} label="A server rack with blades sliding out" />
        </Reveal>

        <Reveal delay={0.1}>
          <div className="overflow-hidden rounded-[20px] border border-white/10 bg-white/[0.03]">
            <div role="tablist" className="flex gap-1 border-b border-white/10 p-2">
              {(Object.keys(commands) as Tab[]).map((t) => (
                <button
                  key={t}
                  role="tab"
                  type="button"
                  aria-selected={tab === t}
                  onClick={() => setTab(t)}
                  className={cn(
                    "cursor-pointer rounded-full px-3.5 py-1.5 text-[13px] font-semibold transition-colors",
                    tab === t ? "bg-[#ffd43b] text-[#18181b]" : "text-white/55 hover:text-white",
                  )}
                >
                  {t === "docker" ? "Docker" : "From source"}
                </button>
              ))}
            </div>
            <pre className="overflow-x-auto p-6 font-[family-name:var(--font-mono)] text-[14px] leading-7 text-white/85">
              {commands[tab].map((line) => (
                <div key={line}>
                  <span className="select-none text-[#ffd43b]">$ </span>
                  {line}
                </div>
              ))}
            </pre>
          </div>
          <a
            href={`${GITHUB_URL}#readme`}
            target="_blank"
            rel="noreferrer"
            className="mt-6 inline-flex h-11 items-center rounded-full bg-[#ffd43b] px-5 text-[15px] font-semibold text-[#18181b] transition-opacity hover:opacity-90"
          >
            Read the setup guide
          </a>
        </Reveal>
      </div>
    </section>
  );
}
