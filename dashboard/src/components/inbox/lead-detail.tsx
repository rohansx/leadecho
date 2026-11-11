import { useState } from "react";
import { parseResearch, sourceHost } from "@/lib/research";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { AnimatePresence, motion } from "motion/react";
import {
  ArchiveIcon,
  ArrowLeft,
  ArrowRight,
  Ban,
  Brain,
  Check,
  CheckCircle2,
  Copy,
  ExternalLink,
  Link2,
  Loader2,
  MessageSquareText,
  ShieldAlert,
  Sparkles,
  Users,
  Mail,
} from "lucide-react";
import { MuteAuthorButton } from "@/components/sources/mute-author-button"; // sources
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  approveReply,
  classifyMention,
  createReply,
  draftReply,
  getMentionPerson360,
  listReplies,
  listResponseProfiles,
  updateMentionStatus,
} from "@/lib/api";
import { API_BASE_URL, QUERY_KEYS } from "@/lib/constants";
import type { Mention, Reply } from "@/lib/types";
import { FeedbackButtons, TagChips } from "@/components/inbox/intent-tags";
import { EnrichmentCard } from "./enrichment-card"; // enrich
import { LeadAssistant } from "./lead-assistant";
import { companyMismatch, mailtoHref, safeHttpUrl } from "@/lib/outreach";

const awarenessLabels: Record<string, string> = {
  problem_aware: "Problem aware",
  solution_aware: "Solution aware",
  product_aware: "Product aware",
  purchase_ready: "Purchase ready",
};

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime();
  const hours = Math.floor(diff / (1000 * 60 * 60));
  if (hours < 1) return "just now";
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function SignalRow({ label, value }: { label: string; value: React.ReactNode }) {
  if (value == null || value === "") return null;
  return (
    <div className="flex items-start justify-between gap-4 py-2 text-sm border-b border-border last:border-0">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className="text-right text-foreground-soft">{value}</span>
    </div>
  );
}

