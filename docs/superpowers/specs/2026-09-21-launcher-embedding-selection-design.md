# Launcher Embedding-Auswahl — Design (Rev. 4)

> **Datum:** 2026-09-21
> **Kontext:** SP3/A9 (pgvector-Embeddings, Multi-Provider) ist im Backend implementiert
> (Branch `fix/sp3-ai-truth`, Commits `b2ce7da`, `78301bb`). Der Setup-Launcher
> (`internal/launcher`, UI `internal/launcher/ui`) bietet bisher nur Chat-Konfiguration.
> **Status:** Rev. 2/3 nach Spec-Review verworfen (jeweils 4 Blocker); Rev. 4 behebt die
> Rev.-3-Blocker, siehe Korrekturprotokoll am Ende. Entscheidungen: `internal/ai`-Import
> in die Launcher-Binary wird akzeptiert (Leaf-Package-Extraktion später).

## Ziel

Der Installer ermöglicht die Auswahl des Embedding-Modells — **nur dort, wo es sinnvoll
ist**. Die bestehende Chat-Auswahl bleibt unverändert.

## Ist-Zustand (präzise)

- Launcher-UI: Provider-Dropdown `ollama` / `openai` (OpenAI-kompatibel: OpenRouter,
  Groq, vLLM, …) / `none`; Felder `ai-url`, `ai-model`, `ai-key`; `/api/ai/test` probe
  prüft ausschließlich Chat; `testAIConnection` liest nur `data.success`/`data.message`.
- Chat-Default **inkonsistent im Ist-Zustand**: `onAIProviderChange` setzt `gemma4:12b`,
  `templates.go` schreibt `gemma4:12b`, aber `ui/index.html` hat das Feld-Default
  `gemma2:12b`. → Korrektur auf `gemma4:12b` (Teil dieses Tasks). CLI-Default
  (`cmd/setup-launcher/cli.go`: `gemma2:9b`) bleibt unverändert (out of scope).
- `NewGateway` ist **kein reiner Config-Konstruktor** (internal/ai/gateway.go:117–131):
  leere Felder fallen auf die Prozess-Env zurück — `EmbeddingModel` auf
  `AI_EMBEDDING_MODEL`, dann `OLLAMA_EMBEDDING_MODEL`, dann (nur bei Ollama) auf
  `qwen3-embedding:0.6b`; leerer `APIKey` auf `AI_API_KEY`; `OLLAMA_TIMEOUT_SECONDS`
  überschreibt den Client-Timeout (gateway.go:123) und wird vom Env-Fallback-Flag
  **nicht** gestoppt.
- URL-Felder im Gateway sind **getrennt**: `embedOllama` nimmt `OllamaBaseURL`
  (gateway.go:344), `embedOpenAICompatible` nimmt `AIBaseURL` (gateway.go:323–334) und
  ignoriert die Chat-URL. `AIBaseURL` wird im Gateway getrimmt, `OllamaBaseURL` nicht.
  `callOpenAI` (Chat) liest zudem `AI_MODEL`/`OPENAI_MODEL` an jedem Flag vorbei
  (gateway.go:166) — der Chat-Probe darf deshalb **nicht** über `NewGateway.Generate`
  laufen.
- `ProbeAIConnection`: eigener Chat-Client mit 5 s Timeout / 6 s Probe-Context
  (ai_probe.go:48, :59). Kaltes Modell-Loading braucht länger → falsch-roter Test.
- `cmd/server/main.go:161`: Startcheck für Embeddings mit 5 s-Context. Das ist keine
  Fehlfahrt (Mismatch = `log.Fatalf`, Timeout = nur Warnung, main.go:164), aber 5 s
  schneidet kalte Modell-Loads zu hart ab.
- `TranslateHostForDocker` wird nur in `config.go` (Env-/Compose-Schicht, Zeile 245)
  angewandt — nicht im Probe. So bleibt es.
