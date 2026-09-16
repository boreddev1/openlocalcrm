import { test, expect } from '@playwright/test';

test.describe('Sales Reports & 12-Month Forecast', () => {
  test('should display sales report, forecast bars, and § 355 BGB Widerruf stats', async ({ page }) => {
    await page.goto('/reports');

    await expect(page.getByRole('heading', { name: /Vertriebs-Report & Forecast/i })).toBeVisible();
    await expect(page.getByText('Gesichertes Volumen (Won)', { exact: true })).toBeVisible();
    await expect(page.getByText('Gewichteter Forecast')).toBeVisible();
    await expect(page.getByText('Wandlungsquote (Lead ➔ Won)')).toBeVisible();
    await expect(page.getByText('§ 355 BGB Widerrufe')).toBeVisible();

    // Verify Forecast Months
    await expect(page.getByText('Sep 2026')).toBeVisible();
    await expect(page.getByText('Okt 2026')).toBeVisible();
    await expect(page.getByText('Nov 2026')).toBeVisible();
    await expect(page.getByText('Dez 2026')).toBeVisible();
  });
});
