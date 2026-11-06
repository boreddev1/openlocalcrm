import { test, expect } from '@playwright/test';

test.describe('Sales Reports & 12-Month Forecast', () => {
  test('should display sales report, forecast bars, and § 355 BGB Widerruf stats', async ({ page }) => {
    await page.goto('/reports');

    await expect(page.getByRole('heading', { name: /Vertriebs-Report & Forecast/i })).toBeVisible();
    await expect(page.getByText('Gesichertes Volumen (Won)', { exact: true })).toBeVisible();
    await expect(page.getByText('Gewichteter Forecast')).toBeVisible();
    await expect(page.getByText('Wandlungsquote (Lead ➔ Won)')).toBeVisible();
    await expect(page.getByText('§ 355 BGB Widerrufe')).toBeVisible();

    // Verify the forecast list renders month rows. `month_name` is produced by Go's
    // time.Format("Jan 2006") — an ENGLISH abbreviation — and ReportsPage renders
    // f.month_name verbatim (de-DE is used there only for euro amounts). The previous
    // expectations asserted German forms ('Okt 2026', 'Dez 2026') that this UI cannot
    // produce, and pinned a calendar window that stops matching the seeded data once
    // the month turns. Assert the shape the API guarantees instead of a fixed list.
    await expect(page.getByText('Noch keine Deals im Forecast')).toHaveCount(0);
    const forecastMonths = page.getByText(/^[A-Z][a-z]{2} \d{4}$/);
    await expect(forecastMonths.first()).toBeVisible({ timeout: 10_000 });
  });
});