- Install-Payload → `.env`: `AI_PROVIDER`, `OLLAMA_BASE_URL`, `OLLAMA_MODEL`,
  `AI_API_KEY`, `AI_BASE_URL`. `engine.go` ist eine **Whitelist**
  (`RecoverEnvFromDocker`, engine.go:86 ff.) ohne Embedding-Keys; `SetupConfig` kein
  Embedding-Feld; `settings.go` kein `EmbeddingModel`; `ImportSettingsToEnv` baut den
  SetupConfig **Feld für Feld** (settings.go:235 ff.) — ohne explizite Zuweisung wird
  ein Importwert verworfen. Es gibt kein `LoadConfig` — relevant sind
  `ReadExistingConfig` (config.go:70) und der Recovery-SetupConfig in
  `DetectExistingInstallation` (config.go:138–147). `RecoverEnvFromDocker` liest die
  **Container-Env** — dort steht `OLLAMA_EMBEDDING_MODEL` oft nur aus dem
  Compose-Default, nicht aus einer Nutzerwahl.
- `templates.go` (Launcher-Compose-Template) hat **keine** Embedding-Env; die
  Repo-`docker-compose.yml` hat `AI_EMBEDDING_MODEL=${AI_EMBEDDING_MODEL:-}` (Zeile 39,
  korrekt) aber `OLLAMA_EMBEDDING_MODEL=${OLLAMA_EMBEDDING_MODEL:-qwen3-embedding:
  0.6b}` (Zeile 38, Problem: `:-` behandelt **leer wie ungesetzt**). `EnsureComposeAnd
  CaddyFiles` (templates.go:154) überschreibt eine **vorhandene** docker-compose.yml
  nicht — „Konfiguration anpassen" schreibt nur `.env`. `ApplyStagedFiles`
  (updater.go:285) überschreibt bei Updates die Launcher-Dateien mit den Repo-Dateien.
- Backend (SP3): `AI_EMBEDDING_MODEL` (→ `OLLAMA_EMBEDDING_MODEL` Alias), Provider
  `ollama` + OpenAI-kompatibel; `anthropic`/`deepseek` → `ErrEmbeddingsUnsupported`;
  leerer OpenAI-Modellname → `ErrEmbeddingModelNotConfigured` **ohne HTTP-Call**.
  `ai.EmbeddingDimensions = 1024`, `ai.DefaultOllamaEmbeddingModel =
  "qwen3-embedding:0.6b"` (beide exportiert), Spalte `vector(1024)` (Migration 00015).
  `.env.example` ist korrekt (`AI_EMBEDDING_MODEL` leer, Default nur an der
  `OLLAMA_EMBEDDING_MODEL`-Zeile) — **wird nicht geändert**.
- Bestehende Probe-Tests (`ai_probe_test.go:29` ff.) bedienen nur den Chat-Pfad und
  werden mit den neuen Overall-Success-Semantiken rot → müssen aktualisiert werden.

## Design

### Probe ruft das Gateway auf — mit reinem Konstruktor (Blocker 1 + 2, Rev. 3)

Der Launcher **reimplementiert keine Embedding-HTTP-Logik**, aber `NewGateway` wird für
den Probe in einen reinen Konstruktor verwandelt: neues Feld
`GatewayConfig.DisableEnvFallback bool` (nur der Launcher-Probe setzt es). Mit dem Flag:

- werden **alle** env-Backfills ignoriert (`AI_EMBEDDING_MODEL`,
  `OLLAMA_EMBEDDING_MODEL`, `AI_API_KEY`, `AI_MODEL`/`OPENAI_MODEL` **und**
  `OLLAMA_TIMEOUT_SECONDS` — das Flag erzwingt 30 s);
