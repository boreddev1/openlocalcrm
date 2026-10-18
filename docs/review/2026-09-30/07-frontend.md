# 07 – Frontend-Review (web/)

Stand: 2026-09-30, read-only. Belege als `datei:zeile` oder Command-Output. Aufwand: S = <0,5 Tag, M = 1-2 Tage, L = >2 Tage.

## Ausgeführte Gates (web/)

| Befehl | Ergebnis |
|---|---|
| `pnpm lint` | 0 Fehler, 2 Warnungen (`OfferCalculatorModal.tsx:117` und `SimulationWidget.tsx:288`, jeweils `react-hooks/exhaustive-deps`) |
| `pnpm typecheck` | sauber |
| `pnpm format:check` | sauber |
| `pnpm build` | ok; ein Haupt-Chunk `index-*.js` 870,46 kB (gzip 221,5 kB) plus lazy `SimulationWidget` 11,4 kB; Vite meldet Chunk >800 kB. `web/dist` danach unverändert zu git (Hash `index-BsRaD7pK.js` identisch, `git status` sauber). |
| `pnpm audit` | "No known vulnerabilities found" |
| `pnpm outdated` | u. a. react 18.3.1 -> 19.3.0, vite 6.4.3 -> 8.3.1, typescript 6.0.3 -> 7.0.2, tailwindcss 3.4.19 -> 4.3.3, react-leaflet 4.2.1 -> 5.0.0 (Major-Rückstand, kein Sicherheitsbefund) |

## Findings

### F-7-01 (S2) Nach Token-Refresh scheitert der wiederholte Request mit 403 (veralteter CSRF-Header)
- Evidenz: `web/src/api/client.ts:100-112` (fetch-Interceptor wiederholt bei 401 nach Refresh `originalFetch(input, init)` mit unverändertem `init`, Zeile 109); `client.ts:126-132` (`apiFetch` schreibt `X-CSRF-Token` einmalig aus dem Cookie in `headers`); `internal/server/handlers/auth.go:183` (Refresh ruft `auth.SetCSRFCookie` und rotiert das Cookie); `internal/auth/csrf.go:170-190` (Middleware vergleicht Header mit Cookie per `ConstantTimeCompare`); `internal/server/router.go:349-351` (AuthMiddleware läuft vor CSRF, also kommt zuerst 401).
- Ablauf laut Code: abgelaufenes Access-Token -> 401 -> Refresh rotiert das CSRF-Cookie -> Retry sendet den alten Header -> `csrf_token_mismatch` (403). Betroffen sind alle mutierenden Requests (POST/PUT/PATCH/DELETE), die genau den Refresh auslösen. Nicht im Browser nachgestellt (statisch aus Code abgeleitet).
- Auswirkung: erste Speicheraktion nach Access-Token-Ablauf scheitert scheinbar zufällig; Nutzer sehen "Ungültiges CSRF-Token".
- Fix: beim Retry Header neu aus dem Cookie setzen (Request klonen, `X-CSRF-Token` ersetzen). Playwright-Test für "Access abgelaufen + POST".
- Aufwand: S

### F-7-02 (S2) Karte (MapView) verstößt gegen die eigene CSP und leitet Adressdaten an Drittanbieter
- Evidenz: `web/index.html:9-14` (Leaflet-CSS von `unpkg.com`), `web/src/pages/MapViewPage.tsx:12-14` (Marker-Icons von `unpkg.com`), `MapViewPage.tsx:142-143` (Kacheln `https://{s}.tile.openstreetmap.org`); CSP im Go-Server `internal/server/router.go:110` (`style-src 'self' 'unsafe-inline'`, `img-src 'self' data:`) sowie `Caddyfile:22` (`img-src 'self' data: https:`, `style-src 'self' 'unsafe-inline'`). Beide Header werden gesetzt; der Browser wendet beide an (Schnittmenge).
- Auswirkung (1, Funktion): Stylesheet von unpkg und Kacheln/Icons sind mit `style-src`/`img-src` nicht erlaubt, die Karte ist unter Produktiv-Headern voraussichtlich kaputt (nicht im Browser verifiziert). (2, Datenschutz): würde die CSP gelockert, gehen Client-IP und Kachel-Koordinaten (= Standort der Kundenadressen) an OSM und unpkg; das widerspricht dem Anspruch "Datensouveränität / Single-Tenant" und ist ohne Hinweis/Einwilligung DSGVO-relevant. Zudem CDN-Abhängigkeit zur Laufzeit trotz "Air-gapped"-Anspruch.
- Fix: Leaflet-CSS und Icons über das npm-Paket bündeln (`import 'leaflet/dist/leaflet.css'`); Kachelserver konfigurierbar machen (self-hosted) oder Feature mit Datenschutzhinweis/Opt-in versehen; CSP an einer Stelle definieren (siehe F-8-10).
- Aufwand: M

