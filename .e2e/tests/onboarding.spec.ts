import { test, expect, type Page } from "@playwright/test";

/**
 * LeadEcho "onboarding" feature — the 3-step setup wizard.
 *
 * Route: /routes/onboarding.tsx, rendered at /app/onboarding.
 *   IMPORTANT — route topology (verified against routeTree.gen.ts):
 *     "/onboarding" is a TOP-LEVEL route (sibling of "/_dashboard", not a child of it).
 *     => The _dashboard layout guard (which redirects users whose onboarding is
 *        NOT complete TO /onboarding) does NOT run on this page.
 *     => The onboarding component itself has NO "already complete" guard. Its ONLY
 *        redirect is `if (!loading && !user) navigate({ to: "/login" })`.
 *   Consequently, for the seeded user (onboarding already complete + authenticated),
 *   visiting /app/onboarding renders Step 1 — it does NOT bounce to /app/inbox and does
 *   NOT show a "completed" state. We assert that REAL behavior (see note in the task:
 *   the hypothesised redirect/completed-state does not exist in this build).
 *
 * API contract (api.ts + backend handler/onboarding.go + router.go):
 *   GET   /api/v1/settings/onboarding              -> { completed: bool, step: number }
 *   PATCH /api/v1/settings/onboarding   { step?, completed? } -> { completed, step }
 *   POST  /api/v1/settings/onboarding/analyze-url  { url } -> ProductAnalysis (200)
 *         ** With NO AI provider configured (GLM_API_KEY / OPENAI_API_KEY absent, as in
 *            this env) the handler short-circuits with HTTP 400 + body
 *            "no AI provider configured — set GLM_API_KEY or OPENAI_API_KEY". **
 *   POST  /api/v1/settings/onboarding/complete     { product_name, ... } ->
 *            { status:"completed", profile_id } (200). product_name is REQUIRED (400 if "").
 *
 * Wizard UI (from onboarding.tsx):
 *   Step 1 (step===1): label "Step 1 of 3", heading "Enter your product URL",
 *     <input placeholder="https://yourproduct.com">, primary <button> "Analyze & Set Up"
 *     (disabled while url is blank or analyze is pending; text -> "Analyzing your product..."
 *     while pending), an error <p> (text-destructive) showing analyzeMutation.error.message
 *     on failure, and a "Skip — set up manually" link that jumps straight to Step 2.
 *   Step 2 (step===2): label "Step 2 of 3", heading "Review & customize", inputs for
 *     Product name + Description, chip editors for Pain points / Monitoring keywords /
 *     (when reddit selected) Subreddits — each with an "Add ..." placeholder input, an "Add"
 *     <button>, and per-chip "×" remove buttons — a PLATFORM toggle row (Reddit, Hacker News,
 *     Twitter / X, LinkedIn, Dev.to, Lobsters, Indie Hackers; reddit+hackernews preselected),
 *     and a primary "Deploy Agents" <button> (disabled until product name is non-blank;
 *     text -> "Deploying..." while pending).
 *   Step 3 (step===3): heading "Deploying your agents", an animated checklist (DEPLOY_STEPS),
 *     and once finished an "Open Inbox →" <button> that navigates to /inbox.
 *
 * MUTATION SAFETY / IDEMPOTENCY:
 *   The real POST /complete creates a monitoring profile + keywords in the SHARED workspace,
 *   has no UI/API delete for the wizard, and is not idempotent. So we NEVER drive the real
 *   complete endpoint from the UI. The deploy-animation test stubs the /complete response via
 *   page.route so no server state is mutated. Steps 1 & 2 are otherwise pure client state, and
 *   the chip editors use an epoch-ms suffix so nothing we type could ever collide on reruns.
 */

const ONBOARDING_URL = "/onboarding";

interface OnboardingStatusDTO {
  completed: boolean;
  step: number;
}

async function fetchStatus(page: Page): Promise<OnboardingStatusDTO> {
  const res = await page.request.get("/api/v1/settings/onboarding");
  expect(res.ok(), "/settings/onboarding GET ok").toBeTruthy();
  return (await res.json()) as OnboardingStatusDTO;
}

