import { test, expect, Locator, Page, TestInfo } from '@playwright/test';

// Issue #71 — regression guard for the scroll/overflow defect PR #68 fixed in the
// contact EDIT modal (commit d8cbe35). The suspicion was that the SIBLING
// contact modals share the same pattern: a `fixed inset-0 flex items-center
// justify-center` overlay whose panel has no bounded max-height / overflow-y.
// When such a panel is taller than the viewport, flex-centering pushes the
// primary action (save / confirm) below the visible area and there is nothing
// to scroll — Playwright reports "element is outside of the viewport" and real
// users on a 1280x720 laptop lose the button.
//
// Every test below opens one contact-related modal with LONG content at exactly
// 1280x720 and asserts the primary action is EITHER fully visible without
// scrolling, OR reachable via a genuinely scrollable modal container. A modal
// that overflows with no scroll container FAILS, which is the point.

const VIEWPORT = { width: 1280, height: 720 };

// A realistically long Gemma research payload: the demo stub returns a one-line
// summary that trivially fits, so to exercise the overflow path we drive the
// SAME component with a long-but-plausible summary + many industry tags. Only
// the render content is controlled; the assertions stay strict.
const LONG_SUMMARY = Array.from({ length: 40 }, (_unused, i) =>
  `Absatz ${i + 1}: Die Weber Maschinenbau GmbH beliefert Industriekunden mit maßgeschneiderten PV-Großanlagen, Speichersystemen und Wartungsverträgen im Raum Stuttgart und darüber hinaus.`,
).join(' ');
const MANY_TAGS = Array.from({ length: 60 }, (_unused, i) => `#Branchenfeld${i + 1}`);

interface ReachMetrics {
  panelClientHeight: number;
  panelScrollHeight: number;
  scrollRoom: number;
  overflowY: string;
  actionTop: number;
  actionBottom: number;
  belowFoldPx: number;
  reachable: boolean;
  mode: 'no-scroll' | 'scrollable' | 'UNREACHABLE';
}

// panel = the single child div of the fixed overlay (the element #68 capped with
// max-h-[calc(100vh-2rem)] overflow-y-auto). action = the modal's primary button.
async function assertPrimaryActionReachable(
  overlay: Locator,
  action: Locator,
  label: string,
): Promise<ReachMetrics> {
  await expect(action, `${label}: primary action must exist`).toBeAttached();

  const before = await action.boundingBox();
  expect(before, `${label}: primary action has a bounding box`).not.toBeNull();

  const panel = overlay.locator('> div').first();
  const geom = await panel.evaluate((el) => {
    const style = getComputedStyle(el);
    return {
      overflowY: style.overflowY,
      clientHeight: el.clientHeight,
      scrollHeight: el.scrollHeight,
    };
  });

  const inViewportBefore = before!.y >= 0 && before!.y + before!.height <= VIEWPORT.height;

  let reachable = inViewportBefore;
  let mode: ReachMetrics['mode'] = 'no-scroll';

  if (!inViewportBefore) {
    // Off-screen: the ONLY acceptable state is a scrollable modal container that
    // can actually bring the action into view.
    const hasScrollContainer =
      (geom.overflowY === 'auto' || geom.overflowY === 'scroll') &&
      geom.scrollHeight - geom.clientHeight > 1;
    if (hasScrollContainer) {
      await action.scrollIntoViewIfNeeded();
      const after = await action.boundingBox();
      reachable = !!after && after.y >= 0 && after.y + after.height <= VIEWPORT.height;
      mode = reachable ? 'scrollable' : 'UNREACHABLE';
    } else {
      // No scrollable ancestor can reveal it: this is the #68 overflow pattern.
      reachable = false;
      mode = 'UNREACHABLE';
    }
  }

  const metrics: ReachMetrics = {
    panelClientHeight: geom.clientHeight,
    panelScrollHeight: geom.scrollHeight,
    scrollRoom: geom.scrollHeight - geom.clientHeight,
    overflowY: geom.overflowY,
    actionTop: before!.y,
    actionBottom: before!.y + before!.height,
    belowFoldPx: Math.max(0, Math.round(before!.y + before!.height - VIEWPORT.height)),
    reachable,
    mode,
  };

  expect(
    reachable,
    `${label}: primary action sits ${metrics.belowFoldPx}px below the ${VIEWPORT.height}px ` +
      `viewport (actionBottom=${Math.round(metrics.actionBottom)}) and is UNREACHABLE — ` +
      `panel overflow-y=${metrics.overflowY}, scrollRoom=${Math.round(metrics.scrollRoom)}px, ` +
      `clientH=${metrics.panelClientHeight} vs scrollH=${metrics.panelScrollHeight}. ` +
      `This is the #68 pattern: the modal needs a bounded max-height + overflow-y-auto.`,
  ).toBe(true);

  // Reachable is not enough — it must also be enabled and clickable.
  await expect(action, `${label}: primary action must be enabled`).toBeEnabled();

  return metrics;
}

async function report(testInfo: TestInfo, label: string, metrics: ReachMetrics) {
  await testInfo.attach(`${label}-metrics`, {
    body: JSON.stringify({ viewport: VIEWPORT, ...metrics }, null, 2),
    contentType: 'application/json',
  });
  // eslint-disable-next-line no-console
  console.log(
    `[modal-overflow] ${label}: mode=${metrics.mode} actionBottom=${Math.round(
      metrics.actionBottom,
    )} belowFold=${metrics.belowFoldPx}px overflowY=${metrics.overflowY} scrollRoom=${Math.round(
      metrics.scrollRoom,
    )}px clientH=${metrics.panelClientHeight} scrollH=${metrics.panelScrollHeight}`,
  );
}