### F-7-03 (S3) Query-Cache wird bei Logout/Login nicht geleert (Datenübertragung zwischen Benutzern im selben Tab)
- Evidenz: `web/src/context/AuthContext.tsx:100-102` (`logout` setzt nur `setUser(null)`), `AuthContext.tsx:69-71` (`auth:logout` ebenso); `grep -rn "queryClient.clear|removeQueries|resetQueries" web/src` liefert keinen Treffer; `main.tsx:10-16` `staleTime` 30 s.
- Auswirkung: Nach Benutzerwechsel (auch DemoBanner-Schnellwechsel, `DemoBanner.tsx:30`) zeigen gecachte Listen (Kontakte, Deals, Notizen, Inbox) kurz Daten des vorherigen Benutzers, bevor der Refetch läuft; bei Rollenwechsel (ADMIN -> VERTRIEB) sind Daten sichtbar, die die neue Rolle nicht sehen dürfte.
- Fix: bei Logout, `auth:logout` und Login `queryClient.clear()` (Provider mit Zugriff auf den Client).
- Aufwand: S

### F-7-04 (S3) SSE-Events werden nie ausgewertet, "Live-Updates" sind nur ein Statuspunkt; SSE-Reconnect ohne Token-Refresh
- Evidenz: `grep -rn "lastEvent\|email_received\|deal_updated\|todo_assigned\|ai_stream" web/src` -> nur `hooks/useSSE.ts`; einziger Aufrufer `components/layout/Header.tsx:15` nutzt nur `connected`. Benachrichtigungen kommen per Polling (`Header.tsx:22` `refetchInterval: 15000`). `useSSE.ts:22-27`: bei `onerror` Reconnect alle 3 s ohne Backoff; `EventSource` läuft nicht durch den fetch-Interceptor (kein Refresh), bei abgelaufenem Access-Token also Dauer-Schleife mit 401.
- Auswirkung: Architektur-/README-Zusage "Realtime via SSE" ist im UI nicht eingelöst (Inbox, Deals, Todos aktualisieren sich nicht); abgelaufene Session erzeugt alle 3 s Requests (kann Rate-Limiter/Log-Rauschen erzeugen).
- Fix: `lastEvent` in einem Provider konsumieren und `queryClient.invalidateQueries` je Event-Typ; Reconnect mit exponentiellem Backoff und vorherigem `/api/v1/me`-/Refresh-Aufruf.
- Aufwand: M

### F-7-05 (S3) Mutationen ohne Fehlerbehandlung (stille Fehlschläge), Drag-and-Drop ohne Rückmeldung
- Evidenz: 42 `useMutation`-Aufrufe (`grep -rn useMutation web/src | wc -l`), nur 9 `onError` (`DealsPage.tsx:224`, `InboxPage.tsx:240,274`, `AutomationsPage.tsx:78,100,120`, u. a.). `DealsPage.tsx:159-176` (`createMutation`, `updateMutation`, `deleteMutation` ohne `onError`); `DealsPage.tsx:239-254` `handleDragEnd` ruft `updateMutation.mutate` ohne Fehlerpfad. Die Kanban-Karte springt zurück (kein optimistisches Update, damit kein Rollback-Bug), der Nutzer erfährt aber nicht, dass z. B. 403/409 auftrat. Zusätzlich sendet das DnD ein vollständiges `PUT` des ganzen Deals (`{...deal, stage}`) = Lost-Update-Risiko bei paralleler Bearbeitung.
- Auswirkung: Nutzer glauben, Änderung sei gespeichert; bei 403 (Ownership aus SP8a) keine Meldung.
- Fix: globaler `MutationCache.onError` im QueryClient (Toast) plus gezieltes `PATCH stage`; Fehlerbanner nutzen wie beim Widerruf.
- Aufwand: M

