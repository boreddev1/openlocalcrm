import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import {
  Plus,
  X,
  Tag,
  Edit3,
  Trash2,
  ShieldCheck,
  DollarSign,
  Search,
  AlertCircle,
  CheckCircle2,
  RefreshCw,
  Sparkles,
} from 'lucide-react';
import { DndContext, DragEndEvent } from '@dnd-kit/core';
import { OfferCalculatorModal } from '../components/calculator/OfferCalculatorModal';

interface Deal {
  id: string;
  title: string;
  value: string;
  currency: string;
  stage: string;
  probability: number;
  contact_name?: string;
  notes?: string;
  signed_at?: string;
}

const STAGES = [
  {
    id: 'LEAD',
    label: 'Lead / Erstkontakt',
    color: 'border-blue-500/30 bg-blue-500/5 text-blue-400',
  },
  {
    id: 'QUALIFIED',
    label: 'Qualifiziert',
    color: 'border-purple-500/30 bg-purple-500/5 text-purple-400',
  },
  {
    id: 'OFFER_SENT',
    label: 'Angebot vorliegend',
    color: 'border-amber-500/30 bg-amber-500/5 text-amber-400',
  },
  {
    id: 'NEGOTIATION',
    label: 'Verhandlung',
    color: 'border-cyan-500/30 bg-cyan-500/5 text-cyan-400',
  },
  {
    id: 'WON',
    label: 'Gewonnen / Abschluss',
    color: 'border-emerald-500/30 bg-emerald-500/5 text-emerald-400',
  },
  { id: 'LOST', label: 'Verloren', color: 'border-rose-500/30 bg-rose-500/5 text-rose-400' },
];

