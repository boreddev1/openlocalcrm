import { test, expect } from '@playwright/test';

test.describe('Companies Management (§3.2 Full CRUD & AI Research)', () => {
  test('should create, filter, research, edit, and delete company successfully', async ({ page }) => {
    await page.goto('/companies');

    await expect(page.getByRole('heading', { name: /Firmen & Accounts/i })).toBeVisible();

    // 1. Create Company
    const openModalBtn = page.getByRole('button', { name: /Firma anlegen/i });
    await openModalBtn.click();
    await expect(page.getByText('Neue Firma anlegen (§3.2)')).toBeVisible();

    await page.locator('input[placeholder="z.B. Energie Südwest GmbH"]').fill('Hessen Solar Power GmbH');
    await page.locator('input[placeholder="energie-suedwest.de"]').fill('hessen-solar-power.de');
    await page.locator('input[placeholder="+49 69 1234567"]').fill('+49 69 77889900');
    await page.locator('input[placeholder="Photovoltaik & Energie"]').fill('Solarmodule & Speicher');
    await page.locator('input[placeholder="Solarstraße 10"]').fill('Sonnenallee 5');
    await page.locator('input[placeholder="60314"]').fill('60314');
    await page.locator('input[placeholder="Frankfurt"]').fill('Frankfurt');

    await page.locator('form').getByRole('button', { name: /Firma erstellen/i }).click();
    await expect(page.getByText('Neue Firma anlegen (§3.2)')).not.toBeVisible();

    // Verify company appears
    await expect(page.getByText('Hessen Solar Power GmbH')).toBeVisible();
    await expect(page.getByText('hessen-solar-power.de')).toBeVisible();

    // 2. Trigger Gemma 12B AI Research on Company
    const compCard = page.locator('div.bg-slate-900').filter({ hasText: 'Hessen Solar Power GmbH' });
    await compCard.getByRole('button', { name: /KI-Recherche/i }).click();
    await expect(page.getByText(/Gemma 12B Recherche: Hessen Solar Power GmbH/i)).toBeVisible();
    await expect(page.getByText(/KI-Zusammenfassung & Angebot/i)).toBeVisible();

    await page.getByRole('button', { name: /Als Firmennotiz übernehmen/i }).click();
    await expect(page.getByText(/Recherche-Ergebnis für Hessen Solar Power GmbH als Account-Notiz gespeichert/i)).toBeVisible();

    // 3. Edit Company
    await compCard.getByRole('button', { name: /Bearbeiten/i }).click();
    await expect(page.getByText('Firma bearbeiten')).toBeVisible();

    await page.locator('div:has-text("Firma bearbeiten")').getByRole('button', { name: /Änderungen speichern/i }).click();
    await expect(page.getByText('Firma bearbeiten')).not.toBeVisible();

    // 4. Delete Company with confirmation (§9.3)
    await compCard.getByRole('button', { name: /Löschen/i }).click();
    await expect(page.getByText('Firma wirklich löschen?')).toBeVisible();

    await page.getByRole('button', { name: /Endgültig löschen/i }).click();
    await expect(page.getByText('Firma wirklich löschen?')).not.toBeVisible();
    await expect(page.getByText('Hessen Solar Power GmbH')).not.toBeVisible();
  });
});
