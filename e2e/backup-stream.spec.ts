import { test, expect } from '@playwright/test';

test.describe('Backup streaming honesty (§8.7)', () => {
  test('an interrupted 200 response must not start a download', async ({ page }) => {
    // Serve HTTP 200 with a Content-Length that promises far more than the body
    // delivers, then close the stream. This is exactly the "export died halfway
    // through" case: the status line already said 200, so only a byte-level
    // check can catch it.
    await page.route('**/api/v1/backup/export', async (route) => {
      const body = '{"format":"openlocalcrm-backup","version":1,"tables":{"users":[';
      await route.fulfill({
        status: 200,
        headers: {
          'Content-Type': 'application/json',
          'Content-Length': String(body.length + 5000),
        },
        body,
      });
    });

    let downloadStarted = false;
    page.on('download', () => {
      downloadStarted = true;
    });

    await page.goto('/settings');
    await page.getByRole('button', { name: /Datensicherung & Backups/i }).click();
    await expect(page.getByText('PostgreSQL Datenbank-Backup')).toBeVisible();

    await page.getByTestId('download-backup').click();

    // The UI must report the truncated stream instead of pretending success.
    const status = page.getByTestId('backup-status');
    await expect(status).toBeVisible();
    await expect(status).toContainText(/Export fehlgeschlagen|unvollständig/i);
    await expect(status).toContainText(/keine Datei gespeichert/i);
    expect(downloadStarted, 'a truncated backup stream must never start a download').toBe(
      false,
    );
  });

  test('a complete stream is downloaded and reported with its byte count', async ({ page }) => {
    const envelope =
      '{"format":"openlocalcrm-backup","version":1,"exported_at":"2026-09-26T10:00:00Z","tables":{}}';

    await page.route('**/api/v1/backup/export', async (route) => {
      await route.fulfill({
        status: 200,
        headers: {
          'Content-Type': 'application/json',
          'Content-Length': String(envelope.length),
        },
        body: envelope,
      });
    });

    const downloadPromise = page.waitForEvent('download');
    await page.goto('/settings');
    await page.getByRole('button', { name: /Datensicherung & Backups/i }).click();
    await page.getByTestId('download-backup').click();

    const download = await downloadPromise;
    expect(download.suggestedFilename()).toMatch(/^openlocalcrm-backup-.*\.json$/);
    await expect(page.getByTestId('backup-status')).toContainText(
      new RegExp(`${envelope.length} Bytes`),
    );
  });

  test('a stream that errors mid-flight never produces a file', async ({ page }) => {
    // A response whose status line already said 200 and whose body dies after
    // the first chunk. Only the reader can catch this, and the Blob must not be
    // created, so nothing is ever downloaded.
    await page.addInitScript(() => {
      const original = window.fetch;
      window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
        const url = typeof input === 'string' ? input : input.toString();
        if (!url.includes('/api/v1/backup/export')) return original(input, init);

        const stream = new ReadableStream<Uint8Array>({
          start(controller) {
            controller.enqueue(new TextEncoder().encode('{"format":"openlocalcrm-backup",'));
            // A network reset surfaces as a stream error, not as a short body.
            controller.error(new Error('connection reset while streaming the backup'));
          },
        });
        return Promise.resolve(
          new Response(stream, {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }) as typeof window.fetch;
    });

    let downloadStarted = false;
    page.on('download', () => {
      downloadStarted = true;
    });

    await page.goto('/settings');
    await page.getByRole('button', { name: /Datensicherung & Backups/i }).click();
    await page.getByTestId('download-backup').click();

    const status = page.getByTestId('backup-status');
    await expect(status).toBeVisible();
    await expect(status).toContainText(/Export fehlgeschlagen/i);
    await page.waitForTimeout(500);
    expect(downloadStarted, 'an aborted backup stream must never start a download').toBe(false);
  });

  test('the restore drill reports an honest result in demo mode', async ({ page }) => {
    await page.goto('/settings');
    await page.getByRole('button', { name: /Datensicherung & Backups/i }).click();
    await page.getByRole('button', { name: /Restore-Drill ausführen/i }).click();

    // Demo mode has no PostgreSQL pool, so a real scratch round trip is
    // impossible and the endpoint must say so. It must never report a pass.
    const status = page.getByText(/Restore-Drill erfolgreich|Prüfung fehlgeschlagen/i).first();
    await expect(status).toBeVisible();
    await expect(status).toContainText(/Prüfung fehlgeschlagen|verfügbar/i);
  });

  test('the restore form defaults to merge and requires an explicit replace choice', async ({
    page,
  }) => {
    await page.goto('/settings');
    await page.getByRole('button', { name: /Datensicherung & Backups/i }).click();

    await expect(page.getByTestId('restore-mode-merge')).toBeChecked();
    await expect(page.getByTestId('restore-mode-replace')).not.toBeChecked();

    await page.getByTestId('restore-mode-replace').check();
    await expect(page.getByTestId('restore-mode-replace')).toBeChecked();
  });
});
