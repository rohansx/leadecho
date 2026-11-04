import { test, expect } from "@playwright/test";
import type { Page } from "@playwright/test";

/**
 * LeadEcho "knowledge-base" feature — the RAG document store (Documents CRUD).
 *
 * Route: /_dashboard/knowledge-base.tsx, rendered at /app/knowledge-base.
 * Reads:
 *   GET    /api/v1/documents          -> Document[]
 * Mutates (UI):
 *   POST   /api/v1/documents          body { title, content, source_url? } -> Document (201)
 *   DELETE /api/v1/documents/{id}     -> { status: "deleted" } (200)
 * Mutates (API only — NOT wired to any UI control in this route):
 *   PUT    /api/v1/documents/{id}     body { title?, content? } -> Document (200)
 *
 * The page renders:
 *   - heading "Knowledge Base" (<h2> via <Text as="h2">)
 *   - an "Add Document" / "Cancel" toggle button (top-right)
 *   - when toggled open, a form with:
 *       Input  placeholder "Document title"
 *       Input  placeholder "Source URL (optional)"
 *       textarea placeholder "Paste your document content here (markdown supported)..."
 *       a "Save Document" button (disabled until BOTH title & content are non-blank)
 *   - an empty-state card: "No documents yet. Add product pages, FAQs, or case studies..."
 *   - one <Card> per document: CardTitle = doc.title, a Badge with doc.content_type
 *     (defaults to "markdown"), an optional "<n> KB" file-size Badge, the (line-clamped)
 *     content, an optional source_url link, and a ghost icon Button (Trash2) that
 *     deletes the doc.
 *
 * IMPORTANT (read from the code):
 *   - The route only imports listDocuments / createDocument / deleteDocument. There is NO
 *     edit/update control in the JSX. The PUT /documents/{id} endpoint exists and the
 *     api.ts wrapper (updateDocument) is defined, but is unreachable from this page. So
 *     the "edit" case is exercised through the API, then verified to surface in the UI
 *     after a refetch (real behavior, not just HTTP 200).
 *   - Embedding / RAG requires AI keys (absent in this env). chunk_count is therefore not
 *     asserted — only the CRUD-visible fields are checked.
 *
 * IDEMPOTENCY: every doc created here carries a unique epoch-ms suffix and is deleted in a
 * finally block (via UI where the test is about UI deletion, otherwise via the API), so
 * reruns never collide and no rows are left behind. The shared/seeded workspace data is
 * never mutated.
 */

const KB_URL = "/knowledge-base";

interface DocumentDTO {
  id: string;
  title: string;
  content: string;
  content_type: string;
  source_url?: string;
  file_size_bytes?: number;
}

// Locate the document <Card> whose CardTitle text matches `title`, by walking up from
// the title element to the nearest rounded Card wrapper.
function docCard(page: Page, title: string) {
  return page
    .getByText(title, { exact: true })
    .locator("xpath=ancestor::div[contains(@class,'rounded')][1]");
}

// Delete a document straight through the authenticated request context — used by
// finally-blocks so cleanup is robust even if a UI assertion fails mid-test.
async function apiDelete(page: Page, id: string | undefined) {
  if (!id) return;
  await page.request.delete(`/api/v1/documents/${id}`).catch(() => {});
}

