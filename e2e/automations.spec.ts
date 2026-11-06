import { test, expect } from '@playwright/test';

test.describe('Automations & Workflows (§6.4)', () => {
  test('should display active workflow routines and open creation modal', async ({ page }) => {
    await page.goto('/automations');

    await expect(page.getByRole('heading', { name: /Automationen & Workflows/i })).toBeVisible();
    await expect(page.getByText('Erstkontakt & Qualifizierung (Neuer Lead)')).toBeVisible();
    await expect(page.getByText('Deal-Abschluss Routine (Phase: WON)')).toBeVisible();

    // Open creation modal
    await page.getByRole('button', { name: /Neue Automatisierung anlegen/i }).click();
    await expect(page.getByRole('heading', { name: /Neue Automatisierungs-Routine anlegen/i })).toBeVisible();
    await page.getByPlaceholder(/z.B. PV-Lead Qualifizierung & Angebot/i).fill('PV-Express Onboarding Routine');
    await page.getByRole('button', { name: /Abbrechen/i }).click();
  });

  test('should switch to Workflow-Runs and allow HITL step approval', async ({ page }) => {
    await page.goto('/automations');

    // Switch to Runs tab
    await page.getByRole('button', { name: /Workflow-Läufe & HITL Freigaben/i }).click();
    const weberRow = page.locator('tr').filter({ hasText: 'Dr. Michael Weber' });
    await expect(weberRow.getByText('Wartet auf Freigabe')).toBeVisible();

    // Precondition for issue #63: step 2 of the seeded workflow is DRAFT_EMAIL and
    // AIDrafter.DraftEmail deliberately refuses to invent a sender account
    // (internal/core/automation/draft.go:43-50). The test therefore brings its own
    // active email account through the real API (POST /api/v1/emails/accounts,
    // internal/server/router.go:523) instead of seeding one into the demo dataset.
    const cookies = await page.context().cookies();
    const csrfToken = cookies.find((c) => c.name === 'csrf_token')?.value;
    expect(csrfToken, 'csrf_token cookie missing from authenticated context').toBeTruthy();

    const accountRes = await page.request.post('/api/v1/emails/accounts', {
      headers: { 'X-CSRF-Token': csrfToken ?? '' },
      data: {
        name: 'E2E HITL Absendekonto',
        email_address: 'e2e-hitl@openlocalcrm.local',
        provider: 'IMAP',
        imap_host: 'imap.openlocalcrm.local',
        imap_port: 993,
        smtp_host: 'smtp.openlocalcrm.local',
        smtp_port: 587,
        username: 'e2e-hitl@openlocalcrm.local',
        is_active: true,
        account_type: 'team',
      },
    });
    expect(accountRes.status(), await accountRes.text()).toBe(201);
    expect((await accountRes.json()).is_active).toBe(true);

    // Approve step 2 of the Weber run.
    const approveBtn = weberRow.getByRole('button', { name: /Schritt freigeben/i });
    await expect(approveBtn).toBeVisible();
    const [approveResponse] = await Promise.all([
      page.waitForResponse((res) => res.url().includes('/approve') && res.request().method() === 'POST'),
      approveBtn.click(),
    ]);

    // Real success copy from web/src/pages/AutomationsPage.tsx:117: once the approved
    // DRAFT_EMAIL step executes, step 3 (CREATE_TASK) is not a HITL pause — the run
    // executes it immediately and completes. AutomationsPage.tsx:114
    // ("Schritt freigegeben. Nächster Schritt N wartet auf Freigabe.") only fires when
    // the NEXT step is another DRAFT_EMAIL, which wf-1 does not have. The previous
    // expectation "Schritt für Dr. Michael Weber freigegeben" exists nowhere in web/src.
    expect(approveResponse.status(), 'HITL approve must execute step 2 via the real KI gateway').toBe(200);
    await expect(page.getByText(/^Schritt 3 erfolgreich ausgeführt\.$/)).toBeVisible();
    await expect(weberRow.getByText('Abgeschlossen')).toBeVisible();
  });
});
