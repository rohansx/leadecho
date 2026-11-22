import { Fragment } from "react";
import { cn } from "@/lib/utils";
import { parseMarkdown, type Inline } from "@/lib/markdown";

function Inlines({ nodes }: { nodes: Inline[] }) {
  return (
    <>
      {nodes.map((n, i) => {
        switch (n.kind) {
          case "bold":
            return <strong key={i} className="font-semibold text-foreground"><Inlines nodes={n.children} /></strong>;
          case "italic":
            return <em key={i}><Inlines nodes={n.children} /></em>;
          case "code":
            return <code key={i} className="rounded bg-muted px-1 py-px font-mono text-[0.85em]">{n.text}</code>;
          case "link":
            return (
              <a key={i} href={n.href} target="_blank" rel="noreferrer noopener nofollow" className="break-words text-primary-ink underline underline-offset-2">
                <Inlines nodes={n.children} />
              </a>
            );
          default:
            return <Fragment key={i}>{n.text}</Fragment>;
        }
      })}
    </>
  );
}

function Lines({ lines }: { lines: Inline[][] }) {
  return (
    <>
      {lines.map((l, i) => (
        <Fragment key={i}>
          {i > 0 && <br />}
          <Inlines nodes={l} />
        </Fragment>
      ))}
    </>
  );
}

interface PostMarkdownProps {
  text: string;
  className?: string;
}

/** A social post body with its Markdown rendered (safely: no HTML, no images). */
export function PostMarkdown({ text, className }: PostMarkdownProps) {
  const blocks = parseMarkdown(text);
  return (
    <div className={cn("space-y-2.5 break-words text-sm leading-relaxed text-foreground-soft", className)}>
      {blocks.map((b, i) => {
        switch (b.kind) {
          case "heading":
            return <p key={i} className="font-semibold text-foreground"><Inlines nodes={b.content} /></p>;
          case "list": {
            const Tag = b.ordered ? "ol" : "ul";
            return (
              <Tag key={i} className={cn("space-y-1 pl-5", b.ordered ? "list-decimal" : "list-disc")}>
                {b.items.map((it, j) => <li key={j}><Inlines nodes={it} /></li>)}
              </Tag>
            );
          }
          case "quote":
            return <blockquote key={i} className="border-l-2 border-border pl-3 italic"><Lines lines={b.lines} /></blockquote>;
          case "code":
            return <pre key={i} className="overflow-x-auto rounded-md bg-muted p-2.5 font-mono text-xs leading-5">{b.text}</pre>;
          default:
            return <p key={i}><Lines lines={b.lines} /></p>;
        }
      })}
    </div>
  );
}