- gilt: `EmbeddingModel` wird vom Launcher **explizit** gesetzt:
  - Formularwert nicht leer → genau dieser Wert.
  - Ollama und leer → `ai.DefaultOllamaEmbeddingModel` (das ist der Default, kein
    Fehler — der Probe prüft den realen Standard).
  - OpenAI-kompatibel und leer → **Pre-Gate vor dem Gateway-Call**: ehrlicher Fehler
    `ErrEmbeddingModelNotConfigured`-Semantik („`AI_EMBEDDING_MODEL` ist nicht
    konfiguriert") **ohne HTTP-Call** — der Env-Fallback des Gateways würde sonst
    trotzdem einen Call auslösen.
- gilt: `APIKey` = Formularwert (leer bleibt leer).
- **URL-Mapping** — eine Eingabe, zwei Gateway-Felder:
  - Provider `ollama` → `GatewayConfig.OllamaBaseURL` = eingegebene URL, getrimmt
    (`TrimRight "/"`), **kein** `TranslateHostForDocker` (das bleibt in der
    Env-/Compose-Schicht).
  - Provider `openai`/`mistral`/`nebius`/OpenAI-kompatibel →
    `GatewayConfig.AIBaseURL` = eingegebene URL, getrimmt.
  Ohne das Mapping würde Ollama trotz eingegebenem Port auf `localhost:11434` und
  OpenRouter/Groq/vLLM auf `api.openai.com` geprüft.
- Der Probe ruft `ai.NewGateway(cfg).ValidateEmbeddingModel(ctx)` auf und erbt damit
  beide Ollama-Response-Shapes, `ErrEmbeddingModelNotConfigured`,
  `ErrEmbeddingsUnsupported` (Providername im Text) und die Dimensionsprüfung gegen
  `ai.EmbeddingDimensions` (1024).

**Der Chat-Probe bleibt der eigene HTTP-Client** (nicht `NewGateway.Generate`): mit
denselben 30 s wie der Embedding-Teil. Das URL-Mapping gilt nur für den
Embedding-Validierungspfad.

### Overall-Success nur wenn Chat UND Embedding OK

- `success` des Probes = Chat OK **und** Embedding OK; `testAIConnection` zeigt sonst
  ❌ rot.
- Der Launcher wickelt den Gateway-Fehler in `errors.Is(err,
  ai.ErrEmbeddingDimensionMismatch)` — der Gateway-Text enthält die Konsequenz nicht,
  der Probe-Text schon: „`<Modell>` liefert N Dimensionen, Spalte erwartet 1024 —
  **crm-server wird mit dieser Konfiguration nicht starten** (Resize-Migration nötig)".
  Nur der Mismatch ist im Server `log.Fatalf`; unerreichbar ist dort Warnung — der
  Probe bildet dieselbe Semantik ab.
- Weitere ehrliche Fehlerfälle: nicht erreichbar, Status != 200, kein Vektor,
  Modell fehlt (`AI_EMBEDDING_MODEL`-Hinweis), Provider ohne Embedding-API.

### UI (`internal/launcher/ui`)

In der bestehenden KI-Box ein zweites Feld „Embedding-Modell" (`ai-embedding-model`):

| Provider | Verhalten |
|---|---|
| `ollama` | Vorbefüllt `qwen3-embedding:0.6b` (lokal verfügbar, 1024-dim, verifiziert) |
| `openai` | Leeres Feld, Placeholder **nur native-1024-Beispiele**: `mistral-embed` („beliebiges 1024-dim Modell"). `text-embedding-3-small` (1536) und `Qwen3-Embedding-8B` (4096) sind keine Beispiele — das Gateway sendet keinen `dimensions`-Parameter, solche Modelle würden Probe und Serverstart killen. |
| `none` | Feld wird **geleert und nicht mitgeschrieben** (Payload ohne Embedding-Wert; `.env`-Generierung schreibt die beiden Embedding-Zeilen bei `none` nicht). |

Chat-Felder/Defaults: `onAIProviderChange` und `templates.go` bleiben wie gehabt
(`gemma4:12b`); das inkonsistente HTML-Feld-Default `gemma2:12b` wird auf `gemma4:12b`
korrigiert.

### Verdrahtung (vollständig)

1. **`SetupConfig`** (`config.go`): neues Feld `AIEmbeddingModel` (JSON
   `ai_embedding_model`) — `handleSetup` bindet es über das bestehende
   JSON-Decoding automatisch mit.
2. **`app.js`**: `startSetup` und `testAIConnection` senden `ai_embedding_model` bzw.
   `embedding_model`; `onAIProviderChange` setzt das Embedding-Default je Provider.
3. **Reihenfolge beim Laden gespeicherter Werte** (`editExistingConfig`,
   `loadSettingsFromFile`): zuerst Provider setzen und `onAIProviderChange()` aufrufen,
   **danach** das gespeicherte Embedding-Modell setzen — sonst überschreibt der
   Provider-Default den Importwert (gleiches Ordnungsmuster wie beim bestehenden
   `ai_model`).
4. **`engine.go` Whitelist**: `RecoverEnvFromDocker` bekommt **beide** Keys
   (`AI_EMBEDDING_MODEL` **und** `OLLAMA_EMBEDDING_MODEL`) — sonst sieht der Recovery
   den Alias nicht und Punkt 5 liest ins Leere.
5. **`.env`-Generierung** (`GenerateEnvContentWithExisting`): **provider-scoped**
   Auflösung:
   - OpenAI-kompatibel: `cfg.AIEmbeddingModel` (nicht leer) → `AI_EMBEDDING_MODEL`
     (nicht leer) → **leer**. **Kein** `OLLAMA_EMBEDDING_MODEL`-Fallback — der
     Container-Alias stammt oft nur aus dem Compose-Default, nicht aus einer
     Nutzerwahl; ihn zu erben würde genau den qwen3-Leak schreiben, den die leeren
     Compose-Defaults verhindern sollen.
   - Ollama: `cfg.AIEmbeddingModel` → `AI_EMBEDDING_MODEL` →
     `OLLAMA_EMBEDDING_MODEL` → leer (Alias **nur bei Ollama** erben).
   Die `AI_EMBEDDING_MODEL`-Zeile wird **immer** geschrieben (Wert darf leer sein);
   die `OLLAMA_EMBEDDING_MODEL`-Zeile nur, wenn der aufgelöste Wert nicht leer ist —
   eine Bestandsinstallation mit nur `OLLAMA_EMBEDDING_MODEL` funktioniert auch nach
   einem Rewrite weiter. Bei Provider `none` werden beide Embedding-Zeilen **nicht**
   geschrieben.
6. **Bestandslesen**: `ReadExistingConfig` **und** der Recovery-SetupConfig in
   `DetectExistingInstallation` lesen beide Keys mit derselben provider-scoped Regel —
   sonst liefert `/api/status` das Feld nicht und `editExistingConfig` setzt ins Leere.
7. **`settings.go`**: `AISettings` um `EmbeddingModel` erweitern; Export schreibt
   `AI_EMBEDDING_MODEL` (Klartext, kein Secret); Import liest beide Keys (AI gewinnt)
   **und** `ImportSettingsToEnv` weist `AIEmbeddingModel` explizit im SetupConfig zu
   (settings.go:235 ff. — sonst verwirft der Import den Wert);
   `ValidateSettingsValues` prüft `ai.embedding_model` auf Steuerzeichen
   (.env-Injection).
8. **Leere Defaults an beiden Compose-Dateien**: `templates.go` bekommt am
   Server-Block `AI_EMBEDDING_MODEL=${AI_EMBEDDING_MODEL:-}` und
   `OLLAMA_EMBEDDING_MODEL=${OLLAMA_EMBEDDING_MODEL:-}`; die Repo-`docker-compose.yml`
   Zeile 38 wird von `:-qwen3-embedding:0.6b` auf `:-` (leer) geändert. Begründung:
   `:-` behandelt leer wie ungesetzt, das `qwen3`-Default an der Alias-Zeile schickt
   bei OpenAI-Installationen qwen3 an OpenAI, und `ApplyStagedFiles` überschreibt das
   Launcher-Template ohnehin mit der Repo-Compose. Ollama fällt danach im Gateway auf
   `DefaultOllamaEmbeddingModel` zurück; OpenAI ohne Modell erhält den ehrlichen
   Config-Fehler. Worker-Block: keine Embedding-Env (Worker nutzt keine Embeddings).
9. **Bestehende Compose patchen** (Rev.-3-Lücke): `EnsureComposeAndCaddyFiles`
   überschreibt eine vorhandene docker-compose.yml nicht — die neue Funktion
   (templates.go) ersetzt in einer **vorhandenen** docker-compose.yml **ausschließlich**
   Embedding-Zeilen, die das hartkodierte `qwen3`-Default enthalten, durch die
   leere-Default-Form (idempotent, nur diese zwei Zeilen, Server-Block). Fehlende
   Embedding-Zeilen bleiben fehlend — das ist der sichere Zustand (ohne Env-Zeile
   fällt der Gateway-Konstruktor ehrlich auf Default bzw. Config-Fehler zurück). Läuft
   nach `EnsureComposeAndCaddyFiles` beim Setup und bei „Konfiguration anpassen".
10. **`AIProbeRequest`**: `EmbeddingModel string json:"embedding_model"` —
    `handleAITest` decodiert sonst leer. (Der Setup-Payload bleibt
    `ai_embedding_model`.)

### Timeout-Vertrag (Blocker 4)

- `DisableEnvFallback` stoppt auch `OLLAMA_TIMEOUT_SECONDS`: Probe-Gateway erzwingt
  30 s (kein Env-Hijack des Timeouts).
- Chat-Probe-Client (ai_probe.go:48/:59): 5 s/6 s → **30 s**.
- Startcheck in `cmd/server/main.go`: Context-Cap **30 s fix** — nicht der
  Generierungs-Timeout (`OLLAMA_TIMEOUT_SECONDS`), denn ein unbegrenzter
  Generierungs-Timeout darf den Boot nie blocken (der Check läuft vor
  `ListenAndServe`). Mismatch bleibt `log.Fatalf`, Timeout bleibt Warnung; 30 s
  deckt kalte Modell-Loads, die ein grüner Probe akzeptiert hat.
- Konsistenz (Hinweis): ein grüner Probe (30 s) garantiert den Kaltstart nicht
  hundertprozentig, aber Probe und Startcheck teilen jetzt dieselbe Grenze und
  derselbe Fehler ist nicht mehr „falsch rot" sondern echt.

## Fehlerbehandlung

Kein Fake-Success: Probe schlägt bei jedem Problem ehrlich fehl (Verbindung, Status,
Shape, Dimension, fehlendes Modell, Provider ohne Embedding-API). Ehrliche Texte inkl.
expliziter Konsequenz bei Dimensionsmismatch (crm-server startet nicht).

## Teststrategie

- `ai_probe_test.go`: bestehende Chat-Tests aktualisieren (Fake-Ollama muss auch
  `/api/embeddings` bedienen bzw. der Embedding-Modell-Value wird gesetzt);
  Embedding-OK (1024), Dimensionsmismatch (inkl. Konsequenz-Text), nicht erreichbar,
  Modell fehlt (OpenAI-kompatibel, **kein HTTP-Call** — Request-Zähler-Assertion),
  Ollama mit leerem Feld → Default-Modell geprüft; kein Launcher-Env-Einfluss
  (`DisableEnvFallback`-Test: Env gesetzt, Konfigfeld leer → kein Fallback);
  URL-Mapping-Tests (eingegebener Port landet bei `OllamaBaseURL` bzw. `AIBaseURL`);
  Response-Shape rückwärtskompatibel; Chat-Probe-Timeout 30 s.
- `internal/ai`: `DisableEnvFallback`-Semantik (alle Felder inkl.
  `OLLAMA_TIMEOUT_SECONDS`: Env gesetzt, Konfigfeld leer → kein Backfill, 30 s
  erzwungen; Konfigfeld gesetzt → gewinnt).
- `config_test.go`: `.env`-Generierung (Formularwert gewinnt; provider-scoped
  Alias-Erben — openai ohne Formularwert/AI-Key schreibt **leer**, nicht qwen3;
  Alias-Erhalt bei Rewrite für Bestandsinstallationen; `none` schreibt keine
  Embedding-Zeilen), `ReadExistingConfig`/Recovery lesen beide Keys.
- `settings_test.go`: Export/Import-Round-Trip inkl. `EmbeddingModel`, Zuweisung in
  `ImportSettingsToEnv`, `ai.embedding_model`-Validierung (Steuerzeichen-Abweisung).
- `templates.go`/Compose-Patch: vorhandene Compose mit `:-qwen3...`-Zeilen → gepatcht
  auf leere Defaults; fehlende Zeilen bleiben fehlend; Repo-`docker-compose.yml`
  Zeile 38 geändert.
- `cmd/server/main.go`: Startcheck-Cap 30 s (Timeout-Env wird nicht konsultiert).
- Live-Verifikation: Launcher-UI lokal starten, mit lokalem Ollama
  (`qwen3-embedding:0.6b`) den ✅-Fall; Mismatch-Fall mit bewusst falsch
  dimensioniertem Modell; Bestandsinstallation (nur `OLLAMA_EMBEDDING_MODEL`) →
  Rewrite erhält das Modell; `none`-Provider ohne Embedding-Spur; alte Compose mit
  qwen3-Default wird beim Setup gepatcht.

## Ausdrücklich out of scope

- Chat-Provider-Erweiterung (Claude/DeepSeek/Nebius/Mistral) — Chat-Auswahl bleibt
  unverändert (Nutzerentscheidung).
- `.env.example` bleibt unverändert (bereits korrekt).
- Re-Embed-Job, Dimensions-Resize-Mechanik im Launcher (Hinweis-Text genügt).
- CLI-Default `gemma2:9b` in `cmd/setup-launcher/cli.go`.
- Leaf-Package-Extraktion des Embedding-Clients (späterer Refactor, Import akzeptiert).

## Korrekturprotokoll

**Rev. 1 → Rev. 2:** Probe nachgebaut → Gateway-Call; Overall-Success = Chat UND
Embedding OK; `.env.example`-Änderung gestrichen; Alias-Erhalt; Placeholder auf
native-1024; Ist-Zustand präzisiert; Timeout 30 s; Probe ohne
`TranslateHostForDocker`.

**Rev. 2 → Rev. 3:** Env-Fallback umgangen (reiner Konstruktor, Pre-Gate, explizite
Felder); URL-Mapping; Auflösungsreihenfolge um Formularwert erweitert; leere
Compose-Defaults in beiden Dateien; `AIProbeRequest`-Tag; `errors.Is`-Wicklung;
Startcheck-Timeout; `none`-Verhalten; bestehende Probe-Tests.

**Rev. 3 → Rev. 4 (dieser Review):**
1. Alias-Auflösung **provider-scoped**: OpenAI erbt `OLLAMA_EMBEDDING_MODEL` **nicht**
   (Formularwert → `AI_EMBEDDING_MODEL` → leer); Ollama-Alias nur bei Ollama erben —
   der Container-Alias stammt oft nur aus dem Compose-Default, nicht aus einer
   Nutzerwahl.
2. Recovery-Whitelist (`engine.go:RecoverEnvFromDocker`) bekommt **beide**
   Embedding-Keys; sonst liest der Recovery den Alias nicht.
3. **Bestehende Compose wird gepatcht**: `EnsureComposeAndCaddyFiles` überschreibt
   vorhandene Compose-Dateien nicht; neue idempotente Patch-Funktion ersetzt nur die
   `qwen3`-Default-Embedding-Zeilen durch leere Defaults (fehlende Zeilen = sicherer
   Zustand, bleiben fehlend); läuft beim Setup und bei „Konfiguration anpassen".
4. Timeout-Vertrag präzisiert: `DisableEnvFallback` stoppt auch
   `OLLAMA_TIMEOUT_SECONDS` (30 s erzwungen); Chat-Probe-Client auf 30 s; Startcheck
   30 s fix (nicht Generierungs-Timeout — unbegrenzter Generierungs-Timeout darf den
   Boot nie blocken).
5. Festgenagelt: Chat-Probe bleibt eigener HTTP-Client (nicht
   `NewGateway.Generate` — `callOpenAI` nimmt `OllamaBaseURL` und liest
   `AI_MODEL`/`OPENAI_MODEL` am Flag vorbei); URL-Mapping gilt nur für
   `ValidateEmbeddingModel`; `ImportSettingsToEnv` weist `AIEmbeddingModel` explizit
   zu; `AIProbeRequest`-Tag `embedding_model`.
6. Launcher-Binary: `internal/ai`-Import akzeptiert (Nutzerentscheidung).
