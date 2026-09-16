import { test, expect } from '@playwright/test';

test.describe('Deterministic Offer Calculator with AI OCR & Full Manual Sales Rep Adjustments (§5.2 / §5.6 / §6.3)', () => {
  test('should extract bill, allow manual sales rep adjustments on hardware and pricing, and create deal with custom terms', async ({ page }) => {
    await page.goto('/deals');

    // 1. Open Calculator Modal
    const calcBtn = page.getByRole('button', { name: /KI-Angebotsrechner & OCR/i });
    await expect(calcBtn).toBeVisible();
    await calcBtn.click();

    // Verify Modal Header
    await expect(page.getByRole('heading', { name: /KI-Dokumentenextraktion & Manuell anpassbarer Angebotsrechner/i })).toBeVisible();

    // 2. Trigger AI OCR extraction on demo bill
    const parseBillBtn = page.getByRole('button', { name: /Demo-Rechnung einlesen/i });
    await parseBillBtn.click();
    await expect(page.getByText(/Stromrechnung & Zählerdaten erfolgreich eingelesen/i)).toBeVisible();

    // 3. Switch to Hardware Tab & make manual adjustments
    await page.getByRole('button', { name: /2. Hardware/i }).click();
    await expect(page.getByText(/Hardware-Konfiguration & Leistungsdimensionierung/i)).toBeVisible();

    // 4. Switch to Pricing Tab & enter manual sales rep discount
    await page.getByRole('button', { name: /3. Preis & Rabatt/i }).click();
    await expect(page.getByText(/Kaufmännische Preisanpassung, Rabatte & Sonderpositionen/i)).toBeVisible();
    await page.getByPlaceholder('0').first().fill('500'); // 500 € Rabatt

    // 5. Human-in-the-Loop Rating & Deal Creation
    await page.getByPlaceholder(/z.B. Dachstatik geprüft/i).fill('33 Module geprüft, 500 € Rabatt für Messeaktion gewährt.');
    const approveDealBtn = page.getByRole('button', { name: /Berechnung freigeben & Deal anlegen/i });
    await expect(approveDealBtn).toBeEnabled();
    await approveDealBtn.click();

    // Verify Deal creation and appearance on Kanban Board
    await expect(page.getByText(/Angebot erfolgreich mit individuellen Anpassungen als Deal/i)).toBeVisible();
    await expect(page.getByRole('heading', { name: /KI-Dokumentenextraktion & Manuell anpassbarer Angebotsrechner/i })).not.toBeVisible();
    await expect(page.getByText(/PV-Angebot 14.5 kWp \+ 12 kWh Speicher/i)).toBeVisible();
  });
});
