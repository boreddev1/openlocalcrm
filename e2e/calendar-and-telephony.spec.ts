import { test, expect } from '@playwright/test';

test.describe('Calendar & Telephony (§3.6 & §7.1a)', () => {
  test('should manage appointments and export RFC 5545 ICS', async ({ page }) => {
    await page.goto('/calendar');

    // 1. Verify Header & Seeded Appointments
    await expect(page.getByRole('heading', { name: /Termine & Kalender/i })).toBeVisible();
    await expect(page.getByText('Internes Vertriebs-Meeting')).toBeVisible();
    await expect(page.getByText('Privater Termin / Facharzt')).toBeVisible();

    // 2. Create New CRM Appointment
    await page.getByRole('button', { name: /Termin vereinbaren/i }).click();
    await page.getByPlaceholder(/z.B. Vor-Ort Dachberatung/i).fill('Vor-Ort-Dachberatung Familie Schmidt');
    await page.getByRole('button', { name: /Termin speichern/i }).click();
    await expect(page.getByRole('heading', { name: 'Vor-Ort-Dachberatung Familie Schmidt' })).toBeVisible();

    // 3. Verify Direct Backend RFC 5545 ICS Export Link
    const appCard = page.locator('.bg-slate-900').filter({ hasText: 'Vor-Ort-Dachberatung Familie Schmidt' });
    const icsLink = appCard.getByRole('link', { name: /ICS/i });
    await expect(icsLink).toBeVisible();
    await expect(icsLink).toHaveAttribute('href', /\/api\/v1\/appointments\/.*\/ics/);

    // 4. Edit & Delete Appointment
    await appCard.getByRole('button', { name: /Bearbeiten/i }).click();
    await page.getByRole('button', { name: /Änderungen speichern/i }).click();
    await appCard.getByRole('button', { name: /Löschen/i }).click();
    await expect(page.getByText('Termin wirklich löschen?')).toBeVisible();
    await page.getByRole('button', { name: /Endgültig löschen/i }).click();
    await expect(page.getByText('Termin wirklich löschen?')).not.toBeVisible();
    await expect(page.getByRole('heading', { name: 'Vor-Ort-Dachberatung Familie Schmidt' })).not.toBeVisible();
  });

  test('should trigger telephony modal from contacts page and log call', async ({ page }) => {
    await page.goto('/contacts');

    const phoneBtn = page.getByRole('button', { name: /\+49 170 8899221/i });
    await expect(phoneBtn).toBeVisible();
    await phoneBtn.click();

    await expect(page.getByText(/Anruf-Protokoll|Click-to-Call/i)).toBeVisible();
    await page.getByPlaceholder(/Notizen zum Telefonat festhalten/i).fill('Kunde erreicht: Sehr interessiert.');
    await page.getByRole('button', { name: /Anruf protokollieren/i }).click();
    await expect(page.getByText(/Anruf-Protokoll|Click-to-Call/i)).not.toBeVisible();
  });
});
