import { test, expect, type Page, type Locator } from "@playwright/test";

/**
 * LeadEcho "profiles" feature — pain-point / monitoring profile CRUD.
 *
 * Route: /_dashboard/profiles.tsx, rendered at /app/profiles.
 * Reads:
 *   GET  /api/v1/profiles            -> MonitoringProfile[]  (bare array, NOT paginated)
 * Mutates:
 *   POST   /api/v1/profiles          body { name, description, pain_points } -> profile (201)
 *   PUT    /api/v1/profiles/{id}     body { name?, description?, pain_points?, is_active? }
 *                                       -> profile (200)
 *   DELETE /api/v1/profiles/{id}     -> { status: "deleted" } (200)
 *   GET    /api/v1/profiles/{id}     -> profile (200)
 *
 * UI shape (from profiles.tsx):
 *   - Page heading <h2> "Pain-Point Profiles" (Text as="h2").
 *   - "Add Profile" card:
 *       * Name  <input placeholder="e.g. Slow CI/CD pipelines">
 *       * Description <input placeholder="Brief description of this pain point...">
 *       * Pain-point phrases <textarea placeholder="my builds take forever\n...one per line...">
 *       * A "Create Profile" <button> — DISABLED while the name input is empty
 *         (disabled={!name.trim() || addMutation.isPending}).
 *     On success the three fields clear and ["profiles"] is invalidated → list refetches.
 *   - "Active Profiles" card: CardDescription reads "<n> profile(s) configured"
 *     (singular only when n === 1), then either a "Loading..." line, the empty-state
 *     ("No profiles yet. Add one above to start semantic monitoring."), or one row per
 *     profile. Each row: the name (font-medium truncate span), then exactly TWO icon
 *     <button>s with NO accessible text — [0] = active/inactive toggle (Power/PowerOff),
 *     [1] = delete (Trash2) — plus an optional description <p> and pain_point Badges.
 *     Active rows use `bg-card`; inactive rows use `bg-muted opacity-60`.
 *
 * Seed (from the authenticated storageState's onboarding): a monitoring profile named
 * "Acme Analytics" (description "Self-serve product analytics for SaaS teams"). NOTE:
 * in this environment the pain-point embedder is a no-op, so the seeded profile's
 * `pain_points` come back as []; we therefore do NOT assert pain-point badges on seeds
 * (or on UI-created profiles). Because onboarding may have run more than once, the seed
 * name can appear MORE THAN ONCE — every assertion on the seed tolerates duplicates
 * (count >= 1 / .first()) and we NEVER mutate or delete a seeded profile.
 *
 * IDEMPOTENCY: every test that creates a profile uses a unique epoch-ms suffix so reruns
 * never collide, scopes its row assertions to that unique name, and deletes what it
 * created (via the UI where that IS the behavior under test, otherwise via the API) in a
 * finally block.
 */

const PROFILES_URL = "/profiles";
const SEED_NAME = "Acme Analytics";

interface ProfileDTO {
  id: string;
  name: string;
  description: string;
  pain_points: string[];
  is_active: boolean;
}

// Locate a profile row (the per-profile wrapper div) by its exact name text. The name
// renders in a font-medium/truncate <span>; the closest ancestor with rounded border-2
// is the row that also holds the toggle + delete buttons. Scoped to a UNIQUE name so it
// resolves to a single row (the seed name may repeat — never use this for the seed).
function profileRow(page: Page, name: string): Locator {
  return page
    .getByText(name, { exact: true })
    .locator(
      "xpath=ancestor::div[contains(@class,'rounded') and contains(@class,'border-2')][1]",
    );
}

// The two icon buttons in a row have no accessible name; [0] toggles active, [1] deletes.
function toggleButton(row: Locator): Locator {
  return row.getByRole("button").nth(0);
}
function deleteButton(row: Locator): Locator {
  return row.getByRole("button").nth(1);
}

async function fetchProfiles(page: Page): Promise<ProfileDTO[]> {
  const res = await page.request.get("/api/v1/profiles");
  expect(res.ok(), "/profiles endpoint ok").toBeTruthy();
  const list = (await res.json()) as ProfileDTO[];
  expect(Array.isArray(list), "profiles payload is an array").toBeTruthy();
  return list;
}

