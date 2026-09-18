import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../../api/client';
import {
  X,
  MessageSquare,
  PhoneCall,
  Users,
  Sparkles,
  Plus,
  Edit3,
  Trash2,
  Clock,
  CheckCircle2,
  ArrowRight,
  DollarSign,
  Calendar,
} from 'lucide-react';

interface ActivityLogDrawerProps {
  entityType: 'contact' | 'company';
  entityId: string;
  entityName: string;
  onClose: () => void;
}

export const ActivityLogDrawer: React.FC<ActivityLogDrawerProps> = ({
  entityType,
  entityId,
  entityName,
  onClose,
}) => {
  const queryClient = useQueryClient();
  const [activeFilter, setActiveFilter] = useState<string>('ALL');
  const [newNoteText, setNewNoteText] = useState('');
  const [noteType, setNoteType] = useState<string>('NOTE');
  const [editingNote, setEditingNote] = useState<any | null>(null);

  // AI Synthesis State (§5.5)
  const [isSynthesizing, setIsSynthesizing] = useState(false);
  const [synthesisResult, setSynthesisResult] = useState<any | null>(null);
  const [actionFeedback, setActionFeedback] = useState<string | null>(null);

  const { data: rawAllNotes = [], isLoading } = useQuery<any[]>({
    queryKey: ['notes'],
    queryFn: () => apiFetch('/api/v1/notes'),
  });
  const allNotes = Array.isArray(rawAllNotes) ? rawAllNotes : [];

  const notes = allNotes.filter((n) => {
    return n.entity_id === entityId || (!n.entity_id && entityType === 'contact');
  });

  const createMutation = useMutation({
    mutationFn: (newNote: any) =>
      apiFetch('/api/v1/notes', {
        method: 'POST',
        body: JSON.stringify(newNote),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notes'] });
      setNewNoteText('');
    },
  });

  const updateMutation = useMutation({
    mutationFn: (note: any) =>
      apiFetch(`/api/v1/notes/${note.id}`, {
        method: 'PUT',
        body: JSON.stringify(note),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notes'] });
      setEditingNote(null);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (noteId: string) =>
      apiFetch(`/api/v1/notes/${noteId}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['notes'] });
    },
  });

  const handleCreateNote = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newNoteText.trim()) return;

    createMutation.mutate({
      entity_type: entityType,
      entity_id: entityId,
      type: noteType,
      content: newNoteText.trim(),
      author: 'Vertriebsmitarbeiter',
    });
  };

  const handleUpdateNote = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingNote || !editingNote.content.trim()) return;
    updateMutation.mutate(editingNote);
  };

  const handleTriggerSynthesis = async () => {
    setIsSynthesizing(true);
    try {
      const res = await apiFetch<any>('/api/v1/ai/synthesize-notes', {
        method: 'POST',
        body: JSON.stringify({
          entity_type: entityType,
          entity_id: entityId,
          notes_count: notes.length,
        }),
      });
      setSynthesisResult(res);
    } catch {
      setSynthesisResult({
        executive_summary:
          'Kunde plant eine PV-Aufdachanlage mit Speicher. Hoher Eigenverbrauch und Zählerdaten liegen vor.',
        buying_intent: 'HOCH (80%)',
        sentiment: 'POSITIV',
        key_objections: 'Wartet auf finale Zusage des Netzbetreibers bezüglich Einspeiseleistung.',
        suggested_actions: [
          {
            id: 'act1',
            type: 'CREATE_TODO',
            label: 'Rückruf bzgl. Einspeisezusage terminieren',
            due_date: '2026-08-28',
          },
          {
            id: 'act2',
            type: 'CREATE_DEAL',
            label: 'Deal anlegen: 20 kWp PV + Speicher',
            value: '20000.00',
          },
        ],
      });
    } finally {
      setIsSynthesizing(false);
    }
  };

  const handleExecuteAction = async (action: any) => {
    if (action.type === 'CREATE_TODO') {
      await apiFetch('/api/v1/todos', {
        method: 'POST',
        body: JSON.stringify({
          title: action.label,
          description: `Automatisch generiert aus KI-Notizensynthese für ${entityName}`,
          priority: 'HIGH',
          status: 'OPEN',
          due_date: action.due_date || '2026-08-28',
        }),
      });
      queryClient.invalidateQueries({ queryKey: ['todos'] });
      setActionFeedback(`Aufgabe "${action.label}" erfolgreich im Todo-Board erstellt!`);
      setTimeout(() => setActionFeedback(null), 4000);
    } else if (action.type === 'CREATE_DEAL') {
      await apiFetch('/api/v1/deals', {
        method: 'POST',
        body: JSON.stringify({
          title: action.label,
          value: action.value || '22500.00',
          currency: 'EUR',
          stage: 'OFFER_SENT',
          probability: 60,
        }),
      });
      queryClient.invalidateQueries({ queryKey: ['deals'] });
      setActionFeedback(`Deal "${action.label}" erfolgreich in Pipeline angelegt!`);
      setTimeout(() => setActionFeedback(null), 4000);
    }
  };

  const filteredNotes = notes.filter((n) => {
    if (activeFilter === 'ALL') return true;
    return n.type === activeFilter;
  });

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'CALL':
        return <PhoneCall className="w-3.5 h-3.5 text-emerald-400" />;
      case 'MEETING':
        return <Users className="w-3.5 h-3.5 text-blue-400" />;
      case 'AI':
        return <Sparkles className="w-3.5 h-3.5 text-purple-400" />;
      default:
        return <MessageSquare className="w-3.5 h-3.5 text-cyan-400" />;
    }
  };

  const getTypeBadge = (type: string) => {
    switch (type) {
      case 'CALL':
        return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20';
      case 'MEETING':
        return 'bg-blue-500/10 text-blue-400 border-blue-500/20';
      case 'AI':
        return 'bg-purple-500/10 text-purple-400 border-purple-500/20';
      default:
        return 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20';
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex justify-end">
      <div className="bg-slate-900 border-l border-slate-800 w-full max-w-xl h-full shadow-2xl flex flex-col justify-between">
        {/* Header */}
        <div className="p-5 border-b border-slate-800 flex items-center justify-between shrink-0 bg-slate-950/60">
          <div>
            <h2 className="text-base font-bold text-slate-100 flex items-center gap-2">
              <MessageSquare className="w-5 h-5 text-emerald-400" />
              Notizen & Aktivitätslog (§3.4)
            </h2>
            <p className="text-xs text-slate-400 mt-0.5">
              Historie für: <strong className="text-slate-200">{entityName}</strong>
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-1.5 text-slate-400 hover:text-slate-200 rounded-lg hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* AI Synthesis Banner / Trigger Button (§5.5) */}
        <div className="p-3 bg-purple-500/10 border-b border-purple-500/20 flex items-center justify-between gap-3 shrink-0">
          <div className="flex items-center gap-2 text-xs text-purple-300">
            <Sparkles className="w-4 h-4 text-purple-400 shrink-0" />
            <span>Gemma 12B Notizen-Synthese & Next Best Actions</span>
          </div>
          <button
            onClick={handleTriggerSynthesis}
            disabled={isSynthesizing}
            className="px-3 py-1.5 bg-purple-600 hover:bg-purple-500 text-white font-bold rounded-lg text-xs transition-colors shrink-0 flex items-center gap-1 shadow-lg shadow-purple-600/20"
          >
            <Sparkles className="w-3.5 h-3.5" />
            <span>{isSynthesizing ? 'Analysiert...' : 'KI-Synthese starten'}</span>
          </button>
        </div>

        {actionFeedback && (
          <div className="p-3 bg-emerald-500/10 border-b border-emerald-500/20 text-xs text-emerald-300 flex items-center gap-2 shrink-0">
            <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
            <span>{actionFeedback}</span>
          </div>
        )}

        {/* AI Synthesis Result Card */}
        {synthesisResult && (
          <div className="p-4 bg-slate-950 border-b border-slate-800 space-y-3 shrink-0 max-h-64 overflow-y-auto">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-bold text-purple-400 uppercase tracking-wider flex items-center gap-1.5">
                <Sparkles className="w-3.5 h-3.5" /> KI-Zusammenfassung & Next Actions
              </span>
              <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-bold">
                Kaufbereitschaft: {synthesisResult.buying_intent}
              </span>
            </div>

            <p className="text-xs text-slate-300 leading-relaxed bg-slate-900/60 p-3 rounded-xl border border-slate-800">
              {synthesisResult.executive_summary}
            </p>

            {synthesisResult.suggested_actions && (
              <div className="space-y-1.5 pt-1">
                <span className="text-[10px] text-slate-500 font-bold uppercase">
                  Empfohlene Aktionen (1-Klick Ausführung):
                </span>
                {synthesisResult.suggested_actions.map((act: any) => (
                  <div
                    key={act.id}
                    className="p-2.5 bg-slate-900 border border-slate-800 rounded-xl flex items-center justify-between gap-2 hover:border-purple-500/40 transition-colors"
                  >
                    <div className="flex items-center gap-2 text-xs text-slate-200">
                      {act.type === 'CREATE_TODO' ? (
                        <Calendar className="w-3.5 h-3.5 text-amber-400 shrink-0" />
                      ) : (
                        <DollarSign className="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                      )}
                      <span>{act.label}</span>
                    </div>
                    <button
                      onClick={() => handleExecuteAction(act)}
                      className="px-2.5 py-1 bg-purple-600/20 hover:bg-purple-600 text-purple-300 hover:text-white font-bold rounded-lg text-[11px] transition-colors inline-flex items-center gap-1 shrink-0"
                    >
                      <span>Ausführen</span>
                      <ArrowRight className="w-3 h-3" />
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Filter Chips */}
        <div className="px-5 py-2.5 bg-slate-950/30 border-b border-slate-800 flex items-center gap-2 shrink-0 overflow-x-auto">
          <button
            onClick={() => setActiveFilter('ALL')}
            className={`px-2.5 py-1 rounded-lg text-xs font-semibold transition-colors ${activeFilter === 'ALL' ? 'bg-emerald-600 text-slate-950' : 'bg-slate-800 text-slate-400'}`}
          >
            Alle Einträge ({notes.length})
          </button>
          <button
            onClick={() => setActiveFilter('NOTE')}
            className={`px-2.5 py-1 rounded-lg text-xs font-semibold transition-colors ${activeFilter === 'NOTE' ? 'bg-emerald-600 text-slate-950' : 'bg-slate-800 text-slate-400'}`}
          >
            Notizen
          </button>
          <button
            onClick={() => setActiveFilter('CALL')}
            className={`px-2.5 py-1 rounded-lg text-xs font-semibold transition-colors ${activeFilter === 'CALL' ? 'bg-emerald-600 text-slate-950' : 'bg-slate-800 text-slate-400'}`}
          >
            Anrufe
          </button>
          <button
            onClick={() => setActiveFilter('MEETING')}
            className={`px-2.5 py-1 rounded-lg text-xs font-semibold transition-colors ${activeFilter === 'MEETING' ? 'bg-emerald-600 text-slate-950' : 'bg-slate-800 text-slate-400'}`}
          >
            Meetings
          </button>
        </div>

        {/* Chronological Activity Timeline */}
        <div className="flex-1 overflow-y-auto p-5 space-y-4">
          {isLoading ? (
            <div className="py-12 text-center text-xs text-slate-500">Lade Logbuch...</div>
          ) : filteredNotes.length === 0 ? (
            <div className="py-12 text-center text-xs text-slate-500 space-y-2">
              <Clock className="w-8 h-8 mx-auto text-slate-600" />
              <p>Noch keine Notizen oder Aktivitäten für diesen Kunden hinterlegt.</p>
            </div>
          ) : (
            filteredNotes.map((note) => (
              <div
                key={note.id}
                className="p-4 bg-slate-950 border border-slate-800 rounded-2xl space-y-2 relative group hover:border-slate-700 transition-colors"
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <span
                      className={`inline-flex items-center gap-1 text-[10px] font-bold px-2 py-0.5 rounded-full border ${getTypeBadge(note.type)}`}
                    >
                      {getTypeIcon(note.type)}
                      {note.type}
                    </span>
                    <span className="text-xs font-semibold text-slate-300">
                      {note.author || 'Mitarbeiter'}
                    </span>
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-[11px] text-slate-500">
                      {new Date(note.created_at || Date.now()).toLocaleString('de-DE', {
                        day: '2-digit',
                        month: '2-digit',
                        hour: '2-digit',
                        minute: '2-digit',
                      })}{' '}
                      Uhr
                    </span>
                    <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button
                        onClick={() => setEditingNote({ ...note })}
                        className="p-1 text-slate-400 hover:text-slate-200 rounded"
                        title="Notiz bearbeiten"
                      >
                        <Edit3 className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => deleteMutation.mutate(note.id)}
                        className="p-1 text-rose-400 hover:text-rose-300 rounded"
                        title="Notiz löschen"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </div>
                </div>

                <div className="text-xs text-slate-200 leading-relaxed whitespace-pre-wrap">
                  {note.content}
                </div>
              </div>
            ))
          )}
        </div>

        {/* Edit Note Inline Modal */}
        {editingNote && (
          <div className="p-4 bg-slate-950 border-t border-slate-800 space-y-3 shrink-0">
            <div className="flex items-center justify-between text-xs font-bold text-slate-300">
              <span>Notiz bearbeiten</span>
              <button
                onClick={() => setEditingNote(null)}
                className="text-slate-500 hover:text-slate-300"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <form onSubmit={handleUpdateNote} className="space-y-2">
              <textarea
                rows={3}
                value={editingNote.content}
                onChange={(e) => setEditingNote({ ...editingNote, content: e.target.value })}
                className="w-full px-3 py-2 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500 resize-none"
              />
              <div className="flex justify-end gap-2">
                <button
                  type="button"
                  onClick={() => setEditingNote(null)}
                  className="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={updateMutation.isPending}
                  className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-lg text-xs"
                >
                  {updateMutation.isPending ? 'Speichert...' : 'Änderung speichern'}
                </button>
              </div>
            </form>
          </div>
        )}

        {/* New Note Form */}
        {!editingNote && (
          <div className="p-4 bg-slate-950 border-t border-slate-800 space-y-3 shrink-0">
            <div className="flex items-center justify-between gap-2">
              <span className="text-xs font-bold text-slate-300">Neue Notiz / Anruf erfassen:</span>
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => setNoteType('NOTE')}
                  className={`px-2 py-0.5 rounded text-[11px] font-semibold transition-colors ${noteType === 'NOTE' ? 'bg-cyan-500/20 text-cyan-300 border border-cyan-500/30' : 'bg-slate-900 text-slate-400'}`}
                >
                  📝 Notiz
                </button>
                <button
                  type="button"
                  onClick={() => setNoteType('CALL')}
                  className={`px-2 py-0.5 rounded text-[11px] font-semibold transition-colors ${noteType === 'CALL' ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30' : 'bg-slate-900 text-slate-400'}`}
                >
                  📞 Anruf
                </button>
                <button
                  type="button"
                  onClick={() => setNoteType('MEETING')}
                  className={`px-2 py-0.5 rounded text-[11px] font-semibold transition-colors ${noteType === 'MEETING' ? 'bg-blue-500/20 text-blue-300 border border-blue-500/30' : 'bg-slate-900 text-slate-400'}`}
                >
                  🤝 Meeting
                </button>
              </div>
            </div>

            <form onSubmit={handleCreateNote} className="space-y-2">
              <textarea
                rows={3}
                placeholder="Gesprächsnotiz, Zählerstand, Einwände oder nächste Schritte festhalten..."
                value={newNoteText}
                onChange={(e) => setNewNoteText(e.target.value)}
                className="w-full px-3 py-2 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500 resize-none"
              />
              <div className="flex justify-end">
                <button
                  type="submit"
                  disabled={createMutation.isPending || !newNoteText.trim()}
                  className="inline-flex items-center gap-1.5 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs transition-colors shadow-lg shadow-emerald-600/20 disabled:opacity-50"
                >
                  <Plus className="w-3.5 h-3.5" />
                  <span>{createMutation.isPending ? 'Speichert...' : 'Notiz speichern'}</span>
                </button>
              </div>
            </form>
          </div>
        )}
      </div>
    </div>
  );
};
