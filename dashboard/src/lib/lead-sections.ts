// A company signal's body is written by the backend (monitor/hiring.go,
// monitor/prospect.go) as paragraphs, some starting with a fixed label.
// This splits it back into those labelled parts so the detail view can show
// them as rows instead of one run of text.

export interface LeadSection {
  label: string;
  text: string;
  /** The web-search paragraph: the research panel shows the same facts. */
  research: boolean;
}

export interface LeadSections {
  intro: string;
  sections: LeadSection[];
}

// [prefix as the backend writes it, label shown, is research]
const LABELS: [RegExp, string, boolean][] = [
  [/^Who to contact:\s*/, "Who to contact", false],
  [/^Not confirmed as a lead:\s*/, "Not confirmed", false],
  [/^What a web search found[^:\n]*:\s*/, "Web search (not verified)", true],
  [/^Why this is a signal:\s*/, "Why this is a signal", false],
  [/^What this segment struggles with:\s*/, "Segment pain", false],
];

/** Returns null when the text has no labelled paragraph: render it as it is. */
export function splitLeadSections(content: string | null | undefined): LeadSections | null {
  if (!content) return null;
  const intro: string[] = [];
  const sections: LeadSection[] = [];
  for (const para of content.split(/\n{2,}/)) {
    const hit = LABELS.find(([re]) => re.test(para));
    if (hit) {
      sections.push({ label: hit[1], text: para.replace(hit[0], "").trim(), research: hit[2] });
    } else if (sections.length > 0) {
      // An unlabelled paragraph after a label belongs to that label.
      const last = sections[sections.length - 1];
      sections[sections.length - 1] = { ...last, text: `${last.text}\n\n${para}` };
    } else {
      intro.push(para);
    }
  }
  if (sections.length === 0) return null;
  return { intro: intro.join("\n\n").trim(), sections };
}