// The wizard redirects already-completed users to /inbox, and the shared test
// user is completed. Present the status endpoint as "not completed" (GET only)
// so the wizard UI renders. Returns a switch to restore the real status.
async function mockIncompleteOnboarding(page: Page): Promise<() => void> {
  let incomplete = true;
  await page.route("**/api/v1/settings/onboarding", async (route) => {
    if (incomplete && route.request().method() === "GET") {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ completed: false, step: 1 }),
      });
      return;
    }
    await route.continue();
  });
  return () => {
    incomplete = false;
  };
}

async function aiConfigured(page: Page): Promise<boolean> {
  const r = await page.request.get("/api/v1/llm/config");
  if (!r.ok()) return false;
  const body = (await r.json()) as { health?: { chat_ok?: boolean } };
  return body.health?.chat_ok === true;
}

// Reach Step 2 deterministically from a fresh Step-1 render via the "Skip — set up
// manually" link (no API call, no AI needed). Returns once the Step 2 heading is visible.
async function gotoStep2(page: Page): Promise<() => void> {
  const restore = await mockIncompleteOnboarding(page);
  await page.goto(ONBOARDING_URL);
  await expect(
    page.getByRole("heading", { name: "Enter your product URL" }),
  ).toBeVisible({ timeout: 15_000 });
  await page.getByRole("button", { name: "Skip — set up manually" }).click();
  await expect(
    page.getByRole("heading", { name: "Review & customize" }),
  ).toBeVisible({ timeout: 15_000 });
  return restore;
}

