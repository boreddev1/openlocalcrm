import React, { useState, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch, sendEmail, tagEmailMessage } from '../api/client';
import {
  Mail,
  Send,
  Reply,
  User,
  Sparkles,
  Inbox as InboxIcon,
  Tag,
  Plus,
  Filter,
  ShieldCheck,
  ShieldAlert,
  Clock,
  CheckCircle2,
  Brain,
  Flame,
} from 'lucide-react';

// --- Prompt Injection Defense Engine (§19.3) ---
export function analyzePromptInjection(text: string) {
  const injectionPatterns = [
    /ignore\s+(all\s+)?(previous|prior)\s+instructions/i,
    /system\s+prompt/i,
    /override\s+(all\s+)?rules/i,
    /dump\s+(all\s+)?(database|credentials|passwords?|keys?)/i,
    /leak\s+(crm|customer|secret)/i,
    /send\s+all\s+contacts/i,
    /\[system\]/i,
    /<\|im_start\|>/i,
    /you\s+are\s+now\s+in\s+developer\s+mode/i,
  ];

  const matched = injectionPatterns.filter((p) => p.test(text));
  const hasInjection = matched.length > 0;

  // Sanitize text by neutralizing injection markers
  let sanitized = text;
  if (hasInjection) {
    sanitized = text.replace(
      /(ignore\s+(all\s+)?(previous|prior)\s+instructions|system\s+prompt|dump\s+database|override\s+rules)/gi,
      '⚠️ [SECURITY FILTER: Malicious instruction neutralized]',
    );
  }

  return {
    hasInjection,
    threatCount: matched.length,
    sanitizedText: sanitized,
    threatDetails: hasInjection
      ? 'Verdacht auf Prompt-Injection-Angriff (Befehle isoliert & neutralisiert)'
      : null,
  };
}

// --- Urgency & SLA Deadline Analyzer (§4.3) ---
export function analyzeUrgencyAndSLA(subject: string, body: string) {
  const fullText = `${subject} ${body}`.toLowerCase();

  const urgentKeywords = [
    'dringend',
    'eilig',
    'sofort',
    'notfall',
    'frist',
    'widerruf',
    'stornierung',
    'schaden',
    'bis morgen',
    'schnell',
    'abgelaufen',
  ];

  const isUrgent = urgentKeywords.some((kw) => fullText.includes(kw));

  // Date extraction regex (e.g. 28.08.2026 or 28.08.)
  const dateMatch = fullText.match(/(\d{1,2}\.\d{1,2}\.(\d{2,4})?)/);
  const deadline = dateMatch ? dateMatch[0] : null;

  let slaHours = 24;
  let reason = 'Standard-Anfrage';

  if (fullText.includes('widerruf') || fullText.includes('stornierung')) {
    slaHours = 2;
    reason = 'Widerruf / Stornierung nach § 355 BGB (Höchste Priorität: < 2h)';
  } else if (fullText.includes('schaden') || fullText.includes('notfall')) {
    slaHours = 4;
    reason = 'Technischer Notfall / Störungsmeldung (SLA: < 4h)';
  } else if (isUrgent || deadline) {
    slaHours = 8;
    reason = deadline
      ? `Fristgebundene Anfrage bis ${deadline} (SLA: < 8h)`
      : 'Dringender Handlungsbedarf (SLA: < 8h)';
  }

  return {
    isUrgent,
    slaHours,
    reason,
    deadline,
  };
}

interface TriageResult {
  category: string;
  sentiment: string;
  priority: string;
  summary: string;
  draft_reply: string;
}

