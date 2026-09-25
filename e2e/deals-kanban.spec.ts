import { test, expect, type Request, type Response } from '@playwright/test';

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
    test.setTimeout(40_000);

    type MockDeal = {
      id: string;
      title: string;
      value: string;
      currency: string;
      stage: string;
      probability: number;
      widerrufen_at?: string;
      widerruf_grund?: string;
    };

    const dealTitle = `Report invalidation ${Date.now()}-${Math.random().toString(36).slice(2)}`;
    let mockDeal: MockDeal | null = null;
    const reportData = {
      forecast: [
        {
          month_name: 'Regression Monat',
          weighted_eur: 1234.56,
          committed_eur: 0,
          deal_count: 1,
        },
      ],
      conversion_stats: {
        total_leads: 1,
        won_deals: 0,
        lost_deals: 0,
        revoked_deals: 0,
        conversion_rate: 0,
        avg_deal_volume_eur: 0,
      },
    };

    await page.route(/\/api\/v1\/deals(?:\?.*)?$/, async (route) => {
      const request = route.request();
      if (request.method() === 'GET') {
        await route.fulfill({ json: mockDeal ? [mockDeal] : [] });
        return;
      }
      if (request.method() === 'POST') {
        const payload = request.postDataJSON() as { title?: string };
        mockDeal = {
          id: 'e2e-sales-report-deal',
          title: payload.title ?? dealTitle,
          value: '18500.00',
          currency: 'EUR',
          stage: 'LEAD',
          probability: 20,
        };
        await route.fulfill({ status: 201, json: mockDeal });
        return;
      }
      await route.continue();
    });

    await page.route(/\/api\/v1\/deals\/[^/]+\/revoke(?:\?.*)?$/, async (route) => {
      const request = route.request();
      if (request.method() !== 'POST') {
        await route.continue();
        return;
      }
      const payload = request.postDataJSON() as {
        widerrufen_at?: string;
        widerruf_grund?: string;
      };
      if (!mockDeal) {
        await route.fulfill({ status: 404, json: { error: 'deal_not_found' } });
        return;
      }
      mockDeal = {
        ...mockDeal,
        stage: 'WIDERRUFEN',
        probability: 0,
        widerrufen_at: payload.widerrufen_at ?? new Date().toISOString(),
        widerruf_grund: payload.widerruf_grund ?? 'Automated regression reason',
      };
      await route.fulfill({ json: mockDeal });
    });

    await page.route(/\/api\/v1\/reports\/sales(?:\?.*)?$/, async (route) => {
      if (route.request().method() !== 'GET') {
        await route.continue();
        return;
      }
      await route.fulfill({ json: reportData });
    });

    let salesReportRequestCount = 0;
    let documentLoadCount = 0;
    const isSalesReportRequest = (request: Request) =>
      request.method() === 'GET' && new URL(request.url()).pathname === '/api/v1/reports/sales';
    const isSalesReportResponse = (response: Response) =>
      response.request().method() === 'GET' &&
      new URL(response.url()).pathname === '/api/v1/reports/sales';

    page.on('request', (request) => {
      if (isSalesReportRequest(request)) salesReportRequestCount += 1;
    });
    page.on('load', () => {
      documentLoadCount += 1;
    });

    const firstSalesReportRequest = page.waitForRequest(isSalesReportRequest);
    const firstSalesReportResponse = page.waitForResponse(isSalesReportResponse);
    await page.goto('/reports');
    await firstSalesReportRequest;
    const initialSalesReportResponse = await firstSalesReportResponse;
    expect(initialSalesReportResponse.ok()).toBe(true);
    await expect(page.getByText('Regression Monat', { exact: true })).toBeVisible();
    await expect.poll(() => salesReportRequestCount).toBe(1);
    await expect.poll(() => documentLoadCount).toBe(1);

    const navigationMarker = 'sales-report-client-navigation';
    await page.evaluate((marker) => {
      document.documentElement.setAttribute('data-sales-report-navigation-marker', marker);
    }, navigationMarker);
    const staleTimeStartedAt = Date.now();

    await page.getByRole('link', { name: /Deals & Pipeline/i }).click();
    await expect(page).toHaveURL(/\/deals$/);
    await expect(page.locator('html')).toHaveAttribute(
      'data-sales-report-navigation-marker',
      navigationMarker,
    );
    await expect.poll(() => documentLoadCount).toBe(1);
    await page.getByRole('button', { name: /Deal anlegen/i }).click();
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
        /\/api\/v1\/deals\/[^/]+\/revoke$/.test(new URL(response.url()).pathname),
    );
    await page.getByRole('button', { name: /Widerruf bestätigen/i }).click();
    const revokeResponse = await revokeResponsePromise;
    expect(revokeResponse.ok()).toBe(true);
    await expect(page.getByText('Widerruf erfassen (§355 BGB)')).not.toBeVisible();

    const secondSalesReportRequest = page.waitForRequest(isSalesReportRequest);
    const secondSalesReportResponse = page.waitForResponse(isSalesReportResponse);
    await page.getByRole('link', { name: /Vertriebs-Report/i }).click();
    await secondSalesReportRequest;
    const refreshedSalesReportResponse = await secondSalesReportResponse;
    expect(refreshedSalesReportResponse.ok()).toBe(true);
    await expect(page.getByText('Regression Monat', { exact: true })).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute(
      'data-sales-report-navigation-marker',
      navigationMarker,
    );
    await expect.poll(() => documentLoadCount).toBe(1);
    await expect.poll(() => salesReportRequestCount).toBe(2);
    expect(Date.now() - staleTimeStartedAt).toBeLessThan(30_000);
  });
});
