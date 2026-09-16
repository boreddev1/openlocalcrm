import { test, expect } from '@playwright/test';

test.describe('Customer Notes, Activity Log & AI Synthesis (§3.4 & §5.5)', () => {
  test('should open activity log drawer, create call note, trigger AI synthesis and execute action', async ({ page }) => {
    await page.goto('/contacts');

    // 1. Open Notizen & Activity Log Drawer for first contact
    const notesBtn = page.getByRole('button', { name: 'Notizen' }).first();
    await expect(notesBtn).toBeVisible();
    await notesBtn.click();

    const drawer = page.locator('div.fixed.inset-0').filter({ hasText: 'Notizen & Aktivitätslog' });
    await expect(drawer).toBeVisible();

    // 2. Select CALL type and write call note
    await drawer.getByRole('button', { name: '📞 Anruf' }).click();
    await drawer.getByPlaceholder(/Gesprächsnotiz, Zählerstand, Einwände/i).fill('Telefonat mit Kunden: Zählerstand 42.150 kWh notiert. Vor-Ort Begehung gewünscht.');

    // Save Note
    await drawer.getByRole('button', { name: /Notiz speichern/i }).click();

    // Verify newly created note appears in timeline
    await expect(drawer.getByText('Telefonat mit Kunden: Zählerstand 42.150 kWh notiert. Vor-Ort Begehung gewünscht.')).toBeVisible();

    // 3. Trigger Gemma 12B AI Synthesis & Next Best Actions (§5.5)
    const synthBtn = drawer.getByRole('button', { name: /KI-Synthese starten/i });
    await synthBtn.click();

    // Verify AI Synthesis Summary and Suggested Actions
    await expect(drawer.getByText(/KI-Zusammenfassung & Next Actions/i)).toBeVisible();
    await expect(drawer.getByText(/Kaufbereitschaft:/i)).toBeVisible();

    // 4. 1-Click Action Execution: Create Todo from AI Suggestion
    const executeTodoBtn = drawer.getByRole('button', { name: /Ausführen/i }).first();
    await executeTodoBtn.click();
    await expect(drawer.getByText(/erfolgreich im Todo-Board erstellt/i)).toBeVisible();

    // 5. Close Drawer
    const closeBtn = drawer.locator('button').filter({ has: page.locator('svg.lucide-x') });
    await closeBtn.click();
    await expect(drawer).not.toBeVisible();
  });
});