### F-7-06 (S3) Keine Code-Splitting-Strategie, Chunk-Warnung stummgeschaltet
- Evidenz: `pnpm build` -> `index-*.js` 870 kB; `web/vite.config.ts:27` `chunkSizeWarningLimit: 800`; `web/src/App.tsx:1-15` alle 13 Seiten statisch importiert; nur `SimulationWidget` ist lazy (`AppLayout.tsx:11`). Leaflet, dnd-kit, qrcode werden auch für Login geladen.
- Fix: `React.lazy` pro Route, `manualChunks` für leaflet/dnd-kit/react-query.
- Aufwand: S

### F-7-07 (S3) Riesige Seitenkomponenten, hohe Wartungskosten
- Evidenz: `wc -l`: `SettingsPage.tsx` 2548, `AIAssistantPage.tsx` 1384, `ContactsPage.tsx` 1146, `TodosPage.tsx` 1042, `DealsPage.tsx` 987, `CompaniesPage.tsx` 855, `InboxPage.tsx` 797 Zeilen; Formulare, Modals und Queries in einer Datei; wiederholtes Muster für Feedbackbanner (`setTimeout(() => setFeedbackBanner(null), 4000)` in jeder Mutation).
- Fix: in Feature-Komponenten und Hooks (`useDeals`, `useContacts`) zerlegen; gemeinsamen Toast-Provider.
- Aufwand: L

### F-7-08 (S3) Demo-/Beispielwerte sind Standardwerte in Produktiv-Formularen
- Evidenz: `DealsPage.tsx:231-238` (Vorbelegung `value: '18500.00'`, `contact_name: 'Max Mustermann'`, `notes: 'Photovoltaikanlage 12 kWp...'`), `TodosPage.tsx:104` ('Sabine Mustermann'), `CalendarPage.tsx:70-76` (Kontakt, Firma, `assigned_to: 'Max Mustermann (Vertrieb)'`), `SettingsPage.tsx:988` (`last_name: 'Mustermann'`).
- Auswirkung: versehentlich gespeicherte Fantasie-Deals verfälschen Pipeline-Zahlen und Reports; `assigned_to` als Freitext-Name statt ID.
- Fix: Formulare leer starten, Platzhalter statt Werte.
- Aufwand: S

### F-7-09 (S3) `any` und deaktivierte Lint-Regel, schwache Typisierung der API-Schicht
- Evidenz: `web/eslint.config.js:19` `'@typescript-eslint/no-explicit-any': 'off'`; `grep -rnE ': any|as any|@ts-ignore|eslint-disable' web/src | wc -l` = 100 Treffer (0 `@ts-ignore`, 0 `eslint-disable`); `api/client.ts` liefert `apiFetch<T>` ohne Laufzeitvalidierung und gibt bei 204/leerem Body `{} as T` zurück (`client.ts:150-156`), `sendEmail(): Promise<any>` etc. Spezialfall Nullable-Felder: `getFieldText`/`.String?.` (`InboxPage.tsx:168,212,387`) zeigt, dass pgtype-Objekte (`{String, Valid}`) ungefiltert ans UI durchgereicht werden.
- Fix: API-Typen aus OpenAPI/Go-Structs erzeugen oder Zod-Schemas, Regel auf `warn`; Backend-JSON für pgtype vereinheitlichen.
- Aufwand: L

