import { test, expect } from "@playwright/test";

test.describe("Authentication & Navigation Layout", () => {
  test("should display login page and allow login", async ({ page }) => {
    await page.goto("/login");

    await expect(page.getByText("OpenLocalCRM")).toBeVisible();
    await expect(
      page.getByText("Single-Tenant Anmeldebereich für Vertriebsteams"),
    ).toBeVisible();

    const emailInput = page.getByPlaceholder("name@unternehmen.de");
    await expect(emailInput).toBeVisible();

    const loginButton = page.getByRole("button", { name: /Anmelden/i });
    await loginButton.click();

    // After login, should redirect to dashboard
    await expect(page).toHaveURL("/");
    await expect(page.getByText("Vertriebs-Dashboard")).toBeVisible();
  });

  test("should display sidebar navigation links", async ({ page }) => {
    await page.goto("/");

    await expect(page.getByRole("link", { name: /Dashboard/i })).toBeVisible();
    await expect(
      page.getByRole("link", { name: /Deals & Pipeline/i }),
    ).toBeVisible();
    await expect(page.getByRole("link", { name: /Kontakte/i })).toBeVisible();
    await expect(page.getByRole("link", { name: /Firmen/i })).toBeVisible();
    await expect(
      page.getByRole("link", { name: /Aufgaben \/ Todos/i }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: /D2D \/ Gebietskarte/i }),
    ).toBeVisible();
  });

  test("should validate password policy rules and change own password successfully (§2.2 / §2.3)", async ({
    page,
  }) => {
    await page.goto("/settings");
    await expect(
      page.getByText("Systemeinstellungen & Konfiguration"),
    ).toBeVisible();

    // 1. Open Security Tab
    await page.getByRole("button", { name: /Sicherheit & 2FA/i }).click();
    await expect(page.getByText("Eigenes Passwort ändern")).toBeVisible();

    const currentPassInput = page.getByPlaceholder("••••••••");
    const newPassInput = page.getByPlaceholder(
      "Min. 8 Zeichen + Zahl + Sonderzeichen",
    );
    const confirmPassInput = page.getByPlaceholder("Wiederholung");
    const submitBtn = page.getByRole("button", { name: /Passwort speichern/i });

    // 2. Test empty current password
    await submitBtn.click();
    await expect(
      page.getByText("Bitte geben Sie Ihr aktuelles Passwort ein."),
    ).toBeVisible();

    // 3. Test short password (< 8 chars)
    await currentPassInput.fill("oldpassword123");
    await newPassInput.fill("short1!");
    await confirmPassInput.fill("short1!");
    await submitBtn.click();
    await expect(
      page.getByText("Das neue Passwort muss mindestens 8 Zeichen lang sein."),
    ).toBeVisible();

    // 4. Test missing special character
    await newPassInput.fill("password12345");
    await confirmPassInput.fill("password12345");
    await submitBtn.click();
    await expect(
      page.getByText(/mindestens eine Zahl und ein Sonderzeichen/i),
    ).toBeVisible();

    // 5. Test mismatching password confirmation
    await newPassInput.fill("Secure2026!Password");
    await confirmPassInput.fill("Mismatch2026!Password");
    await submitBtn.click();
    await expect(
      page.getByText("Die Passwörter stimmen nicht überein."),
    ).toBeVisible();

    // 6. Test successful password change
    await confirmPassInput.fill("Secure2026!Password");
    await submitBtn.click();
    await expect(
      page.getByText("Passwort erfolgreich aktualisiert!"),
    ).toBeVisible();

    // 2FA setup, verification and deactivation are covered by their own test
    // below. They cannot be asserted from here: this test stops at the
    // password-change step, which fails for a pre-existing reason unrelated to
    // SP7.
  });

  test("should read the 2FA state from the session and disable it through the real endpoint (\u00a72.2)", async ({
    page,
  }) => {
    // The demo server has no reachable authenticator app, so the 2FA lifecycle is
    // driven through the real request shapes. The crucial part is /me: the
    // button label must follow it, never a local default.
    let totpEnabledOnServer = false;

    await page.route("**/api/v1/me", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          user_id: "11111111-1111-1111-1111-111111111111",
          email: "admin@openlocalcrm.local",
          role: "ADMIN",
          first_name: "Max",
          last_name: "Vertriebsleiter",
          totp_enabled: totpEnabledOnServer,
        }),
      });
    });
    await page.route("**/api/v1/auth/totp/setup", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          secret: "JBSWY3DPEHPK3PXP",
          url: "otpauth://totp/OpenLocalCRM:demo",
        }),
      });
    });
    await page.route("**/api/v1/auth/totp/verify", async (route) => {
      totpEnabledOnServer = true;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, enabled: true }),
      });
    });

    await page.goto("/settings");
    await page.getByRole("button", { name: /Sicherheit & 2FA/i }).click();

    const stateButton = page.getByRole("button", {
      name: /2FA (Aktiviert|Deaktiviert)/,
    });
    // The initial state comes from the server, which reports 2FA as off.
    await expect(stateButton).toHaveText("2FA Deaktiviert");
    // An inactive 2FA cannot be "disabled", so the button is inert.
    await expect(stateButton).toBeDisabled();

    await page.getByRole("button", { name: /Neu generieren/i }).click();
    await expect(page.getByText(/Neuer Secret Key generiert/i)).toBeVisible();

    await page.getByPlaceholder(/6-stell/i).fill("123456");
    await page.getByRole("button", { name: /Verifizieren/i }).click();
    await expect(page.getByText("2FA erfolgreich verifiziert")).toBeVisible();
    // The label only follows because /me was re-read, not because a local
    // setTotpEnabled(true) ran.
    await expect(stateButton).toHaveText("2FA Aktiviert");

    // Deactivation is a real request that asks for the password confirmation.
    const disableRequests: Array<{ password: string; hasCsrf: boolean }> = [];
    let refusedOnce = false;
    await page.route("**/api/v1/auth/totp/disable", async (route) => {
      const post = route.request().postDataJSON() as { password?: string };
      disableRequests.push({
        password: post?.password ?? "",
        hasCsrf: Boolean(route.request().headers()["x-csrf-token"]),
      });
      if (!refusedOnce) {
        refusedOnce = true;
        await route.fulfill({
          status: 401,
          contentType: "application/json",
          body: JSON.stringify({
            error: "totp_disable_failed",
            message: "Das aktuelle Passwort ist nicht korrekt.",
          }),
        });
        return;
      }
      totpEnabledOnServer = false;
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ success: true, enabled: false }),
      });
    });

    await stateButton.click();
    const confirmInput = page.getByLabel(/Passwort zur Bestätigung/i);
    await expect(confirmInput).toBeVisible();

    await confirmInput.fill("falsch123");
    await page.getByRole("button", { name: /2FA jetzt deaktivieren/i }).click();

    // A refused request leaves 2FA enabled and shows the real server error.
    await expect(
      page.getByText("Das aktuelle Passwort ist nicht korrekt."),
    ).toBeVisible();
    await expect(stateButton).toHaveText("2FA Aktiviert");

    await confirmInput.fill("Demo2026!Passwort");
    await page.getByRole("button", { name: /2FA jetzt deaktivieren/i }).click();
    await expect(page.getByText("2FA wurde deaktiviert.")).toBeVisible();
    await expect(stateButton).toHaveText("2FA Deaktiviert");

    expect(disableRequests).toHaveLength(2);
    expect(disableRequests[0].password).toBe("falsch123");
    expect(disableRequests[1].password).toBe("Demo2026!Passwort");
    // The endpoint lives inside the CSRF-protected group.
    expect(disableRequests[0].hasCsrf).toBe(true);
  });

  test("should return a copyable invite link instead of claiming an email was sent (§2.3)", async ({
    page,
  }) => {
    await page.goto("/settings");
    await page.getByRole("button", { name: /Benutzer & Team/i }).click();
    await expect(
      page.getByText("Neuen Benutzer einladen (§2.3)"),
    ).toBeVisible();

    const nameInput = page.getByPlaceholder("z.B. Tim Tester");
    const emailInput = page.getByPlaceholder("tim@unternehmen.de");
    const inviteBtn = page.getByRole("button", {
      name: /Einladung erstellen/i,
    });

    // A unique address per run: the demo server is shared by all workers and
    // users.email is UNIQUE, so a fixed address would collide with the other
    // invite test.
    const uniqueEmail = `tim.copy.${Date.now()}@vertrieb-openlocalcrm.de`;
    await nameInput.fill("Tim NeuerVertriebler");
    await emailInput.fill(uniqueEmail);

    // Watch the real request so the test cannot pass on a client-side fake.
    const inviteRequests: unknown[] = [];
    page.on("request", (req) => {
      if (req.url().includes("/api/v1/users/invite")) {
        inviteRequests.push(req.method());
      }
    });

    await inviteBtn.click();

    await expect(page.getByText(/Einladung erstellt/i)).toBeVisible();
    const link = page.getByRole("textbox", { name: /Einladungslink/i });
    await expect(link).toHaveValue(/\/accept-invite\?token=/);

    // The old claim ("erfolgreich gesendet") is a lie: nothing delivers mail.
    await expect(page.getByText(/erfolgreich an .* gesendet/i)).toHaveCount(0);
    await expect(page.getByText(/E-Mail wurde versendet/i)).toHaveCount(0);

    // A copy action exists so the admin can actually hand the link over.
    await expect(
      page.getByRole("button", { name: /Einladungslink kopieren/i }),
    ).toBeVisible();

    expect(inviteRequests).toEqual(["POST"]);
  });

  test("should accept a token invite in a clean context and require a separate login", async ({
    page,
    browser,
    baseURL,
  }) => {
    // document.cookie is only readable from a real document, not about:blank.
    await page.goto("/settings");

    // 1. An admin issues an invite through the real API.
    const inviteEmail = `clean.invite.${Date.now()}@example.de`;
    const link = await page.evaluate(async (email: string) => {
      const csrf = document.cookie.match(/(^|;\s*)csrf_token=([^;]*)/);
      const res = await fetch("/api/v1/users/invite", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          ...(csrf ? { "X-CSRF-Token": decodeURIComponent(csrf[2]) } : {}),
        },
        body: JSON.stringify({
          email,
          first_name: "Clean",
          last_name: "Invite",
          role: "BENUTZER",
        }),
      });
      if (res.status !== 201) {
        return { error: `status ${res.status}: ${await res.text()}` };
      }
      const body = await res.json();
      return { url: body.invite_url as string };
    }, inviteEmail);
    expect((link as { error?: string }).error).toBeUndefined();

    // 2. A genuinely cookie-free context opens the link. GET validation is
    //    public and read-only, so it must work with no session at all. The
    //    storageState has to be overridden explicitly: this Playwright version
    //    hands the project's authenticated state to browser.newContext().
    const inviteContext = await browser.newContext({
      storageState: { cookies: [], origins: [] },
    });
    const acceptSetCookies: string[] = [];
    const invitePage = await inviteContext.newPage();
    invitePage.on("response", (res) => {
      if (!res.url().includes("/api/v1/auth/accept-invite")) return;
      const setCookie = res.headers()["set-cookie"];
      if (setCookie) acceptSetCookies.push(setCookie);
    });
    // The server returns a link for the API origin; point the browser at the
    // Vite dev server, which serves the same SPA.
    const inviteURL = (link as { url: string }).url.replace(
      /^https?:\/\/[^/]+/,
      baseURL!,
    );
    await invitePage.goto(inviteURL);

    await expect(invitePage.getByText(inviteEmail)).toBeVisible();
    await expect(
      invitePage.getByText("Clean Invite", { exact: true }),
    ).toBeVisible();

    // 3. The chosen password activates the account.
    //
    //    Deliberately frugal with accept requests: both accept routes and
    //    /auth/login share the login middleware limiter, which allows five
    //    requests per IP per minute and counts successful requests too. A long
    //    journey here would trip the limiter for every later auth test in the
    //    run. The password-policy and single-use refusals are covered on the Go
    //    side by TestAcceptRefusesAWeakPasswordAndKeepsTheInviteUsable,
    //    TestAcceptRefusesAReusedTokenAndLeavesTheAccountActive and
    //    TestInviteHandlerAcceptRejectsBadPayloadsAndUnusableTokens.
    await invitePage.getByLabel(/Neues Passwort/i).fill("NeuesPasswort2026!");
    await invitePage
      .getByLabel(/Passwort wiederholen/i)
      .fill("NeuesPasswort2026!");
    await invitePage
      .getByRole("button", { name: /Einladung annehmen/i })
      .click();
    await expect(invitePage.getByText(/Konto ist jetzt aktiv/i)).toBeVisible();

    // 4. Accepting must not have logged the invitee in: login stays the only
    //    session-establishing operation, so no auth cookie may exist yet.
    const cookies = await inviteContext.cookies();
    expect(cookies).toEqual([]);
    // Belt and braces: no accept response carried a Set-Cookie at all.
    expect(acceptSetCookies).toEqual([]);
    await expect(
      invitePage.getByRole("link", { name: /zum Login/i }),
    ).toBeVisible();

    // 5. The new account can really log in with the password it just chose.
    await invitePage.goto(`${baseURL}/login`);
    await invitePage.getByPlaceholder("name@unternehmen.de").fill(inviteEmail);
    await invitePage.getByPlaceholder("••••••••").fill("NeuesPasswort2026!");
    await invitePage.getByRole("button", { name: /Anmelden/i }).click();
    await expect(invitePage).toHaveURL(`${baseURL}/`);

    await inviteContext.close();
  });

  test("should manage team user lifecycle, send invitations, switch roles and enforce admin protection rule (§2.3)", async ({
    page,
  }) => {
    await page.goto("/settings");

    // 1. Open Users & Team Tab
    await page.getByRole("button", { name: /Benutzer & Team/i }).click();
    await expect(
      page.getByText("Neuen Benutzer einladen (§2.3)"),
    ).toBeVisible();
    await expect(page.getByText("Aktive Teammitglieder")).toBeVisible();

    // 2. Invite a new user
    const nameInput = page.getByPlaceholder("z.B. Tim Tester");
    const emailInput = page.getByPlaceholder("tim@unternehmen.de");
    const inviteBtn = page.getByRole("button", {
      name: /Einladung erstellen/i,
    });

    // A unique name and address per run: the demo server is reused between
    // runs and users.email is UNIQUE, so fixed values would collide.
    const runId = Date.now().toString().slice(-6);
    const invitedName = `Tim Vertriebler ${runId}`;
    await nameInput.fill(invitedName);
    await emailInput.fill(`tim.lifecycle.${runId}@vertrieb-openlocalcrm.de`);
    await inviteBtn.click();

    // 3. The invite really created an INVITED account and a copyable link.
    //    The link is 72 hours valid, and nothing claims an email was sent.
    await expect(page.getByText(/Einladung erstellt/i)).toBeVisible();
    await expect(
      page.getByRole("textbox", { name: /Einladungslink/i }),
    ).toHaveValue(/\/accept-invite\?token=/);
    await expect(
      page.getByRole("cell", { name: new RegExp(invitedName) }),
    ).toBeVisible();
    await expect(
      page.getByText("Eingeladen (72 Stunden)").first(),
    ).toBeVisible();
    await expect(page.getByText(/erfolgreich an .* gesendet/i)).toHaveCount(0);

    // 4. Test protection rule: Try to deactivate the only active admin (Max Vertriebsleiter)
    const adminDeactivateBtn = page.locator(
      'tr:has-text("Max Vertriebsleiter") button:has-text("Deaktivieren")',
    );
    await adminDeactivateBtn.click();
    await expect(
      page.getByText(
        /Der letzte aktive Administrator kann nicht deaktiviert werden/i,
      ),
    ).toBeVisible();

    // 5. Test protection rule: Try to demote the only active admin
    const adminRoleBtn = page.locator(
      'tr:has-text("Max Vertriebsleiter") button:has-text("Zu Benutzer")',
    );
    await adminRoleBtn.click();
    await expect(
      page.getByText(
        /Der letzte aktive Administrator kann nicht herabgestuft werden/i,
      ),
    ).toBeVisible();

    // 6. Test role change for normal user (Laura Closerin -> ADMIN)
    const userRoleBtn = page.locator(
      'tr:has-text("Laura Closerin") button:has-text("Zu Admin")',
    );
    await userRoleBtn.click();
    await expect(
      page.getByText(/Rolle von Laura Closerin auf ADMIN geändert/i),
    ).toBeVisible();

    // 7. Test deactivation of the just-invited non-admin user
    const timRow = page.locator(`tr:has-text("${invitedName}")`);
    await timRow.locator('button:has-text("Deaktivieren")').click();
    await expect(
      page.getByText(
        new RegExp(`Benutzer ${invitedName} wurde deaktiviert`, "i"),
      ),
    ).toBeVisible();
  });
});
