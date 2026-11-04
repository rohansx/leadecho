import { test, expect } from "@playwright/test";
import { execSync } from "node:child_process";

/**
 * LeadEcho "pipeline" feature — the leads kanban board.
 *
 * Route: /_dashboard/pipeline.tsx, rendered at /app/pipeline.
 * Reads:
 *   GET  /api/v1/leads?limit=100   -> PaginatedResponse<Lead> ({ data, total, ... })
 *   GET  /api/v1/leads/counts      -> [{ status, count }]   (status === lead stage)
 * Mutates:
 *   PATCH /api/v1/leads/{id}/stage  body { stage }  -> updated Lead
 *
 * The board renders FIVE fixed columns in this order, each with a Badge label and a
 * numeric count: Prospect, Qualified, Engaged, Converted, Lost. An empty column shows
 * a dashed "No leads" placeholder. Each lead card shows contact_name (falling back to
 * username/"Unknown"), an optional platform Badge, company, "$<value>/yr", tags, notes,
 * and a "Next stage" <Button> whose visible label is the literal next stage key
 * (prospect->qualified->engaged->converted). "converted" and "lost" cards have NO
 * next-stage button (nextStage maps them to null).
 *
 * The authenticated storageState seeds exactly three leads for this workspace, one each
 * in prospect / qualified / engaged ("Dana Prospect", "Quinn Qualified", "Evan Engaged",
 * all on hackernews), so Converted and Lost start empty.
 *
 * IDEMPOTENCY NOTE: the backend exposes NO DELETE for leads (router only has
 * GET/POST/PATCH). The mutation test therefore creates its own uniquely-named lead via
 * the API, drives the UI stage transition on THAT card, and resets its stage back via
 * the API so reruns never collide. The created lead row is not deletable, so each run
 * leaves one extra prospect lead behind — but uniquely suffixed, so the assertions stay
 * deterministic. The shared seeded leads are never mutated.
 */

const PIPELINE_URL = "/pipeline";

// The environment no longer ships fixture leads; insert the three the specs rely
// on (idempotent via NOT EXISTS) for the e2e test user's workspace.
function seedLeads(): void {
  const rows: Array<[string, string, string, number]> = [
    ["Dana Prospect", "prospect", "Acme Prospect Co", 5000],
    ["Quinn Qualified", "qualified", "Quill Inc", 8000],
    ["Evan Engaged", "engaged", "GreenLeaf", 12000],
  ];
  for (const [name, stage, company, value] of rows) {
    execSync(
      `docker exec leadecho-e2e-postgres psql -U leadecho -d leadecho_dev -c ` +
        `"INSERT INTO leads (workspace_id, stage, contact_name, company, platform, estimated_value) ` +
        `SELECT u.workspace_id,'${stage}','${name}','${company}','hackernews',${value} FROM users u ` +
        `WHERE u.email='e2e-tester@leadecho.test' AND NOT EXISTS ` +
        `(SELECT 1 FROM leads l WHERE l.workspace_id=u.workspace_id AND l.contact_name='${name}');"`,
      { stdio: "pipe" },
    );
  }
}

test.beforeAll(() => seedLeads());

// Column labels exactly as rendered by `stages` in pipeline.tsx.
const STAGE_LABELS = ["Prospect", "Qualified", "Engaged", "Converted", "Lost"];

// Locate a column by its header Badge label, then return the column wrapper div
// (min-w-[256px]) that holds both the header and the lead-cards list.
function column(page: import("@playwright/test").Page, label: string) {
  return page
    .getByText(label, { exact: true })
    .locator("xpath=ancestor::div[contains(@class,'min-w-[256px]')][1]");
}

