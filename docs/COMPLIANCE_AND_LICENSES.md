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

AGENTS.md formuliert die Regel als „dependencies must be 100% permissive …
**No GPL/AGPL/SSPL or other copyleft**". Erzwungen wird sie von
`./scripts/check-licenses.sh` gegen [THIRD-PARTY-LICENSES.csv](../THIRD-PARTY-LICENSES.csv).
Die maschinelle Grenze muss dieselbe sein wie die Prosa — dazu ist der Check am
2026-11-06 verschärft worden (Issue [#126](https://github.com/boreddev1/openlocalcrm/issues/126)):
Die alte Fassung suchte die CSV nach einer Handvoll verbotener Zeichenketten ab
(`AGPL|SSPL|BUSL|Commons-Clause|GPL-2|GPL-3`) und nannte alles, was sie nicht fand,
konform. So lief MPL-2.0 — Datei-Level-Copyleft, im Repo längst in Nutzung über
`github.com/riverqueue/river` — kommentarlos durch, ohne dass jemand entschieden hätte.

### 2.1 Erlaubte Drittanbieter-Lizenzen (ohne Entscheidung)

Der Check arbeitet jetzt als **Whitelist**, nicht als Blacklist. Diese SPDX-Ids
gelten als permissiv und brauchen keine Einzel-Freigabe:

`MIT`, `MIT-0`, `Apache-2.0`, `BSD-2-Clause`, `BSD-3-Clause`, `0BSD`, `ISC`,
`CC0-1.0`, `Unlicense`

Doppel-Lizenzierung („MIT OR Apache-2.0") ist erfüllt, wenn **eine** Alternative
permissiv ist — der Licensee darf wählen.

Diese Liste ist hartcodiert im Skript und bewusst **nicht** in der Ausnahmedatei:
Wer sie erweitert, ändert die Lizenz-Policy des Projekts und nicht ein
Dependency-Problem. Erweiterung nur mit Owner-Freigabe und hier dokumentiert.

### 2.2 Streng verboten — keine Ausnahme möglich

Alles, was (normalisiert) `AGPL`, `LGPL`, `GPL`, `SSPL`, `AFFERO`,
„General Public", `BUSL`, `Commons-Clause`, `FSL-`, `CC-BY-NC`,
`NonCommercial`, `Proprietary` oder „Restricted" trägt, scheitert als `BANNED`.
Eine Aufnahme in `license-exceptions.json` kann das **nicht** überstimmen; das ist
die harte Grenze aus AGENTS.md. (`LGPL` war schon vorher de facto blockiert, weil
`GPL-2`/`GPL-3` als Teilstring griffen — die Verschärfung ändert daran nichts.)

### 2.3 Alles andere braucht eine explizite, befristete Entscheidung

MPL-2.0 (Datei-Level-Copyleft), EPL/CPL/CDDL, Hippocratic-2.1 (MIT **plus**
Nutzungsbeschränkung, keine OSI-Lizenz) und jede Lizenz, die der Check nicht
kennt, sind „copyleft-tier": sie bestehen nur mit einem getragenen Eintrag in
[license-exceptions.json](../license-exceptions.json). Unknown ist hier keine
Freifahrtschein-Kategorie — ein Erkennungsfehler wird genauso blockiert wie ein
bekanntes Copyleft.

### 2.4 Die Ausnahmedatei

`license-exceptions.json` liegt im Repo-Root, direkt neben der CSV, die es
freigibt — analog zu `web/audit-exceptions.json`, das neben `web/package.json`
liegt. Ein Eintrag pro Abhängigkeit:

```json
{
  "package": "github.com/riverqueue/river",
  "license": "MPL-2.0",
  "decision": "accepted",
  "reason": "…warum das tragbar ist (Datei-Level-Copyleft, build-time-only, Kernabhängigkeit ohne geprüfte permissive Alternative)…",
  "issue": "https://github.com/boreddev1/openlocalcrm/issues/126",
  "acceptedBy": "ewenwitte",
  "recordedOn": "2026-11-06",
  "reviewBy": "2027-06-30"
}
```

`decision` ist `accepted` (dann `acceptedBy` Pflicht) oder `pending` (dann
`decidedBy` Pflicht: wer die Entscheidung schuldet). `pending` ist **keine**
Genehmigung, sondern ein sichtbar gemachter Befund, der das Team noch nicht
entschieden hat — solche Einträge müssen innerhalb von 90 Tagen verfallen, sonst
`BAD ENTRY`. Freigegebene Einträge verfallen nach einem Jahr (`reviewBy`).

Geprüft wird auf:

| Label | Bedeutung | Folge |
| --- | --- | --- |
| `UNACCEPTED` | copyleft-tier ohne passenden Eintrag (package + license) | blockiert |
| `BANNED` | hartes Copyleft | blockiert, durch keine Ausnahme heilbar |
| `DEAD` | Eintrag trifft nichts mehr (Paket weg oder Lizenz geändert) | blockiert — die Datei darf nicht rotten |
| `OVERDUE` | `reviewBy` überschritten | blockiert — Schweigen ist keine Entscheidung |
| `BAD ENTRY` | Felder fehlen, ungültige Daten, `pending` zu lange geparkt | blockiert |
| `UNLISTED` | direkte go.mod-Pflicht oder npm-Laufzeitabhängigkeit fehlt in der CSV | blockiert |
| `GHOST ROW` | CSV-Zeile beschreibt kein Paket im Graph/Lockfile mehr | blockiert |
| `DRIFT` | Lizenz-Link zeigt auf einen anderen Tag als die Build wählt | blockiert |
| `MALFORMED` / `DUP ROW` | CSV-Zeile nicht `paket,url,lizenz` oder doppelt | blockiert |

Freigegebene und offene Einträge werden bei **jedem** Lauf vollständig ausgedruckt
(Paket, Lizenz, Wer, Wann, Bis wann, Issue, Begründung) — eine Ausnahme, die man
nicht sieht, ist keine Dokumentation.

**Warum nicht die Issue-Liveness wie beim Web-Audit?** Der Audit-Check verlangt ein
*offenes* Issue, weil eine ungepatchte Schwachstelle ein fortlaufendes Risiko ist.
Eine Lizenz-Entscheidung ist ein historisches Dokument: das trennende Issue/PR wird
mit dem Merge geschlossen. Geprüft wird deshalb Link-Form und Vollständigkeit,
nicht der Issue-Status.

### 2.5 Aktuell getragen

| Paket | Lizenz | Status | Warum (Kurzfassung) | Issue | Recorded | Review bis |
| --- | --- | --- | --- | --- | --- | --- |
| `github.com/riverqueue/river` | MPL-2.0 | accepted | Kernabhängigkeit crm-worker, Postgres-nativ; Datei-Level-Copyleft ohne Linking-Zwang; permissive Alternative **nicht** ermittelt | [#126](https://github.com/boreddev1/openlocalcrm/issues/126) | 2026-11-06 | 2027-06-30 |
| `…/river/riverdriver`, `…/riverdriver/riverpgxv5`, `…/rivershared`, `…/rivertype` | MPL-2.0 | accepted | Untermodule desselben Repos; eigene LICENSE-Datei existiert upstream nicht (alter Link: HTTP 404), es gilt die Wurzel-Lizenz | [#126](https://github.com/boreddev1/openlocalcrm/issues/126) | 2026-11-06 | 2027-06-30 |
| `lightningcss` | MPL-2.0 | accepted | Build-time-Addon von vite 8; kein Code im Bundle, nur erzeugte CSS-Variablen im Output (geprüft gegen `web/dist`) | [#126](https://github.com/boreddev1/openlocalcrm/issues/126) | 2026-11-06 | 2027-11-06 |
| `react-leaflet`, `@react-leaflet/core` | Hippocratic-2.1 | **pending** | Keine permissive Lizenz (MIT + Nutzungsbeschränkung). Durch die Verschärfung sichtbar geworden, **nicht** freigegeben. Ersatz wäre leaflet direkt. Entscheidung offen | [#126](https://github.com/boreddev1/openlocalcrm/issues/126) | 2026-11-06 | 2026-12-04 |

### 2.6 Was ein grüner Check **nicht** bedeutet

- **Lizenzen transitiver Abhängigkeiten werden nicht geprüft.** Die CSV ist eine
  gepflegte Liste: Pflicht sind alle direkten go.mod-Anforderungen und alle
  npm-Laufzeitabhängigkeiten. Weitere 17 Go-Module im Graph (überwiegend
  Test-Abhängigkeiten) und der npm-Transitivbaum stehen nicht darin — dort gilt
  die Prosa bisher nur so weit, wie jemand nachschaut. Der natürliche nächste
  Schritt ist ein `go-licenses`-Abgleich; CI installiert `github.com/google/go-licenses`
  bereits, nutzt es aber aktuell in keinem Schritt.
- **Lizenz-Angaben sind Handeinträge.** Eine unbekannte oder falsch geschriebene
  Lizenz-Id landet in der copyleft-tier und wird blockiert, nicht durchgewinkt — aber der
  Check weiß nicht, ob „MIT" wirklich MIT ist. Die URLs wurden am 2026-11-06
  live geprüft (44/44 erreichbar, dabei sechs tote Links in der alten Fassung
  gefunden und korrigiert). Offline läuft der Check ohne Netzwerk:
  `LICENSE_GATE_ONLINE=1 ./scripts/check-licenses.sh` prüft die Links mit;
  ein Netzfehler ist dann eine Warnung, ein 404 ein Fehler.
- **`replace`-Direktiven im go.mod** sind von der Versions-Prüfung ausgenommen.
- Der Skript-Check ersetzt keine Rechtsberatung. Für DSGVO-/UWG-seitige Fragen
  siehe §1 und `docs/SECURITY_REVIEW_*.md`.

### 2.7 Verboten

- Die Whitelist in §2.1 erweitern, um eine Abhängigkeit durchzubekommen — das ist
  ein Policy-Wechsel, kein Dependency-Fix.
- Einen Eintrag löschen, damit der Check grün wird. Zu löschen ist ein Eintrag nur,
  wenn die Ausnahme selbst entfällt (Paket ersetzt, Lizenz geändert).
- `reviewBy` ohne neuen `reason` nach hinten schieben.
- `AGENTS.md`-Prosa abschwächen, statt den Check anzupassen (genau das war #126).
- Den Check in Hooks/CI auskommentieren oder `make check-licenses` umleiten.

### 2.8 Lokal prüfen

```bash
./scripts/check-licenses.sh                      # offline, nutzt go.mod + CSV + license-exceptions.json
LICENSE_GATE_ONLINE=1 ./scripts/check-licenses.sh  # zusätzlich jeder Lizenz-URL live abgreifen
make check-licenses                              # derselbe Einstiegspunkt wie CI (backend-test-Job)
```

Exit 0 mit ausgedruckten Ausnahmen, oder ein nicht-Null-Exit mit einem der Labels
aus §2.4. Der Check braucht `go` und `node` (beides dokumentierte
Voraussetzungen); fehlen sie, scheitert er — ein Check, der nicht lief, darf nicht
als Erfolg gelesen werden.
