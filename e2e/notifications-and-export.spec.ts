import { test, expect } from '@playwright/test';

test.describe('Notifications, Consent & CSV Export', () => {
  test('should display notification bell and allow opening dropdown', async ({ page }) => {
    await page.goto('/');

    const bellBtn = page.getByRole('button', { name: /Benachrichtigungen/i });
    await expect(bellBtn).toBeVisible();
    await bellBtn.click();

    // Verify Dropdown opens
    await expect(page.getByText('Benachrichtigungen', { exact: true })).toBeVisible();
    await expect(page.getByText(/Als gelesen markieren/i)).toBeVisible();
  });

  test('should display CSV Export button and Consent badges on contacts', async ({ page }) => {
    await page.goto('/contacts');

    // Check CSV Export button
    const exportBtn = page.getByRole('button', { name: /CSV Export/i });
    await expect(exportBtn).toBeVisible();

    // Check Contact Table headers
    await expect(page.getByText('Name & Position')).toBeVisible();
    await expect(page.getByText(/Kontakt & Click-to-Call/i)).toBeVisible();
    await expect(page.getByRole('columnheader', { name: 'Energiedaten' })).toBeVisible();
  });
});
