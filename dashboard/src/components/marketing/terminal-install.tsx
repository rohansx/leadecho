import { useState } from "react";
import { GITHUB_URL } from "@/lib/constants";
import { cn } from "@/lib/utils";
import { Section, SectionHead } from "./section";
import { btnSecondary } from "./buttons";

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

export function TerminalInstall() {
  const [tab, setTab] = useState<Tab>("docker");

  return (
    <Section id="self-host">
      <SectionHead label="Self-host" title="Run it on your own servers." muted="The full product, MIT licensed.">
        Every part of LeadEcho is in one repository and starts with one compose file. You pay only for your own AI
        usage.
      </SectionHead>

      <div className="mt-10 overflow-hidden rounded-[18px] bg-terminal text-[#ece8dc]">
        <div role="tablist" className="flex gap-1 border-b border-white/10 p-2">
          {(Object.keys(commands) as Tab[]).map((t) => (
            <button
              key={t}
              role="tab"
              type="button"
              aria-selected={tab === t}
              onClick={() => setTab(t)}
              className={cn(
                "cursor-pointer rounded-full px-3.5 py-1.5 text-[13px] font-medium transition-colors",
                tab === t ? "bg-yellow text-[#1c1a14]" : "text-white/60 hover:text-white",
              )}
            >
              {t === "docker" ? "Docker" : "From source"}
            </button>
          ))}
        </div>
        <pre className="overflow-x-auto p-6 font-[family-name:var(--font-plexmono)] text-[14px] leading-7">
          {commands[tab].map((line) => (
            <div key={line}>
              <span className="select-none text-white/40">$ </span>
              {line}
            </div>
          ))}
        </pre>
      </div>
      <a href={`${GITHUB_URL}#readme`} target="_blank" rel="noreferrer" className={`mt-6 ${btnSecondary}`}>
        Read the setup guide
      </a>
    </Section>
  );
}
