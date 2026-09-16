import { test, expect } from '@playwright/test';

test.describe('Authentication & Navigation Layout', () => {
  test('should display login page and allow login', async ({ page }) => {
    await page.goto('/login');

    await expect(page.getByText('OpenLocalCRM')).toBeVisible();
    await expect(page.getByText('Single-Tenant Anmeldebereich für Vertriebsteams')).toBeVisible();

    const emailInput = page.getByPlaceholder('name@unternehmen.de');
    await expect(emailInput).toBeVisible();

    const loginButton = page.getByRole('button', { name: /Anmelden/i });
    await loginButton.click();

    // After login, should redirect to dashboard
    await expect(page).toHaveURL('/');
    await expect(page.getByText('Vertriebs-Dashboard')).toBeVisible();
  });

  test('should display sidebar navigation links', async ({ page }) => {
    await page.goto('/');

    await expect(page.getByRole('link', { name: /Dashboard/i })).toBeVisible();
    await expect(page.getByRole('link', { name: /Deals & Pipeline/i })).toBeVisible();
    await expect(page.getByRole('link', { name: /Kontakte/i })).toBeVisible();
    await expect(page.getByRole('link', { name: /Firmen/i })).toBeVisible();
    await expect(page.getByRole('link', { name: /Aufgaben \/ Todos/i })).toBeVisible();
    await expect(page.getByRole('link', { name: /D2D \/ Gebietskarte/i })).toBeVisible();
  });

  test('should validate password policy rules and change own password successfully (§2.2 / §2.3)', async ({ page }) => {
    await page.goto('/settings');
    await expect(page.getByText('Systemeinstellungen & Konfiguration')).toBeVisible();

    // 1. Open Security Tab
    await page.getByRole('button', { name: /Sicherheit & 2FA/i }).click();
    await expect(page.getByText('Eigenes Passwort ändern')).toBeVisible();

    const currentPassInput = page.getByPlaceholder('••••••••');
    const newPassInput = page.getByPlaceholder('Min. 8 Zeichen + Zahl + Sonderzeichen');
    const confirmPassInput = page.getByPlaceholder('Wiederholung');
    const submitBtn = page.getByRole('button', { name: /Passwort speichern/i });

    // 2. Test empty current password
    await submitBtn.click();
    await expect(page.getByText('Bitte geben Sie Ihr aktuelles Passwort ein.')).toBeVisible();

    // 3. Test short password (< 8 chars)
    await currentPassInput.fill('oldpassword123');
    await newPassInput.fill('short1!');
    await confirmPassInput.fill('short1!');
    await submitBtn.click();
    await expect(page.getByText('Das neue Passwort muss mindestens 8 Zeichen lang sein.')).toBeVisible();

    // 4. Test missing special character
    await newPassInput.fill('password12345');
    await confirmPassInput.fill('password12345');
    await submitBtn.click();
    await expect(page.getByText(/mindestens eine Zahl und ein Sonderzeichen/i)).toBeVisible();

    // 5. Test mismatching password confirmation
    await newPassInput.fill('Secure2026!Password');
    await confirmPassInput.fill('Mismatch2026!Password');
    await submitBtn.click();
    await expect(page.getByText('Die Passwörter stimmen nicht überein.')).toBeVisible();

    // 6. Test successful password change
    await confirmPassInput.fill('Secure2026!Password');
    await submitBtn.click();
    await expect(page.getByText('Passwort erfolgreich aktualisiert!')).toBeVisible();

    // 7. Test TOTP 2FA Setup, Regeneration, Invalid Code rejection & Verification
    await expect(page.getByText('Zwei-Faktor-Authentifizierung (RFC 6238 TOTP)')).toBeVisible();

    // Test Key Regeneration
    const regenBtn = page.getByRole('button', { name: /Neu generieren/i });
    await regenBtn.click();
    await expect(page.getByText(/Neuer Secret Key generiert/i)).toBeVisible();

    const totpInput = page.getByPlaceholder(/6-stelliger Code/i);
    const verifyBtn = page.getByRole('button', { name: /Verifizieren/i });

    // Test Invalid Code Rejection
    await totpInput.fill('000000');
    await verifyBtn.click();
    await expect(page.getByText(/Ungültiger Authenticator-Code/i)).toBeVisible();

    // Test Valid Code Verification & Activation
    await totpInput.fill('123456');
    await verifyBtn.click();
    await expect(page.getByText('2FA erfolgreich verifiziert')).toBeVisible();
    await expect(page.getByRole('button', { name: '2FA Aktiviert' })).toBeVisible();

    // Test Deactivation Toggle
    await page.getByRole('button', { name: '2FA Aktiviert' }).click();
    await expect(page.getByRole('button', { name: '2FA Deaktiviert' })).toBeVisible();
  });

  test('should manage team user lifecycle, send invitations, switch roles and enforce admin protection rule (§2.3)', async ({ page }) => {
    await page.goto('/settings');

    // 1. Open Users & Team Tab
    await page.getByRole('button', { name: /Benutzer & Team/i }).click();
    await expect(page.getByText('Neuen Benutzer einladen (§2.3)')).toBeVisible();
    await expect(page.getByText('Aktive Teammitglieder')).toBeVisible();

    // 2. Invite a new user
    const nameInput = page.getByPlaceholder('z.B. Tim Tester');
    const emailInput = page.getByPlaceholder('tim@unternehmen.de');
    const inviteBtn = page.getByRole('button', { name: /Einladung senden/i });

    await nameInput.fill('Tim NeuerVertriebler');
    await emailInput.fill('tim@vertrieb-openlocalcrm.de');
    await inviteBtn.click();

    // 3. Verify user is added with INVITED status (7 days valid)
    await expect(page.getByText(/Einladung erfolgreich an tim@vertrieb-openlocalcrm.de gesendet/i)).toBeVisible();
    await expect(page.getByText('Tim NeuerVertriebler')).toBeVisible();
    await expect(page.getByText('Eingeladen (7 Tage)')).toBeVisible();

    // 4. Test protection rule: Try to deactivate the only active admin (Max Vertriebsleiter)
    const adminDeactivateBtn = page.locator('tr:has-text("Max Vertriebsleiter") button:has-text("Deaktivieren")');
    await adminDeactivateBtn.click();
    await expect(page.getByText(/Der letzte aktive Administrator kann nicht deaktiviert werden/i)).toBeVisible();

    // 5. Test protection rule: Try to demote the only active admin
    const adminRoleBtn = page.locator('tr:has-text("Max Vertriebsleiter") button:has-text("Zu Benutzer")');
    await adminRoleBtn.click();
    await expect(page.getByText(/Der letzte aktive Administrator kann nicht herabgestuft werden/i)).toBeVisible();

    // 6. Test role change for normal user (Laura Closerin -> ADMIN)
    const userRoleBtn = page.locator('tr:has-text("Laura Closerin") button:has-text("Zu Admin")');
    await userRoleBtn.click();
    await expect(page.getByText(/Rolle von Laura Closerin auf ADMIN geändert/i)).toBeVisible();

    // 7. Test deactivation of non-admin user (Tim NeuerVertriebler -> Deaktivieren)
    const timDeactivateBtn = page.locator('tr:has-text("Tim NeuerVertriebler") button:has-text("Deaktivieren")');
    await timDeactivateBtn.click();
    await expect(page.getByText(/Benutzer Tim NeuerVertriebler wurde deaktiviert/i)).toBeVisible();
  });
});
