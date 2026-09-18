import { test as setup, expect } from '@playwright/test';
import path from 'path';

const authFile = path.resolve(__dirname, '../playwright/.auth/user.json');

setup('authenticate as demo admin', async ({ page }) => {
  await page.goto('/login');
  await expect(page.getByText('OpenLocalCRM')).toBeVisible();

  const emailInput = page.getByPlaceholder('name@unternehmen.de');
  await expect(emailInput).toBeVisible();

  // In demo mode health check auto-fills demo credentials, but ensure fallback
  await expect(async () => {
    const val = await emailInput.inputValue();
    if (!val) {
      await emailInput.fill('admin@openlocalcrm.local');
      await page.getByPlaceholder('••••••••').fill('demo123');
    }
    expect(await emailInput.inputValue()).toBe('admin@openlocalcrm.local');
  }).toPass({ timeout: 5000 });

  const loginButton = page.getByRole('button', { name: /Anmelden/i });
  await expect(loginButton).toBeVisible();
  await loginButton.click();

  // After login, should redirect to dashboard
  await expect(page).toHaveURL('/');
  await expect(page.getByText('Vertriebs-Dashboard')).toBeVisible();

  // Save auth state for other tests to use
  await page.context().storageState({ path: authFile });
});
