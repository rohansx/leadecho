import { test, expect } from "@playwright/test";
import { execSync } from "node:child_process";

// Seed a "tricky" mention directly in the DB: high relevance (9.0) but a
// non-lead intent (comparison). Per CountMentionsByTier this belongs to the
// "filtered" bucket; before the fix it fell through all three tier LIST queries
// and was unreachable from every inbox tab. Idempotent.
function seedTierProbe(): string {
  const platformId = "hn_tier_probe_regression";
  execSync(
    `docker exec leadecho-e2e-postgres psql -U leadecho -d leadecho_dev -c ` +
      `"INSERT INTO mentions (workspace_id, platform, platform_id, url, content, status, relevance_score, intent) ` +
      `SELECT u.workspace_id,'hackernews','${platformId}','https://news.ycombinator.com/item?id=probe',` +
      `'comparing analytics tools','new',9.0,'comparison' FROM users u WHERE u.email='e2e-tester@leadecho.test' ` +
      `ON CONFLICT DO NOTHING;"`,
    { stdio: "pipe" },
  );
  return platformId;
}

/**
 * API-level regression tests locking in the backend correctness/validation fixes
 * from the adversarial bug-hunt. These hit the proxied /api directly (fast,
 * precise) using the authenticated storageState from the global setup.
 *
 * Each test cleans up anything it creates.
 */

