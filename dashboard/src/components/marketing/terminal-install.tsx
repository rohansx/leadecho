import { useState } from "react";
import { GITHUB_URL } from "@/lib/constants";
import { Section, SectionHead } from "./section";
import { btnSecondary } from "./buttons";

const commands = {
  docker: [
    "git clone https://github.com/rohansx/leadecho.git",
    "cd leadecho",
    "cp .env.example .env    # add your AI keys",
    "docker compose up -d",
  ],
  manual: [
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
      <SectionHead title="Run it on your own servers">
        Every part of LeadEcho is in one repository and starts with one compose file. You pay only for your own AI
        usage.
      </SectionHead>

      <div className="mt-12 max-w-3xl">
        <div className="overflow-hidden rounded-[10px] bg-terminal text-[#dfe5ee]">
          <div role="tablist" className="flex border-b border-white/10 px-2">
            {(Object.keys(commands) as Tab[]).map((t) => (
              <button
                key={t}
                role="tab"
                type="button"
                aria-selected={tab === t}
                onClick={() => setTab(t)}
                className={`cursor-pointer border-b-2 px-4 py-3 text-[13px] font-semibold transition-colors ${
                  tab === t ? "border-marker text-white" : "border-transparent text-white/55 hover:text-white/85"
                }`}
              >
                {t === "docker" ? "Docker" : "From source"}
              </button>
            ))}
          </div>
          <pre className="overflow-x-auto p-6 font-[family-name:var(--font-mono)] text-[14px] leading-7">
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
      </div>
    </Section>
  );
}