export const DealsPage: React.FC = () => {
  const queryClient = useQueryClient();
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isCalculatorOpen, setIsCalculatorOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [editingDeal, setEditingDeal] = useState<Deal | null>(null);
  const [deletingDeal, setDeletingDeal] = useState<Deal | null>(null);
  const [resubmissionDeal, setResubmissionDeal] = useState<Deal | null>(null);
  const [resubmissionData, setResubmissionData] = useState({
    title: '',
    product_segment: 'Stromspeicher',
    due_date: '2026-11-25',
    notes: '',
  });
  const [feedbackBanner, setFeedbackBanner] = useState<string | null>(null);

  const { data: deals = [] } = useQuery<Deal[]>({
    queryKey: ['deals'],
    queryFn: () => apiFetch('/api/v1/deals'),
  });

  const createMutation = useMutation({
    mutationFn: (newDeal: any) =>
      apiFetch('/api/v1/deals', {
        method: 'POST',
        body: JSON.stringify(newDeal),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deals'] });
      setIsModalOpen(false);
      setFeedbackBanner('Deal erfolgreich erstellt!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (deal: any) =>
      apiFetch(`/api/v1/deals/${deal.id}`, {
        method: 'PUT',
        body: JSON.stringify(deal),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deals'] });
      setEditingDeal(null);
      setFeedbackBanner('Deal aktualisiert!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (dealId: string) =>
      apiFetch(`/api/v1/deals/${dealId}`, {
        method: 'DELETE',
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['deals'] });
      setDeletingDeal(null);
      setFeedbackBanner('Deal gelöscht.');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const [formData, setFormData] = useState({
    title: '',
    value: '18500.00',
    currency: 'EUR',
    stage: 'LEAD',
    probability: 20,
    contact_name: 'Max Mustermann',
    notes: 'Photovoltaikanlage 12 kWp mit 10 kWh Speicher.',
  });

  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event;
    if (!over) return;

    const dealId = active.id as string;
    const newStage = over.id as string;
    const deal = deals.find((d) => d.id === dealId);
    if (deal && deal.stage !== newStage) {
      updateMutation.mutate({ ...deal, stage: newStage });
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate({
      ...formData,
      probability: Number(formData.probability),
    });
  };

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingDeal) return;
    updateMutation.mutate(editingDeal);
  };

  const filteredDeals = deals.filter((d) => {
    const term = search.toLowerCase();
    return d.title?.toLowerCase().includes(term) || d.contact_name?.toLowerCase().includes(term);
  });

  const totalPipeline = deals.reduce((sum, d) => sum + (parseFloat(d.value) || 0), 0);
  const weightedPipeline = deals.reduce(
    (sum, d) => sum + (parseFloat(d.value) || 0) * ((d.probability || 0) / 100),
    0,
  );
  const wonPipeline = deals
    .filter((d) => d.stage === 'WON')
    .reduce((sum, d) => sum + (parseFloat(d.value) || 0), 0);
  const activeDealsCount = deals.filter((d) => d.stage !== 'LOST' && d.stage !== 'WON').length;

  return (
    <div className="space-y-6 max-w-7xl mx-auto flex flex-col h-[calc(100vh-6rem)]">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
        <div>
          <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2.5">
            <DollarSign className="w-6 h-6 text-emerald-400" />
            Deal-Pipeline & Kanban (§3.3)
          </h1>
          <p className="text-sm text-slate-400">
            Verkaufsphasen, Wahrscheinlichkeiten & § 355 BGB 14-Tage Widerrufs-Tracking
          </p>
        </div>
        <div className="flex items-center gap-3">
          <div className="relative">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
            <input
              type="text"
              placeholder="Deal filtern..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9 pr-3 py-1.5 bg-slate-900 border border-slate-800 rounded-lg text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
            />
          </div>
          <button
            onClick={() => setIsCalculatorOpen(true)}
            className="inline-flex items-center gap-2 px-3.5 py-2 bg-purple-600 hover:bg-purple-500 text-white font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-purple-600/20 cursor-pointer"
            title="KI-Dokumentenextraktion & Deterministischer Angebotsrechner (§5.2 / §5.6)"
          >
            <Sparkles className="w-4 h-4" />
            <span>⚡ KI-Angebotsrechner & OCR</span>
          </button>
          <button
            onClick={() => setIsModalOpen(true)}
            className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-emerald-600/20"
          >
            <Plus className="w-4 h-4" />
            Deal anlegen
          </button>
        </div>
      </div>

      {/* Pipeline Summary Metrics Cards (§3.3) */}
      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 shrink-0">
        <div className="p-3 bg-slate-900 border border-slate-800 rounded-xl">
          <div className="text-[11px] font-semibold text-slate-400">Gesamt-Pipeline</div>
          <div className="text-lg font-bold text-slate-100 mt-0.5">
            {totalPipeline.toLocaleString('de-DE')} €
          </div>
        </div>
        <div className="p-3 bg-slate-900 border border-slate-800 rounded-xl">
          <div className="text-[11px] font-semibold text-purple-400">Gewichteter Forecast</div>
          <div className="text-lg font-bold text-purple-300 mt-0.5">
            {Math.round(weightedPipeline).toLocaleString('de-DE')} €
          </div>
        </div>
        <div className="p-3 bg-slate-900 border border-slate-800 rounded-xl">
          <div className="text-[11px] font-semibold text-emerald-400">Gewonnene Abschlüsse</div>
          <div className="text-lg font-bold text-emerald-400 mt-0.5">
            {wonPipeline.toLocaleString('de-DE')} €
          </div>
        </div>
        <div className="p-3 bg-slate-900 border border-slate-800 rounded-xl">
          <div className="text-[11px] font-semibold text-blue-400">Aktive Deals im Trichter</div>
          <div className="text-lg font-bold text-blue-300 mt-0.5">{activeDealsCount} Deals</div>
        </div>
      </div>

      {feedbackBanner && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2 shrink-0">
          <CheckCircle2 className="w-4 h-4" />
          {feedbackBanner}
        </div>
      )}

      {/* Kanban Board Container */}
      <DndContext onDragEnd={handleDragEnd}>
        <div className="flex-1 overflow-x-auto pb-4">
          <div className="grid grid-flow-col auto-cols-[310px] gap-4 h-full min-w-max">
            {STAGES.map((stage) => {
              const stageDeals = filteredDeals.filter((d) => d.stage === stage.id);
              const totalVolume = stageDeals.reduce(
                (sum, d) => sum + (parseFloat(d.value) || 0),
                0,
              );

              return (
                <div
                  key={stage.id}
                  id={stage.id}
                  className="bg-slate-900/70 border border-slate-800 rounded-2xl p-4 flex flex-col h-full"
                >
                  {/* Column Header */}
                  <div className="flex items-center justify-between pb-3 border-b border-slate-800 shrink-0">
                    <div className="flex items-center gap-2">
                      <span
                        className={`text-xs font-semibold px-2.5 py-1 rounded-full border ${stage.color}`}
                      >
                        {stage.label}
                      </span>
                    </div>
                    <div className="text-right">
                      <span className="text-xs font-bold text-slate-400 bg-slate-800 px-2 py-0.5 rounded-full">
                        {stageDeals.length}
                      </span>
                      {totalVolume > 0 && (
                        <div className="text-[10px] text-slate-500 mt-0.5">
                          {totalVolume.toLocaleString('de-DE')} €
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Deals Cards Container */}
                  <div className="flex-1 overflow-y-auto pt-3 space-y-3">
                    {stageDeals.length === 0 ? (
                      <div className="h-32 border border-dashed border-slate-800/80 rounded-xl flex items-center justify-center text-xs text-slate-600">
                        Keine Deals
                      </div>
                    ) : (
                      stageDeals.map((deal) => (
                        <div
                          key={deal.id}
                          id={deal.id?.startsWith('d-') ? deal.id : `d-${deal.id}`}
                          className="bg-slate-950 border border-slate-800 hover:border-slate-700 p-4 rounded-xl shadow-sm transition-all space-y-3 group"
                        >
                          <div className="flex items-start justify-between gap-2">
                            <div className="font-semibold text-sm text-slate-200">{deal.title}</div>
                            <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                              <button
                                onClick={() => setEditingDeal({ ...deal })}
                                className="p-1 text-slate-400 hover:text-slate-200 hover:bg-slate-800 rounded"
                                title="Deal bearbeiten"
                                aria-label="Bearbeiten"
                              >
                                <Edit3 className="w-3.5 h-3.5" />
                              </button>
                              <button
                                onClick={() => setDeletingDeal(deal)}
                                className="p-1 text-rose-400 hover:text-rose-300 hover:bg-rose-500/10 rounded"
                                title="Deal löschen"
                                aria-label="Löschen"
                              >
                                <Trash2 className="w-3.5 h-3.5" />
                              </button>
                            </div>
                          </div>

                          <div className="flex items-center justify-between text-xs text-slate-400">
                            <span className="font-bold text-emerald-400 text-sm">
                              {deal.value
                                ? `${parseFloat(deal.value).toLocaleString('de-DE')} ${deal.currency}`
                                : '0,00 EUR'}
                            </span>
                            <span className="flex items-center gap-1 text-[11px] text-slate-400 bg-slate-900 px-2 py-0.5 rounded border border-slate-800">
                              <Tag className="w-3 h-3 text-purple-400" /> {deal.probability}%
                            </span>
                          </div>

                          {/* Stage Transition Selector & Resubmission Trigger */}
                          <div className="pt-2 border-t border-slate-900 flex items-center justify-between gap-2">
                            <select
                              value={deal.stage}
                              onChange={(e) =>
                                updateMutation.mutate({ ...deal, stage: e.target.value })
                              }
                              className="text-[11px] bg-slate-900 border border-slate-800 text-slate-300 rounded px-2 py-1 focus:outline-none"
                            >
                              {STAGES.map((s) => (
                                <option key={s.id} value={s.id}>
                                  {s.label}
                                </option>
                              ))}
                            </select>

                            <div className="flex items-center gap-1.5">
                              <button
                                onClick={() => {
                                  setResubmissionDeal(deal);
                                  setResubmissionData({
                                    title: `Cross-Selling: ${deal.title}`,
                                    product_segment: 'Stromspeicher',
                                    due_date: new Date(Date.now() + 90 * 86400000)
                                      .toISOString()
                                      .split('T')[0],
                                    notes: `Folgekontakt für ${deal.title} (${deal.value} €)`,
                                  });
                                }}
                                className="text-[10px] px-2 py-0.5 rounded bg-purple-500/10 hover:bg-purple-500/20 text-purple-400 border border-purple-500/20 font-medium inline-flex items-center gap-1 transition-colors cursor-pointer"
                                title="Wiedervorlage & Cross-Selling anlegen (§3.5)"
                              >
                                <RefreshCw className="w-2.5 h-2.5" />
                                <span>Wiedervorlage</span>
                              </button>
                              {deal.stage === 'WON' && (
                                <span className="text-[10px] px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-medium inline-flex items-center gap-1">
                                  <ShieldCheck className="w-3 h-3" /> §355 OK
                                </span>
                              )}
                            </div>
                          </div>
                        </div>
                      ))
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </DndContext>

      {/* Create Deal Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg">Neuen Deal anlegen (§3.3)</h3>
              <button
                onClick={() => setIsModalOpen(false)}
                className="text-slate-400 hover:text-slate-200"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">
                  Deal Titel *
                </label>
                <input
                  required
                  type="text"
                  placeholder="z.B. 10 kWp PV-Anlage & Speicher Müller"
                  value={formData.title}
                  onChange={(e) => setFormData({ ...formData, title: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <label className="block text-xs font-medium text-slate-400 mb-1">
                    Volumen (€)
                  </label>
                  <input
                    type="text"
                    value={formData.value}
                    onChange={(e) => setFormData({ ...formData, value: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">
                    Wahrsch. (%)
                  </label>
                  <input
                    type="number"
                    min="0"
                    max="100"
                    value={formData.probability}
                    onChange={(e) =>
                      setFormData({ ...formData, probability: Number(e.target.value) })
                    }
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Phase</label>
                <select
                  value={formData.stage}
                  onChange={(e) => setFormData({ ...formData, stage: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                >
                  {STAGES.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.label}
                    </option>
                  ))}
                </select>
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setIsModalOpen(false)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-sm font-medium transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={createMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors"
                >
                  {createMutation.isPending ? 'Speichert...' : 'Deal erstellen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Deal Modal */}
      {editingDeal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <Edit3 className="w-5 h-5 text-emerald-400" />
                <span>Deal bearbeiten</span>
              </h3>
              <button
                onClick={() => setEditingDeal(null)}
                className="text-slate-400 hover:text-slate-200"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUpdate} className="space-y-4">
              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">
                  Deal Titel *
                </label>
                <input
                  required
                  type="text"
                  value={editingDeal.title}
                  onChange={(e) => setEditingDeal({ ...editingDeal, title: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <label className="block text-xs font-medium text-slate-400 mb-1">
                    Volumen (€)
                  </label>
                  <input
                    type="text"
                    value={editingDeal.value}
                    onChange={(e) => setEditingDeal({ ...editingDeal, value: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">
                    Wahrsch. (%)
                  </label>
                  <input
                    type="number"
                    min="0"
                    max="100"
                    value={editingDeal.probability}
                    onChange={(e) =>
                      setEditingDeal({ ...editingDeal, probability: Number(e.target.value) })
                    }
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Phase</label>
                <select
                  value={editingDeal.stage}
                  onChange={(e) => setEditingDeal({ ...editingDeal, stage: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                >
                  {STAGES.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.label}
                    </option>
                  ))}
                </select>
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setEditingDeal(null)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-sm font-medium transition-colors"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  disabled={updateMutation.isPending}
                  className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors"
                >
                  {updateMutation.isPending ? 'Speichert...' : 'Änderungen speichern'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Resubmission & Cross-Selling Modal (§3.5) */}
      {resubmissionDeal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <RefreshCw className="w-5 h-5 text-purple-400" />
                Wiedervorlage & Cross-Selling anlegen (§3.5)
              </h3>
              <button
                onClick={() => setResubmissionDeal(null)}
                className="text-slate-400 hover:text-slate-200"
              >
                <X className="w-5 h-5" />
              </button>
            </div>

            <div className="p-3 bg-purple-500/10 border border-purple-500/20 rounded-xl text-xs text-purple-300">
              Verknüpft mit Deal:{' '}
              <strong className="text-purple-200">{resubmissionDeal.title}</strong> (
              {resubmissionDeal.value} EUR)
            </div>

            <form
              onSubmit={async (e) => {
                e.preventDefault();
                await apiFetch('/api/v1/todos', {
                  method: 'POST',
                  body: JSON.stringify({
                    title: resubmissionData.title,
                    description: resubmissionData.notes,
                    category: 'CROSS_SELLING',
                    product_segment: resubmissionData.product_segment,
                    contact_name: resubmissionDeal.contact_name || 'Kunde',
                    due_date: resubmissionData.due_date,
                    priority: 'HIGH',
                    status: 'OPEN',
                  }),
                });
                queryClient.invalidateQueries({ queryKey: ['todos'] });
                setResubmissionDeal(null);
                setFeedbackBanner(
                  `Wiedervorlage für "${resubmissionData.product_segment}" erfolgreich angelegt!`,
                );
                setTimeout(() => setFeedbackBanner(null), 4000);
              }}
              className="space-y-3"
            >
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Titel / Betreff
                </label>
                <input
                  type="text"
                  required
                  value={resubmissionData.title}
                  onChange={(e) =>
                    setResubmissionData({ ...resubmissionData, title: e.target.value })
                  }
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-purple-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    Cross-Selling Produkt
                  </label>
                  <select
                    value={resubmissionData.product_segment}
                    onChange={(e) =>
                      setResubmissionData({ ...resubmissionData, product_segment: e.target.value })
                    }
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-purple-500"
                  >
                    <option value="Stromspeicher">Stromspeicher-Nachrüstung</option>
                    <option value="Wärmepumpe">Wärmepumpe & Sektorkopplung</option>
                    <option value="Wallbox / E-Mobilität">Wallbox / E-Mobilität</option>
                    <option value="Wartungsvertrag / Service">Jahreswartung & Ertragscheck</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-semibold text-slate-300 mb-1">
                    Wiedervorlage-Datum
                  </label>
                  <input
                    type="date"
                    required
                    value={resubmissionData.due_date}
                    onChange={(e) =>
                      setResubmissionData({ ...resubmissionData, due_date: e.target.value })
                    }
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-purple-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Notizen zum Folgeangebot
                </label>
                <textarea
                  rows={2}
                  value={resubmissionData.notes}
                  onChange={(e) =>
                    setResubmissionData({ ...resubmissionData, notes: e.target.value })
                  }
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-purple-500 resize-none"
                />
              </div>

              <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setResubmissionDeal(null)}
                  className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-semibold"
                >
                  Abbrechen
                </button>
                <button
                  type="submit"
                  className="px-4 py-2 bg-purple-600 hover:bg-purple-500 text-white rounded-xl text-xs font-bold shadow-lg shadow-purple-600/20"
                >
                  Wiedervorlage speichern
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deletingDeal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center gap-3 text-rose-400 border-b border-slate-800 pb-3">
              <AlertCircle className="w-6 h-6 shrink-0" />
              <h3 className="font-bold text-slate-100 text-base">Deal wirklich löschen?</h3>
            </div>
            <p className="text-xs text-slate-300 leading-relaxed">
              Möchten Sie den Deal <strong className="text-slate-100">{deletingDeal.title}</strong>{' '}
              unwiderruflich aus der Pipeline entfernen?
            </p>
            <div className="flex justify-end gap-3 pt-3">
              <button
                onClick={() => setDeletingDeal(null)}
                className="px-3.5 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs font-semibold"
              >
                Abbrechen
              </button>
              <button
                onClick={() => deleteMutation.mutate(deletingDeal.id)}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-500 text-white font-semibold rounded-lg text-xs"
              >
                {deleteMutation.isPending ? 'Löscht...' : 'Endgültig löschen'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Offer Calculator & OCR Modal (§5.2 / §5.6) */}
      {isCalculatorOpen && <OfferCalculatorModal onClose={() => setIsCalculatorOpen(false)} />}
    </div>
  );
};
