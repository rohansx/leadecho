import { createFileRoute, redirect } from "@tanstack/react-router";

// Automations now live under Workflows -> Scheduled; keep old links working.
export const Route = createFileRoute("/_dashboard/automations")({
  beforeLoad: () => {
    throw redirect({ to: "/workflows", search: { tab: "scheduled" } });
  },
});
