import { test, expect } from '@playwright/test';

test.describe('Automations & Workflows (§6.4)', () => {
  test('should display active workflow routines and open creation modal', async ({ page }) => {
    await page.goto('/automations');

    await expect(page.getByRole('heading', { name: /Automationen & Workflows/i })).toBeVisible();
    await expect(page.getByText('Erstkontakt & Qualifizierung (Neuer Lead)')).toBeVisible();
    await expect(page.getByText('Deal-Abschluss Routine (Phase: WON)')).toBeVisible();

    // Open creation modal
    await page.getByRole('button', { name: /Neue Automatisierung anlegen/i }).click();
    await expect(page.getByRole('heading', { name: /Neue Automatisierungs-Routine anlegen/i })).toBeVisible();
    await page.getByPlaceholder(/z.B. PV-Lead Qualifizierung & Angebot/i).fill('PV-Express Onboarding Routine');
    await page.getByRole('button', { name: /Abbrechen/i }).click();
  });

  test('should switch to Workflow-Runs and allow HITL step approval', async ({ page }) => {
    await page.goto('/automations');

    // Switch to Runs tab
    await page.getByRole('button', { name: /Workflow-Läufe & HITL Freigaben/i }).click();
    await expect(page.getByText('Dr. Michael Weber')).toBeVisible();
    await expect(page.getByText('Wartet auf Freigabe')).toBeVisible();

    // Approve step
    const approveBtn = page.getByRole('button', { name: /Schritt freigeben/i }).first();
    await expect(approveBtn).toBeVisible();
    await approveBtn.click();

    // Verify toast feedback
    await expect(page.getByText(/Schritt für Dr. Michael Weber freigegeben/i)).toBeVisible();
  });
});
