# 🗺️ D2D-Vertikale & Konnektor-Leitfaden

Diese Anleitung erklärt die Direktvertriebs-Funktionen (D2D / Außendienst) und die Integration externer Konnektoren in **OpenLocalCRM**.

---

## 1. D2D Gebietsorganisation (Setter/Closer Workflow)

```mermaid
stateDiagram-v2
    [*] --> Gebietserfassung: Geokoordinate / Pin auf Karte
    Gebietserfassung --> Erstkontakt: Setter besucht Adresse
    Erstkontakt --> TerminVereinbart: Qualifizierter Lead (Strom/Gas Zähler erfasst)
    TerminVereinbart --> VorOrtBeratung: Closer besucht Kunden
    VorOrtBeratung --> VertragAbgeschlossen: Deal WON
    VorOrtBeratung --> KeinInteresse: Deal LOST
    VertragAbgeschlossen --> Widerruf: Widerruf nach § 355 BGB (innerhalb 14 Tage)
    VertragAbgeschlossen --> [*]
```

- **Gebietskarte (`/map`):** Basiert auf **Leaflet & OpenStreetMap**. Stellt Kunden und Leads standortgenau dar.
- **Energie- & Zählerdaten:** Zählernummern (`zaehlernummer`), Stromverbrauch (`stromverbrauch_kwh`) und Gasverbrauch (`gasverbrauch_kwh`) können direkt vor Ort erfasst werden.
- **Widerrufs-Handling:** Bei Verbraucherverträgen nach § 355 BGB wird der Widerrufsstatus mit Datum und Begründung sauber separat erfasst, ohne die Abschlussstatistiken zu verzerren.

---

## 2. Lead-Intake Konnektor (Webhook API)

Für Website-Tarifrechner, Lead-Portale oder externe Vertriebspartner steht der Endpunkt `/api/v1/connectors/lead-intake` zur Verfügung:

```bash
curl -X POST https://crm.ihre-domain.de/api/v1/connectors/lead-intake \
  -H "Authorization: Bearer openlocalcrm-secret-connector-token" \
  -H "Content-Type: application/json" \
  -d '{
    "first_name": "Maximilian",
    "last_name": "Mustermann",
    "email": "max@muster.de",
    "phone": "+49 170 1234567",
    "street": "Kaiserstraße 12",
    "zip": "60311",
    "city": "Frankfurt am Main",
    "deal_title": "10 kWp PV-Anlage",
    "deal_value": "16900.00",
    "source": "D2D_CAMPAIGN"
  }'
```
