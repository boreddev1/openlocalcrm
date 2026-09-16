# Fachspezifikation v3 — OpenLocalCRM (Single-Tenant)

> **Status:** Draft (15 Review-Runden: … Logik-Review (25 Fixes),
> Nutzungs-Review (17 Fixes), Perspektiven-Reviews (8 Fixes),
> Fach-Review (16 Fixes), Markt-Review (Generalistische Ausrichtung +
> Energie als Leit-Vertikale + 5 Marktlücken geschlossen) — wartet auf
> User-Review)
> **Datum:** 2026-08-22
> **Art:** Rein fachliche Spezifikation — bewusst OHNE Festlegung auf Tech-Stack,
> Datenbank-Schema oder Frameworks. Die technische Ausarbeitung erfolgt in einem
> separaten Brainstorming auf Basis dieses Dokuments.
> **Quelle:** Aggregation und fachliche Neudurchdenkung von ~80 Specs und ~40
> Implementierungsplänen des MVP (`freelancer-sales-crm`). Der MVP dient als
> Feature-Referenz; diese Spec beschreibt das System neu — sauber, einfach,
> ressourcenschonend.

---

## 1. Zielsetzung & Abgrenzung

### 1.1 Zweck & Zielgruppe

OpenLocalCRM v3 ist ein **generalistisches, KI-gestütztes CRM für kleine Teams**
(typisch 1–10 Nutzer), die Vertrieb über E-Mail, Telefon und Außendienst
betreiben. Der fachliche Kern (Kontakte, Firmen, Deals, E-Mail, Todos,
Termine, Dokumente, Agent) ist branchenneutral; **Branchen-Schärfe entsteht
durch Module** (7): die erste und für die Implementierung führende Vertikale
ist **Energie- und Feldvertrieb (D2D)** — sie ist Referenz-Kunde und
Priorität für die Umsetzung (§17), ohne den generalistischen Kern einzuengen.

Kernversprechen:

1. **E-Mail ist der Zentraleingangskanal** — eingehende E-Mails werden automatisch
   zugeordnet, klassifiziert und in Aktionen (Todos, Antwort-Entwürfe) überführt.
2. **KI assistiert, der Mensch entscheidet** — jede KI-Aktion mit kaufmännischer
   oder kommunikativer Folgewirkung erfordert eine explizite Bestätigung
   (Human-in-the-Loop).
3. **Externe Daten werden aufbereitet am Kunden sichtbar** — über die
   Konnektor-Schiene (7.1) werden beliebige externe Datenquellen (Tarifrechner,
   Auskunfteien, Telefonie, Marktdaten) an Kontakten/Firmen angebunden.
4. **Kanalkompetenz:** E-Mail (Vollintegration), Telefonie (Click-to-Call über
   Konnektoren), Website-Formulare (Intake) — die wichtigsten
   Vertriebskommunikationskanäle.

### 1.2 Herkunft & Ansatz

Das bestehende System (MVP) ist über 6 Wochen als Multi-Tenant-Plattform mit
Global-Admin, RLS, RBAC-Matrix und zweistelliger Container-Anzahl gewachsen
(in der Spitze 21 Hintergrund-Worker, ~9–10 GB RAM). Die fachlichen Features
sind valide und erprobt; die Architektur ist es nicht.

Diese Spec übernimmt die **fachlich bewährten Features** des MVP und denkt sie für
ein **Single-Tenant-System** neu durch:

- Eine Organisation, kein Mandantenkonzept, kein Global-Admin.
- Zwei Rollen (Admin, Benutzer) statt RBAC-Matrix.
- Ein Docker-Stack mit wenigen Kern-Services und optionalen Zusatz-Services.
- Ressourcenschonend: Ziel Gesamt-RAM < 4 GB (MVP: ~9–10 GB).

### 1.3 Qualitätsziele

| Ziel | Bedeutung |
|------|-----------|
| **Einfachheit** | Single-Tenant, 2 Rollen, keine Mandanten-Indirektion. Ein Feature existiert nur, wenn es in dieser Spec steht. |
| **Ressourcenschonung** | Wenige Container, geringer RAM-Bedarf, lauffähig auf kleinem VPS (4 GB). |
| **Docker mit optionalen Services** | Kern-Stack läuft allein; KI-Inferenz, Meta-Suche, Monitoring u. a. sind optionale Profile. Das System degradiert gracefully (siehe 1.5). |
| **Sauber durchdacht** | Jedes Modul hat einen klaren Zweck, definierte Zustände und Regeln. Keine Altlasten, keine toten Endpunkte. |
| **Datenhoheit** | Daten jederzeit importierbar und exportierbar (CSV/JSON). Backup/Restore nachweisbar (Restore-Drill). |
| **Compliance** | DSGVO/BDSG (Löschklassen, Portabilität, Log-Hygiene), EU AI Act (Transparenz, menschliche Aufsicht, Guardrails), UWG § 7, TDDDG, BFSG — vollständig aufgeschlüsselt in §19; Qualitäts-/Sicherheitsstandards in §20 |

### 1.4 Explizite Ausschlüsse

Folgende MVP-Bestandteile fließen **bewusst nicht** in v3 ein:

| Ausschluss | Begründung |
|------------|------------|
| Multi-Tenancy, RLS, Mandanten-Verwaltung | Single-Tenant-Entscheidung |
| Global-Admin-Bereich | Single-Tenant-Entscheidung |
| Invoicing, Mahnwesen, Produkte/Tarife, Multi-Währung | Bewusste Produktentscheidung (kein ERP) |
| DATEV/GoBD-Buchungsstapel | Folgt aus Invoicing-Ausschluss |
| Provisions-Engine, Provisions-Dashboards | Altlast aus Beratungs-Umfeld |
| Legal-Chat/Paragraphen-Modul | Altlast |
| Model-Training-Pipeline (LoRA/Distill/Quantize, Registry, Datasets) | Unverhältnismäßig für Single-Tenant; KI kommt über konfigurierbare LLM-Anbieter |
| SSO/OIDC | War auch im MVP offene Lücke; Passwort + optionale 2FA reichen für Zielgruppe |
| Styleguide-/Guide-Seiten, MD3-Experimente | Interne Artefakte |
| EnWG-Unbundling (Netz/Vertrieb-Trennung) | Regulatorische Spezialanforderung, nicht Zielgruppen-kern |

### 1.5 Kern- vs. Optional-Fähigkeiten (Graceful Degradation)

Das System ist in **Kernfähigkeiten** (immer verfügbar) und **optionale
Fähigkeiten** (abhängig von optionalen Services) getrennt. Fehlt ein optionaler
Service, wird die zugehörige Funktion in der UI sauber deaktiviert/hinweisend
ausgeblendet — niemals fehlerhaft.

| Fähigkeit | Kategorie | Abhängigkeit |
|-----------|-----------|--------------|
| CRM-Kern (Kontakte, Firmen, Deals, Todos, Kalender, Suche) | Kern | — |
| Dokumente, Import/Export, Workflows (manuelle Schritte) | Kern | — |
| E-Mail-Abruf & -Versand (IMAP/SMTP/Graph), Inbox, manuelle Zuordnung | Kern | — |
| Benachrichtigungen (In-App, siehe 14) | Kern | — |
| KI-Triage, Drafts, Agent, Research, Transkription | Optional | LLM-Anbieter (externer API-Key oder lokale Inferenz) |
| Meta-Suche für Research | Optional | Meta-Such-Service |
| Tarifrechner/Konnektoren | Optional | jeweilige externe API |
| Audio-Transkription | Optional | Transkriptions-Service |
| Monitoring/Dashboards | Optional | Monitoring-Stack |

**Regel:** Die Konfiguration legt fest, welche optionalen Fähigkeiten aktiv sind.
Deaktivierte Fähigkeiten erscheinen nicht in Menüs/Buttons (kein toter UI-Zustand).

---

## 2. Benutzer, Rollen & Rechte

### 2.1 Grundmodell

- **Single-Tenant:** genau eine Organisation pro Installation.
- **Zwei Rollen:** `Admin` und `Benutzer`. Keine weiteren Rollen, keine
  Rechte-Matrix, keine Objekt-Ownership-Scopes (kein „meine Kontakte" vs. „alle").
- Alle Benutzer sehen dieselben Daten (Shared-Workspace-Prinzip). **Ausnahme:**
  externe Kalendertermine sind privat je Benutzer (siehe 3.6).
