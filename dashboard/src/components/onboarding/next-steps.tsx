import { Link } from "@tanstack/react-router";
import { Bell, Bot, Chrome, Link2, UserPlus } from "lucide-react";

// Optional follow-ups, in order of moat. All link to existing settings pages.
const STEPS = [
  { icon: Chrome, title: "Install the extension", body: "Post replies in one click from the thread.", to: "/settings" },
  { icon: Link2, title: "Set the conversion webhook", body: "Attribute signups and revenue to replies.", to: "/settings" },
  { icon: Bell, title: "Alerts", body: "Daily digest by default; add Slack if you like.", to: "/alerts" },
  { icon: Bot, title: "Connect Claude via MCP", body: "Triage leads from your AI assistant.", to: "/settings" },
  { icon: UserPlus, title: "Invite a teammate", body: "Share the inbox and replies.", to: "/settings" },
] as const;

/** Step 5: optional next-step cards. */
export function NextSteps() {
  return (
    <div>
      <p className="block text-xs text-muted-foreground uppercase tracking-wider mb-2 font-medium">Optional next steps</p>
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
        {STEPS.map((s) => (
          <Link key={s.title} to={s.to} className="flex gap-2.5 p-3 border border-border rounded-lg hover:bg-muted transition-colors">
            <s.icon className="size-4 mt-0.5 text-primary shrink-0" />
            <span>
              <span className="block text-sm font-semibold">{s.title}</span>
              <span className="block text-xs text-muted-foreground">{s.body}</span>
            </span>
          </Link>
        ))}
      </div>
    </div>
  );
}