function OverviewTab({
  mention,
  onFeedback,
}: {
  mention: Mention;
  onFeedback: (status: "spam" | "archived", reason?: string) => void;
}) {
  const queryClient = useQueryClient();
  const [localDraft, setDraft] = useState<Awaited<ReturnType<typeof draftReply>> | null>(null);
  const [copied, setCopied] = useState(false);
  const [destinationURL, setDestinationURL] = useState("");
  const [approveError, setApproveError] = useState<string | null>(null);
  const [manualReply, setManualReply] = useState("");
  const [manualError, setManualError] = useState<string | null>(null);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentions] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionCounts] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.mentionTierCounts] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.replies, mention.id] });
    queryClient.invalidateQueries({ queryKey: [QUERY_KEYS.scoringPrecision] });
  };

  const classifyMutation = useMutation({
    mutationFn: () => classifyMention(mention.id),
    onSuccess: invalidate,
  });

  // voice: optional response profile; "" = workspace default
  const [profileId, setProfileId] = useState("");
  const { data: voices = [] } = useQuery({
    queryKey: [QUERY_KEYS.responseProfiles],
    queryFn: listResponseProfiles,
  });

  const draftMutation = useMutation({
    mutationFn: () => draftReply(mention.id, profileId || undefined),
    onSuccess: (data) => {
      setDraft(data);
      invalidate();
    },
  });

  const markRepliedMutation = useMutation({
    mutationFn: () => updateMentionStatus(mention.id, "replied"),
    onSuccess: invalidate,
  });

  const manualMutation = useMutation({
    mutationFn: (content: string) => createReply(mention.id, content),
    onSuccess: (reply) => {
      setDraft((prev) =>
        prev
          ? { ...prev, reply, should_reply: true }
          : { reply, should_reply: true, status: "done", reason: "" } as Awaited<ReturnType<typeof draftReply>>,
      );
      setManualReply("");
      setManualError(null);
      invalidate();
    },
    onError: (err) => {
      setManualError(err instanceof Error ? err.message : "Failed to create draft");
    },
  });

  const handleManualDraft = () => {
    if (!manualReply.trim()) return;
    setManualError(null);
    manualMutation.mutate(manualReply.trim());
  };

  const approveMutation = useMutation({
    mutationFn: (replyId: string) =>
      approveReply(replyId, {
        destination_url: destinationURL.trim() || undefined,
        append_short_url: true,
      }),
    onSuccess: (reply) => {
      setApproveError(null);
      setDraft((prev) => (prev ? { ...prev, reply, should_reply: true } : prev));
      invalidate();
    },
    onError: (err: Error) => setApproveError(err.message),
  });

  const handleCopy = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1800);
  };

  // A draft saved earlier (by a workflow, the digest or a hiring signal) shows
  // without the user asking for a new one.
  const { data: savedReplies } = useQuery({
    queryKey: [QUERY_KEYS.replies, mention.id],
    queryFn: () => listReplies(mention.id),
  });
  const savedDraft = savedReplies?.find((r: Reply) => r.status === "draft");
  const draft =
    localDraft ??
    (savedDraft
      ? ({ reply: savedDraft, should_reply: true, status: "done", reason: "" } as Awaited<ReturnType<typeof draftReply>>)
      : null);
  // A hiring signal is a company, not a post: there is nothing to reply to,
  // and its draft is a first message to the contact.
  const isHiringSignal = (mention.platform_metadata as { signal?: string } | null)?.signal === "hiring";

  const signalCompany = (mention.platform_metadata as { company?: string } | null)?.company;
  const contactMismatch = isHiringSignal && companyMismatch(signalCompany, mention.person?.company);

  const profileUrl = safeHttpUrl(mention.person?.linkedin_url) ?? safeHttpUrl(mention.author_profile_url);

  const replyBody = draft?.reply
    ? draft.reply.edited_content ?? draft.reply.content
    : "";

  return (
    <div className="space-y-5">
      <article className="rounded-xl border border-border bg-card p-5">
        <div className="flex items-center gap-2 text-xs text-muted-foreground mb-3">
          <span className="font-medium text-foreground-soft">{mention.platform}</span>
          <span>·</span>
          {mention.person ? (
            <span className="text-foreground">
              <span className="font-medium">{mention.person.name || mention.author_username}</span>
              {[mention.person.title || mention.person.bio, [mention.person.company, mention.person.employees && `(${mention.person.employees} employees)`].filter(Boolean).join(" ")]
                .filter(Boolean)
                .map((part, i) => (
                  <span key={`${i}-${part}`} className="text-muted-foreground"> · {part}</span>
                ))}
            </span>
          ) : (
            <span>@{mention.author_username ?? "unknown"}</span>
          )}
          <a
            href={mention.url}
            target="_blank"
            rel="noreferrer"
            className="ml-auto inline-flex items-center gap-1 text-primary-ink hover:underline"
          >
            View original <ExternalLink className="h-3 w-3" />
          </a>
        </div>
        {mention.title && (
          <h3 className="font-[family-name:var(--font-head)] text-lg font-medium mb-2">{mention.title}</h3>
        )}
        <p className="text-sm text-foreground-soft leading-relaxed whitespace-pre-wrap">{mention.content}</p>
        <div className="mt-3">
          <TagChips tags={mention.intent_tags} sentiment={mention.sentiment} max={13} />
        </div>
      </article>

      <FeedbackButtons key={mention.id} mention={mention} />

      <div className="flex flex-wrap gap-2">
        {!mention.intent && (
          <Button size="sm" variant="outline" onClick={() => classifyMutation.mutate()} disabled={classifyMutation.isPending}>
            {classifyMutation.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Brain className="h-3.5 w-3.5" />
            )}
            Classify
          </Button>
        )}
        {!isHiringSignal && (
          <Button size="sm" onClick={() => draftMutation.mutate()} disabled={draftMutation.isPending}>
            {draftMutation.isPending ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Sparkles className="h-3.5 w-3.5" />
            )}
            Draft reply
          </Button>
        )}
        {voices.length > 1 && (
          <select
            aria-label="Reply voice"
            value={profileId}
            onChange={(e) => setProfileId(e.target.value)}
            className="h-8 rounded-lg border border-border bg-card px-2 text-[13px] text-foreground shadow-xs"
          >
            <option value="">Default voice</option>
            {voices.map((v) => (
              <option key={v.id} value={v.id}>{v.name}</option>
            ))}
          </select>
        )}
        {mention.status !== "replied" && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => markRepliedMutation.mutate()}
            disabled={markRepliedMutation.isPending}
          >
            <Check className="h-3.5 w-3.5" />
            Mark as replied
          </Button>
        )}
        {mention.status !== "spam" && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onFeedback("spam", "Marked spam from inbox")}
            className="text-destructive hover:text-destructive"
          >
            <ShieldAlert className="h-3.5 w-3.5" />
            Spam
          </Button>
        )}
        {mention.status !== "archived" && (
          <Button
            size="sm"
            variant="ghost"
            onClick={() => onFeedback("archived", "Not a lead")}
          >
            <Ban className="h-3.5 w-3.5" />
            Not a lead
          </Button>
        )}
      </div>

      <AnimatePresence>
        {draft && (
          <motion.div
            initial={{ opacity: 0, height: 0 }}
            animate={{ opacity: 1, height: "auto" }}
            exit={{ opacity: 0, height: 0 }}
            transition={{ duration: 0.25 }}
            className="overflow-hidden"
          >
            <div className="rounded-xl border border-primary/30 bg-accent-soft/40 p-4 space-y-3">
              {draft.status === "queued" && !draft.reply ? (
                <>
                  <div className="text-sm font-medium mb-1">Draft queued</div>
                  <p className="text-sm text-foreground-soft">
                    The drafter is working on this in the background. It will appear here shortly —
                    reopen the mention to refresh.
                  </p>
                </>
              ) : draft.should_reply === false ? (
                <>
                  <div className="text-sm font-medium mb-1">Not worth replying</div>
                  <p className="text-sm text-foreground-soft">{draft.reason}</p>
                  <div className="pt-2 border-t border-border/50">
                    <p className="text-xs text-muted-foreground mb-2">
                      Still want to reply? You can draft one manually.
                    </p>
                    <div className="flex gap-2">
                      <textarea
                        value={manualReply}
                        onChange={(e) => setManualReply(e.target.value)}
                        placeholder="Type your reply..."
                        className="flex-1 min-h-[80px] px-3 py-2 text-sm rounded-lg border border-border bg-background text-foreground resize-y"
                      />
                    </div>
                    <div className="flex gap-2 mt-2">
                      <Button
                        size="sm"
                        onClick={() => handleManualDraft()}
                        disabled={!manualReply.trim() || manualMutation.isPending}
                      >
                        {manualMutation.isPending ? (
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <MessageSquareText className="h-3.5 w-3.5" />
                        )}
                        Create draft
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setManualReply("")}
                      >
                        Clear
                      </Button>
                    </div>
                    {manualError && (
                      <p className="text-xs text-destructive mt-1">{manualError}</p>
                    )}
                  </div>
                </>
              ) : draft.reply ? (
                <>
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-1.5 flex-wrap text-xs">
                      <span className="font-medium text-sm mr-1">{isHiringSignal ? "First message to send" : "AI draft"}</span>
                      <span className="rounded-full bg-muted px-2 py-0.5 capitalize">{draft.reply.status}</span>
                      {draft.tone && <span className="rounded-full bg-muted px-2 py-0.5">{draft.tone}</span>}
                      {draft.template_style && (
                        <span className="rounded-full bg-muted px-2 py-0.5">
                          {draft.template_style.replace(/_/g, " ")}
                        </span>
                      )}
                      {draft.awareness_level && (
                        <span className="rounded-full bg-accent-soft text-primary-ink px-2 py-0.5">
                          {awarenessLabels[draft.awareness_level] ?? draft.awareness_level}
                        </span>
                      )}
                    </div>
                    <div className="flex items-center gap-1">
                      {isHiringSignal && mention.person?.email && (
                        <a
                          href={mailtoHref(mention.person.email, replyBody)}
                          className="inline-flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-[13px] font-medium text-foreground hover:bg-muted"
                        >
                          <Mail className="h-3.5 w-3.5" />
                          Open in email
                        </a>
                      )}
                      {isHiringSignal && profileUrl && (
                        <a
                          href={profileUrl}
                          target="_blank"
                          rel="noreferrer"
                          className="inline-flex h-8 items-center gap-1.5 rounded-lg px-2.5 text-[13px] font-medium text-foreground hover:bg-muted"
                        >
                          <ExternalLink className="h-3.5 w-3.5" />
                          LinkedIn profile
                        </a>
                      )}
                      <Button size="sm" variant="ghost" onClick={() => handleCopy(replyBody)}>
                        {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
                        {copied ? "Copied" : "Copy"}
                      </Button>
                    </div>
                  </div>
                  {contactMismatch && (
                    <p className="rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2 text-xs text-foreground">
                      Check this contact before sending: their profile lists {mention.person?.company}, not {signalCompany}. They may have moved, or this may be a different person.
                    </p>
                  )}
                  <p className="text-sm leading-relaxed whitespace-pre-wrap">{replyBody}</p>

                  {draft.reply.status === "draft" && (
                    <div className="rounded-lg border border-border bg-card/80 p-3 space-y-2">
                      <div className="flex items-center gap-1.5 text-xs font-medium text-foreground-soft">
                        <Link2 className="h-3.5 w-3.5" />
                        Track conversion (optional)
                      </div>
                      <Input
                        value={destinationURL}
                        onChange={(e) => setDestinationURL(e.target.value)}
                        placeholder="https://yoursite.com/signup"
                        className="h-9 text-sm"
                      />
                      <p className="text-[11px] text-muted-foreground">
                        Approving creates a tracked short link (it replaces your product URL, or is added when the reply names your product) and queues the reply for the Chrome extension.
                      </p>
                      {approveError && (
                        <p className="text-xs text-destructive">{approveError}</p>
                      )}
                      <Button
                        size="sm"
                        onClick={() => approveMutation.mutate(draft.reply!.id)}
                        disabled={approveMutation.isPending}
                      >
                        {approveMutation.isPending ? (
                          <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                          <CheckCircle2 className="h-3.5 w-3.5" />
                        )}
                        Approve for posting
                      </Button>
                    </div>
                  )}

                  {draft.reply.status === "approved" && (
                    <div className="rounded-lg border border-emerald-500/30 bg-emerald-500/5 px-3 py-2 text-xs text-foreground-soft">
                      Approved — waiting in the extension reply queue.
                      {draft.reply.short_url && (
                        <button
                          type="button"
                          className="ml-2 text-primary-ink hover:underline cursor-pointer"
                          onClick={() =>
                            handleCopy(
                              draft.reply!.short_url!.startsWith("http")
                                ? draft.reply!.short_url!
                                : `${API_BASE_URL}${draft.reply!.short_url}`,
                            )
                          }
                        >
                          Copy tracked link
                        </button>
                      )}
                    </div>
                  )}
                </>
              ) : null}
            </div>
          </motion.div>
        )}
      </AnimatePresence>

      <LeadAssistant key={mention.id} mention={mention} />
    </div>
  );
}