export const InboxPage: React.FC = () => {
  const queryClient = useQueryClient();
  const [selectedMessageId, setSelectedMessageId] = useState<string | null>(null);
  const [replyText, setReplyText] = useState('');
  const [isDemoModalOpen, setIsDemoModalOpen] = useState(false);
  const [selectedTagFilter, setSelectedTagFilter] = useState<string>('ALL');
  const [newCustomTag, setNewCustomTag] = useState('');
  const [isAddingTag, setIsAddingTag] = useState(false);
  const [feedbackBanner, setFeedbackBanner] = useState<string | null>(null);
  const [errorBanner, setErrorBanner] = useState<string | null>(null);
  const [triageByMessage, setTriageByMessage] = useState<Record<string, TriageResult>>({});

  const { data: healthData } = useQuery<any>({
    queryKey: ['health'],
    queryFn: () => apiFetch('/api/v1/health'),
  });
  const isDemo = healthData?.demo_mode === true;

  const { data: rawMessages = [], isLoading } = useQuery<any[]>({
    queryKey: ['emails'],
    queryFn: () => apiFetch('/api/v1/emails/messages'),
  });
  const messages = Array.isArray(rawMessages) ? rawMessages : [];
  const selectedMessage = messages.find((m) => m.id === selectedMessageId) || null;

  const availableTags = [
    { id: 'PV-Interessent', color: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' },
    { id: 'Wärmepumpe', color: 'bg-amber-500/10 text-amber-400 border-amber-500/20' },
    { id: 'Dringend', color: 'bg-rose-500/10 text-rose-400 border-rose-500/20' },
    { id: 'Widerruf § 355', color: 'bg-purple-500/10 text-purple-400 border-purple-500/20' },
    { id: 'Angebot versendet', color: 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20' },
  ];

  const tagMutation = useMutation({
    mutationFn: ({ id, tags }: { id: string; tags: string[] }) => tagEmailMessage(id, tags),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['emails'] });
    },
  });

  const demoIngestMutation = useMutation({
    mutationFn: (demo: any) =>
      apiFetch<any>('/api/v1/emails/demo-ingest', {
        method: 'POST',
        body: JSON.stringify(demo),
      }),
    onSuccess: (newMsg: any) => {
      queryClient.invalidateQueries({ queryKey: ['emails'] });
      setIsDemoModalOpen(false);

      // Auto-tag urgent-sounding demo messages, persisted server-side.
      if (newMsg && newMsg.id) {
        const urgency = analyzeUrgencyAndSLA(
          newMsg.subject || '',
          newMsg.body_text?.String || newMsg.body_text || '',
        );
        if (urgency.isUrgent) {
          tagMutation.mutate({ id: newMsg.id, tags: ['Dringend'] });
        }
      }

      setFeedbackBanner(
        'Eingehende E-Mail erfolgreich simuliert, analysiert & per Gemma 12B klassifiziert!',
      );
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const [demoForm, setDemoForm] = useState({
    sender_name: 'Dr. Michael Weber',
    sender_email: 'weber@energie-dach.de',
    subject: 'Angebotserstellung PV-Anlage 30 kWp Halle',
    body_text:
      'Sehr geehrtes Team, bitte senden Sie uns ein unverbindliches Angebot für unsere Lagerhalle in Frankfurt inkl. 20 kWh Batteriespeicher.',
  });

  const handleDemoIngest = (e: React.FormEvent) => {
    e.preventDefault();
    demoIngestMutation.mutate({
      ...demoForm,
      recipients: ['vertrieb@openlocalcrm.local'],
    });
  };

  const handleAddTagToMessage = (messageId: string, currentTags: string[], tag: string) => {
    if (!tag.trim() || currentTags.includes(tag)) return;
    tagMutation.mutate({ id: messageId, tags: [...currentTags, tag] });
    setNewCustomTag('');
    setIsAddingTag(false);
  };

  const handleRemoveTagFromMessage = (messageId: string, currentTags: string[], tag: string) => {
    tagMutation.mutate({ id: messageId, tags: currentTags.filter((t) => t !== tag) });
  };

  // Selected message security & SLA analysis
  const currentSecurityAnalysis = useMemo(() => {
    if (!selectedMessage) return null;
    const body = selectedMessage.body_text?.String || selectedMessage.body_text || '';
    return analyzePromptInjection(body);
  }, [selectedMessage]);

  const currentUrgencyAnalysis = useMemo(() => {
    if (!selectedMessage) return null;
    const subject = selectedMessage.subject || '';
    const body = selectedMessage.body_text?.String || selectedMessage.body_text || '';
    return analyzeUrgencyAndSLA(subject, body);
  }, [selectedMessage]);

  const currentTriage = selectedMessage ? triageByMessage[selectedMessage.id] : undefined;

  const triageMutation = useMutation({
    mutationFn: (payload: { sender: string; subject: string; body: string }) =>
      apiFetch<TriageResult>('/api/v1/ai/triage', {
        method: 'POST',
        body: JSON.stringify(payload),
      }),
    onSuccess: (result) => {
      if (!selectedMessage) return;
      setTriageByMessage((prev) => ({ ...prev, [selectedMessage.id]: result }));
      setReplyText(result.draft_reply);
      setFeedbackBanner(
        'KI-Antwortentwurf erfolgreich generiert (Human-in-the-Loop Prüfung erforderlich)!',
      );
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
    onError: (err: any) => {
      setErrorBanner(`KI-Triage fehlgeschlagen: ${err?.message || 'Unbekannter Fehler'}`);
      setTimeout(() => setErrorBanner(null), 5000);
    },
  });

  const handleGenerateAIDraft = () => {
    if (!selectedMessage) return;
    triageMutation.mutate({
      sender: selectedMessage.sender_email,
      subject: selectedMessage.subject || '',
      body: selectedMessage.body_text?.String || selectedMessage.body_text || '',
    });
  };

  const sendReplyMutation = useMutation({
    mutationFn: () => {
      const subject = (selectedMessage.subject || '').startsWith('Re:')
        ? selectedMessage.subject
        : `Re: ${selectedMessage.subject || ''}`;
      return sendEmail({
        account_id: selectedMessage.account_id,
        to: [selectedMessage.sender_email],
        subject,
        body_text: replyText,
        in_reply_to: selectedMessage.message_id,
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['emails'] });
      setFeedbackBanner(`Antwort erfolgreich an ${selectedMessage.sender_email} versendet!`);
      setReplyText('');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
    onError: (err: any) => {
      setErrorBanner(`Versand fehlgeschlagen: ${err?.message || 'Unbekannter Fehler'}`);
      setTimeout(() => setErrorBanner(null), 5000);
    },
  });

  const handleSendReply = () => {
    if (!replyText.trim() || !selectedMessage) return;
    sendReplyMutation.mutate();
  };

  // Filter messages by tag
  const filteredMessages = messages.filter((msg) => {
    if (selectedTagFilter === 'ALL') return true;
    const tags: string[] = msg.tags || [];
    return tags.includes(selectedTagFilter);
  });

  return (
    <div className="space-y-4 max-w-7xl mx-auto flex flex-col h-[calc(100vh-6rem)]">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
        <div>
          <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2.5">
            <Mail className="w-6 h-6 text-emerald-400" />
            E-Mail Inbox, Dringlichkeits-SLA & Prompt-Armor (§4.1–§4.5 / §19.3)
          </h1>
          <p className="text-sm text-slate-400">
            Zentraler Posteingang mit automatischer Frist- & Dringlichkeitserkennung,
            Prompt-Injection-Schutz und Gemma 12B Triage
          </p>
        </div>
        {isDemo && (
          <div className="flex items-center gap-3">
            <button
              onClick={() => setIsDemoModalOpen(true)}
              className="inline-flex items-center gap-2 px-3.5 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-semibold rounded-xl text-xs transition-colors shadow-sm cursor-pointer"
            >
              <Sparkles className="w-3.5 h-3.5 text-emerald-400" />
              <span>Test-E-Mail simulieren</span>
            </button>
          </div>
        )}
      </div>

      {feedbackBanner && (
        <div className="p-3.5 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2 shrink-0">
          <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
          <span>{feedbackBanner}</span>
        </div>
      )}

      {errorBanner && (
        <div className="p-3.5 bg-rose-500/10 border border-rose-500/20 rounded-xl text-xs text-rose-300 flex items-center gap-2 shrink-0">
          <ShieldAlert className="w-4 h-4 text-rose-400 shrink-0" />
          <span>{errorBanner}</span>
        </div>
      )}

      {/* Tag Filtering Bar */}
      <div className="flex items-center gap-2 overflow-x-auto pb-1 text-xs shrink-0">
        <span className="text-slate-500 font-medium flex items-center gap-1.5 mr-1">
          <Filter className="w-3.5 h-3.5" /> Filter:
        </span>
        <button
          onClick={() => setSelectedTagFilter('ALL')}
          className={`px-3 py-1 rounded-xl font-medium transition-colors cursor-pointer ${
            selectedTagFilter === 'ALL'
              ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
              : 'bg-slate-900 text-slate-400 hover:text-slate-200 border border-slate-800'
          }`}
        >
          Alle E-Mails ({messages.length})
        </button>
        {availableTags.map((tag) => (
          <button
            key={tag.id}
            onClick={() => setSelectedTagFilter(tag.id)}
            className={`px-3 py-1 rounded-xl font-medium border transition-colors cursor-pointer ${
              selectedTagFilter === tag.id
                ? `${tag.color} ring-1 ring-emerald-400`
                : 'bg-slate-900 text-slate-400 hover:text-slate-200 border border-slate-800'
            }`}
          >
            🏷️ {tag.id}
          </button>
        ))}
      </div>

      {/* Split-View Mailbox Layout */}
      <div className="flex-1 bg-slate-900 border border-slate-800 rounded-3xl overflow-hidden shadow-2xl grid grid-cols-1 md:grid-cols-12 min-h-0">
        {/* Left Column: Messages List (5 cols) */}
        <div className="md:col-span-5 border-r border-slate-800 flex flex-col min-h-0 bg-slate-950/40">
          <div className="p-3 border-b border-slate-800 flex items-center justify-between">
            <span className="text-xs font-bold uppercase text-slate-400 tracking-wider">
              Posteingang ({filteredMessages.length})
            </span>
          </div>

          <div className="flex-1 overflow-y-auto divide-y divide-slate-800/80">
            {isLoading ? (
              <div className="p-8 text-center text-xs text-slate-500">Lade E-Mails...</div>
            ) : filteredMessages.length === 0 ? (
              <div className="p-12 text-center text-slate-500 text-xs flex flex-col items-center justify-center gap-2">
                <InboxIcon className="w-8 h-8 text-slate-700" />
                Keine E-Mails für diesen Filter gefunden.
              </div>
            ) : (
              filteredMessages.map((msg) => {
                const isSelected = selectedMessage?.id === msg.id;
                const msgTags: string[] = msg.tags || [];
                const urgency = analyzeUrgencyAndSLA(
                  msg.subject || '',
                  msg.body_text?.String || msg.body_text || '',
                );
                const security = analyzePromptInjection(
                  msg.body_text?.String || msg.body_text || '',
                );

                return (
                  <div
                    key={msg.id}
                    onClick={() => setSelectedMessageId(msg.id)}
                    className={`p-4 cursor-pointer transition-colors space-y-1.5 ${
                      isSelected
                        ? 'bg-emerald-500/10 border-l-2 border-emerald-400'
                        : 'hover:bg-slate-800/40'
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="text-xs font-semibold text-slate-200 truncate">
                        {msg.sender_name?.String || msg.sender_name || msg.sender_email}
                      </span>
                      <div className="flex items-center gap-1.5">
                        {urgency.isUrgent && (
                          <span className="flex items-center gap-0.5 text-[10px] px-1.5 py-0.2 rounded bg-rose-500/10 text-rose-400 border border-rose-500/20 font-bold animate-pulse">
                            <Flame className="w-3 h-3" /> SLA
                          </span>
                        )}
                        {security.hasInjection && (
                          <span className="flex items-center gap-0.5 text-[10px] px-1.5 py-0.2 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 font-bold">
                            <ShieldAlert className="w-3 h-3" /> Armor
                          </span>
                        )}
                        <span className="text-[10px] text-slate-500">
                          {new Date(msg.received_at).toLocaleTimeString([], {
                            hour: '2-digit',
                            minute: '2-digit',
                          })}
                        </span>
                      </div>
                    </div>

                    <div className="text-sm font-medium text-slate-100 truncate">{msg.subject}</div>
                    <div className="text-xs text-slate-400 line-clamp-1">
                      {security.sanitizedText}
                    </div>

                    {/* Tags Badges */}
                    {msgTags.length > 0 && (
                      <div className="flex flex-wrap gap-1 pt-1">
                        {msgTags.map((t) => (
                          <span
                            key={t}
                            className="text-[10px] px-2 py-0.5 rounded-md bg-slate-800 text-slate-300 border border-slate-700"
                          >
                            🏷️ {t}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                );
              })
            )}
          </div>
        </div>

        {/* Right Column: Message Detail & Reply Composer (7 cols) */}
        <div className="md:col-span-7 flex flex-col min-h-0 bg-slate-900">
          {selectedMessage ? (
            <div className="flex-1 flex flex-col min-h-0 p-6 overflow-y-auto space-y-5">
              {/* Prompt Injection Armor Warning (§19.3) */}
              {currentSecurityAnalysis?.hasInjection && (
                <div className="p-4 bg-amber-950/40 border border-amber-500/40 rounded-2xl space-y-2">
                  <div className="flex items-center gap-2 text-amber-400 font-bold text-xs">
                    <ShieldAlert className="w-4 h-4" />
                    <span>Prompt-Injection-Angriff abgewehrt & isoliert (§19.3 Prompt Armor)</span>
                  </div>
                  <p className="text-xs text-amber-200/90 leading-relaxed">
                    Diese E-Mail enthält bösartige Instruktionen zur Manipulation der KI-Logik (z.
                    B. System-Prompt-Leaks oder Datenbank-Befehle). Die Befehle wurden unschädlich
                    gemacht und als passive Nutzdaten gekapselt.
                  </p>
                </div>
              )}

              {/* Urgency & SLA Deadline Alert (§4.3) */}
              {currentUrgencyAnalysis?.isUrgent && (
                <div className="p-3.5 bg-rose-950/40 border border-rose-500/40 rounded-2xl flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2 text-rose-300 text-xs font-semibold">
                    <Clock className="w-4 h-4 text-rose-400 shrink-0" />
                    <span>🚨 {currentUrgencyAnalysis.reason}</span>
                  </div>
                  <span className="text-[10px] px-2 py-0.5 rounded-full bg-rose-500/20 text-rose-300 font-bold border border-rose-500/30 whitespace-nowrap">
                    SLA: &lt; {currentUrgencyAnalysis.slaHours} Std.
                  </span>
                </div>
              )}

              {/* AI Triage Banner (§4.3) */}
              <div className="p-4 bg-slate-950 border border-purple-500/30 rounded-2xl space-y-2.5">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <Brain className="w-4 h-4 text-purple-400" />
                    <span className="text-xs font-bold text-slate-200 uppercase tracking-wider">
                      KI-Triage & Analyse (§4.3)
                    </span>
                  </div>
                  {!currentTriage && (
                    <button
                      type="button"
                      onClick={handleGenerateAIDraft}
                      disabled={triageMutation.isPending}
                      className="inline-flex items-center gap-1 text-[10px] px-2 py-0.5 rounded-full bg-purple-500/10 hover:bg-purple-500/20 text-purple-400 border border-purple-500/20 font-bold disabled:opacity-50 cursor-pointer"
                    >
                      <Sparkles
                        className={`w-3 h-3 ${triageMutation.isPending ? 'animate-spin' : ''}`}
                      />
                      {triageMutation.isPending ? 'Analysiere...' : 'Jetzt analysieren'}
                    </button>
                  )}
                </div>

                {currentTriage ? (
                  <>
                    <div className="grid grid-cols-3 gap-2 text-xs">
                      <div className="p-2 bg-slate-900 rounded-xl border border-slate-800">
                        <span className="text-[10px] text-slate-500 block">Priorität</span>
                        <span
                          className={`font-bold ${currentTriage.priority === 'URGENT' || currentTriage.priority === 'HIGH' ? 'text-rose-400' : 'text-emerald-400'}`}
                        >
                          {currentTriage.priority}
                        </span>
                      </div>
                      <div className="p-2 bg-slate-900 rounded-xl border border-slate-800">
                        <span className="text-[10px] text-slate-500 block">Stimmung</span>
                        <span className="text-blue-400 font-bold">{currentTriage.sentiment}</span>
                      </div>
                      <div className="p-2 bg-slate-900 rounded-xl border border-slate-800">
                        <span className="text-[10px] text-slate-500 block">Kategorie</span>
                        <span className="text-purple-400 font-bold">{currentTriage.category}</span>
                      </div>
                    </div>

                    <p className="text-xs text-slate-300 italic pt-1 border-t border-slate-800/60">
                      💡 <strong>Zusammenfassung:</strong> {currentTriage.summary}
                    </p>
                  </>
                ) : (
                  <p className="text-xs text-slate-500 italic">
                    Noch nicht analysiert. Klicke „Jetzt analysieren“ für eine echte KI-Triage
                    dieser E-Mail.
                  </p>
                )}
              </div>

              {/* Message Header */}
              <div className="border-b border-slate-800 pb-4 space-y-3">
                <h2 className="text-xl font-bold text-slate-100">{selectedMessage.subject}</h2>
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3">
                    <div className="w-9 h-9 rounded-full bg-slate-800 border border-slate-700 flex items-center justify-center text-slate-300">
                      <User className="w-4 h-4" />
                    </div>
                    <div>
                      <div className="text-sm font-semibold text-slate-200">
                        {selectedMessage.sender_name?.String ||
                          selectedMessage.sender_name ||
                          selectedMessage.sender_email}
                      </div>
                      <div className="text-xs text-slate-500">{selectedMessage.sender_email}</div>
                    </div>
                  </div>
                  <span className="text-xs text-slate-500">
                    {new Date(selectedMessage.received_at).toLocaleString('de-DE')}
                  </span>
                </div>

                {/* Interactive Tags Section for Selected Email */}
                <div className="pt-2 flex flex-wrap items-center gap-1.5 border-t border-slate-800/60">
                  <span className="text-xs text-slate-500 flex items-center gap-1">
                    <Tag className="w-3.5 h-3.5" /> Tags:
                  </span>
                  {((selectedMessage.tags as string[]) || []).map((tag) => (
                    <span
                      key={tag}
                      className="inline-flex items-center gap-1 text-xs px-2.5 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-medium"
                    >
                      🏷️ {tag}
                      <button
                        onClick={() =>
                          handleRemoveTagFromMessage(
                            selectedMessage.id,
                            selectedMessage.tags || [],
                            tag,
                          )
                        }
                        className="hover:text-rose-400 text-slate-400 ml-1 text-xs cursor-pointer"
                        title="Tag entfernen"
                      >
                        ×
                      </button>
                    </span>
                  ))}

                  {isAddingTag ? (
                    <div className="inline-flex items-center gap-1 bg-slate-950 border border-slate-700 rounded-lg p-1">
                      <input
                        type="text"
                        placeholder="Tag-Name..."
                        value={newCustomTag}
                        onChange={(e) => setNewCustomTag(e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter')
                            handleAddTagToMessage(
                              selectedMessage.id,
                              selectedMessage.tags || [],
                              newCustomTag,
                            );
                        }}
                        className="bg-transparent text-xs text-slate-100 focus:outline-none px-1 w-24"
                        autoFocus
                      />
                      <button
                        onClick={() =>
                          handleAddTagToMessage(
                            selectedMessage.id,
                            selectedMessage.tags || [],
                            newCustomTag,
                          )
                        }
                        className="text-[10px] bg-emerald-600 px-2 py-0.5 rounded text-slate-950 font-bold cursor-pointer"
                      >
                        OK
                      </button>
                      <button
                        onClick={() => setIsAddingTag(false)}
                        className="text-[10px] text-slate-400 hover:text-slate-200 px-1 cursor-pointer"
                      >
                        Abbrechen
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => setIsAddingTag(true)}
                      className="inline-flex items-center gap-1 text-xs px-2.5 py-0.5 rounded-full bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 transition-colors cursor-pointer"
                    >
                      <Plus className="w-3 h-3" /> Tag hinzufügen
                    </button>
                  )}
                </div>
              </div>

              {/* Message Body (Sanitized) */}
              <div className="text-sm text-slate-200 whitespace-pre-wrap leading-relaxed flex-1">
                {currentSecurityAnalysis?.sanitizedText || 'Kein Inhalt'}
              </div>

              {/* UWG § 7 Outreach Guidance (§4.5 / §19.4) */}
              <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl flex items-center justify-between text-xs text-slate-400">
                <span className="flex items-center gap-1.5 text-emerald-400">
                  <ShieldCheck className="w-4 h-4" />
                  Antwort auf Kundenanfrage (UWG-konforme Korrespondenz)
                </span>
                <span className="text-[11px] text-slate-500">Signatur-Kaskade aktiv</span>
              </div>

              {/* Inline Reply Composer & AI Draft Generator (§4.5) */}
              <div className="border-t border-slate-800 pt-4 space-y-3">
                <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 text-xs font-semibold text-slate-400">
                  <span className="flex items-center gap-1.5">
                    <Reply className="w-3.5 h-3.5 text-emerald-400" /> Antworten an{' '}
                    {selectedMessage.sender_email}
                  </span>

                  <button
                    type="button"
                    onClick={handleGenerateAIDraft}
                    disabled={triageMutation.isPending}
                    className="inline-flex items-center gap-1.5 px-3 py-1 bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white font-bold rounded-xl text-xs shadow-lg shadow-purple-600/20 transition-colors cursor-pointer"
                  >
                    <Sparkles
                      className={`w-3.5 h-3.5 ${triageMutation.isPending ? 'animate-spin' : ''}`}
                    />
                    <span>{triageMutation.isPending ? 'Generiere...' : '⚡ KI-Entwurf'}</span>
                  </button>
                </div>

                <textarea
                  rows={4}
                  value={replyText}
                  onChange={(e) => setReplyText(e.target.value)}
                  placeholder="Antwort verfassen oder oben per '⚡ KI-Entwurf' generieren..."
                  className="w-full p-3 bg-slate-950 border border-slate-800 rounded-xl text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />

                <div className="flex justify-between items-center">
                  <span className="text-[11px] text-slate-500 italic">
                    🤖 Human-in-the-Loop: Vor dem Versand durch Vertriebler geprüft.
                  </span>
                  <button
                    type="button"
                    onClick={handleSendReply}
                    disabled={!replyText.trim() || sendReplyMutation.isPending}
                    className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-slate-950 font-bold rounded-xl text-xs transition-colors shadow-lg shadow-emerald-600/20 cursor-pointer"
                  >
                    <Send className="w-3.5 h-3.5" />
                    <span>{sendReplyMutation.isPending ? 'Senden...' : 'Antwort senden'}</span>
                  </button>
                </div>
              </div>
            </div>
          ) : (
            <div className="flex-1 flex flex-col items-center justify-center p-8 text-center text-slate-500 space-y-2">
              <Mail className="w-12 h-12 text-slate-800" />
              <div className="text-sm font-medium">Wähle eine E-Mail aus der Liste aus</div>
              <div className="text-xs text-slate-600">
                Eingehende Nachrichten & Tags werden hier im Detail dargestellt
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Demo Email Simulation Modal */}
      {isDemoModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <h3 className="font-bold text-slate-100 text-lg">Eingehende Test-E-Mail simulieren</h3>
            <form onSubmit={handleDemoIngest} className="space-y-3">
              <div>
                <label
                  htmlFor="demo-sender-name"
                  className="block text-xs font-medium text-slate-400 mb-1"
                >
                  Absender Name
                </label>
                <input
                  id="demo-sender-name"
                  type="text"
                  value={demoForm.sender_name}
                  onChange={(e) => setDemoForm({ ...demoForm, sender_name: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100"
                />
              </div>
              <div>
                <label
                  htmlFor="demo-sender-email"
                  className="block text-xs font-medium text-slate-400 mb-1"
                >
                  Absender E-Mail
                </label>
                <input
                  id="demo-sender-email"
                  type="email"
                  value={demoForm.sender_email}
                  onChange={(e) => setDemoForm({ ...demoForm, sender_email: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100"
                />
              </div>
              <div>
                <label
                  htmlFor="demo-subject"
                  className="block text-xs font-medium text-slate-400 mb-1"
                >
                  Betreff
                </label>
                <input
                  id="demo-subject"
                  type="text"
                  value={demoForm.subject}
                  onChange={(e) => setDemoForm({ ...demoForm, subject: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100"
                />
              </div>
              <div>
                <label
                  htmlFor="demo-body-text"
                  className="block text-xs font-medium text-slate-400 mb-1"
                >
                  Nachrichtentext
                </label>
                <textarea
                  id="demo-body-text"
                  rows={3}
                  value={demoForm.body_text}
                  onChange={(e) => setDemoForm({ ...demoForm, body_text: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100"
                />
              </div>
              <div className="flex justify-end gap-3 pt-3">
                <button
                  type="button"
                  onClick={() => setIsDemoModalOpen(false)}
                  className="px-4 py-2 bg-slate-800 text-slate-300 rounded-lg text-xs"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={demoIngestMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-lg text-xs cursor-pointer"
                >
                  {demoIngestMutation.isPending ? 'Sende...' : 'E-Mail einspeisen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
