import { test, expect } from '@playwright/test';

test.describe('Contacts Management (§3.1 Full CRUD & AI Research)', () => {
  test('should create, edit, trigger AI research, and delete contact successfully', async ({ page }) => {
    await page.goto('/contacts');

    await expect(page.getByRole('heading', { name: 'Kontakte & Leads' })).toBeVisible();

    // 1. Create Contact
    const openModalBtn = page.getByRole('button', { name: /Kontakt anlegen/i });
    await openModalBtn.click();
    await expect(page.getByText('Neuen Kontakt anlegen')).toBeVisible();

    await page.locator('input[placeholder="Vorname"]').fill('Erika');
    await page.locator('input[placeholder="Nachname"]').fill('Musterfrau');
    await page.locator('input[type="email"]').fill('erika.musterfrau@energie-test.de');
    await page.locator('input[placeholder="+49 170 1234567"]').fill('+49 170 9876543');
    await page.locator('input[placeholder="z.B. Geschäftsführer"]').fill('Inhaberin');
    await page.locator('input[placeholder="Musterstraße 1"]').fill('Solarweg 42');
    await page.locator('input[placeholder="60311"]').fill('60311');
    await page.locator('input[placeholder="Frankfurt am Main"]').fill('Frankfurt');
    await page.locator('input[placeholder="z.B. 1EMH0012345678"]').fill('1EMH9988776655');

    await page.locator('form').getByRole('button', { name: /Kontakt anlegen/i }).click();
    await expect(page.getByText('Neuen Kontakt anlegen')).not.toBeVisible();

    // Verify contact in table
    await expect(page.getByText('Erika Musterfrau')).toBeVisible();
    await expect(page.getByText('Solarweg 42, 60311 Frankfurt')).toBeVisible();

    // 2. Trigger Gemma 12B AI Research on Contact (§5.4)
    const row = page.getByRole('row', { name: /Erika Musterfrau/i });
    await expect(row).toBeVisible();

    const researchBtn = row.getByRole('button', { name: /KI-Recherche/i });
    await researchBtn.click();
    await expect(page.getByText(/Gemma 12B Recherche/i)).toBeVisible();
    await expect(page.getByText(/KI-Zusammenfassung/i)).toBeVisible();

    // Adopt research as note
    await page.getByRole('button', { name: /Als Notiz am Kontakt übernehmen/i }).click();
    await expect(page.getByText(/Recherche-Ergebnis für Erika Musterfrau als CRM-Notiz gespeichert/i)).toBeVisible();

    // 3. Edit Contact
    const editBtn = page.getByRole('row', { name: /Erika Musterfrau/i }).getByRole('button', { name: /Bearbeiten/i });
    await editBtn.click();
    await expect(page.getByText('Kontakt bearbeiten')).toBeVisible();

    await page.locator('div:has-text("Kontakt bearbeiten")').getByRole('button', { name: /Änderungen speichern/i }).click();
    await expect(page.getByText('Kontakt bearbeiten')).not.toBeVisible();

    // 4. Delete Contact with confirmation (§9.3)
    const deleteBtn = page.getByRole('row', { name: /Erika Musterfrau/i }).getByRole('button', { name: /Löschen/i });
    await deleteBtn.click();
    await expect(page.getByText('Kontakt wirklich löschen?')).toBeVisible();

    await page.getByRole('button', { name: /Endgültig löschen/i }).click();
    await expect(page.getByText('Kontakt wirklich löschen?')).not.toBeVisible();
    await expect(page.getByText('Erika Musterfrau')).not.toBeVisible();
  });

  test('should filter contacts via search input and test duplicate check (§3.10)', async ({ page }) => {
    await page.goto('/contacts');

    const searchInput = page.getByPlaceholder('Nach Name, E-Mail oder Zählernummer filtern...');
    await expect(searchInput).toBeVisible();

    await searchInput.fill('Weber');
    await expect(searchInput).toHaveValue('Weber');
    await expect(page.getByText('Dr. Michael Weber')).toBeVisible();

    // Test Duplicate Check (§3.10)
    const dupCheckBtn = page.getByRole('button', { name: /Dubletten prüfen/i });
    await dupCheckBtn.click();
    await expect(page.getByText(/Dubletten gefunden/i)).toBeVisible();
  });
});
