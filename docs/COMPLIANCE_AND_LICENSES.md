# ⚖️ Compliance, Datenschutz & Open-Source-Lizenzen

Dieses Dokument beschreibt die Einhaltung der gesetzlichen Bestimmungen (DSGVO, UWG § 7, BGB § 355) und die Open-Source-Lizenzrichtlinie (§22 der Fachspezifikation).

---

## 1. Gesetzliche Vorgaben

### 1.1 DSGVO / BDSG
- **Single-Tenant Isolation:** Vollständige physische und logische Trennung aller Kundendaten.
- **Revisionssicheres Audit-Log (§3.10):** Jede Änderung an personenbezogenen Daten wird in `audit_logs` mit Vorher-/Nachher-JSONB-Diffs unveränderbar protokolliert.
- **Datenübertragbarkeit (Art. 20 DSGVO):** Endpunkt `/api/v1/export/contacts.csv` für standardisierte CSV-Datenexporte.
- **PII-Prompt-Guards:** Sensible Daten (IBANs, Kreditkarten, Passwörter) werden vor dem Verlassen des Systems an KI-Modelle automatisch maskiert.

### 1.2 UWG § 7 (Werbeeinwilligungen)
- Kontakte führen zwei getrennte, dokumentierte Einwilligungen:
  - `consent_phone` (Telefon-Opt-in)
  - `consent_email` (E-Mail-Opt-in)
  - `consent_updated_at` (Zeitstempel der letzten Einwilligung)

### 1.3 BGB § 355 (Verbraucher-Widerrufsrecht)
- Deals im Status `WON` können bei Verbraucherwiderruf in den Status `WIDERRUFEN` überführt werden.
- Erfassung von `widerrufen_at` und `widerruf_grund` zur Vermeidung von Verzerrungen in der Vertriebsstatistik.

---

## 2. Open-Source Lizenz-Policy (§22)

Das gesamte Repository steht unter der **[MIT-Lizenz](LICENSE)**.

### Erlaubte Drittanbieter-Lizenzen
- **Permissive Lizenzen:** MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, Unlicense, MPL-2.0 (dynamisch verlinkt).
- **Streng verboten:** GPL v2/v3, AGPL v3, SSPL, BSL oder proprietäre Copyleft-Lizenzen.

### Automatisierte Lizenz-Prüfung
Das Skript `./scripts/check-licenses.sh` scannt alle Abhängigkeiten und generiert das Verzeichnis [THIRD-PARTY-LICENSES.csv](THIRD-PARTY-LICENSES.csv):

```bash
./scripts/check-licenses.sh
```
