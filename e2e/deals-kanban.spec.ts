import { test, expect } from '@playwright/test';

test.describe('Deals Kanban Pipeline (§3.3 Full CRUD & Stage Transitions)', () => {
  test('should display all pipeline stages in kanban board', async ({ page }) => {
    await page.goto('/deals');

    await expect(page.getByRole('heading', { name: /Deal-Pipeline & Kanban/i })).toBeVisible();

    // Check all Kanban column stages
    await expect(
      page.locator('#LEAD span').filter({ hasText: 'Lead / Erstkontakt' }),
    ).toBeVisible();
    await expect(page.locator('#QUALIFIED span').filter({ hasText: 'Qualifiziert' })).toBeVisible();
    await expect(
      page.locator('#OFFER_SENT span').filter({ hasText: 'Angebot vorliegend' }),
    ).toBeVisible();
    await expect(
      page.locator('#NEGOTIATION span').filter({ hasText: 'Verhandlung' }),
    ).toBeVisible();
    await expect(
      page.locator('#WON span').filter({ hasText: 'Gewonnen / Abschluss' }),
    ).toBeVisible();
    await expect(page.locator('#LOST span').filter({ hasText: 'Verloren' })).toBeVisible();
  });

  test('should create, edit stage, and delete deal successfully', async ({ page }) => {
    await page.goto('/deals');

    // 1. Create Deal
    const openDealBtn = page.getByRole('button', { name: /Deal anlegen/i });
    await openDealBtn.click();
    await expect(page.getByText('Neuen Deal anlegen (§3.3)')).toBeVisible();

    await page
      .getByPlaceholder('z.B. 10 kWp PV-Anlage & Speicher Müller')
      .fill('Photovoltaik 15kWp Gewerbe');
    await page.locator('input[value="18500.00"]').fill('24500.00');

    await page.getByRole('button', { name: /Deal erstellen/i }).click();
    await expect(page.getByText('Neuen Deal anlegen (§3.3)')).not.toBeVisible();

    // Verify deal appears
    await expect(page.getByText('Photovoltaik 15kWp Gewerbe')).toBeVisible();

    // 2. Edit Deal
    const dealCard = page
      .locator('div[id^="d-"]')
      .filter({ hasText: 'Photovoltaik 15kWp Gewerbe' });
    await dealCard.hover();
    await dealCard.getByRole('button', { name: /Bearbeiten/i }).click();
    await expect(page.getByText('Deal bearbeiten')).toBeVisible();

    await page
      .locator('div:has-text("Deal bearbeiten")')
      .getByRole('button', { name: /Änderungen speichern/i })
      .click();
    await expect(page.getByText('Deal bearbeiten')).not.toBeVisible();

    // 3. Delete Deal
    await dealCard.hover();
    await dealCard.getByRole('button', { name: /Löschen/i }).click();
    await expect(page.getByText('Deal wirklich löschen?')).toBeVisible();

    await page.getByRole('button', { name: /Endgültig löschen/i }).click();
    await expect(page.getByText('Deal wirklich löschen?')).not.toBeVisible();
    await expect(page.getByText('Photovoltaik 15kWp Gewerbe')).not.toBeVisible();
  });

  test('should refresh sales report after revoking a deal', async ({ page }) => {
    let salesReportRequestCount = 0;
    const isSalesReportRequest = (request: { method(): string; url(): string }) =>
      request.method() === 'GET' && new URL(request.url()).pathname === '/api/v1/reports/sales';

    page.on('request', (request) => {
      if (isSalesReportRequest(request)) salesReportRequestCount += 1;
    });

    const firstSalesReportRequest = page.waitForRequest(isSalesReportRequest);
    await page.goto('/reports');
    await firstSalesReportRequest;
    await expect.poll(() => salesReportRequestCount).toBe(1);
    await expect(page.getByRole('heading', { name: /Vertriebs-Report & Forecast/i })).toBeVisible();

    await page.getByRole('link', { name: /Deals & Pipeline/i }).click();
    await expect(page).toHaveURL(/\/deals$/);
    await page.getByRole('button', { name: /Deal anlegen/i }).click();

    const dealTitle = `Report invalidation ${Date.now()}-${Math.random().toString(36).slice(2)}`;
    await page.getByPlaceholder('z.B. 10 kWp PV-Anlage & Speicher Müller').fill(dealTitle);
    await page.getByRole('button', { name: /Deal erstellen/i }).click();

    const dealCard = page
      .getByText(dealTitle, { exact: true })
      .locator('xpath=ancestor::div[contains(@class, "bg-slate-950")][1]');
    await expect(dealCard).toBeVisible();
    await dealCard.getByRole('button', { name: /Widerruf erfassen/i }).click();
    await expect(page.getByText('Widerruf erfassen (§355 BGB)')).toBeVisible();

    const revokeResponsePromise = page.waitForResponse(
      (response) =>
        response.request().method() === 'POST' &&
        /\/api\/v1\/deals\/[^/]+\/revoke$/.test(new URL(response.url()).pathname) &&
        response.ok(),
    );
    await page.getByRole('button', { name: /Widerruf bestätigen/i }).click();
    const revokeResponse = await revokeResponsePromise;
    expect(revokeResponse.ok()).toBe(true);
    await expect(page.getByText('Widerruf erfassen (§355 BGB)')).not.toBeVisible();

    const secondSalesReportRequest = page.waitForRequest(isSalesReportRequest);
    await page.getByRole('link', { name: /Vertriebs-Report/i }).click();
    await secondSalesReportRequest;
    await expect.poll(() => salesReportRequestCount).toBe(2);
  });
});
