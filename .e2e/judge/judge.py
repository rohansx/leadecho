#!/usr/bin/env python3
"""LLM-as-judge for LeadEcho output quality.

Grades, for one workspace, (1) the mentions LeadEcho discovered and scored and
(2) any reply drafts, against that workspace's own product context (monitoring
profile + knowledge-base documents). Uses Claude via the official Anthropic SDK
with a strict JSON schema, so every grade is machine-readable.

Usage:
  ANTHROPIC_API_KEY=... python3 -I judge.py --workspace <uuid> [--limit 25] [--out report.json]

Reads Postgres through `docker exec <container> psql` (the isolated e2e stack);
override with --psql "psql postgres://...".
"""

import argparse
import json
import shlex
import subprocess
import sys
from concurrent.futures import ThreadPoolExecutor

import anthropic

MODEL = "claude-opus-5-5"

GRADE_SCHEMA = {
    "type": "object",
    "properties": {
        "real_buyer_signal": {"type": "boolean"},
        "relevance_1_5": {"type": "integer", "enum": [1, 2, 3, 4, 5]},
        "intent_label_correct": {"type": "boolean"},
        "correct_intent": {
            "type": "string",
            "enum": ["buy_signal", "recommendation_ask", "complaint", "comparison", "general"],
        },
        "score_calibration": {"type": "string", "enum": ["too_high", "about_right", "too_low"]},
        "reply": {
            "type": "object",
            "properties": {
                "present": {"type": "boolean"},
                "factual_accuracy_1_5": {"type": "integer", "enum": [0, 1, 2, 3, 4, 5]},
                "helpfulness_1_5": {"type": "integer", "enum": [0, 1, 2, 3, 4, 5]},
                "not_spammy_1_5": {"type": "integer", "enum": [0, 1, 2, 3, 4, 5]},
                "would_convert_1_5": {"type": "integer", "enum": [0, 1, 2, 3, 4, 5]},
                "hallucinated_claims": {"type": "array", "items": {"type": "string"}},
            },
            "required": [
                "present", "factual_accuracy_1_5", "helpfulness_1_5",
                "not_spammy_1_5", "would_convert_1_5", "hallucinated_claims",
            ],
            "additionalProperties": False,
        },
        "verdict": {"type": "string"},
    },
    "required": [
        "real_buyer_signal", "relevance_1_5", "intent_label_correct", "correct_intent",
        "score_calibration", "reply", "verdict",
    ],
    "additionalProperties": False,
}

SYSTEM = """You are an exacting B2B go-to-market reviewer auditing an automated lead-finding tool.
The tool watches public posts (Hacker News, Reddit, Dev.to, ...) for people who have the
problem a product solves, scores them, and drafts a reply the founder may post.

Grade strictly against the PRODUCT CONTEXT you are given. A "real buyer signal" means the
author (or their team) plausibly has the problem and could adopt the product soon; news,
launches by competitors, generic commentary and the founder's own posts are not buyer
signals. Relevance 5 = squarely the product's pain; 1 = unrelated.

For the reply (if present): factual accuracy means every claim about the product is
supported by the product context; list any claim that is not in hallucinated_claims.
Penalise replies that read like ads, ignore the post's actual question, or would get
removed by community moderators. If no reply is present, set reply.present=false and
all reply scores to 0. Keep verdict to one or two sentences."""


def run_sql(psql: list[str], sql: str):
    out = subprocess.run(psql + ["-At", "-c", sql], check=True, capture_output=True, text=True).stdout.strip()
    return json.loads(out) if out else None


def load(psql: list[str], ws: str, limit: int, ids: list[str] | None = None):
    ws_lit = "'" + ws.replace("'", "") + "'::uuid"
    # ids are UUIDs from a previous report; keep only hex/dash to stay injection-safe.
    safe = ["'" + "".join(c for c in i if c in "0123456789abcdef-") + "'" for i in (ids or [])]
    id_filter = f"and m.id in ({','.join(safe)})" if safe else ""
    profile = run_sql(psql, f"""select json_build_object('name', name, 'description', description,
        'pain_points', (select coalesce(json_agg(phrase), '[]'::json) from pain_point_embeddings e where e.profile_id = p.id))
        from monitoring_profiles p where workspace_id = {ws_lit} and is_active limit 1""")
    docs = run_sql(psql, f"""select coalesce(json_agg(json_build_object('title', title, 'content', left(content, 6000))), '[]'::json)
        from documents where workspace_id = {ws_lit} and is_active""")
    mentions = run_sql(psql, f"""select coalesce(json_agg(t), '[]'::json) from (
        select m.id, m.platform, m.title, left(m.content, 3000) as content, m.url,
               m.intent, m.relevance_score, m.conversion_probability,
               m.scoring_metadata->>'reasoning' as reasoning,
               (select r.content from replies r where r.mention_id = m.id order by r.created_at desc limit 1) as reply
        from mentions m where m.workspace_id = {ws_lit} and m.intent is not null {id_filter}
        order by m.relevance_score desc nulls last limit {int(limit)}) t""")
    return profile or {}, docs or [], mentions or []