test.describe("pipeline / leads kanban", () => {
  test("page loads authenticated and renders the Lead Pipeline heading", async ({
    page,
  }) => {
    const resp = await page.goto(PIPELINE_URL);
    expect(resp?.status(), "pipeline HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Lead Pipeline", exact: true }),
    ).toBeVisible();
  });

  test("all five stage columns render", async ({ page }) => {
    await page.goto(PIPELINE_URL);

    // Wait for the board to finish its initial react-query load (the "Loading
    // pipeline..." card is gone once data resolves).
    await expect(page.getByText("Loading pipeline...")).toHaveCount(0, {
      timeout: 15_000,
    });

    for (const label of STAGE_LABELS) {
      await expect(
        page.getByText(label, { exact: true }),
        `column header "${label}" visible`,
      ).toBeVisible();
    }
  });

  test("header lead count matches the /leads payload and seeded cards render", async ({
    page,
  }) => {
    // Read the raw payload through the authenticated request context.
    const res = await page.request.get("/api/v1/leads?limit=100");
    expect(res.ok(), "/leads endpoint ok").toBeTruthy();
    const payload = (await res.json()) as {
      data: { id: string; stage: string; contact_name: string | null }[];
      total: number;
    };
    const total = payload.data.length;
    expect(total, "fixture seeds at least the 3 base leads").toBeGreaterThanOrEqual(
      3,
    );

    await page.goto(PIPELINE_URL);

    // Header Badge reads "<n> leads" and must equal the payload length.
    await expect(
      page.getByText(`${total} leads`, { exact: true }),
      "header lead-count badge matches API",
    ).toBeVisible();

    // The three seeded leads render as cards under their respective columns.
    await expect(
      column(page, "Prospect").getByText("Dana Prospect", { exact: true }),
      "Dana Prospect under Prospect",
    ).toBeVisible();
    await expect(
      column(page, "Qualified").getByText("Quinn Qualified", { exact: true }),
      "Quinn Qualified under Qualified",
    ).toBeVisible();
    await expect(
      column(page, "Engaged").getByText("Evan Engaged", { exact: true }),
      "Evan Engaged under Engaged",
    ).toBeVisible();
  });

  test("populated card shows platform, company and estimated value", async ({
    page,
  }) => {
    await page.goto(PIPELINE_URL);

    // Scope to the seeded "Evan Engaged" card (engaged / hackernews / GreenLeaf / 12000).
    const card = column(page, "Engaged")
      .locator("xpath=.//*[contains(@class,'rounded')]") // any descendant Card
      .filter({ hasText: "Evan Engaged" })
      .first();
    await expect(card).toBeVisible();

    await expect(card.getByText("hackernews", { exact: true })).toBeVisible();
    await expect(card.getByText("GreenLeaf", { exact: true })).toBeVisible();
    // estimated_value 12000 -> toLocaleString() -> "12,000", rendered "$12,000/yr".
    await expect(card.getByText(/\$12,000\/yr/)).toBeVisible();
  });

  test("counts endpoint contract: statuses are valid stages, counts non-negative", async ({
    page,
  }) => {
    const res = await page.request.get("/api/v1/leads/counts");
    expect(res.ok(), "/leads/counts endpoint ok").toBeTruthy();
    const counts = (await res.json()) as { status: string; count: number }[];

    expect(Array.isArray(counts), "counts is an array").toBeTruthy();
    for (const c of counts) {
      expect(STAGE_LABELS.map((s) => s.toLowerCase())).toContain(c.status);
      expect(Number.isInteger(c.count), `count for ${c.status} is int`).toBeTruthy();
      expect(c.count, `count for ${c.status} >= 0`).toBeGreaterThanOrEqual(0);
    }

    // The UI renders the count value next to each stage Badge. Verify the Prospect
    // column's rendered count equals the API count (or the rendered cards as fallback,
    // exactly mirroring `countMap.get(stage) ?? stageLeads.length` in the component).
    const prospectCount =
      counts.find((c) => c.status === "prospect")?.count ?? 0;
    await page.goto(PIPELINE_URL);
    const prospectHeader = column(page, "Prospect");
    // The count span sits right after the "Prospect" badge inside the column header.
    await expect(
      prospectHeader.getByText(String(prospectCount), { exact: true }).first(),
      "Prospect column count reflects API",
    ).toBeVisible();
  });

  test("Converted and Lost columns are empty (placeholder) on the seeded fixture", async ({
    page,
  }) => {
    // Confirm via API that no seeded lead occupies converted/lost before asserting UI.
    const res = await page.request.get("/api/v1/leads?limit=100");
    const payload = (await res.json()) as { data: { stage: string }[] };
    const convertedLeads = payload.data.filter((l) => l.stage === "converted");
    const lostLeads = payload.data.filter((l) => l.stage === "lost");

    await page.goto(PIPELINE_URL);
    await expect(page.getByText("Loading pipeline...")).toHaveCount(0, {
      timeout: 15_000,
    });

    if (convertedLeads.length === 0) {
      await expect(
        column(page, "Converted").getByText("No leads", { exact: true }),
        "Converted column shows empty placeholder",
      ).toBeVisible();
    }
    if (lostLeads.length === 0) {
      await expect(
        column(page, "Lost").getByText("No leads", { exact: true }),
        "Lost column shows empty placeholder",
      ).toBeVisible();
    }
  });

  test("moving a lead to the next stage updates the board (self-resetting)", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const name = `E2E Mover ${uniqueSuffix}`;

    // Create a dedicated lead in `prospect` so we never mutate the shared seed data.
    const createRes = await page.request.post("/api/v1/leads", {
      data: {
        stage: "prospect",
        contact_name: name,
        company: `E2E Co ${uniqueSuffix}`,
        platform: "reddit",
      },
    });
    expect(createRes.ok(), "create lead ok").toBeTruthy();
    const created = (await createRes.json()) as { id: string; stage: string };
    expect(created.stage).toBe("prospect");
    const leadId = created.id;

    try {
      await page.goto(PIPELINE_URL);
      await expect(page.getByText("Loading pipeline...")).toHaveCount(0, {
        timeout: 15_000,
      });

      // Our new card starts in the Prospect column.
      const prospectCol = column(page, "Prospect");
      const movingCard = prospectCol
        .locator("xpath=.//*[contains(@class,'rounded')]")
        .filter({ hasText: name })
        .first();
      await expect(movingCard, "new lead card in Prospect").toBeVisible({
        timeout: 15_000,
      });

      // The next-stage button on a prospect card is labelled "qualified".
      // Click it → moveMutation PATCHes /leads/{id}/stage then invalidates queries.
      await movingCard.getByRole("button", { name: "qualified" }).click();

      // After the mutation + refetch, the card must appear under the Qualified column
      // and no longer under Prospect (real behavior, not just a 200).
      const qualifiedCol = column(page, "Qualified");
      await expect(
        qualifiedCol
          .locator("xpath=.//*[contains(@class,'rounded')]")
          .filter({ hasText: name })
          .first(),
        "moved card now in Qualified",
      ).toBeVisible({ timeout: 15_000 });
      await expect(
        prospectCol.getByText(name, { exact: true }),
        "moved card no longer in Prospect",
      ).toHaveCount(0, { timeout: 15_000 });

      // Confirm the persisted state via the API too.
      const after = await page.request.get(`/api/v1/leads/${leadId}`);
      expect(after.ok()).toBeTruthy();
      expect(((await after.json()) as { stage: string }).stage).toBe(
        "qualified",
      );
    } finally {
      // Reset stage back to prospect so reruns find a clean prospect card.
      // (No DELETE endpoint exists for leads, so the row itself cannot be removed.)
      await page.request.patch(`/api/v1/leads/${leadId}/stage`, {
        data: { stage: "prospect" },
      });
    }
  });
});
