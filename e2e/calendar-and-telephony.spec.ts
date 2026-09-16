import { test, expect } from '@playwright/test';

test.describe('Calendar & Click-to-Call Telephony (§3.6 & §7.1a)', () => {
  test('should manage appointments, sync M365 & Google Calendar, and export ICS', async ({ page }) => {
    await page.goto('/calendar');

    // 1. Verify External M365 & Google Sync Bar & Appointments
    await expect(page.getByText('M365 & Google OAuth verbunden')).toBeVisible();
    await expect(page.getByText('Internes Vertriebs-Meeting (M365)')).toBeVisible();
    await expect(page.getByText('Privater Termin / Facharzt (Google)')).toBeVisible();

    // 2. Trigger Sync Button
    const syncBtn = page.getByRole('button', { name: /Jetzt abgleichen/i });
    await syncBtn.click();
    await expect(page.getByText(/Microsoft 365 & Google Kalender synchronisiert/i)).toBeVisible();

    // 3. Toggle External Visibility
    await page.getByRole('button', { name: /Externe Termine sichtbar/i }).click();
    await expect(page.getByText('Internes Vertriebs-Meeting (M365)')).not.toBeVisible();
    await page.getByRole('button', { name: /Externe Termine ausgeblendet/i }).click();
    await expect(page.getByText('Internes Vertriebs-Meeting (M365)')).toBeVisible();

    // 4. Create New CRM Appointment
    await page.getByRole('button', { name: /Termin vereinbaren/i }).click();
    await page.getByPlaceholder(/z.B. Vor-Ort Dachberatung/i).fill('Vor-Ort-Dachberatung Familie Schmidt');
    await page.getByRole('button', { name: /Termin speichern/i }).click();
    await expect(page.getByRole('heading', { name: 'Vor-Ort-Dachberatung Familie Schmidt' })).toBeVisible();

    // 5. Push to M365/Google & ICS Download & Invite Email
    const appCard = page.locator('.bg-slate-900').filter({ hasText: 'Vor-Ort-Dachberatung Familie Schmidt' });
    await appCard.getByRole('button', { name: /An M365 pushen/i }).click();
    await expect(page.getByText(/erfolgreich in Ihren Microsoft 365 & Google Kalender übertragen/i)).toBeVisible();
    await expect(appCard.getByRole('button', { name: /In M365 gepusht/i })).toBeVisible();

    await appCard.getByRole('button', { name: /Einladen/i }).click();
    await page.getByRole('button', { name: /Einladungsmail senden/i }).click();
    await expect(page.getByText(/ICS-Kalendereinladung erfolgreich an/i)).toBeVisible();

    // 6. Edit & Delete Appointment
    await appCard.getByRole('button', { name: /Bearbeiten/i }).click();
    await page.getByRole('button', { name: /Änderungen speichern/i }).click();
    await appCard.getByRole('button', { name: /Löschen/i }).click();
    await expect(page.getByText('Termin wirklich löschen?')).toBeVisible();
    await page.getByRole('button', { name: /Endgültig löschen/i }).click();
    await expect(page.getByText('Termin wirklich löschen?')).not.toBeVisible();
    await expect(page.getByRole('heading', { name: 'Vor-Ort-Dachberatung Familie Schmidt' })).not.toBeVisible();
  });

  test('should trigger Click-to-Call modal from contacts page and log call', async ({ page }) => {
    await page.goto('/contacts');

    const phoneBtn = page.getByRole('button', { name: /\+49 170 8899221/i });
    await expect(phoneBtn).toBeVisible();
    await phoneBtn.click();

    await expect(page.getByText('Click-to-Call Telefonie (§7.1a)')).toBeVisible();
    await page.getByPlaceholder(/Notizen zum Telefonat festhalten/i).fill('Kunde erreicht: Sehr interessiert.');
    await page.getByRole('button', { name: /Anruf protokollieren/i }).click();
    await expect(page.getByText('Click-to-Call Telefonie (§7.1a)')).not.toBeVisible();
  });
});