### F-7-10 (S3) Barrierefreiheit: Labels nicht verknüpft
- Evidenz: `grep -rn "<label" web/src | wc -l` = 60, davon `grep -c htmlFor` = 0 (keine einzige `htmlFor`-Verknüpfung); 34 Treffer für `aria-label|role=` bei 157 Verwendungen von Formularfeldern/Buttons insgesamt (Größenordnung, keine vollständige Analyse). Modals: kein Fokus-Trap/Escape-Nachweis gefunden (nicht vollständig geprüft; Stichprobe `DealsPage`, `ContactsPage`).
- Auswirkung: Screenreader und Klick-auf-Label funktionieren nicht; Verstoß gegen BFSG/WCAG 2.1 (B2B-Software in DE seit 06/2025 im Fokus).
- Fix: `htmlFor`/`id`-Paare oder `<label>` umschließend; Dialog-Komponente mit `role="dialog"`, `aria-modal`, Fokus-Trap; Kontrast prüfen (`text-slate-400` auf `bg-slate-950`, nicht gemessen).
- Aufwand: M

### F-7-11 (S3) Native `alert()`/`confirm()` an kritischen Stellen
- Evidenz: `SettingsPage.tsx:577` (`window.confirm` vor Replace-Restore), `:642`, `:725`, `:729`, `:947`; `ContactsPage.tsx:163` (`alert`). Die Replace-Restore-Bestätigung nennt "ALLE 24 Tabellen" (hartkodierte Zahl, `SettingsPage.tsx:578`), ohne dass der Nutzer den Dateinamen oder das Backup-Datum bestätigt.
- Auswirkung: destruktive Aktion (Truncate aller Tabellen) nur durch einen Standarddialog geschützt; Zahl kann vom Backend abweichen.
- Fix: eigener Bestätigungsdialog mit Tippbestätigung ("ERSETZEN") und dynamischer Tabellenzahl aus der Server-Antwort.
- Aufwand: S

### F-7-12 (S4) Vorschau-CSV-Parser naiv, ganze Datei im Speicher
- Evidenz: `CSVImportModal.tsx:50-60` (`split('\n')`/`split(',')`, keine Anführungszeichen-/CRLF-Behandlung, `readAsText` ohne Größenlimit); Server-Limit 50 MB (`router.go:~125`).
- Auswirkung: nur Vorschau falsch bei Feldern mit Komma; große Dateien blockieren den Tab.
- Fix: Größenlimit clientseitig, robusten Parser nutzen oder nur Header zeigen.
- Aufwand: S

### F-7-13 (S4) Doppelte `auth:logout`-Auslöser und Interceptor-Nebenwirkungen
- Evidenz: `client.ts:111` (Interceptor) und `client.ts:184` (`apiFetch`) lösen beide `auth:logout` aus; der Interceptor greift für alle `fetch`-Aufrufe global (`window.fetch` wird überschrieben, `client.ts:89-118`), inkl. `AcceptInvitePage.tsx:38` (öffentliche Route, kein Refresh sinnvoll). Ausnahmeliste per `urlStr.includes(...)` ist substring-basiert. Module-Variable `lastRefreshSuccess` (2-s-Fenster, `client.ts:39,71`) kann bei zwei nahen 401 unterschiedlicher Tabs zum Überspringen des Refresh führen.
- Fix: Interceptor auf `apiFetch` beschränken oder Ausnahmen exakt matchen.
- Aufwand: S

### F-7-14 (S4) Demo-Zugangsdaten im Produktiv-Bundle
- Evidenz: `DemoBanner.tsx:30-31,~55-70` (`login(email, 'demo123')`, Admin-Schnellwechsel). Anzeige nur bei `demo_mode` aus `/api/v1/health` (`router.go:135-143`, öffentlich).
- Auswirkung: kein Leck in Produktion (Banner nur im Demo-Modus), aber das Passwort `demo123` steht im ausgelieferten JS; bei fälschlich aktiviertem `DEMO_MODE=true` sofort Admin-Zugang. Einordnung gehört zu Task 3/8 (Betrieb).
- Fix: Build-Flag (`import.meta.env.VITE_DEMO`) oder Demo-Bundle getrennt.
- Aufwand: S

### F-7-15 (S4) i18n / Konsistenz
- Evidenz: UI überwiegend Deutsch (`lang="de"`, `web/index.html:2`), aber gemischte Begriffe (Seitentitel "Single-Tenant Sales Workspace" `index.html:7`, Emojis in Statusmeldungen `SettingsPage.tsx:~592`), Zahlen-/Datumsformate lokal per Hand (`localDateTimestamp`). Kein i18n-Framework; alle Strings hartkodiert.
- Fix: nur nötig, wenn Mehrsprachigkeit Ziel ist; sonst zentrale Textkonstanten.
- Aufwand: L (optional)

