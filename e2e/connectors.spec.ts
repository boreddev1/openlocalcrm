import { test, expect } from '@playwright/test';

test.describe('Settings & Lead-Intake Connectors', () => {
  test('should display single-tenant system info and webhook configuration', async ({ page }) => {
    await page.goto('/settings');

    await expect(page.getByRole('heading', { name: /Systemeinstellungen/i })).toBeVisible();

    // Check Webhook section
    await expect(page.getByText('Lead-Intake & Partner-Konnektor (REST Webhook)')).toBeVisible();
    await expect(page.getByRole('button', { name: /Test-Payload senden/i })).toBeVisible();

    // Check copy button
    const copyBtn = page.getByRole('button', { name: /Kopieren/i });
    await expect(copyBtn).toBeVisible();

    // Check switching to System-Info tab
    await page.getByRole('button', { name: /System-Info & DSGVO/i }).click();
    await expect(page.getByText('Server-Architektur')).toBeVisible();
    await expect(page.getByText('Datenbank-Engine')).toBeVisible();
    await expect(page.getByText('Lizenz-Compliance (§22)')).toBeVisible();
  });
});
