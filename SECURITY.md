# Sicherheitshinweise & Sicherheitsrichtlinie (SECURITY.md)

OpenLocalCRM legt höchsten Wert auf Datensicherheit, Integrität und den Schutz vertraulicher Kunden- und Vertriebsdaten.

---

## Unterstützte Versionen

| Version | Unterstützt |
| :--- | :--- |
| `1.0.x` | ✅ Aktiv unterstützt mit Sicherheits-Patches |
| `< 1.0.0` | ❌ EOL / Nicht mehr unterstützt |

---

## Melden einer Sicherheitslücke (Vulnerability Disclosure)

Wenn Sie eine Sicherheitslücke in OpenLocalCRM entdecken, bitten wir Sie, diese **nicht öffentlich** über GitHub Issues zu melden.

### Vorgehensweise:
1. Senden Sie eine E-Mail mit einer detaillierten Beschreibung der Schwachstelle an **`security@openlocalcrm.local`** (oder eröffnen Sie ein [GitHub Private Security Advisory](https://github.com/boreddev1/openlocalcrm/security/advisories)).
2. Fügen Sie nach Möglichkeit folgende Informationen bei:
   - Betroffene Komponente / Datei / Endpunkt
   - Schritt-für-Schritt-Anleitung zur Reproduktion (Proof of Concept)
   - Mögliche Auswirkungen und Gefahrenstufe
3. Wir bestätigen den Eingang Ihrer Meldung innerhalb von **48 Stunden** und stellen zeitnah einen Patch bereit.

### Sicherheitsprinzipien in OpenLocalCRM:
- **Zero-Trust Input Handling:** Alle Eingaben werden serverseitig sanitisiert (HTML-Escape, CSV-Formelschutz, Regex-Validierung).
- **Kryptographische Sitzungen:** Ed25519 JWTs mit HttpOnly-Cookies und Double-Submit-CSRF-Schutz.
- **Argon2id Password Hashing:** Zeitresistente Hash-Funktionen für alle Benutzer-Anmeldedaten.
- **DSGVO & UWG § 7 Konformität:** Auditierbare Werbeeinwilligungen und strikte PII-Filter vor KI-Inferenz.