test.describe("backend regressions: error mapping & validation", () => {
  // ── mentions: tier list/count consistency + status validation ──
  test("high-score non-lead mention is reachable via the 'filtered' tier (no vanishing)", async ({
    page,
  }) => {
    const probe = seedTierProbe();
    const idsIn = async (tier: string) => {
      const list = await (
        await page.request.get(`/api/v1/mentions?tier=${tier}&limit=1000`)
      ).json();
      const items = Array.isArray(list) ? list : list.data ?? list.mentions;
      return items.map((m: any) => m.platform_id);
    };
    // Belongs to "filtered" (score>=7 but intent not a lead intent) — must be
    // present there and absent from leads_ready / worth_watching.
    expect(await idsIn("filtered"), "probe reachable in filtered tier").toContain(
      probe,
    );
    expect(await idsIn("leads_ready")).not.toContain(probe);
    expect(await idsIn("worth_watching")).not.toContain(probe);
  });

  test("inbox filters compose (AND) and total is the real match count, not page size", async ({
    page,
  }) => {
    const get = async (qs: string) =>
      (await page.request.get(`/api/v1/mentions?${qs}`)).json();

    // total reflects the full match set even with a tiny page.
    const all = await get("limit=2");
    expect(all.data.length).toBeLessThanOrEqual(2);
    expect(all.total, "total must exceed page size").toBeGreaterThan(2);

    // A platform constraint must narrow a search (previously platform was ignored
    // because only the first matching filter was applied).
    const search = await get("search=analytics&limit=50");
    const searchTwitter = await get(
      "search=analytics&platform=twitter&limit=50",
    );
    expect(searchTwitter.total).toBeLessThanOrEqual(search.total);
    // Every returned row must satisfy BOTH constraints.
    for (const m of searchTwitter.data) expect(m.platform).toBe("twitter");
  });

  test("inbox rejects unknown enum filter values with 400 (not a 500 enum-cast)", async ({
    page,
  }) => {
    for (const qs of [
      "status=bogus",
      "platform=bogus",
      "intent=bogus",
      "tier=filtered&status=bogus", // composed: must still validate
      "search=foo&platform=bogus",
    ]) {
      const res = await page.request.get(`/api/v1/mentions?${qs}`);
      expect(res.status(), `${qs} -> 400`).toBe(400);
    }
    // Valid values still return 200.
    expect(
      (await page.request.get("/api/v1/mentions?platform=hackernews&status=new"))
        .status(),
    ).toBe(200);
  });

  test("mention status update: invalid status -> 400, bogus id -> 404", async ({
    page,
  }) => {
    const bad = await page.request.patch(
      "/api/v1/mentions/00000000-0000-0000-0000-0000000000ff/status",
      { data: { status: "not-a-real-status" } },
    );
    expect(bad.status()).toBe(400);

    const missing = await page.request.patch(
      "/api/v1/mentions/00000000-0000-0000-0000-0000000000ff/status",
      { data: { status: "reviewed" } },
    );
    expect(missing.status()).toBe(404);
  });

  // ── leads: stage validation + not-found mapping ──
  test("lead stage update: invalid stage -> 400, non-existent lead -> 404 (not 500)", async ({
    page,
  }) => {
    const badStage = await page.request.patch(
      "/api/v1/leads/00000000-0000-0000-0000-0000000000ff/stage",
      { data: { stage: "totally-invalid" } },
    );
    expect(badStage.status()).toBe(400);

    const missing = await page.request.patch(
      "/api/v1/leads/00000000-0000-0000-0000-0000000000ff/stage",
      { data: { stage: "qualified" } },
    );
    expect(missing.status(), "non-existent lead must be 404, not 500").toBe(404);
  });

  // ── keywords: duplicate -> 409, invalid platform -> 400, delete semantics ──
  test("keyword create: duplicate term -> 409; invalid platform -> 400", async ({
    page,
  }) => {
    const term = `e2e-reg-kw-${Date.now()}`;
    const first = await page.request.post("/api/v1/keywords", {
      data: { term, platforms: ["reddit"], match_type: "broad" },
    });
    expect(first.status()).toBe(201);
    const id = (await first.json()).id;

    const dup = await page.request.post("/api/v1/keywords", {
      data: { term, platforms: ["reddit"], match_type: "broad" },
    });
    expect(dup.status(), "duplicate term must be 409, not 500").toBe(409);

    const badPlatform = await page.request.post("/api/v1/keywords", {
      data: { term: `${term}-x`, platforms: ["myspace"], match_type: "broad" },
    });
    expect(badPlatform.status(), "invalid platform must be 400").toBe(400);

    // cleanup
    expect((await page.request.delete(`/api/v1/keywords/${id}`)).status()).toBe(
      200,
    );
  });

  test("keyword delete: malformed id -> 400, non-existent -> 404", async ({
    page,
  }) => {
    expect(
      (await page.request.delete("/api/v1/keywords/not-a-uuid")).status(),
      "malformed id -> 400",
    ).toBe(400);
    // 36-char but non-hex must also be 400 (not a uuid-cast 500).
    expect(
      (
        await page.request.delete(
          "/api/v1/keywords/zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
        )
      ).status(),
      "36-char non-hex id -> 400",
    ).toBe(400);
    expect(
      (
        await page.request.delete(
          "/api/v1/keywords/00000000-0000-0000-0000-0000000000ff",
        )
      ).status(),
      "non-existent -> 404 (not a false 200 'deleted')",
    ).toBe(404);
  });

  // ── documents: no soft-delete resurrection, source_url scheme guard ──
  test("document update does not resurrect a soft-deleted doc; deleted doc 404s", async ({
    page,
  }) => {
    const create = await page.request.post("/api/v1/documents", {
      data: { title: `reg-doc-${Date.now()}`, content: "hello world" },
    });
    expect(create.status()).toBe(201);
    const id = (await create.json()).id;

    expect((await page.request.delete(`/api/v1/documents/${id}`)).status()).toBe(
      200,
    );
    // After soft-delete: GET 404, and UPDATE must NOT revive it.
    expect((await page.request.get(`/api/v1/documents/${id}`)).status()).toBe(
      404,
    );
    const revive = await page.request.put(`/api/v1/documents/${id}`, {
      data: { title: "revived?", content: "should not work" },
    });
    expect(revive.status(), "updating a deleted doc must 404, not resurrect").toBe(
      404,
    );
  });

  test("document source_url rejects non-http(s) (javascript:) scheme", async ({
    page,
  }) => {
    const xss = await page.request.post("/api/v1/documents", {
      data: {
        title: `reg-xss-${Date.now()}`,
        content: "x",
        source_url: "javascript:alert(1)",
      },
    });
    expect(xss.status(), "javascript: source_url must be 400").toBe(400);
  });

  // ── profiles: delete semantics + name validation + truthful pain_points ──
  test("profile delete non-existent -> 404; whitespace name -> 400", async ({
    page,
  }) => {
    expect(
      (
        await page.request.delete(
          "/api/v1/profiles/00000000-0000-0000-0000-0000000000ff",
        )
      ).status(),
    ).toBe(404);

    const ws = await page.request.post("/api/v1/profiles", {
      data: { name: "   ", description: "x", pain_points: [] },
    });
    expect(ws.status(), "whitespace-only name must be 400").toBe(400);
  });

  test("profile create reports only persisted pain_points (no phantom phrases without an embedder)", async ({
    page,
  }) => {
    const create = await page.request.post("/api/v1/profiles", {
      data: {
        name: `reg-prof-${Date.now()}`,
        description: "x",
        pain_points: ["phantom phrase A", "phantom phrase B"],
      },
    });
    expect(create.status()).toBe(201);
    const body = await create.json();
    // No embedder configured in e2e → phrases are NOT persisted → response must
    // not claim they were (previously it echoed the request body optimistically).
    expect(
      body.pain_points,
      "pain_points must reflect what was actually stored",
    ).toEqual([]);
    // cleanup
    await page.request.delete(`/api/v1/profiles/${body.id}`);
  });

  // ── security: SSRF guard + open-redirect guard (validation only, no egress) ──
  test("webhook test rejects non-https and disallowed hosts (SSRF guard)", async ({
    page,
  }) => {
    // http scheme -> 400 (would otherwise allow http://169.254.169.254 metadata SSRF)
    const httpScheme = await page.request.post(
      "/api/v1/notifications/webhooks/test",
      { data: { channel: "slack", webhook_url: "http://169.254.169.254/" } },
    );
    expect(httpScheme.status()).toBe(400);

    // https but host not on the provider allowlist -> 400 (no connection made)
    const badHost = await page.request.post(
      "/api/v1/notifications/webhooks/test",
      { data: { channel: "slack", webhook_url: "https://evil.example.com/x" } },
    );
    expect(badHost.status()).toBe(400);

    // discord with wrong path -> 400
    const badPath = await page.request.post(
      "/api/v1/notifications/webhooks/test",
      { data: { channel: "discord", webhook_url: "https://discord.com/not-a-webhook" } },
    );
    expect(badPath.status()).toBe(400);
  });

  test("UTM link create rejects non-http(s) destination (open-redirect guard)", async ({
    page,
  }) => {
    for (const dest of ["javascript:alert(1)", "data:text/html,x", "ftp://x/y"]) {
      const res = await page.request.post("/api/v1/utm-links", {
        data: { destination_url: dest, utm_source: "x" },
      });
      expect(res.status(), `${dest} must be rejected`).toBe(400);
    }
    // A valid destination still works (and is cleaned up).
    const ok = await page.request.post("/api/v1/utm-links", {
      data: { destination_url: "https://example.com/landing", utm_source: "e2e" },
    });
    expect(ok.status()).toBe(201);
    const id = (await ok.json()).id;
    if (id) await page.request.delete(`/api/v1/utm-links/${id}`);
  });

  test("onboarding complete is idempotent (no duplicate profiles, nil subreddits ok)", async ({
    browser,
  }) => {
    const ctx = await browser.newContext({ baseURL: "http://localhost:13100" });
    const email = `idem-${Date.now()}@leadecho.test`;
    const reg = await ctx.request.post("/api/v1/auth/register", {
      data: { email, password: "idem-pass-123", name: "Idem" },
    });
    expect(reg.ok()).toBeTruthy();

    // No `subreddits` key on purpose — must not blow up a NOT NULL column.
    const payload = {
      product_name: "Idem Co",
      keywords: ["alpha-kw", "beta-kw"],
      platforms: ["reddit"],
    };
    const first = await ctx.request.post(
      "/api/v1/settings/onboarding/complete",
      { data: payload },
    );
    expect(first.status()).toBe(200);
    const profilesAfter1 = (
      await (await ctx.request.get("/api/v1/profiles")).json()
    ).length;
    expect(profilesAfter1).toBe(1);

    // Re-submitting must be a no-op, not create a second profile.
    const second = await ctx.request.post(
      "/api/v1/settings/onboarding/complete",
      { data: payload },
    );
    expect((await second.json()).already_completed).toBe(true);
    const profilesAfter2 = (
      await (await ctx.request.get("/api/v1/profiles")).json()
    ).length;
    expect(profilesAfter2, "no duplicate profile on re-complete").toBe(1);

    await ctx.close();
  });

  test("mention response includes awareness_level field", async ({ page }) => {
    const list = await (
      await page.request.get("/api/v1/mentions?limit=1")
    ).json();
    const items = Array.isArray(list) ? list : list.data ?? list.mentions;
    expect(items.length).toBeGreaterThan(0);
    expect(items[0]).toHaveProperty("awareness_level");
  });
});