### F-7-16 (S4) Tests: 0 Unit-Tests im Frontend
- Evidenz: `web/package.json` ohne Test-Runner (nur Playwright-E2E `test:e2e`); keine `*.test.*`-Dateien in `web/`. CI führt Playwright im Demo-Modus aus (F-8-06).
- Risikokomponenten, die ein Vitest-Grundgerüst zuerst brauchen (Reihenfolge nach Risiko): (1) `api/client.ts` (Refresh-Lock, CSRF-Retry, 401-Ausnahmen, Stream-Download-Integrität `downloadToFile`, `uploadBackup` committed-Flag), (2) `context/AuthContext.tsx` (`totp_enabled` null-Semantik, Login-Flow), (3) `DealsPage` Kanban (`handleDragEnd`, Widerruf-Modal), (4) `SettingsPage` Backup-/Restore-/Import-Sanitizing, (5) `CSVImportModal`, (6) `useSSE`.
- Aufwand: M

## Geprüft ohne Befund (verified OK)

- Kein Token in `localStorage`/`sessionStorage`: `grep -rnE 'localStorage|sessionStorage' web/src` liefert keinen Treffer; Session nur über HttpOnly-Cookies (`AuthContext.tsx:63-66`), `credentials: 'include'` in allen Fetch-Pfaden (`client.ts:139,214,...`).
- CSRF-Header wird für POST/PUT/PATCH/DELETE aus dem Cookie gesetzt (`client.ts:126-132`), Upload/Download ebenso (`client.ts:208-212`, `:300`); Refresh sendet ebenfalls CSRF (`client.ts:75-77`). Einschränkung siehe F-7-01.
- Kein `dangerouslySetInnerHTML`, `innerHTML`, `eval(`, `window.open` in `web/src` (grep leer). Mail-Inhalte (`InboxPage.tsx`), Notizen und AI-Antworten (`AIChatDrawer.tsx:218`, `AIAssistantPage.tsx:392,620`) werden als React-Text gerendert (`whitespace-pre-wrap`), kein Markdown-HTML-Renderer; XSS über Inhalte nicht möglich. Die beiden `href={...}`: `tel:` mit `contact.phone.trim()` (`CallModal.tsx:120`; `tel:` mit Nutzerdaten ist unkritisch, React blockt `javascript:` nicht generell, aber das Präfix ist fest) und ICS-Download `/api/v1/appointments/${app.id}/ics` (`CalendarPage.tsx:335`, ID aus API).
- 401-Behandlung ohne Redirect-Schleife: `ProtectedRoute` (`App.tsx:19-33`) leitet auf `/login`; Login-/TOTP-Disable-Endpunkte sind von der Session-Ablauf-Logik ausgenommen (`client.ts:53-56`); Refresh wird cross-tab per Web Lock serialisiert (`client.ts:44-52`).
- `/accept-invite` ist öffentlich und bewusst außerhalb von `ProtectedRoute` (`App.tsx:47-50`); Token wird `encodeURIComponent`-kodiert (`AcceptInvitePage.tsx:38`). Hinweis: Token liegt in der URL (Referrer-Policy `strict-origin-when-cross-origin` schützt Cross-Origin-Leaks: `router.go:108`, `Caddyfile:18`).
- Backup-Download prüft Stream-Vollständigkeit gegen `Content-Length` und erzeugt bei Abbruch keine Datei (`client.ts:262-291`); Restore behandelt `committed`-Flag (`client.ts:340-352`).
- Fehlergrenze vorhanden (`ErrorBoundary.tsx`, `main.tsx:19`); Simulation ist lazy und nur im Demo-Modus eingebunden (`AppLayout.tsx:11-13,60-63`).
- Abhängigkeiten: `pnpm audit` ohne bekannte Lücken; Lizenzlage siehe 08-ops-supply-chain.md (F-8-09).
