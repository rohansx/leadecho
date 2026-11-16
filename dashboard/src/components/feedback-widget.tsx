// Lobby's feedback widget (trylobby.app): a "Report a problem" button that
// lets people pin a comment to any element. Our API decides who gets it
// (people Lobby lets give feedback: claimed prospects and testers) and hands
// over only the public key and that user's own reporter token.
import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api";

interface FeedbackWidgetConfig {
  enabled: boolean;
  project?: string;
  reporter?: string;
}

const SCRIPT_ID = "lobby-feedback";
const SCRIPT_SRC = "https://trylobby.app/v1/fb.js";

export function FeedbackWidget() {
  const { data } = useQuery({
    queryKey: ["feedback-widget"],
    queryFn: () => request<FeedbackWidgetConfig>("/feedback/widget"),
    staleTime: Infinity,
    retry: false,
  });

  useEffect(() => {
    if (!data?.enabled || !data.project || !data.reporter || document.getElementById(SCRIPT_ID)) return;
    const s = document.createElement("script");
    s.id = SCRIPT_ID;
    s.src = SCRIPT_SRC;
    s.defer = true;
    s.dataset.project = data.project;
    s.dataset.reporter = data.reporter;
    s.dataset.accent = "#f0b429"; // --primary
    s.dataset.accentInk = "#1c1405"; // --primary-foreground
    document.body.appendChild(s);
  }, [data]);

  return null;
}
