import { test, expect } from "@playwright/test";

test.describe("10-Minute Continuous Live User & E-Mail Simulation Suite", () => {
  test("should run continuous user actions and simulated email stream", async ({
    page,
  }) => {
    // 1. Open Deals page (login session)
    await page.goto("/deals");
    await expect(page.getByText("Deal-Pipeline & Kanban")).toBeVisible();

    // 2. Expand Live Simulation Widget and toggle Start
    const simPill = page.getByRole("button", { name: /Live-Simulation/i });
    await expect(simPill).toBeVisible();
    await simPill.click();

    const startSimBtn = page.getByRole("button", {
      name: /Simulation Starten/i,
    });
    await expect(startSimBtn).toBeVisible();
    await startSimBtn.click();

    // Verify Pause button is active
    await expect(page.getByRole("button", { name: /Pause/i })).toBeVisible();

    // 3. Test Speed Selector
    await page.getByRole("button", { name: "5x" }).click();

    // Collapse widget to keep canvas clean
    await page.getByRole("button", { name: /Minimieren/i }).click();

    // 4. Navigate to Contacts & Test CSV Import Wizard (§6.2)
    await page.goto("/contacts");
    await expect(
      page.getByRole("heading", { name: "Kontakte & Leads" }),
    ).toBeVisible();
    const csvImportBtn = page.getByRole("button", { name: /CSV Import/i });
    await expect(csvImportBtn).toBeVisible();
    await csvImportBtn.click();

    // Sample CSV preview & execution
    await page.getByRole("button", { name: /Beispiel-CSV laden/i }).click();
    await expect(
      page.getByText("beispiel_kunden_leads.csv geladen"),
    ).toBeVisible();
    await page.getByRole("button", { name: /Import jetzt ausführen/i }).click();
    await expect(page.getByText("Import erfolgreich")).toBeVisible();

    // 5. Navigate to Settings & Test Password Change & Backup & Templates
    await page.goto("/settings");
    await expect(
      page.getByText("Systemeinstellungen & Konfiguration"),
    ).toBeVisible();

    // Test Security / Password tab (§2.2)
    await page.getByRole("button", { name: /Sicherheit & 2FA/i }).click();
    await expect(page.getByText("Eigenes Passwort ändern")).toBeVisible();

    // Test Backup & Restore-Drill tab (§8.7)
    await page
      .getByRole("button", { name: /Datensicherung & Backups/i })
      .click();
    await expect(page.getByText("PostgreSQL Datenbank-Backup")).toBeVisible();
    const restoreDrillBtn = page.getByRole("button", {
      name: /Restore-Drill ausführen/i,
    });
    await expect(restoreDrillBtn).toBeVisible();
    await restoreDrillBtn.click();
    // Demo mode has no PostgreSQL pool, so the drill cannot perform a real
    // scratch-schema round trip. It must report that honestly instead of
    // claiming a pass it never verified.
    await expect(page.getByText(/Prüfung fehlgeschlagen/i)).toBeVisible();
    await expect(page.getByText(/Restore-Drill erfolgreich/i)).toHaveCount(0);

    // Test E-Mail Templates tab (§4.6 / §5.1): the list comes from
    // email_templates, not from a hard-coded array in the component.
    await page.getByRole("button", { name: /E-Mail-Vorlagen/i }).click();
    await expect(
      page.getByText("KI-Template-Generator per Chat"),
    ).toBeVisible();
    await expect(
      page.getByText("Noch keine Vorlagen gespeichert"),
    ).toBeVisible();
    await expect(
      page.getByText("PV 10 kWp Erstangebot mit Speicher"),
    ).toHaveCount(0);

    // A template really persists through the API and shows up in the list.
    const created = await page.evaluate(async () => {
      const csrf = document.cookie.match(/(^|;\s*)csrf_token=([^;]*)/);
      const res = await fetch("/api/v1/email-templates", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          ...(csrf ? { "X-CSRF-Token": decodeURIComponent(csrf[2]) } : {}),
        },
        body: JSON.stringify({
          key: "live-simulation-offer",
          name: "Live-Simulation Angebot",
          subject: "Ihr Angebot für {{contact_name}}",
          body: "Hallo {{contact_name}}, hier ist Ihr Angebot.",
          category: "vertrieb",
        }),
      });
      return {
        status: res.status,
        body: res.status === 201 ? await res.json() : await res.text(),
      };
    });
    expect(created.status).toBe(201);
    // An omitted locale must be defaulted by the service, because the generated
    // INSERT names the column explicitly and the column default never fires.
    expect((created.body as { locale: string }).locale).toBe("de-DE");

    await page.reload();
    await page.getByRole("button", { name: /E-Mail-Vorlagen/i }).click();
    await expect(page.getByText("Live-Simulation Angebot")).toBeVisible();
    await expect(page.getByText("de-DE")).toBeVisible();

    // Generation is a real model call. Demo mode has no reachable provider, so
    // the UI must report that honestly instead of showing a canned template.
    await page
      .getByPlaceholder(/Erstelle ein Template/i)
      .fill("Nachfass-Mail für PV-Kunden");
    await page.getByRole("button", { name: /Template generieren/i }).click();
    await expect(page.getByText(/KI-Vorlage:/)).toHaveCount(0);
    await expect(
      page.getByText(/KI-Dienststelle konnte keine Vorlage liefern/i),
    ).toBeVisible();

    // 6. Navigate to Automations & Workflows (§6.4)
    await page.goto("/automations");
    await expect(page.getByText("Automationen & Workflows")).toBeVisible();
    const newRoutineBtn = page.getByRole("button", {
      name: /Neue Automatisierung anlegen/i,
    });
    await expect(newRoutineBtn).toBeVisible();

    // 7. Navigate to Inbox & check live status
    await page.goto("/inbox");
    await expect(
      page.getByRole("heading", { name: /E-Mail Inbox/i }),
    ).toBeVisible();
  });
});
