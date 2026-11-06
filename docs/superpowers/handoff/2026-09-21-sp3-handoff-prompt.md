# SP3 Handoff Prompt

Copy/paste this as the opening message for the next agent session.

---

Wir arbeiten an der Umsetzung von docs/superpowers/plans/2026-09-21-fake-feature-elimination-plan.md
basierend auf der Spezifikation docs/superpowers/specs/2026-09-21-fake-feature-elimination-design.md.

Status:
- Repo: openlocalcrm, main ist aktuell auf Commit 726a867 (SP1 Security + SP2 E-Mail Full-Scope
  sind vollständig gemerged via PR #20 und #21).
- Fortschritts-Ledger: .superpowers/sdd/progress.md (lokal, .gitignore'd — vor Beginn lesen für
  vollen Kontext zu SP1/SP2 inkl. eines bereits aufgetretenen und gelösten Zwischenfalls).

Nächster Schritt: Starte mit Phase 3: SP3 (AI Truthfulness & Embeddings, Findings A1–A9).

1. Branch von main erstellen: fix/sp3-ai-truth
2. Task 3.1 (A1–A5): Fakes aus internal/ai/gateway.go (simulateGemmaResponse) und
   internal/ai/chat.go (generateSmartFallback, isGenericSimulatedReply) entfernen; bei
   LLM-Fehler ehrliches ErrUpstreamUnavailable / HTTP 502 statt Canned-Text. SynthesizeNotes
   in handlers/notes.go muss echte Notizen einlesen und gateway.Generate aufrufen.
3. Task 3.2 (A9): Migration für vector(768) + HNSW-Index auf knowledge_base_articles
   (nächste freie Nummer verwenden — NICHT 00009 wie im Plan-Dokument literal steht, das ist
   veraltet; aktuell ist 00013 die letzte Migration, also 00014 verwenden). Gateway.GenerateEmbedding
   via Ollama Embeddings API (Default-Modell nomic-embed-text), Cosinus-Distanz-Suche in ai.sql.

Wichtige Invarianten (aus AGENTS.md und dieser Session gelernt):
1. TDD: failing test zuerst schreiben, dann implementieren (siehe Tasks-Steps im Plan).
2. Migrations-Symlink (migrations -> internal/db/migrations) muss synchron & ein echter
   Symlink bleiben — nach jedem `git status`/Commit kurz mit `ls -la migrations` prüfen.
3. Nach Änderungen an internal/db/queries/*.sql immer `$(go env GOPATH)/bin/sqlc generate`
   ausführen und internal/db/demo/querier.go (In-Memory-Querier) für neue Queries erweitern.
4. Coverage-Gate: `./scripts/coverage.sh --min 50.0 --diff 50.0` muss vor jedem Commit grün
   sein (Pre-Push-Hook erzwingt das ohnehin).
5. Keine Mocks/Dummy-Simulatoren — echte Implementierung. Bei externen Fehlern ehrlich fehlschlagen
   (HTTP 502/503), niemals err == nil mit erfundener Antwort.
6. Für UI-nahe Änderungen: lokal mit DEMO_MODE=true PORT=8080 ./bin/crm-server (Backend) +
   cd web && pnpm dev (Frontend) starten und mit Playwright MCP live durchklicken, bevor
   "fertig" gemeldet wird — Ollama muss lokal laufen (curl localhost:11434/api/tags) und
   OLLAMA_MODEL auf ein tatsächlich vorhandenes Modell zeigen (aktuell z.B. gemma4:12b,
   NICHT der Default "mistral", der hier nicht gepullt ist).
7. Nach Abschluss von SP3: Branch pushen, PR gegen main öffnen (Vorlage: .github/pull_request_template.md),
   und den Nutzer fragen, ob gemergt werden soll, bevor SP4 gestartet wird.
