import { test, expect } from "@playwright/test";

test.describe("Settings & Lead-Intake Connectors", () => {
  test("should display single-tenant system info and webhook configuration", async ({
    page,
  }) => {
    await page.goto("/settings");

    await expect(
      page.getByRole("heading", { name: /Systemeinstellungen/i }),
    ).toBeVisible();

    // Check Webhook section
    await expect(
      page.getByText("Lead-Intake & Partner-Konnektor (REST Webhook)"),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: /Test-Payload senden/i }),
    ).toBeVisible();

    // Check copy button. The webhook section renders two buttons named
    // "Kopieren": the Webhook-URL copy (first in DOM order, enabled) and the
    // Connector-Token copy (disabled while the token field is empty). The test
    // means the URL copy, so .first() is scoped with that reason.
    const copyBtn = page.getByRole("button", { name: "Kopieren" }).first();
    await expect(copyBtn).toBeVisible();

    // Check switching to System-Info tab
    await page.getByRole("button", { name: /System-Info & DSGVO/i }).click();
    await expect(page.getByText("Server-Architektur")).toBeVisible();
    await expect(page.getByText("Datenbank-Engine")).toBeVisible();
    await expect(page.getByText("Lizenz-Compliance (§22)")).toBeVisible();
  });

  test("should hold no default connector token and send the real Bearer contract", async ({
    page,
  }) => {
    await page.goto("/settings");

    const tokenField = page.getByLabel(/Bearer-Token/i);
    await expect(tokenField).toBeVisible();

    // No production or demo default. The field starts empty, is masked, and
    // offers no placeholder that could be mistaken for a working value.
    await expect(tokenField).toHaveValue("");
    await expect(tokenField).toHaveAttribute("type", "password");
    await expect(tokenField).toHaveAttribute("placeholder", "");
    await expect(page.getByText("demo-connector-token")).toHaveCount(0);

    // Test the webhook: the browser must send the entered token as a real
    // Authorization: Bearer header, and must not fall back to X-Connector-Token.
    const seen: Array<{
      authorization: string | undefined;
      xConnectorToken: string | undefined;
      contentType: string | undefined;
      body: Record<string, unknown> | undefined;
    }> = [];

    await page.route("**/api/v1/connectors/lead-intake", async (route) => {
      const headers = route.request().headers();
      let body: Record<string, unknown> | undefined;
      try {
        body = route.request().postDataJSON() as Record<string, unknown>;
      } catch {
        body = undefined;
      }
      seen.push({
        authorization: headers["authorization"],
        xConnectorToken: headers["x-connector-token"],
        contentType: headers["content-type"],
        body,
      });
      // The demo instance runs with a real demo token, so a matching one is
      // accepted and the real contact id comes back.
      await route.fulfill({
        status: 201,
        contentType: "application/json",
        body: JSON.stringify({ status: "success", contact_id: "abc-123" }),
      });
    });

    // Sending without a token must be refused client-side, with no request at
    // all: an empty Authorization header would just be a guaranteed 401.
    await page.getByRole("button", { name: /Test-Payload senden/i }).click();
    await expect(
      page.getByText(/Bitte geben Sie den Connector-Token ein/i),
    ).toBeVisible();
    expect(seen).toHaveLength(0);

    await tokenField.fill("e2e-connector-token");
    await page.getByRole("button", { name: /Test-Payload senden/i }).click();

    await expect(page.getByText(/abc-123/)).toBeVisible();
    expect(seen).toHaveLength(1);
    expect(seen[0].authorization).toBe("Bearer e2e-connector-token");
    expect(seen[0].xConnectorToken).toBeUndefined();
    expect(seen[0].contentType).toContain("application/json");

    // The payload must be a schema-valid LeadIntakePayload: real first/last
    // name and a string deal_value, as the Go struct requires.
    const body = seen[0].body!;
    expect(body.first_name).toBe("Sabine");
    expect(body.last_name).toBe("Mustermann");
    expect(String(body.email)).toMatch(/@example\.de$/);
    expect(typeof body.deal_value).toBe("string");
    expect(body.deal_value).toBe("18500.00");
    expect(body.deal_title).toBe("PV-Anlage Test");
    expect(body.source).toBe("settings-webhook-test");
  });

  test("should render a real webhook error instead of a fabricated success", async ({
    page,
  }) => {
    await page.goto("/settings");

    const tokenField = page.getByLabel(/Bearer-Token/i);
    await tokenField.fill("wrong-token");

    await page.route("**/api/v1/connectors/lead-intake", async (route) => {
      await route.fulfill({
        status: 401,
        contentType: "application/json",
        body: JSON.stringify({ error: "unauthorized connector token" }),
      });
    });

    await page.getByRole("button", { name: /Test-Payload senden/i }).click();

    await expect(page.getByText(/unauthorized connector token/i)).toBeVisible();
    await expect(
      page.getByText(/erfolgreich über Webhook eingespeist/i),
    ).toHaveCount(0);
  });
});
