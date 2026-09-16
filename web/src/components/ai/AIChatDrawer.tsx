import React, { useState, useRef, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { Bot, X, Send, Sparkles, ShieldCheck, Minimize2, Maximize2, Zap, ArrowRight } from 'lucide-react';
import { apiFetch } from '../../api/client';
import { useQueryClient } from '@tanstack/react-query';

interface ActionCard {
  type: string;
  title: string;
  badge: string;
  route: string;
}

interface Message {
  id: string;
  role: 'user' | 'assistant';
  content: string;
  timestamp: string;
  actionCard?: ActionCard;
}

export const AIChatDrawer: React.FC = () => {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [isOpen, setIsOpen] = useState(false);
  const [isExpanded, setIsExpanded] = useState(false);
  const [input, setInput] = useState('');
  const [loading, setLoading] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  const [messages, setMessages] = useState<Message[]>([
    {
      id: 'm-welcome',
      role: 'assistant',
      content: 'Hallo! Ich bin Ihr OpenLocalCRM KI-Vertriebs-Copilot (Gemma 12B). Sie können mich direkt anweisen, Automatisierungen (§6.4), Tags, Deals oder E-Mail-Vorlagen im System anzulegen.',
      timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    },
  ]);

  useEffect(() => {
    if (isOpen) {
      messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, isOpen]);

  const handleSend = async (e?: React.FormEvent, customPrompt?: string) => {
    if (e) e.preventDefault();
    const promptToSend = customPrompt || input;
    if (!promptToSend.trim() || loading) return;

    const userMsg: Message = {
      id: 'u-' + Date.now(),
      role: 'user',
      content: promptToSend,
      timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    };

    setMessages((prev) => [...prev, userMsg]);
    if (!customPrompt) setInput('');
    setLoading(true);

    try {
      const res = await apiFetch<any>('/api/v1/ai/chat', {
        method: 'POST',
        body: JSON.stringify({
          messages: [...messages, userMsg].map((m) => ({ role: m.role, content: m.content })),
          context: '5 aktive Deals (106.700 € Pipeline-Volumen), 3 Kontakte in Frankfurt',
        }),
      });

      const assistantMsg: Message = {
        id: 'a-' + Date.now(),
        role: 'assistant',
        content: res.reply || 'Aktion erfolgreich ausgeführt.',
        actionCard: res.actionCard,
        timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
      };
      setMessages((prev) => [...prev, assistantMsg]);

      // Invalidate relevant React Query caches if an entity was created
      if (res.actionCard) {
        queryClient.invalidateQueries({ queryKey: ['automations'] });
        queryClient.invalidateQueries({ queryKey: ['deals'] });
        queryClient.invalidateQueries({ queryKey: ['contacts'] });
        queryClient.invalidateQueries({ queryKey: ['emails'] });
      }
    } catch {
      setMessages((prev) => [
        ...prev,
        {
          id: 'err-' + Date.now(),
          role: 'assistant',
          content: 'Entschuldigung, die KI-Verbindung konnte nicht hergestellt werden.',
          timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        },
      ]);
    } finally {
      setLoading(false);
    }
  };

  const quickChips = [
    '⚡ Workflow für PV-Leads anlegen',
    '🏷️ Tag "Gewerbe-PV" erstellen',
    '🤝 Deal für Dr. Weber anlegen',
    '📝 E-Mail-Vorlage generieren',
    'Fasse die Pipeline zusammen',
  ];

  return (
    <>
      {/* Floating Action Button */}
      {!isOpen && (
        <button
          onClick={() => setIsOpen(true)}
          className="fixed bottom-6 right-6 z-40 p-3.5 bg-gradient-to-r from-emerald-600 to-teal-500 hover:from-emerald-500 hover:to-teal-400 text-slate-950 font-bold rounded-full shadow-2xl shadow-emerald-500/30 flex items-center gap-2.5 transition-all hover:scale-105 border border-emerald-400/30"
          aria-label="KI-Assistent öffnen"
        >
          <Bot className="w-6 h-6" />
          <span className="text-xs font-bold pr-1 hidden sm:inline">KI-Copilot</span>
        </button>
      )}

      {/* Floating Chat Drawer Window */}
      {isOpen && (
        <div
          className={`fixed z-50 transition-all duration-200 bg-slate-900 border border-slate-800 shadow-2xl rounded-2xl flex flex-col overflow-hidden ${
            isExpanded
              ? 'inset-4 sm:inset-10'
              : 'bottom-6 right-6 w-[95vw] sm:w-[440px] h-[600px] max-h-[85vh]'
          }`}
        >
          {/* Header */}
          <div className="px-4 py-3.5 bg-slate-950 border-b border-slate-800 flex items-center justify-between">
            <div className="flex items-center gap-2.5">
              <div className="w-8 h-8 rounded-xl bg-emerald-500/10 text-emerald-400 flex items-center justify-center border border-emerald-500/20">
                <Bot className="w-4 h-4" />
              </div>
              <div>
                <div className="text-xs font-bold text-slate-100 flex items-center gap-1.5">
                  OpenLocalCRM KI-Copilot
                  <span className="text-[9px] px-1.5 py-0.2 rounded bg-emerald-500/10 text-emerald-400 font-mono">
                    Gemma 12B
                  </span>
                </div>
                <div className="text-[10px] text-slate-400 flex items-center gap-1">
                  <ShieldCheck className="w-3 h-3 text-emerald-400 inline" /> PII-Schutz aktiv
                </div>
              </div>
            </div>

            <div className="flex items-center gap-1 text-slate-400">
              <button
                onClick={() => setIsExpanded(!isExpanded)}
                className="p-1.5 hover:text-slate-200 rounded-lg hover:bg-slate-800"
                title={isExpanded ? 'Verkleinern' : 'Vergrößern'}
              >
                {isExpanded ? <Minimize2 className="w-4 h-4" /> : <Maximize2 className="w-4 h-4" />}
              </button>
              <button
                onClick={() => setIsOpen(false)}
                className="p-1.5 hover:text-slate-200 rounded-lg hover:bg-slate-800"
                title="Schließen"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Messages Stream */}
          <div className="flex-1 overflow-y-auto p-4 space-y-3.5 bg-slate-950/40">
            {messages.map((msg) => (
              <div
                key={msg.id}
                className={`flex gap-2.5 ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}
              >
                {msg.role === 'assistant' && (
                  <div className="w-6 h-6 rounded-lg bg-emerald-500/10 text-emerald-400 flex items-center justify-center text-xs shrink-0 mt-0.5 border border-emerald-500/20">
                    <Sparkles className="w-3.5 h-3.5" />
                  </div>
                )}
                <div
                  className={`max-w-[85%] rounded-2xl px-3.5 py-2.5 text-xs leading-relaxed shadow-sm ${
                    msg.role === 'user'
                      ? 'bg-emerald-600 text-slate-950 font-medium rounded-tr-none'
                      : 'bg-slate-800/90 text-slate-200 border border-slate-700/60 rounded-tl-none'
                  }`}
                >
                  <div className="whitespace-pre-wrap">{msg.content}</div>

                  {/* Interactive Action Result Card */}
                  {msg.actionCard && (
                    <div className="mt-2.5 p-2.5 bg-slate-900 border border-emerald-500/30 rounded-xl flex items-center justify-between gap-2 shadow-lg animate-fadeIn">
                      <div className="flex items-center gap-2 min-w-0">
                        <div className="w-6 h-6 rounded-lg bg-emerald-500/20 text-emerald-400 flex items-center justify-center shrink-0">
                          <Zap className="w-3.5 h-3.5" />
                        </div>
                        <div className="min-w-0">
                          <div className="text-[11px] font-bold text-slate-100 truncate">{msg.actionCard.title}</div>
                          <div className="text-[9px] text-emerald-400 font-mono font-bold">{msg.actionCard.badge}</div>
                        </div>
                      </div>
                      <button
                        onClick={() => {
                          if (msg.actionCard) {
                            navigate(msg.actionCard.route);
                            setIsOpen(false);
                          }
                        }}
                        className="px-2.5 py-1 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-lg text-[10px] shrink-0 flex items-center gap-1 transition-colors"
                      >
                        <span>Öffnen</span>
                        <ArrowRight className="w-3 h-3" />
                      </button>
                    </div>
                  )}

                  <div
                    className={`text-[9px] mt-1 text-right ${
                      msg.role === 'user' ? 'text-emerald-950/70' : 'text-slate-500'
                    }`}
                  >
                    {msg.timestamp}
                  </div>
                </div>
              </div>
            ))}

            {loading && (
              <div className="flex items-center gap-2 text-xs text-emerald-400 font-medium py-1">
                <Sparkles className="w-4 h-4 animate-spin" />
                <span>Gemma 12B führt Anweisung aus...</span>
              </div>
            )}
            <div ref={messagesEndRef} />
          </div>

          {/* Quick Prompts */}
          <div className="px-3 py-2 bg-slate-900 border-t border-slate-800/80 flex gap-1.5 overflow-x-auto text-[11px]">
            {quickChips.map((chip, idx) => (
              <button
                key={idx}
                onClick={() => handleSend(undefined, chip)}
                className="whitespace-nowrap px-2.5 py-1 rounded-full bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 transition-colors"
              >
                {chip}
              </button>
            ))}
          </div>

          {/* Input Footer */}
          <form onSubmit={handleSend} className="p-3 bg-slate-950 border-t border-slate-800 flex gap-2">
            <input
              type="text"
              placeholder="Frage an den Copilot stellen (z.B. 'Workflow anlegen', 'Tag erstellen')..."
              value={input}
              onChange={(e) => setInput(e.target.value)}
              className="flex-1 px-3 py-2 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
            />
            <button
              type="submit"
              disabled={loading || !input.trim()}
              className="p-2 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-40 text-slate-950 font-bold rounded-xl transition-colors shrink-0"
            >
              <Send className="w-4 h-4" />
            </button>
          </form>
        </div>
      )}
    </>
  );
};