test.describe("knowledge-base / documents CRUD", () => {
  test("page loads authenticated and renders the Knowledge Base heading", async ({
    page,
  }) => {
    const resp = await page.goto(KB_URL);
    expect(resp?.status(), "knowledge-base HTTP status").toBeLessThan(400);

    // storageState carries the session → no bounce to login / onboarding.
    await expect(page).not.toHaveURL(/\/login/);
    await expect(page).not.toHaveURL(/\/onboarding/);

    await expect(
      page.getByRole("heading", { name: "Knowledge Base", exact: true }),
    ).toBeVisible();
    // The descriptive subheading from the JSX.
    await expect(
      page.getByText(
        "Upload product docs, FAQs, and case studies. Used by AI when drafting replies.",
        { exact: true },
      ),
    ).toBeVisible();
    // The toggle starts in the "add" state.
    await expect(
      page.getByRole("button", { name: "Add Document" }),
    ).toBeVisible();
  });

  test("add-document form toggles open and closed, and Save is gated on title+content", async ({
    page,
  }) => {
    await page.goto(KB_URL);

    const addToggle = page.getByRole("button", { name: "Add Document" });
    await expect(addToggle).toBeVisible();

    // Form inputs are hidden until the toggle is clicked.
    await expect(page.getByPlaceholder("Document title")).toHaveCount(0);

    await addToggle.click();

    // Now the form is open; the top-right toggle relabels to "Cancel".
    // (The open form ALSO has its own "Cancel" button, so scope to the first.)
    await expect(
      page.getByRole("button", { name: "Cancel" }).first(),
    ).toBeVisible();
    const titleInput = page.getByPlaceholder("Document title");
    const contentInput = page.getByPlaceholder(
      "Paste your document content here (markdown supported)...",
    );
    await expect(titleInput).toBeVisible();
    await expect(
      page.getByPlaceholder("Source URL (optional)"),
    ).toBeVisible();
    await expect(contentInput).toBeVisible();

    // Save is disabled with no input.
    const save = page.getByRole("button", { name: "Save Document" });
    await expect(save).toBeDisabled();

    // Still disabled with only a title (content required too).
    await titleInput.fill("only a title");
    await expect(save).toBeDisabled();

    // Enabled once both title and content are non-blank.
    await contentInput.fill("some content");
    await expect(save).toBeEnabled();

    // Clearing content re-disables it.
    await contentInput.fill("");
    await expect(save).toBeDisabled();

    // The top-right "Cancel" toggle closes the form (form inputs disappear).
    await page.getByRole("button", { name: "Cancel" }).first().click();
    await expect(page.getByPlaceholder("Document title")).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Add Document" }),
    ).toBeVisible();
  });

  test("creating a document through the form lists it, then deleting it removes it", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const title = `E2E KB Doc ${uniqueSuffix}`;
    const content = `Knowledge base body content ${uniqueSuffix}. This text is used by RAG when drafting replies.`;
    let createdId: string | undefined;

    // Capture the POST response so we learn the new doc's id for guaranteed cleanup.
    const createPromise = page.waitForResponse(
      (r) =>
        r.url().includes("/api/v1/documents") &&
        r.request().method() === "POST",
    );

    try {
      await page.goto(KB_URL);

      await page.getByRole("button", { name: "Add Document" }).click();
      await page.getByPlaceholder("Document title").fill(title);
      await page
        .getByPlaceholder(
          "Paste your document content here (markdown supported)...",
        )
        .fill(content);

      const save = page.getByRole("button", { name: "Save Document" });
      await expect(save).toBeEnabled();
      await save.click();

      const createResp = await createPromise;
      expect(createResp.status(), "POST /documents -> 201").toBe(201);
      createdId = ((await createResp.json()) as DocumentDTO).id;

      // The mutation's onSuccess invalidates ["documents"] → the new card renders.
      const card = docCard(page, title);
      await expect(card, "created doc card appears in the list").toBeVisible({
        timeout: 15_000,
      });
      await expect(
        card.getByText(content, { exact: false }),
        "card shows the document content",
      ).toBeVisible();
      // content_type defaults to "markdown" server-side and renders as a Badge.
      await expect(
        card.getByText("markdown", { exact: true }),
        "content_type badge defaults to markdown",
      ).toBeVisible();

      // The form closed itself on success (onSuccess sets showAdd=false).
      await expect(
        page.getByPlaceholder("Document title"),
        "add form closed after save",
      ).toHaveCount(0);
      await expect(
        page.getByRole("button", { name: "Add Document" }),
      ).toBeVisible();

      // ── Delete THROUGH the UI ──────────────────────────────────────────────
      // The delete control is the only Button inside the card (ghost icon, Trash2).
      const deleteResp = page.waitForResponse(
        (r) =>
          r.url().includes(`/api/v1/documents/${createdId}`) &&
          r.request().method() === "DELETE",
      );
      await card.getByRole("button").last().click();
      const dr = await deleteResp;
      expect(dr.status(), "DELETE /documents/{id} -> 200").toBe(200);

      // After deleteMutation invalidates ["documents"], the card is gone.
      await expect(
        page.getByText(title, { exact: true }),
        "deleted doc no longer listed",
      ).toHaveCount(0, { timeout: 15_000 });
      createdId = undefined; // already deleted; nothing for finally to clean up.
    } finally {
      await apiDelete(page, createdId);
    }
  });

  test("list reflects the /documents API payload and an optional source_url renders as a link", async ({
    page,
  }) => {
    const uniqueSuffix = Date.now();
    const title = `E2E KB Linked ${uniqueSuffix}`;
    const sourceUrl = `https://example.com/kb-${uniqueSuffix}`;
    let createdId: string | undefined;

    try {
      // Seed via the API (source_url is a UI input, but seeding keeps this test focused
      // on the LIST rendering, which is the assertion under test).
      const res = await page.request.post("/api/v1/documents", {
        data: {
          title,
          content: `Linked document content ${uniqueSuffix}`,
          source_url: sourceUrl,
        },
      });
      expect(res.status(), "seed POST /documents -> 201").toBe(201);
      const created = (await res.json()) as DocumentDTO;
      createdId = created.id;
      expect(created.source_url, "server persists source_url").toBe(sourceUrl);

      // The list endpoint must include our doc; the UI list must mirror that payload.
      const listRes = await page.request.get("/api/v1/documents");
      expect(listRes.ok(), "GET /documents ok").toBeTruthy();
      const docs = (await listRes.json()) as DocumentDTO[];
      expect(
        docs.some((d) => d.id === createdId),
        "seeded doc present in /documents payload",
      ).toBeTruthy();

      await page.goto(KB_URL);

      const card = docCard(page, title);
      await expect(card, "seeded doc card renders").toBeVisible({
        timeout: 15_000,
      });
      // The source_url renders as an anchor whose visible text is the URL itself.
      const link = card.getByRole("link", { name: sourceUrl });
      await expect(link).toBeVisible();
      await expect(link).toHaveAttribute("href", sourceUrl);
      await expect(link).toHaveAttribute("target", "_blank");
    } finally {
      await apiDelete(page, createdId);
    }
  });

  test("editing a document via PUT surfaces the new title in the list after refetch", async ({
    page,
  }) => {
    // NOTE: this route has no edit UI; PUT /documents/{id} is the documented update path.
    // We drive the update through the API and verify the UI reflects it on reload — i.e.
    // real persisted behavior, not just a 200.
    const uniqueSuffix = Date.now();
    const original = `E2E KB Editable ${uniqueSuffix}`;
    const edited = `E2E KB Edited ${uniqueSuffix}`;
    const editedContent = `Edited body content ${uniqueSuffix}`;
    let createdId: string | undefined;

    try {
      const createRes = await page.request.post("/api/v1/documents", {
        data: { title: original, content: `Original body ${uniqueSuffix}` },
      });
      expect(createRes.status(), "seed POST /documents -> 201").toBe(201);
      createdId = ((await createRes.json()) as DocumentDTO).id;

      // Confirm the original renders first.
      await page.goto(KB_URL);
      await expect(docCard(page, original)).toBeVisible({ timeout: 15_000 });

      // Update title + content via the API.
      const putRes = await page.request.put(
        `/api/v1/documents/${createdId}`,
        { data: { title: edited, content: editedContent } },
      );
      expect(putRes.status(), "PUT /documents/{id} -> 200").toBe(200);
      const updated = (await putRes.json()) as DocumentDTO;
      expect(updated.title, "PUT returns updated title").toBe(edited);
      expect(updated.content, "PUT returns updated content").toBe(editedContent);

      // Reload so the documents query refetches; the edited values must now render and
      // the old title must be gone.
      await page.goto(KB_URL);
      const editedCard = docCard(page, edited);
      await expect(editedCard, "edited title appears in the list").toBeVisible({
        timeout: 15_000,
      });
      await expect(
        editedCard.getByText(editedContent, { exact: false }),
        "edited content appears",
      ).toBeVisible();
      await expect(
        page.getByText(original, { exact: true }),
        "original title no longer listed",
      ).toHaveCount(0);
    } finally {
      await apiDelete(page, createdId);
    }
  });

  test("empty-state OR populated-state is consistent with the /documents payload", async ({
    page,
  }) => {
    // This workspace's document set is not part of the seeded fixture, so it may be empty
    // or populated depending on prior runs. Assert whichever branch the API dictates, so
    // the test is deterministic either way (covers the empty-state card explicitly).
    const listRes = await page.request.get("/api/v1/documents");
    expect(listRes.ok(), "GET /documents ok").toBeTruthy();
    const docs = (await listRes.json()) as DocumentDTO[];

    await page.goto(KB_URL);
    // Wait for the loading card to clear before asserting empty/populated.
    await expect(page.getByText("Loading documents...")).toHaveCount(0, {
      timeout: 15_000,
    });

    if (docs.length === 0) {
      await expect(
        page.getByText(
          "No documents yet. Add product pages, FAQs, or case studies to help AI draft better replies.",
          { exact: true },
        ),
        "empty-state card renders when there are no docs",
      ).toBeVisible();
    } else {
      // Empty-state must NOT show, and the first doc's title must render.
      await expect(
        page.getByText("No documents yet.", { exact: false }),
      ).toHaveCount(0);
      await expect(
        page.getByText(docs[0].title, { exact: true }).first(),
        "first doc from payload renders",
      ).toBeVisible({ timeout: 15_000 });
    }
  });
});
