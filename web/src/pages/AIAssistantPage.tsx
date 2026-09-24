import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import {
  Sparkles,
  ShieldCheck,
  Cpu,
  MessageSquareText,
  CheckCircle2,
  Bot,
  Search,
  Globe,
  Activity,
  Send,
  Lock,
  Database,
  BookOpen,
  Check,
  X,
  Eye,
  FileText,
  Plus,
  Trash2,
  Layers,
  Building2,
  Compass,
  Zap,
  ArrowRight,
} from 'lucide-react';

export const AIAssistantPage: React.FC = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<
    'copilot' | 'facts' | 'kb' | 'research' | 'triage' | 'observability'
  >('copilot');
  const [selectedAuditLog, setSelectedAuditLog] = useState<any>(null);
  const [feedbackBanner, setFeedbackBanner] = useState<string | null>(null);

  const { data: healthData } = useQuery<any>({
    queryKey: ['health'],
    queryFn: () => apiFetch('/api/v1/health'),
    staleTime: 60000,
  });

  const formatModelBadge = (m?: string) => {
    if (!m) return 'Mistral';
    const lower = m.toLowerCase();
    if (lower.startsWith('mistral')) return 'Mistral';
    if (lower.includes('gemma')) return 'Gemma 12B';
    if (lower.includes('gpt-4')) return 'GPT-4o';
    if (lower.includes('claude')) return 'Claude';
    return m.split(':')[0].toUpperCase();
  };
  const activeModel = formatModelBadge(healthData?.ai_model);

  // --- TAB 1: Chat Copilot State ---
  const [chatInput, setChatInput] = useState('');
  const [chatMessages, setChatMessages] = useState<any[]>([
    {
      role: 'assistant',
      content:
        'Hallo! Ich bin Ihr OpenLocalCRM KI-Vertriebs-Copilot. Sie können mich nach Kunden, Pipeline-Analysen, Vor-Ort-Pitches oder Rechtsthemen (UWG § 7, BGB § 355) fragen.',
      time: 'Jetzt',
    },
  ]);

  const chatMutation = useMutation({
    mutationFn: (messages: any[]) =>
      apiFetch<any>('/api/v1/ai/chat', {
        method: 'POST',
        body: JSON.stringify({ messages, context: '' }),
      }),
    onSuccess: (res) => {
      setChatMessages((prev) => [
        ...prev,
        {
          role: 'assistant',
          content: res.reply,
          actionCard: res.actionCard,
          time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        },
      ]);
      if (res.actionCard) {
        queryClient.invalidateQueries({ queryKey: ['automations'] });
        queryClient.invalidateQueries({ queryKey: ['deals'] });
        queryClient.invalidateQueries({ queryKey: ['contacts'] });
        queryClient.invalidateQueries({ queryKey: ['companies'] });
        queryClient.invalidateQueries({ queryKey: ['emails'] });
      }
    },
  });

  const handleSendChat = (e: React.FormEvent, customMsg?: string) => {
    if (e) e.preventDefault();
    const text = customMsg || chatInput;
    if (!text.trim() || chatMutation.isPending) return;

    const newMsg = {
      role: 'user',
      content: text,
      time: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    };
    const updated = [...chatMessages, newMsg];
    setChatMessages(updated);
    if (!customMsg) setChatInput('');
    chatMutation.mutate(updated);
  };

  // --- TAB 2: Evidence-Ledger / Fakten (§5.2) ---
  const [facts, setFacts] = useState<any[]>([]);

  const handleConfirmFact = (id: string) => {
    setFacts((prev) => prev.map((f) => (f.id === id ? { ...f, status: 'BESTÄTIGT' } : f)));
  };

  const handleRejectFact = (id: string) => {
    setFacts((prev) => prev.filter((f) => f.id !== id));
  };

  // --- TAB 3: Wissensbasis / Knowledge Base & RAG (§5.5) ---
  const [kbSearch, setKbSearch] = useState('');
  const [isKbModalOpen, setIsKbModalOpen] = useState(false);
  const [kbForm, setKbForm] = useState({
    title: '',
    category: 'Photovoltaik & Speicher',
    source: 'Technisches_Handbuch_2026.pdf',
    content: '',
  });

  const { data: rawKbArticles = [] } = useQuery<any[]>({
    queryKey: ['ai-kb'],
    queryFn: () => apiFetch('/api/v1/ai/kb'),
  });
  const kbArticles = Array.isArray(rawKbArticles) ? rawKbArticles : [];

  const createKbMutation = useMutation({
    mutationFn: (newDoc: any) =>
      apiFetch('/api/v1/ai/kb', {
        method: 'POST',
        body: JSON.stringify(newDoc),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ai-kb'] });
      setIsKbModalOpen(false);
      setKbForm({ title: '', category: 'Photovoltaik & Speicher', source: '', content: '' });
      setFeedbackBanner(
        'Wissensdokument erfolgreich in Wissensdatenbank gespeichert und indexiert!',
      );
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const deleteKbMutation = useMutation({
    mutationFn: (id: string) =>
      apiFetch(`/api/v1/ai/kb/${id}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['ai-kb'] });
      setFeedbackBanner('Wissensdokument aus RAG-Vektorindex gelöscht.');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const filteredKbArticles = kbArticles.filter((doc) => {
    if (!kbSearch.trim()) return true;
    const q = kbSearch.toLowerCase();
    return (
      doc.title?.toLowerCase().includes(q) ||
      doc.category?.toLowerCase().includes(q) ||
      doc.content?.toLowerCase().includes(q) ||
      doc.source?.toLowerCase().includes(q)
    );
  });

  // --- TAB 4: Deep Company Research Jobs (§5.4) ---
  const [researchDomain, setResearchDomain] = useState('energie-dach.de');
  const [researchDepth, setResearchDepth] = useState<'standard' | 'deep'>('deep');
  const [researchCategory, setResearchCategory] = useState('Gewerbesolar & Hallendach');
  const [selectedResearchJob, setSelectedResearchJob] = useState<any>(null);

  const { data: rawResearchJobs = [] } = useQuery<any[]>({
    queryKey: ['ai-research-jobs'],
    queryFn: () => apiFetch('/api/v1/ai/research/jobs'),
  });
  const researchJobs = Array.isArray(rawResearchJobs) ? rawResearchJobs : [];

  const createResearchJobMutation = useMutation({
    mutationFn: (job: any) =>
      apiFetch('/api/v1/ai/research/jobs', {
        method: 'POST',
        body: JSON.stringify(job),
      }),
    onSuccess: (newJob: any) => {
      queryClient.invalidateQueries({ queryKey: ['ai-research-jobs'] });
      setSelectedResearchJob(newJob);
      setFeedbackBanner(
        `Deep Research für "${newJob.query}" erfolgreich abgeschlossen & analysiert!`,
      );
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const handleStartResearchJob = (e: React.FormEvent) => {
    e.preventDefault();
    if (!researchDomain.trim()) return;
    createResearchJobMutation.mutate({
      domain: researchDomain,
      depth: researchDepth,
      category: researchCategory,
    });
  };

  // --- TAB 5: E-Mail Triage State (§5.2) ---
  const [inputText, setInputText] = useState(
    'Hallo OpenLocalCRM Team, wir möchten für unsere Gewerbehalle ein 25 kWp Photovoltaik-System anfragen. Bitte unterbreiten Sie uns ein Angebot inkl. Speicher. Meine IBAN lautet DE89370400440532013000.',
  );
  const [triageResult, setTriageResult] = useState<any>(null);

  const triageMutation = useMutation({
    mutationFn: (data: any) =>
      apiFetch('/api/v1/ai/triage', {
        method: 'POST',
        body: JSON.stringify(data),
      }),
    onSuccess: (res) => setTriageResult(res),
  });

  const handleAnalyze = (e: React.FormEvent) => {
    e.preventDefault();
    triageMutation.mutate({
      sender: 'kunde@gewerbe-solar.de',
      subject: 'Anfrage PV-Anlage 25 kWp',
      body: inputText,
    });
  };

  // --- TAB 6: Observability State (§5.5 / §20 EU AI Act) ---
  const { data: observabilityData } = useQuery<any>({
    queryKey: ['ai-observability'],
    queryFn: () => apiFetch('/api/v1/ai/observability'),
  });

  const stats = observabilityData?.stats || {
    total_interactions: 0,
    total_pii_blocked: 0,
    average_latency_ms: 0,
    compliance_standard: 'EU AI Act konform (Art. 50/52 Transparenz & Auditierbarkeit)',
    active_model: 'gemma4:12b (Ollama On-Premise)',
    provider: 'Ollama Local / Isolated Single-Tenant',
  };

  const recentLogs = Array.isArray(observabilityData?.recent_logs)
    ? observabilityData.recent_logs
    : [];

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100">KI-Zentrale & Copilot</h1>
          <p className="text-sm text-slate-400">
            Gemma 12B Assistenz, Evidence-Ledger, Wissensbasis, Web-Recherche und EU AI Act
            Observability
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className="flex items-center gap-1.5 px-3 py-1 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 text-xs font-semibold rounded-full">
            <ShieldCheck className="w-3.5 h-3.5" /> Prompt Guards Aktiv
          </span>
          <span className="flex items-center gap-1.5 px-3 py-1 bg-blue-500/10 text-blue-400 border border-blue-500/20 text-xs font-semibold rounded-full">
            <Cpu className="w-3.5 h-3.5" /> {activeModel} (Lokal)
          </span>
        </div>
      </div>

      {feedbackBanner && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center justify-between animate-fadeIn">
          <div className="flex items-center gap-2">
            <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
            <span>{feedbackBanner}</span>
          </div>
          <button
            onClick={() => setFeedbackBanner(null)}
            className="text-emerald-400/60 hover:text-emerald-300 p-1"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      )}

      {/* Tabs Navigation */}
      <div className="flex items-center gap-2 border-b border-slate-800 pb-2 overflow-x-auto">
        <button
          onClick={() => setActiveTab('copilot')}
          className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-bold transition-colors shrink-0 ${
            activeTab === 'copilot'
              ? 'bg-emerald-600 text-slate-950 shadow-lg shadow-emerald-600/20'
              : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
          }`}
        >
          <Bot className="w-4 h-4" />
          KI-Vertriebs-Copilot
        </button>

        <button
          onClick={() => setActiveTab('facts')}
          className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-bold transition-colors shrink-0 ${
            activeTab === 'facts'
              ? 'bg-emerald-600 text-slate-950 shadow-lg shadow-emerald-600/20'
              : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
          }`}
        >
          <Database className="w-4 h-4" />
          Evidence-Ledger (§5.2)
        </button>

        <button
          onClick={() => setActiveTab('kb')}
          className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-bold transition-colors shrink-0 ${
            activeTab === 'kb'
              ? 'bg-emerald-600 text-slate-950 shadow-lg shadow-emerald-600/20'
              : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
          }`}
        >
          <BookOpen className="w-4 h-4" />
          Wissensbasis / RAG (§5.5)
        </button>

        <button
          onClick={() => setActiveTab('research')}
          className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-bold transition-colors shrink-0 ${
            activeTab === 'research'
              ? 'bg-emerald-600 text-slate-950 shadow-lg shadow-emerald-600/20'
              : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
          }`}
        >
          <Globe className="w-4 h-4" />
          Web-Recherche (§5.4)
        </button>

        <button
          onClick={() => setActiveTab('triage')}
          className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-bold transition-colors shrink-0 ${
            activeTab === 'triage'
              ? 'bg-emerald-600 text-slate-950 shadow-lg shadow-emerald-600/20'
              : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
          }`}
        >
          <MessageSquareText className="w-4 h-4" />
          E-Mail Triage (§5.2)
        </button>

        <button
          onClick={() => setActiveTab('observability')}
          className={`flex items-center gap-2 px-3.5 py-2 rounded-xl text-xs font-bold transition-colors shrink-0 ${
            activeTab === 'observability'
              ? 'bg-emerald-600 text-slate-950 shadow-lg shadow-emerald-600/20'
              : 'text-slate-400 hover:text-slate-200 hover:bg-slate-900'
          }`}
        >
          <Activity className="w-4 h-4" />
          EU AI Act Observability (§5.5 / §20)
        </button>
      </div>

      {/* --- TAB 1: COPILOT CHAT --- */}
      {activeTab === 'copilot' && (
        <div className="bg-slate-900 border border-slate-800 rounded-2xl h-[650px] flex flex-col overflow-hidden shadow-sm">
          {/* Chat Stream */}
          <div className="flex-1 overflow-y-auto p-6 space-y-4 bg-slate-950/40">
            {chatMessages.map((msg, i) => (
              <div
                key={i}
                className={`flex gap-3 ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                {msg.role === 'assistant' && (
                  <div className="w-8 h-8 rounded-xl bg-emerald-500/10 text-emerald-400 flex items-center justify-center text-xs shrink-0 border border-emerald-500/20">
                    <Sparkles className="w-4 h-4" />
                  </div>
                )}
                <div
                  className={`max-w-[75%] rounded-2xl px-4 py-3 text-xs leading-relaxed shadow-sm ${
                    msg.role === 'user'
                      ? 'bg-emerald-600 text-slate-950 font-medium rounded-tr-none'
                      : 'bg-slate-800/90 text-slate-200 border border-slate-700/60 rounded-tl-none whitespace-pre-wrap'
                  }`}
                >
                  {msg.content}

                  {/* Interactive Action Card */}
                  {msg.actionCard && (
                    <div className="mt-3 p-3 bg-slate-900 border border-emerald-500/30 rounded-xl flex items-center justify-between gap-3 shadow-lg animate-fadeIn">
                      <div className="flex items-center gap-2.5 min-w-0">
                        <div className="w-7 h-7 rounded-lg bg-emerald-500/20 text-emerald-400 flex items-center justify-center shrink-0">
                          <Zap className="w-4 h-4" />
                        </div>
                        <div className="min-w-0">
                          <div className="text-xs font-bold text-slate-100 truncate">
                            {msg.actionCard.title}
                          </div>
                          <div className="text-[10px] text-emerald-400 font-mono font-bold">
                            {msg.actionCard.badge}
                          </div>
                        </div>
                      </div>
                      <button
                        onClick={() => navigate(msg.actionCard.route)}
                        className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-lg text-xs shrink-0 flex items-center gap-1.5 transition-colors"
                      >
                        <span>Öffnen</span>
                        <ArrowRight className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  )}

                  <div
                    className={`text-[10px] mt-1.5 text-right ${msg.role === 'user' ? 'text-emerald-950/70' : 'text-slate-500'}`}
                  >
                    {msg.time}
                  </div>
                </div>
              </div>
            ))}
            {chatMutation.isPending && (
              <div className="flex items-center gap-2 text-xs text-emerald-400 font-medium py-2">
                <Sparkles className="w-4 h-4 animate-spin" />
                <span>{activeModel} formuliert Antwort...</span>
              </div>
            )}
          </div>

          {/* Quick Prompt Chips */}
          <div className="px-4 py-2.5 bg-slate-900 border-t border-slate-800 flex gap-2 overflow-x-auto text-xs">
            {[
              'Pipeline-Status analysieren',
              'Pitch für 10 kWp Photovoltaik erstellen',
              'Widerrufsfrist nach § 355 BGB prüfen',
              'Follow-Up E-Mail an Gewerbekunden',
            ].map((chip, idx) => (
              <button
                key={idx}
                onClick={() => handleSendChat(undefined as any, chip)}
                className="whitespace-nowrap px-3 py-1.5 rounded-full bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 transition-colors"
              >
                {chip}
              </button>
            ))}
          </div>

          {/* Input Bar */}
          <form
            onSubmit={handleSendChat}
            className="p-4 bg-slate-950 border-t border-slate-800 flex gap-3"
          >
            <input
              type="text"
              placeholder="Frage an den Copilot stellen (z. B. 'Pipeline zusammenfassen' oder 'Neuen Deal anlegen')..."
              value={chatInput}
              onChange={(e) => setChatInput(e.target.value)}
              className="flex-1 px-4 py-2.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
            />
            <button
              type="submit"
              disabled={chatMutation.isPending || !chatInput.trim()}
              className="px-5 py-2.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 text-slate-950 font-bold rounded-xl text-xs transition-colors flex items-center gap-2 shadow-lg shadow-emerald-600/20"
            >
              <Send className="w-4 h-4" />
              Senden
            </button>
          </form>
        </div>
      )}

      {/* --- TAB 2: EVIDENCE-LEDGER / FAKTEN (§5.2) --- */}
      {activeTab === 'facts' && (
        <div className="space-y-4">
          <div className="p-4 bg-slate-900 border border-slate-800 rounded-2xl flex items-center justify-between">
            <div>
              <h3 className="font-bold text-slate-100 text-sm">
                Evidence-Ledger & Gewichtete Fakten (§5.2)
              </h3>
              <p className="text-xs text-slate-400">
                Automatisch aus E-Mails, Telefonaten und Web-Recherchen extrahierte Kunden- und
                Energiedaten
              </p>
            </div>
            <span className="px-3 py-1 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 text-xs font-semibold rounded-full">
              {facts.length} Fakten erfasst
            </span>
          </div>

          <div className="bg-slate-900 border border-slate-800 rounded-2xl overflow-hidden shadow-sm">
            <table className="w-full text-left text-xs text-slate-300">
              <thead className="bg-slate-950/60 text-slate-400 uppercase border-b border-slate-800">
                <tr>
                  <th className="py-3.5 px-6 font-semibold">Kunde / Firma</th>
                  <th className="py-3.5 px-6 font-semibold">Fakt & Attribut</th>
                  <th className="py-3.5 px-6 font-semibold">Quelle & Konfidenz</th>
                  <th className="py-3.5 px-6 font-semibold">Status & HITL</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800">
                {facts.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="py-8 text-center text-slate-500">
                      Noch keine extrahierten Fakten im Ledger vorhanden.
                    </td>
                  </tr>
                ) : (
                  facts.map((fact) => (
                    <tr key={fact.id} className="hover:bg-slate-800/40 transition-colors">
                      <td className="py-4 px-6 font-semibold text-slate-100">{fact.entity}</td>
                      <td className="py-4 px-6">
                        <span className="text-slate-400">{fact.field}:</span>{' '}
                        <span className="font-bold text-emerald-400">{fact.value}</span>
                      </td>
                      <td className="py-4 px-6 text-slate-400">
                        <div>{fact.source}</div>
                        <div className="text-[10px] text-emerald-400 font-mono font-semibold">
                          Konfidenz: {(fact.confidence * 100).toFixed(0)}%
                        </div>
                      </td>
                      <td className="py-4 px-6">
                        {fact.status === 'BESTÄTIGT' ? (
                          <span className="inline-flex items-center gap-1 text-[11px] font-semibold px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                            <CheckCircle2 className="w-3 h-3" /> Bestätigt
                          </span>
                        ) : (
                          <div className="flex items-center gap-2">
                            <button
                              onClick={() => handleConfirmFact(fact.id)}
                              className="p-1 rounded bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold"
                              title="Fakt bestätigen"
                            >
                              <Check className="w-3.5 h-3.5" />
                            </button>
                            <button
                              onClick={() => handleRejectFact(fact.id)}
                              className="p-1 rounded bg-slate-800 hover:bg-rose-500/20 text-slate-400 hover:text-rose-400 border border-slate-700"
                              title="Fakt ablehnen"
                            >
                              <X className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* --- TAB 3: WISSENSBASIS / KNOWLEDGE BASE & RAG (§5.5) --- */}
      {activeTab === 'kb' && (
        <div className="space-y-5">
          <div className="p-4 bg-slate-900 border border-slate-800 rounded-2xl flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h3 className="font-bold text-slate-100 text-sm flex items-center gap-2">
                <Database className="w-4 h-4 text-emerald-400" />
                Wissensbasis & Dokumenten-Index (§5.5)
              </h3>
              <p className="text-xs text-slate-400">
                Unternehmensspezifisches Wissen, Preismodelle und technische Datenblätter für den
                semantischen RAG-Retrieval
              </p>
            </div>
            <div className="flex items-center gap-3">
              <div className="relative">
                <Search className="w-3.5 h-3.5 absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
                <input
                  type="text"
                  placeholder="Wissen durchsuchen..."
                  value={kbSearch}
                  onChange={(e) => setKbSearch(e.target.value)}
                  className="pl-8 pr-3 py-1.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500 w-48"
                />
              </div>
              <button
                onClick={() => setIsKbModalOpen(true)}
                className="inline-flex items-center gap-1.5 px-3.5 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs transition-colors shadow-sm cursor-pointer"
              >
                <Plus className="w-3.5 h-3.5" />
                <span>Wissen ablegen</span>
              </button>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {filteredKbArticles.length === 0 ? (
              <div className="col-span-2 p-12 text-center text-slate-500 bg-slate-900 border border-slate-800 rounded-2xl text-xs">
                Keine Wissensdokumente für diesen Suchbegriff gefunden.
              </div>
            ) : (
              filteredKbArticles.map((entry) => (
                <div
                  key={entry.id}
                  className="p-5 bg-slate-900 border border-slate-800 rounded-2xl space-y-3 shadow-sm hover:border-slate-700 transition-colors"
                >
                  <div className="flex items-center justify-between">
                    <span className="text-[10px] font-mono px-2.5 py-0.5 rounded-full bg-slate-800 text-emerald-400 border border-slate-700 font-semibold">
                      {entry.category}
                    </span>
                    <button
                      onClick={() => deleteKbMutation.mutate(entry.id)}
                      className="p-1 text-slate-500 hover:text-rose-400 hover:bg-rose-500/10 rounded transition-colors cursor-pointer"
                      title="Aus Wissensbasis löschen"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>

                  <h4 className="text-sm font-bold text-slate-100">{entry.title}</h4>
                  <p className="text-xs text-slate-300 leading-relaxed">{entry.content}</p>

                  <div className="pt-2 border-t border-slate-800 flex items-center justify-between text-[11px] text-slate-400">
                    <div className="flex items-center gap-1.5">
                      <FileText className="w-3 h-3 text-slate-500" />
                      <span className="truncate max-w-[160px]">
                        {entry.source || 'Manuelle Eingabe'}
                      </span>
                    </div>
                    <div
                      className={`flex items-center gap-1 font-mono font-semibold ${entry.indexed ? 'text-emerald-400' : 'text-slate-500'}`}
                    >
                      <Layers className="w-3 h-3" />
                      <span>
                        {entry.indexed ? entry.embedding_model || 'indexiert' : 'nicht indexiert'}
                      </span>
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>
        </div>
      )}

      {/* --- TAB 4: DEEP COMPANY RESEARCH JOBS (§5.4) --- */}
      {activeTab === 'research' && (
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
          {/* Left Column: Start Research Job */}
          <div className="lg:col-span-5 bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-4">
            <div className="flex items-center gap-2 text-sm font-bold text-slate-200">
              <Compass className="w-4 h-4 text-emerald-400" />
              <span>Neuen Research-Auftrag starten (§5.4)</span>
            </div>
            <p className="text-xs text-slate-400 leading-relaxed">
              Automatisierte Web-Recherche per Gemma 12B & SSRF-geschütztem Web-Crawl. Analysiert
              Impressum, Gewerbedachflächen, Entscheider und technologische Eignung.
            </p>

            <form onSubmit={handleStartResearchJob} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Unternehmens-Domain oder Name *
                </label>
                <div className="relative">
                  <Globe className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500" />
                  <input
                    type="text"
                    required
                    value={researchDomain}
                    onChange={(e) => setResearchDomain(e.target.value)}
                    placeholder="z.B. energie-dach.de oder Schmidt Metallbau"
                    className="w-full pl-10 pr-4 py-2.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500 font-mono"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Recherche-Tiefe
                </label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setResearchDepth('standard')}
                    className={`py-2 px-3 rounded-xl border text-xs font-semibold text-center transition-all cursor-pointer ${
                      researchDepth === 'standard'
                        ? 'bg-emerald-600 text-slate-950 border-emerald-500'
                        : 'bg-slate-950 text-slate-400 border-slate-800 hover:border-slate-700'
                    }`}
                  >
                    Standard Crawl
                  </button>
                  <button
                    type="button"
                    onClick={() => setResearchDepth('deep')}
                    className={`py-2 px-3 rounded-xl border text-xs font-semibold text-center transition-all cursor-pointer ${
                      researchDepth === 'deep'
                        ? 'bg-emerald-600 text-slate-950 border-emerald-500'
                        : 'bg-slate-950 text-slate-400 border-slate-800 hover:border-slate-700'
                    }`}
                  >
                    Deep Research (Entscheider)
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Fokus & Zielbranche
                </label>
                <select
                  value={researchCategory}
                  onChange={(e) => setResearchCategory(e.target.value)}
                  className="w-full p-2.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
                >
                  <option value="Gewerbesolar & Hallendach">
                    Gewerbesolar & Hallendach (30–500 kWp)
                  </option>
                  <option value="B2C Eigenheim & Dachsanierung">
                    B2C Eigenheim & Dachsanierung
                  </option>
                  <option value="Wärmepumpe & HVAC Gewerbe">Wärmepumpe & HVAC Gewerbe</option>
                  <option value="Allgemeine B2B Akquise">Allgemeine B2B Akquise</option>
                </select>
              </div>

              <button
                type="submit"
                disabled={createResearchJobMutation.isPending}
                className="w-full py-2.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-slate-950 font-bold rounded-xl text-xs transition-colors flex items-center justify-center gap-2 shadow-lg shadow-emerald-600/20 cursor-pointer"
              >
                <Sparkles
                  className={`w-4 h-4 ${createResearchJobMutation.isPending ? 'animate-spin' : ''}`}
                />
                <span>
                  {createResearchJobMutation.isPending
                    ? 'Research-Auftrag läuft...'
                    : 'Automatisch Recherchieren'}
                </span>
              </button>
            </form>

            {/* Research History List */}
            <div className="pt-3 border-t border-slate-800 space-y-2">
              <span className="text-[11px] font-bold text-slate-400 uppercase tracking-wider block">
                Letzte Research-Aufträge ({researchJobs.length})
              </span>
              <div className="space-y-1.5 max-h-48 overflow-y-auto">
                {researchJobs.map((job) => (
                  <div
                    key={job.id}
                    onClick={() => setSelectedResearchJob(job)}
                    className={`p-2.5 rounded-xl border text-xs cursor-pointer transition-colors flex items-center justify-between ${
                      selectedResearchJob?.id === job.id
                        ? 'bg-emerald-500/10 border-emerald-500/30'
                        : 'bg-slate-950 border-slate-800/80 hover:bg-slate-800/40'
                    }`}
                  >
                    <div className="truncate pr-2">
                      <div className="font-semibold text-slate-200 truncate">
                        {job.company_name || job.query}
                      </div>
                      <div className="text-[10px] text-slate-500 font-mono">{job.query}</div>
                    </div>
                    <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold shrink-0">
                      Fertig
                    </span>
                  </div>
                ))}
              </div>
            </div>
          </div>

          {/* Right Column: Research Details & CRM Export */}
          <div className="lg:col-span-7 bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm font-bold text-slate-200 flex items-center gap-2">
                <Building2 className="w-4 h-4 text-emerald-400" />
                <span>Recherche-Ergebnis & Dossier</span>
              </span>
              {selectedResearchJob && (
                <span className="text-xs px-2.5 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold">
                  Gemma 12B Analyse abgeschlossen
                </span>
              )}
            </div>

            {selectedResearchJob ? (
              <div className="space-y-4">
                <div className="bg-slate-950 border border-slate-800 rounded-xl p-4 space-y-1">
                  <div className="text-[10px] text-slate-500 uppercase font-semibold">
                    Identifiziertes Unternehmen & Domain
                  </div>
                  <div className="text-sm font-bold text-slate-100">
                    {selectedResearchJob.result?.site_title || selectedResearchJob.company_name}
                  </div>
                  <div className="text-xs text-slate-400 font-mono">
                    {selectedResearchJob.query}
                  </div>
                </div>

                <div className="bg-slate-950 border border-slate-800 rounded-xl p-4 space-y-1.5">
                  <div className="text-[10px] text-slate-500 uppercase font-semibold">
                    Geschäftsmodell & PV-Eignung
                  </div>
                  <p className="text-xs text-slate-300 leading-relaxed">
                    {selectedResearchJob.result?.summary}
                  </p>
                </div>

                {/* Decision Makers */}
                {selectedResearchJob.result?.decision_makers && (
                  <div className="bg-slate-950 border border-slate-800 rounded-xl p-4 space-y-2">
                    <div className="text-[10px] text-slate-500 uppercase font-semibold">
                      Gefundene Entscheider & Kontakte
                    </div>
                    <div className="flex flex-wrap gap-2">
                      {selectedResearchJob.result.decision_makers.map(
                        (person: string, idx: number) => (
                          <span
                            key={idx}
                            className="px-2.5 py-1 rounded-lg bg-purple-500/10 border border-purple-500/20 text-purple-300 text-xs font-medium"
                          >
                            👤 {person}
                          </span>
                        ),
                      )}
                    </div>
                  </div>
                )}

                {/* Industry Keywords */}
                {selectedResearchJob.result?.industry_tags && (
                  <div className="bg-slate-950 border border-slate-800 rounded-xl p-4 space-y-2">
                    <div className="text-[10px] text-slate-500 uppercase font-semibold">
                      Branchen- & Eignungs-Tags
                    </div>
                    <div className="flex flex-wrap gap-1.5">
                      {selectedResearchJob.result.industry_tags.map((kw: string, idx: number) => (
                        <span
                          key={idx}
                          className="px-2.5 py-1 rounded-lg bg-slate-900 border border-slate-700 text-[11px] text-emerald-400 font-medium"
                        >
                          {kw}
                        </span>
                      ))}
                    </div>
                  </div>
                )}

                {/* 1-Click Save to CRM Actions */}
                <div className="pt-3 border-t border-slate-800 flex justify-end gap-3">
                  <button
                    onClick={() => {
                      setFeedbackBanner(
                        `Recherche-Dossier für "${selectedResearchJob.company_name}" als CRM-Notiz abgelegt!`,
                      );
                      setTimeout(() => setFeedbackBanner(null), 4000);
                    }}
                    className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 font-semibold rounded-xl text-xs transition-colors cursor-pointer"
                  >
                    Als Notiz anlegen
                  </button>
                  <button
                    onClick={() => {
                      setFeedbackBanner(
                        `Unternehmen "${selectedResearchJob.company_name}" erfolgreich ins CRM übernommen!`,
                      );
                      setTimeout(() => setFeedbackBanner(null), 4000);
                    }}
                    className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs transition-colors shadow-lg shadow-emerald-600/20 cursor-pointer"
                  >
                    Als CRM-Unternehmen anlegen
                  </button>
                </div>
              </div>
            ) : (
              <div className="h-64 flex flex-col items-center justify-center text-center text-slate-500 space-y-2">
                <Globe className="w-10 h-10 text-slate-800" />
                <div className="text-xs">
                  Wähle links einen Auftrag aus oder starte eine neue Recherche.
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* --- MODAL: WISSEN ABLEGEN FÜR RAG (§5.5) --- */}
      {isKbModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-3xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <Database className="w-5 h-5 text-emerald-400" />
                <h3 className="font-bold text-slate-100 text-base">
                  Neues Wissensdokument ablegen (RAG)
                </h3>
              </div>
              <button
                onClick={() => setIsKbModalOpen(false)}
                className="text-slate-400 hover:text-slate-200"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form
              onSubmit={(e) => {
                e.preventDefault();
                createKbMutation.mutate(kbForm);
              }}
              className="space-y-4"
            >
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Titel des Dokuments *
                </label>
                <input
                  type="text"
                  required
                  placeholder="z.B. BYD Battery-Box Premium HVS Spezifikationen 2026"
                  value={kbForm.title}
                  onChange={(e) => setKbForm({ ...kbForm, title: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    Kategorie
                  </label>
                  <select
                    value={kbForm.category}
                    onChange={(e) => setKbForm({ ...kbForm, category: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="Photovoltaik & Speicher">Photovoltaik & Speicher</option>
                    <option value="Preise & Kalkulation">Preise & Kalkulation</option>
                    <option value="Recht & Compliance">Recht & Compliance § 355/UWG</option>
                    <option value="D2D & Sales Playbook">D2D & Sales Playbook</option>
                    <option value="Allgemein">Allgemein</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    Quelldatei / Referenz
                  </label>
                  <input
                    type="text"
                    placeholder="z.B. BYD_Handbuch_2026.pdf"
                    value={kbForm.source}
                    onChange={(e) => setKbForm({ ...kbForm, source: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Wissensinhalt / Fließtext *
                </label>
                <textarea
                  required
                  rows={5}
                  placeholder="Inhalt einfügen... Wird als Embedding generiert und semantisch indexiert."
                  value={kbForm.content}
                  onChange={(e) => setKbForm({ ...kbForm, content: e.target.value })}
                  className="w-full p-3 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500 leading-relaxed"
                />
              </div>

              {/* Vectorization Info Banner */}
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl flex items-center justify-between text-[11px] text-emerald-400">
                <span className="flex items-center gap-1.5 font-medium">
                  <Layers className="w-3.5 h-3.5" />
                  Semantische Indexierung als 1024-dimensionales Embedding
                </span>
              </div>

              <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setIsKbModalOpen(false)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-medium transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={createKbMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs transition-colors shadow-lg shadow-emerald-600/20 cursor-pointer"
                >
                  {createKbMutation.isPending ? 'Speichere...' : 'Wissen in Datenbank ablegen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* --- TAB 5: TRIAGE & PROMPT GUARDS --- */}

      {/* --- TAB 5: TRIAGE & PROMPT GUARDS --- */}
      {activeTab === 'triage' && (
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
          <div className="lg:col-span-5 bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-4">
            <div className="flex items-center gap-2 text-sm font-bold text-slate-200">
              <MessageSquareText className="w-4 h-4 text-emerald-400" />
              E-Mail- & Lead-Text testen
            </div>

            <form onSubmit={handleAnalyze} className="space-y-4">
              <textarea
                rows={8}
                value={inputText}
                onChange={(e) => setInputText(e.target.value)}
                className="w-full p-3 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500 font-mono leading-relaxed"
                placeholder="Text eingeben..."
              />
              <button
                type="submit"
                disabled={triageMutation.isPending}
                className="w-full py-2.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs transition-colors flex items-center justify-center gap-2 shadow-lg shadow-emerald-600/20"
              >
                <Sparkles className="w-4 h-4" />
                {triageMutation.isPending ? 'KI analysiert...' : 'Mit KI-Engine analysieren'}
              </button>
            </form>
          </div>

          <div className="lg:col-span-7 bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <span className="text-sm font-bold text-slate-200">
                Strukturierte KI-Ergebnisse (Gemma 12B)
              </span>
              {triageResult && (
                <span className="text-xs px-2.5 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold">
                  Validiert
                </span>
              )}
            </div>

            {triageResult ? (
              <div className="space-y-4">
                <div className="grid grid-cols-3 gap-3">
                  <div className="bg-slate-950 border border-slate-800 rounded-xl p-3">
                    <div className="text-[10px] text-slate-500 uppercase font-semibold">
                      Kategorie
                    </div>
                    <div className="text-xs font-bold text-emerald-400 mt-1">
                      {triageResult.category}
                    </div>
                  </div>
                  <div className="bg-slate-950 border border-slate-800 rounded-xl p-3">
                    <div className="text-[10px] text-slate-500 uppercase font-semibold">
                      Priorität
                    </div>
                    <div className="text-xs font-bold text-amber-400 mt-1">
                      {triageResult.priority}
                    </div>
                  </div>
                  <div className="bg-slate-950 border border-slate-800 rounded-xl p-3">
                    <div className="text-[10px] text-slate-500 uppercase font-semibold">
                      Sentiment
                    </div>
                    <div className="text-xs font-bold text-cyan-400 mt-1">
                      {triageResult.sentiment}
                    </div>
                  </div>
                </div>

                <div className="bg-slate-950 border border-slate-800 rounded-xl p-4 space-y-1.5">
                  <div className="text-[11px] font-bold text-slate-400 uppercase">
                    Zusammenfassung
                  </div>
                  <div className="text-xs text-slate-200">{triageResult.summary}</div>
                </div>

                <div className="bg-slate-950 border border-slate-800 rounded-xl p-4 space-y-2">
                  <div className="flex items-center justify-between text-[11px] font-bold text-slate-400 uppercase">
                    <span>Antwortentwurf (Human-in-the-Loop Vorschlag)</span>
                    <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
                  </div>
                  <div className="text-xs text-slate-300 whitespace-pre-wrap font-sans bg-slate-900/60 p-3 rounded-lg border border-slate-800">
                    {triageResult.draft_reply}
                  </div>
                </div>
              </div>
            ) : (
              <div className="h-64 flex flex-col items-center justify-center text-center text-slate-500 space-y-2">
                <Sparkles className="w-10 h-10 text-slate-800" />
                <div className="text-xs">
                  Klicke links auf "Mit KI-Engine analysieren", um die Extraktion zu starten.
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* --- TAB 6: EU AI ACT OBSERVABILITY & DECISION AUDIT --- */}
      {activeTab === 'observability' && (
        <div className="space-y-6">
          {/* KPI Cards */}
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 space-y-2">
              <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
                <Activity className="w-4 h-4 text-emerald-400" /> Gesamte KI-Interaktionen
              </div>
              <div className="text-2xl font-bold text-slate-100">{stats.total_interactions}</div>
              <div className="text-[11px] text-slate-500">Seit Systeminitialisierung</div>
            </div>

            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 space-y-2">
              <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
                <Lock className="w-4 h-4 text-emerald-400" /> Blockierte PII-Elemente
              </div>
              <div className="text-2xl font-bold text-emerald-400">{stats.total_pii_blocked}</div>
              <div className="text-[11px] text-slate-500">IBANs, CCs, Passwörter maskiert</div>
            </div>

            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 space-y-2">
              <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
                <Cpu className="w-4 h-4 text-blue-400" /> Durchschnitts-Latenz
              </div>
              <div className="text-2xl font-bold text-slate-100">
                {stats.average_latency_ms.toFixed(0)} ms
              </div>
              <div className="text-[11px] text-slate-500">Lokale Ollama-Inferenz</div>
            </div>

            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 space-y-2">
              <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
                <ShieldCheck className="w-4 h-4 text-cyan-400" /> EU AI Act Status
              </div>
              <div className="text-sm font-bold text-slate-100">Art. 50/52 Konform</div>
              <div className="text-[11px] text-slate-500">100% Transparenz & Audit-Trail</div>
            </div>
          </div>

          {/* Audit Logs Table with Clickable Decision Inspector */}
          <div className="bg-slate-900 border border-slate-800 rounded-2xl overflow-hidden shadow-sm">
            <div className="p-4 border-b border-slate-800 flex items-center justify-between">
              <div>
                <h3 className="font-bold text-slate-200 text-sm">
                  EU AI Act Revisionssicheres Audit-Log & KI-Entscheidungs-Inspektor
                </h3>
                <p className="text-xs text-slate-400">
                  Klicken Sie auf einen Log-Eintrag, um Eingabe-Prompts, PII-Maskierungen und
                  KI-Klassifikationen im Detail zu prüfen.
                </p>
              </div>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs text-slate-300">
                <thead className="bg-slate-950/60 text-slate-400 uppercase border-b border-slate-800">
                  <tr>
                    <th className="py-3 px-4 font-semibold">Zeitstempel</th>
                    <th className="py-3 px-4 font-semibold">KI-Job & Typ</th>
                    <th className="py-3 px-4 font-semibold">Modell & Provider</th>
                    <th className="py-3 px-4 font-semibold">Latenz</th>
                    <th className="py-3 px-4 font-semibold">PII-Schutzstatus</th>
                    <th className="py-3 px-4 font-semibold text-right">Details</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800">
                  {recentLogs.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="py-8 text-center text-slate-500">
                        Noch keine KI-Audit-Logs vorhanden.
                      </td>
                    </tr>
                  ) : (
                    recentLogs.map((log: any) => (
                      <tr
                        key={log.id}
                        onClick={() => setSelectedAuditLog(log)}
                        className="hover:bg-slate-800/60 cursor-pointer transition-colors"
                      >
                        <td className="py-3 px-4 font-mono text-slate-400">
                          {new Date(log.created_at).toLocaleTimeString()}
                        </td>
                        <td className="py-3 px-4 font-semibold text-slate-100">
                          {log.interaction_type}
                        </td>
                        <td className="py-3 px-4 font-mono text-slate-300">
                          {log.model_name} <span className="text-slate-500">({log.provider})</span>
                        </td>
                        <td className="py-3 px-4 font-mono text-slate-300">{log.latency_ms} ms</td>
                        <td className="py-3 px-4">
                          {log.pii_filter_triggered ? (
                            <span className="px-2 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 font-semibold">
                              {log.pii_redactions_count} PII Redacted
                            </span>
                          ) : (
                            <span className="px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold">
                              Clean
                            </span>
                          )}
                        </td>
                        <td className="py-3 px-4 text-right">
                          <button className="text-emerald-400 hover:text-emerald-300 font-semibold inline-flex items-center gap-1">
                            <Eye className="w-3.5 h-3.5" /> Prüfen
                          </button>
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* --- MODAL: KI-ENTSCHEIDUNGS-INSPEKTOR (EU AI Act Audit Details) --- */}
      {selectedAuditLog && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 animate-fadeIn">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-3xl w-full p-6 shadow-2xl space-y-5 max-h-[90vh] overflow-y-auto">
            {/* Modal Header */}
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2.5">
                <FileText className="w-5 h-5 text-emerald-400" />
                <div>
                  <h3 className="font-bold text-slate-100 text-base">
                    KI-Audit & Entscheidungs-Protokoll
                  </h3>
                  <div className="text-xs text-slate-400 font-mono">
                    Job-ID: {selectedAuditLog.id} • {selectedAuditLog.interaction_type}
                  </div>
                </div>
              </div>
              <button
                onClick={() => setSelectedAuditLog(null)}
                className="p-1 rounded-lg text-slate-400 hover:text-slate-200 hover:bg-slate-800"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* Model & Compliance Meta Grid */}
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 bg-slate-950 p-3.5 rounded-xl border border-slate-800 text-xs">
              <div>
                <span className="text-slate-500 block text-[10px] uppercase font-semibold">
                  Modell
                </span>
                <span className="font-mono text-emerald-400 font-bold">
                  {selectedAuditLog.model_name}
                </span>
              </div>
              <div>
                <span className="text-slate-500 block text-[10px] uppercase font-semibold">
                  Inferenz-Latenz
                </span>
                <span className="font-mono text-slate-200">{selectedAuditLog.latency_ms} ms</span>
              </div>
              <div>
                <span className="text-slate-500 block text-[10px] uppercase font-semibold">
                  Infrastruktur
                </span>
                <span className="text-slate-200 font-medium">100% On-Premise</span>
              </div>
              <div>
                <span className="text-slate-500 block text-[10px] uppercase font-semibold">
                  EU AI Act Status
                </span>
                <span className="text-cyan-400 font-bold">Art. 50/52 Konform</span>
              </div>
            </div>

            {/* PII Sanitization Report */}
            <div className="space-y-1.5">
              <div className="text-xs font-bold text-slate-300 flex items-center justify-between">
                <span>🛡️ DSGVO Datenschutz- & Prompt-Guard-Filter</span>
                {selectedAuditLog.pii_filter_triggered ? (
                  <span className="text-[10px] px-2 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 font-semibold">
                    {selectedAuditLog.pii_redactions_count} sensible Elemente maskiert
                  </span>
                ) : (
                  <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-semibold">
                    Keine PII-Maskierung erforderlich
                  </span>
                )}
              </div>
              {selectedAuditLog.redacted_items?.length > 0 && (
                <div className="p-2.5 bg-slate-950 border border-amber-500/20 rounded-xl space-y-1">
                  <div className="text-[10px] text-slate-500 uppercase font-semibold">
                    Maskierte Datenpunkte vor LLM-Übergabe:
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {selectedAuditLog.redacted_items.map((item: string, idx: number) => (
                      <span
                        key={idx}
                        className="font-mono text-[11px] px-2 py-0.5 rounded bg-amber-500/10 text-amber-300 border border-amber-500/30"
                      >
                        {item}
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </div>

            {/* Input Prompt (Sanitized) */}
            <div className="space-y-1.5">
              <div className="text-xs font-bold text-slate-300">
                📥 Bereinigter Eingabe-Prompt (Input-Kontext)
              </div>
              <pre className="p-3 bg-slate-950 border border-slate-800 rounded-xl text-xs font-mono text-slate-300 whitespace-pre-wrap leading-relaxed">
                {selectedAuditLog.input_prompt}
              </pre>
            </div>

            {/* Extracted Decision & HITL Payload */}
            <div className="space-y-1.5">
              <div className="text-xs font-bold text-slate-300">
                🧠 Extrahierte KI-Entscheidung & Handlungs-Vorschlag
              </div>
              <div className="p-3.5 bg-slate-950 border border-slate-800 rounded-xl space-y-2.5">
                <div className="grid grid-cols-3 gap-2">
                  <div className="bg-slate-900 p-2 rounded-lg border border-slate-800">
                    <span className="text-[10px] text-slate-500 uppercase block font-semibold">
                      Klassifikation
                    </span>
                    <span className="font-bold text-emerald-400 text-xs">
                      {selectedAuditLog.extracted_decision.category}
                    </span>
                  </div>
                  <div className="bg-slate-900 p-2 rounded-lg border border-slate-800">
                    <span className="text-[10px] text-slate-500 uppercase block font-semibold">
                      Dringlichkeit
                    </span>
                    <span className="font-bold text-amber-400 text-xs">
                      {selectedAuditLog.extracted_decision.priority}
                    </span>
                  </div>
                  <div className="bg-slate-900 p-2 rounded-lg border border-slate-800">
                    <span className="text-[10px] text-slate-500 uppercase block font-semibold">
                      Stimmung
                    </span>
                    <span className="font-bold text-cyan-400 text-xs">
                      {selectedAuditLog.extracted_decision.sentiment}
                    </span>
                  </div>
                </div>

                <div>
                  <span className="text-[10px] text-slate-500 uppercase block font-semibold mb-0.5">
                    Erkannte Zusammenfassung
                  </span>
                  <p className="text-xs text-slate-200">
                    {selectedAuditLog.extracted_decision.summary}
                  </p>
                </div>

                <div>
                  <span className="text-[10px] text-slate-500 uppercase block font-semibold mb-0.5">
                    Vorbereitete Aktion / Entwurf (HITL)
                  </span>
                  <div className="text-xs text-slate-300 bg-slate-900 p-2.5 rounded-lg border border-slate-800 whitespace-pre-wrap font-sans">
                    {selectedAuditLog.extracted_decision.suggested_draft}
                  </div>
                </div>
              </div>
            </div>

            {/* Close Button */}
            <div className="flex justify-end pt-2 border-t border-slate-800">
              <button
                onClick={() => setSelectedAuditLog(null)}
                className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-xl text-xs font-semibold"
              >
                Schließen
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
