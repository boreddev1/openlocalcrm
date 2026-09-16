import { test, expect } from '@playwright/test';

test.describe('Todos, Cross-Selling, Postpone & Cancel with Notes (§3.5)', () => {
  test('should create task, postpone due date, cancel with reason and note, filter, and delete', async ({ page }) => {
    await page.goto('/todos');

    await expect(page.getByRole('heading', { name: /Aufgaben, SLA & Cross-Selling Wiedervorlagen/i })).toBeVisible();

    // 1. Create Cross-Selling Resubmission
    const openResubmissionBtn = page.getByRole('button', { name: /Wiedervorlage \/ Cross-Selling anlegen/i });
    await openResubmissionBtn.click();
    await expect(page.getByText('Wiedervorlage / Cross-Selling anlegen (§3.5)')).toBeVisible();

    // Click 3-Months Preset
    await page.getByRole('button', { name: /3 Monate/i }).click();
    await page.getByPlaceholder('z.B. Sabine Mustermann').fill('Sabine Mustermann');
    await page.getByRole('button', { name: /Eintrag erstellen/i }).click();

    await expect(page.getByText('Wiedervorlage: Batteriespeicher-Nachrüstung anbieten')).toBeVisible();

    // 2. Postpone task (+3 Tage)
    const todoRow = page.locator('div.p-4').filter({ hasText: 'Wiedervorlage: Batteriespeicher-Nachrüstung anbieten' });
    await todoRow.getByRole('button', { name: /Verschieben/i }).click();
    await expect(page.getByText('Aufgabe verschieben')).toBeVisible();

    await page.getByRole('button', { name: /\+3 Tage/i }).click();
    await expect(page.getByText('Aufgabe verschieben')).not.toBeVisible();
    await expect(page.getByText(/Verschoben um 3 Tage/i)).toBeVisible();

    // 3. Cancel / Skip task with reason and note
    await todoRow.getByRole('button', { name: /Überspringen/i }).click();
    await expect(page.getByText('Aufgabe abbrechen / überspringen')).toBeVisible();

    await page.getByPlaceholder(/z.B. Kunde teilte im Telefonat mit/i).fill('Kunde möchte erst im Frühjahr 2027 über Speicher sprechen.');
    await page.getByRole('button', { name: /Mit Notiz überspringen/i }).click();
    await expect(page.getByText('Aufgabe abbrechen / überspringen')).not.toBeVisible();

    // Verify cancellation status and note
    await expect(page.getByText('Abgebrochen / Übersprungen').first()).toBeVisible();

    // 4. Filter by Cancelled
    await page.getByRole('button', { name: 'Abgebrochen / Übersprungen' }).click();
    await expect(page.getByText('Wiedervorlage: Batteriespeicher-Nachrüstung anbieten')).toBeVisible();

    // 5. Delete Entry
    await todoRow.hover();
    await todoRow.getByRole('button', { name: /Löschen/i }).click();
    await expect(page.getByText('Aufgabe wirklich löschen?')).toBeVisible();
    await page.getByRole('button', { name: /Endgültig löschen/i }).click();
    await expect(page.getByText('Aufgabe wirklich löschen?')).not.toBeVisible();
    await expect(page.getByRole('heading', { name: 'Wiedervorlage: Batteriespeicher-Nachrüstung anbieten' })).not.toBeVisible();
  });
});
