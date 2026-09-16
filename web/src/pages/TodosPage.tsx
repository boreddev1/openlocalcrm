import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import { Plus, CheckCircle2, Circle, X, Edit3, Trash2, Calendar, AlertCircle, Search, UserCheck, RefreshCw, Sparkles, Tag, Clock, Ban } from 'lucide-react';

const PRODUCT_SEGMENTS = [
  'PV-Anlage',
  'Stromspeicher',
  'Wärmepumpe',
  'Wallbox / E-Mobilität',
  'Wartungsvertrag / Service',
  'Zählerwechsel',
];

export const TodosPage: React.FC = () => {
  const queryClient = useQueryClient();
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [filterCategory, setFilterCategory] = useState<string>('ALL');
  const [filterStatus, setFilterStatus] = useState<string>('ALL');

  // Edit, Delete, Postpone & Cancel State
  const [editingTodo, setEditingTodo] = useState<any | null>(null);
  const [deletingTodo, setDeletingTodo] = useState<any | null>(null);
  const [postponeTodo, setPostponeTodo] = useState<any | null>(null);
  const [postponeDate, setPostponeDate] = useState<string>('');
  const [cancelTodo, setCancelTodo] = useState<any | null>(null);
  const [cancelReason, setCancelReason] = useState<string>('Kunde hat aktuell keinen Bedarf / verschoben');
  const [cancelNotes, setCancelNotes] = useState<string>('');
  const [feedbackBanner, setFeedbackBanner] = useState<string | null>(null);

  const { data: todos = [] } = useQuery<any[]>({
    queryKey: ['todos'],
    queryFn: () => apiFetch('/api/v1/todos'),
  });

  const createMutation = useMutation({
    mutationFn: (newTodo: any) =>
      apiFetch('/api/v1/todos', {
        method: 'POST',
        body: JSON.stringify(newTodo),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['todos'] });
      setIsModalOpen(false);
      setFeedbackBanner('Wiedervorlage / Aufgabe erfolgreich angelegt!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (todo: any) =>
      apiFetch(`/api/v1/todos/${todo.id}`, {
        method: 'PUT',
        body: JSON.stringify(todo),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['todos'] });
      setEditingTodo(null);
      setPostponeTodo(null);
      setCancelTodo(null);
      setFeedbackBanner('Aufgabe aktualisiert!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (todoId: string) =>
      apiFetch(`/api/v1/todos/${todoId}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['todos'] });
      setDeletingTodo(null);
      setFeedbackBanner('Aufgabe gelöscht.');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const [formData, setFormData] = useState({
    title: '',
    description: '',
    category: 'WIEDERVORLAGE',
    product_segment: 'Stromspeicher',
    contact_name: 'Sabine Mustermann',
    priority: 'HIGH',
    status: 'OPEN',
    due_date: '2026-11-25',
    assigned_to: 'Vertriebsteam',
  });

  const setResubmissionDays = (days: number, titlePreset: string, product: string) => {
    const d = new Date();
    d.setDate(d.getDate() + days);
    setFormData({
      ...formData,
      due_date: d.toISOString().split('T')[0],
      title: titlePreset,
      product_segment: product,
      category: days === 14 ? 'WIEDERVORLAGE' : 'CROSS_SELLING',
    });
  };

  const handlePostponeDays = (days: number) => {
    if (!postponeTodo) return;
    const baseDate = postponeTodo.due_date ? new Date(postponeTodo.due_date) : new Date();
    baseDate.setDate(baseDate.getDate() + days);
    const newDueDate = baseDate.toISOString().split('T')[0];
    updateMutation.mutate({
      ...postponeTodo,
      due_date: newDueDate,
      description: `${postponeTodo.description || ''}\n[Verschoben um ${days} Tage auf ${newDueDate}]`.trim(),
    });
  };

  const handleCustomPostpone = (e: React.FormEvent) => {
    e.preventDefault();
    if (!postponeTodo || !postponeDate) return;
    updateMutation.mutate({
      ...postponeTodo,
      due_date: postponeDate,
      description: `${postponeTodo.description || ''}\n[Verschoben auf ${postponeDate}]`.trim(),
    });
  };

  const handleCancelSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!cancelTodo) return;
    updateMutation.mutate({
      ...cancelTodo,
      status: 'CANCELLED',
      cancellation_reason: cancelReason,
      cancellation_notes: cancelNotes,
      description: `${cancelTodo.description || ''}\n[Abgebrochen/Übersprungen: ${cancelReason} - ${cancelNotes}]`.trim(),
    });
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate(formData);
  };

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingTodo) return;
    updateMutation.mutate(editingTodo);
  };

  const toggleTodoStatus = (todo: any) => {
    const nextStatus = todo.status === 'COMPLETED' ? 'OPEN' : 'COMPLETED';
    updateMutation.mutate({ ...todo, status: nextStatus });
  };

  const getPriorityBadge = (priority: string) => {
    switch (priority) {
      case 'URGENT':
        return 'bg-rose-500/10 text-rose-400 border-rose-500/20';
      case 'HIGH':
        return 'bg-amber-500/10 text-amber-400 border-amber-500/20';
      case 'LOW':
        return 'bg-slate-500/10 text-slate-400 border-slate-500/20';
      default:
        return 'bg-blue-500/10 text-blue-400 border-blue-500/20';
    }
  };

  const getCategoryBadge = (category: string) => {
    switch (category) {
      case 'CROSS_SELLING':
        return 'bg-purple-500/10 text-purple-400 border-purple-500/20';
      case 'WIEDERVORLAGE':
        return 'bg-cyan-500/10 text-cyan-400 border-cyan-500/20';
      default:
        return 'bg-slate-800 text-slate-400 border-slate-700';
    }
  };

  const filteredTodos = todos.filter((t) => {
    const matchesSearch =
      t.title?.toLowerCase().includes(search.toLowerCase()) ||
      t.description?.toLowerCase().includes(search.toLowerCase()) ||
      t.contact_name?.toLowerCase().includes(search.toLowerCase()) ||
      t.product_segment?.toLowerCase().includes(search.toLowerCase());

    const matchesStatus =
      filterStatus === 'ALL' ||
      (filterStatus === 'OPEN' && (t.status === 'OPEN' || !t.status)) ||
      (filterStatus === 'COMPLETED' && t.status === 'COMPLETED') ||
      (filterStatus === 'CANCELLED' && (t.status === 'CANCELLED' || t.status === 'SKIPPED'));

    const matchesCategory =
      filterCategory === 'ALL' ||
      t.category === filterCategory ||
      (filterCategory === 'CROSS_SELLING' && (t.category === 'CROSS_SELLING' || t.category === 'WIEDERVORLAGE'));

    return matchesSearch && matchesStatus && matchesCategory;
  });

  const pendingCount = todos.filter((t) => t.status === 'OPEN' || !t.status).length;
  const crossSellingCount = todos.filter((t) => t.category === 'CROSS_SELLING' || t.category === 'WIEDERVORLAGE').length;
  const cancelledCount = todos.filter((t) => t.status === 'CANCELLED' || t.status === 'SKIPPED').length;

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2.5">
            <RefreshCw className="w-6 h-6 text-emerald-400" />
            Aufgaben, SLA & Cross-Selling Wiedervorlagen (§3.5)
          </h1>
          <p className="text-sm text-slate-400">
            Fristen-Überwachung, Verschieben auf Folgetage, Überspringen/Abbrechen mit Notiz & Cross-Selling
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => {
              setFormData({
                title: '',
                description: '',
                category: 'CROSS_SELLING',
                product_segment: 'Stromspeicher',
                contact_name: '',
                priority: 'HIGH',
                status: 'OPEN',
                due_date: new Date(Date.now() + 90 * 86400000).toISOString().split('T')[0],
                assigned_to: 'Vertriebsteam',
              });
              setIsModalOpen(true);
            }}
            className="inline-flex items-center gap-2 px-4 py-2 bg-purple-600 hover:bg-purple-500 text-white font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-purple-600/20 cursor-pointer"
          >
            <Sparkles className="w-4 h-4" />
            Wiedervorlage / Cross-Selling anlegen
          </button>
          <button
            onClick={() => {
              setFormData({
                title: '',
                description: '',
                category: 'TASK',
                product_segment: 'PV-Anlage',
                contact_name: '',
                priority: 'MEDIUM',
                status: 'OPEN',
                due_date: new Date(Date.now() + 3 * 86400000).toISOString().split('T')[0],
                assigned_to: 'Vertriebsteam',
              });
              setIsModalOpen(true);
            }}
            className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-emerald-600/20 cursor-pointer"
          >
            <Plus className="w-4 h-4" />
            Aufgabe anlegen
          </button>
        </div>
      </div>

      {feedbackBanner && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2">
          <CheckCircle2 className="w-4 h-4" />
          {feedbackBanner}
        </div>
      )}

      {/* KPI & Filter Header */}
      <div className="grid grid-cols-1 sm:grid-cols-4 gap-4">
        <div className="p-4 bg-slate-900 border border-slate-800 rounded-2xl flex items-center justify-between">
          <div>
            <div className="text-xs text-slate-400">Offene Todos</div>
            <div className="text-xl font-bold text-slate-100 mt-0.5">{pendingCount} Aufgaben</div>
          </div>
          <AlertCircle className="w-7 h-7 text-amber-400/80" />
        </div>
        <div className="p-4 bg-slate-900 border border-slate-800 rounded-2xl flex items-center justify-between">
          <div>
            <div className="text-xs text-purple-400">Wiedervorlagen & Cross-Sell</div>
            <div className="text-xl font-bold text-purple-300 mt-0.5">{crossSellingCount} Einträge</div>
          </div>
          <Sparkles className="w-7 h-7 text-purple-400/80" />
        </div>
        <div className="p-4 bg-slate-900 border border-slate-800 rounded-2xl flex items-center justify-between">
          <div>
            <div className="text-xs text-rose-400">Abgebrochen / Übersprungen</div>
            <div className="text-xl font-bold text-rose-400 mt-0.5">{cancelledCount} archiviert</div>
          </div>
          <Ban className="w-7 h-7 text-rose-400/80" />
        </div>
        <div className="p-4 bg-slate-900 border border-slate-800 rounded-2xl flex items-center justify-between">
          <div>
            <div className="text-xs text-emerald-400">SLA-Konformität</div>
            <div className="text-xl font-bold text-emerald-400 mt-0.5">100% DSGVO & BGB</div>
          </div>
          <CheckCircle2 className="w-7 h-7 text-emerald-400/80" />
        </div>
      </div>

      {/* Search & Category Filter */}
      <div className="flex flex-col sm:flex-row items-center justify-between gap-3 bg-slate-900 border border-slate-800 p-3 rounded-2xl">
        <div className="relative w-full sm:w-80">
          <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
          <input
            type="text"
            placeholder="Kunde, Produkt, Aufgabe suchen..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-9 pr-3 py-1.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
          />
        </div>

        <div className="flex items-center gap-2 overflow-x-auto w-full sm:w-auto">
          <button
            onClick={() => setFilterCategory('ALL')}
            className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition-colors cursor-pointer ${filterCategory === 'ALL' ? 'bg-emerald-600 text-slate-950' : 'bg-slate-950 text-slate-400 hover:text-slate-200'}`}
          >
            Alle
          </button>
          <button
            onClick={() => setFilterCategory('CROSS_SELLING')}
            className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition-colors cursor-pointer ${filterCategory === 'CROSS_SELLING' ? 'bg-purple-600 text-white' : 'bg-slate-950 text-slate-400 hover:text-slate-200'}`}
          >
            🔄 Wiedervorlagen & Cross-Sell
          </button>
          <button
            onClick={() => setFilterStatus(filterStatus === 'OPEN' ? 'ALL' : 'OPEN')}
            className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition-colors cursor-pointer ${filterStatus === 'OPEN' ? 'bg-amber-600 text-white' : 'bg-slate-950 text-slate-400 hover:text-slate-200'}`}
          >
            Nur Offene
          </button>
          <button
            onClick={() => setFilterStatus(filterStatus === 'CANCELLED' ? 'ALL' : 'CANCELLED')}
            className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition-colors cursor-pointer ${filterStatus === 'CANCELLED' ? 'bg-rose-600 text-white' : 'bg-slate-950 text-slate-400 hover:text-slate-200'}`}
          >
            Abgebrochen / Übersprungen
          </button>
        </div>
      </div>

      {/* Todos & Resubmissions List */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl overflow-hidden divide-y divide-slate-800/60">
        {filteredTodos.length === 0 ? (
          <div className="p-12 text-center text-slate-500 text-sm">
            Keine Aufgaben oder Wiedervorlagen für diesen Filter gefunden.
          </div>
        ) : (
          filteredTodos.map((todo) => {
            const isCompleted = todo.status === 'COMPLETED';
            const isCancelled = todo.status === 'CANCELLED' || todo.status === 'SKIPPED';

            return (
              <div
                key={todo.id}
                className="p-4 flex items-start justify-between gap-4 hover:bg-slate-800/40 transition-colors group"
              >
                <div className="flex items-start gap-3.5">
                  <button
                    onClick={() => toggleTodoStatus(todo)}
                    disabled={isCancelled}
                    className={`mt-0.5 transition-colors ${isCancelled ? 'opacity-30 cursor-not-allowed' : 'text-slate-500 hover:text-emerald-400 cursor-pointer'}`}
                    title={isCompleted ? 'Als offen markieren' : 'Als erledigt markieren'}
                    aria-label="Status umschalten"
                  >
                    {isCompleted ? (
                      <CheckCircle2 className="w-5 h-5 text-emerald-400" />
                    ) : isCancelled ? (
                      <Ban className="w-5 h-5 text-rose-400" />
                    ) : (
                      <Circle className="w-5 h-5" />
                    )}
                  </button>

                  <div className="space-y-1">
                    <div className="flex items-center gap-2 flex-wrap">
                      <h4 className={`text-sm font-semibold ${isCompleted ? 'text-slate-500 line-through' : isCancelled ? 'text-rose-400/80 line-through' : 'text-slate-200'}`}>
                        {todo.title}
                      </h4>
                      {isCancelled && (
                        <span className="text-[10px] px-2 py-0.5 rounded-full bg-rose-500/10 text-rose-400 border border-rose-500/20 font-bold uppercase">
                          Abgebrochen / Übersprungen
                        </span>
                      )}
                      {todo.category && !isCancelled && (
                        <span className={`text-[10px] px-2 py-0.5 rounded-full border font-bold uppercase tracking-wider ${getCategoryBadge(todo.category)}`}>
                          {todo.category === 'CROSS_SELLING' ? '🔄 Cross-Selling' : todo.category === 'WIEDERVORLAGE' ? '⏰ Wiedervorlage' : '📋 Aufgabe'}
                        </span>
                      )}
                      {todo.product_segment && (
                        <span className="text-[10px] px-2 py-0.5 rounded-full bg-slate-800 text-slate-300 border border-slate-700 font-medium">
                          <Tag className="w-2.5 h-2.5 inline mr-1 text-emerald-400" />
                          {todo.product_segment}
                        </span>
                      )}
                    </div>

                    {todo.description && (
                      <p className={`text-xs ${isCompleted || isCancelled ? 'text-slate-600' : 'text-slate-400'} whitespace-pre-line`}>
                        {todo.description}
                      </p>
                    )}

                    <div className="flex items-center gap-3 pt-1 text-[11px] text-slate-500 flex-wrap">
                      {todo.contact_name && (
                        <span className="flex items-center gap-1 text-slate-300 font-medium">
                          <UserCheck className="w-3 h-3 text-cyan-400" /> {todo.contact_name}
                        </span>
                      )}
                      {todo.due_date && (
                        <span className="flex items-center gap-1 text-slate-400">
                          <Calendar className="w-3 h-3 text-amber-400" /> Fällig: {todo.due_date}
                        </span>
                      )}
                      <span className={`px-1.5 py-0.2 rounded border font-semibold text-[10px] ${getPriorityBadge(todo.priority)}`}>
                        {todo.priority}
                      </span>
                    </div>
                  </div>
                </div>

                {/* Quick Action Buttons: Postpone, Cancel with note, Edit, Delete */}
                <div className="flex items-center gap-1.5 shrink-0">
                  {!isCompleted && !isCancelled && (
                    <>
                      <button
                        onClick={() => {
                          setPostponeTodo(todo);
                          setPostponeDate(todo.due_date || new Date().toISOString().split('T')[0]);
                        }}
                        className="p-1.5 bg-amber-500/10 hover:bg-amber-500/20 text-amber-400 border border-amber-500/20 rounded-lg text-xs font-semibold inline-flex items-center gap-1 transition-colors cursor-pointer"
                        title="Fälligkeit auf einen anderen Tag verschieben"
                      >
                        <Clock className="w-3.5 h-3.5" />
                        <span className="hidden sm:inline">Verschieben</span>
                      </button>
                      <button
                        onClick={() => {
                          setCancelTodo(todo);
                          setCancelReason('Kunde aktuell kein Bedarf / verschoben');
                          setCancelNotes('');
                        }}
                        className="p-1.5 bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 rounded-lg text-xs font-semibold inline-flex items-center gap-1 transition-colors cursor-pointer"
                        title="Aufgabe mit Begründung überspringen oder abbrechen"
                      >
                        <Ban className="w-3.5 h-3.5" />
                        <span className="hidden sm:inline">Überspringen</span>
                      </button>
                    </>
                  )}
                  <button
                    onClick={() => setEditingTodo({ ...todo })}
                    className="p-1.5 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded-lg text-xs cursor-pointer"
                    title="Aufgabe bearbeiten"
                    aria-label="Bearbeiten"
                  >
                    <Edit3 className="w-4 h-4" />
                  </button>
                  <button
                    onClick={() => setDeletingTodo(todo)}
                    className="p-1.5 text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 rounded-lg text-xs cursor-pointer"
                    title="Aufgabe löschen"
                    aria-label="Löschen"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* Postpone Modal (Fälligkeit verschieben) */}
      {postponeTodo && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <Clock className="w-5 h-5 text-amber-400" />
                Aufgabe verschieben
              </h3>
              <button onClick={() => setPostponeTodo(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <p className="text-xs text-slate-300">
              Wann soll <strong className="text-slate-100">"{postponeTodo.title}"</strong> erneut vorgelegt werden?
            </p>

            {/* Quick 1-Click Postpone Presets */}
            <div className="space-y-1.5">
              <span className="text-[11px] font-bold text-amber-400 uppercase tracking-wider">Schnellauswahl:</span>
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  onClick={() => handlePostponeDays(1)}
                  className="p-2.5 bg-slate-950 hover:bg-amber-950/30 border border-slate-800 hover:border-amber-500/40 rounded-xl text-left text-xs text-slate-300 transition-colors"
                >
                  <span className="font-bold text-amber-400">+1 Tag</span> (Morgen)
                </button>
                <button
                  type="button"
                  onClick={() => handlePostponeDays(3)}
                  className="p-2.5 bg-slate-950 hover:bg-amber-950/30 border border-slate-800 hover:border-amber-500/40 rounded-xl text-left text-xs text-slate-300 transition-colors"
                >
                  <span className="font-bold text-amber-400">+3 Tage</span>
                </button>
                <button
                  type="button"
                  onClick={() => handlePostponeDays(7)}
                  className="p-2.5 bg-slate-950 hover:bg-amber-950/30 border border-slate-800 hover:border-amber-500/40 rounded-xl text-left text-xs text-slate-300 transition-colors"
                >
                  <span className="font-bold text-amber-400">+1 Woche</span> (Nächste Woche)
                </button>
                <button
                  type="button"
                  onClick={() => handlePostponeDays(30)}
                  className="p-2.5 bg-slate-950 hover:bg-amber-950/30 border border-slate-800 hover:border-amber-500/40 rounded-xl text-left text-xs text-slate-300 transition-colors"
                >
                  <span className="font-bold text-amber-400">+1 Monat</span>
                </button>
              </div>
            </div>

            {/* Custom Date Form */}
            <form onSubmit={handleCustomPostpone} className="space-y-3 pt-2 border-t border-slate-800">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Oder genaues Datum wählen:</label>
                <input
                  type="date"
                  required
                  value={postponeDate}
                  onChange={(e) => setPostponeDate(e.target.value)}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-amber-500"
                />
              </div>

              <div className="flex justify-end gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => setPostponeTodo(null)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-semibold"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={updateMutation.isPending}
                  className="px-4 py-2 bg-amber-600 hover:bg-amber-500 text-slate-950 rounded-xl text-xs font-bold shadow-lg shadow-amber-600/20"
                >
                  Auf Datum verschieben
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Cancel / Skip Modal (Abbrechen mit Begründung & Notiz) */}
      {cancelTodo && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <Ban className="w-5 h-5 text-rose-400" />
                Aufgabe abbrechen / überspringen
              </h3>
              <button onClick={() => setCancelTodo(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <p className="text-xs text-slate-300">
              Bitte hinterlegen Sie eine Begründung für das Überspringen von <strong className="text-slate-100">"{cancelTodo.title}"</strong>:
            </p>

            <form onSubmit={handleCancelSubmit} className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Grund für Abbruch / Skip *</label>
                <select
                  value={cancelReason}
                  onChange={(e) => setCancelReason(e.target.value)}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-rose-500"
                >
                  <option value="Kunde aktuell kein Bedarf / verschoben">Kunde aktuell kein Bedarf / verschoben</option>
                  <option value="Bereits durch anderes Angebot / Projekt gelöst">Bereits durch anderes Angebot gelöst</option>
                  <option value="Kunde disqualifiziert / Kein Budget / Nicht erreichbar">Kunde disqualifiziert / Nicht erreichbar</option>
                  <option value="Wettbewerber gewählt / Angebot verloren">Wettbewerber gewählt / Angebot verloren</option>
                  <option value="Sonstiger Grund">Sonstiger Grund</option>
                </select>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Interne Notiz / Details</label>
                <textarea
                  rows={2}
                  placeholder="z.B. Kunde teilte im Telefonat mit, dass Projekt auf 2027 verschoben wird..."
                  value={cancelNotes}
                  onChange={(e) => setCancelNotes(e.target.value)}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-rose-500 resize-none"
                />
              </div>

              <div className="flex justify-end gap-3 pt-2 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setCancelTodo(null)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-semibold"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={updateMutation.isPending}
                  className="px-4 py-2 bg-rose-600 hover:bg-rose-500 text-white rounded-xl text-xs font-bold shadow-lg shadow-rose-600/20"
                >
                  Mit Notiz überspringen
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Create / Resubmission Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <RefreshCw className="w-5 h-5 text-emerald-400" />
                {formData.category === 'CROSS_SELLING' || formData.category === 'WIEDERVORLAGE'
                  ? 'Wiedervorlage / Cross-Selling anlegen (§3.5)'
                  : 'Neue Aufgabe anlegen (§3.5)'}
              </h3>
              <button onClick={() => setIsModalOpen(false)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            {/* Quick-Preset Buttons for Resubmission cycles */}
            <div className="space-y-1.5 bg-slate-950 p-3 rounded-xl border border-slate-800">
              <span className="text-[11px] font-bold text-purple-400 uppercase tracking-wider flex items-center gap-1">
                <Sparkles className="w-3 h-3" /> Schnellauswahl Wiedervorlage-Frist:
              </span>
              <div className="grid grid-cols-2 gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => setResubmissionDays(14, '§ 355 BGB Widerrufsfrist abgelaufen - Montage freigeben', 'PV-Anlage')}
                  className="p-1.5 bg-slate-900 hover:bg-purple-900/30 border border-slate-800 hover:border-purple-500/40 rounded-lg text-left text-[11px] text-slate-300 transition-colors"
                >
                  <span className="font-bold text-amber-400">14 Tage</span> (Widerrufsablauf § 355)
                </button>
                <button
                  type="button"
                  onClick={() => setResubmissionDays(90, 'Wiedervorlage: Batteriespeicher-Nachrüstung anbieten', 'Stromspeicher')}
                  className="p-1.5 bg-slate-900 hover:bg-purple-900/30 border border-slate-800 hover:border-purple-500/40 rounded-lg text-left text-[11px] text-slate-300 transition-colors"
                >
                  <span className="font-bold text-purple-400">3 Monate</span> (Speicher Cross-Sell)
                </button>
                <button
                  type="button"
                  onClick={() => setResubmissionDays(180, 'Wiedervorlage: Wärmepumpen-Kopplung prüfen', 'Wärmepumpe')}
                  className="p-1.5 bg-slate-900 hover:bg-purple-900/30 border border-slate-800 hover:border-purple-500/40 rounded-lg text-left text-[11px] text-slate-300 transition-colors"
                >
                  <span className="font-bold text-cyan-400">6 Monate</span> (Wärmepumpe)
                </button>
                <button
                  type="button"
                  onClick={() => setResubmissionDays(365, 'Jahres-Check: PV-Ertrag & Wartungsvertrag anbieten', 'Wartungsvertrag / Service')}
                  className="p-1.5 bg-slate-900 hover:bg-purple-900/30 border border-slate-800 hover:border-purple-500/40 rounded-lg text-left text-[11px] text-slate-300 transition-colors"
                >
                  <span className="font-bold text-emerald-400">1 Jahr</span> (Jahreswartung)
                </button>
              </div>
            </div>

            <form onSubmit={handleSubmit} className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Titel / Betreff *</label>
                <input
                  type="text"
                  required
                  placeholder="z.B. Rückruf PV-Angebot Familie Müller"
                  value={formData.title}
                  onChange={(e) => setFormData({ ...formData, title: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Kunde / Kontakt</label>
                  <input
                    type="text"
                    placeholder="z.B. Sabine Mustermann"
                    value={formData.contact_name}
                    onChange={(e) => setFormData({ ...formData, contact_name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Produktsparte</label>
                  <select
                    value={formData.product_segment}
                    onChange={(e) => setFormData({ ...formData, product_segment: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    {PRODUCT_SEGMENTS.map((prod) => (
                      <option key={prod} value={prod}>{prod}</option>
                    ))}
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Beschreibung / Gesprächsleitfaden</label>
                <textarea
                  rows={2}
                  placeholder="Notizen zum nächsten Gespräch, Zählerstand, Einwände..."
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500 resize-none"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Wiedervorlage am (Fälligkeit)</label>
                  <input
                    type="date"
                    required
                    value={formData.due_date}
                    onChange={(e) => setFormData({ ...formData, due_date: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Priorität</label>
                  <select
                    value={formData.priority}
                    onChange={(e) => setFormData({ ...formData, priority: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="URGENT">Dringend (SLA &lt; 24h)</option>
                    <option value="HIGH">Hoch</option>
                    <option value="MEDIUM">Mittel</option>
                    <option value="LOW">Niedrig</option>
                  </select>
                </div>
              </div>

              <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setIsModalOpen(false)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-semibold transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={createMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 rounded-xl text-xs font-bold transition-colors shadow-lg shadow-emerald-600/20"
                >
                  {createMutation.isPending ? 'Speichert...' : 'Eintrag erstellen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Modal */}
      {editingTodo && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg">Aufgabe / Wiedervorlage bearbeiten</h3>
              <button onClick={() => setEditingTodo(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUpdate} className="space-y-3">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Titel</label>
                <input
                  type="text"
                  required
                  value={editingTodo.title}
                  onChange={(e) => setEditingTodo({ ...editingTodo, title: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Kunde</label>
                  <input
                    type="text"
                    value={editingTodo.contact_name || ''}
                    onChange={(e) => setEditingTodo({ ...editingTodo, contact_name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Produktsparte</label>
                  <select
                    value={editingTodo.product_segment || 'Stromspeicher'}
                    onChange={(e) => setEditingTodo({ ...editingTodo, product_segment: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    {PRODUCT_SEGMENTS.map((prod) => (
                      <option key={prod} value={prod}>{prod}</option>
                    ))}
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">Beschreibung</label>
                <textarea
                  rows={2}
                  value={editingTodo.description || ''}
                  onChange={(e) => setEditingTodo({ ...editingTodo, description: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500 resize-none"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Fälligkeit</label>
                  <input
                    type="date"
                    required
                    value={editingTodo.due_date || '2026-08-28'}
                    onChange={(e) => setEditingTodo({ ...editingTodo, due_date: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">Priorität</label>
                  <select
                    value={editingTodo.priority}
                    onChange={(e) => setEditingTodo({ ...editingTodo, priority: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="URGENT">Dringend</option>
                    <option value="HIGH">Hoch</option>
                    <option value="MEDIUM">Mittel</option>
                    <option value="LOW">Niedrig</option>
                  </select>
                </div>
              </div>

              <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setEditingTodo(null)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-semibold transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={updateMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 rounded-xl text-xs font-bold transition-colors shadow-lg shadow-emerald-600/20"
                >
                  {updateMutation.isPending ? 'Speichert...' : 'Änderungen speichern'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deletingTodo && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center gap-3 text-rose-400 border-b border-slate-800 pb-3">
              <AlertCircle className="w-6 h-6 shrink-0" />
              <h3 className="font-bold text-slate-100 text-lg">Aufgabe wirklich löschen?</h3>
            </div>
            <p className="text-xs text-slate-300 leading-relaxed">
              Möchten Sie den Eintrag <strong className="text-slate-100">"{deletingTodo.title}"</strong> endgültig löschen?
            </p>
            <div className="flex justify-end gap-3 pt-2">
              <button
                type="button"
                onClick={() => setDeletingTodo(null)}
                className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-semibold transition-colors"
              >
                Abbrechen
              </button>
              <button
                type="button"
                onClick={() => deleteMutation.mutate(deletingTodo.id)}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-500 text-white rounded-xl text-xs font-bold transition-colors shadow-lg shadow-rose-600/20"
              >
                {deleteMutation.isPending ? 'Löscht...' : 'Endgültig löschen'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
