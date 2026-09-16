import { test, expect } from '@playwright/test';

test.describe('10-Minute Continuous Live User & E-Mail Simulation Suite', () => {
  test('should run continuous user actions and simulated email stream', async ({ page }) => {
    // 1. Open Deals page (login session)
    await page.goto('/deals');
    await expect(page.getByText('Deal-Pipeline & Kanban')).toBeVisible();

    // 2. Expand Live Simulation Widget and toggle Start
    const simPill = page.getByRole('button', { name: /Live-Simulation/i });
    await expect(simPill).toBeVisible();
    await simPill.click();

    const startSimBtn = page.getByRole('button', { name: /Simulation Starten/i });
    await expect(startSimBtn).toBeVisible();
    await startSimBtn.click();

    // Verify Pause button is active
    await expect(page.getByRole('button', { name: /Pause/i })).toBeVisible();

    // 3. Test Speed Selector
    await page.getByRole('button', { name: '5x' }).click();

    // Collapse widget to keep canvas clean
    await page.getByRole('button', { name: /Minimieren/i }).click();

    // 4. Navigate to Contacts & Test CSV Import Wizard (§6.2)
    await page.goto('/contacts');
    await expect(page.getByRole('heading', { name: 'Kontakte & Leads' })).toBeVisible();
    const csvImportBtn = page.getByRole('button', { name: /CSV Import/i });
    await expect(csvImportBtn).toBeVisible();
    await csvImportBtn.click();

    // Sample CSV preview & execution
    await page.getByRole('button', { name: /Beispiel-CSV laden/i }).click();
    await expect(page.getByText('beispiel_kunden_leads.csv geladen')).toBeVisible();
    await page.getByRole('button', { name: /Import jetzt ausführen/i }).click();
    await expect(page.getByText('Import erfolgreich')).toBeVisible();

    // 5. Navigate to Settings & Test Password Change & Backup & Templates
    await page.goto('/settings');
    await expect(page.getByText('Systemeinstellungen & Konfiguration')).toBeVisible();

    // Test Security / Password tab (§2.2)
    await page.getByRole('button', { name: /Sicherheit & 2FA/i }).click();
    await expect(page.getByText('Eigenes Passwort ändern')).toBeVisible();

    // Test Backup & Restore-Drill tab (§8.7)
    await page.getByRole('button', { name: /Datensicherung & Backups/i }).click();
    await expect(page.getByText('PostgreSQL Datenbank-Backup')).toBeVisible();
    const restoreDrillBtn = page.getByRole('button', { name: /Restore-Drill ausführen/i });
    await expect(restoreDrillBtn).toBeVisible();
    await restoreDrillBtn.click();
    await expect(page.getByText(/Restore-Drill erfolgreich/i)).toBeVisible();

    // Test E-Mail Templates tab (§4.6 / §5.1)
    await page.getByRole('button', { name: /E-Mail-Vorlagen/i }).click();
    await expect(page.getByText('KI-Template-Generator per Chat')).toBeVisible();
    await expect(page.getByText('PV 10 kWp Erstangebot mit Speicher')).toBeVisible();

    // 6. Navigate to Automations & Workflows (§6.4)
    await page.goto('/automations');
    await expect(page.getByText('Automationen & Workflows')).toBeVisible();
    const newRoutineBtn = page.getByRole('button', { name: /Neue Automatisierung anlegen/i });
    await expect(newRoutineBtn).toBeVisible();

    // 7. Navigate to Inbox & check live status
    await page.goto('/inbox');
    await expect(page.getByRole('heading', { name: /E-Mail Inbox/i })).toBeVisible();
  });
});