- **Arbeits-Zuweisung (kein Rechte-Modell):** Todos, Termine und offene
  E-Mails können an einen Benutzer **zugewiesen** werden — reine
  Arbeitsorganisation: Sichtbarkeit bleibt für alle, aber die Zuständigkeit ist
  erkennbar („Wer macht was?"). Nicht zugewiesene Objekte gelten als Team-Pool.
  Kalender und Dashboard bieten jeweils einen Filter „Meine".

### 2.2 Rechte-Matrix (fachlich)

| Bereich | Admin | Benutzer |
|---------|-------|----------|
| CRM-Daten (Kontakte, Firmen, Deals, Todos, Kalender, Dokumente) lesen/schreiben | ✓ | ✓ |
| Inbox, Drafts prüfen/senden | ✓ | ✓ |
| Agent & Research nutzen | ✓ | ✓ |
| Karte, Setter-/Closer-Views nutzen | ✓ | ✓ |
| Workflows starten/pausieren | ✓ | ✓ |
| Workflows definieren, Templates verwalten | ✓ | ✗ |
| Custom Fields definieren | ✓ | ✗ |
| E-Mail-Konten einrichten | ✓ | ✗ |
| Konnektoren konfigurieren | ✓ | ✗ |
| Tracking-Domains verwalten | ✓ | ✗ |
| Benutzer einladen/deaktivieren | ✓ | ✗ |
| API-Tokens, Webhooks, Guardrails, Audit-Log, Backup | ✓ | ✗ |
| Eigenes Passwort/2FA verwalten | ✓ | ✓ |
| Eigene externe Kalender verbinden | ✓ | ✓ |

### 2.3 Benutzer-Lifecycle

- **Erster Benutzer:** Beim initialen Setup wird der erste Admin angelegt
  (Einrichtungs-Assistent, nur solange kein Benutzer existiert). Der Assistent
  bietet optional das Einspielen von **Demo-Daten** an (Beispiel-Kontakte,
  -Firmen, -Deals, -Todos) zum gefahrlosen Ausprobieren; Demo-Daten sind als
  solche markiert und vollständig entfernbar.
- **Weitere Benutzer:** Admin lädt per E-Mail ein → Einladungslink (**7 Tage
  gültig**, danach verfallen; erneute Einladung möglich) → Passwort setzen
  (Regeln: min. 8 Zeichen, 1 Zahl, 1 Sonderzeichen) → Konto aktiv.
- **Deaktivierung:** Admin deaktiviert Benutzer (Login gesperrt, Daten bleiben,
  Audit-Einträge bleiben zuordenbar). Keine Löschung von Benutzerkonten mit
  Historie — Löschung nur im Rahmen des Löschkonzepts (siehe 9.3).
  **Schutzregel:** Der letzte aktive Admin kann weder deaktiviert noch auf die
  Benutzer-Rolle herabgestuft werden — die Operation wird mit Hinweis abgelehnt
  (System darf nicht verwaist werden).
- **Rollenwechsel:** Admin kann Benutzer zwischen Admin und Benutzer umstufen.
  Herabstufung eines Admins nur, wenn mindestens ein weiterer aktiver Admin
  existiert. Jeder Rollenwechsel erfordert Bestätigung, wird im Audit-Log
  protokolliert und dem betroffenen Benutzer als Benachrichtigung zugestellt.

---

## 3. Modul: CRM-Kern

### 3.1 Kontakte

**Felder:** Name*, E-Mail (optional — D2D-Leads starten oft nur mit Telefon;
Identifikation über E-Mail **oder** Telefon), Telefon, Rolle im Unternehmen
(Freitext), Firma (Account-Verknüpfung, Inline-Neuanlage möglich), Quelle,
Status (Lifecycle-Status wie bei Firmen, siehe 3.2), Kundentyp
(`Privatkunde`/`Geschäftskunde`/`Partner`; Default ohne Firma: `Privatkunde`,
mit Firma: `Geschäftskunde`; korrigierbar — Fundament der UWG-Stufung 4.5),
Signatur, Out-of-Office, **zugewiesen an** (Benutzer, optional — „meine
Kunden"-Sicht, Team-Pool wenn leer, siehe 2.1).

**Vervollständigung:** Fehlende E-Mail/Telefon werden automatisch ergänzt,
sobald der erste Kontakt darüber entsteht (E-Mail-Matching 4.2, Form-Intake
6.5).

**Energie-/Besuchsdaten** (fachliche Erweiterungen, siehe Modul 7): Geokoordinaten
(automatisch per Geocoding aus der Kontakt-Adresse — dieselbe Regel wie bei
Firmen, 3.2; D2D besucht Kontakt-Adressen; ohne Adresse oder bei Geocoding-Fehler
bleiben die Koordinaten leer), Energiedaten (siehe 7.2). **Kein separater
Besuchsstatus:** Der D2D-Fortschritt wird allein durch die **Vertriebsstufe**
(7.6) beschrieben — die Kartenfarben (7.4) mappen auf sie
(`lead`=offen, `setter_gespraech`=besucht, `closer_termin`=heiss,
`abgeschlossen`/`verloren`=erledigt). Ein Status, eine Wahrheit.

**Detailansicht:** Stammdaten, verknüpfte E-Mails, Timeline (siehe 3.4),
Dokumente, Deals, Agent-Panel (siehe 5.7), Fakten (siehe 5.2), Herkunfts-Badge
bei Web-Intake (6.5). Die Website-Story (Tracking-Seitenaufrufe) gehört zur
Firmen-Detailansicht (Tab „Website", siehe 3.2/6.5), nicht zum Kontakt.

**Out-of-Office:** aktiv, von/bis, Nachricht — pro Kontakt, Firma und E-Mail-Konto
pflegbar; aktives OOO wird als Badge angezeigt. **Signatur-Kaskade** beim
E-Mail-Versand: Kontakt → Firma → E-Mail-Konto → leer.

**Regeln:**
- Kontakte entstehen manuell, per E-Mail-Matching (siehe 4.2), per Form-Intake
  (siehe 6.5) oder per Import (siehe 6.2).
- Ein Kontakt gehört zu genau einer Firma (n:1); Verknüpfung änderbar.

**Werbe-Einwilligungen (UWG § 7, siehe 19.4):** Jeder Kontakt trägt zwei
Consent-Felder mit Datum, Quelle und Nachweis-Referenz:
- **E-Mail-Werbung:** erteilt / nicht erteilt / widerrufen (mit Widerrufsdatum).
- **Telefon-Werbung:** erteilt / nicht erteilt / widerrufen (mit Widerrufsdatum).
Consent kann manuell gepflegt werden, entsteht über Form-Intake (Consent-Version,
6.5) und kann jederzeit widerrufen werden (Widerruf wirkt ab sofort, wird im
Logbuch vermerkt). Diese Felder steuern die gestufte Warn-Logik in
Drafts/Workflows (siehe 4.5/6.4 — Stufe hängt am **Kundentyp des Kontakts**:
Privatkunde = strenge B2C-Warnung, Geschäftskunde = dezenter B2B-Hinweis) —
sie sind die fachliche Basis für rechtskonformen Outreach.

### 3.2 Firmen (Accounts)

**Pflichtfelder bei Neuanlage:** Firmenname*, Ansprechpartner*, E-Mail*.

**Weitere Felder:** Telefon, Website, Adresse, PLZ, Ort, Geokoordinaten
(automatisch per Geocoding aus der Adresse; bei Fehler leer, keine Fehlermeldung
ans Speichern), Branche (Dropdown, vorbefüllt — Energie, Solar, Wind, Gas,
Handwerk, IT, Sonstige — frei erweiterbar), Kundentyp
(Geschäftskunde/Privatkunde/Partner), Priorität (Hoch/Mittel/Niedrig), Notizen.

**Lifecycle-Status (Pipeline):** `neu → kontaktiert → angebot → verhandlung →
abgeschlossen → verloren`. Der Pipeline-Status liegt auf Firmen und Kontakten
(nicht auf Deals, siehe 3.3). Jede Statusänderung erzeugt einen Logbuch-Eintrag.
**Abgrenzung zur Vertriebsstufe (7.6):** Der Lifecycle-Status beschreibt die
**Beziehungsentwicklung** („Beziehungsphase"), die Vertriebsstufe am Kontakt
beschreibt den **D2D-Abschlussprozess** (lead → … → abgeschlossen). Die UI
zeigt pro Kontext genau eine der beiden (Firmen-Detail: Lifecycle;
Setter/Closer: Vertriebsstufe) — niemand muss zwei Pipelines gleichzeitig
pflegen.

**Detailansicht (Tabs):** Details / Ansprechpartner / Logbuch / Wiedervorlage /
Dokumente / Deals / Website / Agent.

### 3.3 Deals (Abschlüsse)

Deals sind **Abschluss-Erfassungen**, keine Stage-Board-Objekte. Die
Vertriebspipeline wird über den Lifecycle-Status von Firmen/Kontakten (3.2) und
den Setter/Closer-Workflow (7.6) abgebildet.

**Felder:** Titel*, Bezug* (**Firma oder Kontakt — genau einer von beiden
Pflicht**; bei Privatkunden genügt der Kontakt allein, ohne Firma — keine
Dummy-Firmen), Anbieter/Partner (Freitext mit Vorschlägen), Vertragsbeginn/
-ende, monatlicher Wert (Zahl, EUR), Einmalwert (Zahl, EUR), Tarif/Leistung
(Freitext, optional — was verkauft wurde), Konnektor-Ergebnis (Verweis,
optional — z. B. Tarifrechner-Ergebnis als Verkaufsgrundlage, siehe 7.3),
Notizen, Abschlusszeitpunkt, **Abschlussart** (Aufzählung: `buero`/`d2d`,
Default `buero`) — steuert die Geofencing-Pflicht (siehe 7.5).

**Statuswerte:** `open` | `won` | `lost` | `widerrufen`. Der MVP-Status
`partial` wurde bewusst gestrichen (keine definierte Semantik, im MVP praktisch
ungenutzt — siehe 10.3). **`widerrufen`** ist nur aus `won` erreichbar
(Verbraucher-Widerrufsrecht, § 355 BGB): mit Widerrufsdatum und
Pflicht-Begründung; zählt gesondert in der Auswertung (nicht als `lost` —
der Kunde hat nicht abgelehnt), triggert **keine** Anschluss-Wiedervorlage,
erzeugt einen einmaligen Vorschlag „Widerrufs-Arbeitsschritte prüfen"
(Vertragsdaten korrigieren, ggf. Nachfass-Termin).

**Währung:** Alle Beträge in **EUR** (fix). Multi-Währung ist bewusst
ausgeschlossen (siehe 1.4, 10.3).

**Regeln:**
- **Doppelverkauf-Warnung:** Wird ein Deal an einem Kontakt/Bezug angelegt, zu
  dem bereits ein `won`-Deal mit laufender Vertragszeit (Vertragsende in der
  Zukunft) existiert, zeigt das System eine deutliche Warnung (Doppelbelieferung
  / Provision-Betrug-Schutz). Anlage bleibt möglich (HITL — es kann ein
  legitimes Zweitgeschäft sein, z. B. Gas nach Strom), Warnung wird
  protokolliert.
- Bei `won` schlägt das System halbautomatisch vor: eine **terminierte
  Wiedervorlage** „Anschlussvertrag {Firma oder Kontakt}" (Todo mit Fälligkeit
  3 Monate vor Vertragsende, Frist konfigurierbar). Der Vorschlag ist kein
  flüchtiger Chip: einmal bestätigt, existiert das Todo dauerhaft mit
  Fälligkeit — Vertragsenden verstreichen nie unbemerkt. Zusätzlich passende
  Workflow-Vorschläge (siehe 6.4).
- **Bei `lost`** schlägt das System eine **Reaktivierungs-Wiedervorlage** vor
  (Default: in 6 Monaten, konfigurierbar) — Vertrieb lebt vom Wiederansetzen;
  einmal bestätigt oder verworfen, keine Wiederholung für diesen Deal.
- **Lieferstart-Prüfung:** Ein `won`-Deal mit **zukünftigem Vertragsbeginn**
  erzeugt automatisch den einmaligen Vorschlag eines Todos „Lieferstart prüfen"
  (fällig am Vertragsbeginn — Energie-Wechsel laufen über Wochen; ob der Wechsel
  tatsächlich startet, wird damit sichtbar).
- **Energiedaten-Pflege bei `won`:** Mit dem Abschluss wird vorgeschlagen, die
  Energiedaten des Kontakts zu aktualisieren (neuer Anbieter, Vertragsende aus
  dem Deal) — sonst rechnet die Kündigungsfrist-Wiedervorlage (7.2) künftig mit
  Altanbieter-Daten.
- Deal-Wechsel zu `won`/`lost`/`widerrufen` erzeugt Logbuch-Eintrag und ist
  Audit-relevant.

### 3.4 Aktivitäten, Logbuch & Timeline

**Aktivitätstypen:** `note`, `call`, `email`, `meeting`, `status_change`, `system`.
Attribute: Typ, Titel, Text, Priorität (dringend/normal), Zeitstempel, Autor.

**Automatische System-Einträge** bei: Statusänderung, Kontakt angelegt/verknüpft,
Todo erstellt/erledigt, Wiedervorlage fällig, Deal angelegt, Dokument
hochgeladen/gelöscht, Workflow-Schritt ausgeführt, Import-Lauf.

**KI-Extraktion aus Freitext-Notizen:** Beim Speichern einer Notiz schlägt die KI
als Chips vor: Wiedervorlage (erkanntes Datum/Zeit), Todo (erkannte Aufgabe),
E-Mail-Entwurf (erkannter Kontext). Jeder Vorschlag einzeln bestätig-/verwerfbar
(HITL).

**Timeline:** Pro Firma/Kontakt/Deal werden alle Ereignisse chronologisch
vereinigt — Aktivitäten, Termine, E-Mails, Fakten, Rechecks, Agent-Events.
Geladen werden die jüngsten 50 Einträge; „Weitere laden" blättert in
50er-Schritten nach (kein hartes Limit für langjährige Kunden). Die Timeline
ergänzt das Logbuch um die KI-/Agent-Sicht.

**`lastActivityAt`:** Wird auf Firmen/Kontakten/Deals bei jeder neuen
Aktivität/E-Mail/Termin automatisch aktualisiert (Basis für Sortierung und
„lange nicht kontaktiert"-Filter).

### 3.5 Todos & Wiedervorlagen

**Felder:** Titel*, Notiz, Firma, Kontakt, Fälligkeit, Priorität
(Niedrig/Mittel/Hoch/Dringend), Kategorie (`follow_up`/`call`/`email`/`contract`/
`other`), Status (offen/erledigt), **zugewiesen an** (Benutzer, optional —
Team-Pool, wenn leer; siehe 2.1).

**Wiedervorlage = Todo mit Firma + Fälligkeit** (kein separates Objektmodell).

**Fälligkeit:** Schnellauswahl Heute/Morgen/Nächste Woche/freies Datum;
Verschieben um +1 Tag/+1 Woche/+2 Wochen. Keine Wiederholungsregeln.

**Ansichten:** Gruppierung Überfällig (rot)/Heute/Diese Woche/Kommende/Erledigt
(einklappbar); Jahr→Monat→Tag-Drilldown; Filter nach Zeitraum, Status, Priorität,
Firma.

### 3.6 Kalender & Termine

**Termin-Felder:** Typ (`meeting`/`call`/`email_reminder`/`deadline`), Titel*,
Firma, Kontakt, Start*, Ende (leer = ganztags), Ort/Link, Erinnerung (15 Min/
30 Min/1 Std/1 Tag), Status (`planned`/`done`/`cancelled`),
Draft-Verknüpfung (nur bei `email_reminder`), **zugewiesen an** (Benutzer,
optional — Team-Pool, wenn leer; „Termin für Closer buchen" setzt dieses Feld,
siehe 7.6).

**Erinnerungen:** Zur konfigurierten Erinnerungszeit (15 Min/30 Min/1 Std/1 Tag
vor Start) wird eine **Benachrichtigung** ausgelöst (Zentrum, siehe 14) — an den
zugewiesenen Benutzer, sonst an alle — zusätzlich zur visuellen Markierung im
Kalender. Ein Kalender, der nur anzeigt, aber nicht aktiv erinnert, wäre im
Vertriebsalltag unbrauchbar.

**Einladung an externe Teilnehmer:** Zu jedem Termin mit Firma/Kontakt-Bezug
kann eine **Einladungs-E-Mail mit Kalender-Datei (ICS)** an die
Kunden-E-Mail-Adresse versendet werden (über das E-Mail-Konto des Termins,
Default primäres Konto 4.1; Betreff/Ort/Zeit aus dem Termin; Teilnahme-Zusage
muss nicht zurückverarbeitet werden — die Einladung ist eine
**Bestätigungsmail**, kein Abstimmungs-Workflow). Dies senkt No-show-Raten
und ist Standard-Erwartung an jeden Terminkalender im Vertrieb. Der Versand
wird als `email`-Aktivität protokolliert.

**Ansichten:** Monat (Default), Woche (08–18 Uhr), Tag; Wochenstart Montag,
Datumsformat TT.MM.JJJJ. **Benutzer-Filter:** „alle Termine" (Default) oder
„meine Termine" (mir zugewiesen oder von mir importiert). Farbcodierung:
Anruf blau, Meeting lila, Todo grau (überfällig rot), E-Mail-Erinnerung orange,
Deadline rot.

**Todo-Integration:** Der Kalender zeigt Todos ganztags am Fälltag mit Checkbox
(direkt abhakbar); Drag-and-Drop verschiebt Fälligkeiten. Sidebar „Heute fällig".

**E-Mail-Erinnerungen:** Termin vom Typ `email_reminder` kann einen Antwort-Entwurf
verknüpfen („Draft öffnen", „Als gesendet markieren"). Entstehung: manuell, per
KI aus Logbuch (siehe 3.4) oder automatisch aus Workflows (siehe 6.4).

**Externe Kalender (Google/Microsoft):** Jeder Benutzer verbindet seinen eigenen
Kalender per OAuth (Read-Only-Import). Synchronisation alle 15 Minuten, Fenster:
30 Tage zurück / 60 Tage voraus. Externe Termine: grau mit Provider-Icon, nicht
editierbar, Link zum Original, ein-/ausblendbar. Teilnehmer/Einladungen werden
angezeigt (Name, E-Mail, Antwortstatus). Verbindungen trennbar/pausierbar,
manueller Sync-Button.

**Sichtbarkeit (Privatsphäre-Regel):** Externe Termine sind **ausschließlich für
den verbindenden Benutzer** sichtbar — auch im geteilten Arbeitsbereich (2.1).
Sie erscheinen nur in dessen Kalenderansicht und werden nicht in gemeinsamen
Ansichten (Firma/Kontakt-Timeline, Dashboard anderer Benutzer) angezeigt. Dies
ist die dokumentierte Ausnahme vom Shared-Workspace-Prinzip.

### 3.7 Globale Suche & Quick-Switcher

Durchsucht: Kontakte (Name/E-Mail/Telefon), Firmen (Name/Domain/Website), Deals
(Titel), Todos (Titel), Dokumente (Titel), **E-Mails (Betreff + Body-Volltext)**.
Ab 2 Zeichen, initial max. 10 Treffer je Typ, gruppiert nach Typ — mit
**„Weitere anzeigen" je Typ** (kein totes Ende bei häufigen Namen; Treffer
werden nachgeladen). Sortierung je Typ nach Relevanz (Präfix-Treffer und
kürzlich aktive Entitäten zuerst, gewichtet nach `lastActivityAt`).
Derselbe Suchindex speist die Cmd/Ctrl+K-Palette (Quick-Switcher) in der Shell.
E-Mail-Treffer öffnen die E-Mail in der Inbox-Ansicht.

### 3.8 Bulk-Actions & Saved Views

**Bulk-Actions:** Auf Kontakten/Firmen/Deals per Checkbox-Auswahl: Status ändern,
Custom Field setzen (mit Validierung), CSV-Export der Auswahl. **Kein
Bulk-Delete** (bewusst, Schutz vor Datenverlust).

**Saved Views:** Benannte Filter+Sortierung pro Entitätstyp; speichern/laden/
löschen; für alle Listenansichten (Kontakte, Firmen, Deals, Todos).

### 3.9 Custom Fields

**Feldtypen:** `text`, `number`, `date`, `select` (Optionsliste), `url`, `email`,
`phone`, `boolean`, `user`.

**Anhängbar an:** Kontakte, Firmen, Deals.

**Definition (Admin):** Schlüssel (unique je Entitätstyp, keine Kollision mit
Kernfeldern), Label, Typ, Optionen, Pflicht-Flag, Reihenfolge, aktiv/inaktiv.
Löschen = Deaktivieren (Daten bleiben lesbar, keine Eingabe mehr).

**Validierung:** Typgerechte Prüfung (Zahlen-/Datums-/E-Mail-/URL-Format);
Select-Werte nur aus Optionsliste; Pflichtfelder erzwungen. Werte werden in
Formularen und Detailansichten dynamisch gerendert und sind bulk-setzbar (3.8).

**Feldtyp `user` bei deaktivierten Benutzern:** Bestehende Werte bleiben gültig
und werden mit Marker „(deaktiviert)" angezeigt; in Auswahl-Listen werden nur
aktive Benutzer angeboten. Der Wert verfällt nicht durch Deaktivierung
(Konsistenz mit 2.3: Daten deaktivierter Benutzer bleiben erhalten).

### 3.10 Dubletten-Merge (Kontakte & Firmen)

Neben der Import-Dedup (6.2) gibt es **Merge im Tagesgeschäft**: Zwei Kontakte
oder zwei Firmen können manuell zusammengeführt werden (entstanden durch
E-Mail-Matching- oder Setter-Dubletten). Regeln wie im Import-Wizard: Feldliste
beider Objekte nebeneinander, Konflikt je Feld entscheidbar, Default „neuerer
Wert gewinnt", **niemals ungefragt überschreiben**; Referenzen (Deals, Todos,
E-Mails, Fakten, Energiedaten, Einwilligungen) wandern auf das Zielobjekt;
der Merge erzeugt Logbuch- und Audit-Eintrag; das Quell-Objekt wird hart
gelöscht (16.1).

### 3.11 Bewusste fachliche Vereinfachungen (dokumentiert)

- **Angebots-Verfolgung:** Angebote sind Dokumente (6.1) plus Lifecycle-Stufe
  `angebot` (3.2) — ein eigener Angebots-Status (versendet/angenommen/
  abgelehnt) und Angebots-Validität sind bewusst nicht modelliert (YAGNI);
  wer sie braucht, nutzt Custom Fields (3.9). Die Verknüpfung
  Angebot-Dokument ↔ Deal bleibt der manuellen Zuordnung überlassen.
- **Zweitgeschäft nach Abschluss:** Die Vertriebsstufe `abgeschlossen`
  beschreibt das **letzte** Geschäft; ein neuer Deal am selben Kontakt
  (z. B. Gas nach Strom) ist jederzeit möglich und kein Widerspruch
  (Doppelverkauf-Warnung 3.3 schützt bei Bedarf).
- **Closer-Termin-No-show:** Ein verpasster Termin wird über den Termin-Status
  `cancelled` (3.6) erfasst; die Vertriebsstufe bleibt `closer_termin` bis
  der Prozess bewusst weitergeführt oder verworfen wird.

---

## 4. Modul: E-Mail & KI-Triage

### 4.1 E-Mail-Konten

**Typen:** Klassische Postfächer (IMAP mit App-Passwort) und Microsoft-365-Konten
(OAuth-Freigabe). Jedes Konto umfasst **Abruf und Versand**: klassisch per
IMAP + SMTP (Server, Port, Zugangsdaten), Microsoft 365 über Graph (Senden im
Auftrag des Kontos).

**Verwaltung (Admin):** Hinzufügen, Verbindungstest (Abruf **und** Testversand),
Abfrage-Takt konfigurierbar (Standard 60 s), aktivieren/deaktivieren, Signatur
und Out-of-Office pro Konto (Basis der Signatur-Kaskade, siehe 3.1).

**Primäres Konto:** Genau ein aktives E-Mail-Konto ist als **primär** markiert.
Es dient als Versandkonto für Workflow-Drafts (6.4) und als Fallback, wenn ein
Draft keinem Konto zuzuordnen ist. Ohne primäres Konto sind Workflow-Versand-
Schritte blockiert (Hinweis statt Fehler).

**Versand:** Freigegebene Drafts (4.5) werden über den Versandweg des jeweiligen
Kontos verschickt; die gesendete E-Mail wird im Gesendet-Ordner des Postfachs
abgelegt und mit Draft, Kontakt und Firma verknüpft. Versandfehler (z. B.
SMTP-Authentifizierung) landen in der Fehlerliste (siehe 4.2) mit Retry.

**Duplikatschutz:** Das System merkt sich den letzten Abholstand je Postfach;
neue Nachrichten werden per eindeutiger Message-ID erkannt (kein doppelter
Import).

**Erst-Abgleich (Backfill):** Beim Einrichten eines E-Mail-Kontos wird optional
die **Historie der letzten 90 Tage** aus dem Postfach importiert (konfigurierbar
0–365 Tage, Default 90). Backfill-E-Mails durchlaufen Zuordnung, Parsen und
Triage wie reguläre E-Mails (Badges ja) — aber mit **Abarbeitungs-Status
`erledigt` und ohne Todo-Extraktion**: Die Historie dient dem Timeline-Kontext
und der Duplikats-Vermeidung, nicht der Arbeitsverwaltung (alte E-Mails sind
längst im Altsystem beantwortet). Optional: „letzte X Tage voll verarbeiten"
(Default 3 — inklusive Todo-Extraktion und Status `offen`). Der Backfill läuft
gestaffelt im Hintergrund und ist jederzeit abbrechbar.

### 4.2 Eingangsverarbeitung & Zuordnung

Ablauf eingehende E-Mail → Aktion in 4 Stufen:

1. **Abholen** der Roh-E-Mail.
2. **Parsen:** Text/HTML extrahieren, Anhänge (max. 25 MB pro E-Mail) ablegen
   und referenzieren.
   - **Anhang-Prüfung:** Anhänge werden vor der Ablage auf Schadsoftware
     gescannt; blockierte Anhänge bleiben referenziert, sind aber gesperrt
     (Hinweis „gesperrt" in der E-Mail-Ansicht, kein Download).
   - **HTML-Darstellung:** Body-HTML wird ausschließlich bereinigt dargestellt —
     kein aktiver Inhalt (Skripte, Makros), keine externen Nachladungen
     (Bilder/Tracker) ohne explizite Benutzeraktion.
3. **Zuordnung (4-Stufen-Match):**
   0. **Exakte Absender-Adresse → bekannter Kontakt** (das häufigste Match
      überhaupt — deckt Privatkunden mit Freemail-Adressen und Bestandskunden;
      Firma wird aus dem Kontakt mitgeliefert);
   a. Absender-Domain trifft bekannte Firma (deckt den B2B-Großteil);
   b. KI extrahiert aus Inhalt/Signatur Firmenname, Domain und Kontaktdaten
      (Name, E-Mail, Telefon, Titel, Konfidenz) — für diese Funktion gilt keine
      PII-Maskierung (siehe Guardrail-Policy 8.5);
   c. Unscharfer Namensvergleich (Ähnlichkeit > 0,6) → Treffer.
   Sonst: Bei **unbekannter** Domain und plausibler Firma in Signatur/Inhalt
   schlägt das System eine Firmenneuanlage vor (HITL-Bestätigung); bei
   unbekanntem **Privatkunden** (keine Firma erkennbar) wird der Kontakt
   **ohne Firma** angelegt (Quelle „Email"); andernfalls bleibt die Zuordnung
   offen und ist manuell nachpflegbar (Inbox zeigt „nicht zugeordnet").

   **Ähnlichkeitsfunktion (zentrale Definition, auch genutzt in 5.4):**
   normalisierte Levenshtein-Ähnlichkeit (0–1) auf kleingeschriebenen,
   Umlaut-aufgelösten Namen (Firmenname bzw. Vor- + Nachname); Treffer nur,
   wenn der Wert den jeweiligen Schwellwert strikt überschreitet.
4. **Triage** (siehe 4.3), sofern KI-Fähigkeit aktiv.

Kontakte werden per E-Mail-Adresse + Firma aktualisiert oder neu angelegt
(Quelle „Email"). Alle KI-Ergebnisse sind als Vorschläge gekennzeichnet.
Flüchtige Fehler werden automatisch wiederholt; dauerhafte Fehler landen in einer
Fehlerliste mit Retry-Möglichkeit (Admin sicht- und behebbar).

**Strukturelle KI-Fehler (ungültige/unvollständige Antworten):** Jede KI-Antwort
wird gegen das erwartete Schema geprüft (Triage-Labels, Extraktionsfelder).
Bei ungültiger Struktur: **1 automatischer Retry**; schlägt auch dieser fehl,
gilt die E-Mail als flüchtiger Fehler (Retry-Queue) und nach **3 Fehlschlägen
insgesamt** als dauerhafter Fehler (Fehlerliste, 13.3). Teilergebnisse werden
verworfen — eine E-Mail wird entweder vollständig verarbeitet oder gar nicht
(kein Mischzustand aus halber Triage).

**Inbox-Ansicht:** Chronologische E-Mail-Liste über alle Konten mit
Triage-Badges (Wichtigkeit, Handlungsbedarf, Stimmung, Lead-Score, eigene Labels),
Zuordnung zu Firma/Kontakt (inkl. Abschnitt „nicht zugeordnet" für manuelle
Nachpflege), Filter nach Konto, Labels, Handlungsbedarf und Gelesen-Status, sowie
Aktionen (Draft erstellen, Todo anlegen, Zuordnung korrigieren).

**Gelesen-Status:** Jede E-Mail trägt einen Gelesen-Status (Default: ungelesen).
Öffnen der E-Mail in der Inbox setzt sie auf gelesen; der Status ist manuell
umschaltbar (gelesen/ungelesen). Der Dashboard-Wert „ungelesene Inbox" (13.1)
zählt E-Mails mit Status ungelesen.

**Abarbeitungs-Status:** Jede E-Mail trägt einen Abarbeitungs-Status
(`offen`/`in_arbeit`/`erledigt`, Default `offen`) und kann an einen Benutzer
zugewiesen werden (2.1). **„Beantwortet" wird automatisch abgeleitet:** Sobald
ein Reply-Draft auf dieser E-Mail den Zustand `sent` erreicht hat, gilt sie als
beantwortet (Badge, kein separater Zustand). Inbox-Default-Ansicht ist
„offen & unbeantwortet" — bereits abgearbeitete E-Mails bleiben über den Filter
abrufbar (kein Archiv, kein Löschen; LK-5 unberührt). Damit bleibt die Inbox
auch nach Monaten übersichtlich: Was zu tun ist, ist sichtbar.

### 4.3 Triage & eigene Labels

**Standard-Label-Matrix der KI:** Wichtigkeit (`low`/`medium`/`high`/`urgent`),
Handlungsbedarf (`needs_reply`/`no_action`/`for_info`), Stimmung
(`positive`/`negative`/`neutral`), Lead-Score 0–100, Themenkategorie, Sprache,
deutsche Kurzzusammenfassung (1–2 Sätze), explizite Deadline.

**Lead-Score (Definition & Zweck):** Die KI schätzt je E-Mail das
Vertriebsinteresse auf einer Skala 0–100 ein (Kauf-/Wechselabsicht, Dringlichkeit,
Qualität der Anfrage). Der Score ist eine **Priorisierungshilfe**: er wird als
Badge in der Inbox angezeigt, ist im Inbox-Filter und in der Sortierung nutzbar —
er löst automatisch keine Aktionen aus und ist kein berechnetes Modell, sondern
Teil der Triage-Antwort (mit derselben Kennzeichnungs-/Review-Logik wie alle
Triage-Labels).

**Themenkategorien (kontrolliertes Vokabular):** Der Admin verwaltet in den
Einstellungen eine Kategorien-Liste (frei erweiterbar, z. B. „Angebot",
„Beschwerde", „Vertragsfrage", „Sonstiges"). Die KI ordnet jede E-Mail genau
einer Kategorie zu; passt keine, wird „Sonstiges" vergeben. Damit sind Filter
und Auswertungen über Kategorien möglich (kein unkontrolliertes Vokabular).

**Eigene Labels (fachliches Kernkonzept):** Statt nur der festen Kategorien kann
der Admin **pro Postfach eigene Labels** definieren: Name, Typ (Text/Auswahl/
Ja-Nein), Auswahl-Optionen, Bedeutungsbeschreibung — plus **frei formulierte
KI-Instruktionen**, die die Einordnung steuern. Die Inbox ist nach allen Labels
dynamisch filterbar.

**Versionierung:** Alle KI-Anweisungen sind versioniert; jede KI-Aktion
protokolliert, welche Anweisungs-Version sie erzeugt hat (Nachvollziehbarkeit,
siehe 8.4).

**Manuelle Korrektur (Trust-Loop):** Alle Triage-Labels (Wichtigkeit,
Handlungsbedarf, Stimmung, Themenkategorie, Lead-Score) sind manuell
überschreibbar. Korrigierte Werte gewinnen dauerhaft gegen spätere
Neu-Bewertung derselben E-Mail (sie wird nicht erneut klassifiziert); die
Korrektur wird im KI-Audit protokolliert. Wird `needs_reply` nachträglich
gesetzt, wird das Todo (4.4) unmittelbar nachgeholt — ein KI-Fehler kann nie
dazu führen, dass ein Kunde unbeantwortet bleibt.

### 4.4 Todo-Extraktion

Bei Handlungsbedarf `needs_reply` wird automatisch ein offenes Todo angelegt:
Fälligkeit = explizit genannte Deadline, sonst **Empfangsdatum + 1 Werktag**
(Antwortpuffer — ohne Puffer wäre jede E-Mail bereits bei Ansicht überfällig).
Das Todo verlinkt auf die Ursprungs-E-Mail.

**Prioritäts-Mapping (verbindlich):** Triage-Wichtigkeit → Todo-Priorität:
`urgent` → Dringend, `high` → Hoch, `medium` → Mittel, `low` → Niedrig.

### 4.5 Drafts (Antwort-Entwürfe)

**Entstehung:** Manuell (leerer Entwurf oder aus Template, 4.6), KI-gestützt aus
E-Mails (Volltext oder Stichpunkte), aus Logbuch-Kontext (siehe 3.4) oder aus
Workflow-Schritten (siehe 6.4). Grundlage für KI-gestützte Entwürfe ist ein
**Tonalitätsprofil** aus der bisherigen E-Mail-Historie — analysiert in
zwei Scopes: **global** (Organisationsstil) und **pro Kunde** (Stilmerkmale +
Beispielsätze aus dem Verlauf mit diesem Kunden). Ein Gruppen-Scope existiert
bewusst nicht (siehe 10.3). **Fallback:** Ohne ausreichende Historie (weniger
als 5 gesendete E-Mails im Scope) wird das globale Profil verwendet; ohne jedes
Profil ein neutraler Geschäftsstil.

**Zustände:** `queued → in_review → approved → sent` bzw. `→ rejected`
(mit Review-Notizen).

**Regeln:**
- **Zwingende Review-Queue vor jedem Versand** (Human-in-the-Loop); kein
  automatischer Versand.
- Jeder Draft trägt ein „KI-gestützt"-Badge; Entwürfe enthalten einen
  Fußnote-Hinweis („KI-gestützt erstellt, finale Prüfung durch Mensch").
- Versand übernimmt Signatur-Kaskade (3.1) und sendet über das jeweilige
  E-Mail-Konto.
- **UWG-Warnung (19.4, gestuft):** Die Warnung bei fehlender E-Mail-Werbe-
  Einwilligung ist **nach Risikolage gestuft**, damit sie ernst genommen wird
  statt zur Gewohnheit zu werden:
  - **B2C-Kontakt ohne Consent:** deutliche Warnung (Verbraucher-Werbung braucht
    Opt-in; Ausnahmen nur § 7 Abs. 3 UWG Bestandskunde oder laufende
    Geschäftsbehandlung).
  - **B2B-Kontakt ohne dokumentierten Consent:** dezenter Hinweis (§ 7 Abs. 2
    UWG: bei Unternehmen genügt mutmaßliche Einwilligung).
  - **Antwort auf eine eingehende Nachricht desselben Kontakts:** keine Warnung
    (eine Antwort ist keine unaufgeforderte Werbung).
  Der Versand bleibt stets möglich (rechtliche Bewertung beim Menschen —
  HITL); Warnung und Versand werden im Audit-Log protokolliert.
  **Keine Verschleierung:** Jede ausgehende E-Mail trägt die Identität der
  Organisation (Name + ladungsfähige Adresse über die Signatur) und einen
  Hinweis auf die Widerspruchsmöglichkeit.
- **Empfänger:** genau ein Empfänger (kein CC/BCC) — bewusst einfach gehalten
  (siehe 10.3). Anhänge können übernommen werden (max. 25 MB): aus der
  Ursprungs-E-Mail **oder aus der Dokumenten-Ablage** (z. B. Angebot
  mitschicken, siehe 6.1/4.6).
- **Unbekannte Absender:** Für E-Mails ohne zugeordneten Kontakt (4.2) kann ein
  Draft mit einer **freien E-Mail-Adresse** als Empfänger erstellt werden;
  optional kann dabei inline ein Kontakt angelegt werden (Quelle „manuell").
  Damit ist der Erstkontakt-Fall vollständig abgedeckt.

**Umgang mit E-Mails (kein Löschen/Archivieren im CRM):** Importierte E-Mails
werden im CRM weder manuell gelöscht noch archiviert — sie verbleiben zusätzlich
im Ursprungs-Postfach. Die Aufbewahrung und Löschung regelt das Löschkonzept
(LK-5, siehe 9.3). Aktionen in der Inbox beschränken sich auf Zuordnung,
Labels, Draft und Todo.

### 4.6 E-Mail-Templates

Name, Betreff, Markdown-Text mit Platzhaltern (`{{contact_name}}`,
`{{account_name}}`, `{{user_name}}`, `{{date}}` u. a.), Anhänge, Vorschau mit
echten Daten. Templates werden in Workflows (6.4) und manuell genutzt.

---

## 5. Modul: KI-Agent & Research

### 5.1 Agent & Tools

Chat mit einem Assistenten, der Werkzeuge (Tools) aufruft. **11 Tools in zwei
Klassen:**

**Lesend/planend — laufen automatisch (7):**

| Tool | Wirkung |
|------|---------|
| `kb_search` | Wissensbasis per Bedeutungssuche durchsuchen |
| `read_entity_history` | letzte 30 Aktivitäten/termine/E-Mails/Fakten zu Kontakt/Firma/Deal |
| `search_crm` | CRM-Suche über Kontakte/Firmen/Deals |
| `list_deals` | Deals auflisten (optional nach Status) |
| `list_outstanding_work` | offene Todos und Follow-ups auflisten |
| `get_entity_facts` | gespeicherte Fakten einer Entität lesen |
| `schedule_recheck` | begründeten Recheck (Wiedervorlage) in 1–365 Tagen planen; fällige Rechecks werden automatisch verarbeitet und stoßen bei Firmen Research an |

**Schreibend — erzeugen einen Proposal (4):**

| Tool | Wirkung |
|------|---------|
| `crm_manage_entities` | Todo, Kontakt oder Deal anlegen |
| `record_fact` | Fakt im Evidence-Ledger erfassen — immer als Proposal; nach Freigabe Status `accepted` unabhängig vom Gewicht (siehe 5.3) |
| `templates_create_or_update` | E-Mail-Vorlage anlegen/aktualisieren |
| `workflows_create_routine` | Workflow-Routine anlegen |

**Approval-Matrix (Grundregel):** Lesende Tools laufen automatisch; **schreibende
Tools erzeugen einen Proposal** (Vorschlag) statt direkt zu schreiben (siehe 5.3).

**Chat-Verlauf:** Konversationen werden persistiert und bleiben über das
Agent-Event-Log (5.7) einsehbar (Nachrichten, Tool-Aufrufe, Entscheidungen) —
kein separates Konversations-Objekt. Der Chat ist je Entität (über das
Agent-Panel) und global (Bereich „Agent", siehe 13.2) nutzbar.

### 5.2 Evidence-Ledger (gewichtete Fakten)

Alle KI-Erkenntnisse über eine Entität werden als **gewichtete Fakten** geführt —
mit Quelle, Belegen und Konfidenz.

**Quellengewichte:** E-Mail-Signatur 0,90; Kalender-Teilnahme 0,80; verifizierte
Research-Ergebnisse 0,75; LinkedIn-Match 0,70; Web-Zitat 0,40; unbelegte
KI-Aussage 0,20. Weitere Quellen (z. B. Konnektor-Ergebnisse, siehe 7.1) erhalten
konfigurierbare Gewichte unterhalb der Auto-Schwelle, sodass sie stets als
Vorschlag enden.

**Schwellen:** ≥ 0,70 → automatisch gespeichert (Status `auto`); 0,30–0,70 →
Vorschlag an den Menschen (HITL, Status `suggested`); < 0,30 → verworfen.
**Geltungsbereich der Schwellen:** nur für **automatische Fakt-Quellen**
(Research, E-Mail-/Transkript-Extraktion, Konnektoren). Vom Agenten über
`record_fact` erfasste Fakten durchlaufen stattdessen das Proposal-Verfahren
(siehe 5.3) — dort entscheidet allein der Mensch, nicht das Gewicht.

**Fakt-Status:** `auto` / `suggested` / `accepted` / `rejected`. Abgelehnte und
manuell korrigierte Fakten bleiben dokumentiert (manuelle Korrektur gewinnt immer
gegen spätere KI-Vorschläge).

### 5.3 Proposals & Human-in-the-Loop

**Proposals** sind persistente Vorschläge (Tool, Vorhaben, Zusammenfassung) mit
Status `pending` / `accepted` / `rejected` / `expired` (TTL **48 Stunden** —
Vertriebsalltag hat kein Chat-Tempo; Nutzer sitzen in Kundengesprächen und
dürfen Vorschläge nicht ungelesen verlieren). 12 Stunden vor Ablauf erzeugt ein
offener Proposal eine Erinnerungs-Benachrichtigung (14); abgelaufene Proposals
bleiben im Agent-Panel einsehbar und sind mit einem Klick **reaktivierbar**
(neues Proposal, frisches TTL). Die Entscheidung des Nutzers übernimmt oder
verwirft den Vorschlag; übernommene Fakten werden zu `accepted`-Fakten im
Evidence-Ledger.

**Zustandsübergang bei `record_fact` (explizit):** Approval des Proposals →
der Fakt wird direkt `accepted` — **unabhängig vom Quellgewicht** (die
menschliche Bestätigung ist die Freigabe; keine zweite Bewertungsstufe).
Ablehnung → kein Fakt-Eintrag, das Proposal wird `rejected`. Damit gibt es pro
Agent-Fakt genau eine HITL-Entscheidung, nicht zwei.

### 5.4 Research (automatische Recherche)

**Research-Job** = automatische Unternehmens-/Personenrecherche (Firma,
Geschäftsführung, Entscheider, Produkte, News) pro Kunde.

**Aktivierung (Ort der Schalter):** Research hat einen **globalen Master-
Schalter** in den KI-Einstellungen (8.9, „Research aktiv"); ist er aus, findet
kein Research statt und die zugehörigen UI-Elemente erscheinen nicht (1.5).
Zusätzlich gibt es das **Research-täglich-Flag pro Firma** (12.1) und den
manuellen Button pro Firma.

**Trigger (nur bei aktiviertem Research):** neu angelegte Firma (automatisch);
täglicher Lauf (03:00 Uhr) nur für als „täglich" markierte Firmen; manueller
Button „Jetzt recherchieren"; Ad-hoc-Recherche ohne Kundenbezug.

**Wissenslücke (Definition):** Pro Firma wird die Wissensbasis gegen ein
Soll-Profil geprüft: Pflicht-Wissensfelder `Geschäftstätigkeit`, `Führung`
(Geschäftsführung/Entscheider), `Produkte/Leistungen`, `News`. Ein Feld gilt
als Lücke, wenn es **leer** ist oder der letzte Eintrag älter ist als die
Feld-Frist: News 30 Tage, alle anderen Felder 90 Tage. Jede Lücke erzeugt
Suchanfragen, bis das Kontingent erreicht ist.

**Ablauf (6 Stufen):**
1. Planer ermittelt die Wissenslücken je Firma (Regel oben).
2. Max. 5 Suchanfragen pro Job.
3. Meta-Suche (selbst gehostet; max. 10 Treffer/Anfrage, 24h-Dubletten-Cache).
4. Seiten-Abruf (Standard-Scraper; JS-lastige Seiten über Browser-Scraper;
   LinkedIn experimentell, per Feature-Flag, strikt gedrosselt, nur einzeln
   freigegebene URLs). **SSRF-Schutz:** Der Abruf blockiert intern
   erreichbare Adressen (private IP-Bereiche, Loopback, Link-Local,
   Cloud-Metadaten-Endpunkte, eigene Installations-URL) — Research darf nie
   als Sprungbrett ins interne Netz missbraucht werden können.
5. KI-Extraktion: Personen (Rolle, Bio), Firmen (Größe, Umsatz), Produkte, News.
6. Dedup/Merge: Quell-URL eindeutig; Namensähnlichkeit > 0,7 (Ähnlichkeits-
   funktion gemäß 4.2) → Zusammenführung; neuere Daten gewinnen; manuelle
   Korrekturen bleiben dokumentiert.

Ergebnisse fließen als Fakten in das Evidence-Ledger (Gewicht 0,75 verifiziert /
0,40 Web-Zitat) und in die Wissensbasis (5.5).

**Anreicherungs-Status:** Firmen tragen einen sichtbaren Anreicherungs-Status
(nicht angestoßen / läuft / abgeschlossen / fehlgeschlagen, mit Zeitstempel des
letzten Laufs), damit Benutzer Research-Zustand und -Alter sofort erkennen.

### 5.5 Wissensbasis (KB)

Pro Kunde geführte Wissenssammlung: Inhalt, Zusammenfassung, Titel, Quell-URL,
Entity-Tags, Herkunft (Recherche/E-Mail/Anruf). **Datenschutz:** Roh-HTML wird
nach 90 Tagen gelöscht — aufbereiteter Inhalt bleibt. Suche: semantisch (RAG)
plus Volltext. Keine automatische Aktualisierung — nur über Research-Trigger.

### 5.6 Transkription

Ablauf: Audio-Upload (Datei, Dauer, Sprache) → automatische Transkription (immer
gekennzeichnet „automatisch transkribiert – Irrtümer möglich", Sprecher-Erkennung,
Konfidenzwert) → Annotation: Verknüpfung mit Kunde/Firma und daraus abgeleitete
Todos. Unbelegte KI-Aussagen aus Transkripten fließen als schwache Evidenz
(Gewicht 0,20) in das Evidence-Ledger ein.

**Upload-Limits:** Formate mp3, m4a, wav, ogg; maximale Dauer 60 Minuten;
maximale Dateigröße 200 MB. Überschreitungen werden mit Hinweis abgelehnt
(Vertriebsanrufe bleiben damit vollständig abgedeckt; Kosten-/Laufzeit-Kontrolle
für die Transkription).

### 5.7 Agent-Transparenz (EU AI Act)

**Ereignis-Log pro Datensatz** (Kontakt/Firma/Deal) mit Event-Typen: `tool_call`,
`fact_auto`, `fact_suggested`, `proposal_created`, `proposal_decided`,
`research_started`, `lead_discarded`, `question`, `recheck_scheduled` — jeweils
mit Agent-Nachricht und Detail.

**Panel:** Tab „Agent" in jeder Detailansicht als Timeline mit Badges und
aufklappbaren Details; Leerzustand „Keine Agent-Aktivität".

**Fachliche Pflichten aus der EU AI Act:**
- Art. 14 (menschliche Aufsicht): keine KI-Aktion mit Folgewirkung ohne
  Bestätigungsstufe. **Ausnahmen (dokumentiert):** Research (keine
  Kundenkommunikation) sowie rein interne Ableitungen ohne Außenwirkung —
  Todo-Extraktion (4.4) und Recheck-Verarbeitung (5.1) — diese erzeugen
  interne, jederzeit lösch- bzw. korrigierbare Arbeitsobjekte.
- Art. 50 (Transparenz): jede KI-Ausgabe sichtbar und maschinell lesbar
  gekennzeichnet („KI-gestützt"-Badges, Disclaimers).
- Risikoklasse-Eintrag pro KI-Aktion (Standard „limited risk"); kein
  selbstlernendes System ohne Einwilligung.

---

## 6. Modul: Dokumente & Daten

### 6.1 Dokumenten-Ablage

Upload pro Firma (Titel, Kategorie, Dateityp, Größe) mit Filter- und
Downloadansicht. **Kategorien:** Angebot, Vertrag, Vollmacht, Rechnung,
Korrespondenz, Sonstig. KI schlägt beim Upload Kategorie + Verknüpfung vor
(HITL). Upload/Löschung erzeugt Logbuch- und Audit-Eintrag. Downloads nur für
autorisierte Benutzer (beide Rollen). Keine Versionierung.

**Upload-Limits:** max. 25 MB pro Datei (konsistent mit E-Mail-Anhängen, 4.2).
Erlaubte Dateitypen: PDF, Office-Dokumente (doc/docx/xls/xlsx/ppt/pptx), Bilder
(jpg/png/gif/webp), Text/CSV (txt/csv), E-Mail-Formate (eml/msg). Andere Typen
werden mit Hinweis abgelehnt.

### 6.2 Import

**Quellen:** CSV und XLSX. Eine Zeile kann atomar verknüpfte Objekte erzeugen:
Firma, Ansprechpartner, Aktivität/Logbuch, Wiedervorlage/Todo, Energiedaten,
Kalendertermin, **Custom-Field-Werte** (für Firma/Kontakt — Mapping auf
Feld-Schlüssel, Validierung gemäß Custom-Field-Definition: Typ, Optionsliste,
Pflicht; ungültige Werte erzeugen Validierungsfehler in Schritt 4). Berechtigt:
nur Admin.

**Ablauf (5-Schritt-Wizard):**
1. **Upload** (max. 20 MB / 10.000 Datenzeilen; Zweck, Quelle und Rechtsgrundlage
   müssen dokumentiert werden).
2. **Strukturerkennung** (Blätter, Kopfzeile, Datenbeginn).
3. **Mapping** — deklarative Regeln (Quellspalte → Zielfeld, Transformationen wie
   E-Mail-/Telefon-Normalisierung, Wertetabellen, Konstanten). Keine freien
   Skripte, keine KI-Entscheidungen. Wiederverwendbare, versionierte Vorlagen;
   mitgelieferte Systemvorlagen kopierbar.
4. **Prüfung** — Vorschau, Validierungsfehler, manuelle Zellkorrektur,
   Dubletten-Erkennung.
5. **Import** — versionierter, gesperrter Plan; keine Änderung ohne neue
   Freigabe.

**Dedup-Kaskade (Trefferreihenfolge):** externe Referenz → E-Mail → Telefon →
Website-Domain → Firmenname+PLZ → Firmenname+Ort. Jede Dublette erfordert eine
explizite Entscheidung (zusammenführen / neu anlegen / überspringen). Leere
Felder werden ergänzt, **bestehende Werte niemals ungefragt überschrieben**;
Konflikte feldweise entscheidbar; erneute Prüfung unmittelbar vor dem Schreiben.

**Rollback & Aufbewahrung:** Rollback 30 Tage möglich, konfliktbewusst (schützt
spätere manuelle Änderungen). Roh-/Staging-Daten standardmäßig 30 Tage, danach
nur minimierte Laufmetadaten; sofortige Löschung möglich (beendet Rollback).

### 6.3 Export & Reporting

- **DSGVO-Übertragbarkeit (Art. 20):** maschinenlesbarer Export als JSON/CSV je
  Entitätstyp (Kontakte, Firmen, Deals, Todos, Termine, Dokumente-Metadaten).
- **Auskunft:** „Datenexport für Person X" im Admin-Bereich; der verbindliche
  Umfang ist in der Löschbegehren-Mechanik (9.3) definiert.
- **Asynchrone Export-Jobs** mit Typ + Filter, Download nach Fertigstellung.
- Protokolle (Zugriff/Änderung) als CSV/JSON exportierbar.
- **Reporting (fachlich):** Auswertungen entstehen aus drei Bausteinen —
  Dashboard-Kennzahlen (13.1, 7.6 Vertriebs-Dashboard), dem **Vertriebs-Report**
  (siehe unten) und asynchronen Export-Jobs (beliebiger Typ + Filter). Es gibt
  kein separates Berichtswesen-Modul; Finanz-/Provisions-Reports entfallen mit
  den Ausschlüssen (1.4).
- **Vertriebs-Report (Forecast & Zeitreihe):** Zwei Standard-Auswertungen als
  Dashboard-Widgets (Admin-Ansicht 13.1, Werte je Rolle einsehbar):
  - **Forecast (erwartete Werte):** Summe der monatlichen Werte aller `open`
    Deals, gruppiert nach erwartetem Abschlussmonat (Abschlusszeitpunkt, bei
    `open`-Deals gesetzt oder geschätzt) — plus `won`-Werte mit
    Vertragsbeginn im Zeitraum als „gesichert" daneben.
  - **Zeitreihe Abschlüsse:** Summe der Einmal- und Monatswerte abgeschlossener
    (`won`) Deals je Monat als Tabelle/einfache Kurve der letzten 12 Monate,
    inkl. Anzahl Abschlüsse und Wandlungsquote (won : (won+lost+widerrufen)).
  Beide Auswertungen sind rein lesbar aus dem Deal-Datenmodell (3.3/12.2),
  ohne eigene Entitäten, und über den Export-Job (Typ `vertriebsreport`,
    Filter Zeitraum) als CSV abrufbar.

### 6.4 Workflows & Automationen

**Workflow** = vordefinierte Schrittfolge pro Firma/Kontakt.

**Start:** manuell durch einen Benutzer auf einer Firma/einem Kontakt, oder als
System-Vorschlag bei Deal-Status `won` (3.3). **Kein vollautomatischer Start**
ohne Benutzeraktion — der erste Schritt eines Laufs erfordert immer eine
menschliche Bestätigung (HITL, konsistent mit 9.4).

**Schritt-Aktionen:** E-Mail-Entwurf (aus Template, mit Verzögerung in Tagen),
Warten (Tage), Antwort prüfen (mit Wenn-keine-Antwort-Ersatzaktion), Todo
erstellen (Titel/Priorität), Termin erstellen (Typ/Dauer).

**Versandkonto für Workflow-Drafts:** Ein Workflow nutzt das pro Workflow
konfigurierte E-Mail-Konto; ist keines konfiguriert, das **primäre aktive
Konto** (4.1). Existiert kein primäres aktives Konto, ist der Entwurfs-Schritt
blockiert und der Lauf pausiert mit Hinweis (kein stiller Fehler).

**Versionierung (laufende Läufe):** Ein Lauf arbeitet mit einem **Snapshot der
Workflow-Definition zum Startzeitpunkt**; Änderungen an der Workflow-Definition
betreffen nur neu gestartete Läufe, niemals aktive. Dasselbe gilt für Templates:
ein vorbereiteter Schritt (`prepared`) enthält den **Template-Inhalt zum
Zeitpunkt der Vorbereitung** (Kopie); spätere Template-Änderungen wirken nicht
in bereits vorbereitete Schritte. Damit brechen Definition-/Template-Änderungen
niemals laufende Automationen.

**Ausführung semi-automatisch (HITL):** Das System bereitet den fälligen Schritt
vor (Status `prepared`) → der Benutzer bestätigt & sendet, bearbeitet oder
überspringt → nächster Schritt.

**UWG-Regel für Outreach-Schritte:** E-Mail-Schritte an Kontakte ohne
E-Mail-Werbe-Einwilligung (3.1) werden bei der Vorbereitung mit der UWG-Warnung
(4.5) markiert — der Schritt bleibt ausführbar (HITL-Entscheidung beim
Benutzer), Warnung + Entscheidung werden protokolliert. Damit bleibt
Workflow-Outreach nachweisbar einwilligungsorientiert (siehe 19.4).

**Zustände:** Schritt: `pending`/`prepared`/`confirmed`/`skipped`/`done`;
Lauf: `active`/`paused`/`completed`/`cancelled` (pausieren/fortsetzen/abbrechen).

**Antwort-Prüfung (explizite Regeln):**
- Als Antwort zählt jede eingehende E-Mail des **Kontakts oder eines anderen
  Absenders derselben Firma** (Domain-Match), die **nach** der Vorbereitung des
  Schritts (Zeitstempel ≥ `prepared`) eingeht.
- Reine Auto-Replies (Abwesenheits-, Bestätigungs-Automaten) zählen nicht —
  Erkennung über übliche Auto-Reply-Kennzeichen (Header/Subject-Muster).
- Evaluation erfolgt **fortlaufend bei jedem E-Mail-Eingang**: eine Antwort
  beendet das Warten sofort (Schritt → `done`, Lauf setzt fort).
- Die Wenn-keine-Antwort-Ersatzaktion feuert **nur**, wenn die Wartezeit
  vollständig verstrichen ist, ohne dass eine Antwort einging.

**Vordefinierte Workflows (mitgeliefert, anpassbar):** Erstkontakt (E-Mail →
3 Tage warten → Nachfass-Todo), Vollmacht-Anfrage (E-Mail → 7 Tage → Antwort-Check
mit Nachfass-E-Mail → 14 Tage → Anruf-Todo hoch), Wiedervorlage (7 Tage warten →
Todo + E-Mail), Nachfassung (E-Mail → 7 Tage → Anruf-Todo).

### 6.5 Website-Tracking & Form-Intake

**Tracking:** First-Party-Snippet für eigene Websites; Domains werden im Admin
konfiguriert (aktiv/inaktiv/pausierbar). Getrackt werden:
- **Besucher:** anonyme ID, erster/letzter Besuch, Seitenaufruf-Zähler, Referrer,
  UTM-Quelle/Medium/Kampagne (nur letzter Wert).
- **Events:** `page_view`, `form_submit`, `lead_qualified` mit Pfad.

**Consent-Gate (DSGVO + TDDDG, siehe 19.3):** ohne Einwilligung nur anonyme
Seitenaufrufe (kein Referrer/UTM) und **keinerlei Speicherung auf dem Endgerät**
(kein Cookie, kein localStorage-Eintrag, keine gerätebezogene ID —
Endgeräte-Zugriff nur mit Einwilligung); der Besucher-Zähler läuft dann
serverseitig ohne Gerätebezug. Bot-Filter und Rate-Limit; Formular-Einreichungen
erfordern zwingend Einwilligung.

**Formulare (konfigurierbar):** Der Admin verwaltet **beliebig viele
Intake-Formulare** je Tracking-Domain — jedes mit eigenem Namen, Zweck und
Feldliste: Pflichtfeld E-Mail + Consent sowie optional weitere Felder (Name,
Firma, Nachricht, Telefon, Auswahlfelder). Damit lassen sich verschiedene
Kanäle abbilden („Beratung anfragen", „Rückruf wünschen", „Newsletter"). Jede
Submission speichert Formular-Referenz, Consent-Version und Feldwerte.
Standard-Formular ist eins mit Name/Firma/Nachricht (der bisherige
Form-Intake-Zustand).

**Form-Intake (Lead-Erfassung von der Website):** Spam-Muster → `ignored`.
E-Mail-Dedupe: bekannter
Kontakt → `matched`; sonst Neuanlage Kontakt mit Quelle „Web-Intake".
**Firmen-Zuordnung:** nur zu einer **bestehenden** Firma per E-Mail-Domain; bei
unbekannter Domain bleibt der Kontakt ohne Firma (keine automatische
Firmenneuanlage durch Intake — Firmenneuanlage erfordert die Pflichtfelder aus
3.2 und geschieht manuell oder per E-Mail-Matching-Vorschlag 4.2).
Zustand → `contact_created`. **Submission-Status:** `new` / `matched` /
`contact_created` / `ignored`.

**Consent-Nachweis (DSGVO-Beweiskraft):** Jede Submission speichert neben
`Consent (Ja/Nein)` und `eingegangen am` zusätzlich die **Consent-Version**
(Hash des zum Zeitpunkt angezeigten Einwilligungstextes). Consent-Texte werden
versioniert verwaltet; so ist nachweisbar, **welcher** Text **wann** akzeptiert
wurde.

Seitenaufrufe erscheinen als „Story" im Firmen-Tab „Website"; Kontakte tragen ein
Herkunfts-Badge (Quelle „Web-Intake").

---

## 7. Modul: Konnektoren & Vertikalen (Referenz: Energie/Feldvertrieb)

Dieses Modul enthält die **generische Konnektor-Schiene** (branchenneutral)
und die **erste Vertikale** — Energie- und Feldvertrieb — als
Referenzimplementierung und Implementierungspriorität (§17).

### 7.1 Generische Konnektor-Schiene

**Konzept:** Ein **Konnektor** ist eine angebundene externe Daten- oder
Funktionsquelle (API), deren Ergebnisse normalisiert, aufbereitet und direkt
in der Kundenansicht angezeigt werden — **Daten-Konnektoren** (Tarifrechner,
Auskunfteien, Marktdaten, Branchen-APIs) und **Funktions-Konnektoren**
(Telefonie). Energie/Tarifrechner ist der erste Daten-Konnektor, Telefonie der
erste Funktions-Konnektor; weitere folgen demselben Muster.

**Erweiterungsmechanismus (Entscheidung):** Ein Konnektor-Typ ist ein
**implementierter Adapter** (Entwicklung), kein konfigurierbares Mapping. Der
Admin wählt einen vorhandenen Typ, hinterlegt Credentials und aktiviert ihn —
das Ausgabe-Schema (aufbereitete Ansicht) ist je Typ fest definiert. Ein neuer
Konnektor-Typ ist damit bewusst eine Entwicklungsaufgabe; „frei konfigurierbare
API-Mappings" sind ausgeschlossen (YAGNI — sie wären ein Integrationsprojekt
für sich und passen nicht zur Einfachheits-Prämisse).

**Fachliche Anforderungen an jeden Konnektor:**
- Admin konfiguriert: Typ, Zugangsdaten/Endpunkt, aktiv/inaktiv.
- Ergebnisse werden **pro Kunde** (Firma/Kontakt) dargestellt (Daten) bzw.
  ausgeführt (Funktionen) — als strukturierte, lesbare Ansicht, nie als
  Roh-JSON.
- Daten-Konnektoren: Abruf on-demand (Button) oder bei definierten Triggern;
  Ergebnisse werden mit Zeitstempel und Quelle gespeichert
  (Nachvollziehbarkeit).
- Fehler der externen API werden gracefully angezeigt (kein Absturz, Hinweis).
- Konnektor-Ergebnisse können als Fakt in das Evidence-Ledger fließen; das Gewicht
  ist je Konnektor-Typ konfigurierbar und liegt bewusst unter der Auto-Schwelle
  0,70 (Default 0,65), sodass Konnektor-Daten immer als Vorschlag beim Menschen
  landen (siehe 5.2).

### 7.1a Telefonie-Konnektor (Funktions-Konnektor)

Telefonie ist ein Standardkanal des Vertriebs (neben E-Mail). Die Anbindung
erfolgt als **Funktions-Konnektor** über die Konnektor-Schiene — der konkrete
Telefonie-Anbieter ist konfigurierbar (z. B. SIP/VoIP-Provider mit
Click-to-Call-API; ohne aktiven Konnektor bleibt Telefonie ein manuelles
Aktivitätsfeld).

- **Click-to-Call:** Telefonnummern an Kontakten/Firmen sind klickbar —
  wählt über den konfigurierten Anbieter; ein Klick erzeugt automatisch eine
  `call`-Aktivität (Logbuch) mit laufender Zeit und leerem Notizfeld.
- **Anruf-Notiz:** Nach Gesprächsende wird die Aktivität direkt am Kontakt
  mit Ergebnis befüllbar (Notiz; optional KI-Extraktions-Chips wie bei
  Freitext-Notizen, 3.4).
- **Eingehende Anrufe** (falls vom Anbieter signalisiert): Matching über die
  Rufnummer (Stufe-0-Logik analog 4.2) öffnet den passenden Kontakt mit
  Anruf-Notiz-Aufforderung; unbekannte Nummern erzeugen einen Eintrag in der
  Aktivitäten-Übersicht zur manuellen Zuordnung.
- **Datenschutz:** Keine Aufzeichnung von Gesprächsinhalten; protokolliert
  werden nur Nummer, Zeitpunkt und Dauer (Log-Hygiene 8.4).

### 7.2 Energiedaten

**Strukturierte Maske am Kontakt:** Zählernummer, Messstellenbetreiber (MSB),
Verbrauch Strom (kWh), Verbrauch Gas (kWh), aktueller Anbieter, Vertragslaufzeit
bis, Kündigungsfrist (Monate), Zählerstand-Ablesedatum.

**Validierung:** kWh als positive Zahl, Zählernummer-Format. Die Maske ist in
Setter- und Closer-Views (7.6) sowie in der Kontakt-Detailansicht eingebettet.

**Automatische Kündigungsfrist-Wiedervorlage:** Sind Vertragslaufzeit und
Kündigungsfrist erfasst, erzeugt das System automatisch den Vorschlag einer
**terminierten Wiedervorlage** „Kündigungsfrist läuft ab" — fällig zum frühest
möglichen Kündigungszeitpunkt (Fristende minus Kündigungsfrist Monate minus
1 Monat Vorlauf; Vorlauf konfigurierbar). Der Vorschlag wird einmalig per
Benachrichtigung vorgestellt (HITL): bestätigt → dauerhaftes Todo mit
Fälligkeit; verworfen → keine Wiederholung für diesen Vertrag.

### 7.3 Tarifrechner (Energie-Konnektor)

Eingabe: PLZ + Verbrauch → Ausgabe: Top-3-Tarife, jährliche Ersparnis gegenüber
dem aktuellen Tarif, CO₂-Einsparung. Die Anbindung ist als Konnektor (7.1)
implementiert — der konkrete Tarif-API-Anbieter ist konfigurierbar. Die Ausgabe
wird im Closer-View (7.6) und in der Kontakt-Detailansicht aufbereitet angezeigt.

**Ergebnisvertrag bei unvollständigen Daten:**
- Weniger als 3 Tarife verfügbar → verfügbare Tarife anzeigen mit Hinweis
  („nur X Tarife gefunden").
- Kein CO₂-Wert geliefert → CO₂-Zeile entfällt (kein Platzhalterwert).
- Kein aktueller Tarif des Kontakts bekannt (Energiedaten leer) → Ersparnis
  entfällt, Tarife werden trotzdem angezeigt.
- API nicht erreichbar/Zeitüberschreitung → Fehlerhinweis + Zeitstempel des
  letzten erfolgreichen Abrufs (falls vorhanden); kein Absturz, kein Retry-Loop
  in der UI.

### 7.4 Smart-Map & Gebiete

**Karte** (OpenStreetMap-basiert, ohne externe API-Keys): Firmen/Kontakte als
farbcodierte Pins nach **Vertriebsstufe** (7.6; Mapping: `lead`=offen/grau,
`setter_gespraech`=besucht/blau, `closer_termin`=heiss/orange,
`abgeschlossen`/`verloren`=erledigt/grün bzw. schwarz).

**Features:** Gebiete per Polygon abgrenzen, Cluster für viele nahe Marker, Klick
auf Marker → Detail-Sidepanel (Kontakt- oder Firmen-Panel je nach Pin-Typ),
Routenoptimierung (Sortierung nach nächstgelegenem Ziel; Distanzberechnung aus
Geokoordinaten).

**Voraussetzung:** Geokoordinaten an Firmen (automatisches Geocoding, siehe 3.2)
und Kontakten (siehe 3.1); der D2D-Fortschritt (ehemaliger Besuchsstatus) ist
in der Vertriebsstufe am Kontakt abgebildet (siehe 3.1/7.6).

### 7.5 Geofencing & Fraud-Prävention

**Geltungsbereich:** Geofencing gilt **ausschließlich für Deals mit Abschlussart
`d2d`** (Außendienst-Abschluss vor Ort beim Kontakt). Deals mit Abschlussart
`buero` (Default — z. B. Abschluss durch den Closer im Büro, Desktop, ohne GPS)
sind jederzeit und überall ohne Check-In möglich.

**GPS-Check-In (nur D2D):** Mobil per Geräte-Standort; Abgleich mit den
Geokoordinaten des Kontakts; Radius konfigurierbar (Default 100 m). **Nur ein
gültiger Check-In schaltet die Anlage eines D2D-Deals frei.**

**Gültigkeitsfenster:** Ein Check-In gilt für **2 Stunden** und nur für den
eingecheckten Kontakt; innerhalb des Fensters können mehrere D2D-Deals zu
diesem Kontakt angelegt werden. Offline aufgezeichnete Check-Ins starten das
Fenster erst **mit der Synchronisation** (7.6) — ein Funkloch lässt die
Gültigkeit nicht verfallen. Danach ist ein neuer Check-In nötig.

**GPS-Ausnahme (Admin-Override):** Für legitime Fälle ohne GPS-Empfang
(Funkloch, Keller, defektes Gerät) kann ein Admin einen einzelnen D2D-Deal
nachträglich ohne gültigen Check-In freigeben — nur mit Pflicht-Begründung und
Audit-Eintrag. Der Deal wird im Fraud-Log als „Override" markiert.

**Fraud-Protokoll:** Jeder Check-In/Vertrag speichert Position, Genauigkeit und
Zeitstempel. Abweichungen (außerhalb Radius, fehlende GPS-Daten, Overrides)
landen in einem Fraud-Log. Admin-Dashboard zeigt auffällige Muster (z. B.
ungewöhnlich viele Abschlüsse in kurzer Zeit, Abschlüsse ohne GPS-Daten,
Override-Häufung pro Benutzer).

### 7.6 Setter/Closer-Workflow

Zweistufiger Vertrieb als **Views** (nicht als Rollen — jeder Benutzer kann beide
Views nutzen):

**Setter-View (Schnellerfassung):** Minimale Felder: Name, Adresse, Telefon
(E-Mail optional — D2D-Leads starten oft nur mit Telefon, siehe 3.1) +
Energiedaten-Maske (7.2) + „Termin für Closer buchen" (Kalender-Integration
3.6 — **mit Closer-Auswahl**; das Feld „zugewiesen an" am Termin wird gesetzt;
Vorschlag-Default: der Benutzer mit den wenigsten Terminen am gewählten Tag,
überschreibbar). Ziel: Erfassung in < 30 Sekunden. Bei Eingabe von
E-Mail/Telefon wird der Bestand geprüft (Dubletten-Erkennung wie in der
Dedup-Kaskade 6.2); ein erkannter bestehender Kontakt wird übernommen statt
dupliziert.

**Offline-Fähigkeit (Außendienst-Realität):** Ohne Netzverbindung werden
Erfassung **und D2D-Deal-Anlage** lokal zwischengespeichert (die Deal-Anlage
prüft den am Gerät gültigen Check-In offline; Synchronisation startet das
2h-Fenster, 7.5) und bei wiederhergestellter Verbindung automatisch
synchronisiert (Konfliktregel: letztes Speichern gewinnt, 16.3; der Sync wird im
Logbuch vermerkt). GPS-Check-Ins werden offline aufgezeichnet und nachträglich
übermittelt; für Fälle ohne GPS-Empfang existiert der Admin-Override (7.5).

**Closer-View (Abschluss):** Alle Setter-Daten read-only + Einsparrechner (7.3)
+ Abschluss-Erfassung (Deal, siehe 3.3; ggf. geofencing-gesperrt, siehe 7.5).

**Vertriebs-Pipeline (Datenfundament):** Die Stufen Lead → Setter-Gespräch →
Closer-Termin → Abschluss sind als **Vertriebsstufe am Kontakt** modelliert
(siehe 12.1; Werte `lead`/`setter_gespraech`/`closer_termin`/`abgeschlossen`/
`verloren`). **Stufenwechsel:** Setter-Schnellerfassung setzt `setter_gespraech`;
gebuchter Closer-Termin setzt `closer_termin`; Deal `won` mit diesem Kontakt
setzt `abgeschlossen`; `verloren` ist manuell setzbar. Jeder Wechsel erzeugt
einen Logbuch-Eintrag und aktualisiert den Stufen-Zeitstempel.

**Management-Dashboard (Admin):** Conversion-Rate pro Stufe (aus
Vertriebsstufen-Verteilung + Zeitstempeln), Durchlaufzeiten zwischen
Stufenwechseln, Aktivität pro Benutzer.

---

## 8. Modul: Security & Betrieb

### 8.1 Authentifizierung & 2FA

- Login: E-Mail + Passwort; bei aktivem 2FA zusätzlich 6-stelliger TOTP-Code
  (Authenticator-App) oder Einmal-Recovery-Code.
- **2FA ist optional** (Produktentscheidung, siehe 10.3): Jeder Benutzer kann
  TOTP in seinen Einstellungen frei aktivieren und deaktivieren — empfohlen,
  aber nicht erzwungen. Es gibt keinen Setup-Zwang und keine Grace-Period.
- **Setup (bei Aktivierung):** QR-Code scannen, Code bestätigen, danach 10
  Einmal-Recovery-Codes (Format XXXXX-XXXXX), nur einmalig angezeigt,
  regenerierbar (invalidiert alte). **Deaktivierung nur mit gültigem TOTP-Code**
  (Schutz vor unbemerktem Entfernen).
- **Schutz:** max. 5 Fehlversuche (Login und 2FA) → 5 Minuten Sperre.
  Session-Ablauf nach 24 Stunden. Passwortregeln: min. 8 Zeichen, 1 Zahl,
  1 Sonderzeichen.
- **Sitzungs-Invalidierung:** Jede Passwort-Änderung, jeder Admin-Passwort-Reset
  und jede 2FA-Aktivierung oder -Deaktivierung invalidiert **alle bestehenden
  Sitzungen** des Benutzers (sofortiger Re-Login erforderlich).
- **Öffentlich erreichbare Endpunkte (abschließende Liste):** Login,
  Einladungsannahme, Tracking-Endpunkt und Form-Intake (beide aus 6.5 — das
  Website-Snippet muss ohne Anmeldung Daten liefern können).
  **Sicherheitsregel für Tracking/Intake:** Diese Endpunkte dürfen niemals
  authentifizierte Daten lesen oder schreiben und keine CRM-Daten abfragen;
  Pflicht sind Rate-Limit, Bot-Filter und Honeypot-Felder gegen Spam. Alle
  übrigen Bereiche erfordern Anmeldung.
- **Passwort vergessen:** kein E-Mail-Selbst-Reset (bewusst, Einfachheit +
  Sicherheit). Der Admin setzt das Passwort des Benutzers zurück → dieser muss
  beim nächsten Login das Passwort ändern und 2FA neu einrichten.
- **Notfall-Wiederherstellung:** Ist der einzige Admin ausgesperrt (Passwort und
  Recovery-Codes verloren), setzt ein berechtigter lokaler Systemeingriff
  (Server-Konsole) Admin-Passwort und 2FA zurück. Der Vorgang wird im Audit-Log
  protokolliert.

### 8.2 API-Tokens

Maschineller API-Zugriff im Namen eines Benutzers. Lebenszyklus: Erstellen mit
Name + optional Ablaufdatum → Klartext-Token wird **einmalig** angezeigt (nur Hash
gespeichert) → Nutzung als Bearer-Token (letzte Verwendung dokumentiert) →
Widerruf; widerrufene/abgelaufene Tokens sofort ungültig.

**Scopes:** feingranular im Muster `modul.aktion`; leere Scopes = alle Rechte des
zugehörigen Benutzers. **Grundregel:** Scopes können die Rechte des Token-
Inhabers nur einschränken, niemals erweitern — ein Token gewährt nie mehr Rechte
als die Rolle seines Inhabers (2.2). Admin-exklusive Operationen (z. B. Import,
6.2) sind daher nur mit Tokens von Admins nutzbar, unabhängig vom Scope.
**Verbindlicher Scope-Katalog:**

| Modul | Aktionen |
|-------|----------|
| `contacts` | `read`, `write` |
| `accounts` | `read`, `write` |
| `deals` | `read`, `write` |
| `todos` | `read`, `write` |
| `events` | `read`, `write` |
| `documents` | `read`, `write` |
| `emails` | `read` (Inbox/Triage-Zustände), `write` (Zuordnung, Labels) |
| `drafts` | `read`, `write` (inkl. Freigabe/Versand) |
| `workflows` | `read`, `write` (Läufe starten/pausieren) |
| `research` | `read`, `write` (Jobs anstoßen) |
| `import` | `write` |
| `export` | `read` |

Admin-exklusive Bereiche (Benutzer-Verwaltung, Guardrails, Backup, Webhooks,
API-Tokens selbst) sind **nicht** per API-Token zugänglich.

**API-Umfang (fachlich):** Die API bildet die UI-Fähigkeiten der Module 3–8 je
Modul ab (CRUD, Suche, Import/Export, Triage-Aktionen); es gibt keine
ausschließlichen API-Fähigkeiten ohne UI-Äquivalent.

### 8.3 Webhooks (Outbound)

Benachrichtigung externer Systeme bei CRM-Ereignissen (nur ausgehend, kein
Empfang).

**Events:** Deals (angelegt/geändert), Kontakte (angelegt/geändert), Firmen
(angelegt/geändert), Termine (angelegt), Todos (angelegt/erledigt).

**Auslieferung:** Jede Auslieferung erhält eine HMAC-SHA256-Signatur über den
Payload (pro Webhook eigenes Secret) — Empfänger prüft Echtheit/Integrität.
5 Wiederholungen mit wachsendem Abstand. Delivery-Status (ausstehend/ausgeliefert/
fehlgeschlagen) mit Fehlerursache und letzter Auslieferung; Testauslieferung in
der UI. Keine Nach-Wiedergabe vergangener Events.

**Payload-Schema (verbindlicher Vertrag):** Jede Auslieferung ist ein
JSON-Objekt mit festem Umschlag:

| Feld | Inhalt |
|------|--------|
| `event` | Event-Typ gemäß Event-Katalog oben (z. B. `deal.created`) |
| `timestamp` | Zeitstempel des Ereignisses (ISO 8601, Installations-Zeitzone 16.2) |
| `object_type` | Entitätstyp (`deal`, `contact`, `account`, `event`, `todo`) |
| `object_id` | ID des Objekts |
| `actor` | E-Mail des auslösenden Benutzers (bei System-Aktionen `system`) |
| `data` | Momentaufnahme des Objekts: Feldnamen/-werte gemäß logischem
  Datenmodell (12) |
| `changed_fields` | Nur bei `geändert`: Liste der geänderten Feldnamen —
  **Top-Level-Feld** im Umschlag (Geschwister von `data`, nicht darin) |

Der Umschlag ist über alle Events identisch; `data` folgt den Attribut-
Definitionen des jeweiligen Entitätstyps in §12. Schema-Änderungen sind nur
abwärtskompatibel erlaubt (neue Felder, keine Umbenennungen/Löschungen).

### 8.4 Audit-Log & KI-Audit

**Audit-Log:** Wer wann was (Aktion, Zielobjekt-Typ + ID) nach jedem erfolgreichen
Erstellen/Ändern/Löschen in: Deals, Kontakte, Firmen, Termine, Todos,
Workflow-Läufe, Dokumente (Upload/Löschung), Import-Läufe (inkl. Freigabe/
Rollback), Kalenderverbindungen, Auth (Login, 2FA-Setup), Benutzer-Verwaltung.
**Zusätzlich** werden alle Aktionen protokolliert, für die andere Abschnitte
dieser Spec eine Audit-Pflicht vorsehen — insbesondere: Rollenwechsel (2.3),
UWG-Warnung + Versand ohne Einwilligung (4.5), Geofencing-Override (7.5),
Guardrail-Policy-Überschreibung (8.5), KI-Konfigurations-Änderungen (8.9),
Löschbegehren-Abschluss (9.3), API-Token-/Webhook-Verwaltung (8.2/8.3).

**Unveränderbar** (nur Einfügen, nie Änderung/Löschung). **Log-Hygiene:** nur
IDs, Status, Feldnamen als Metadaten — keine Rohinhalte (Notizen, E-Mail-Texte,
Prompts), keine personenbezogenen Daten. Admin-Ansicht mit Filter (Aktion,
Zieltyp, Benutzer, Zeitraum).

**KI-Audit (getrennt):** Jede KI-Aktion protokolliert: Anweisungs-Version (4.3),
Modell, Token-Verbrauch, ausgelöste Guardrails, blockiert ja/nein, Latenz,
Status. Prompts werden **nur als Hash** (`prompt_hash`) geführt — keine
Klartext-Prompts, keine Roh-E-Mails.

### 8.5 Guardrails

Zentrale Prüfung **aller** KI-Aufrufe, konfigurierbar durch den Admin:

- **Inbound:** Toxizitätsfilter, PII-Maskierung (E-Mail/Telefon/Adresse),
  Jailbreak-/Prompt-Injection-Erkennung, Längenbegrenzung.
- **Outbound:** Content-Moderation (toxische/verbotene Inhalte), PII-Maskierung
  der Antwort, Längenbegrenzung.
- **Kostenkontrolle:** Anfragen pro Tag, Token-Budget pro Monat.
- **Themensteuerung:** Themen-Blacklist, Wettbewerber-Nennung.

Blockierte Aufrufe enden geordnet (kein Absturz) und werden im KI-Audit markiert.

**Guardrail-Policy je KI-Funktion (auflösender Zielkonflikt):** PII-Maskierung
und Extraktionszweck schließen sich aus — deshalb gilt jede Guardrail-Regel
**pro KI-Funktion** als konfigurierbare Policy. Die Maskierung von
E-Mail/Telefon/Adresse ist für Funktionen **deaktiviert, deren Zweck die
Erkennung genau dieser Daten ist**; für alle anderen Funktionen ist sie aktiv.

| KI-Funktion | PII-Maskierung inbound (Default) | Begründung |
|-------------|----------------------------------|------------|
| Triage & Kontakt-/Firmen-Extraktion (4.2) | **aus** | Zweck ist das Extrahieren von Kontaktdaten |
| Transkription & Annotation (5.6) | **aus** | Sprecher-/Kundenzuordnung nötig |
| Draft-Generierung (4.5) | **an** | Antworttext braucht keine Roh-PII Dritter |
| Agent-Chat (5.1) | **an** | Dialog ohne PII-Durchreiche |
| Research-Extraktion (5.4) | **an** (nur öffentliche Quellen) | Web-Daten, keine Kunden-PII |
| KB-Suche (5.5) | **aus** | Suchanfragen enthalten regelmäßig Kontaktnamen; Maskierung würde die Suche brechen. Restrisiko gedeckt durch Anbieter-Eignung (8.10) |

**Grundsatz zur Maskierung:** PII-Maskierung schützt **Inhalte, die einem Modell
als Verarbeitungsmaterial übergeben werden** (E-Mail-Bodies, Chat-Kontext).
Reine Such-Queries gegen die eigene Wissensbasis gelten nicht als
Verarbeitungsmaterial — sie werden durch die Anbieter-Eignung (8.10) gedeckt.

Der Admin kann jede Zeile überschreiben; jede Überschreibung erfordert eine
Bestätigung und wird im Audit-Log protokolliert. Weitere Guardrail-Regeln
(Toxizität, Injection, Kosten, Themen) gelten unverändert für alle Funktionen.

### 8.6 KI-Observability

Admin-Dashboard mit **ausschließlich Aggregaten und Systemmetadaten** — keine
Klartext-E-Mails, keine Nutzer-Prompts, keine Überwachung einzelner Benutzer.

Inhalte: LLM-Fehlerraten, Guardrail-Blockierungsspitzen, Token-Verbrauch,
Research-Pipeline (Jobs, Suchanfragen, extrahiertes Wissen, Dedup-Quote),
Nutzungsaktivität (E-Mail-Durchsatz, Entwürfe geprüft/freigegeben/verworfen,
transkribierte Audio-Minuten). Alerts: Fehlerrate > 10 %/24h, Guardrail-Spitzen,
ungewöhnliche Token-Spitzen.

**Alert-Zustellung:** Alerts erscheinen als In-App-Benachrichtigung (14). Für
kritische Alerts (LLM-Fehlerrate, Guardrail-Spitzen, Backup-Fehlschlag) ist
zusätzlich eine **E-Mail-Benachrichtigung an den Admin** konfigurierbar
(aktiv/inaktiv in den Einstellungen, Versand über das primäre E-Mail-Konto 4.1) —
damit Alarme auch ohne aktiven Login bemerkt werden. Existiert kein primäres
E-Mail-Konto, bleibt es bei der In-App-Zustellung (mit Hinweis in der
Konfigurationsansicht, kein stiller Verlust).

### 8.7 Backup & Restore

- **Gesichert:** Datenbank + Dateiablage (Dokumente) + verschlüsseltes
  Secrets-Archiv; Manifest mit Version, Zeitstempel, Prüfsummen;
  Vollständigkeits-Marker erst nach erfolgreichem Abschluss.
- **Rhythmus:** automatisch täglich (konfigurierbare Uhrzeit, Default 03:30) +
  jederzeit manuell auslösbar (Admin-UI oder CLI). Vor jeder Daten-Migration
  wird automatisch ein Pre-Migration-Backup erstellt (siehe 8.8).
- **Aufbewahrung:** rollierend gemäß LK-7 (Default 30 Tage, konfigurierbar).
- **Ablage:** Backup-Bereich des Objekt-/Dateispeichers. **Offsite-Ablage ist
  verpflichtend** (andernfalls ist RTO/RPO bei Totalausfall des VPS unerreichbar
  — ein Backup auf demselben Server ist bei dessen Verlust mit verloren);
  konfigurierbar ist lediglich das Ziel (z. B. externer Objektspeicher).
  **Schlüssel-Escrow:** Der Schlüssel für das verschlüsselte Secrets-Archiv
  wird zusätzlich **außerhalb des Servers** verwahrt (Betriebshandbuch, 21.7) —
  ohne Escrow wäre ein Restore auf neuem VPS unmöglich.
- **Restore:** Validierung (Marker, Manifest, Schlüssel), vorheriger
  Sicherheits-Snapshot, Bestätigungspflicht, automatische Schema-Anpassung,
  Health-Check. Unvollständige Backups werden abgelehnt.
- **Restore-Drill:** Automatisierter Nachweis — Daten anlegen, sichern, Umgebung
  wischen, wiederherstellen, Datenbestände und Dateien vergleichen. Regelmäßig
  ausführbar, Ergebnis protokolliert.
- **RTO/RPO-Ziele (verbindlich):** **RPO = 24 Stunden** (tägliches Default-
  Backup; manuelle Backups reduzieren den Verlust zusätzlich) — mehr als ein Tag
  Datenverlust ist fachlich nicht akzeptabel. **RTO = 4 Stunden** — innerhalb
  von 4 Stunden nach Feststellung eines Ausfalls ist das System per Restore
  (inkl. Health-Check) wieder betriebsbereit auf demselben oder einem neuen
  VPS. Der Restore-Drill misst die tatsächliche Restore-Dauer gegen die RTO.

### 8.8 Deployment & Betrieb (fachliche Anforderungen)

- Auslieferung als **Docker-Stack**: wenige Kern-Services, optionale Services
  als Profile (siehe 1.5).
- Kern-Stack muss auf einem VPS mit 4 GB RAM lauffähig sein.
- HTTPS, Health-Checks je Service, zentrale Status-Ansicht für den Admin.
- **Betreiber-Überwachung (außerhalb der App):** Die Health-Endpoints sind
  extern monitorbar; der Betreiber betreibt ein externes Uptime-Monitoring
  gegen den Health-Endpoint (Betriebshandbuch, 21.7) — die In-App-Alerts
  (8.6) funktionieren naturgemäß nicht, wenn der gesamte Server ausgefallen
  ist. Geplante Downtime bei Upgrades ist kurz und im Changelog angekündigt
  (21.4).
- Upgrade-fähig: Datenmigrationen mit Pre-Migration-Backup; Regressionssuite
  muss grün bleiben.

### 8.9 KI-Konfiguration (Admin)

- Der Admin wählt den **KI-Anbieter-Typ**: externe API (API-Schlüssel eines
  OpenAI-kompatiblen Anbieters) oder lokale Inferenz (optionales Profil);
  ohne Konfiguration sind alle KI-Fähigkeiten deaktiviert (Graceful Degradation,
  siehe 1.5).
- **Modell je KI-Funktion** getrennt wählbar: Triage, Draft, Agent,
  Research-Extraktion, Transkription, KB-Suche — so können günstige Modelle für
  Routineaufgaben und stärkere für den Agent genutzt werden.
- **Verbindungstest** bei der Konfiguration; Änderungen werden im Audit-Log
  protokolliert.
- Kostenlimits (Anfragen/Tag, Token-Budget/Monat) siehe Guardrails (8.5);
  Überschreitung deaktiviert KI-Aufrufe geordnet mit Hinweis (kein Absturz).

### 8.10 Datenschutz bei KI-Verarbeitung

KI-Funktionen verarbeiten zwangsläufig personenbezogene Daten (E-Mail-Inhalte,
Audiodateien, Kontaktdaten). Fachliche Anforderungen:

- **Datenkategorien-Deklaration:** Jede KI-Funktion deklariert, welche
  Datenkategorien sie an ein Modell übermittelt: Triage/Extraktion =
  E-Mail-Inhalt + Absenderdaten; Draft = E-Mail-Inhalt + Tonalitätshistorie;
  Agent = CRM-Daten der abgefragten Entitäten; Research = nur öffentliche
  Web-Daten; Transkription = Audiodatei + Transkript. Die Deklaration ist Teil
  der KI-Einstellungen (12.5) und in der Admin-Ansicht sichtbar.
- **Anbieter-Eignung als Aktivierungsvoraussetzung:** Eine KI-Funktion mit
  personenbezogenen Daten darf nur aktiviert werden, wenn der konfigurierte
  Anbieter Auftragsverarbeitung (Art. 28 DSGVO) vertraglich unterstützt und
  Verarbeitung in der EU bzw. mit angemessenem Datenschutzniveau zusichert —
  oder wenn lokale Inferenz (ohne Datenabfluss) genutzt wird. Ohne geeigneten
  Anbieter bleibt die Funktion deaktiviert (Graceful Degradation, 1.5); die
  Konfigurationsansicht zeigt den Grund an.
- **Eignung ist eine dokumentierte Admin-Erklärung (kein automatisierter
  Check):** Der Admin bestätigt die Eignung in einem Bestätigungs-Dialog mit
  Rechtsfolgen-Hinweis und hinterlegt dabei zwingend eine **Eignungs-Referenz**
  (Freitext: AVV-Dokument/Link/Vertragsbezeichnung). Die Referenz wird in den
  KI-Einstellungen gespeichert (12.5) und ist im Admin-Bereich einsehbar.
  Lokal: Bei lokaler Inferenz entfällt die Eignungs-Erklärung (kein Datenabfluss).
- **PII-Redaktion als Schutzschicht:** Die Guardrail-Policy (8.5) maskiert PII
  für alle Funktionen ohne Extraktionszweck; dies reduziert die an externe
  Anbieter übermittelten personenbezogenen Daten auf das fachlich notwendige
  Minimum.
- **Lokale Inferenz als Alternative:** Für Installationen mit erhöhten
  Datenschutz-Anforderungen ist der vollständige KI-Betrieb über lokale
  Inferenz (optionales Profil) möglich — ohne jeglichen Datenabfluss.

---

## 9. Nicht-funktionale Anforderungen

### 9.1 Ressourcen & Betrieb

| Anforderung | Ziel |
|-------------|------|
| Gesamt-RAM Kern-Stack | < 4 GB (inkl. Datenbank, App, Worker) |
| Container-Anzahl Kern-Stack | ≤ 5 |
| Optionale Services | Als Profile zuschaltbar (KI-Inferenz lokal, Meta-Suche, Monitoring) |
| Startzeit | < 3 Minuten bis einsatzbereit |
| Ziel-Hardware | 1 VPS, 4 GB RAM, 2 vCPU |
| Globale Suche | Antwort < 1 s bei bis zu 50.000 CRM-Datensätzen und 200.000 E-Mails (E-Mail-Volltextsuche eingeschlossen, innerhalb der Kapazitätsannahmen unten) |
| Kartenansicht | 1.000+ Pins ohne Performance-Einbruch (Cluster aktiv) |
| Listenansichten | 100 Zeilen ohne spürbare Ladezeit; darüber Seitenweise (Paginierung) |
| E-Mail-Verarbeitung | Eingang → Zuordnung + Triage < 60 s nach Abholung |

**Kapazitätsannahmen (Auslegungsbasis):** 10 Benutzer; 50.000 Kontakte/Firmen;
200.000 E-Mails pro Jahr; 10.000 Dokumente (~50 GB Dateiablage); 1.000
Tracking-Events pro Tag; 100 Transkripte pro Monat. Innerhalb dieser Dimensionen
müssen die Performance-Ziele oben gelten; darüber ist das Verhalten nicht
spezifiziert (Single-Tenant-Zielgruppe bleibt deutlich darunter).

### 9.2 Benutzerführung

- Zweisprachigkeit: Deutsch + Englisch (vollständige UI-Übersetzung).
- Dark Mode (system-synchronisiert).
- Responsive: Desktop primär, mobile Nutzung für Setter-View/Karte/Check-In
  zwingend.
- **Barrierefreiheit (verbindlich, BFSG):** Die UI erfüllt die Anforderungen des
  Barrierefreiheitsstärkungsgesetzes (BFSG, seit 28.06.2025 auch für die
  Privatwirtschaft) in der technischen Ausprägung **EN 301 549 / WCAG 2.1 Level
  AA**: vollständige Tastatur-Bedienbarkeit, ausreichende Kontraste, semantische
  Beschriftungen für Formulare, Screen-Reader-Tauglichkeit, Fokus-Indikatoren.
  Die Einhaltung wird je Phase per Accessibility-Prüfung nachgewiesen (siehe
  21.3); Ausnahmen nur für intern begründete Einzelfälle, dokumentiert.

### 9.3 Datenschutz (DSGVO & DIN EN ISO/IEC 27555)

- **Datenportabilität:** Export jederzeit möglich (siehe 6.3).
- **Log-Hygiene:** keine PII, keine Credentials, keine Roh-Prompts in Logs.
- **Import:** Rechtsgrundlage und Zweck werden dokumentiert (siehe 6.2);
  Staging-Daten nach 30 Tagen minimiert.
- **Tracking:** Consent-Gate, anonyme IDs, Bot-Filter (siehe 6.5).
- **KI-Verarbeitung:** Datenkategorien-Deklaration je KI-Funktion, Anbieter-
  Eignung (AVV/EU-Verarbeitung) als Aktivierungsvoraussetzung, lokale Inferenz
  als Datenabfluss-freie Alternative (siehe 8.10).

**Löschkonzept (konkrete Löschklassen):** Jede personenbezogene Entität ist einer
Löschklasse mit fester Frist zugeordnet; Löschung erfolgt fristgerecht automatisch
und nachweisbar (Löschzeitpunkt protokolliert im Audit-Log).

| Löschklasse | Frist | Zugeordnete Daten |
|-------------|-------|-------------------|
| LK-1 Flüchtige Verarbeitungsdaten | 30 Tage | Import-Staging/Rohdaten (6.2), abgelaufene/verworfene Proposals, Fehlerlisten-Einträge nach Behebung |
| LK-2 Rohmaterial KI/Research | 90 Tage | Roh-HTML aus Research (5.5), Audio-Rohdateien nach erfolgreicher Transkription (Transkript selbst bleibt) |
| LK-3 Nutzungs-/Trackingdaten | 12 Monate | Tracking-Besucher & -Events (6.5); danach Löschung, Aggregat-Statistiken bleiben |
| LK-4 Protokolle & Nachweise | 6 Jahre | Audit-Log, KI-Audit (8.4), Fraud-Log (7.5), Geo-Check-Ins — unveränderbar, dienen dem Nachweis |
| LK-5 Geschäftsdaten | 6 Jahre | E-Mail-Korrespondenz, Dokumente der Kategorien Vertrag/Vollmacht/Angebot/Rechnung (6.1), Deals inkl. Abschlussdaten |
| LK-6 Stammdaten | bis Widerruf/Löschbegehren | Kontakte, Firmen, Benutzerkonten; nach Löschbegehren Löschung binnen 30 Tagen (DSGVO Art. 17), Referenzen in LK-4/LK-5 werden pseudonymisiert |
| LK-7 Sicherungen | rollierend, Default 30 Tage | Backups (8.7); ältere Sicherungen werden überschrieben/gelöscht |

**Regeln:** Fristbeginn ist je Klasse definiert (Erzeugung, letzte Verarbeitung
bzw. Vertragsende). Löschbegehren werden im Admin-Bereich ausgelöst und
protokolliert. Gesetzliche Aufbewahrungspflichten (LK-4/LK-5) gehen einem
Löschbegehren vor — dann Pseudonymisierung statt Löschung.

**Löschbegehren einer Person (Art. 17) — Mechanik:** Ein Löschbegehren wird im
Admin-Bereich für einen Kontakt ausgelöst und läuft binnen 30 Tagen:

1. **Löschen (LK-6):** Der Kontakt und alle direkt personenbezogenen Daten
   werden gelöscht — zugehörige Todos, Geo-Check-Ins, Transkripte, Fakten,
   Proposals, Agent-Events, Tracking-Events, Form-Submissions (Kaskade gemäß
   16.1).
2. **Pseudonymisieren (LK-5, unveränderliche E-Mails):** In Betreff und Body
   betroffener E-Mails werden Name, Adresse und E-Mail-Adresse der Person durch
   den generischen Platzhalter `[PERSONENDATEN-ENTFERNT]` ersetzt; der übrige
   Inhalt bleibt unverändert. Es wird bewusst **kein** Mapping (Pseudonym →
   Person) gespeichert, damit keine personenbezogenen Restdaten verbleiben.
   Die Pseudonymisierung selbst wird im Audit-Log protokolliert (Anzahl
   betroffener E-Mails, Zeitstempel — ohne Inhaltsbezug).
3. **Unberührt:** Audit-Log und KI-Audit (LK-4) bleiben unverändert — sie
   enthalten log-hygienisch nur IDs/Metadaten (8.4); Deals der Person bleiben
   als kaufmännische Daten bestehen (Kontakt-Verweis wird leer, siehe 16.1).
4. **Nachweis:** Der Abschluss des Löschbegehrens (gelöschte Objekttypen,
   Anzahl pseudonymisierter E-Mails) wird als Audit-Eintrag dokumentiert.

**Auskunft (Korrespondierender Umfang, siehe 6.3):** Der Export „Datenexport
für Person X" umfasst alle Daten mit Personenbezug vor einer Löschung: Kontakt,
E-Mails mit der Person, Todos, Fakten, Transkripte, Tracking-Events,
Form-Submissions, Dokumente-Metadaten — als JSON; E-Mail-Bodies mit
Drittadressen werden nach derselben Platzhalter-Regel geschwärzt.

### 9.4 EU AI Act (Verordnung (EU) 2024/1689)

**Anwendungsstand:** Der AI Act gilt seit 01.08.2024; die Verbote und die
KI-Kompetenz-Pflicht (Art. 4 „AI Literacy") gelten seit 02.02.2025; die
übrigen Pflichten — einschließlich Art. 50 Transparenz — gelten seit
**02.08.2026** und sind damit zum Zeitpunkt dieser Spec vollständig anwendbar
(siehe 19.1).

- **Risikoklassifizierung:** Das System ist ein KI-System mit begrenztem
  Risiko („limited risk"); es fällt nicht unter die Verbote (Art. 5) und ist
  kein Hochrisiko-System (Anhang III) — die Pflichten beschränken sich auf
  Transparenz und Aufsicht.
- **Transparenz (Art. 50):** UI informiert über KI-Interaktion; generierte
  Inhalte sichtbar und maschinenlesbar gekennzeichnet.
- **Menschliche Aufsicht (Art. 14):** KI-Aktionen mit kaufmännischer/rechtlicher
  Folgewirkung nur mit expliziter Bestätigung (HITL durchgängig: Drafts,
  Proposals, Workflow-Schritte, Import-Freigabe, KI-Extraktions-Chips).
  Dokumentierte Ausnahmen: Research, Todo-Extraktion, Recheck-Verarbeitung
  (rein intern, ohne Außenwirkung — siehe 5.7).
- **KI-Kompetenz (Art. 4):** Benutzer, die KI-Funktionen nutzen, erhalten eine
  kontextuelle Einweisung (Kurzhinweise in der UI: was die KI tut, was der
  Mensch prüfen muss); Admins erhalten eine erweiterte Einweisung
  (Guardrails, Kosten, Datenschutz-Rahmen).
- **Guardrails:** Alle LLM-Aufrufe über zentrale Inbound-/Outbound-Guardrails
  (siehe 8.5).

### 9.5 Testbarkeit (fachlich)

- Jedes Modul definiert konkrete Akzeptanzkriterien auf Fach-Ebene (Zustände,
  Regeln, Abläufe) — ausformuliert in §15, prüfbar als E2E-Szenarien je Rolle
  (Admin, Benutzer).
- Tests prüfen echte Funktionalität (Daten anlegen, bearbeiten, löschen,
  verifizieren) — nicht nur Statuscodes.

---

## 10. Traceability: MVP → v3

### 10.1 Übernommene Specs (fachlich aggregiert)

| MVP-Spec-Cluster | Ziel in v3 |
|------------------|------------|
| Plattform-Foundation (Email-Triage, Research/KB, Drafts, Todos, Transkription) | Module 4, 5 |
| CRM-Core-Detail-CRUD, Account-Contact-UX, Shell/Admin-CRM-Views | Modul 3 |
| Todos/Calendar, Calendar-Integration | 3.5, 3.6 |
| AP-7 CRM-Produktivität (Suche, Timeline, Bulk, Saved Views) | 3.4, 3.7, 3.8 |
| AP-5 Custom Fields | 3.9 |
| AP-1 Evidence-Ledger, AP-2 Agent-Tools, AP-3 Agent-Transparenz, AP-6 Self-Scheduling | Modul 5 |
| AP-4 Website-Tracking + Form-Intake | 6.5 |
| AP-8 Integrations v1 (LinkedIn-Fakt, Konnektor-Vorarbeit) | 5.4 (LinkedIn experimentell), 7.1 (Konnektor-Schiene) |
| AP-12 Quick Wins (Anreicherungs-Status) | 5.4 |
| Workflows/Templates | 6.4 |
| D2D-/Energievertriebs-Module (Energiedaten, Tarifrechner, Map, Geofencing, Setter/Closer) | Modul 7 (ohne EnWG-Unbundling) |
| 2FA-Security, Auth-Gate/OTP-Grace | 8.1 (2FA-Pflicht und Grace-Period abgelöst — 2FA jetzt optional, siehe 10.3) |
| Enterprise-Gaps (API-Tokens, Webhooks, Audit-Writes) | 8.2–8.4 |
| Backup/Restore/Import, Tenant-Data-Import, Dedup-Merger | 6.2, 8.7 |
| CSV-Export (v2.0) | 6.3 |
| Guardrails (Product-Keys-Guardrails), Prompt-Engineering, KI-Observability | 8.5, 8.6, 4.3 |
| KI-Label-System | 4.3 |
| Dokumente | 6.1 |
| Demo-User-Data | 2.3 (Demo-Daten im Setup) |
| v1-Production-Readiness (fachliche Anteile: HITL, Observability) | 1.3, 8.6 |

### 10.2 Bewusst nicht übernommen

| MVP-Bestandteil | Begründung |
|-----------------|------------|
| Multi-Tenancy/RLS, Global-Admin, Tenant-Verwaltung, Invitations-Tenant-Logik | Single-Tenant |
| RBAC-Matrix (roles/permissions/user_features) | 2-Rollen-Modell |
| Invoicing, Mahnwesen, Produkte/Tarife, Multi-Währung, Rechnungs-PDF | Produktentscheidung |
| DATEV/GoBD, Provisions-Engine, Legal-Chat | Produktentscheidung/Altlast |
| Model-Training-Pipeline, Model-Registry, Datasets | Unverhältnismäßig (siehe 1.4) |
| EnWG-Unbundling | Regulatorische Spezialanforderung |
| SSO/OIDC | Nicht Zielgruppen-relevant |
| Styleguide, Guide-Seiten, MD3-Experimente | Interne Artefakte |
| Health-Dashboard als eigener Service | Geht in 8.8 (zentrale Status-Ansicht) auf |
| E-Signature-Platzhalter (DocuSign) | YAGNI — Deal-Erfassung ohne E-Signature |

### 10.3 Fachliche Neuentscheidungen gegenüber dem MVP

| Thema | MVP | v3 | Begründung |
|-------|-----|-----|------------|
| Deal-Stages | Keine Stages, Pipeline auf Accounts/Kontakten | Beibehalten: Deals = Abschlüsse, Pipeline = Lifecycle-Status + Setter/Closer-Pipeline | Bewusstes MVP-Design, bestätigt durch Energie-Vertriebsfluss |
| Produkt-Referenzen an Deals | Deals verweisen auf Produkte | Entfernt (Produkte ausgeschlossen); Anbieter/Partner bleibt Freitext | Konsistenz mit Invoicing-/Produkte-Ausschluss |
| Externe Kalender | Admin verbindet für den Tenant | Jeder Benutzer verbindet seinen eigenen Kalender | Einfacher, passt zu Single-Tenant |
| Setter/Closer | Als Rollen-Views angelegt | Als Views für alle Benutzer | 2-Rollen-Modell |
| Energiedaten-Verortung | JSONB am Kontakt | Beibehalten: strukturierte Maske am Kontakt | Bewährt |
| Webhook-Events | Inkl. Rechnungen | Ohne Rechnungen, dafür Firmen-Events | Konsistenz mit Feature-Umfang |
| Triage-Kategorien | Fest + KI-Labels | Fest (Basis) + eigene Labels pro Postfach | Bewährtes MVP-Konzept übernommen |
| Suchumfang | Kontakte, Firmen, Deals, Todos, Rechnungen | Kontakte, Firmen, Deals, Todos, Dokumente, E-Mails (Volltext) | Rechnungen entfallen; Dokumente und E-Mails neu durchsuchbar |
| Agent-Tools | 11 Tools (MVP-Stand) | Dieselben 11 Tools, unverändert | Bewährt; keine Spekulation über Zukunfts-Tools |
| Form-Intake-Fakt | Geplant (Spez), nicht implementiert | Nicht übernommen (YAGNI) | Kontakt mit Quelle „Web-Intake" reicht fachlich |
| E-Mail-Empfänger | Nur Einzel-Empfänger (kein CC/BCC) | Beibehalten: ein Empfänger, kein CC/BCC | YAGNI; Zielgruppe schreibt 1:1 |
| Währung | Währungsfeld an Deals (AP-9 Multi-Währung) | EUR fix, kein Währungsfeld | Multi-Währung ausgeschlossen (1.4) |
| Passwort vergessen | Nicht vorhanden | Admin-Reset + Notfall-Wiederherstellung (8.1) | Neu in v3; kein E-Mail-Selbst-Reset (Einfachheit/Sicherheit) |
| E-Mail-Löschung/Archiv | Nicht vorhanden | Bewusst nicht übernommen (Aufbewahrung nach LK-5) | Datenhoheit + Einfachheit |
| Löschkaskaden | Nicht geregelt (Hard-Deletes) | Explizite Kaskaden-Regeln (16.1) | Sauber durchdacht statt implizit |
| Workflow-Start | Teil-automatisch | Nur manuell/Vorschlag, erster Schritt mit Bestätigung | HITL-Konsistenz (9.4) |
| Tonalitäts-Scopes | global/Gruppe/Kunde (Gruppe nie definiert) | Nur global + pro Kunde, mit Historie-Fallback | „Gruppe" war im MVP ein undefiniertes Konzept (YAGNI) |
| 2FA | Pflicht für alle Benutzer (mit Grace-Period für Global-Admin) | Optional: frei aktivier-/deaktivierbar je Benutzer, kein Setup-Zwang | Produktentscheidung: weniger Onboarding-Reibung für Freelancer-Zielgruppe; Deaktivierung nur mit gültigem TOTP bleibt als Schutz |
| Deal-Status `partial` | Vorhanden, ohne Semantik | Gestrichen: nur `open`/`won`/`lost` | Im MVP praktisch ungenutzt (nur won/lost-Transitionen); Zustand ohne Definition bricht UI/Reporting |
| Geofencing | Für jede Deal-Anlage (Büro-Closer ausgesperrt) | Nur für Abschlussart `d2d`; `buero` immer möglich; 2h-Gültigkeitsfenster; Admin-Override mit Audit | Praxis-Tauglichkeit im Außendienst + Büroalltag |
| Löschbegehren-Mechanik | Nicht geregelt | Explizite 4-Schritt-Mechanik (Löschen/Pseudonymisieren/Unberührt/Nachweis, 9.3) | DSGVO Art. 17 vs. unveränderliche LK-5-E-Mails aufgelöst |
| Lead-Score | Vorhanden, ohne Definition | Definiert: KI-Priorisierungshilfe 0–100, nur Anzeige/Filter/Sortierung, keine Folgeautomatik (4.3) | Zahl ohne Bedeutung wäre totes Feature |
| Themenkategorie | „Produktkategorie", freie Klassifikation | Admin-verwaltete Kategorien-Liste mit „Sonstiges"-Fallback (4.3) | Kontrolliertes Vokabular für Filter/Auswertungen |
| Timeline | Hart bei 50 Einträgen zu Ende | Paginierung in 50er-Schritten (3.4) | Historie langjähriger Kunden |
| Workflow-/Template-Änderungen | Nicht geregelt | Snapshot-Semantik: aktive Läufe arbeiten mit Start-/Vorbereitungs-Zustand (6.4) | Laufende Automationen brechen nie |
| Arbeits-Zuweisung | Nicht vorhanden (Shared Workspace ohne Zuständigkeit) | Zugewiesen-an auf Todo/Termin/E-Mail + „Meine"-Filter (2.1) | Team-Nutzung (Zielgruppe 1–10 Nutzer) ohne Zuständigkeit nicht bedienbar |
| Abarbeitungs-Status E-Mail | Nicht vorhanden | offen/in_arbeit/erledigt + „beantwortet" automatisch aus gesendetem Draft (4.2) | Inbox ohne Abarbeitungszustand nach Wochen unbenutzbar |
| Proposal-TTL | 15 Minuten (MVP) | 48 Stunden + Ablauf-Erinnerung + Reaktivierung (5.3) | Nutzer in Kundengesprächen verlieren Vorschläge sonst ungelesen |
| Triage-Korrektur | Nicht möglich | Alle Triage-Labels manuell überschreibbar, Korrektur gewinnt dauerhaft (4.3) | KI-Fehler dürften nie zu verlorenen Kunden führen |
| Deal-Bezug | Firma Pflicht | Firma ODER Kontakt (Privatkunde ohne Firma) (3.3) | Energie-D2D ist überwiegend B2C — Firma-Pflicht erzwingt Dummy-Daten |
| E-Mail-Historie am Tag 1 | Nur zukünftige Abrufe | 90-Tage-Backfill beim Konto-Setup (4.1) | E-Mail-CRM ohne Historie startet wertlos |
| Deal-Status `widerrufen` | Nicht vorhanden | `widerrufen` nur aus `won` (§ 355 BGB), gesonderte Auswertung, keine Anschluss-Wiedervorlage (3.3) | B2C-Widerrufsrecht ist Vertriebsalltag; `lost`-Missbrauch verfälscht Conversion |
| Kontakt-E-Mail optional | E-Mail Pflichtfeld | Identifikation über E-Mail ODER Telefon (3.1) | D2D-Leads starten oft nur mit Telefonnummer |
| Kundentyp am Kontakt | Nur an der Firma | Pflichtfeld am Kontakt (UWG-Stufungs-Fundament, 3.1/4.5) | B2C-Kunden haben keine Firma |
| Besuchsstatus | Eigenes Feld am Kontakt | In Vertriebsstufe aufgegangen (3.1, 7.4) | Zwei Statussysteme für denselben D2D-Prozess sind widerspruchsanfällig |
| Backfill-Nebenwirkung | — (Backfill neu in v3) | `erledigt` + keine Todo-Extraktion für Historie (4.1) | Volle Verarbeitung erzeugt Todo-Flut am Tag 1 |
| Dubletten-Merge | Nur im Import-Wizard | Merge auch im Tagesgeschäft (3.10) | Matching/Setter erzeugen Dubletten laufend |
| Telefonie | Nicht vorhanden | Telefonie-Konnektor: Click-to-Call, Anruf-Aktivität, eingehende Anrufe (7.1a) | Telefon ist Standardkanal des Vertriebs; ohne Integration bleiben Aktivitäten undokumentiert |
| Termin-Einladung | Nicht vorhanden | ICS-Einladungsmail vom Termin (3.6) | Bestätigungsmail senkt No-show-Rate — Standard-Erwartung |
| Forecast & Zeitreihe | Nicht vorhanden | Vertriebs-Report: Forecast + Abschluss-Zeitreihe als Dashboard-Widgets + Export (6.3) | „Was kommt / wie lief es?" sind Kernfragen jedes Vertriebs |
| Intake-Formular | Ein hart kodiertes Form | Beliebig viele konfigurierbare Formulare je Domain (6.5, 12.4) | Verschiedene Kanäle brauchen verschiedene Feldsätze |
| Positionierung | Schwerpunkt Energievertrieb | Generalistischer Kern + Energie als erste Vertikale/Implementierungs-Leitbild (1.1, 17) | Kern branchenneutral, Schärfe über Module — andere Vertikalen ohne Kernänderung |
| Soft-Deletes/Papierkorb | Nicht vorhanden (Hard-Deletes) | Bewusst beibehalten: Hart-Löschung mit expliziten Kaskaden (16.1) | Einfachheit; Audit-Log erhält die Nachweisbarkeit |

### 10.4 Daten-Migration vom MVP (Entscheidung)

**Entscheidung: kein automatisches Migrations-Werkzeug MVP → v3.** Begründung:
Das MVP-Schema ist Multi-Tenant-geprägt (RLS, tenant_id an allen Entitäten,
RBAC-Tabellen, ausgeschlossene Module wie Invoicing/Provisions) — eine
1:1-Übernahme würde die Altlasten konservieren, die v3 bewusst abstellt.

**Offizieller Übernahmeweg:** CSV/XLSX-Export der MVP-Kernbestände (Kontakte,
Firmen, Deals, Todos, Termine, Dokumente-Metadaten) → Import-Wizard (6.2) in v3
mit Mapping-Vorlagen, Dedup-Kaskade und Rollback. Damit gilt:

- Übernahme ist eine **bewusste, prüfbare Aktion** (Wizard mit Vorschau und
  Dubletten-Entscheidung), kein unsichtbarer Lift-and-Shift.
- Nicht übernommene MVP-Daten (E-Mail-Historie, Agent-/Evidence-Daten,
  Tracking) verbleiben im MVP-Archiv; für E-Mail-Bestände gilt die fachliche
  Empfehlung, nur aktive Vorgänge zu übernehmen.
- Dokumente (Dateien) werden außerhalb des Wizards als Datei-Upload übernommen
  (Kategorie + Firma), da der Import-Wizard zeilenbasiert ist.

---

## 11. Offene Punkte & vertagte Entscheidungen

Diese Punkte sind **bewusst nicht** Teil dieser fachlichen Spec und werden im
technischen Brainstorming entschieden:

1. Tech-Stack (Sprache(n), Framework, ORM, Job-Queue).
2. Container-Zuschnitt und konkrete optionale Profile (welche Images).
3. Datenbank-Produkt und Schema-Design.
4. LLM-Anbieter-Anbindung konkret (API-kompatibler Standard, lokale Inferenz).
5. Dateispeicher-Strategie (Objektspeicher vs. Dateisystem-Volume).
6. Konkrete Tarif-API-Anbieter für den Energie-Konnektor.
7. CI/CD-Pipeline und Deployment-Automatisierung.

---

## 12. Logisches Datenmodell

Vollständiges logisches Modell: alle Entitäten mit Attributen, fachlichen Typen,
Pflichtfeldern (*) und Kardinalitäten. Konventionen: Typen sind fachlich (Text,
Zahl, Datum, Zeitstempel, Ja/Nein, Aufzählung, Liste, Struktur, Verweis, Secret);
`→ X` = Verweis auf Entität X; `1—n` / `n—1` / `1—1` = Kardinalitäten. Die
physische Abbildung (Schema, Normalisierung, Speicherung) ist bewusst vertagt
(§11). Verhaltensregeln stehen in den Modul-Abschnitten (§3–§8).

### 12.1 Personen & Organisation

**Benutzer**
- Attribute: Vollname* (Text), E-Mail* (Text, eindeutig), Passwort (Secret),
  Rolle* (Aufzählung: `admin`/`user`), Status* (Aufzählung: `aktiv`/
  `deaktiviert`), 2FA aktiv (Ja/Nein, Default nein — optional, siehe 8.1),
  2FA-Geheimnis (Secret), Recovery-Codes (Liste, Secret), letzter Login
  (Zeitstempel), angelegt am (Zeitstempel)
- Beziehungen: 1—n API-Tokens, Kalenderverbindungen, ausgestellte Einladungen;
  ist Autor von Aktivitäten, Geo-Check-Ins, Dokument-Uploads

**Einladung**
- Attribute: E-Mail*, Rolle* (Aufzählung), Einladungs-Token (Secret), gültig bis
  (Zeitstempel, Default +7 Tage, siehe 2.3), Status* (Aufzählung: `offen`/
  `angenommen`/`verfallen`), ausgestellt von (→ Benutzer), angelegt am
  (Zeitstempel)
- Beziehungen: n—1 Benutzer (Aussteller); erzeugt bei Annahme 1 Benutzer

**Firma (Account)**
- Attribute: Firmenname*, Ansprechpartner*, E-Mail* (Text), Telefon, Website
  (Text), Adresse, PLZ, Ort (Text), Geokoordinaten (Zahl, automatisch per
  Geocoding), Branche (Aufzählung, frei erweiterbar), Kundentyp (Aufzählung:
  `Geschäftskunde`/`Privatkunde`/`Partner`), Priorität (Aufzählung: `Hoch`/
  `Mittel`/`Niedrig`), Notizen (Text), Lifecycle-Status* (Aufzählung: `neu`/
  `kontaktiert`/`angebot`/`verhandlung`/`abgeschlossen`/`verloren`), letzte
  Aktivität (Zeitstempel, automatisch), Anreicherungs-Status (Aufzählung:
  `nicht angestoßen`/`läuft`/`abgeschlossen`/`fehlgeschlagen`) + Zeitstempel
  des letzten Laufs, Research-täglich-Flag (Ja/Nein), Custom-Field-Werte
- Beziehungen: 1—n Kontakte, Deals, Aktivitäten, Todos, Termine, Dokumente,
  Fakten, Proposals, Agent-Events, KB-Einträge, Workflow-Läufe,
  Konnektor-Ergebnisse, Tracking-Besucher (per Domain-Zuordnung)

**Kontakt**
- Attribute: Name*, E-Mail (Text, optional — Identifikation über E-Mail oder
  Telefon, siehe 3.1), Telefon (Text), Rolle im Unternehmen (Text),
  Firma (→ Firma, optional), Quelle* (Aufzählung: `manuell`/`email`/
  `web_intake`/`import`), Kundentyp* (Aufzählung: `Privatkunde`/
  `Geschäftskunde`/`Partner`, Default abhängig von Firma-Zuordnung, siehe 3.1),
  Lifecycle-Status (Aufzählung wie Firma), Signatur
  (Text), Out-of-Office (Struktur: aktiv Ja/Nein, von/bis Datum, Nachricht
  Text), zugewiesen an (→ Benutzer, optional — „meine Kunden", siehe 2.1),
  Geokoordinaten (Zahl), Energiedaten (Struktur: Zählernummer Text, MSB Text,
  Verbrauch Strom kWh Zahl, Verbrauch Gas kWh Zahl, aktueller Anbieter Text,
  Vertragslaufzeit bis Datum, Kündigungsfrist Monate Zahl, Zählerstand-Ablesung
  Datum), letzte Aktivität (Zeitstempel, automatisch), Vertriebsstufe
  (Aufzählung: `lead`/`setter_gespraech`/`closer_termin`/`abgeschlossen`/
  `verloren`, Default `lead`) + Zeitstempel des letzten Stufenwechsels,
  E-Mail-Werbe-Einwilligung (Aufzählung: `erteilt`/`nicht erteilt`/`widerrufen`
  + Datum + Quelle + Nachweis-Referenz, siehe 3.1/19.4), Telefon-Werbe-
  Einwilligung (dieselbe Struktur), Custom-Field-Werte
- Beziehungen: n—1 Firma; 1—n Deals, Todos, Aktivitäten, Transkripte,
  Geo-Check-Ins, Form-Submissions, Fakten, Proposals, Agent-Events,
  Tracking-Events

### 12.2 Vertrieb & Arbeit

**Deal (Abschluss)**
- Attribute: Titel*, Bezug* (→ Firma **oder** → Kontakt — genau einer von
  beiden Pflicht, siehe 3.3), Anbieter/Partner (Text mit Vorschlägen),
  Vertragsbeginn (Datum), Vertragsende (Datum), monatlicher Wert (Zahl),
  Einmalwert (Zahl), Tarif/Leistung (Text, optional), Konnektor-Ergebnis
  (→ Konnektor-Ergebnis, optional), Notizen (Text), Status* (Aufzählung:
  `open`/`won`/`lost`/`widerrufen` — `widerrufen` nur aus `won`, mit
  Widerrufsdatum + Begründung, siehe 3.3), Abschlussart* (Aufzählung:
  `buero`/`d2d`, Default `buero`), Abschlusszeitpunkt (Zeitstempel),
  zugewiesen an (→ Benutzer, optional, siehe 2.1), angelegt am (Zeitstempel)
- Beziehungen: n—1 Bezug (Firma oder Kontakt); 1—n Aktivitäten, Fakten,
  Proposals, Agent-Events; bewusst keine Produkt-Referenzen (siehe 10.3)

**Aktivität (Logbuch)**
- Attribute: Typ* (Aufzählung: `note`/`call`/`email`/`meeting`/`status_change`/
  `system`), Titel (Text), Text (Text), Priorität (Aufzählung: `dringend`/
  `normal`), Zeitstempel*, Autor (→ Benutzer, bei `system` leer)
- Beziehungen: n—1 Zielentität (Firma | Kontakt | Deal)

**Todo**
- Attribute: Titel*, Notiz (Text), Firma (→ Firma, optional), Kontakt (→
  Kontakt, optional), Fälligkeit (Datum, optional), Priorität (Aufzählung:
  `Niedrig`/`Mittel`/`Hoch`/`Dringend`), Kategorie (Aufzählung: `follow_up`/
  `call`/`email`/`contract`/`other`), Status* (Aufzählung: `offen`/`erledigt`),
  zugewiesen an (→ Benutzer, optional — Team-Pool, wenn leer, siehe 2.1),
  Ursprungs-E-Mail (→ E-Mail, optional), angelegt am (Zeitstempel)
- Beziehungen: n—1 Firma/Kontakt (optional); Wiedervorlage = Todo mit Firma +
  Fälligkeit (kein eigenes Objekt)

**Termin**
- Attribute: Typ* (Aufzählung: `meeting`/`call`/`email_reminder`/`deadline`),
  Titel*, Firma (→ Firma, optional), Kontakt (→ Kontakt, optional), Start*
  (Zeitstempel), Ende (Zeitstempel, leer = ganztags), Ort/Link (Text),
  Erinnerung (Aufzählung: `15 Min`/`30 Min`/`1 Std`/`1 Tag`/keine), Status*
  (Aufzählung: `planned`/`done`/`cancelled`), Draft (→ Draft, nur bei
  `email_reminder`), zugewiesen an (→ Benutzer, optional, siehe 2.1),
  extern (Ja/Nein), externer Provider + Original-Link +
  Teilnehmer (Liste: Name, E-Mail, Antwortstatus) bei Kalender-Import
- Beziehungen: n—1 Firma/Kontakt (optional); 1—1 Draft (optional)

**Geo-Check-In**
- Attribute: Kontakt* (→ Kontakt), Benutzer* (→ Benutzer), Breitengrad*,
  Längengrad* (Zahl), Genauigkeit (Zahl), innerhalb Radius (Ja/Nein),
  Zeitstempel*
- Beziehungen: n—1 Kontakt, n—1 Benutzer; Abweichung → 1 Fraud-Log-Eintrag

**Fraud-Log-Eintrag**
- Attribute: Auslöser (Aufzählung: `außerhalb Radius`/`keine GPS-Daten`/
  `Muster-Auffälligkeit`), Referenz (→ Geo-Check-In | Deal), Detail (Text),
  Zeitstempel*
- Beziehungen: n—1 Geo-Check-In/Deal; sichtbar im Admin-Dashboard (7.5)

### 12.3 E-Mail & KI

**E-Mail-Konto**
- Attribute: Bezeichnung*, Typ* (Aufzählung: `imap`/`microsoft365`),
  Abruf-Konfiguration (Struktur: Server/Port/Benutzer/App-Passwort bzw.
  OAuth-Verweis, Secret), Versand-Konfiguration (Struktur: SMTP Server/Port/
  Zugangsdaten bzw. Graph, Secret), Abfrage-Takt (Zahl Sekunden, Default 60),
  aktiv (Ja/Nein), primär (Ja/Nein — genau ein aktives Konto ist primär,
  siehe 4.1), Signatur (Text), Out-of-Office (Struktur: aktiv, von/bis,
  Nachricht), letzter Abruf (Zeitstempel), Verbindungsstatus (Aufzählung:
  `ok`/`fehlerhaft`)
- Beziehungen: 1—n E-Mails, Drafts, Label-Definitionen

**E-Mail**
- Attribute: Konto* (→ E-Mail-Konto), Message-ID* (Text, eindeutig je Konto),
  Absender*, Empfänger (Liste), CC (Liste), Betreff, Body-Text, Body-HTML
  (Text), Anhänge (Liste → Dokument), empfangen am (Zeitstempel), Firma (→
  Firma, optional), Kontakt (→ Kontakt, optional), Zuordnungsstatus* (Aufzählung:
  `zugeordnet`/`nicht zugeordnet`), Triage: Wichtigkeit (Aufzählung: `low`/
  `medium`/`high`/`urgent`), Handlungsbedarf (Aufzählung: `needs_reply`/
  `no_action`/`for_info`), Stimmung (Aufzählung: `positive`/`negative`/
  `neutral`), Lead-Score (Zahl 0–100), Themenkategorie (→ Themenkategorie,
  kontrollierte Liste, siehe 4.3), Sprache
  (Text), Kurzzusammenfassung (Text), explizite Deadline (Datum, optional),
  eigene Labels (Liste: → Label-Definition + Wert), gelesen (Ja/Nein, Default
  nein), Abarbeitungs-Status* (Aufzählung: `offen`/`in_arbeit`/`erledigt`,
  Default `offen`), beantwortet (Ja/Nein, abgeleitet aus gesendetem Reply-Draft,
  siehe 4.2), zugewiesen an (→ Benutzer, optional, siehe 2.1),
  Verarbeitungsstatus (Aufzählung: `ok`/`retry`/`dauerhafter Fehler`)
- Beziehungen: n—1 E-Mail-Konto; n—1 Firma/Kontakt (optional); 1—1 Draft
  (Antwort, optional); 1—n Todos (aus Extraktion)

**Label-Definition (eigene Labels)**
- Attribute: Konto* (→ E-Mail-Konto), Name*, Typ* (Aufzählung: `text`/`auswahl`/
  `ja_nein`), Optionen (Liste, bei `auswahl`), Bedeutungsbeschreibung (Text),
  KI-Instruktion (Text, versioniert), aktiv (Ja/Nein)
- Beziehungen: n—1 E-Mail-Konto; 1—n Label-Werte an E-Mails

**Themenkategorie**
- Attribute: Name* (Text, eindeutig), Sortierung (Zahl), aktiv (Ja/Nein);
  „Sonstiges" ist eine nicht löschbare System-Kategorie (Fallback, siehe 4.3)
- Beziehungen: 1—n E-Mails (Triage-Zuordnung)

**Draft (Antwort-Entwurf)**
- Attribute: Ursprungs-E-Mail (→ E-Mail, optional), Konto* (→ E-Mail-Konto),
  Empfänger (→ Kontakt **oder** freie E-Mail-Adresse bei unbekanntem Absender,
  siehe 4.5), Inhalt (Text), Modus (Aufzählung: `volltext`/`stichpunkte`),
  Tonalitäts-Scope (Aufzählung: `global`/`kunde`),
  Zustand* (Aufzählung: `queued`/`in_review`/`approved`/`rejected`/`sent`),
  Review-Notizen (Text), KI-gestützt (Ja/Nein, immer wahr bei KI-Erzeugung),
  Termin (→ Termin, optional), gesendet am (Zeitstempel)
- Beziehungen: n—1 E-Mail-Konto; n—1 E-Mail/Kontakt (optional); 1—1 Termin
  (optional)

**Template**
- Attribute: Name*, Betreff (Text mit Platzhaltern), Text (Markdown mit
  Platzhaltern), Anhänge (Liste), aktiv (Ja/Nein)
- Beziehungen: genutzt in Workflow-Schritten und manuell

**Fakt (Evidence-Ledger)**
- Attribute: Zielentität* (Firma | Kontakt | Deal), Schlüssel* (Text), Wert*
  (strukturierter Wert), Quelle (Aufzählung/Text: z. B. `email.signature`,
  `research.verified`, Konnektor-Typ), Gewicht* (Zahl 0–1), Belege (Liste:
  Text/URL), Status* (Aufzählung: `auto`/`suggested`/`accepted`/`rejected`),
  entschieden von (→ Benutzer, optional), angelegt am (Zeitstempel)
- Beziehungen: n—1 Zielentität; 1—1 Proposal (optional, Ursprung)

**Proposal**
- Attribute: Tool* (Aufzählung der schreibenden Tools: `crm_manage_entities`/
  `record_fact`/`templates_create_or_update`/`workflows_create_routine`),
  Vorhaben* (Text), Zusammenfassung (Text), Zielentität (Firma | Kontakt |
  Deal), Status* (Aufzählung: `pending`/`accepted`/`rejected`/`expired`),
  läuft ab am (Zeitstempel, TTL 48 Stunden, siehe 5.3), entschieden von (→
  Benutzer, optional), angelegt am (Zeitstempel)
- Beziehungen: n—1 Zielentität; kann 1 Fakt, 1 Objekt (Todo/Kontakt/Deal),
  1 Template oder 1 Workflow erzeugen

**Agent-Event**
- Attribute: Zielentität* (Firma | Kontakt | Deal), Typ* (Aufzählung:
  `tool_call`/`fact_auto`/`fact_suggested`/`proposal_created`/
  `proposal_decided`/`research_started`/`lead_discarded`/`question`/
  `recheck_scheduled`), Agent-Nachricht (Text), Detail (Struktur), Zeitstempel*
- Beziehungen: n—1 Zielentität; Referenz auf Proposal/Fakt/Research-Job
  (optional)

**Research-Job**
- Attribute: Firma (→ Firma, optional = Ad-hoc), Auslöser* (Aufzählung:
  `neue_firma`/`täglich`/`manuell`/`adhoc`), Status* (Aufzählung: `geplant`/
  `läuft`/`abgeschlossen`/`fehlgeschlagen`), Suchanfragen (Liste, max. 5),
  gestartet am / abgeschlossen am (Zeitstempel)
- Beziehungen: n—1 Firma (optional); erzeugt 1—n KB-Einträge und Fakten

**KB-Eintrag (Wissensbasis)**
- Attribute: Firma (→ Firma, optional), Titel, Inhalt, Zusammenfassung (Text),
  Quell-URL (Text), Entity-Tags (Liste), Herkunft (Aufzählung: `recherche`/
  `email`/`anruf`), Roh-HTML-Ablauf (Datum, +90 Tage), angelegt am (Zeitstempel)
- Beziehungen: n—1 Firma (optional); n—1 Research-Job (optional)

**Transkript**
- Attribute: Kontakt/Firma (→, optional), Audio-Metadaten (Struktur: Dateiname,
  Dauer, Sprache), Text (Text), Sprecher (Liste: Kennung + Konfidenz),
  Gesamt-Konfidenz (Zahl), KI-Kennzeichnung (Ja/Nein, immer wahr), Status
  (Aufzählung: `läuft`/`fertig`/`fehlgeschlagen`), angelegt am (Zeitstempel)
- Beziehungen: n—1 Kontakt/Firma (optional); 1—n abgeleitete Todos

### 12.4 Daten, Automation & Konnektoren

**Dokument**
- Attribute: Firma (→ Firma, optional — E-Mail-Anhänge referenzieren die
  Ursprungs-E-Mail), Titel*, Kategorie* (Aufzählung: `Angebot`/`Vertrag`/
  `Vollmacht`/`Rechnung`/`Korrespondenz`/`Sonstig`; bei Upload KI-Vorschlag mit
  HITL-Übernahme), Dateityp (Text), Größe (Zahl), Speicher-Referenz (Text),
  hochgeladen von (→ Benutzer), angelegt am (Zeitstempel)
- Beziehungen: n—1 Firma (optional); Upload/Löschung → Audit- und Logbuch-Eintrag

**Workflow**
- Attribute: Name*, Beschreibung (Text), Schritte* (geordnete Liste von
  Schritt-Definitionen), E-Mail-Konto (→ E-Mail-Konto, optional — Default:
  primäres aktives Konto, siehe 6.4), aktiv (Ja/Nein), angelegt am (Zeitstempel)
- Schritt-Definition: Aktion* (Aufzählung: `email_entwurf`/`warten`/
  `antwort_prüfen`/`todo_erstellen`/`termin_erstellen`), Template (→ Template,
  bei `email_entwurf`), Verzögerung/Wartezeit (Zahl Tage), Ersatzaktion
  (verschachtelte Schritt-Definition, bei `antwort_prüfen`), Todo-Titel +
  Priorität (bei `todo_erstellen`), Termin-Typ + Dauer (bei `termin_erstellen`)
- Beziehungen: 1—n Läufe

**Workflow-Lauf**
- Attribute: Workflow* (→ Workflow), Ziel (Firma | Kontakt)*, Zustand* (Aufzählung:
  `active`/`paused`/`completed`/`cancelled`), aktuelle Schritt-Position (Zahl),
  gestartet am (Zeitstempel)
- Beziehungen: n—1 Workflow + Ziel; 1—n Lauf-Schritte

**Workflow-Lauf-Schritt**
- Attribute: Lauf* (→ Workflow-Lauf), Schritt-Nummer*, Status* (Aufzählung:
  `pending`/`prepared`/`confirmed`/`skipped`/`done`), fällig am (Zeitstempel),
  ausgeführt am (Zeitstempel), Draft (→ Draft, optional), Todo (→ Todo,
  optional), Termin (→ Termin, optional)
- Beziehungen: n—1 Lauf

**Custom-Field-Definition**
- Attribute: Entitätstyp* (Aufzählung: `kontakt`/`firma`/`deal`), Schlüssel*
  (Text, unique je Entitätstyp, keine Kernfeld-Kollision), Label*, Feldtyp*
  (Aufzählung: `text`/`number`/`date`/`select`/`url`/`email`/`phone`/`boolean`/
  `user`), Optionen (Liste, bei `select`), Pflicht (Ja/Nein), Reihenfolge
  (Zahl), aktiv (Ja/Nein; Deaktivieren statt Löschen)
- Beziehungen: 1—n Custom-Field-Werte

**Custom-Field-Wert**
- Attribute: Definition* (→ Custom-Field-Definition), Zielentität* (Kontakt |
  Firma | Deal), Wert (typgerecht validiert)
- Beziehungen: n—1 Definition + Zielentität

**Saved View**
- Attribute: Benutzer* (→ Benutzer), Name*, Entitätstyp* (Aufzählung: `kontakt`/
  `firma`/`deal`/`todo`), Filter (Struktur), Sortierung (Struktur)
- Beziehungen: n—1 Benutzer

**Konnektor**
- Attribute: Bezeichnung*, Typ (Aufzählung: Daten — `tarifrechner`/
  `auskunftei`/`marktdaten`/`branchen_api`/`sonstige`; Funktion — `telefonie`),
  Endpunkt/Zugangsdaten (Secret),
  aktiv (Ja/Nein), Fakt-Gewicht (Zahl, Default 0,65 — stets < 0,70; nur
  Daten-Konnektoren), angelegt
  am (Zeitstempel)
- Beziehungen: 1—n Konnektor-Ergebnisse (Daten-Konnektoren)

**Konnektor-Ergebnis**
- Attribute: Konnektor* (→ Konnektor), Ziel (Firma | Kontakt)*, aufbereitete
  Daten (Struktur, lesbare Darstellung — nie Roh-JSON), abgerufen am
  (Zeitstempel), Quelle (Text), Fakt (→ Fakt, optional)
- Beziehungen: n—1 Konnektor + Ziel

**Tracking-Domain**
- Attribute: Domain*, Status (Aufzählung: `aktiv`/`inaktiv`/`pausiert`),
  konfiguriert am (Zeitstempel)
- Beziehungen: 1—n Tracking-Besucher, Intake-Formulare, Form-Submissions

**Intake-Formular**
- Attribute: Tracking-Domain* (→ Tracking-Domain), Name*, Zweck (Text),
  Felddefinitionen* (Liste: Pflicht E-Mail + Consent, optional Name/Firma/
  Nachricht/Telefon/Auswahlfelder), aktiv (Ja/Nein)
- Beziehungen: n—1 Tracking-Domain; 1—n Form-Submissions

**Tracking-Besucher**
- Attribute: anonyme ID* (Text, keine PII), Domain (→ Tracking-Domain), Firma
  (→ Firma, optional — Zuordnung über die Domain der konfigurierten
  Tracking-Domain zur Firma), erster Besuch / letzter Besuch (Zeitstempel),
  Seitenaufrufe (Zahl), Referrer (Text, nur mit Consent), UTM-Quelle/-Medium/
  -Kampagne (Text, nur letzter Wert, nur mit Consent), Consent (Ja/Nein)
- Beziehungen: n—1 Tracking-Domain; n—1 Firma (optional); 1—n Tracking-Events

**Tracking-Event**
- Attribute: Besucher* (→ Tracking-Besucher), Typ* (Aufzählung: `page_view`/
  `form_submit`/`lead_qualified`), Pfad (Text), Zeitstempel*
- Beziehungen: n—1 Besucher

**Form-Submission**
- Attribute: Domain (→ Tracking-Domain), Formular (→ Intake-Formular),
  E-Mail*, Consent* (Ja/Nein, zwingend),
  Consent-Version (Hash des akzeptierten Einwilligungstextes), Feldwerte
  (Struktur gemäß Formular-Definition), Name/Firma/
  Nachricht (Text, optional), Status* (Aufzählung: `new`/`matched`/
  `contact_created`/`ignored`), Kontakt (→ Kontakt, optional), eingegangen am
  (Zeitstempel)
- Beziehungen: n—1 Domain + Formular; 1—1 Kontakt (optional, Ergebnis der Dedupe)

### 12.5 Sicherheit & Betrieb

**API-Token**
- Attribute: Benutzer* (→ Benutzer), Name*, Hash (Secret — Klartext nur einmalig
  angezeigt), Scopes (Liste: `modul.aktion`, leer = alle Rechte des Benutzers),
  Ablaufdatum (Datum, optional), letzte Verwendung (Zeitstempel), widerrufen
  (Ja/Nein), angelegt am (Zeitstempel)
- Beziehungen: n—1 Benutzer

**Kalenderverbindung**
- Attribute: Benutzer* (→ Benutzer), Provider* (Aufzählung: `google`/
  `microsoft`), OAuth-Verweis (Secret), Status* (Aufzählung: `aktiv`/`pausiert`/
  `getrennt`), letzter Sync (Zeitstempel), Sync-Fenster (Struktur: 30 Tage
  zurück / 60 Tage voraus)
- Beziehungen: n—1 Benutzer; importiert externe Termine (read-only)

**Webhook**
- Attribute: Ziel-URL*, Secret (Secret, je Webhook eigen), abonnierte Events*
  (Liste aus: Deal angelegt/geändert, Kontakt angelegt/geändert, Firma angelegt/
  geändert, Termin angelegt, Todo angelegt/erledigt), aktiv (Ja/Nein), angelegt
  am (Zeitstempel)
- Beziehungen: 1—n Deliveries

**Webhook-Delivery**
- Attribute: Webhook* (→ Webhook), Event-Typ*, Payload-Referenz (Text), Status*
  (Aufzählung: `ausstehend`/`ausgeliefert`/`fehlgeschlagen`), Versuche (Zahl,
  max. 5), Fehlerursache (Text), letzte Auslieferung (Zeitstempel)
- Beziehungen: n—1 Webhook

**Audit-Eintrag**
- Attribute: Benutzer* (→ Benutzer), Aktion* (Aufzählung: `erstellt`/`geändert`/
  `gelöscht` + Sonderaktionen wie `import_freigegeben`, `rollback`, `login`,
  `2fa_setup`), Zieltyp* (Text), Ziel-ID* (Text), Metadaten (nur Feldnamen/
  Status/IDs — keine Rohinhalte), Zeitstempel* — unveränderbar (nur Einfügen)
- Beziehungen: referenziert Zielobjekt fachlich (keine FK-Pflicht)

**KI-Audit-Eintrag**
- Attribute: KI-Aktion* (Aufzählung: `triage`/`draft`/`agent`/`research`/
  `transkription`/`extraktion`/`guardrail`), Anweisungs-Version (Text), Modell
  (Text), Token-Verbrauch (Zahl), ausgelöste Guardrails (Liste), blockiert
  (Ja/Nein), prompt_hash (Text — nie Klartext), Latenz (Zahl), Status (Text),
  Zeitstempel*
- Beziehungen: referenziert auslösendes Objekt fachlich (E-Mail, Draft, …)

**Backup**
- Attribute: Zeitstempel*, Manifest (Struktur: Version, Prüfsummen),
  Vollständigkeits-Marker (Ja/Nein, erst nach Erfolg), Umfang (Struktur:
  Datenbank + Dateiablage + Secrets-Archiv), Ablageort (Text), Restore-Drill-
  Ergebnis (Aufzählung: `nicht geprüft`/`erfolgreich`/`fehlgeschlagen` +
  Zeitstempel)
- Beziehungen: —

**KI-Einstellungen**
- Attribute: Anbieter-Typ (Aufzählung: `externe_api`/`lokale_inferenz`/
  `deaktiviert`), API-Schlüssel-Verweis (Secret), Modell je KI-Funktion
  (Struktur: Triage, Draft, Agent, Research-Extraktion, Transkription,
  KB-Suche), Datenkategorien-Deklaration je KI-Funktion (Struktur, siehe 8.10),
  Anbieter-Eignung erklärt (Ja/Nein + Zeitstempel) + Eignungs-Referenz (Text,
  Pflicht bei externer API: AVV-Dokument/Link, siehe 8.10), Research aktiv
  (Ja/Nein — globaler Master-Schalter, siehe 5.4), Guardrail-Policy je
  KI-Funktion (Struktur, siehe 8.5), Kostenlimits (Struktur: Anfragen/Tag,
  Token-Budget/Monat — siehe 8.5), Alert-E-Mail an Admin (Ja/Nein, siehe 8.6),
  Digest-Uhrzeit (Zeit, Default 08:00, siehe 14), geändert von (→ Benutzer),
  geändert am (Zeitstempel)
- Beziehungen: steuert alle optionalen KI-Fähigkeiten (1.5)

---

## 13. Navigation & UI-Struktur

### 13.1 Shell

- **Sidebar** (Hauptnavigation, lokalisiert, einklappbar) + **Top-Bar** mit
  globaler Suche/Cmd+K (3.7), Benachrichtigungs-Glocke (siehe 14) und Benutzer-Menü.
- Zweisprachigkeit (DE/EN) und Dark Mode durchgängig (9.2).

### 13.2 Hauptnavigation (alle Benutzer)

| Bereich | Inhalt |
|---------|--------|
| **Dashboard** | **Meine heute fälligen Todos / meine heutigen Termine** (Filter „Meine / Team", 2.1) + Team-Pool-Umschalter, offene Proposals, offene & unbeantwortete Inbox, Pipeline-Übersicht (Lifecycle-Status), Research-/Anreicherungs-Status, Vertriebs-Kennzahlen Basis (Stufenverteilung); die Detail-Auswertung (Conversion, Durchlaufzeiten, Aktivität pro Benutzer) ist Admin-vorbehalten (7.6) |
| **Inbox** | E-Mail-Liste mit Triage-/Label-Badges, Zuordnung, Aktionen (4.2) |
| **Kontakte** | Liste (Saved Views, Bulk, Filter) + Detail (3.1) |
| **Firmen** | Liste + Detail mit Tabs (3.2) |
| **Deals** | Liste + Detail (3.3) |
| **Todos** | gruppierte Liste mit Drilldown (3.5) |
| **Kalender** | Monat/Woche/Tag mit Todo-Merge (3.6) |
| **Karte** | Smart-Map mit Gebieten (7.4) |
| **Wissensbasis** | Semantische + Volltext-Suche über alle KB-Einträge, Blättern pro Firma, Quellenanzeige (5.5) |
| **Setter** | Schnellerfassung (7.6) |
| **Closer** | Abschluss-Ansicht (7.6) |
| **Agent** | Chat-Oberfläche (5.1) + offene Proposals im Überblick |

### 13.3 Admin-Bereich (nur Admin)

Benutzer & Einladungen / E-Mail-Konten / **Verarbeitungsfehler** (dauerhafte
Fehler aus E-Mail-Verarbeitung, Versand und Research mit Retry-Aktion, siehe
4.2/5.4) / Konnektoren / Tracking-Domains / Workflows / Templates / Custom
Fields / Guardrails / API-Tokens / Webhooks / Audit-Log / KI-Observability /
Backup & Restore / Import / Einstellungen.

### 13.4 Einstellungen

- **Je Benutzer:** Passwort, 2FA, eigene externe Kalenderverbindungen, Sprache,
  Theme, eigene Saved Views.
- **Nur Admin:** alle organisationsbezogenen Einstellungen (13.3).

---

## 14. Benachrichtigungen

**Konzept:** In-App-Benachrichtigungszentrum (Glocke in der Top-Bar) mit
Ungelesen-Status und Deep-Link zum jeweiligen Objekt. Keine Push-
Benachrichtigungen; E-Mail-Zustellung ausschließlich für kritische Admin-Alerts
gemäß 8.6 (konfigurierbar) — ansonsten bewusst In-App-only (YAGNI).

**Auslösende Ereignisse:**

| Ereignis | Empfänger |
|----------|-----------|
| Todo heute fällig / überfällig (Tages-Digest) | zugewiesener Benutzer, sonst alle |
| Workflow-Schritt vorbereitet (wartet auf Bestätigung) | zugewiesener Benutzer, sonst alle |
| Offenes Agent-Proposal (HITL-Entscheidung nötig) + Proposal-Ablauf-Erinnerung (12 h vorher, 5.3) | alle Benutzer |
| Termin-Erinnerung (konfigurierte Zeit vor Start, siehe 3.6) | zugewiesener Benutzer, sonst alle |
| E-Mail mit `needs_reply` noch ohne Zuordnung/Aktion | alle Benutzer |
| Research-Job abgeschlossen / fehlgeschlagen | alle Benutzer |
| Dauerhafter Verarbeitungsfehler (E-Mail/Versand) | Admin |
| Import abgeschlossen / fehlgeschlagen | Admin |
| Backup abgeschlossen / fehlgeschlagen, Restore-Drill-Ergebnis | Admin |
| Guardrail-Spitze / LLM-Fehlerrate-Alert (8.6) | Admin |

**Regeln:** Jede Benachrichtigung referenziert genau ein Objekt (Deep-Link);
Erledigung des zugrundeliegenden Objekts (z. B. Proposal entschieden) markiert die
Benachrichtigung automatisch als gelesen.

**Aggregation (Spam-Schutz):** Benachrichtigungen gibt es in zwei Klassen:
- **Sofort (einzeln):** handlungsbedürftige Ereignisse — offene Proposals,
  dauerhafte Verarbeitungsfehler, Import-/Backup-/Research-Fehlschläge,
  Alerts (8.6), Rollenwechsel.
- **Tages-Digest (aggregiert):** Mengen-Ereignisse — fällige/überfällige Todos
  werden einmal täglich als **ein** Eintrag mit Zähler zusammengefasst
  („X Todos heute fällig, Y überfällig") statt einer Benachrichtigung pro Todo.
  Der Digest erscheint morgens (konfigurierbare Uhrzeit, Default 08:00).

---

## 15. Akzeptanzkriterien je Modul

Fachliche, als E2E-Szenario prüfbare Kriterien (je Rolle Admin/Benutzer, wo
relevant). Sie konkretisieren §9.5.

### 15.1 CRM-Kern (Modul 3)
- Firma mit Pflichtfeldern anlegen → Geocoding setzt Koordinaten; Firma erscheint
  in Liste und Suche.
- Lifecycle-Status einer Firma ändern → Logbuch-Eintrag entsteht.
- Kontakt anlegen, Firma verknüpfen, Verknüpfung ändern → Detailansichten
  konsistent.
- Deal auf `won` setzen → Vorschlag „terminierte Wiedervorlage Anschlussvertrag"
  (3 Monate vor Vertragsende): einmal bestätigt, existiert das Todo dauerhaft;
  Logbuch- und Audit-Eintrag vorhanden. Bei zukünftigem Vertragsbeginn
  zusätzlich „Lieferstart prüfen"-Vorschlag; Energiedaten-Aktualisierung wird
  vorgeschlagen.
- Deal auf `lost` setzen → Reaktivierungs-Wiedervorlage-Vorschlag (Default
  6 Monate); Deal `won` → `widerrufen` → Widerrufsdatum + Begründung Pflicht,
  keine Anschluss-Wiedervorlage, separate Auswertungszählung.
- Doppelverkauf: zweiten Deal an Kontakt mit laufendem `won`-Vertrag anlegen →
  deutliche Warnung; Anlage möglich, Warnung protokolliert.
- Deal mit Kontakt-Bezug ohne Firma anlegen (Privatkunde) → möglich; Deal ohne
  jeden Bezug → abgelehnt.
- Dubletten-Merge: zwei Kontakte zusammenführen → Feldkonflikt entscheidbar,
  Referenzen wandern, Quell-Objekt gelöscht, Audit-Eintrag vorhanden.
- Todo mit Fälligkeit „Morgen" anlegen → erscheint in Gruppe „Morgen"; im
  Kalender am Fälltag mit Checkbox abhakbar.
- Termin anlegen → Monatsansicht zeigt ihn farbcodiert; Drag-and-Drop eines Todos
  verschiebt dessen Fälligkeit; Einladung senden → E-Mail mit ICS-Anhang im
  Gesendet-Ordner, `email`-Aktivität protokolliert.
- Globale Suche ab 2 Zeichen findet Kontakt/Firma/Deal/Todo/Dokument gruppiert;
  Cmd+K öffnet dieselbe Suche.
- E-Mail-Volltextsuche: Begriff aus einem E-Mail-Body → Treffer öffnet die
  E-Mail in der Inbox.
- Firma mit verknüpftem Kontakt löschen → abgelehnt mit Hinweis; Firma mit
  zugeordneter E-Mail löschen → abgelehnt mit Hinweis; Kontakt löschen →
  Deals bleiben, zugehörige Todos/Fakten/Proposals/Transkripte/Tracking-Daten
  mitgelöscht (volle Kaskade gemäß 16.1).
- Bulk: 3 Kontakte auswählen, Status ändern + Custom Field setzen → beides
  übernommen (Validierung schlägt bei ungültigem Wert fehl).
- Saved View speichern, laden, löschen → Filter+Sortierung reproduzierbar.
- Custom Field (select): Wert außerhalb der Optionsliste wird abgelehnt;
  Pflichtfeld ohne Wert blockiert Speichern.

### 15.2 E-Mail & KI-Triage (Modul 4)
- IMAP-Konto einrichten → Verbindungs- und Versandtest erfolgreich.
- Eingehende E-Mail bekannter Domain → automatisch Firma/Kontakt zugeordnet,
  Triage-Badges sichtbar.
- E-Mail mit `needs_reply` → offenes Todo mit Link zur Ursprungs-E-Mail,
  Fälligkeit Empfang + 1 Werktag (falls keine Deadline erkannt).
- E-Mail öffnen → Gelesen-Status gesetzt; Dashboard zählt nur ungelesene.
- Abarbeitungs-Status: E-Mail auf `erledigt` setzen → verschwindet aus der
  Default-Inbox; Reply-Draft senden → E-Mail zeigt „beantwortet" automatisch.
- Triage-Korrektur: Handlungsbedarf manuell auf `needs_reply` ändern → Todo
  wird nachgeholt; korrigierte Labels bleiben bei erneuter Verarbeitung stabil.
- Backfill: Konto mit 90-Tage-Historie einrichten → alte E-Mails erscheinen
  mit Abarbeitungs-Status `erledigt`, **ohne** Todo-Extraktion (letzte 3 Tage
  voll verarbeitet inkl. Todos).
- E-Mail bekannte Absender-Adresse → Stufe-0-Match ordnet direkt den Kontakt
  zu (auch Freemail/Privatkunde); unbekannter Privatkunde ohne Firma →
  Kontakt ohne Firma, Quelle „Email".
- E-Mail von unbekannter Domain mit Firma in Signatur → Vorschlag
  Firmenneuanlage (HITL); ohne Vorschlag bleibt „nicht zugeordnet".
- Eigenes Label pro Postfach definieren → Inbox danach filterbar.
- Draft erzeugen → Review → freigeben → senden: gesendete E-Mail im
  Gesendet-Ordner, mit Draft/Kontakt/Firma verknüpft; „KI-gestützt"-Kennzeichnung
  vorhanden. Ablehnen mit Notiz möglich; kein Versand ohne Freigabe.
- E-Mail ohne zugeordneten Kontakt → Draft mit freier Empfänger-Adresse möglich
  (optional Inline-Kontakt-Anlage).
- E-Mail mit präpariertem HTML-Anhang/Skript → Darstellung bereinigt, kein
  aktiver Inhalt; blockierter Anhang gesperrt mit Hinweis.
- Template mit Platzhaltern → Vorschau zeigt echte Daten.

### 15.3 KI-Agent & Research (Modul 5)
- Agent-Chat: `search_crm` liefert Treffer; `crm_manage_entities` erzeugt
  Proposal → Freigabe legt Objekt an, Ablehnung verwirft es.
- `record_fact` → Proposal → Freigabe → Fakt hat Status `accepted` (unabhängig
  vom Gewicht, keine zweite Bewertungsstufe); Ablehnung → kein Fakt.
- Automatische Fakt-Quelle (Research/Extraktion): Gewicht ≥ 0,70 → Status
  `auto`; 0,30–0,70 → `suggested`; < 0,30 → verworfen.
- Proposal nach 48 Stunden ohne Entscheidung → `expired` (bleibt einsehbar und
  reaktivierbar); 12 Stunden vor Ablauf erscheint die Erinnerungs-Benachrichtigung.
- Neue Firma (Research aktiviert) → Research-Job läuft, KB-Eintrag + Fakten
  entstehen, Anreicherungs-Status sichtbar.
- Audio hochladen → Transkript mit Kennzeichnung, Verknüpfung + Todo möglich.
- Agent-Tab einer Entität zeigt Ereignis-Timeline (Badges, aufklappbar).

### 15.4 Dokumente & Daten (Modul 6)
- Dokument hochladen → KI-Kategorievorschlag, Übernahme per Bestätigung;
  Logbuch- und Audit-Eintrag.
- Import: CSV hochladen → Mapping → Dubletten-Entscheidung → Import; Rollback
  innerhalb 30 Tage stellt Vorzustand her (ohne spätere manuelle Änderungen zu
  überschreiben).
- Export Kontakte als CSV und JSON → maschinenlesbar, vollständig.
- Vertriebs-Report: Forecast-Widget zeigt Summe offener Monatswerte je
  erwartetem Abschlussmonat; Zeitreihe zeigt won-Werte je Monat (12 Monate)
  inkl. Wandlungsquote (widerrufen separat gezählt); Export-Job
  `vertriebsreport` liefert CSV mit denselben Zahlen.
- Formulare: zweites Intake-Formular mit eigenem Feldsatz anlegen →
  Submissions landen mit Formular-Referenz und Consent-Version.
- Workflow: Lauf starten → Schritt `prepared` → bestätigen → nächster Schritt;
  pausieren/fortsetzen/abbrechen funktioniert; eingehende Antwort beendet Warten
  (Antwort = E-Mail des Kontakts oder der Firmen-Domain nach `prepared`, keine
  Auto-Reply); Ersatzaktion feuert nur nach vollständig verstrichener Wartezeit.
- Workflow-Draft: nutzt konfiguriertes Konto, sonst primäres aktives Konto;
  ohne primäres Konto ist der Schritt blockiert (Hinweis, Lauf pausiert).
- Tracking: Seitenaufruf mit/ohne Consent (ohne Consent kein Referrer/UTM);
  Form-Intake: bekannte E-Mail → `matched`, neue → `contact_created`,
  Spam-Muster → `ignored`.

### 15.5 Energie & Konnektoren (Modul 7)
- Energiedaten-Maske ausfüllen → negative kWh und ungültige Zählnummer werden
  abgelehnt.
- Kündigungsfrist-Wiedervorlage: Vertragsende + Kündigungsfrist erfassen →
  Vorschlag „Kündigungsfrist läuft ab" erscheint einmalig; bestätigt →
  dauerhaftes Todo mit korrekter Fälligkeit.
- Geofencing offline: Check-In ohne Netz aufzeichnen → 2h-Fenster beginnt erst
  mit Synchronisation; D2D-Deal offline anlegen → wird mit Sync übernommen.
- Telefonie-Konnektor: aktiven Telefonie-Konnektor konfigurieren →
  Telefonnummer am Kontakt klickbar; Klick wählt und erzeugt `call`-Aktivität
  mit Laufzeit; Anruf-Notiz nach Gespräch befüllbar; ohne Konnektor bleibt
  Telefon ein manuelles Aktivitätsfeld (1.5). Eingehender Anruf von bekannter
  Nummer → Kontakt öffnet sich mit Notiz-Aufforderung.
- Tarifrechner (PLZ + Verbrauch) → Top-3-Tarife, Ersparnis, CO₂ aufbereitet
  sichtbar.
- Karte: Pins farbcodiert nach Vertriebsstufe (Mapping 7.4), Gebiets-Polygon,
  Cluster, Klick → Sidepanel je Pin-Typ.
- Geofencing: D2D-Deal ohne gültigen Check-In → blockiert; Check-In außerhalb
  Radius → blockiert + Fraud-Log; innerhalb Radius → D2D-Deal-Anlage frei
  (2h-Fenster, mehrere Deals zum selben Kontakt möglich).
- Deal mit Abschlussart `buero` → ohne Check-In jederzeit anlegbar (auch
  Desktop, auch außerhalb jedes Radius).
- Admin-Override für D2D-Deal ohne Check-In → nur mit Begründung, Audit-Eintrag
  und Fraud-Log-Markierung „Override".
- Setter-Schnellerfassung (< 30 s, manueller Prüf-Kriterium) → Kontakt erhält
  Vertriebsstufe `setter_gespraech`; Closer-Termin gebucht → `closer_termin`;
  Deal `won` → `abgeschlossen`. Jeder Wechsel mit Logbuch-Eintrag.
- Closer-View zeigt Setter-Daten read-only + Einsparrechner + Deal-Anlage.
- Management-Dashboard zeigt Conversion pro Stufe (berechnet aus
  Vertriebsstufen-Verteilung + Zeitstempeln).

### 15.6 Security & Betrieb (Modul 8)
- 5 fehlgeschlagene Logins → 5-Minuten-Sperre.
- 2FA optional: Aktivierung mit Recovery-Codes; Login danach nur mit TOTP/
  Recovery-Code; Deaktivierung nur mit gültigem TOTP. Benutzer ohne 2FA können
  sich weiterhin nur mit dem Passwort anmelden.
- Passwort-Reset durch Admin → Benutzer muss Passwort ändern (und 2FA neu
  einrichten, falls aktiv); alle bestehenden Sitzungen des Benutzers sind
  invalidiert; Notfall-Wiederherstellung für einzigen Admin protokolliert.
- Letzter aktiver Admin: Deaktivierung und Herabstufung werden abgelehnt.
- Rollenwechsel Admin↔Benutzer: nur mit Bestätigung, Audit-Eintrag und
  Benachrichtigung an den Betroffenen.
- API-Token mit Scope `contacts.read` kann Deals weder lesen noch schreiben;
  Admin-Bereiche (Backup, Guardrails) sind per Token nicht erreichbar.
- Webhook-Auslieferung: Payload enthält den definierten Umschlag
  (event/timestamp/object_type/object_id/actor/data); HMAC-Prüfung schlägt bei
  Manipulation fehl.
- Form-Intake: Submission speichert Consent-Version (Hash) + Zeitstempel.
- Löschbegehren für Kontakt → Kontakt + Todos/Fakten/Transkripte/Tracking
  gelöscht; E-Mails der Person enthalten Platzhalter `[PERSONENDATEN-ENTFERNT]`
  (übriger Body intakt); Audit-/KI-Audit unverändert; Abschluss als
  Audit-Eintrag nachweisbar.
- Auskunft „Datenexport für Person X" → JSON mit allen personenbezogenen Daten
  (Umfang gemäß 9.3), Drittadressen in Bodies geschwärzt.
- API-Token: Klartext nur einmal sichtbar; Nutzung dokumentiert; Widerruf →
  sofort ungültig; Scope `contacts.read` gewährt keine Deal-Rechte; Token eines
  Benutzers kann nie Admin-Operationen ausführen.
- Webhook: Testauslieferung mit HMAC-Signatur; fehlgeschlagene Auslieferung →
  5 Retries, Delivery-Status sichtbar.
- Audit: Objekt anlegen/ändern/löschen → unveränderlicher Eintrag ohne
  Rohinhalte.
- Guardrail-Blockierung → Aufruf endet geordnet, KI-Audit-Eintrag vorhanden.
- Backup → Restore-Drill: Daten anlegen, sichern, wischen, wiederherstellen,
  Bestände und Dateien identisch.

### 15.7 Querschnittliche Regeln (Modul 16)
- Firma mit verknüpften Kontakten/Deals löschen → abgelehnt mit Hinweis.
- Kontakt löschen → Deals bleiben (Kontakt-Verweis leer), Todos, Geo-Check-Ins,
  Transkripte, Fakten, Proposals, Agent-Events, Tracking-Events und
  Form-Submissions mitgelöscht, E-Mail-Zuordnungen bleiben (16.1).
- Zeitzone: alle Zeiten in Installations-Zeitzone (Default Europe/Berlin).
- Parallel-Edit: letztes Speichern gewinnt, Änderung im Logbuch/Audit sichtbar.
- Jede Listen-/Detailansicht hat einen lokalisierten Empty-State mit Aktion.

### 15.8 Standards & Compliance (Module 19–22)
- UWG: Draft an Kontakt ohne E-Mail-Werbe-Einwilligung → deutlich sichtbare
  Warnung in der Review; Versand bleibt möglich, Warnung + Versand im Audit-Log;
  Widerruf der Einwilligung am Kontakt wirkt sofort (Logbuch-Eintrag).
- TDDDG: Website ohne Consent → kein Cookie/localStorage-Eintrag auf dem
  Endgerät nachweisbar; mit Consent → anonyme ID gesetzt.
- AI Act: Jede KI-Ausgabe trägt sichtbare + maschinenlesbare Kennzeichnung;
  KI-Einweisungshinweise für Benutzer und Admin vorhanden.
- BFSG: Tastatur-Rundgang durch alle Hauptansichten ohne Maus möglich;
  Kontrast-Prüfung gegen WCAG 2.1 AA bestanden.
- Lizenz-Compliance: Aufnahme einer AGPL-Abhängigkeit → Build blockiert;
  `THIRD-PARTY-LICENSES` liegt der Auslieferung bei.
- Definition of Done: Änderung ohne E2E-Nachweis der betroffenen
  Akzeptanzkriterien → gilt nicht als fertig (Prozess-Prüfung).
- Guardrail-Policy: Triage-Extraktion erhält E-Mail-Adressen unmaskiert
  (Extraktionszweck), Agent-Chat erhält PII maskiert; Admin-Überschreibung
  nur mit Bestätigung + Audit-Eintrag.
- KI-Funktion ohne AVV-fähigen Anbieter → bleibt deaktiviert,
  Konfigurationsansicht zeigt den Grund.

---

## 16. Querschnittliche Fachregeln

Regeln, die mehrere Module betreffen und bewusst zentral festgelegt werden.

### 16.1 Löschregeln & Kaskaden

Grundsatz: **Hart-Löschung** (kein Papierkorb, keine Soft-Deletes) mit Schutz
der Referenz-Integrität. Audit-/Logbuch-Einträge bleiben stets erhalten
(unveränderbar); Autoren-Referenzen gelöschter Benutzer werden pseudonymisiert.
Löschungen erfordern eine Bestätigung; Bulk-Delete gibt es nicht (3.8).

| Objekt | Löschverhalten |
|--------|----------------|
| Firma | Nur möglich ohne verknüpfte Kontakte/Deals **und ohne zugeordnete E-Mails** — sonst Ablehnung mit Hinweis (erst auflösen/umschlüsseln bzw. E-Mail-Zuordnung ändern). Aktivitäten, Dokumente, Fakten, KB-Einträge, Workflow-Läufe der Firma werden mitgelöscht |
| Kontakt | Löscht zugehörige Todos, Geo-Check-Ins, Transkripte, Fakten, Proposals, Agent-Events, Tracking-Events und Form-Submissions des Kontakts; Deals bleiben bestehen (Kontakt-Verweis wird leer); E-Mail-Zuordnungen bleiben (Absender weiterhin sichtbar) |
| Deal | Löscht Deal-Aktivitäten und -Fakten; Firma/Kontakt unberührt |
| E-Mail-Konto | Nur deaktivierbar, solange E-Mails existieren; endgültiges Löschen entfernt alle importierten E-Mails und Drafts des Kontos |
| E-Mail | Nicht einzeln löschbar in der UI (Aufbewahrung nach LK-5); Löschung nur im Rahmen des Löschkonzepts (9.3) oder mit dem E-Mail-Konto |
| Dokument | Löscht die Datei; Logbuch-/Audit-Eintrag bleibt |
| Workflow | Löschen beendet aktive Läufe (`cancelled`); abgeschlossene Läufe bleiben im Audit nachvollziehbar |
| Template | Nur deaktivierbar, solange in Workflows referenziert; sonst löschbar |
| Custom-Field-Definition | Deaktivieren statt Löschen (3.9); endgültiges Löschen nur ohne existierende Werte |
| Benutzer | Deaktivieren statt Löschen (2.3); Löschung nur über Löschkonzept (LK-6) |
| Konnektor | Löschen behält Konnektor-Ergebnisse (mit Quell-Hinweis „Konnektor entfernt") |
| Tracking-Daten | Automatisch nach LK-3 (12 Monate) |

### 16.2 Zeitzone & Datumsformate

- Alle Zeitstempel werden in der **Zeitzone der Installation** gespeichert und
  angezeigt (beim Setup konfiguriert, Default `Europe/Berlin`). Keine
  Pro-Benutzer- oder Pro-Termin-Zeitzonen (YAGNI für die Zielgruppe).
- Datumsformat TT.MM.JJJJ, Zeit HH:MM (24-Stunden), Wochenstart Montag —
  unabhängig von der UI-Sprache.

### 16.3 Paralleles Bearbeiten

Mehrere Benutzer können dasselbe Objekt gleichzeitig bearbeiten. Regel:
**letztes Speichern gewinnt** — kein Sperren, kein Merge-Dialog (Einfachheit).
Wer wann was geändert hat, bleibt über Logbuch und Audit-Log jederzeit
nachvollziehbar (Schutz vor stillen Überschreibungen durch Transparenz statt
durch Technik).

### 16.4 Empty States & Onboarding

Jede Listen- und Detailansicht definiert einen **Empty-State** mit
Handlungsangebot (lokalisiert, niemals eine Fehlermeldung):

| Ansicht | Empty-State |
|---------|-------------|
| Dashboard | Setup-Checkliste: E-Mail-Konto einrichten → erster Kontakt → optional Demo-Daten einspielen |
| Inbox | „Noch keine E-Mails — E-Mail-Konto einrichten" (Direktlink, nur Admin) |
| Kontakte/Firmen/Deals/Todos | „Ersten Eintrag anlegen" + Import-Angebot |
| Kalender | „Keine Termine — Termin anlegen oder externen Kalender verbinden" |
| Karte | Hinweis auf fehlende Geokoordinaten + Link zu Firmen ohne Adresse |
| Agent-Panel | „Keine Agent-Aktivität" (5.7) |
| Wissensbasis | Hinweis auf Research-Aktivierung (8.9) |
| Konnektor-Ansicht | „Konnektor konfigurieren" (nur Admin) |

### 16.5 Währung, Zahlen & Formate

- Alle Geldbeträge in **EUR** (fix, siehe 3.3 und 10.3).
- Zahlenformatierung folgt der UI-Sprache (DE: 1.234,56 / EN: 1,234.56).
- Einheiten werden in Konnektor-Ansichten mitgeführt (kWh, CO₂-kg, %).

---

## 17. Implementierungsphasen

Der Gesamtumfang dieser Spec bleibt unverändert — die Phasen definieren nur die
**Reihenfolge** für die Implementierungsplanung. Jede Phase endet mit einem
lauffähigen, testbaren Stand (Docker-Stack grün, E2E der Phase grün).

**Prioritätsgrundsatz:** Der generalistische Kern (Module 2–6, 8) wird
branchenneutral gebaut; die **Vertikale Energie/Feldvertrieb (Modul 7 ab 7.2)
ist die Leit-Referenz und läuft in den Phasen 1 und 5 als erste ausgearbeitete
Anwendung** — d. h.: Energiedaten-Feldstruktur, Geocoding, Vertriebsstufen und
Setter/Closer-Flow sind Teil der frühen Phasen (weil der Referenz-Kunde sie
am ersten Tag braucht), während Konnektoren/Tarifrechner/Karte in Phase 5
folgen. Andere Vertikalen entstehen später ausschließlich über neue
Konnektor-Typen und Custom Fields — ohne Kernänderung.

| Phase | Inhalt | Voraussetzungen | Exit-Kriterium (grob) |
|-------|--------|-----------------|----------------------|
| **1 — Kern & Fundament** | Modul 2 (Benutzer/Rollen), Modul 3 (CRM-Kern komplett, inkl. Energiedaten-Feldstruktur am Kontakt, Vertriebsstufen und Dubletten-Merge 3.10), 6.1 Dokumente, 6.2 Import, 6.3 Export, 8.1 Auth (inkl. optionaler 2FA), 8.4 Audit-Log (Schreiben ab Phase 1, da Rollenwechsel/Löschungen/Importe audit-pflichtig sind), 14 Benachrichtigungen (Zentrum + Todo-Digest), §16 Querschnittsregeln, Docker-Kern-Stack, Setup-Assistent (2.3) | — | CRM ohne KI vollständig bedienbar; Daten importier-/exportierbar; Login + Rollen funktionieren; Audit-Einträge werden geschrieben; **Energie-Referenzkunde kann Kontakte mit Energiedaten/Vertriebsstufen führen** |
| **2 — E-Mail manuell** | 4.1 E-Mail-Konten (IMAP/Graph, primäres Konto, Backfill), 4.2 Abruf/Parsen/Zuordnung Stufe 0/3a + manuelle Nachpflege, Inbox ohne KI-Badges, 4.6 Templates, 4.5 Drafts (manuell, ohne KI-Generierung), Versand, Termin-Einladungen (3.6 ICS) | Phase 1 | E-Mails laufen rein, werden zugeordnet, manuell beantwortet und gesendet; Termine bestätigbar |
| **3 — KI-Grundlagen** | 8.9 KI-Konfiguration, 8.10 Datenschutz-Rahmen, 8.5 Guardrails (inkl. Policy je Funktion), 4.2 Stufe 3b/3c (KI-Extraktion), 4.3 Triage + eigene Labels + Lead-Score + Themenkategorien, 4.4 Todo-Extraktion, 4.5 KI-Generierung + Tonalität, 3.4 KI-Extraktions-Chips | Phase 2 + konfigurierter KI-Anbieter | Triage, KI-Drafts und Extraktion funktionieren mit HITL; ohne Anbieter bleibt Phase-2-Stand erhalten (1.5) |
| **4 — Agent, Research & Automation** | Modul 5 komplett (Agent, Evidence-Ledger, Proposals, Research, KB, Transkription, Transparenz), 6.4 Workflows, 8.6 KI-Observability, Vertriebs-Report (6.3 Forecast/Zeitreihe) | Phase 3 | Agent mit Proposal-Flow, Research befüllt KB, Workflows laufen semi-automatisch |
| **5 — Vertikale Energie & Betrieb** | Modul 7 komplett (Konnektor-Schiene + Telefonie-Konnektor 7.1a, Energiedaten-Automatik 7.2, Tarifrechner 7.3, Karte 7.4, Geofencing 7.5, Setter/Closer-Views 7.6), 6.5 Tracking/konfigurierbare Formulare, 8.2 API-Tokens, 8.3 Webhooks, 8.7 Backup/Restore inkl. Drill, Löschkonzept-Betrieb (9.3) | Phase 4 | Gesamtsystem gemäß dieser Spec inkl. Referenz-Vertikale; Restore-Drill gegen RTO/RPO bestanden |

**Regeln für die Phasen:** Features werden nicht zwischen Phasen verschoben,
um Abhängigkeiten zu brechen — die Modul-Zugehörigkeit dieser Spec bleibt
verbindlich. Optionale Fähigkeiten (1.5) sind ab ihrer Phase verfügbar, aber
nie Voraussetzung für das Exit-Kriterium einer früheren Phase.

---

## 18. Glossar

| Begriff | Definition | Referenz |
|---------|-----------|----------|
| **Admin / Benutzer** | Die beiden einzigen Rollen; Admin zusätzlich mit Admin-Bereich | 2 |
| **Abschlussart** | Deal-Attribut `buero`/`d2d`; steuert Geofencing-Pflicht | 3.3, 7.5 |
| **Anreicherungs-Status** | Sichtbarer Research-Zustand pro Firma | 5.4 |
| **Audit-Log** | Unveränderliches Wer-wann-was-Protokoll ohne Rohinhalte | 8.4 |
| **Consent-Version** | Hash des bei Form-Intake akzeptierten Einwilligungstextes | 6.5 |
| **Custom Field** | Admin-definiertes Zusatzfeld an Kontakt/Firma/Deal | 3.9 |
| **Deal** | Abschluss-Erfassung (kein Stage-Board-Objekt) | 3.3 |
| **Draft** | Antwort-Entwurf mit zwingender Review-Queue vor Versand | 4.5 |
| **Evidence-Ledger** | Gewichtete Fakten pro Entität mit Quellen und Schwellen | 5.2 |
| **Fakt** | Einzelne gewichtete Erkenntnis im Evidence-Ledger | 5.2 |
| **Graceful Degradation** | Deaktivierte optionale Fähigkeiten erscheinen nicht fehlerhaft, sondern gar nicht | 1.5 |
| **Guardrail** | Zentrale Prüfung aller KI-Aufrufe (Inbound/Outbound/Kosten/Themen) | 8.5 |
| **HITL** | Human-in-the-Loop: explizite menschliche Bestätigung vor Folgewirkung | 9.4 |
| **Inbox** | E-Mail-Liste mit Triage-Badges, Zuordnung und Aktionen | 4.2 |
| **KI-Audit** | Getrenntes Protokoll aller KI-Aktionen (nur prompt_hash, nie Klartext) | 8.4 |
| **Konnektor** | Implementierter Adapter für eine externe Quelle — Daten-Konnektoren (aufbereitete Ansichten) und Funktions-Konnektoren (z. B. Telefonie) | 7.1 |
| **Label (eigenes)** | Pro Postfach definierte Triage-Kategorie mit KI-Instruktion | 4.3 |
| **Lead-Score** | KI-Priorisierungshilfe 0–100 je E-Mail, ohne Folgeautomatik | 4.3 |
| **Logbuch** | Aktivitäten-Historie einer Entität (manuell + System-Einträge) | 3.4 |
| **Löschklasse (LK)** | Aufbewahrungs-/Löschfrist-Kategorie nach Löschkonzept | 9.3 |
| **Primäres E-Mail-Konto** | Das eine aktive Konto für Workflow-Versand/Fallback | 4.1 |
| **Proposal** | Persistenter, zeitbegrenzter Vorschlag des Agenten zur HITL-Entscheidung | 5.3 |
| **Research-Job** | Automatische Unternehmens-/Personenrecherche pro Firma | 5.4 |
| **Saved View** | Persönliche benannte Filter+Sortierung pro Liste | 3.8 |
| **Tonalitätsprofil** | Stilprofil aus E-Mail-Historie (global oder pro Kunde) für Drafts | 4.5 |
| **Tracking-Domain** | Admin-konfigurierte Website für First-Party-Tracking | 6.5 |
| **Triage** | KI-Klassifikation eingehender E-Mails (Label-Matrix) | 4.3 |
| **Vertriebsstufe** | D2D-Pipeline-Zustand am Kontakt (lead → … → abgeschlossen/verloren); ersetzt den Besuchsstatus auch für Kartenfarben | 7.6 |
| **Wiedervorlage** | Todo mit Firma + Fälligkeit (kein eigenes Objekt) | 3.5 |
| **Wissensbasis (KB)** | Pro Kunde geführte Wissenssammlung, semantisch + Volltext durchsuchbar | 5.5 |
| **Workflow-Lauf** | Ausführung einer Workflow-Definition auf einer Firma/Kontakt mit Snapshot-Semantik | 6.4 |

---

## 19. Rechtsrahmen (EU & Deutschland)

Verbindliche rechtliche Anforderungen an das Produkt, recherchiert und
konsolidiert (Stand 2026-08-22). Jede Anforderung ist einer umsetzenden
Spec-Stelle zugeordnet.

### 19.1 EU AI Act (Verordnung (EU) 2024/1689)

- **Geltungszeitpunkte:** in Kraft seit 01.08.2024; Verbote + KI-Kompetenz
  (Art. 4) seit 02.02.2025; vollständige Anwendung inkl. Transparenzpflichten
  (Art. 50) seit **02.08.2026** — damit vollständig anwendbar.
- **Klassifizierung:** „limited risk" (kein Anhang-III-Hochrisikosystem, keine
  verbotene Praxis) — begründet in 9.4.
- **Umgesetzte Pflichten:** Transparenz/Kennzeichnung (5.7, 9.4), menschliche
  Aufsicht/HITL (9.4), KI-Kompetenz-Einweisungen (9.4), Guardrails (8.5),
  KI-Audit mit prompt_hash (8.4), Datenkategorien-Deklaration (8.10).
- **Beobachtungspflicht:** Änderungen der Durchführungsleitlinien der
  EU-Kommission (u. a. zu Art. 6) werden je Release geprüft (21.5).

### 19.2 DSGVO & BDSG

- Die datenschutzrechtlichen Fachanforderungen sind vollständig in §9.3
  definiert (Löschklassen nach DIN EN ISO/IEC 27555, Portabilität, Log-Hygiene,
  Löschbegehren-Mechanik, Auskunft).
- **Ergänzend (BDSG):** Verarbeitung besonderer Kategorien personenbezogener
  Daten (Art. 9 DSGVO) findet nicht statt und ist fachlich ausgeschlossen —
  weder Triage noch Research noch Transkription dürfen solche Daten
  kategorisieren oder anreichern; Guardrails (8.5) blockieren entsprechende
  Verarbeitungszwecke nicht technisch, die Datenkategorien-Deklaration (8.10)
  weist sie jedoch nicht aus — das ist die verbindliche Zusage.
- **Auftragsverarbeitung:** Das System selbst verarbeitet Daten für die
  Organisation (Single-Tenant = eigene Installation); für externe KI-Anbieter
  gilt der Eignungs-Rahmen (8.10: AVV/EU-Verarbeitung als Voraussetzung).

### 19.3 TDDDG (Telekommunikation-Digitale-Dienste-Datenschutz-Gesetz)

- **Endgeräte-Zugriff (TDDDG, Einwilligungsvorbehalt):** Zugriff auf
  Informationen in der Endeinrichtung des Nutzers (Cookies, localStorage,
  Device-Fingerprinting) nur mit Einwilligung — umgesetzt in 6.5: ohne Consent
  keinerlei Speicherung auf dem Endgerät, serverseitige Zählung ohne Gerätebezug.
- **Informationspflichten:** Das Tracking-Snippet erfordert auf der
  Kunden-Website die üblichen Hinweistexte (Zweck, Einwilligung, Widerruf);
  die Verantwortung für die Website liegt beim Kunden, das System liefert die
  technisch consent-konforme Implementierung.

### 19.4 UWG § 7 (Werbung per Telefon/E-Mail)

- **Telefonwerbung:** gegenüber Verbrauchern nur mit vorheriger ausdrücklicher
  Einwilligung (Opt-in); gegenüber Unternehmen genügt mutmaßliche Einwilligung.
- **E-Mail-Werbung:** nur mit vorheriger ausdrücklicher Einwilligung; Ausnahme
  Bestandskunden (§ 7 Abs. 3: eigene ähnliche Waren/Dienstleistungen, kein
  Widerspruch, Hinweis bei Erhebung und jeder Verwendung).
- **Umsetzung im System:** Einwilligungs-Felder am Kontakt (3.1), UWG-Warnung
  in Draft-Review (4.5) und Workflow-Vorbereitung (6.4) mit Audit-Protokoll;
  keine Verschleierung der Absenderidentität + Widerspruchshinweis in jeder
  ausgehenden E-Mail (4.5). Das System erzwingt keinen Versand-Stopp (die
  rechtliche Bewertung des Einzelfalls bleibt beim Menschen), macht aber jede
  einwilligungslose Werbekommunikation sichtbar und nachweisbar.
- **D2D-Außendienst:** Hausbesuche fallen nicht unter § 7 UWG (keine
  Fernkommunikation); Geofencing (7.5) dokumentiert die Vor-Ort-Situation.

### 19.5 BFSG (Barrierefreiheitsstärkungsgesetz)

- Seit 28.06.2025 gelten Barrierefreiheits-Anforderungen auch für die
  Privatwirtschaft (Umsetzung Richtlinie (EU) 2019/882); technische Basis:
  EN 301 549 / WCAG 2.1 Level AA.
- **Umsetzung:** verbindliches Ziel in 9.2; Nachweis je Implementierungsphase
  (21.3).

### 19.6 Geprüfte Nicht-Anwendbarkeit (Due-Diligence-Vermerk)

- **NIS2:** nicht anwendbar — das System ist kein „besonderes/wichtiges
  Unternehmen" im Sinne der Sektorenliste; die Sicherheits-Praktiken aus §20
  werden dennoch angewendet.
- **EU Data Act:** nicht anwendbar — keine IoT-/vernetzte-Produkt-Daten.
- **DORA:** nicht anwendbar — kein Finanzsektor-Bezug (Invoicing/Finanzen sind
  ausgeschlossen, 1.4).
- **EnWG:** ausgeschlossen (1.4).

---

## 20. Qualitäts- & Sicherheitsstandards

Verbindliche Standards, an denen sich Entwicklung und Betrieb messen lassen
(siehe auch Entwicklungsstrategie §21).

### 20.1 Anwendungs-Sicherheit (OWASP)

- **OWASP ASVS Level 2** ist die verbindliche Security-Baseline für alle
  Web-/API-Funktionalitäten (Level 2 = Standard für Geschäftsanwendungen mit
  sensiblen Daten — passt zur Zielgruppe).
- **OWASP Top 10** (aktuelle Fassung) dient als Minimum-Checkliste in jedem
  Security-Review; bekannte Produkt-Risikoflächen sind explizit abgedeckt:
  E-Mail-HTML-Darstellung (Sanitizing, 4.2), öffentliche Endpunkte (8.1),
  Anhänge (Scan, 4.2), Session-/Token-Handhabung (8.1/8.2), HMAC-Webhooks (8.3).
- Security-Header, Transport-Verschlüsselung (HTTPS), sichere
  Cookie-/Session-Attribute **sowie CSRF-Schutz und sichere Ausgabekodierung**
  (auch für selbst erzeugte Markdown-Inhalte wie Templates/Drafts, nicht nur
  empfangene E-Mail-HTML) sind Bestandteil der Auslieferung (8.8).

### 20.2 Software-Qualitätsmodell (ISO/IEC 25010)

Die Qualitätsmerkmale werden fachlich wie folgt abgedeckt:
- **Funktionalität:** Akzeptanzkriterien je Modul (§15), Traceability (§10).
- **Zuverlässigkeit:** Fehlerlisten/Retry (4.2), Restore-Drill (8.7),
  RTO/RPO (8.7).
- **Benutzbarkeit:** Empty-States (16.4), i18n/Dark Mode (9.2), Barrierefreiheit
  (9.2/BFSG).
- **Effizienz:** Performance-Ziele + Kapazitätsannahmen (9.1).
- **Sicherheit:** §20.1 + Guardrails + Audit.
- **Kompatibilität/Übertragbarkeit:** Import/Export (6.2/6.3), API-Verträge
  (8.2/8.3).
- **Wartbarkeit:** Entwicklungsstrategie (21), Glossar (18), Phasen (17).

### 20.3 Informationssicherheit & Löschkonzept

- **DIN EN ISO/IEC 27555** (Löschen personenbezogener Daten) — Grundlage der
  Löschklassen (9.3); bereits vollständig umgesetzt.
- **ISO/IEC 27001 (Prinzipien, ohne Zertifizierung):** Zugriffsbeschränkung
  (Rollenmodell 2.2), Least Privilege bei Tokens (8.2), Backup/Restore (8.7),
  Vorfall-Nachvollziehbarkeit (Audit 8.4), sichere Entwicklung (21.6). Eine
  Zertifizierung ist für die Zielgröße nicht vorgesehen — die Praktiken gelten
  trotzdem als Arbeitsstandard.

### 20.4 KI-Governance (ISO/IEC 42001, Prinzipien)

Anlehnung an das KI-Managementsystem-Modell ohne Zertifizierung:
- **Verantwortlichkeiten:** Admin = KI-Verantwortlicher der Organisation
  (Konfiguration, Eignung, Kosten — 8.9/8.10); System = assistierend, nie
  autonom entscheidend (HITL, 9.4).
- **Folgenabschätzung (fachlich):** Die Datenkategorien-Deklaration (8.10)
  dokumentiert je KI-Funktion, welche Daten verarbeitet werden.
- **Überwachung:** KI-Observability (8.6) + KI-Audit (8.4) + Guardrail-
  Protokolle.
- **Beschwerde-/Korrekturweg:** Fakten sind korrigierbar (manuelle Korrektur
  gewinnt, 5.2); Agent-Entscheidungen sind im Event-Log nachvollziehbar (5.7).

### 20.5 Barrierefreiheit (EN 301 549 / WCAG 2.1 AA)

Verbindliches Ziel in 9.2 (BFSG-Umsetzung, siehe 19.5).

---

## 21. Entwicklungsstrategie

Wie die Applikation entwickelt, geprüft und ausgeliefert wird. Gilt für alle
Phasen (§17).

### 21.1 Entwicklungsmodell

- **Spec-first:** Diese Fachspezifikation ist die einzige fachliche Quelle
  („Single Source of Truth"). Vor jeder Implementierung eines Moduls existiert
  ein Implementierungsplan, der auf dieser Spec basiert; Abweichungen von der
  Spec erfordern eine Spec-Änderung (Review-Runde) — nie eine stille
  Code-Entscheidung.
- **Iterativ entlang §17:** Eine Phase nach der anderen; innerhalb einer Phase
  ein Feature-Block nach dem anderen — keine parallelen halbfertigen Baustellen.
- **Einfachheit vor Flexibilität:** YAGNI ist Entscheidungsprinzip — jede
  zusätzliche Abstraktion muss sich gegen „einfachste Implementierung" (1.3)
  rechtfertigen.

### 21.2 Definition of Done (je Änderung)

Eine Änderung gilt nur als fertig, wenn **alle** Quality-Gates grün sind:
1. Unit-Tests (Logik) und Integrations-Tests (Datenfluss) geschrieben und grün —
   Tests vor Code (TDD).
2. E2E-Tests der betroffenen Akzeptanzkriterien (§15) grün — je Rolle
   (Admin/Benutzer), mit echten Datenoperationen (nicht nur Statuscodes).
3. Typprüfung und Linting ohne Fehler.
4. Docker-Stack baut und startet; Health-Checks grün.
5. Security-Check: keine Secrets im Code, OWASP-Top-10-Blick auf die Änderung,
   Dependency-Scan ohne kritische Befunde (21.5).
6. Lizenz-Scan ohne neue unzulässige Lizenzen (22.3).
7. Dokumentation: Spec-Referenz im Plan aktualisiert, Changelog-Eintrag.

### 21.3 Teststrategie

- **Testpyramide:** viele Unit-Tests (fachliche Regeln: Statusmaschinen,
  Schwellen, Kaskaden), weniger Integrations-Tests (Modul-Grenzen, API),
  gezielte E2E-Tests (Benutzerpfade je Rolle).
- **Negativ-Pfade sind Pflicht:** je Modul mindestens die definierten
  Ablehnungsfälle (Lösch-Ablehnung, Geofencing-Verweigerung, Guardrail-Block,
  Validierungsfehler) — nicht nur Happy Paths.
- **Barrierefreiheit:** je Phase automatisierte Accessibility-Prüfung
  (WCAG 2.1 AA, 9.2) + manueller Tastatur-Rundgang.
- **Performance:** Stichproben gegen die Ziele aus 9.1 innerhalb der
  Kapazitätsannahmen (Suche, Karte, Listen, E-Mail-Verarbeitung).
- **Regressionssuite:** bleibt über Upgrades dauerhaft grün (8.8).

### 21.4 Release- & Upgrade-Strategie

- **Versionierung:** Semantische Versionierung (Major/Minor/Patch); jede
  Auslieferung trägt die Version sichtbar im System (Admin-Bereich).
- **Migrationen:** Daten-Migrationen laufen automatisch beim Upgrade, immer mit
  Pre-Migration-Backup (8.7) und dokumentiertem Rollback-Weg; Breaking Changes
  werden im Changelog angekündigt und dürfen nie still passieren.
- **Upgrade-Pfad:** Upgrades erfolgen Version-für-Version (kein Überspringen
  von Major-Releases ohne Migrationsprüfung); der Restore-Drill (8.7) wird je
  Major-Release wiederholt.
- **Changelog:** je Release fachlich verständlich (DE), mit Verweis auf die
  betroffenen Spec-Abschnitte.

### 21.5 Abhängigkeiten & Updates

- **Abhängigkeits-Hygiene:** so wenige Abhängigkeiten wie möglich; jede neue
  Abhängigkeit wird vor Aufnahme geprüft (Lizenz 22.2, Wartungsstatus,
  Security-Historie, Größe).
- **Update-Fenster:** monatliches Update-Fenster für Minor-/Patch-Updates aller
  Abhängigkeiten; **kritische Security-Updates sofort** (außerhalb des
  Fensters).
- **Scans:** Abhängigkeits-Scans (Ökosystem-Werkzeuge, z. B. `pnpm audit` /
  `pip-audit` oder gleichwertig) laufen in der CI und blockieren bei
  kritischen Befunden.
- **Keine unmaintainten Abhängigkeiten:** Abhängigkeiten ohne aktive Wartung
  werden ersetzt oder entfernt.
- **Rechtsbeobachtung:** Änderungen an Durchführungsleitlinien zum AI Act (19.1)
  und an den Standards aus §20 werden je Release-Fenster auf Relevanz geprüft.

### 21.6 Sicherheit im Entwicklungsprozess

- **Keine Secrets im Repository** (keine „change-me"-Fallbacks in Produktion —
  Konfiguration ausschließlich über Umgebungs-/Deployment-Konfiguration).
- **Vier-Augen-Review** vor jeder Übernahme in den Hauptzweig (Code-Review mit
  Skepsis-Prinzip: Diff prüfen, Tests laufen lassen, Seiteneffekte kontrollieren).
- **Log-Hygiene** auch in der Entwicklung: keine PII, keine Credentials, keine
  Roh-Prompts in Logs (9.3).
- **Schwachstellen-Mgmt.:** gemeldete/bekannte Schwachstellen werden priorisiert
  behoben (kritisch = sofort, hoch = nächstes Release); die Behebung erhält
  einen Regressionstest.

### 21.7 Dokumentation & Betriebshandbuch

- Spec (§-Referenzen), Implementierungspläne, Changelog und ein
  Betriebshandbuch (Installation, Backup/Restore, Upgrade, Restore-Drill,
  Notfall-Wiederherstellung 8.1) sind Teil jedes Releases.
- Fortschritt und Review-Stände werden pro Phase protokolliert (Plan- und
  Review-Dokumente bleiben im Repository nachvollziehbar).

---

## 22. Lizenz-Compliance

Gewährleistung, dass keine Open-Source- oder sonstigen Lizenzen verletzt werden
— über den gesamten Lebenszyklus.

### 22.1 Grundsätze

- **Kein fremdes proprietäres Material:** Es wird kein Code, keine Assets und
  keine Inhalte aus proprietären/quellrechtlich ungeklärten Quellen übernommen
  (kein Copy-Paste aus fremden Projekten unbekannter Herkunft —
  Clean-Room-Prinzip).
- **Open Source nur konform:** Jede Open-Source-Abhängigkeit wird nur gemäß
  ihrer Lizenzbedingungen genutzt; Lizenzpflichten (Attribution, Lizenztext-
  Beigabe) werden erfüllt.
- **Eigene Lizenzierung:** Die Entscheidung über die Lizenzierung von v3 selbst
  (Eigenlizenz/Proprietär vs. Open Source) ist eine Geschäftsentscheidung
  außerhalb dieser Spec — sie berührt die Compliance-Pflichten hier nicht.

### 22.2 Lizenz-Policy (verbindlich)

| Kategorie | Lizenzen | Regel |
|-----------|----------|-------|
| **Erlaubt** | MIT, Apache-2.0, BSD-2-Clause, BSD-3-Clause, ISC, CC0 (Assets), Unlicense | Aufnahme ohne weitere Prüfung der Lizenz (Policy-/Security-Prüfung bleibt) |
| **Bedingt** | LGPL, MPL, EUPL | Nur nach Einzelfall-Prüfung und dokumentierter Freigabe (Nutzungsart muss Copyleft-Pflichten erfüllen, z. B. dynamische Verlinkung/Offenlegung) |
| **Unzulässig** | AGPL, SSPL, BUSL, Commons-Clause | Keine Aufnahme — Netzwerk-Copyleft-/Nutzungsbeschränkungen sind mit dem Produktmodell unvereinbar |
| **GPL** | GPL (alle Varianten) | Grundsätzlich unzulässig; Ausnahme nur durch dokumentierte juristische Einzelfall-Prüfung |

### 22.3 Prozess & Durchsetzung

- **Lizenz-Scan in der CI:** Alle Abhängigkeiten werden automatisiert gegen die
  Policy (22.2) geprüft; neue unzulässige Lizenzen **blockieren** den Build
  (Quality-Gate, 21.2 Punkt 6).
- **Attribution:** Ein Verzeichnis aller Drittanbieter-Komponenten mit Lizenz
  (`THIRD-PARTY-LICENSES`) wird mit jedem Release ausgeliefert.
- **Prüfpunkte:** Bei Aufnahme jeder neuen Abhängigkeit, im monatlichen
  Update-Fenster (21.5) und quartalsweise als vollständiges Lizenz-Inventar.
- **KI-Modelle & Assets:** Auch Modell-Gewichte (lokale Inferenz, 8.9) und
  nicht-code Assets (Schriftarten, Icons, Kartenmaterial) unterliegen der
  Policy; **OpenStreetMap-Kacheln erfordern die ODbL-Attribution**
  („© OpenStreetMap contributors") sichtbar in der Kartenansicht (7.4);
  Modell-Lizenzen werden vor Aktivierung geprüft und in den KI-Einstellungen
  dokumentiert. **Dev-Abhängigkeiten:** Die Policy gilt primär für Runtime- und
  Auslieferungs-Abhängigkeiten; reine Entwicklungs-Werkzeuge (Tests, Linter,
  Build) dürfen auch Lizenzen der Kategorie „Bedingt" nutzen, wenn sie nicht
  ausgeliefert werden (Ausweisung im Lizenz-Inventar bleibt Pflicht).

### 22.4 Abgrenzung

- **Rechtsberatung:** Dieses Kapitel ist eine Arbeits-Policy, keine
  Rechtsberatung; Einzelfälle der Kategorien „Bedingt"/„GPL" werden extern
  juristisch geprüft, bevor eine Freigabe dokumentiert wird.

---

## Selbst-Review-Protokoll

*(wird nach jeder Review-Runde aktualisiert)*

### Runde 0 — Erstfassung
- Platzhalter-Scan: keine TBD/TODO verblieben.
- Interne Konsistenz: Deal-Modell (3.3) ↔ Setter/Closer (7.6) ↔ Geofencing (7.5)
  aufeinander abgestimmt; Produkte-Ausschluss konsequent aus Deals entfernt.
- Scope: Fachspec ohne Technik — Tech-Entscheidungen explizit nach §11 vertagt.
- Ambiguitäten: „Konnektor-Gewicht konfigurierbar" (7.1) bewusst als Regel statt
  fester Zahl formuliert.

### Runde 1 — Review-auf-Review (gegen echten MVP-Code geprüft)

**Korrekturen (Übertreibungen/Fehler behoben):**
- §5.1 Tool-Tabelle komplett neu geschrieben: Erstfassung enthielt 3 erfundene
  Tools (`identify_contact`, `write_brief`, `research_entity` — nur in AP-6 als
  Zukunftsidee spezifiziert, nie implementiert) und falsche Freigabe-Zuordnung.
  Verifiziert gegen `packages/api-workers/src/agent/tools.py`: exakt 11 Tools,
  7 automatisch (inkl. `schedule_recheck`), 4 mit Proposal.
- §5.2/§6.5: „Form-Intake erzeugt Fakt mit Gewicht 0,65" entfernt — im MVP-Code
  nicht implementiert (`packages/web/app/api/t/intake/route.ts` erzeugt nur
  Kontakte mit Quelle `web_intake`). YAGNI-Entscheidung in §10.3 dokumentiert.

**Ergänzungen (übersehene Punkte):**
- §4.2 Inbox-Ansicht ergänzt (fehlte als eigener Absatz).
- §5.4 Anreicherungs-Status ergänzt (aus AP-12).
- §10.1 AP-8/AP-12-Traceability ergänzt; §10.3 um Suchumfang-, Tool- und
  Intake-Fakt-Entscheidungen erweitert.

**Verifizierte Kernaussagen (am Code bestätigt):**
- Evidence-Gewichte 0,90/0,80/0,75/0,70/0,40/0,20 und Schwellen 0,70/0,30 —
  exakt wie in `packages/shared/src/shared/evidence/weights.py`.
- Deal-Status `open/won/lost/partial` — `packages/api-workers/src/routers/bulk.py`.
- Fakt-Status `auto/suggested/accepted/rejected`, Proposal-Status
  `pending/accepted/rejected/expired` — `packages/web/lib/db/schema.ts`.
- Form-Intake-Status `ignored/matched/contact_created` — Intake-Route.
- `visit_status` + `energy_data` am Kontakt — Schema.
- Event-Arten `call/meeting/email_reminder/deadline` (+ Todo-Merge) —
  Kalender-Komponenten.

**Verbleibende Restunsicherheit:** Keine Tech-Aussagen enthalten (bewusst);
alle fachlichen Regeln mit MVP-Referenz geprüft.

### Runde 2 — Vollständigkeits-Ergänzung (6 identifizierte Lücken geschlossen)

Auf User-Nachfrage („sind die Specs voll ausformuliert?") systematisch auf
Lücken geprüft und ergänzt:

1. **Akzeptanzkriterien je Modul** → neuer §15 (6 Modulblöcke, je 6–10 konkrete
   E2E-prüfbare Kriterien); §9.5 verweist jetzt darauf.
2. **Navigation/UI-Struktur** → neuer §13 (Shell, Hauptnavigation, Admin-Bereich,
   Einstellungen je Rolle).
3. **Benachrichtigungen** → neuer §14 (In-App-Zentrum, 8 Auslöser-Ereignisse mit
   Empfänger, Deep-Link- und Auto-Gelesen-Regeln); in §1.5 als Kernfähigkeit
   aufgenommen.
4. **Löschklassen konkret** → §9.3 um 7 Löschklassen (LK-1…LK-7) mit Fristen,
   Entitätszuordnung und Regeln (Löschbegehren, Pseudonymisierung vor Löschung)
   erweitert.
5. **E-Mail-Versand** → §4.1 um SMTP/Graph-Versand, Testversand,
   Gesendet-Ordner-Verknüpfung und Versandfehler-Handling erweitert; §1.5
   angepasst.
6. **Konsolidiertes Entitätsmodell** → neuer §12 (34 Entitäten in 5 Domänen mit
   Kerninhalt und Beziehungen).

**Konsistenzprüfung Runde 2:** Neue Abschnitte referenzieren bestehende
Paragraphen (3.x–8.x) bidirektional; keine neuen Tech-Festlegungen eingeführt;
Löschklassen widersprechen nicht den Einzelregeln (Import-Staging 30 Tage,
Roh-HTML 90 Tage, Tracking 12 Monate) — diese sind jetzt die konkreten Instanzen
der Klassen.

### Runde 3 — Logisches Datenmodell ausformuliert + letzte Fach-Lücken

Auf User-Nachfrage („ist wirklich alles fachliche drin, auch das logische
Datenmodell?") §12 von einer Stichwort-Übersicht zu einem **vollständigen
logischen Datenmodell** ausgebaut und die Spec erneut gegen die MVP-Specs
abgeglichen:

1. **§12 Logisches Datenmodell:** 42 Entitäten in 5 Domänen, jede mit
   vollständigen Attributen (fachliche Typen, Pflichtfelder *, Aufzählungs-Werte),
   Kardinalitäten und Beziehungen. Neu gegenüber Runde 2 als eigenständige
   Entitäten: Label-Definition (4.3), Workflow-Lauf-Schritt (6.4), Custom-Field-Wert
   (3.9), Tracking-Domain/-Event (6.5), Fraud-Log-Eintrag (7.5),
   Kalenderverbindung (3.6), KI-Einstellungen (8.9).
2. **KI-Konfiguration** → neuer §8.9 (Anbieter-Typ, Modell je KI-Funktion,
   Verbindungstest, Kostenlimit-Verhalten) — war im MVP über inference-Config +
   Admin-UI geregelt und fehlte hier bisher als fachliche Regel.
3. **Demo-Daten** → §2.3 (optional im Setup-Assistenten, markiert, entfernbar) —
   aus MVP-Spec `demo-user-data`.
4. **Reporting-Klarstellung** → §6.3 umbenannt in „Export & Reporting":
   Auswertungen = Dashboard-Kennzahlen + Export-Jobs, kein separates
   Berichtswesen-Modul.
5. **Konsistenz:** Agent-Event-Typen in §5.7 um `recheck_scheduled` ergänzt
   (entspricht MVP-Code und §12).

**Verbleibende bekannte Abgrenzung:** Physisches Schema, Normalisierung und
Speicherprodukt bleiben bewusst vertagt (§11) — das logische Modell ist davon
unabhängig vollständig.

### Runde 4 — Systematische Vollständigkeitsprüfung (10 letzte Lücken)

Auf erneute User-Nachfrage („ist wirklich alles durchspezifiziert?")
systematische Prüfung aller Module auf fehlende Verhaltensregeln — teilweise
gegen MVP-Code verifiziert (kein Passwort-Reset, kein CC/BCC, keine
Soft-Deletes, Währungsfeld aus AP-9 vorhanden):

1. **Löschregeln & Kaskaden** → neuer §16.1 (12 Objekttypen mit explizitem
   Löschverhalten; Hart-Löschung, Audit bleibt, Pseudonymisierung).
2. **Passwort vergessen / Admin-Lockout** → §8.1 (Admin-Reset mit
   Änderungs- + 2FA-Neueinrichtungs-Zwang; Notfall-Wiederherstellung für den
   einzigen Admin, protokolliert).
3. **E-Mail-Details** → §4.2 (Anhänge max. 25 MB), §4.5 (ein Empfänger, kein
   CC/BCC — YAGNI, in 10.3 dokumentiert), §4.5 (kein Löschen/Archivieren im
   CRM — Aufbewahrung nach LK-5).
4. **Währung** → §3.3 (EUR fix) + §16.5 + 10.3.
5. **Zeitzone** → §16.2 (Installations-Zeitzone, Default Europe/Berlin, keine
   Pro-Termin-TZ).
6. **Backup-Rhythmus** → §8.7 (täglich Default 03:30 + manuell +
   Pre-Migration-Backup; Aufbewahrung nach LK-7).
7. **Workflow-Start** → §6.4 (manuell oder Vorschlag bei `won`; erster Schritt
   immer mit Bestätigung — HITL-Konsistenz).
8. **Agent-Chat-Verlauf** → §5.1 (Persistenz über Agent-Event-Log, kein
   separates Konversations-Objekt; Chat je Entität + global).
9. **Empty States & Onboarding** → §16.4 (8 Ansichten mit konkreten
   Empty-States und Handlungsangeboten).
10. **NFR-Ergänzungen** → §9.1 (funktionale Performance-Ziele: Suche < 1 s bei
    50.000 Datensätzen, Karte 1.000+ Pins, Listen-Paginierung, E-Mail < 60 s),
    §9.2 (Barrierefreiheit-Basis), §8.2 (API-Umfang = UI-Fähigkeiten, keine
    API-Exklusivfunktionen), §16.3 (Parallel-Edit: letztes Speichern gewinnt).

**Konsistenzprüfung Runde 4:** §15.7 (Akzeptanzkriterien für §16) ergänzt;
Traceability 10.1 (Demo-Data) und 10.3 (6 neue Neuentscheidungen) aktualisiert;
Löschkaskaden widersprechen weder Löschkonzept (9.3) noch Datenmodell (§12) —
Referenz-Regeln dort jeweils als „optional" bzw. „bleibt bestehen" kompatibel.

**Gesamtstand:** Alle fachlichen Regeln, Zustände, Abläufe, Kantenfälle,
Löschverhalten, Formate und NFR sind ausformuliert. Bewusst offen bleibt nur die
physische/technische Ebene (§11).

### Runde 5 — Pre-Mortem-Fixes (blockierende Befunde des adversarialen Reviews)

Quelle: `docs/superpowers/reviews/2026-08-22-fachspec-v3-premortem-review.md`
(42 Befunde). Alle **Muss-Befunde** wurden eingearbeitet:

1. **S1-1** Guardrail-/Extraktions-Widerspruch → §8.5 um **Guardrail-Policy je
   KI-Funktion** erweitert (PII-Maskierung für Extraktionsfunktionen aus, für
   andere an; Admin-Override audit-pflichtig) + Policy-Tabelle.
2. **S1-2** Matching-Regel „Neuanlage bei bekannter Domain" → §4.2 korrigiert:
   Firmenneuanlage nur bei **unbekannter** Domain, als HITL-Vorschlag.
3. **S1-3** `record_fact`-Doppel-HITL → §5.2/§5.3: Proposal-Approval → Fakt
   `accepted` unabhängig vom Gewicht; Schwellen gelten nur für automatische
   Quellen.
4. **S3-5** Setter/Closer-Pipeline ohne Datenfundament → **Vertriebsstufe** als
   Attribut am Kontakt (§12.1) mit 5 Werten, Stufenwechsel-Regeln und
   Logbuch-Eintrag (§7.6); Dashboard-Berechnung darauf gegründet.
5. **S3-2** Gelesen/Ungelesen → Attribut an E-Mail (§12.3) + Regeln in §4.2
   (Öffnen = gelesen, manuell umschaltbar, Dashboard zählt ungelesene).
6. **S4-1** LLM/Datenschutz → neuer **§8.10** (Datenkategorien-Deklaration je
   KI-Funktion, AVV/EU-Eignung als Aktivierungsvoraussetzung, lokale Inferenz
   als Alternative) + §9.3 ergänzt + §12.5 KI-Einstellungen erweitert.
7. **S2-1** Ähnlichkeitsfunktion → zentrale Definition in §4.2 (normalisierte
   Levenshtein-Ähnlichkeit, Kleinschreibung, Umlaut-Auflösung); §5.4 verweist.
8. **S2-2** Wissenslücken → §5.4: Soll-Profil (4 Wissensfelder) + Feld-Fristen
   (News 30 Tage, Basis 90 Tage).
9. **S2-3** Antwort-Prüfung → §6.4: Antwort = E-Mail des Kontakts oder der
   Firmen-Domain nach `prepared`, ohne Auto-Replies; fortlaufende Evaluation;
   Ersatzaktion nur nach vollständig verstrichener Wartezeit.

**Akzeptanzkriterien nachgezogen:** §15.2 (Gelesen-Status, Neuanlage-Vorschlag),
§15.3 (record_fact → accepted, automatische Quellen nach Gewicht), §15.5
(Vertriebsstufen-Wechsel + Conversion), §15.6 (Guardrail-Policy,
AVV-Aktivierungssperre).

**Nicht eingearbeitet (laut Review „Sollte/Kann"):** S3-1 (E-Mail-Suche),
S3-3/S3-4 (KB-UI, Fehlerlisten-UI), S3-6…S3-9, S4-2…S4-6, S5-x, S6-x — bleiben
als offene Befunde im Review-Dokument stehen und sind Kandidaten für die
nächste Spec-Revision oder die Implementierungsplanung.

### Runde 6 — Sollte-Liste + MVP-Lücken vollständig eingearbeitet

Auf User-Anweisung („arbeite alles ein, auch das was im MVP fehlt") die
gesamte Sollte-Liste des Pre-Mortem-Reviews plus die MVP-Gap-Entscheidungen
vervollständigt:

1. **S3-1** E-Mail-Volltextsuche → §3.7 (Betreff + Body, Treffer öffnet Inbox).
2. **S3-3** KB-UI → §13.2 eigener Navigationsbereich „Wissensbasis".
3. **S3-4** Fehlerlisten-UI → §13.3 „Verarbeitungsfehler" im Admin-Bereich +
   §14 Benachrichtigung an Admin.
4. **S3-9** Unbekannter Empfänger → §4.5 + §12.3 (freie E-Mail-Adresse als
   Draft-Empfänger, optionale Inline-Kontakt-Anlage).
5. **S5-2** MVP-Migration → neuer §10.4: bewusst kein Auto-Migrations-Tool;
   offizieller Weg = CSV/XLSX-Export → Import-Wizard (6.2).
6. **S5-4** Verträge → §8.2 verbindlicher Scope-Katalog (12 Module,
   Admin-Bereiche token-gesperrt) + §8.3 verbindliches Webhook-Payload-Schema
   (fester Umschlag + `data` nach §12, abwärtskompatibel).
7. **S5-5** → §2.3: letzter aktiver Admin nicht deaktivier-/herabstufbar;
   Rollenwechsel mit Bestätigung + Audit + Benachrichtigung.
8. **S4-2** Consent-Nachweis → §6.5 + §12.4: Consent-Version (Hash des
   akzeptierten Textes) + versionierte Consent-Texte.
9. **S4-3** Anhang-Scan → §4.2: Malware-Scan vor Ablage, blockierte Anhänge
   gesperrt.
10. **S4-4** HTML-Sanitizing → §4.2: bereinigte Darstellung, kein aktiver
    Inhalt, keine externen Nachladungen ohne Aktion.
11. **S4-5** Sitzungs-Invalidierung → §8.1: Passwort-/2FA-Änderung invalidiert
    alle Sitzungen.
12. **S4-6** (im Review durch die Raster gefallen) Kalender-Privatsphäre →
    §3.6 + §2.1: externe Termine nur für den verbindenden Benutzer sichtbar,
    dokumentierte Ausnahme vom Shared-Workspace-Prinzip.

**Zusätzliche MVP-Lücken-Bereinigung (vom User explizit benannt):**
- **Passwort-Reset:** seit Runde 4 in §8.1 (Admin-Reset + Notfall-Wiederherstellung),
  jetzt um Sitzungs-Invalidierung vervollständigt.
- **CC/BCC:** bewusste Entscheidung „ein Empfänger, kein CC/BCC" in §4.5 + §10.3 —
  MVP-Zustand als Design-Entscheidung übernommen (YAGNI).
- **Soft-Deletes:** bewusste Entscheidung Hart-Löschung mit Kaskaden in §16.1,
  jetzt zusätzlich als Neuentscheidung in §10.3 dokumentiert.
- **Währungsfeld (AP-9):** EUR-fix-Entscheidung in §3.3/§16.5/§10.3 (Multi-Währung
  ausgeschlossen).
- **Tonalitäts-„Gruppe"** (S2-5, MVP-Altlast eines undefinierten Konzepts):
  Scope entfernt → nur global + pro Kunde, mit Historie-Fallback (§4.5, §12.3,
  §10.3).

**Akzeptanzkriterien nachgezogen:** §15.1 (E-Mail-Suche, Löschkaskaden),
§15.2 (unbekannter Empfänger, Anhang-Sperre, HTML-Bereinigung), §15.6
(Sitzungs-Invalidierung, letzter-Admin-Schutz, Rollenwechsel, Scope-Durchsetzung,
Payload-Umschlag, Consent-Version).

**Verbleibend offen (Review „Kann"):** S2-4…S2-14 Restpunkte (u. a.
`partial`-Semantik, Lead-Score, Geofencing-Gültigkeitsdauer, Konnektor-
Erweiterungsmechanismus), S3-6…S3-8, S5-1/S5-3/S5-6, S6-1…S6-4 — dokumentiert
im Review-Dokument, Kandidaten für Implementierungsplanung.

### Runde 7 — Produktentscheidung: 2FA optional statt Pflicht

Auf User-Anweisung wurde 2FA von einer Pflicht auf eine optionale Funktion
umgestellt. Geänderte Stellen:

1. **§8.1:** „TOTP-Pflicht für alle Benutzer" → **optional**, frei aktivier-/
   deaktivierbar je Benutzer, empfohlen aber nicht erzwungen; kein Setup-Zwang,
   keine Grace-Period mehr. Deaktivierung weiterhin nur mit gültigem TOTP
   (Schutz vor unbemerktem Entfernen).
2. **§8.1 Öffentlich-Seite:** „nur Login, Einladung und 2FA-Setup" → „nur Login
   und Einladungsannahme" (2FA-Setup liegt jetzt in den Benutzereinstellungen).
3. **§8.1 Sitzungs-Invalidierung:** statt „2FA-Neueinrichtung" jetzt „2FA-
   Aktivierung oder -Deaktivierung" (deckt das optionale Modell ab).
4. **§12.1 Benutzer:** Attribut „Grace-Period-Ende" entfernt; „2FA aktiv" mit
   Default nein gekennzeichnet.
5. **§15.6:** Grace-Period-Kriterium entfernt; 2FA-Kriterien auf das optionale
   Modell umgeschrieben (Login ohne 2FA weiterhin nur mit Passwort möglich).
6. **§10.3:** Neue Neuentscheidungs-Zeile „2FA: Pflicht → optional" mit
   Begründung (Onboarding-Reibung für Freelancer-Zielgruppe); veraltete
   „Grace-Period"-Zeile entfernt.
7. **§10.1:** Traceability-Zeile „2FA-Security, Auth-Gate/OTP-Grace" um Hinweis
   ergänzt, dass Pflicht/Grace-Period abgelöst sind.
8. **§1.4:** SSO-Ausschluss-Begründung auf „Passwort + optionale 2FA" angepasst.

**Konsistenzprüfung:** Grep über „Grace", „TOTP-Pflicht", „Setup-Zwang" zeigt
keine verbliebenen Pflicht-Referenzen (nur noch „Graceful Degradation" als
unabhängiges Konzept und die Protokoll-/Traceability-Einträge).

### Runde 8 — Blockierende Befunde aus Pre-Mortem-Review Runde 2 eingearbeitet

Quelle: `docs/superpowers/reviews/2026-08-22-fachspec-v3-premortem-review-runde2.md`
(K1-1…K1-3, K2-1, K2-2, K3-1):

1. **K1-1** Öffentliche Endpunkte → §8.1: abschließende Liste (Login,
   Einladungsannahme, Tracking, Form-Intake) + Sicherheitsregel (keine
   authentifizierten Daten, Rate-Limit, Bot-Filter, Honeypot). Widerspruch zu
   §6.5 aufgelöst.
2. **K1-2** Firmen-Löschung vs. E-Mails → §16.1: Firmen mit zugeordneten
   E-Mails sind nicht löschbar (konsistent mit Kontakte/Deals-Regel).
   Zusätzlich Kontaktkaskade um Fakten/Proposals/Agent-Events vervollständigt.
3. **K1-3** Workflow-Draft-Konto → §4.1 Konzept **primäres E-Mail-Konto**
   (genau ein aktives Konto), §6.4 Versandkonto-Regel (Workflow-Konto sonst
   primär, ohne primäres Konto blockiert), §12.3/§12.4 Attribute ergänzt.
4. **K2-1** Geofencing → §7.5 neu: gilt nur für Abschlussart `d2d` (neues
   Deal-Feld, §3.3/§12.2, Default `buero`); 2h-Gültigkeitsfenster pro
   Kontakt-Check-In; Admin-Override mit Pflicht-Begründung + Audit +
   Fraud-Log-Markierung; Büro-Deals jederzeit ohne Check-In.
5. **K2-2** `partial` → gestrichen (§3.3, §12.2): nur noch `open`/`won`/`lost`;
   Entscheidung in §10.3 dokumentiert.
6. **K3-1** Löschbegehren-Mechanik → §9.3: 4-Schritt-Mechanik (Löschen nach
   LK-6 / Pseudonymisieren in LK-5-E-Mails via Platzhalter ohne Mapping /
   Audit+Deals unberührt / Nachweis-Audit-Eintrag); korrespondierender
   Auskunft-Umfang definiert und in §6.3 verlinkt.

**Akzeptanzkriterien nachgezogen:** §15.1 (Firmen-Löschung mit E-Mails),
§15.4 (Antwort-Prüfung-Details, Workflow-Versandkonto), §15.5 (Geofencing
D2D/Büro/Fenster/Override), §15.6 (Löschbegehren-Mechanik, Auskunft-Umfang).

**Traceability:** §10.3 um 3 Neuentscheidungen erweitert (partial gestrichen,
Geofencing D2D-Begrenzung, Löschbegehren-Mechanik).

**Verbleibend offen (Review Runde 2, nicht blockierend):** K1-4 (KB-Suche-
Policy), K2-3…K2-5 (Konnektor-Mechanismus, Research-Schalter, Benachrichtigungs-
Aggregation), K3-2/K3-3 (Auskunft jetzt definiert — K3-2 damit erledigt;
K3-3 Eignungs-Dokumentation offen), K4-1…K4-6, K5-1…K5-3.

### Runde 9 — Alle verbleibenden offenen Themen ausgearbeitet (Brainstorming-Abschluss)

Auf User-Anweisung („arbeite alle offenen Themen mit Brainstorming aus") wurden
sämtliche noch offenen Befunde (Review Runde 2 „offen" + Altlasten aus Runde 1)
durchdacht und als Entscheidungen eingearbeitet:

**Aus Review Runde 2:**
1. **K1-4** KB-Suche → §8.5: Maskierung „aus" + neuer Grundsatz (Maskierung
   gilt für Verarbeitungsmaterial, nicht für Such-Queries; Restrisiko über
   Anbieter-Eignung 8.10 gedeckt).
2. **K2-3** Konnektor-Mechanismus → §7.1: Typen sind implementierte Adapter,
   Admin wählt Typ + Credentials; konfigurierbare Mappings bewusst ausgeschlossen
   (YAGNI).
3. **K2-4** Research-Schalter → §5.4: globaler Master-Schalter in den
   KI-Einstellungen (8.9) + täglich-Flag pro Firma + manueller Button.
4. **K2-5** Benachrichtigungs-Aggregation → §14: zwei Klassen (Sofort einzeln /
   Todos als Tages-Digest 08:00 mit Zähler).
5. **K3-3** Eignungs-Dokumentation → §8.10: ehrlich als „dokumentierte
   Admin-Erklärung" benannt, mit Pflicht-Eignungs-Referenz (AVV-Dokument/Link)
   + Bestätigungs-Dialog; §12.5 Attribut ergänzt.
6. **K4-1** Dokumenten-Upload → §6.1: 25 MB, erlaubte Typen (PDF/Office/Bilder/
   Text/E-Mail-Formate).
7. **K4-2** Audio-Upload → §5.6: mp3/m4a/wav/ogg, max. 60 Minuten, max. 200 MB.
8. **K4-3** Custom-Field-Import → §6.2: Custom-Field-Werte importierbar
   (Mapping auf Schlüssel, Validierung gemäß Definition) — schließt den
   Widerspruch zum Migrationsweg §10.4.
9. **K4-4** Prioritäts-Mapping → §4.4: urgente/high/medium/low → Dringend/Hoch/
   Mittel/Niedrig.
10. **K4-5** `changed_fields` → §8.3: Top-Level-Feld (Geschwister von `data`).
11. **K4-6** Custom-Field `user` → §3.9: deaktivierte Benutzer bleiben mit
    Marker angezeigt, Auswahllisten zeigen nur aktive Benutzer.
12. **K5-1** Phasengrenzen → neuer **§17 Implementierungsphasen** (5 Phasen mit
    Exit-Kriterien; Umfang unverändert, nur Reihenfolge).
13. **K5-2** Glossar → neuer **§18** (30 Begriffe mit Definition + Referenz).
14. **K5-3** Reklassifizierung → mit dieser Runde gegenstandslos: alle Befunde
    sind eingearbeitet, keine offenen „Kann"-Punkte mehr.

**Altlasten aus Review Runde 1 (ebenfalls geschlossen):**
15. **S2-6** Lead-Score → §4.3 Definition + Zweck (Priorisierungshilfe, keine
    Folgeautomatik).
16. **S2-7** Themenkategorien → §4.3 Admin-verwaltete Liste statt freier
    Klassifikation.
17. **S2-10** Tarifrechner-Ergebnisvertrag → §7.3 (weniger Tarife, kein CO₂,
    kein aktueller Tarif, API down — je eigener Anzeigefall).
18. **S2-11** Strukturelle LLM-Fehler → §4.2: Schema-Prüfung, 1 Retry, nach 3
    Fehlschlägen dauerhafter Fehler, alles-oder-nichts pro E-Mail.
19. **S2-12** Einladungs-Gültigkeit → §2.3/§12.1: 7 Tage.
20. **S2-14** Versionierung → §6.4: Snapshot der Workflow-Definition bei
    Lauf-Start; Template-Inhalt als Kopie bei `prepared`.
21. **S3-7** Offline-D2D → §7.6: lokale Zwischenspeicherung + Auto-Sync,
    Offline-Check-Ins.
22. **S3-8** Timeline → §3.4: Paginierung in 50er-Schritten.
23. **S5-1** RTO/RPO → §8.7: RPO 24 h, RTO 4 h, Drill misst gegen RTO.
24. **S5-3** Alert-Kanal → §8.6: zusätzlich konfigurierbare E-Mail an Admin für
    kritische Alerts.
25. **S5-6** Kapazitätsannahmen → §9.1: 10 Benutzer, 50.000 Kontakte/Firmen,
    200.000 E-Mails/Jahr, 10.000 Dokumente (~50 GB), 1.000 Tracking-Events/Tag,
    100 Transkripte/Monat.
26. **Karten-Sidepanel** → §7.4: Kontakt- oder Firmen-Panel je nach Pin-Typ.

**Abwägungs-Protokoll (Brainstorming-Kernentscheidungen dieser Runde):**
- Konnektoren: „konfigurierbare Mappings" verworfen — ein Mapping-Editor wäre
  ein eigenes Produkt; Adapter-Modell passt zur Einfachheits-Prämisse.
- Benachrichtigungen: Zuständigkeitsmodell („nur den zuständigen Benutzer")
  verworfen — es bräuchte Ownership (in 2.1 bewusst ausgeschlossen);
  Digest-Modell löst das Spam-Problem ohne Ownership.
- KB-Suche: Maskierung „an mit Ausnahmen" verworfen — Regelkomplexität ohne
  Mehrwert; Eignungs-Rahmen (8.10) deckt das Restrisiko.
- RPO schärfer als „tägliches Backup" abgelehnt (z. B. stündlich) —
  Ressourcen-Prämisse; manuelle Backups stehen als Option bereit.
- Lead-Score als berechnetes Modell verworfen — LLM-Schätzung im Triage-Verbund
  ist einfacher und konsistent mit der Kennzeichnungs-/Review-Logik.

**Traceability:** §10.3 um 4 Neuentscheidungen ergänzt (Lead-Score,
Themenkategorie, Timeline, Snapshot-Semantik).

**Gesamtstand:** Keine offenen fachlichen Punkte mehr. Die Spec definiert für
jedes Modul Zweck, Entitäten, Zustände, Regeln, Abläufe, Kantenfälle, Limits und
Akzeptanzkriterien; Phasen (§17) und Glossar (§18) machen sie
implementierungsplanbar. Bewusst vertagt bleibt ausschließlich die physische
Ebene (§11).

### Runde 10 — Rechtsrahmen (EU/DE), Qualitätsstandards, Entwicklungsstrategie, Lizenz-Compliance

Auf User-Anweisung online recherchiert (Quellen: artificialintelligenceact.eu
Implementation-Timeline, gesetze-im-internet.de UWG § 7, BFSG/TDDDG-Quellen,
owasp.org ASVS) und gegen den Spec abgeglichen. Ergebnis: AI-Act-/DSGVO-Kern war
vorhanden; **neu eingearbeitet**:

**Neue Abschnitte:**
1. **§19 Rechtsrahmen (EU & Deutschland):** AI Act mit korrekten
   Geltungszeitpunkten (vollständige Anwendung inkl. Art. 50 seit 02.08.2026 —
   damit geltendes Recht), DSGVO/BDSG-Ergänzung (keine Art.-9-Daten), TDDDG
   (Endgeräte-Einwilligungsvorbehalt), UWG § 7 (Telefon-/E-Mail-Werbung), BFSG,
   plus Due-Diligence-Vermerk geprüfter Nicht-Anwendbarkeit (NIS2, Data Act,
   DORA, EnWG).
2. **§20 Qualitäts- & Sicherheitsstandards:** OWASP ASVS Level 2 als
   Security-Baseline, OWASP Top 10, ISO/IEC 25010 (Qualitätsmodell-Mapping auf
   die Spec), ISO 27001-Prinzipien (ohne Zertifizierung), ISO/IEC 42001-
   Prinzipien für KI-Governance, EN 301 549/WCAG 2.1 AA.
3. **§21 Entwicklungsstrategie:** Spec-first-Modell, Definition of Done mit 7
   Quality-Gates, Teststrategie (Pyramide, Negativ-Pflicht, Accessibility je
   Phase, Performance-Stichproben), Release-/Upgrade-Strategie (SemVer,
   Migrationen mit Pre-Backup, Version-für-Version), Abhängigkeits-Hygiene
   (monatliches Update-Fenster, kritische Updates sofort, CI-Scans),
   Sicherheitsprozess (keine Secrets, Vier-Augen-Review, Schwachstellen-Mgmt.),
   Betriebshandbuch-Pflicht.
4. **§22 Lizenz-Compliance:** Clean-Room-Grundsatz, verbindliche Lizenz-Policy
   (erlaubt: MIT/Apache-2.0/BSD/ISC/CC0; bedingt: LGPL/MPL/EUPL mit Freigabe;
   unzulässig: AGPL/SSPL/BUSL/Commons-Clause; GPL nur mit juristischer
   Einzelfallprüfung), Lizenz-Scan als Build-Blocker in der CI,
   THIRD-PARTY-LICENSES je Release, Modell-/Asset-Lizenzen eingeschlossen
   (inkl. ODbL-Hinweis für OSM-Kartenmaterial), quartalsweises Lizenz-Inventar.

**Funktionale Konsequenzen in bestehenden Abschnitten:**
5. **UWG-Consent-Management:** neue Einwilligungs-Felder am Kontakt
   (E-Mail/Telefon mit Datum/Quelle/Nachweis, §3.1 + §12.1), UWG-Warnung in
   Draft-Review (§4.5) und Workflow-Vorbereitung (§6.4) mit Audit-Protokoll;
   keine Versand-Blockade (HITL-Einzelfallbewertung bleibt beim Menschen), aber
   vollständige Nachweisbarkeit; Absenderidentität + Widerspruchshinweis Pflicht.
6. **TDDDG-Verschärfung:** §6.5 — ohne Consent keinerlei Speicherung auf dem
   Endgerät (kein Cookie/localStorage/Fingerprint), serverseitige Zählung ohne
   Gerätebezug.
7. **BFSG-Verbindlichkeit:** §9.2 von „Basis" auf EN 301 549 / WCAG 2.1 AA als
   verbindliches Ziel hochgestuft, mit Nachweis je Phase.
8. **AI-Act-Präzisierung:** §9.4 um Geltungszeitpunkte, Risikoklassifizierung
   und Art.-4-KI-Kompetenz-Einweisungen ergänzt.
9. **Akzeptanzkriterien:** neuer Block §15.8 (UWG-Warnung, TDDDG-Nachweis,
   AI-Kennzeichnung, BFSG-Rundgang, Lizenz-Build-Blocker, DoD-Prozessprüfung).
10. **§1.3 Qualitätsziel Compliance** auf den vollständigen Rechtsrahmen (§19)
    und die Standards (§20) erweitert.

**Abwägungs-Protokoll:**
- UWG als harte Versand-Sperre verworfen — die Bestandskunden-Ausnahme
  (§ 7 Abs. 3 UWG) und B2B-mutmaßliche-Einwilligung sind Einzelfallbewertungen,
  die ein System nicht rechtsicher treffen kann; Warnung + Nachweis + HITL ist
  die saubere Lösung.
- ISO-Zertifizierungen (27001/42001) verworfen — für die Zielgröße
  unverhältnismäßig; die Praktiken gelten als Arbeitsstandard.
- GPL pauschal zu verbieten verworfen — juristische Einzelfallprüfung als
  dokumentierter Ausnahmeweg belassen (Policy 22.2).
- TDDDG-Paragraphen-Zitat bewusst ohne Nummer eingetragen (Novelle 03/2026,
  Nummerierung kann sich verschieben — der Einwilligungsvorbehalt als Prinzip
  ist stabil).

**Gesamtstand:** Die Spec deckt jetzt Fachlichkeit (Module 1–16), Struktur
(§17–§18), Rechtsrahmen (§19), Qualitäts-/Sicherheitsstandards (§20),
Entwicklungsstrategie (§21) und Lizenz-Compliance (§22) ab. Bewusst vertagt
bleibt ausschließlich die physische Ebene (§11).

### Runde 11 — Logik-Review (25 Widersprüche/Inkonsistenzen bereinigt)

Quelle: `docs/superpowers/reviews/2026-08-22-fachspec-v3-logik-review-runde3.md`
(vollständige Durchlesung des Dokuments am Stück auf logische Fehler). Alle
25 Befunde sind eingearbeitet:

**Harte Widersprüche (L1–L9):** record_fact-Anmerkung an §5.3 angeglichen;
§14 vs. §8.6 (E-Mail-Alerts nur für kritische Admin-Alerts); §13.2 vs. §7.6
(Dashboard-Basis-Kennzahlen für alle, Detail-Auswertung Admin-only);
Themenkategorie als Verweis statt Freitext (§12.3); Tracking-Besucher mit
optionalem Firma-Attribut (§12.4); Löschbegehren-Kaskade mit §16.1 vereinheit
licht (Transkripte/Tracking-Events/Form-Submissions vollständig); §4.5 um
manuelle Draft-Entstehung ergänzt; TDDDG-Paragraphenzitat aus §6.5 entfernt;
§10.3 Suchumfang um E-Mails ergänzt.

**Datenmodell (L10–L15):** Kontakt-Beziehungen um Fakten/Proposals/Agent-
Events/Tracking-Events ergänzt; Deal-Beziehungen um Proposals; Website-Story
aus Kontakt-Detail in Firmen-Tab verlagert (§3.1); Form-Intake legt nie
automatisch Firmen an (§6.5); Scope-Grundregel „Token ≤ Rolle des Inhabers"
(§8.2); Such-Performance-Ziel um E-Mail-Volltext bei 200.000 E-Mails erweitert
(§9.1).

**Struktur (L16–L19):** §15.7/§15.8 in korrekte Reihenfolge gebracht;
Modul-8-Akzeptanzkriterien von §15.8 nach §15.6 verschoben (inkl. neuer
Token-Rollen-Constraint-Prüfung); Audit-Schreiben von Phase 5 nach Phase 1
verlegt; Benachrichtigungszentrum Phase 1 zugeordnet.

**Veraltete/unvollständige Regeln (L20–L25):** HITL-Ausnahmen vollständig
dokumentiert (Research, Todo-Extraktion, Recheck-Verarbeitung — §5.7/§9.4);
§8.4 Audit-Liste um Zusatzklausel für alle Audit-Pflichten der Spec ergänzt;
Alert-E-Mail-Fallback ohne primäres Konto (§8.6); GPL-Zeile vereinfacht (§22.2);
Energiedaten-Feldstruktur ab Phase 1 klargestellt (§17); Setter-Dubletten-
Prüfung ergänzt (§7.6).

**Verifiziert konsistent (Stichproben):** Draft-Zustandsmaschine, Geofencing-
Regelwerk, Vertriebsstufen, Guardrail-Policy, Löschklassen, Workflow-Snapshots,
Glossar.

**Gesamtstand:** Fachlichkeit, Struktur, Rechtsrahmen, Standards,
Entwicklungsstrategie, Lizenz-Compliance — jetzt auch intern widerspruchsfrei.
Bewusst vertagt bleibt ausschließlich die physische Ebene (§11).

### Runde 12 — Nutzungs-Review eingearbeitet (17 Befunde aus der Fach-Perspektive)

Quelle: `docs/superpowers/reviews/2026-08-22-fachspec-v3-nutzungs-review.md`
(„gebaut wie spezifiziert, am Alltag gescheitert"). Alle 17 Befunde
eingearbeitet:

**Nutzungskiller (K-1…K-5):**
1. **K-1 Arbeits-Zuweisung:** §2.1 neues Konzept (zugewiesen an auf Todo/
   Termin/E-Mail, Team-Pool, „Meine"-Filter) — Sichtbarkeit bleibt geteilt,
   nur Zuständigkeit wird sichtbar; umgesetzt in §3.5, §3.6, §4.2, §7.6
   (Closer-Auswahl), §13.1 (Meine-Dashboard), §14 (Empfänger bevorzugt
   Zugewiesene), §12.2/§12.3 (Attribute).
2. **K-2 Abarbeitungs-Status:** §4.2/§12.3 — `offen`/`in_arbeit`/`erledigt`,
   „beantwortet" automatisch aus gesendetem Reply-Draft, Inbox-Default
   „offen & unbeantwortet" (kein Archiv nötig).
3. **K-3 Proposal-TTL:** 15 Minuten → **48 Stunden** + Ablauf-Erinnerung (12 h)
   + Reaktivierung abgelaufener Proposals (§5.3, §12.3, §14, §15.3).
4. **K-4 Triage-Korrektur:** alle Triage-Labels manuell überschreibbar,
   Korrektur gewinnt dauerhaft, `needs_reply`-Korrektur holt Todo nach (§4.3).
5. **K-5 Termin-Erinnerung:** löst jetzt eine Benachrichtigung aus (zugewiesener
   Benutzer bevorzugt), nicht nur visuelle Markierung (§3.6, §14).

**Reibung (R-1…R-8):** Deal-Bezug Firma ODER Kontakt — Privatkunden ohne
Dummy-Firmen (§3.3/§12.2); Kontakt-Geocoding wie Firmen geregelt (§3.1);
Offline-Check-Ins starten 2h-Fenster erst mit Sync + offline D2D-Deal-Anlage
(§7.5/§7.6); Todo-Fälligkeit Empfang + 1 Werktag (§4.4); UWG-Warnung gestuft
B2C/B2B/Antwort (§4.5, §19.4); Lifecycle vs. Vertriebsstufe rollengeklärt
(§3.2/§7.6); Draft-Anhänge aus Dokumenten-Ablage (§4.5/§4.6); Suche mit
„Weitere anzeigen" + Relevanz-Sortierung (§3.7).

**Verpasster Wert (V-1…V-4):** Automatische Kündigungsfrist-Wiedervorlage aus
Energiedaten (§7.2); Anschlussvertrag als terminierte Wiedervorlage statt
Einmal-Chip (§3.3); Tarif/Leistung + Konnektor-Ergebnis-Verweis am Deal
(§3.3/§12.2); 90-Tage-Postfach-Backfill beim Konto-Setup (§4.1).

**Konsistenz nachgezogen:** §12.2/§12.3 (Attribute), §15.1/15.2/15.3/15.5
(Akzeptanzkriterien inkl. 48h-TTL, Deal-Bezug, Backfill, Abarbeitungs-Status,
Kündigungsfrist, Offline), §10.3 (+7 Neuentscheidungs-Zeilen), §19.4
(gestufte Warnung).

### Runde 13 — Perspektiven-Reviews (Betreiber, Security, Lizenz) + Re-Checks

Quelle: `docs/superpowers/reviews/2026-08-22-fachspec-v3-perspektiven-review.md`
(Neustart aller Reviews aus Nutzer-, Betreiber-, Security- und
Open-Source-Perspektive auf dem Runde-12-Stand). 9 neue Befunde, alle
eingearbeitet:

1. **B-1 Offsite-Backup verpflichtend** (§8.7) — „konfigurierbar" war mit
   RTO 4 h / RPO 24 h bei Totalverlust unvereinbar.
2. **B-2 Schlüssel-Escrow** (§8.7) — Secret-Archiv-Schlüssel außerhalb des
   Servers, sonst Restore auf neuem VPS unmöglich.
3. **B-3 Externe Überwachung** (§8.8) — Uptime-Monitoring gegen Health-Endpoint
   als Betreiber-Pflicht (In-App-Alerts versagen bei Totalausfall).
4. **B-4 Upgrade-Downtime** (§8.8) — geplant, kurz, im Changelog angekündigt.
5. **S-1 SSRF-Schutz für Research-Scraper** (§5.4) — Block privater Netze,
   Loopback, Metadaten-Endpoints, eigener URL.
6. **S-2 CSRF + sichere Markdown-Ausgabekodierung** (§20.1) — auch für eigene
   Templates/Drafts, nicht nur empfangene E-Mail-HTML.
7. **O-1 OSM-Attribution sichtbar** in der Kartenansicht (§22.3, ODbL).
8. **O-2 Dev-Abhängigkeiten gelockert** (§22.3) — „Bedingt"-Lizenzen für reine
   Entwicklungswerkzeuge erlaubt (nicht ausgeliefert), Inventar bleibt Pflicht.
9. **S-3 Audio-Scan** bewusst als akzeptiertes Restrisiko dokumentiert
   (Audio-Container keine praktikablen Malware-Vektoren) — kein Spec-Eingriff.

**Re-Checks:** Logik (0 neue Fehler; TTL-/Bezug-Nachzüge verifiziert),
Implementierbarkeit (0 neue Ambiguitäten; Runde-12-Ergänzungen zustandsarm),
Nutzer (K-1…K-5 als gelöst verifiziert).

### Runde 14 — Fach-Review eingearbeitet (16 Befunde aus Geschäftslogik-Sicht)

Quelle: `docs/superpowers/reviews/2026-08-22-fachspec-v3-fach-review.md`
(Vertriebsexperten-Durchgang, B2C-Fokus). Alle 16 Befunde eingearbeitet:

**Kritische Geschäftslogik (F-1…F-6):**
1. **F-1** Kontakt-E-Mail optional; Identifikation über E-Mail ODER Telefon
   (§3.1, §12.1) — Setter-Hauptfall „Lead nur mit Telefon" funktioniert;
   Vervollständigung automatisch beim ersten E-Mail-Kontakt.
2. **F-2** Deal-Status `widerrufen` (nur aus `won`, §-355-BGB-Realität):
   Widerrufsdatum + Pflicht-Begründung, gesonderte Auswertung, keine
   Anschluss-Wiedervorlage, Einmal-Vorschlag „Widerrufs-Arbeitsschritte
   prüfen" (§3.3, §12.2, §15.1).
3. **F-3** Kundentyp Pflichtfeld am Kontakt (Default ohne/mit Firma) —
   Fundament der UWG-Stufung (§3.1, §4.5, §12.1).
4. **F-4** Backfill: Historie mit Abarbeitungs-Status `erledigt` und **ohne
   Todo-Extraktion** (letzte 3 Tage voll) — keine Todo-Flut am Tag 1 (§4.1,
   §15.2).
5. **F-5** Besuchsstatus aufgelöst: Vertriebsstufe ist der alleinige
   D2D-Status; Kartenfarben mappen auf sie (§3.1, §7.4, §12.1, Glossar).
6. **F-6** Matching um Stufe 0 erweitert: exakte Absender-Adresse → bekannter
   Kontakt (deckt Freemail/Privatkunden); unbekannte Privatkunden werden ohne
   Firma angelegt (§4.2, §15.2).

**Prozess-Vervollständigungen (F-7…F-12):** Doppelverkauf-Warnung bei
laufendem `won`-Vertrag (§3.3); Reaktivierungs-Wiedervorlage bei `lost`
(Default 6 Monate); Lieferstart-Prüfung bei zukünftigem Vertragsbeginn;
Energiedaten-Pflege-Vorschlag bei `won` (§3.3, sonst rechnet die
Kündigungsfrist-Wiedervorlage mit Alt-Daten); **Dubletten-Merge im
Tagesgeschäft** (neuer §3.10); `zugewiesen an` auch an Kontakt und Deal
(„meine Kunden", §3.1/§12.1/§12.2).

**Dokumentierte Vereinfachungen (F-13…F-16):** neuer §3.11 (Angebots-Status
bewusst YAGNI, Zweitgeschäft nach `abgeschlossen` erlaubt, No-show über
Termin-Status).

**Nachgezogen:** §12.1/§12.2 (Attribute), §15.1/15.2 (Akzeptanzkriterien),
§10.3 (+7 Neuentscheidungen), Glossar (Vertriebsstufe).

### Runde 15 — Markt-Review eingearbeitet: Generalistischer Kern + Energie als Leit-Vertikale + 5 Marktlücken geschlossen

Quelle: `docs/superpowers/reviews/2026-08-22-fachspec-v3-markt-startup-bewertung.md`
(8 Startup-Personas; 5 echte Lücken G-1…G-5). User-Entscheidung: Die Spec wird
**generalistisch** ausgerichtet (Kern branchenneutral, Schärfe über Module),
mit **Energie/Feldvertrieb als erster Vertikale und Implementierungs-Leitbild**.

**Strategische Umstellung:**
1. **§1.1 neu formuliert:** generalistisches CRM für kleine Teams; Kern
   branchenneutral; Vertikal-Schärfe über Modul 7; Energie/Feldvertrieb ist
   Referenz- und Prioritätsvertikale für die Implementierung (§17), ohne den
   Kern einzuengen. Kernversprechen um Kanalkompetenz erweitert
   (E-Mail/Telefonie/Website-Formulare).
2. **§17 Phasen neu gewichtet:** Prioritätsgrundsatz eingeführt —
   Energie-Referenz-Anforderungen (Energiedaten-Felder, Vertriebsstufen,
   Geocoding, Merge) ab Phase 1, weil der Referenz-Kunde sie am ersten Tag
   braucht; Konnektoren/Karte/Setter-Closer-Views in Phase 5; Phase 2 um
   Backfill + Termin-Einladungen ergänzt; Phase 4 um Vertriebs-Report ergänzt.
   Andere Vertikalen entstehen später nur über neue Konnektor-Typen und
   Custom Fields — ohne Kernänderung.

**Marktlücken geschlossen (G-1…G-5):**
3. **G-1 Telefonie:** neuer **§7.1a Telefonie-Konnektor** — Konnektoren-Schiene
   auf Daten- und Funktions-Konnektoren erweitert (§7.1, §12.5, Glossar):
   Click-to-Call mit automatischer `call`-Aktivität + Laufzeit, Anruf-Notiz
   (mit KI-Chips), eingehende Anrufe mit Rufnummer-Matching, keine
   Gesprächsaufzeichnung (Datenschutz).
4. **G-2/G-4 Forecast & Zeitreihe:** §6.3 **Vertriebs-Report** —
   Forecast-Widget (offene Monatswerte je erwartetem Abschlussmonat +
   gesicherte won-Werte), Abschluss-Zeitreihe (12 Monate, Wandlungsquote,
   widerrufen separat), Export-Job `vertriebsreport`; rein lesbar aus dem
   Deal-Modell.
5. **G-3 Termin-Einladungen:** §3.6 — ICS-Einladungsmail an Kunden
   (Bestätigungsmail, kein Abstimmungs-Workflow), protokolliert als
   `email`-Aktivität.
6. **G-5 Formulare:** §6.5 — **konfigurierbare Intake-Formulare** (beliebig
   viele je Domain, eigene Feldsätze); neue Entität Intake-Formular (§12.4),
   Form-Submission mit Formular-Referenz.

**Nachgezogen:** §12.4/§12.5 (Entitäten: Intake-Formular, Konnektor-Typ
`telefonie`), §15.1 (ICS), §15.4 (Vertriebs-Report, Formulare), §15.5
(Telefonie), §10.3 (+6 Neuentscheidungen), Glossar (Konnektor).
