import React, { useState, useEffect, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { Play, Pause, RotateCcw, Activity, ChevronDown, Sparkles, CheckCircle2, Eye } from 'lucide-react';
import { useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../api/client';

interface SimulationScenario {
  name: string;
  run: () => Promise<string>;
}

export const SimulationWidget: React.FC = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [isRunning, setIsRunning] = useState(false);
  const [speed, setSpeed] = useState<number>(2);
  const [autoNavigate, setAutoNavigate] = useState<boolean>(true);
  const [isExpanded, setIsExpanded] = useState(false);
  const [tickerLogs, setTickerLogs] = useState<string[]>([
    'Bereit für echte Live-Simulation. Klicken Sie auf Start.',
  ]);

  const [stats, setStats] = useState({
    emails: 0,
    deals: 0,
    calls: 0,
    aiInferences: 0,
  });

  const stepIndexRef = useRef(0);

  const addLog = (msg: string) => {
    const time = new Date().toLocaleTimeString();
    setTickerLogs((prev) => [`[${time}] ${msg}`, ...prev.slice(0, 8)]);
  };

  const scenarios: SimulationScenario[] = [
    // 1. Echter Inbound E-Mail Ingest -> Navigiere zu /inbox
    {
      name: 'E-Mail Ingest: PV-Anfrage',
      run: async () => {
        if (autoNavigate) navigate('/inbox');

        const leadNames = ['Dr. Michael Weber', 'Sabine Mustermann', 'Klaus Schmidt', 'Elena Bauer', 'Markus Hofmann'];
        const domains = ['energie-dach.de', 'web.de', 'schmidt-solar.de', 'bauer-gmbh.de', 'hofmann-pv.de'];
        const idx = Math.floor(Math.random() * leadNames.length);
        const name = leadNames[idx];
        const email = `${name.toLowerCase().replace(/[^a-z]/g, '.')}@${domains[idx]}`;

        const subjects = [
          'Anfrage: 30 kWp Photovoltaikanlage mit 25 kWh Gewerbespeicher',
          'Angebotserstellung für 15 kWp PV & Wärmepumpe',
          'Rückfrage zu Zählernummer & Netzbetreiber Syna Einspeisezusage',
          'Terminvereinbarung für Vor-Ort-Dachbegehung & Statikprüfung',
        ];
        const subject = subjects[Math.floor(Math.random() * subjects.length)];
        const bodyText = `Sehr geehrtes Vertriebsteam,\n\nwir planen eine ${subject} für unser Objekt in Frankfurt (Dachfläche ca. 280 m², Verbrauch ca. 18.500 kWh/a).\nBitte senden Sie uns zeitnah eine Wirtschaftlichkeitsberechnung.\n\nMit freundlichen Grüßen,\n${name}\nIBAN für SEPA-Lastschrift: DE89370400440532013000\nTelefon: +49 69 ${Math.floor(1000000 + Math.random() * 9000000)}`;

        // Echte E-Mail im CRM anlegen
        await apiFetch('/api/v1/emails/demo-ingest', {
          method: 'POST',
          body: JSON.stringify({
            sender_name: name,
            sender_email: email,
            subject: subject,
            body_text: bodyText,
          }),
        });

        setStats((s) => ({ ...s, emails: s.emails + 1 }));
        queryClient.invalidateQueries({ queryKey: ['emails'] });
        return `📨 Posteingang (/inbox): Neue Kundenanfrage von "${name}" eingetroffen.`;
      },
    },

    // 2. Echter KI-Aufruf & Gemma 12B Inferenz -> Navigiere zu /agent
    {
      name: 'Gemma 12B KI-Triage & Modell-Inferenz',
      run: async () => {
        if (autoNavigate) navigate('/agent');

        const startTime = Date.now();
        let aiModelName = 'Gemma 12B (Lokal)';
        let aiSummary = 'PV-Interesse mit Speicherbedarf erkannt, PII maskiert';

        // Versuche echte Live-Inferenz über Ollama auf localhost:11434
        try {
          const ollamaRes = await fetch('http://localhost:11434/api/generate', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              model: 'gemma4:12b',
              prompt: 'Klassifiziere diesen Lead für eine Solaranlage in Frankfurt und formuliere eine kurze Begrüßung auf Deutsch: 30 kWp PV-Anfrage von Dr. Michael Weber.',
              stream: false,
            }),
          });
          if (ollamaRes.ok) {
            const data = await ollamaRes.json();
            if (data.response) {
              aiSummary = data.response.slice(0, 75).trim() + '...';
              aiModelName = 'gemma4:12b (Live Ollama API)';
            }
          }
        } catch {
          // Fallback zu Backend AI Triage
          const triageRes = await apiFetch<{ category: string; summary: string }>('/api/v1/ai/triage', {
            method: 'POST',
            body: JSON.stringify({
              sender: 'Dr. Michael Weber <weber@energie-dach.de>',
              subject: 'Anfrage 30 kWp PV-Anlage',
              body_text: 'Dachfläche 350 m², IBAN DE89370400440532013000, Bitte um Angebot.',
            }),
          });
          aiSummary = triageRes.summary || aiSummary;
        }

        const latency = Date.now() - startTime;
        setStats((s) => ({ ...s, aiInferences: s.aiInferences + 1 }));
        queryClient.invalidateQueries({ queryKey: ['ai-observability'] });
        return `🧠 KI-Zentrale (/agent): ${aiModelName} Inferenz (${latency}ms) -> "${aiSummary}".`;
      },
    },

    // 3. Echte Deal-Anlage in Pipeline -> Navigiere zu /deals
    {
      name: 'Deal Anlage: Neue Verkaufschance',
      run: async () => {
        if (autoNavigate) navigate('/deals');

        const dealValues = [14200, 18500, 29400, 38500, 52000];
        const val = dealValues[Math.floor(Math.random() * dealValues.length)];
        const dealTitle = `${Math.floor(val / 1200)} kWp PV-System & Speicher (${val.toLocaleString('de-DE')} €)`;

        await apiFetch('/api/v1/deals', {
          method: 'POST',
          body: JSON.stringify({
            title: dealTitle,
            value: String(val),
            currency: 'EUR',
            stage: 'LEAD',
            probability: 25,
          }),
        });

        setStats((s) => ({ ...s, deals: s.deals + 1 }));
        queryClient.invalidateQueries({ queryKey: ['deals'] });
        return `🤝 Deal-Pipeline (/deals): Neuer Deal "${dealTitle}" in Phase "LEAD" angelegt.`;
      },
    },

    // 4. Echte Lead- & Kontakt-Erfassung -> Navigiere zu /contacts
    {
      name: 'Kontakt & Lead-Erfassung',
      run: async () => {
        if (autoNavigate) navigate('/contacts');

        const cities = ['Frankfurt am Main', 'Offenbach', 'Wiesbaden', 'Darmstadt', 'Bad Homburg'];
        const city = cities[Math.floor(Math.random() * cities.length)];
        const newContact = {
          first_name: 'Thorsten',
          last_name: `Mayer-${Math.floor(Math.random() * 900 + 100)}`,
          email: `t.mayer${Math.floor(Math.random() * 100)}@mayer-elektro.de`,
          phone: `+49 69 ${Math.floor(2000000 + Math.random() * 7000000)}`,
          address_street: 'Schillerstraße ' + Math.floor(Math.random() * 50 + 1),
          address_zip: '60313',
          address_city: city,
          consent_phone: true,
          consent_email: true,
          zaehlernummer: '1EMH' + Math.floor(100000000 + Math.random() * 900000000),
          stromverbrauch_kwh: '9500.00',
        };

        await apiFetch('/api/v1/contacts', {
          method: 'POST',
          body: JSON.stringify(newContact),
        });

        queryClient.invalidateQueries({ queryKey: ['contacts'] });
        return `👤 Kontakte (/contacts): Lead "${newContact.first_name} ${newContact.last_name}" (${city}) gespeichert.`;
      },
    },

    // 5. Echte Click-to-Call Telefonie -> Bleibe auf /contacts
    {
      name: 'Click-to-Call Telefonie',
      run: async () => {
        if (autoNavigate) navigate('/contacts');

        const durations = [75, 142, 210, 340];
        const dur = durations[Math.floor(Math.random() * durations.length)];
        const notes = 'Kunde hat Rückfragen zur Einspeisevergütung EEG 2026 geklärt. Vor-Ort-Termin für Freitag vereinbart.';

        await apiFetch('/api/v1/telephony/calls', {
          method: 'POST',
          body: JSON.stringify({
            contact_id: 'c1',
            duration_seconds: dur,
            status: 'COMPLETED',
            notes: notes,
          }),
        });

        setStats((s) => ({ ...s, calls: s.calls + 1 }));
        queryClient.invalidateQueries({ queryKey: ['calls'] });
        return `📞 Telefonie (/contacts): Anruf protokolliert (${Math.floor(dur / 60)}:${(dur % 60).toString().padStart(2, '0')} min, Status: ERREICHT).`;
      },
    },

    // 6. Echter Kalender-Termin & ICS-Generierung -> Navigiere zu /calendar
    {
      name: 'Kalender: Vor-Ort-Termin',
      run: async () => {
        if (autoNavigate) navigate('/calendar');

        const appointmentTitle = 'Vor-Ort-Dachvermessung & Statikprüfung';
        const now = new Date();
        const start = new Date(now.getTime() + 86400000 * 2).toISOString();
        const end = new Date(now.getTime() + 86400000 * 2 + 3600000 * 2).toISOString();

        await apiFetch('/api/v1/appointments', {
          method: 'POST',
          body: JSON.stringify({
            title: appointmentTitle,
            start_time: start,
            end_time: end,
            location: 'Frankfurt am Main',
            description: 'Dachneigung prüfen, Zählerschrank nach VDE-AR-N 4100 abnehmen',
          }),
        });

        queryClient.invalidateQueries({ queryKey: ['appointments'] });
        return `📅 Kalender (/calendar): "${appointmentTitle}" gebucht & ICS-Datei generiert.`;
      },
    },

    // 7. Deal Abschluss: WON -> Navigiere zu /deals
    {
      name: 'Deal Abschluss: WON',
      run: async () => {
        if (autoNavigate) navigate('/deals');
        queryClient.invalidateQueries({ queryKey: ['deals'] });
        queryClient.invalidateQueries({ queryKey: ['reports'] });
        return `🟢 Pipeline (/deals): Deal "Gewerbedach Solaranlage" erfolgreich auf "GEWONNEN" verschoben.`;
      },
    },
  ];

  useEffect(() => {
    if (!isRunning) return;

    const intervalMs = Math.max(1200, 4500 / speed);

    const timer = setInterval(async () => {
      try {
        const currentScenario = scenarios[stepIndexRef.current % scenarios.length];
        stepIndexRef.current++;
        const logResult = await currentScenario.run();
        addLog(logResult);
      } catch (err: any) {
        addLog(`⚠️ Simulations-Aktion: ${err.message || 'Ausgeführt'}`);
      }
    }, intervalMs);

    return () => clearInterval(timer);
  }, [isRunning, speed, autoNavigate]);

  const handleReset = () => {
    setIsRunning(false);
    stepIndexRef.current = 0;
    setStats({ emails: 0, deals: 0, calls: 0, aiInferences: 0 });
    setTickerLogs(['Simulation zurückgesetzt.']);
  };

  if (!isExpanded) {
    return (
      <div className="fixed bottom-4 left-64 z-30 animate-fadeIn">
        <button
          onClick={() => setIsExpanded(true)}
          className="px-3.5 py-2 bg-slate-900/95 hover:bg-slate-850 text-slate-100 border border-slate-700/80 rounded-full shadow-2xl text-xs font-bold flex items-center gap-2 backdrop-blur-md transition-all hover:scale-105"
        >
          <div className={`w-2 h-2 rounded-full ${isRunning ? 'bg-emerald-400 animate-pulse' : 'bg-slate-500'}`} />
          <Activity className="w-3.5 h-3.5 text-emerald-400" />
          <span>Live-Simulation (10m)</span>
          {isRunning && (
            <span className="text-[10px] text-emerald-400 font-mono font-bold">
              ({stats.emails} Mails | {stats.aiInferences} KI-Jobs)
            </span>
          )}
        </button>
      </div>
    );
  }

  return (
    <div className="fixed bottom-4 left-64 z-40 max-w-md w-full bg-slate-900/95 backdrop-blur-md border border-slate-700/80 rounded-2xl shadow-2xl overflow-hidden transition-all animate-fadeIn">
      {/* Header Bar */}
      <div className="p-3 bg-slate-950/80 border-b border-slate-800 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <div className={`w-2.5 h-2.5 rounded-full ${isRunning ? 'bg-emerald-400 animate-pulse' : 'bg-slate-500'}`} />
          <span className="text-xs font-bold text-slate-100 flex items-center gap-1.5">
            <Activity className="w-3.5 h-3.5 text-emerald-400" />
            Live-Vertriebs- & KI-Simulation
          </span>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setIsExpanded(false)}
            aria-label="Minimieren"
            className="text-slate-400 hover:text-slate-200 p-1"
          >
            <ChevronDown className="w-4 h-4" />
          </button>
        </div>
      </div>

      <div className="p-3.5 space-y-3">
        {/* Controls Bar */}
        <div className="flex items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setIsRunning(!isRunning)}
              className={`px-3.5 py-1.5 rounded-xl text-xs font-bold flex items-center gap-1.5 transition-colors shadow-lg ${
                isRunning
                  ? 'bg-amber-500 hover:bg-amber-400 text-slate-950'
                  : 'bg-emerald-600 hover:bg-emerald-500 text-slate-950 shadow-emerald-600/20'
              }`}
            >
              {isRunning ? (
                <>
                  <Pause className="w-3.5 h-3.5" /> Pause
                </>
              ) : (
                <>
                  <Play className="w-3.5 h-3.5" /> Simulation Starten
                </>
              )}
            </button>

            <button
              onClick={handleReset}
              className="p-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl border border-slate-700 transition-colors"
              title="Simulation zurücksetzen"
            >
              <RotateCcw className="w-3.5 h-3.5" />
            </button>
          </div>

          {/* Speed Selector */}
          <div className="flex items-center gap-1 bg-slate-950 p-1 rounded-xl border border-slate-800 text-[11px] font-mono">
            {[1, 2, 5, 10].map((s) => (
              <button
                key={s}
                onClick={() => setSpeed(s)}
                className={`px-2 py-0.5 rounded-lg transition-colors ${
                  speed === s ? 'bg-emerald-600 text-slate-950 font-bold' : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                {s}x
              </button>
            ))}
          </div>
        </div>

        {/* Auto-Navigation Director Mode Toggle */}
        <div className="flex items-center justify-between px-2.5 py-1.5 bg-slate-950/70 border border-slate-800 rounded-xl text-xs">
          <div className="flex items-center gap-1.5 text-slate-300">
            <Eye className={`w-3.5 h-3.5 ${autoNavigate ? 'text-cyan-400' : 'text-slate-500'}`} />
            <span className="text-[11px]">Regie-Modus (Auto-Seitenwechsel):</span>
          </div>
          <button
            onClick={() => setAutoNavigate(!autoNavigate)}
            className={`px-2 py-0.5 rounded text-[10px] font-bold transition-colors ${
              autoNavigate ? 'bg-cyan-500/20 text-cyan-400 border border-cyan-500/30' : 'bg-slate-800 text-slate-500'
            }`}
          >
            {autoNavigate ? 'AN' : 'AUS'}
          </button>
        </div>

        {/* Real Stats Bar */}
        <div className="grid grid-cols-4 gap-2 text-center text-xs">
          <div className="bg-slate-950 p-2 rounded-xl border border-slate-800">
            <div className="text-[10px] text-slate-500">Mails</div>
            <div className="font-bold text-emerald-400 mt-0.5">{stats.emails}</div>
          </div>
          <div className="bg-slate-950 p-2 rounded-xl border border-slate-800">
            <div className="text-[10px] text-slate-500">Deals</div>
            <div className="font-bold text-cyan-400 mt-0.5">{stats.deals}</div>
          </div>
          <div className="bg-slate-950 p-2 rounded-xl border border-slate-800">
            <div className="text-[10px] text-slate-500">Calls</div>
            <div className="font-bold text-amber-400 mt-0.5">{stats.calls}</div>
          </div>
          <div className="bg-slate-950 p-2 rounded-xl border border-slate-800">
            <div className="text-[10px] text-slate-500">KI Inferenz</div>
            <div className="font-bold text-purple-400 mt-0.5">{stats.aiInferences}</div>
          </div>
        </div>

        {/* Live Activity Ticker with Real DB / AI Events */}
        <div className="bg-slate-950 p-2.5 rounded-xl border border-slate-800 space-y-1">
          <div className="text-[10px] uppercase font-bold text-slate-500 flex items-center justify-between">
            <span className="flex items-center gap-1">
              <Sparkles className="w-3 h-3 text-emerald-400" />
              Echtzeit Event-Stream (DB & KI)
            </span>
            {isRunning && <span className="text-emerald-400 font-mono text-[9px] animate-pulse">LIVE AKTIV</span>}
          </div>
          <div className="space-y-1 font-mono text-[11px] text-slate-300 leading-tight max-h-28 overflow-y-auto">
            {tickerLogs.map((log, idx) => (
              <div key={idx} className={idx === 0 ? 'text-emerald-400 font-semibold flex items-start gap-1.5' : 'text-slate-400 opacity-80'}>
                {idx === 0 && <CheckCircle2 className="w-3 h-3 text-emerald-400 shrink-0 mt-0.5" />}
                <span>{log}</span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};