test.describe("onboarding / setup wizard", () => {
  test("a completed user is redirected from /onboarding to the inbox", async ({ page }) => {
    const status = await fetchStatus(page);
    expect(status.completed, "seeded user has completed onboarding").toBe(true);

    await page.goto(ONBOARDING_URL);
    await expect(page).toHaveURL(/\/inbox/, { timeout: 15_000 });
  });

  test("an incomplete user sees Step 1 of the wizard", async ({ page }) => {
    await mockIncompleteOnboarding(page);
    const resp = await page.goto(ONBOARDING_URL);
    expect(resp?.status(), "onboarding HTTP status").toBeLessThan(400);
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).toHaveURL(/\/onboarding/);

    await expect(page.getByText("Step 1 of 3", { exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Enter your product URL" })).toBeVisible();
    await expect(page.getByPlaceholder("https://yourproduct.com")).toBeVisible();
    await expect(page.getByRole("button", { name: "Skip — set up manually" })).toBeVisible();
  });

  test("the 'Analyze & Set Up' button is disabled until a URL is entered", async ({
    page,
  }) => {
    await mockIncompleteOnboarding(page);
    await page.goto(ONBOARDING_URL);
    const analyze = page.getByRole("button", { name: "Analyze & Set Up" });

    // disabled={!url.trim() || pending} — blank url => disabled.
    await expect(analyze, "analyze disabled with empty url").toBeDisabled();

    const urlInput = page.getByPlaceholder("https://yourproduct.com");
    // Whitespace-only does NOT enable it (.trim() guard).
    await urlInput.fill("   ");
    await expect(analyze, "analyze still disabled for whitespace url").toBeDisabled();

    // A real value enables it.
    await urlInput.fill("https://example.com");
    await expect(analyze, "analyze enabled once url has content").toBeEnabled();

    // Clearing disables again.
    await urlInput.fill("");
    await expect(analyze, "analyze disabled again after clearing").toBeDisabled();
  });

  test("analyze-url with NO AI provider configured fails gracefully and surfaces the error (no AI output required)", async ({
    page,
  }) => {
    test.skip(
      await aiConfigured(page),
      "an AI provider is configured in this environment; the no-AI failure path is not reachable",
    );
    await mockIncompleteOnboarding(page);
    // First confirm the backend contract directly: with no GLM/OpenAI key the endpoint
    // returns 400 with the "no AI provider configured" message — i.e. analysis output is
    // not expected in this environment.
    const apiRes = await page.request.post(
      "/api/v1/settings/onboarding/analyze-url",
      { data: { url: "https://example.com" } },
    );
    expect(apiRes.status(), "analyze-url returns 400 without AI keys").toBe(400);
    expect(
      await apiRes.text(),
      "error body mentions missing AI provider",
    ).toContain("no AI provider configured");

    // Now exercise the same path through the UI and assert the wizard handles it: it
    // STAYS on Step 1 (never advances to "Review & customize") and renders the error.
    await page.goto(ONBOARDING_URL);
    const urlInput = page.getByPlaceholder("https://yourproduct.com");
    await urlInput.fill("https://example.com");

    const analyze = page.getByRole("button", { name: "Analyze & Set Up" });
    await expect(analyze).toBeEnabled();
    await analyze.click();

    // analyzeMutation.isError -> red <p> showing error.message (which is "400: <body>").
    const err = page.locator("p.text-destructive");
    await expect(err, "an analyze error message is shown").toBeVisible({
      timeout: 15_000,
    });
    await expect(err, "error text reflects the missing-AI-provider failure").toContainText(
      "no AI provider configured",
    );

    // It did NOT advance to Step 2.
    await expect(
      page.getByRole("heading", { name: "Review & customize" }),
      "wizard stays on Step 1 after a failed analyze",
    ).toHaveCount(0);
    await expect(
      page.getByRole("heading", { name: "Enter your product URL" }),
    ).toBeVisible();
  });

  test("'Skip — set up manually' advances to Step 2 with the manual review form (no AI)", async ({
    page,
  }) => {
    await gotoStep2(page);

    await expect(page.getByText("Step 2 of 3", { exact: true })).toBeVisible();

    // Both top inputs render empty when skipping (no AI prefill).
    const productName = page.locator("input").first(); // Product name is the first input on Step 2
    await expect(productName).toBeVisible();
    await expect(productName, "product name input starts empty on skip").toHaveValue("");

    // The chip-editor "Add ..." inputs and the Deploy button are present.
    await expect(page.getByPlaceholder("Add pain point...")).toBeVisible();
    await expect(page.getByPlaceholder("Add keyword...")).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Deploy Agents|Add at least one keyword/ }),
    ).toBeVisible();

    // Default platforms are hackernews + devto, so the Subreddits editor (shown
    // only when reddit is selected) starts hidden. With no keyword the CTA is
    // disabled and says why.
    await expect(page.getByPlaceholder("Add subreddit...")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Add at least one keyword" })).toBeDisabled();
  });

  test("Step 2 is fully editable client-side: chips add/remove, platform toggle, and Deploy gating", async ({
    page,
  }) => {
    const sfx = Date.now();
    await gotoStep2(page);

    // ── Deploy gating: needs a non-blank product name AND >= 1 keyword ──
    const deploy = page.getByRole("button", { name: /Deploy Agents|Add at least one keyword/ });
    await expect(deploy, "deploy disabled with empty product name").toBeDisabled();

    const productName = page.locator("input").first();
    await productName.fill("   "); // whitespace only
    await expect(deploy, "deploy still disabled for whitespace name").toBeDisabled();
    await productName.fill(`Acme ${sfx}`);
    await expect(deploy, "deploy still disabled without a keyword").toBeDisabled();

    // ── Keyword chip: add via the "Add" button, then remove via the × ──
    const kw = `kw-${sfx}`;
    await page.getByPlaceholder("Add keyword...").fill(kw);
    // Scope the "Add" button to the keyword input's own row (its immediate parent
    // div). A loose div.filter({has: input}) matches every ancestor div and so
    // resolves to all three "Add" buttons (keyword/pain-point/subreddit).
    await page
      .getByPlaceholder("Add keyword...")
      .locator("..")
      .getByRole("button", { name: "Add", exact: true })
      .click();
    const kwChip = page.locator("span").filter({ hasText: kw });
    await expect(kwChip, "keyword chip added").toBeVisible();
    await expect(
      page.getByRole("button", { name: "Deploy Agents" }),
      "deploy enabled once name + keyword present",
    ).toBeEnabled();

    // Remove the chip via its × button (the only button inside the chip span).
    await kwChip.getByRole("button").click();
    await expect(
      page.locator("span").filter({ hasText: kw }),
      "keyword chip removed",
    ).toHaveCount(0);
    await expect(deploy, "deploy disabled again with no keyword").toBeDisabled();

    // ── Pain-point chip: add via Enter key on its input ──
    const pp = `pain-${sfx}`;
    const ppInput = page.getByPlaceholder("Add pain point...");
    await ppInput.fill(pp);
    await ppInput.press("Enter");
    const ppChip = page.locator("span").filter({ hasText: pp });
    await expect(ppChip, "pain-point chip added via Enter").toBeVisible();
    // Input clears after add (addChip resets the input).
    await expect(ppInput, "pain-point input cleared after add").toHaveValue("");

    // ── Platform toggle: reddit is off by default; turning it ON reveals Subreddits ──
    await expect(
      page.getByPlaceholder("Add subreddit..."),
      "subreddit editor hidden while reddit unselected",
    ).toHaveCount(0);
    await page.getByRole("button", { name: /^Reddit/ }).click();
    await expect(
      page.getByPlaceholder("Add subreddit..."),
      "subreddit editor visible once reddit selected",
    ).toBeVisible();
    await page.getByRole("button", { name: /^Reddit/ }).click();
    await expect(
      page.getByPlaceholder("Add subreddit..."),
      "subreddit editor hidden again",
    ).toHaveCount(0);
  });

  test("Deploy runs the agent-deployment animation and lands on an 'Open Inbox' CTA (complete stubbed — no shared state mutated)", async ({
    page,
  }) => {
    // Stub the real /complete POST so we never create a profile/keywords in the shared
    // workspace. We still drive the entire UI flow (the mutation onSuccess animation).
    let completeCalls = 0;
    await page.route(
      "**/api/v1/settings/onboarding/complete",
      async (route) => {
        if (route.request().method() === "POST") {
          completeCalls += 1;
          await route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({ status: "completed", profile_id: "stub-profile" }),
          });
          return;
        }
        await route.continue();
      },
    );

    try {
      const restoreStatus = await gotoStep2(page);

      // Give it a product name so Deploy is enabled, then click.
      await page.locator("input").first().fill(`Acme Deploy ${Date.now()}`);
      await page.getByPlaceholder("Add keyword...").fill("e2e-keyword");
      await page.getByPlaceholder("Add keyword...").press("Enter");
      const deploy = page.getByRole("button", { name: /Deploy Agents|Add at least one keyword/ });
      await expect(deploy).toBeEnabled();
      await deploy.click();

      // onSuccess -> Step 3.
      await expect(
        page.getByRole("heading", { name: "Deploying your agents" }),
        "advanced to the deploy step",
      ).toBeVisible({ timeout: 15_000 });
      expect(completeCalls, "complete endpoint was invoked exactly once").toBe(1);

      // First checklist line shows immediately (deployStep starts at 0).
      await expect(
        page.getByText("Creating monitoring profile...", { exact: true }),
      ).toBeVisible();

      // The animation advances on an 800ms interval to the final line; the
      // "Open Inbox →" CTA only appears once the checklist is complete.
      await expect(
        page.getByText("Your agents are live!", { exact: true }),
        "final deploy line reached",
      ).toBeVisible({ timeout: 15_000 });
      const openInbox = page.getByRole("button", { name: "Open Inbox →" });
      await expect(openInbox, "Open Inbox CTA visible after deploy finishes").toBeVisible({
        timeout: 15_000,
      });

      // Clicking it navigates into the dashboard inbox.
      // Real status (completed) applies again so the dashboard guard lets us in.
      restoreStatus();
      await openInbox.click();
      await expect(page, "navigated to /app/inbox").toHaveURL(/\/inbox/, {
        timeout: 15_000,
      });
    } finally {
      await page.unroute("**/api/v1/settings/onboarding/complete");
    }
  });

  test("onboarding status PATCH round-trips (step persisted) and the workspace stays 'completed'", async ({
    page,
  }) => {
    // The seed is completed; capture its current step so we restore it afterward.
    const before = await fetchStatus(page);
    expect(before.completed, "precondition: onboarding completed").toBe(true);
    const originalStep = before.step;

    // Pick a step value distinct from the current one so the round-trip is observable.
    const targetStep = originalStep === 2 ? 3 : 2;

    try {
      const patchRes = await page.request.patch("/api/v1/settings/onboarding", {
        data: { step: targetStep },
      });
      expect(patchRes.ok(), "PATCH onboarding ok").toBeTruthy();
      const patched = (await patchRes.json()) as OnboardingStatusDTO;
      expect(patched.step, "PATCH echoes new step").toBe(targetStep);
      // PATCH only touched step — completed must be untouched (still true).
      expect(patched.completed, "completed flag preserved across step patch").toBe(true);

      // Re-read to confirm persistence.
      const reread = await fetchStatus(page);
      expect(reread.step, "step persisted").toBe(targetStep);
      expect(reread.completed, "still completed after step patch").toBe(true);
    } finally {
      // Restore the original step so the fixture state is left exactly as we found it.
      await page.request.patch("/api/v1/settings/onboarding", {
        data: { step: originalStep, completed: true },
      });
    }
  });
});
