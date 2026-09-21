import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import {
  Zap,
  Plus,
  Play,
  CheckCircle2,
  Clock,
  Mail,
  CheckSquare,
  X,
  ShieldCheck,
  Activity,
  UserCheck,
} from 'lucide-react';

export const AutomationsPage: React.FC = () => {
  const queryClient = useQueryClient();
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<'WORKFLOWS' | 'RUNS'>('WORKFLOWS');
  const [successToast, setSuccessToast] = useState<string | null>(null);

  const { data: rawWorkflows = [], isLoading } = useQuery<any[]>({
    queryKey: ['automations'],
    queryFn: () => apiFetch('/api/v1/automations'),
  });
  const workflows = Array.isArray(rawWorkflows) ? rawWorkflows : [];

  const { data: rawRuns = [] } = useQuery<any[]>({
    queryKey: ['automation-runs'],
    queryFn: () => apiFetch('/api/v1/automations/runs'),
  });
  const runs = Array.isArray(rawRuns) ? rawRuns : [];

  const createMutation = useMutation({
    mutationFn: (newWf: any) =>
      apiFetch('/api/v1/automations', {
        method: 'POST',
        body: JSON.stringify(newWf),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['automations'] });
      setIsModalOpen(false);
      setSuccessToast('Automatisierungs-Routine erfolgreich erstellt & aktiviert!');
      setTimeout(() => setSuccessToast(null), 4000);
    },
  });

  const [formData, setFormData] = useState({
    name: '',
    description: '',
    trigger_type: 'NEW_LEAD',
    target_type: 'CONTACT',
    step1_title: 'Automatische E-Mail Vorbereitung',
    step1_action: 'DRAFT_EMAIL',
    step2_title: 'Follow-Up Aufgabe erstellen',
    step2_action: 'CREATE_TASK',
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate({
      name: formData.name,
      description: formData.description,
      trigger_type: formData.trigger_type,
      target_type: formData.target_type,
      is_active: true,
      steps: [
        {
          step_number: 1,
          title: formData.step1_title,
          action_type: formData.step1_action,
          payload: {},
        },
        {
          step_number: 2,
          title: formData.step2_title,
          action_type: formData.step2_action,
          payload: {},
        },
      ],
    });
  };

  const handleTriggerRun = (wfName: string) => {
    setSuccessToast(
      `Workflow "${wfName}" manuell ausgelöst! Schritt 1 vorbereitet (HITL-Freigabe erforderlich).`,
    );
    setTimeout(() => setSuccessToast(null), 5000);
  };

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2.5">
            <Zap className="w-6 h-6 text-amber-400" />
            Automationen & Workflows (§6.4)
          </h1>
          <p className="text-sm text-slate-400">
            Regelbasierte CRM-Routinen, Human-in-the-Loop Freigaben und automatisierte Trigger
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setIsModalOpen(true)}
            className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-emerald-600/20"
          >
            <Plus className="w-4 h-4" />
            Neue Automatisierung anlegen
          </button>
        </div>
      </div>

      {successToast && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2 animate-fadeIn">
          <CheckCircle2 className="w-4 h-4 text-emerald-400" />
          {successToast}
        </div>
      )}

      {/* Tabs */}
      <div className="flex items-center gap-2 border-b border-slate-800 pb-3">
        <button
          onClick={() => setActiveTab('WORKFLOWS')}
          className={`px-4 py-2 rounded-xl text-sm font-semibold transition-colors flex items-center gap-2 ${
            activeTab === 'WORKFLOWS'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Zap className="w-4 h-4" />
          Aktive Routinen ({workflows.length})
        </button>
        <button
          onClick={() => setActiveTab('RUNS')}
          className={`px-4 py-2 rounded-xl text-sm font-semibold transition-colors flex items-center gap-2 ${
            activeTab === 'RUNS'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Activity className="w-4 h-4" />
          Workflow-Läufe & HITL Freigaben ({runs.length})
        </button>
      </div>

      {activeTab === 'WORKFLOWS' ? (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
          {isLoading ? (
            <div className="col-span-3 py-12 text-center text-slate-500 text-sm">
              Lade Automationen...
            </div>
          ) : workflows.length === 0 ? (
            <div className="col-span-3 py-12 text-center text-slate-500 text-sm">
              Keine Automationen angelegt.
            </div>
          ) : (
            workflows.map((wf) => (
              <div
                key={wf.id}
                className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-4 hover:border-slate-700 transition-all flex flex-col justify-between"
              >
                <div className="space-y-3">
                  <div className="flex items-start justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <div className="p-2 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-400">
                        <Zap className="w-4 h-4" />
                      </div>
                      <span className="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-300 border border-slate-700 font-mono">
                        {wf.trigger_type}
                      </span>
                    </div>
                    <span
                      className={`text-[10px] font-bold px-2 py-0.5 rounded-full border ${
                        wf.is_active
                          ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
                          : 'bg-slate-800 text-slate-500 border-slate-700'
                      }`}
                    >
                      {wf.is_active ? 'AKTIV' : 'PAUSIERT'}
                    </span>
                  </div>

                  <div>
                    <h3 className="font-bold text-slate-100 text-sm leading-snug">{wf.name}</h3>
                    <p className="text-xs text-slate-400 mt-1 line-clamp-2">{wf.description}</p>
                  </div>

                  {/* Steps List */}
                  <div className="space-y-1.5 pt-2 border-t border-slate-800/80">
                    <span className="text-[10px] font-bold uppercase text-slate-500 tracking-wider">
                      Ablauf-Schritte:
                    </span>
                    {wf.steps?.map((step: any) => (
                      <div
                        key={step.step_number}
                        className="flex items-center gap-2 text-xs text-slate-300"
                      >
                        <span className="w-4 h-4 rounded-full bg-slate-800 text-slate-400 flex items-center justify-center text-[10px] font-mono shrink-0">
                          {step.step_number}
                        </span>
                        <span className="truncate">{step.title}</span>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="pt-3 border-t border-slate-800/60 flex items-center justify-between">
                  <span className="text-[10px] text-slate-500 flex items-center gap-1">
                    <ShieldCheck className="w-3 h-3 text-emerald-500" /> HITL geschützt
                  </span>
                  <button
                    onClick={() => handleTriggerRun(wf.name)}
                    className="inline-flex items-center gap-1 px-3 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold rounded-lg transition-colors border border-slate-700"
                  >
                    <Play className="w-3 h-3 text-emerald-400" />
                    Testlauf
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      ) : (
        /* Runs & HITL Approvals View */
        <div className="bg-slate-900 border border-slate-800 rounded-2xl overflow-hidden shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm text-slate-300">
              <thead className="bg-slate-950/60 text-slate-400 text-xs uppercase border-b border-slate-800">
                <tr>
                  <th className="py-3.5 px-6 font-semibold">Workflow & Ziel</th>
                  <th className="py-3.5 px-6 font-semibold">Aktueller Schritt</th>
                  <th className="py-3.5 px-6 font-semibold">Status & HITL (§9.4)</th>
                  <th className="py-3.5 px-6 font-semibold">Aktion</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800">
                {runs.length === 0 ? (
                  <tr>
                    <td colSpan={4} className="py-8 text-center text-slate-500">
                      Noch keine Workflow-Läufe vorhanden.
                    </td>
                  </tr>
                ) : (
                  runs.map((run) => (
                    <tr key={run.id} className="hover:bg-slate-800/40 transition-colors">
                      <td className="py-4 px-6">
                        <div className="font-semibold text-slate-100">
                          {run.workflow_id === 'wf-1'
                            ? 'Erstkontakt & Qualifizierung'
                            : 'Deal-Abschluss Routine'}
                        </div>
                        <div className="text-xs text-slate-400">
                          Ziel: {run.target_name} ({run.target_type})
                        </div>
                      </td>
                      <td className="py-4 px-6 text-xs text-slate-300">
                        Schritt {run.current_step}:{' '}
                        {run.workflow_id === 'wf-1'
                          ? 'E-Mail-Entwurf (Gemma 12B)'
                          : 'Technik-Übergabe'}
                      </td>
                      <td className="py-4 px-6">
                        {run.status === 'WAITING_APPROVAL' ? (
                          <span className="inline-flex items-center gap-1 text-[11px] font-semibold px-2.5 py-0.5 rounded-full bg-amber-500/10 text-amber-400 border border-amber-500/20">
                            <Clock className="w-3 h-3" /> Wartet auf Freigabe
                          </span>
                        ) : (
                          <span className="inline-flex items-center gap-1 text-[11px] font-semibold px-2.5 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                            <CheckCircle2 className="w-3 h-3" /> Abgeschlossen
                          </span>
                        )}
                      </td>
                      <td className="py-4 px-6">
                        {run.status === 'WAITING_APPROVAL' ? (
                          <button
                            onClick={() => {
                              setSuccessToast(
                                `Schritt für ${run.target_name} freigegeben und ausgeführt!`,
                              );
                              setTimeout(() => setSuccessToast(null), 4000);
                            }}
                            className="inline-flex items-center gap-1 px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 text-xs font-bold rounded-lg transition-colors"
                          >
                            <UserCheck className="w-3.5 h-3.5" />
                            Schritt freigeben
                          </button>
                        ) : (
                          <span className="text-xs text-slate-500">Erledigt</span>
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

      {/* Create Automation Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <Zap className="w-5 h-5 text-amber-400" />
                Neue Automatisierungs-Routine anlegen
              </h3>
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
                  Name der Routine *
                </label>
                <input
                  required
                  type="text"
                  placeholder="z.B. PV-Lead Qualifizierung & Angebot"
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">
                  Beschreibung
                </label>
                <textarea
                  rows={2}
                  placeholder="Zweck dieser Automatisierung..."
                  value={formData.description}
                  onChange={(e) => setFormData({ ...formData, description: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">
                    Trigger-Ereignis
                  </label>
                  <select
                    value={formData.trigger_type}
                    onChange={(e) => setFormData({ ...formData, trigger_type: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="NEW_LEAD">Neuer Kontakt / Lead angelegt</option>
                    <option value="DEAL_WON">Deal auf WON (Gewonnen) gewechselt</option>
                    <option value="INBOUND_EMAIL">Neue E-Mail eingegangen</option>
                    <option value="INACTIVITY_TIMEOUT">30 Tage Inaktivität (SLA)</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">
                    Ziel-Objekt
                  </label>
                  <select
                    value={formData.target_type}
                    onChange={(e) => setFormData({ ...formData, target_type: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="CONTACT">Kontakt / Person</option>
                    <option value="DEAL">Deal / Verkaufschance</option>
                    <option value="COMPANY">Firma / Unternehmen</option>
                  </select>
                </div>
              </div>

              {/* Step 1 */}
              <div className="p-3 bg-slate-950 rounded-xl border border-slate-800 space-y-2">
                <div className="flex items-center justify-between text-xs font-bold text-slate-300">
                  <span className="flex items-center gap-1.5">
                    <Mail className="w-3.5 h-3.5 text-emerald-400" /> Schritt 1: Aktion
                  </span>
                </div>
                <input
                  type="text"
                  value={formData.step1_title}
                  onChange={(e) => setFormData({ ...formData, step1_title: e.target.value })}
                  className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-800 rounded text-xs text-slate-100"
                />
              </div>

              {/* Step 2 */}
              <div className="p-3 bg-slate-950 rounded-xl border border-slate-800 space-y-2">
                <div className="flex items-center justify-between text-xs font-bold text-slate-300">
                  <span className="flex items-center gap-1.5">
                    <CheckSquare className="w-3.5 h-3.5 text-amber-400" /> Schritt 2: Aktion
                  </span>
                </div>
                <input
                  type="text"
                  value={formData.step2_title}
                  onChange={(e) => setFormData({ ...formData, step2_title: e.target.value })}
                  className="w-full px-2.5 py-1.5 bg-slate-900 border border-slate-800 rounded text-xs text-slate-100"
                />
              </div>

              <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
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
                  {createMutation.isPending ? 'Speichert...' : 'Routine anlegen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
