import { test, expect } from '@playwright/test';

test.describe('D2D Leaflet Field Map', () => {
  test('should render map view with OpenStreetMap attribution', async ({ page }) => {
    await page.goto('/map');

    await expect(page.getByRole('heading', { name: /D2D & Außendienst-Gebietskarte/i })).toBeVisible();

    // Check Leaflet map container
    const mapContainer = page.locator('.leaflet-container');
    await expect(mapContainer).toBeVisible();

    // Verify OpenStreetMap attribution (required by Section 22.3)
    await expect(page.getByText('OpenStreetMap contributors')).toBeVisible();
  });
});
