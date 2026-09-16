import { test, expect } from '@playwright/test';

test.describe('AI Assistant, Copilot, Research & EU Observability', () => {
  test('should display AI Copilot, Evidence-Ledger, KB, Research, Triage, and Observability tabs', async ({ page }) => {
    await page.goto('/agent');

    // Check main heading and badges
    await expect(page.getByRole('heading', { name: /KI-Zentrale & Copilot/i })).toBeVisible();
    await expect(page.getByText('Prompt Guards Aktiv')).toBeVisible();
    await expect(page.getByText('Gemma 12B (Lokal)')).toBeVisible();

    // 1. Test Copilot Chat Tab
    await expect(page.getByRole('button', { name: /KI-Vertriebs-Copilot/i })).toBeVisible();
    const chatInput = page.getByPlaceholder(/Frage an den Copilot stellen/i);
    await expect(chatInput).toBeVisible();
    await chatInput.fill('Wie ist der aktuelle Pipeline-Status?');
    await page.getByRole('button', { name: /Senden/i }).click();

    // 2. Test Evidence-Ledger Tab (§5.2)
    const factsTabBtn = page.getByRole('button', { name: /Evidence-Ledger/i });
    await factsTabBtn.click();
    await expect(page.getByText('Dachfläche')).toBeVisible();
    await expect(page.getByText('350 m² Südausrichtung')).toBeVisible();

    // 3. Test Knowledge Base Tab (§5.5)
    const kbTabBtn = page.getByRole('button', { name: /Wissensbasis/i });
    await kbTabBtn.click();
    await expect(page.getByText('Photovoltaik & Speicher')).toBeVisible();

    // 4. Test Company Web Research Tab (§5.4)
    const researchTabBtn = page.getByRole('button', { name: /Web-Recherche/i });
    await researchTabBtn.click();
    await expect(page.getByPlaceholder(/z.B. energie-dach.de/i)).toBeVisible();
    const doResearchBtn = page.getByRole('button', { name: /Automatisch Recherchieren/i });
    await expect(doResearchBtn).toBeVisible();
    await doResearchBtn.click();

    // 5. Test E-Mail Triage Tab (§5.2)
    const triageTabBtn = page.getByRole('button', { name: /E-Mail Triage/i });
    await triageTabBtn.click();
    const analyzeBtn = page.getByRole('button', { name: /Mit KI-Engine analysieren/i });
    await expect(analyzeBtn).toBeVisible();
    await analyzeBtn.click();

    // 6. Test EU AI Act Observability Tab (§5.5 / §20)
    const obsTabBtn = page.getByRole('button', { name: /EU AI Act/i });
    await obsTabBtn.click();
    await expect(page.getByText('Art. 50/52 Konform')).toBeVisible();
    await expect(page.getByText('EU AI Act Revisionssicheres Audit-Log')).toBeVisible();

    // 7. Click on an Audit Log Entry to open the Decision Inspector Modal
    const inspectBtn = page.getByRole('button', { name: /Prüfen/i }).first();
    await expect(inspectBtn).toBeVisible();
    await inspectBtn.click();

    // Verify Decision Inspector Modal
    await expect(page.getByText('KI-Audit & Entscheidungs-Protokoll')).toBeVisible();
    await expect(page.getByText('Bereinigter Eingabe-Prompt')).toBeVisible();
    await expect(page.getByText('Extrahierte KI-Entscheidung')).toBeVisible();

    // Close Modal
    await page.getByRole('button', { name: /Schließen/i }).click();
    await expect(page.getByText('KI-Audit & Entscheidungs-Protokoll')).not.toBeVisible();
  });

  test('should open global floating AI chat drawer from any page', async ({ page }) => {
    await page.goto('/deals');

    // Floating Copilot Button
    const floatingBtn = page.getByRole('button', { name: /KI-Copilot|KI-Assistent öffnen/i });
    await expect(floatingBtn).toBeVisible();
    await floatingBtn.click();

    // Verify Chat Window opened
    await expect(page.getByText('OpenLocalCRM KI-Copilot')).toBeVisible();
    await expect(page.getByText('PII-Schutz aktiv')).toBeVisible();
    await expect(page.getByPlaceholder(/Frage an den Copilot stellen/i)).toBeVisible();
  });

  test('should open Command Palette quick-switcher from header', async ({ page }) => {
    await page.goto('/deals');

    // Click Schnellsuche in Header
    const searchBtn = page.getByRole('button', { name: /Schnellsuche/i });
    await expect(searchBtn).toBeVisible();
    await searchBtn.click();

    // Verify Command Palette modal
    await expect(page.getByPlaceholder(/Suche nach Kontakten/i)).toBeVisible();
    await expect(page.getByRole('button', { name: /Deals & Pipeline CRM Kern/i })).toBeVisible();
    await expect(page.getByRole('button', { name: /Automatisierung & Workflows/i })).toBeVisible();
  });
});
