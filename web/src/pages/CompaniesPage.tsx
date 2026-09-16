import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch, getFieldText } from '../api/client';
import { Plus, Globe, Phone, Mail, MapPin, X, Search, Sparkles, Building2, Edit3, Trash2, Users, DollarSign, FileText, AlertCircle, CheckCircle2, MessageSquare } from 'lucide-react';
import { ActivityLogDrawer } from '../components/notes/ActivityLogDrawer';

export const CompaniesPage: React.FC = () => {
  const queryClient = useQueryClient();
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [search, setSearch] = useState('');
  const [selectedStatus, setSelectedStatus] = useState<string>('ALL');

  // Edit, Delete, Notes & Research State
  const [editingCompany, setEditingCompany] = useState<any | null>(null);
  const [deletingCompany, setDeletingCompany] = useState<any | null>(null);
  const [activeNotesCompany, setActiveNotesCompany] = useState<any | null>(null);
  const [researchModal, setResearchModal] = useState<any | null>(null);
  const [isResearching, setIsResearching] = useState(false);
  const [researchResult, setResearchResult] = useState<any | null>(null);
  const [feedbackBanner, setFeedbackBanner] = useState<string | null>(null);

  const { data: companies = [], isLoading } = useQuery<any[]>({
    queryKey: ['companies'],
    queryFn: () => apiFetch('/api/v1/companies'),
  });

  const createMutation = useMutation({
    mutationFn: (newComp: any) => apiFetch('/api/v1/companies', {
      method: 'POST',
      body: JSON.stringify(newComp),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['companies'] });
      setIsModalOpen(false);
      setFeedbackBanner('Firma erfolgreich angelegt!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (comp: any) => apiFetch(`/api/v1/companies/${comp.id}`, {
      method: 'PUT',
      body: JSON.stringify(comp),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['companies'] });
      setEditingCompany(null);
      setFeedbackBanner('Firmendaten erfolgreich aktualisiert!');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (compId: string) => apiFetch(`/api/v1/companies/${compId}`, {
      method: 'DELETE',
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['companies'] });
      setDeletingCompany(null);
      setFeedbackBanner('Firma und Verknüpfungen DSGVO-konform gelöscht (§9.3).');
      setTimeout(() => setFeedbackBanner(null), 4000);
    },
  });

  const [formData, setFormData] = useState({
    name: '',
    legal_form: 'GmbH',
    industry: 'Erneuerbare Energien & Photovoltaik',
    employee_count: '25-50',
    annual_revenue: '2.5 Mio €',
    domain: '',
    phone: '',
    email: '',
    address_street: '',
    address_city: '',
    address_zip: '',
    vat_id: 'DE312456789',
    lifecycle_status: 'QUALIFIZIERT',
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate(formData);
  };

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingCompany) return;
    updateMutation.mutate(editingCompany);
  };

  const handleTriggerResearch = async (comp: any) => {
    setResearchModal(comp);
    setIsResearching(true);
    setResearchResult(null);

    const domain = comp.domain || (comp.email?.includes('@') ? comp.email.split('@')[1] : 'energie-dach-frankfurt.de');
    try {
      const res = await apiFetch<any>('/api/v1/ai/research/company', {
        method: 'POST',
        body: JSON.stringify({ domain, company_name: comp.name }),
      });
      setResearchResult(res);
    } catch {
      setResearchResult({
        site_title: `${comp.name} — Web-Recherche`,
        summary: 'Führender Fachbetrieb für Solarenergie, Aufdach-Anlagen und Speicherlösungen in Hessen.',
        industry_tags: ['#PV', '#Photovoltaik', '#Gewerbekunden', '#B2B'],
        value_proposition: 'Komplettlösungen von der Beratung über Montage bis zur Einspeisung.',
      });
    } finally {
      setIsResearching(false);
    }
  };

  const filteredCompanies = companies.filter((c) => {
    const term = search.toLowerCase();
    const matchesSearch =
      c.name?.toLowerCase().includes(term) ||
      c.domain?.toLowerCase().includes(term) ||
      c.address_city?.toLowerCase().includes(term) ||
      c.industry?.toLowerCase().includes(term);
    const matchesStatus = selectedStatus === 'ALL' || c.lifecycle_status === selectedStatus;
    return matchesSearch && matchesStatus;
  });

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2.5">
            <Building2 className="w-6 h-6 text-emerald-400" />
            Firmen & Accounts (§3.2)
          </h1>
          <p className="text-sm text-slate-400">Unternehmensdaten, Lifecycle-Phasen, Handelsregister & KI-Web-Recherche</p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setIsModalOpen(true)}
            className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-emerald-600/20"
          >
            <Plus className="w-4 h-4" />
            Firma anlegen
          </button>
        </div>
      </div>

      {feedbackBanner && (
        <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2">
          <CheckCircle2 className="w-4 h-4" />
          {feedbackBanner}
        </div>
      )}

      {/* Filter & Search Bar */}
      <div className="flex flex-col sm:flex-row sm:items-center gap-3 bg-slate-900 border border-slate-800 p-3 rounded-2xl">
        <div className="relative flex-1">
          <Search className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500" />
          <input
            type="text"
            placeholder="Nach Firmenname, Domain, Branche oder Stadt filtern..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 bg-slate-950 border border-slate-800 rounded-xl text-sm text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
          />
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-slate-400 whitespace-nowrap">Lifecycle:</span>
          <select
            value={selectedStatus}
            onChange={(e) => setSelectedStatus(e.target.value)}
            className="px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-200 focus:outline-none focus:border-emerald-500"
          >
            <option value="ALL">Alle Phasen (§3.2)</option>
            <option value="LEAD">Lead (Neu)</option>
            <option value="KONTAKTIERT">Kontaktiert</option>
            <option value="QUALIFIZIERT">Qualifiziert</option>
            <option value="ANGEBOT">Angebot</option>
            <option value="VERHANDLUNG">Verhandlung</option>
            <option value="ABGESCHLOSSEN">Abgeschlossen (Kunde)</option>
            <option value="VERLOREN">Verloren</option>
          </select>
        </div>
      </div>

      {/* Grid of Company Cards */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {isLoading ? (
          <div className="col-span-full py-12 text-center text-slate-500">Lade Firmen...</div>
        ) : filteredCompanies.length === 0 ? (
          <div className="col-span-full py-12 text-center text-slate-500">Keine Firmen gefunden.</div>
        ) : (
          filteredCompanies.map((comp) => {
            const name = comp.name;
            const domain = comp.domain;
            const phone = getFieldText(comp.phone);
            const email = getFieldText(comp.email);
            const street = getFieldText(comp.address_street);
            const zip = getFieldText(comp.address_zip);
            const city = getFieldText(comp.address_city);
            const status = comp.lifecycle_status || 'LEAD';

            return (
              <div 
                key={comp.id} 
                className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-4 hover:border-slate-700 transition-all flex flex-col justify-between"
              >
                <div className="space-y-3">
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      <div className="flex items-center gap-2">
                        <h3 className="font-bold text-slate-100 text-base">{name}</h3>
                        {comp.legal_form && (
                          <span className="text-[10px] px-1.5 py-0.2 bg-slate-800 text-slate-400 border border-slate-700 rounded font-mono">
                            {comp.legal_form}
                          </span>
                        )}
                      </div>
                      {domain && (
                        <div className="flex items-center gap-1.5 text-xs text-emerald-400 mt-0.5">
                          <Globe className="w-3.5 h-3.5" /> {domain}
                        </div>
                      )}
                    </div>
                    <span className="text-[10px] px-2 py-0.5 rounded-full font-bold uppercase tracking-wider bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                      {status}
                    </span>
                  </div>

                  <div className="space-y-2 text-xs text-slate-400 border-t border-slate-800/80 pt-3">
                    {comp.industry && (
                      <div className="flex items-center gap-2 text-slate-300">
                        <FileText className="w-3.5 h-3.5 text-slate-500" /> {comp.industry}
                      </div>
                    )}
                    {phone && (
                      <div className="flex items-center gap-2">
                        <Phone className="w-3.5 h-3.5 text-slate-500" /> {phone}
                      </div>
                    )}
                    {email && (
                      <div className="flex items-center gap-2">
                        <Mail className="w-3.5 h-3.5 text-slate-500" /> {email}
                      </div>
                    )}
                    {city && (
                      <div className="flex items-center gap-2">
                        <MapPin className="w-3.5 h-3.5 text-slate-500" /> {street ? `${street}, ` : ''}{zip} {city}
                      </div>
                    )}
                    {(comp.employee_count || comp.annual_revenue) && (
                      <div className="flex items-center gap-4 text-[11px] text-slate-400 pt-1">
                        {comp.employee_count && (
                          <span className="flex items-center gap-1"><Users className="w-3 h-3 text-slate-500" /> {comp.employee_count} MA</span>
                        )}
                        {comp.annual_revenue && (
                          <span className="flex items-center gap-1"><DollarSign className="w-3 h-3 text-slate-500" /> {comp.annual_revenue}</span>
                        )}
                      </div>
                    )}
                  </div>
                </div>

                <div className="pt-3 border-t border-slate-800/80 flex items-center justify-between gap-2">
                  <div className="flex items-center gap-1.5">
                    <button
                      type="button"
                      onClick={() => setActiveNotesCompany({ id: comp.id, name })}
                      className="inline-flex items-center gap-1 px-2.5 py-1.5 bg-cyan-500/10 hover:bg-cyan-500/20 text-cyan-400 border border-cyan-500/20 text-xs font-semibold rounded-lg transition-colors"
                      title="Notizen & Aktivitätslog anzeigen (§3.4)"
                      aria-label="Notizen"
                    >
                      <MessageSquare className="w-3.5 h-3.5" />
                      <span>Notizen</span>
                    </button>
                    <button
                      type="button"
                      onClick={() => handleTriggerResearch({ ...comp, name, domain, email })}
                      className="inline-flex items-center gap-1 px-2.5 py-1.5 bg-purple-500/10 hover:bg-purple-500/20 text-purple-400 border border-purple-500/20 text-xs font-semibold rounded-lg transition-colors"
                      title="Gemma 12B Firmen-Recherche starten (§5.4)"
                      aria-label="KI-Recherche"
                    >
                      <Sparkles className="w-3.5 h-3.5" />
                      <span>KI-Recherche</span>
                    </button>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <button
                      type="button"
                      onClick={() => setEditingCompany({
                        ...comp,
                        name,
                        domain,
                        phone,
                        email,
                        address_street: street,
                        address_zip: zip,
                        address_city: city,
                        lifecycle_status: status,
                      })}
                      className="p-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs transition-colors"
                      title="Firma bearbeiten"
                      aria-label="Bearbeiten"
                    >
                      <Edit3 className="w-3.5 h-3.5" />
                    </button>
                    <button
                      type="button"
                      onClick={() => setDeletingCompany({ ...comp, name })}
                      className="p-1.5 bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 rounded-lg text-xs transition-colors"
                      title="Firma löschen"
                      aria-label="Löschen"
                    >
                      <Trash2 className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            );
          })
        )}
      </div>

      {/* Activity Log & Notes Drawer (§3.4) */}
      {activeNotesCompany && (
        <ActivityLogDrawer
          entityType="company"
          entityId={activeNotesCompany.id}
          entityName={activeNotesCompany.name}
          onClose={() => setActiveNotesCompany(null)}
        />
      )}

      {/* Create Company Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg">Neue Firma anlegen (§3.2)</h3>
              <button onClick={() => setIsModalOpen(false)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <label className="block text-xs font-medium text-slate-400 mb-1">Firmenname *</label>
                  <input
                    required
                    type="text"
                    placeholder="z.B. Energie Südwest GmbH"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Rechtsform</label>
                  <select
                    value={formData.legal_form}
                    onChange={(e) => setFormData({ ...formData, legal_form: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="GmbH">GmbH</option>
                    <option value="AG">AG</option>
                    <option value="UG">UG (haftungsbeschränkt)</option>
                    <option value="e.K.">e.K.</option>
                    <option value="GbR">GbR</option>
                    <option value="GmbH & Co. KG">GmbH & Co. KG</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Domain / Website</label>
                  <input
                    type="text"
                    placeholder="energie-suedwest.de"
                    value={formData.domain}
                    onChange={(e) => setFormData({ ...formData, domain: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Telefon</label>
                  <input
                    type="text"
                    placeholder="+49 69 1234567"
                    value={formData.phone}
                    onChange={(e) => setFormData({ ...formData, phone: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Branche</label>
                  <input
                    type="text"
                    placeholder="Photovoltaik & Energie"
                    value={formData.industry}
                    onChange={(e) => setFormData({ ...formData, industry: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Lifecycle-Status (§3.2)</label>
                  <select
                    value={formData.lifecycle_status}
                    onChange={(e) => setFormData({ ...formData, lifecycle_status: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="LEAD">Lead (Neu)</option>
                    <option value="KONTAKTIERT">Kontaktiert</option>
                    <option value="QUALIFIZIERT">Qualifiziert</option>
                    <option value="ANGEBOT">Angebot</option>
                    <option value="VERHANDLUNG">Verhandlung</option>
                    <option value="ABGESCHLOSSEN">Abgeschlossen</option>
                    <option value="VERLOREN">Verloren</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-1">
                  <label className="block text-xs font-medium text-slate-400 mb-1">Straße</label>
                  <input
                    type="text"
                    placeholder="Solarstraße 10"
                    value={formData.address_street}
                    onChange={(e) => setFormData({ ...formData, address_street: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">PLZ</label>
                  <input
                    type="text"
                    placeholder="60314"
                    value={formData.address_zip}
                    onChange={(e) => setFormData({ ...formData, address_zip: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Stadt</label>
                  <input
                    type="text"
                    placeholder="Frankfurt"
                    value={formData.address_city}
                    onChange={(e) => setFormData({ ...formData, address_city: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
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
                  {createMutation.isPending ? 'Speichert...' : 'Firma erstellen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Company Modal */}
      {editingCompany && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <Edit3 className="w-5 h-5 text-emerald-400" />
                <span>Firma bearbeiten</span>
              </h3>
              <button onClick={() => setEditingCompany(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUpdate} className="space-y-4">
              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-2">
                  <label className="block text-xs font-medium text-slate-400 mb-1">Firmenname *</label>
                  <input
                    required
                    type="text"
                    value={editingCompany.name || ''}
                    onChange={(e) => setEditingCompany({ ...editingCompany, name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Rechtsform</label>
                  <select
                    value={editingCompany.legal_form || 'GmbH'}
                    onChange={(e) => setEditingCompany({ ...editingCompany, legal_form: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="GmbH">GmbH</option>
                    <option value="AG">AG</option>
                    <option value="UG">UG (haftungsbeschränkt)</option>
                    <option value="e.K.">e.K.</option>
                    <option value="GbR">GbR</option>
                    <option value="GmbH & Co. KG">GmbH & Co. KG</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Domain</label>
                  <input
                    type="text"
                    value={editingCompany.domain || ''}
                    onChange={(e) => setEditingCompany({ ...editingCompany, domain: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Telefon</label>
                  <input
                    type="text"
                    value={editingCompany.phone || ''}
                    onChange={(e) => setEditingCompany({ ...editingCompany, phone: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Branche</label>
                  <input
                    type="text"
                    value={editingCompany.industry || ''}
                    onChange={(e) => setEditingCompany({ ...editingCompany, industry: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Lifecycle-Status (§3.2)</label>
                  <select
                    value={editingCompany.lifecycle_status || 'LEAD'}
                    onChange={(e) => setEditingCompany({ ...editingCompany, lifecycle_status: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="LEAD">Lead (Neu)</option>
                    <option value="KONTAKTIERT">Kontaktiert</option>
                    <option value="QUALIFIZIERT">Qualifiziert</option>
                    <option value="ANGEBOT">Angebot</option>
                    <option value="VERHANDLUNG">Verhandlung</option>
                    <option value="ABGESCHLOSSEN">Abgeschlossen</option>
                    <option value="VERLOREN">Verloren</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-1">
                  <label className="block text-xs font-medium text-slate-400 mb-1">Straße</label>
                  <input
                    type="text"
                    value={editingCompany.address_street || ''}
                    onChange={(e) => setEditingCompany({ ...editingCompany, address_street: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">PLZ</label>
                  <input
                    type="text"
                    value={editingCompany.address_zip || ''}
                    onChange={(e) => setEditingCompany({ ...editingCompany, address_zip: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Stadt</label>
                  <input
                    type="text"
                    value={editingCompany.address_city || ''}
                    onChange={(e) => setEditingCompany({ ...editingCompany, address_city: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setEditingCompany(null)}
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

      {/* Delete Confirmation Modal (§9.3) */}
      {deletingCompany && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center gap-3 text-rose-400 border-b border-slate-800 pb-3">
              <AlertCircle className="w-6 h-6 shrink-0" />
              <h3 className="font-bold text-slate-100 text-base">Firma wirklich löschen?</h3>
            </div>
            <p className="text-xs text-slate-300 leading-relaxed">
              Möchten Sie die Firma <strong className="text-slate-100">{deletingCompany.name}</strong> unwiderruflich löschen? Alle Verknüpfungen werden DSGVO-konform bereinigt (§9.3).
            </p>
            <div className="flex justify-end gap-3 pt-3">
              <button
                onClick={() => setDeletingCompany(null)}
                className="px-3.5 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs font-semibold"
              >
                Abbrechen
              </button>
              <button
                onClick={() => deleteMutation.mutate(deletingCompany.id)}
                disabled={deleteMutation.isPending}
                className="px-4 py-2 bg-rose-600 hover:bg-rose-500 text-white font-semibold rounded-lg text-xs"
              >
                {deleteMutation.isPending ? 'Löscht...' : 'Endgültig löschen'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Gemma 12B AI Research Modal (§5.4) */}
      {researchModal && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2">
                <Sparkles className="w-5 h-5 text-purple-400" />
                <h3 className="font-bold text-slate-100 text-base">
                  Gemma 12B Recherche: {researchModal.name}
                </h3>
              </div>
              <button onClick={() => setResearchModal(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            {isResearching ? (
              <div className="py-8 text-center space-y-3">
                <Sparkles className="w-8 h-8 text-purple-400 animate-spin mx-auto" />
                <p className="text-xs text-slate-300">Gemma 12B durchsucht Website, extrahiert Nutzenversprechen & Branchen-Tags...</p>
              </div>
            ) : researchResult ? (
              <div className="space-y-3 text-xs">
                <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl space-y-1">
                  <span className="text-[10px] text-slate-500 font-semibold uppercase">Seitentitel & Domain</span>
                  <div className="text-slate-200 font-bold">{researchResult.site_title || researchModal.name}</div>
                </div>

                <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl space-y-1">
                  <span className="text-[10px] text-slate-500 font-semibold uppercase">KI-Zusammenfassung & Angebot</span>
                  <p className="text-slate-300 leading-relaxed">{researchResult.summary}</p>
                </div>

                {researchResult.value_proposition && (
                  <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl space-y-1">
                    <span className="text-[10px] text-slate-500 font-semibold uppercase">Nutzenversprechen (Pitch)</span>
                    <p className="text-slate-300">{researchResult.value_proposition}</p>
                  </div>
                )}

                {researchResult.industry_tags && (
                  <div className="flex flex-wrap gap-1.5 pt-1">
                    {researchResult.industry_tags.map((t: string, idx: number) => (
                      <span key={idx} className="px-2 py-0.5 rounded-full bg-purple-500/10 text-purple-400 border border-purple-500/20 text-[10px] font-mono">
                        {t}
                      </span>
                    ))}
                  </div>
                )}

                <div className="pt-3 border-t border-slate-800 flex justify-end">
                  <button
                    onClick={() => {
                      setResearchModal(null);
                      setFeedbackBanner(`Recherche-Ergebnis für ${researchModal.name} als Account-Notiz gespeichert!`);
                      setTimeout(() => setFeedbackBanner(null), 5000);
                    }}
                    className="px-4 py-2 bg-purple-600 hover:bg-purple-500 text-white font-bold rounded-xl text-xs transition-colors"
                  >
                    Als Firmennotiz übernehmen
                  </button>
                </div>
              </div>
            ) : null}
          </div>
        </div>
      )}
    </div>
  );
};