def product_context(profile: dict, docs: list) -> str:
    parts = [f"Product: {profile.get('name', '')}", f"Description: {profile.get('description', '')}"]
    if profile.get("pain_points"):
        parts.append("Pain points:\n- " + "\n- ".join(profile["pain_points"]))
    for d in docs:
        parts.append(f"Doc '{d['title']}':\n{d['content']}")
    return "\n\n".join(parts)


def grade(client: anthropic.Anthropic, context: str, m: dict) -> dict:
    item = {k: m.get(k) for k in ("platform", "title", "content", "url", "intent",
                                   "relevance_score", "conversion_probability", "reasoning", "reply")}
    resp = client.messages.create(
        model=MODEL,
        max_tokens=4000,
        system=SYSTEM,
        messages=[{"role": "user", "content":
                   f"PRODUCT CONTEXT\n{context}\n\nTOOL OUTPUT TO GRADE\n{json.dumps(item, ensure_ascii=False, indent=1)}"}],
        output_config={"format": {"type": "json_schema", "schema": GRADE_SCHEMA}},
        # Server-side fallback if Opus declines (category-routed, no model list to maintain).
        extra_headers={"anthropic-beta": "server-side-fallback-2026-07-01"},
        extra_body={"fallbacks": "default"},
    )
    if resp.stop_reason == "refusal":
        return {"error": "refused", "id": m["id"]}
    text = next(b.text for b in resp.content if b.type == "text")
    return {"id": m["id"], "tool": item, "grade": json.loads(text),
            "usage": {"in": resp.usage.input_tokens, "out": resp.usage.output_tokens}}


def summarize(results: list) -> dict:
    graded = [r for r in results if "grade" in r]
    n = len(graded) or 1
    g = [r["grade"] for r in graded]
    replies = [x["reply"] for x in g if x["reply"]["present"]]
    avg = lambda xs: round(sum(xs) / len(xs), 2) if xs else None
    return {
        "graded": len(graded),
        "errors": len(results) - len(graded),
        "precision_real_buyer": round(sum(x["real_buyer_signal"] for x in g) / n, 2),
        "avg_relevance_1_5": avg([x["relevance_1_5"] for x in g]),
        "intent_label_accuracy": round(sum(x["intent_label_correct"] for x in g) / n, 2),
        "calibration": {k: sum(x["score_calibration"] == k for x in g) for k in ("too_high", "about_right", "too_low")},
        "replies_graded": len(replies),
        "reply_avg": {k: avg([r[k] for r in replies]) for k in
                      ("factual_accuracy_1_5", "helpfulness_1_5", "not_spammy_1_5", "would_convert_1_5")},
        "replies_with_hallucinations": sum(bool(r["hallucinated_claims"]) for r in replies),
        "tokens": {"in": sum(r["usage"]["in"] for r in graded), "out": sum(r["usage"]["out"] for r in graded)},
    }


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--workspace", required=True)
    ap.add_argument("--limit", type=int, default=25)
    ap.add_argument("--out", default="judge-report.json")
    ap.add_argument("--ids-from", help="grade the same mentions as this earlier report (A/B)")
    ap.add_argument("--psql", default="docker exec leadecho-e2e-postgres psql -U leadecho -d leadecho_dev")
    args = ap.parse_args()

    ids = None
    if args.ids_from:
        with open(args.ids_from) as f:
            ids = [it["id"] for it in json.load(f)["items"] if "id" in it]
    profile, docs, mentions = load(shlex.split(args.psql), args.workspace, args.limit, ids)
    if not mentions:
        print("no scored mentions for this workspace", file=sys.stderr)
        return 1
    context = product_context(profile, docs)
    client = anthropic.Anthropic()
    with ThreadPoolExecutor(max_workers=4) as pool:
        results = list(pool.map(lambda m: grade(client, context, m), mentions))
    report = {"model": MODEL, "workspace": args.workspace, "summary": summarize(results), "items": results}
    with open(args.out, "w") as f:
        json.dump(report, f, indent=2, ensure_ascii=False)
    print(json.dumps(report["summary"], indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
