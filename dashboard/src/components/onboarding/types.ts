import type { Mention, ProductAnalysis } from "@/lib/types";

export type Goal = "find_buyers" | "competitor_switchers" | "brand_mentions" | "content_ideas";

export const GOALS: { value: Goal; label: string; hint: string }[] = [
  { value: "find_buyers", label: "Find buyers", hint: "People shopping for a tool like yours" },
  { value: "competitor_switchers", label: "Competitor switchers", hint: "Unhappy users of alternatives" },
  { value: "brand_mentions", label: "Brand mentions", hint: "Anyone talking about you" },
  { value: "content_ideas", label: "Content ideas", hint: "Questions worth writing about" },
];

export const goalLabel = (g: string): string => GOALS.find((x) => x.value === g)?.label ?? g;

export interface CustomerSegment {
  name: string;
  why_they_buy: string;
  // Prospecting searches for companies with these (needs criteria + buyer_titles).
  pain?: string;
  criteria?: string[];
  negative_criteria?: string[];
  example_companies?: string[];
  buyer_titles?: string;
  country?: string; // ISO 3166-1 alpha-2, "" = any
}

export interface PainPointDetail {
  problem: string;
  explanation: string;
}

export interface KeywordGroups {
  brand: string[];
  product: string[];
  category: string[];
  problem: string[];
}

export interface BrandProfile {
  overview: string;
  product: string;
  positioning: string;
  customer_segments: CustomerSegment[];
  competitors: string[];
  pain_points: PainPointDetail[];
  keywords: KeywordGroups;
  features: string[];
  target_audience: string;
  source_url: string;
  updated_at?: string;
}

export interface AnalysisResult {
  analysis: ProductAnalysis & { suggested_subreddits?: string[] };
  brand_profile: BrandProfile;
  pages: { url: string; title: string }[];
  fallback: boolean;
}

export interface Sample {
  platform: string;
  url: string;
  title: string;
  text: string;
  author: string;
  created_at: string;
}

export interface QueryTested {
  goal: Goal;
  platform: string;
  keyword: string;
  count: number;
  error?: string;
}

export interface Proposal {
  goal: Goal;
  name: string;
  keywords: string[];
  looks_for: string;
  skips: string;
  platforms: string[];
}

export interface ListenerState {
  goal: Goal;
  steps: string[];
  tests: QueryTested[];
  samples: Sample[];
  proposal?: Proposal;
  feedback: Record<string, "up" | "down">;
  included: boolean;
}

export interface FirstValue {
  tier: "leads_ready" | "worth_watching" | "recent" | "none";
  loosened: boolean;
  mentions: Mention[];
  counts: Record<string, number>;
  scanning: boolean;
}