test.describe("profiles / pain-point profile CRUD", () => {
  test("page loads authenticated and renders the Add Profile form", async ({
    page,
  }) => {
    const resp = await page.goto(PROFILES_URL);
    expect(resp?.status(), "profiles HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Pain-Point Profiles", exact: true }),
    ).toBeVisible();

    // The add form is present with all three inputs and the create button.
    await expect(
      page.getByPlaceholder("e.g. Slow CI/CD pipelines"),
    ).toBeVisible();
    await expect(
      page.getByPlaceholder("Brief description of this pain point..."),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Create Profile" }),
    ).toBeVisible();
  });

  test("the seeded profile renders and the header count matches the API", async ({
    page,
  }) => {
    const list = await fetchProfiles(page);
    expect(list.length, "fixture seeds at least one profile").toBeGreaterThanOrEqual(1);

    const seedCount = list.filter((p) => p.name === SEED_NAME).length;
    expect(seedCount, `seeded profile "${SEED_NAME}" present in API`).toBeGreaterThanOrEqual(
      1,
    );

    await page.goto(PROFILES_URL);

    // "Loading..." line disappears once react-query resolves.
    await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
      timeout: 15_000,
    });

    // CardDescription reads "<n> profile(s) configured" — must equal payload length.
    const plural = list.length !== 1 ? "s" : "";
    await expect(
      page.getByText(`${list.length} profile${plural} configured`, {
        exact: true,
      }),
      "header count matches API",
    ).toBeVisible();

    // The seed name renders at least once (onboarding may have produced duplicates).
    await expect(
      page.getByText(SEED_NAME, { exact: true }).first(),
      `seeded profile "${SEED_NAME}" visible`,
    ).toBeVisible();
    await expect(
      page.getByText(SEED_NAME, { exact: true }),
      "seeded profile rendered for every API row of that name",
    ).toHaveCount(seedCount);
  });

  test("the Create button is disabled until a name is entered", async ({
    page,
  }) => {
    await page.goto(PROFILES_URL);

    const create = page.getByRole("button", { name: "Create Profile" });
    // Empty name → disabled (disabled={!name.trim() || ...}).
    await expect(create, "create disabled with empty name").toBeDisabled();

    // Whitespace-only name does NOT enable it (.trim() guard).
    const nameInput = page.getByPlaceholder("e.g. Slow CI/CD pipelines");
    await nameInput.fill("   ");
    await expect(create, "create still disabled for whitespace name").toBeDisabled();

    // A real name enables it.
    await nameInput.fill("temporary name");
    await expect(create, "create enabled once name has content").toBeEnabled();

    // Clearing it disables again — no profile was created (we never clicked).
    await nameInput.fill("");
    await expect(create, "create disabled again after clearing").toBeDisabled();
  });

  test("create a profile via the UI: it appears in the list, then delete it (self-cleaning)", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const name = `Acme ${uniqueSuffix}`;
    const description = `e2e profile ${uniqueSuffix}`;
    let createdId: string | null = null;

    await page.goto(PROFILES_URL);
    await expect(page.getByText("Loading...", { exact: true })).toHaveCount(0, {
      timeout: 15_000,
    });

    // Baseline count from the API before we add.
    const beforeCount = (await fetchProfiles(page)).length;

    try {
      // Fill the form and submit via the "Create Profile" button.
      const nameInput = page.getByPlaceholder("e.g. Slow CI/CD pipelines");
      const descInput = page.getByPlaceholder(
        "Brief description of this pain point...",
      );
      await nameInput.fill(name);
      await descInput.fill(description);

      const create = page.getByRole("button", { name: "Create Profile" });
      await expect(create).toBeEnabled();
      await create.click();

      // Real behavior: the new profile row appears after the mutation + refetch.
      const newRow = profileRow(page, name);
      await expect(newRow, "newly created profile row visible").toBeVisible({
        timeout: 15_000,
      });

      // Its description renders inside the row.
      await expect(
        newRow.getByText(description, { exact: true }),
        "new profile description visible",
      ).toBeVisible();

      // The inputs are cleared on success (onSuccess resets the fields).
      await expect(nameInput, "name input cleared after create").toHaveValue("");
      await expect(descInput, "description input cleared after create").toHaveValue(
        "",
      );

      // Header count incremented by exactly one.
      await expect(
        page.getByText(`${beforeCount + 1} profiles configured`, {
          exact: true,
        }),
        "header count incremented after create",
      ).toBeVisible({ timeout: 15_000 });

      // Confirm persistence + capture id via the API.
      const afterList = await fetchProfiles(page);
      const created = afterList.find((p) => p.name === name);
      expect(created, "created profile persisted in API").toBeTruthy();
      createdId = created!.id;
      expect(created!.is_active, "new profile is active by default").toBe(true);
      expect(created!.description, "description persisted").toBe(description);

      // ── Now DELETE it through the UI (the behavior under test) ──
      // Delete is the second icon button in the row.
      await deleteButton(newRow).click();

      // After deleteMutation + refetch the row is gone from the list.
      await expect(
        profileRow(page, name),
        "profile row removed after delete",
      ).toHaveCount(0, { timeout: 15_000 });

      // Header count returns to the baseline.
      await expect(
        page.getByText(
          `${beforeCount} profile${beforeCount !== 1 ? "s" : ""} configured`,
          { exact: true },
        ),
        "header count back to baseline after delete",
      ).toBeVisible({ timeout: 15_000 });

      // Confirm deletion via the API.
      const afterDelete = await fetchProfiles(page);
      expect(
        afterDelete.some((p) => p.name === name),
        "profile absent from API after delete",
      ).toBe(false);
      createdId = null; // cleaned up successfully
    } finally {
      // Safety net: if anything above failed after creation, remove the row so reruns
      // stay clean. (Look it up fresh in case createdId was never captured.)
      if (createdId) {
        await page.request.delete(`/api/v1/profiles/${createdId}`);
      } else {
        const leftover = (await fetchProfiles(page)).find((p) => p.name === name);
        if (leftover) await page.request.delete(`/api/v1/profiles/${leftover.id}`);
      }
    }
  });

  test("toggling a profile active/inactive reflects in the UI (self-cleaning)", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const name = `Acme toggle ${uniqueSuffix}`;

    // Create a dedicated profile via the API so we never mutate the shared seed.
    const createRes = await page.request.post("/api/v1/profiles", {
      data: { name, description: "toggle target", pain_points: [] },
    });
    expect(createRes.ok(), "create profile (API) ok").toBeTruthy();
    expect(createRes.status(), "create returns 201").toBe(201);
    const created = (await createRes.json()) as ProfileDTO;
    expect(created.is_active, "new profile starts active").toBe(true);
    const id = created.id;

    try {
      await page.goto(PROFILES_URL);
      const row = profileRow(page, name);
      await expect(row, "profile row visible").toBeVisible({ timeout: 15_000 });

      // Active rows render with bg-card (not the muted/opacity-60 inactive style).
      await expect(row, "row starts active").not.toHaveClass(/opacity-60/);

      // Click the toggle (first icon button) → updateProfile({ is_active:false }).
      await toggleButton(row).click();

      // After the mutation + refetch the row picks up the inactive (opacity-60) style.
      await expect(
        profileRow(page, name),
        "row becomes visually inactive",
      ).toHaveClass(/opacity-60/, { timeout: 15_000 });

      // Confirm the persisted flag via the API.
      const afterToggle = await page.request.get(`/api/v1/profiles/${id}`);
      expect(afterToggle.ok(), "GET profile by id ok").toBeTruthy();
      expect(
        ((await afterToggle.json()) as ProfileDTO).is_active,
        "is_active persisted false",
      ).toBe(false);

      // Toggle back on → row returns to the active style.
      await toggleButton(profileRow(page, name)).click();
      await expect(
        profileRow(page, name),
        "row becomes active again",
      ).not.toHaveClass(/opacity-60/, { timeout: 15_000 });
    } finally {
      await page.request.delete(`/api/v1/profiles/${id}`);
    }
  });

  test("empty-state placeholder renders when the workspace has no profiles (non-destructive)", async ({
    page,
  }) => {
    // We must NOT delete the seeded profile, so we exercise the empty-state branch by
    // intercepting the /profiles GET and returning an empty array for this page only.
    await page.route("**/api/v1/profiles", async (route) => {
      if (route.request().method() === "GET") {
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: "[]",
        });
        return;
      }
      await route.continue();
    });

    await page.goto(PROFILES_URL);

    await expect(
      page.getByText(
        "No profiles yet. Add one above to start semantic monitoring.",
        { exact: true },
      ),
      "empty-state placeholder visible",
    ).toBeVisible({ timeout: 15_000 });

    // The header count reads "0 profiles configured" (plural form, since 0 !== 1).
    await expect(
      page.getByText("0 profiles configured", { exact: true }),
      "header shows zero count",
    ).toBeVisible();

    await page.unroute("**/api/v1/profiles");
  });
});
