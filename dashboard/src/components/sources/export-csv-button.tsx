import { Download } from "lucide-react";
import { exportUrl } from "@/lib/sources-api";

interface ExportCsvButtonProps {
  kind: "mentions" | "leads";
  params?: Record<string, string | undefined>;
  label?: string;
}

/** Plain download link: the browser streams the CSV with the session cookie. */
export function ExportCsvButton({ kind, params = {}, label }: ExportCsvButtonProps) {
  return (
    <a
      href={exportUrl(kind, params)}
      download
      title={`Export ${kind} as CSV`}
      aria-label={`Export ${kind} as CSV`}
      className={
        label
          ? "h-8 px-3 rounded-lg border border-border bg-card text-[13px] font-medium inline-flex items-center gap-1.5 hover:bg-accent shadow-xs"
          : "w-7 h-7 rounded-md border border-border hover:bg-accent flex items-center justify-center cursor-pointer shrink-0"
      }
    >
      <Download className="h-3.5 w-3.5" />
      {label}
    </a>
  );
}
