# Launcher-KI-UX — Design (L7)

> **Datum:** 2026-09-21
> **Kontext:** L1–L6 (Launcher-Embedding-Auswahl) sind implementiert und live verifiziert
> (Branch `fix/sp3-ai-truth`, HEAD `ce9a201`). Nutzer-Feedback: Der KI-Bereich im
> Installer braucht mehr Einstellbarkeit und mehr Klarheit. (Das „Load failed" war kein
> Code-Bug: der Controller hatte die getestete Launcher-Instanz beendet, während der
> Browser-Tab offen war.)
> **Status:** Design freigegeben (Brainstorming 2026-09-21).

## Ziel

Der KI-Bereich des Setup-Launchers wird klarer und praktischer: echte Modell-Dropdowns
(Datalists), Provider-Presets für Mistral/Nebius, **ein** Test-Button mit **zwei
getrennten Statuszeilen** (Chat/Embedding), ehrliche Hinweiszeile zum Embedding-Kontrakt.

## Faktenbasis

- Gateway-Chat unterstützt **nur** `ollama` und `openai` (internal/ai/gateway.go:163–169).
  Mistral AI und Nebius AI Studio sind OpenAI-kompatibel — ein Preset muss deshalb auf
  `AI_PROVIDER=openai` + URL-Prefill mappen, niemals auf `AI_PROVIDER=mistral` (Chat
  würde „unbekannter AI-Provider" werfen).
- Der Probe (`/api/ai/test`, L2) liefert bereits `{success, message, latency_ms,
  embedding:{success, message, dims}}` — die zwei Statuszeilen sind ein reines
  UI-Rendering dieses Responses, kein Backend-Change für die Tests nötig.
- `/api/tags` (Ollama) und `GET /models` (OpenAI-kompatibel) liefern Modelllisten;
  nicht jeder kompatible Endpunkt unterstützt `/models` → ehrlicher Fallback auf
  Freitext.

## Design

### 1. Neuer Endpoint `GET /api/ai/models` (Launcher-Server)

Query-Parameter: `provider` (`ollama`|`openai`), `base_url`, `api_key`.

- `ollama`: `GET {base}/api/tags` → Response `{"models":[{"name":…}]}` → antwortet
  `{"success": true, "models": ["gemma4:12b", "qwen3-embedding:0.6b", …]}`.
- `openai`: `GET {base}/models` mit `Authorization: Bearer <key>` (wenn Key gesetzt)
  → Response `{"data":[{"id":…}]}` → `{"success": true, "models": [ids]}`.
- `base_url`-Default analog Probe: leer + `ollama` → `http://localhost:11434`, sonst
  `https://api.openai.com/v1`. Getrimmt (`TrimRight "/"`), **kein**
  `TranslateHostForDocker` (die UI-Datalist zeigt Host-Perspektive, nicht Container).
- Ehrliche Fehler: nicht erreichbar / Status != 200 / unparsebar →
  `{"success": false, "message": "<ursache>"}` — kein erfundene Modellliste.
- Timeout 10 s; kein Caching (RAM-Budget); Provider != ollama|openai → 400.

### 2. UI: Modell-Datalists statt blindem Freitext

- `ai-model` und `ai-embedding-model` werden `<input list="…">` mit zugeordnetem
  `<datalist id="…">`.
- Abruf der Modelle: bei Provider-Wechsel (nachdem `onAIProviderChange` die URL
  vorgefüllt hat) und bei `change`-Event auf dem URL-Feld. Bei Fehlschlag bleibt der
  Datalist leer — Freitext funktioniert weiter (ehrlich, kein Fake-State).
- Embedding-Datalist erhält dieselbe Modellliste (ehrlich: ein reines Text-Modell
  würde beim Test ehrlich scheitern; der Hinweis unter dem Feld erklärt den Kontrakt).
  Kein Filtern/Erraten, welche Modelle Embedding-fähig sind — `/api/tags` liefert das
  nicht zuverlässig.

### 3. Provider-Presets (Mistral AI, Nebius AI Studio)

- Dropdown-Optionen: `🖥️ Lokales Ollama (DSGVO 100 % lokal)`, `🌐 OpenAI /
  OpenAI-kompatibel (OpenRouter, Groq, vLLM, etc.)`, `⚡ Mistral AI`, `☁️ Nebius AI
  Studio`, `🚫 Keine KI (vorerst deaktiviert)`.
- `onAIProviderChange` behandelt `mistral`/`nebius` als OpenAI-familie:
  URL-Prefill `https://api.mistral.ai/v1` bzw. `https://api.studio.nebius.ai/v1`,
  Key-Gruppe sichtbar, **Modellfeld leer** (kein erfundenes Default — die Probe
  validiert), Embedding-Feld leer.
- Mapping beim Senden (Probe **und** Setup-Payload): Provider-Wert `mistral`/`nebius`
  → `openai` (mit der vorausgefüllten URL). Single Source im JS
  (`effectiveAIProvider()`-Hilfsfunktion), damit Probe und Setup identisch mappen.
  Damit schreibt der Installer `AI_PROVIDER=openai` — Chat läuft über den bestehenden
  OpenAI-kompatiblen Pfad.
- Anthropic/Claude und DeepSeek werden **bewusst nicht** als Optionen angeboten
  (Chat nicht implementiert → Fake-Option wäre unehrlich).

### 4. Ein Test-Button, zwei Statuszeilen

- Der kombinierte Button „🧪 KI-Verbindung testen" bleibt; beide Teil-Tests laufen bei
  jedem Klick (der Probe liefert beide Ergebnisse).
- Zwei Statuszeilen unter dem Button:
  - `Chat: ✅ …(<latency> ms)` / `Chat: ❌ <ursache>`
  - `Embedding: ✅ OK (<modell>, <dims> Dimensionen)` / `Embedding: ❌ <ursache> (inkl.
    Konsequenz bei Dimensionsmismatch)`
- Die kombinierte alte Einzelmeldung entfällt. Der Button selbst zeigt Erfolg nur,
  wenn beide Zeilen grün sind (nutzt das bestehende Overall-`success`).

### 5. Hinweiszeile unter dem Embedding-Feld

Fester Hilfetext (HTML-Kleinunterzeile):
„Benötigt wird ein 1024-dimensionales Modell (DB-Spalte `vector(1024)`); bei Mismatch
startet crm-server nicht. Anthropic/DeepSeek bieten keine Embedding-API an."

## Fehlerbehandlung

Kein Fake-Success: Modellabruf und Probe schlagen ehrlich fehl; UI zeigt immer die
konkrete Ursache; Datalist-Fehlschlag degradiert sichtbar auf Freitext.

## Teststrategie

- Server: Unit-Tests für `/api/ai/models` (ollama-Liste, openai-Liste, nicht
  erreichbar, falscher Status, unparsebar, ungültiger Provider → 400), hermetisch via
  httptest.
- UI: kein Launcher-Frontend-Test-Framework — Live-Verifikation durch den Controller
  (Playwright/klick-für-klick): Presets mappen auf openai+URL, Datalists füllen mit
  lokalen Modellen, zwei Statuszeilen bei OK- und Fehlerfällen, Hinweiszeile sichtbar.

## Ausdrücklich out of scope

- Anthropic/Claude- und DeepSeek-Chat-Provider (Chat im Gateway nicht implementiert).
- Weitere Presets (OpenRouter, Groq) — `openai`-Option deckt sie als URL-Freitext ab.
- Timeout-Feld, Provider-Fähigkeits-Tabellen, Re-Embed-Job.