function Person360Tab({ mentionId }: { mentionId: string }) {
  const { data, isLoading, refetch } = useQuery({
    queryKey: [QUERY_KEYS.person360, mentionId],
    queryFn: () => getMentionPerson360(mentionId),
    refetchInterval: (query) => (query.state.data?.enriched ? false : 3000),
  });

  if (isLoading) {
    return <div className="text-sm text-muted-foreground py-8 text-center">Loading Person360…</div>;
  }

  if (!data?.enriched || !data.person) {
    return (
      <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground space-y-3">
        <Users className="h-5 w-5 mx-auto opacity-50" />
        <p>Enrichment runs when this mention qualifies as a lead.</p>
        <Button variant="outline" size="sm" onClick={() => refetch()}>
          Refresh
        </Button>
      </div>
    );
  }

  const person = data.person;

  return (
    <div className="space-y-4">
      <div className="rounded-xl border border-border bg-card p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h4 className="font-medium">{person.display_name ?? "Unknown"}</h4>
            {person.company && <p className="text-sm text-muted-foreground">{person.company}</p>}
            {person.location && <p className="text-xs text-muted-foreground mt-0.5">{person.location}</p>}
          </div>
          <div className="flex flex-col items-end gap-1">
            {person.icp_fit_score != null && (
              <span className="text-xs rounded-full bg-primary text-primary-foreground px-2 py-0.5">
                ICP fit {Math.round(person.icp_fit_score * 100)}%
              </span>
            )}
            <span className="text-xs rounded-full bg-primary/10 text-primary px-2 py-0.5">
              {Math.round(person.confidence * 100)}% confidence
            </span>
          </div>
        </div>
        {person.bio && <p className="text-sm mt-3 leading-relaxed text-foreground-soft">{person.bio}</p>}
      </div>

      <EnrichmentCard metadata={person.metadata} />

      <div>
        <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-2">
          Linked identities
        </h5>
        <div className="space-y-2">
          {data.identities.map((id) => (
            <div key={id.id} className="flex items-center justify-between rounded-lg border border-border px-3 py-2 text-sm">
              <div>
                <span className="capitalize font-medium">{id.platform}</span>
                <span className="text-muted-foreground ml-2">@{id.handle}</span>
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span>{Math.round(id.confidence * 100)}%</span>
                {id.profile_url && (
                  <a href={id.profile_url} target="_blank" rel="noreferrer" className="hover:text-primary">
                    <ExternalLink className="h-3.5 w-3.5" />
                  </a>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function ThreadTab({ mentionId }: { mentionId: string }) {
  const { data: replies, isLoading } = useQuery({
    queryKey: [QUERY_KEYS.replies, mentionId],
    queryFn: () => listReplies(mentionId),
  });

  if (isLoading) return <div className="text-sm text-muted-foreground py-8 text-center">Loading thread…</div>;

  if (!replies || replies.length === 0) {
    return (
      <div className="rounded-xl border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
        <MessageSquareText className="h-5 w-5 mx-auto mb-2 opacity-50" />
        No replies logged yet. Draft one from the Overview tab — once you post it, it'll show up here.
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {replies.map((r: Reply) => (
        <div key={r.id} className="rounded-xl border border-border bg-card p-4">
          <div className="flex items-center gap-2 text-xs text-muted-foreground mb-1.5">
            <span className="rounded-full bg-muted px-2 py-0.5 capitalize">{r.status}</span>
            {r.template_style && <span>{r.template_style.replace(/_/g, " ")}</span>}
            {r.short_url && <span className="truncate max-w-[140px]">{r.short_url}</span>}
            <span className="ml-auto">{timeAgo(r.created_at)}</span>
          </div>
          <p className="text-sm leading-relaxed whitespace-pre-wrap">{r.edited_content ?? r.content}</p>
        </div>
      ))}
    </div>
  );
}

function ResearchPanel({ mention }: { mention: Mention }) {
  const research = parseResearch(mention.platform_metadata);
  if (!research) return null;
  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-2">About the company</h5>
      {research.whatTheyDo && <p className="text-sm leading-relaxed">{research.whatTheyDo}</p>}
      {research.whyNow && (
        <p className="mt-2 text-sm leading-relaxed">
          <span className="font-medium">Why now: </span>
          {research.whyNow}
        </p>
      )}
      {research.hook && (
        <p className="mt-2 text-sm leading-relaxed text-foreground-soft">
          <span className="font-medium text-foreground">A way in: </span>
          {research.hook}
        </p>
      )}
      {research.facts.length > 0 && (
        <ul className="mt-3 space-y-2 border-t border-border pt-3">
          {research.facts.map((f) => (
            <li key={f.url + f.text} className="text-sm leading-relaxed text-foreground-soft">
              {f.text}{" "}
              <a href={f.url} target="_blank" rel="noreferrer noopener" className="whitespace-nowrap text-xs text-primary-ink underline underline-offset-2">
                {sourceHost(f.url)}
              </a>
            </li>
          ))}
        </ul>
      )}
      <p className="mt-3 text-xs text-muted-foreground">
        Found by searching the public web when this lead was confirmed. The summary above the line is written by AI from the sources below it: check a fact at its
        source before you quote it.
      </p>
    </div>
  );
}

function SignalsPanel({ mention }: { mention: Mention }) {
  const meta = mention.platform_metadata as Record<string, unknown>;
  const subreddit = typeof meta?.subreddit === "string" ? meta.subreddit : null;
  const feedback = mention.scoring_metadata?.last_feedback as
    | { label?: string; reason?: string }
    | undefined;
  const reasoning = typeof mention.scoring_metadata?.reasoning === "string" ? mention.scoring_metadata.reasoning : null;

  return (
    <div className="rounded-xl border border-border bg-card p-5">
      <h5 className="text-xs font-medium uppercase tracking-wide text-muted-foreground mb-2">Why this is here</h5>
      {reasoning && <p className="mb-3 text-sm leading-relaxed text-foreground-soft">{reasoning}</p>}
      <SignalRow label="Platform" value={<span className="capitalize">{mention.platform}</span>} />
      {subreddit && <SignalRow label="Community" value={subreddit} />}
      <SignalRow label="Author" value={mention.author_username ? `@${mention.author_username}` : null} />
      <SignalRow label="Karma" value={mention.author_karma} />
      <SignalRow
        label="Account age"
        value={mention.author_account_age_days != null ? `${mention.author_account_age_days}d` : null}
      />
      <SignalRow label="Type" value={mention.intent ? mention.intent.replace(/_/g, " ") : null} />
      <SignalRow
        label="Fit score"
        value={mention.relevance_score != null ? `${mention.relevance_score.toFixed(1)} / 10` : null}
      />
      {feedback?.label && (
        <SignalRow
          label="Feedback"
          value={`${feedback.label}${feedback.reason ? ` — ${feedback.reason}` : ""}`}
        />
      )}
      {mention.keyword_matches?.length > 0 && (
        <div className="pt-2">
          <div className="text-xs text-muted-foreground mb-1.5">Found by</div>
          <div className="flex flex-wrap gap-1">
            {mention.keyword_matches.map((k) => (
              <span key={k} className="text-xs rounded-full bg-muted px-2 py-0.5">
                {k}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

export function LeadDetail({
  mention,
  onPrev,
  onNext,
  hasPrev,
  hasNext,
  onArchive,
  archiving,
  onFeedback,
  feedbackPending,
}: {
  mention: Mention | null;
  onPrev: () => void;
  onNext: () => void;
  hasPrev: boolean;
  hasNext: boolean;
  onArchive: () => void;
  archiving: boolean;
  onFeedback: (status: "spam" | "archived", reason?: string) => void;
  feedbackPending: boolean;
}) {
  const [tab, setTab] = useState<"overview" | "person360" | "thread">("overview");

  if (!mention) {
    return (
      <section className="flex-1 flex items-center justify-center text-sm text-muted-foreground">
        Select a mention to see the details.
      </section>
    );
  }

  return (
    <section className="flex-1 flex flex-col h-full overflow-hidden">
      <div className="flex items-center gap-3 px-6 h-14 border-b border-border shrink-0">
        <div className="text-sm text-muted-foreground truncate">
          Inbox / <b className="text-foreground">{mention.platform}</b> · {timeAgo(mention.created_at)}
          {mention.status === "spam" && (
            <span className="ml-2 rounded-full bg-destructive/10 text-destructive px-2 py-0.5 text-[10px] uppercase tracking-wide">
              spam
            </span>
          )}
        </div>
        <div className="ml-auto flex items-center gap-1.5">
          <button
            type="button"
            onClick={onPrev}
            disabled={!hasPrev}
            className="h-7 w-7 rounded-md border border-border hover:bg-accent disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center cursor-pointer"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={onNext}
            disabled={!hasNext}
            className="h-7 w-7 rounded-md border border-border hover:bg-accent disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center cursor-pointer"
          >
            <ArrowRight className="h-3.5 w-3.5" />
          </button>
          <button
            type="button"
            onClick={onArchive}
            disabled={archiving || feedbackPending}
            className="h-7 w-7 rounded-md border border-border hover:bg-accent flex items-center justify-center cursor-pointer"
            aria-label="Archive"
          >
            <ArchiveIcon className="h-3.5 w-3.5" />
          </button>
          <MuteAuthorButton key={mention.id} mention={mention} />
        </div>
      </div>

      <div className="flex gap-1 px-6 pt-3 border-b border-border shrink-0">
        {(["overview", "person360", "thread"] as const).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={`relative px-3 pb-3 text-sm font-medium capitalize cursor-pointer transition-colors ${
              tab === t ? "text-foreground" : "text-muted-foreground hover:text-foreground"
            }`}
          >
            {t === "person360" ? "Person360" : t}
            {tab === t && (
              <motion.div layoutId="detail-tab" className="absolute left-0 right-0 -bottom-px h-0.5 bg-primary" />
            )}
          </button>
        ))}
      </div>

      <div className="flex-1 overflow-y-auto p-6 grid lg:grid-cols-[1fr_280px] gap-6 items-start">
        <AnimatePresence mode="wait">
          <motion.div
            key={`${mention.id}-${tab}`}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.18 }}
          >
            {tab === "overview" ? (
              <OverviewTab mention={mention} onFeedback={onFeedback} />
            ) : tab === "person360" ? (
              <Person360Tab mentionId={mention.id} />
            ) : (
              <ThreadTab mentionId={mention.id} />
            )}
          </motion.div>
        </AnimatePresence>
        <ResearchPanel mention={mention} />
        <SignalsPanel mention={mention} />
      </div>
    </section>
  );
}