async function gotoContacts(page: Page) {
  await page.setViewportSize(VIEWPORT);
  await page.goto('/contacts');
  await expect(page.getByRole('heading', { name: 'Kontakte & Leads' })).toBeVisible();
}

// The demo backend is a single shared in-memory DB across all parallel workers, and
// live-simulation.spec's sample-CSV import duplicates the seeded Weber/Mustermann/Becker
// contacts at runtime. So pre-seeded rows are NOT safe to target by name under full-suite
// parallelism. Each modal that needs an existing row creates its OWN uniquely-named contact
// through the (now-fixed) create modal — the same isolation discipline contacts.spec uses
// for "Erika Musterfrau" — then searches for it so the row selector resolves to exactly one.
async function seedUniqueContact(page: Page, firstName: string, lastName: string, email: string) {
  await gotoContacts(page);
  await page.getByRole('button', { name: /Kontakt anlegen/i }).click();
  const createOverlay = page.locator('div.fixed.inset-0', { hasText: 'Neuen Kontakt anlegen' });
  await createOverlay.locator('input[placeholder="Vorname"]').fill(firstName);
  await createOverlay.locator('input[placeholder="Nachname"]').fill(lastName);
  await createOverlay.locator('input[type="email"]').fill(email);
  await createOverlay.getByRole('button', { name: /Kontakt anlegen/i }).click();
  await expect(createOverlay).not.toBeVisible();

  await page.getByPlaceholder('Nach Name, E-Mail oder Zählernummer filtern...').fill(lastName);
  const row = page.getByRole('row', { name: new RegExp(`${firstName} ${lastName}`, 'i') });
  await expect(row).toBeVisible();
  return row;
}

test.describe('Contact modal overflow / save-button reachability (§3.1 / #71)', () => {
  test('Edit modal — "Änderungen speichern" reachable with the long edit form', async ({
    page,
  }, testInfo) => {
    const row = await seedUniqueContact(page, 'Edwin', 'EditEst', 'edwin.editest@overflow71.test');
    await row.getByRole('button', { name: /Bearbeiten/i }).click();

    const overlay = page.locator('div.fixed.inset-0', { hasText: 'Kontakt bearbeiten' });
    const action = overlay.getByRole('button', { name: /Änderungen speichern/i });
    const metrics = await assertPrimaryActionReachable(overlay, action, 'Edit modal');
    await report(testInfo, 'edit-modal', metrics);
  });

  test('Create modal — "Kontakt anlegen" submit reachable with the long create form', async ({
    page,
  }, testInfo) => {
    await gotoContacts(page);
    await page.getByRole('button', { name: /Kontakt anlegen/i }).click();
    await expect(page.getByText('Neuen Kontakt anlegen')).toBeVisible();

    const overlay = page.locator('div.fixed.inset-0', { hasText: 'Neuen Kontakt anlegen' });
    // Scoped to the overlay: only the form's submit button lives inside it.
    const action = overlay.getByRole('button', { name: /Kontakt anlegen/i });
    const metrics = await assertPrimaryActionReachable(overlay, action, 'Create modal');
    await report(testInfo, 'create-modal', metrics);
  });

  test('Gemma research modal — adopt button reachable with a long summary + many tags', async ({
    page,
  }, testInfo) => {
    // Drive the real component with a long-but-plausible research payload.
    await page.route('**/api/v1/ai/research/company', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          site_title: 'Edit Est GmbH — Referenzen',
          summary: LONG_SUMMARY,
          industry_tags: MANY_TAGS,
        }),
      }),
    );

    const row = await seedUniqueContact(
      page,
      'Rita',
      'ReschEst',
      'rita.reschest@overflow71.test',
    );
    await row.getByRole('button', { name: /KI-Recherche/i }).click();

    await expect(page.getByText('KI-Zusammenfassung')).toBeVisible();
    const overlay = page.locator('div.fixed.inset-0', { hasText: 'Recherche:' });
    const action = overlay.getByRole('button', { name: /Als Notiz am Kontakt übernehmen/i });
    const metrics = await assertPrimaryActionReachable(overlay, action, 'Research modal');
    await report(testInfo, 'research-modal', metrics);
  });

  test('CSV import modal — "Import jetzt ausführen" reachable with a loaded preview', async ({
    page,
  }, testInfo) => {
    await gotoContacts(page);
    await page.getByRole('button', { name: /CSV Import/i }).click();
    await expect(page.getByText(/CSV Kontakte- & Lead-Import/)).toBeVisible();

    const overlay = page.locator('div.fixed.inset-0', { hasText: 'CSV Kontakte' });
    // Load the sample so the submit becomes enabled AND the preview table grows the panel.
    await overlay.getByRole('button', { name: /Beispiel-CSV laden/i }).click();

    const action = overlay.getByRole('button', { name: /Import jetzt ausführen/i });
    const metrics = await assertPrimaryActionReachable(overlay, action, 'CSV import modal');
    await report(testInfo, 'csv-modal', metrics);
  });

  test('Delete confirmation modal — "Endgültig löschen" reachable', async ({ page }, testInfo) => {
    const row = await seedUniqueContact(page, 'Dina', 'DelEst', 'dina.delest@overflow71.test');
    await row.getByRole('button', { name: /Löschen/i }).click();

    const overlay = page.locator('div.fixed.inset-0', { hasText: 'Kontakt wirklich löschen' });
    const action = overlay.getByRole('button', { name: /Endgültig löschen/i });
    const metrics = await assertPrimaryActionReachable(overlay, action, 'Delete modal');
    await report(testInfo, 'delete-modal', metrics);
    // Abort — this only asserts reachability; we do not need to mutate the row further.
    await overlay.getByRole('button', { name: /Abbrechen/i }).click();
  });
});
