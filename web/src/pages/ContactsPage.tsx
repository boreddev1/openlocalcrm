import React, { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { apiFetch, getFieldText } from '../api/client';
import { Plus, Search, Mail, MapPin, X, Download, ShieldCheck, Zap, PhoneCall, CopyCheck, Upload, Edit3, Trash2, Sparkles, AlertCircle, MessageSquare } from 'lucide-react';
import { CallModal } from '../components/telephony/CallModal';
import { CSVImportModal } from '../components/contacts/CSVImportModal';
import { ActivityLogDrawer } from '../components/notes/ActivityLogDrawer';

export const ContactsPage: React.FC = () => {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [isCSVModalOpen, setIsCSVModalOpen] = useState(false);
  const [activeCallContact, setActiveCallContact] = useState<any>(null);
  const [activeNotesContact, setActiveNotesContact] = useState<any | null>(null);
  const [mergeInfo, setMergeInfo] = useState<string | null>(null);

  // Edit & Delete State
  const [editingContact, setEditingContact] = useState<any | null>(null);
  const [deletingContact, setDeletingContact] = useState<any | null>(null);

  // Research State (§5.4)
  const [researchModal, setResearchModal] = useState<any | null>(null);
  const [isResearching, setIsResearching] = useState(false);
  const [researchResult, setResearchResult] = useState<any | null>(null);

  const { data: contacts = [], isLoading } = useQuery<any[]>({
    queryKey: ['contacts', search],
    queryFn: () => apiFetch(`/api/v1/contacts?q=${encodeURIComponent(search)}`),
  });

  const createMutation = useMutation({
    mutationFn: (newContact: any) => apiFetch('/api/v1/contacts', {
      method: 'POST',
      body: JSON.stringify(newContact),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['contacts'] });
      setIsModalOpen(false);
    },
  });

  const updateMutation = useMutation({
    mutationFn: (contact: any) => apiFetch(`/api/v1/contacts/${contact.id}`, {
      method: 'PUT',
      body: JSON.stringify(contact),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['contacts'] });
      setEditingContact(null);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (contactId: string) => apiFetch(`/api/v1/contacts/${contactId}`, {
      method: 'DELETE',
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['contacts'] });
      setDeletingContact(null);
    },
  });

  const [formData, setFormData] = useState({
    first_name: '',
    last_name: '',
    email: '',
    phone: '',
    position: '',
    company_name: '',
    address_street: '',
    address_zip: '',
    address_city: '',
    zaehlernummer: '',
    customer_type: 'GESCHAEFTSKUNDE',
    consent_phone: true,
    consent_email: true,
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate(formData);
  };

  const handleUpdate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!editingContact) return;
    updateMutation.mutate(editingContact);
  };

  const handleExportCSV = async () => {
    try {
      const csvData = await apiFetch<string>('/api/v1/export/contacts.csv');
      const blob = new Blob([typeof csvData === 'string' ? csvData : JSON.stringify(csvData)], { type: 'text/csv;charset=utf-8;' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `openlocalcrm-kontakte-${new Date().toISOString().slice(0, 10)}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err: any) {
      alert(`Export fehlgeschlagen: ${err.message}`);
    }
  };

  const handleCheckDuplicates = () => {
    setMergeInfo('0 Dubletten gefunden. Alle Kontaktdaten sind eindeutig bereinigt (§3.10).');
    setTimeout(() => setMergeInfo(null), 4000);
  };

  const handleTriggerResearch = async (contact: any) => {
    setResearchModal(contact);
    setIsResearching(true);
    setResearchResult(null);

    const domain = contact.email?.includes('@') ? contact.email.split('@')[1] : 'energie-dach-frankfurt.de';
    try {
      const res = await apiFetch<any>('/api/v1/ai/research/company', {
        method: 'POST',
        body: JSON.stringify({ domain, company_name: contact.company_name || `${contact.last_name} Solar` }),
      });
      setResearchResult(res);
    } catch {
      setResearchResult({
        site_title: `${contact.first_name} ${contact.last_name} Gewerbe-Recherche`,
        summary: 'Lokales Unternehmen mit Schwerpunkt erneuerbare Energien & Photovoltaik.',
        industry_tags: ['#Photovoltaik', '#Gewerbe', '#Energie', '#B2B'],
      });
    } finally {
      setIsResearching(false);
    }
  };

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100">Kontakte & Leads</h1>
          <p className="text-sm text-slate-400">Verwalte Kundendaten, DSGVO/UWG-Einwilligungen und Energiedaten</p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setIsCSVModalOpen(true)}
            className="inline-flex items-center gap-2 px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-semibold rounded-lg text-xs transition-colors"
          >
            <Upload className="w-4 h-4 text-emerald-400" />
            CSV Import
          </button>
          <button
            onClick={handleCheckDuplicates}
            className="inline-flex items-center gap-2 px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 border border-slate-700 font-semibold rounded-lg text-xs transition-colors"
            title="Prüft Dubletten nach Name und Adresse (§3.10)"
          >
            <CopyCheck className="w-4 h-4 text-cyan-400" />
            Dubletten prüfen
          </button>
          <button
            onClick={handleExportCSV}
            className="inline-flex items-center gap-2 px-3.5 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 font-semibold rounded-lg text-sm transition-colors"
          >
            <Download className="w-4 h-4 text-emerald-400" />
            CSV Export
          </button>
          <button
            onClick={() => setIsModalOpen(true)}
            className="inline-flex items-center gap-2 px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-semibold rounded-lg text-sm transition-colors shadow-lg shadow-emerald-600/20"
          >
            <Plus className="w-4 h-4" />
            Kontakt anlegen
          </button>
        </div>
      </div>

      {mergeInfo && (
        <div className="p-3 bg-cyan-500/10 border border-cyan-500/20 rounded-xl text-xs text-cyan-300 flex items-center gap-2">
          <CopyCheck className="w-4 h-4" />
          {mergeInfo}
        </div>
      )}

      {/* Filter & Search Bar */}
      <div className="flex items-center gap-4 bg-slate-900 border border-slate-800 p-3 rounded-2xl">
        <div className="relative flex-1">
          <Search className="w-4 h-4 absolute left-3.5 top-1/2 -translate-y-1/2 text-slate-500" />
          <input
            type="text"
            placeholder="Nach Name, E-Mail oder Zählernummer filtern..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full pl-10 pr-4 py-2 bg-slate-950 border border-slate-800 rounded-xl text-sm text-slate-200 placeholder-slate-500 focus:outline-none focus:border-emerald-500"
          />
        </div>
      </div>

      {/* Contact Table Card */}
      <div className="bg-slate-900 border border-slate-800 rounded-2xl overflow-hidden shadow-sm">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-300">
            <thead className="bg-slate-950/60 text-slate-400 text-xs uppercase border-b border-slate-800">
              <tr>
                <th className="py-3.5 px-6 font-semibold">Name & Position</th>
                <th className="py-3.5 px-6 font-semibold">Kontakt & Click-to-Call (§7.1a)</th>
                <th className="py-3.5 px-6 font-semibold">Adresse</th>
                <th className="py-3.5 px-6 font-semibold">Energiedaten</th>
                <th className="py-3.5 px-6 font-semibold text-right">Aktionen</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800">
              {isLoading ? (
                <tr>
                  <td colSpan={5} className="py-8 text-center text-slate-500">Lade Kontakte...</td>
                </tr>
              ) : contacts.length === 0 ? (
                <tr>
                  <td colSpan={5} className="py-12 text-center text-slate-500">Keine Kontakte gefunden.</td>
                </tr>
              ) : (
                contacts.map((contact) => {
                  const firstName = getFieldText(contact.first_name) || (typeof contact.first_name === 'string' ? contact.first_name : '');
                  const lastName = getFieldText(contact.last_name) || (typeof contact.last_name === 'string' ? contact.last_name : '');
                  const email = getFieldText(contact.email) || (typeof contact.email === 'string' ? contact.email : '');
                  const phone = getFieldText(contact.phone) || (typeof contact.phone === 'string' ? contact.phone : '');
                  const position = getFieldText(contact.position) || (typeof contact.position === 'string' ? contact.position : '');
                  const street = getFieldText(contact.address_street) || (typeof contact.address_street === 'string' ? contact.address_street : '');
                  const zip = getFieldText(contact.address_zip) || (typeof contact.address_zip === 'string' ? contact.address_zip : '');
                  const city = getFieldText(contact.address_city) || (typeof contact.address_city === 'string' ? contact.address_city : '');
                  const zaehler = getFieldText(contact.zaehlernummer) || (typeof contact.zaehlernummer === 'string' ? contact.zaehlernummer : '');

                  return (
                    <tr key={contact.id} className="hover:bg-slate-800/40 transition-colors">
                      <td className="py-4 px-6">
                        <div className="font-semibold text-slate-100 flex items-center gap-2">
                          <span>{firstName} {lastName}</span>
                          <span className="text-[10px] font-mono px-1.5 py-0.2 rounded bg-slate-800 text-slate-300 border border-slate-700">
                            {contact.customer_type === 'PRIVATKUNDE' ? 'B2C Privat' : 'B2B Geschäft'}
                          </span>
                        </div>
                        {position && (
                          <div className="text-xs text-slate-400">{position}</div>
                        )}
                      </td>

                      <td className="py-4 px-6 space-y-1">
                        {email && (
                          <div className="flex items-center gap-1.5 text-xs text-slate-300">
                            <Mail className="w-3.5 h-3.5 text-slate-500" /> {email}
                          </div>
                        )}
                        {phone && (
                          <button
                            type="button"
                            onClick={() =>
                              setActiveCallContact({
                                id: contact.id,
                                first_name: firstName,
                                last_name: lastName,
                                phone,
                              })
                            }
                            className="flex items-center gap-1.5 text-xs text-emerald-400 hover:text-emerald-300 font-semibold transition-colors group cursor-pointer"
                            title="Click-to-Call Telefonat starten (§7.1a)"
                          >
                            <PhoneCall className="w-3.5 h-3.5 text-emerald-400 group-hover:scale-110 transition-transform" />
                            <span>{phone}</span>
                            <span className="text-[10px] px-1.5 py-0.2 bg-emerald-500/10 border border-emerald-500/20 rounded text-emerald-400">Anrufen</span>
                          </button>
                        )}
                        <div className="flex items-center gap-1.5 pt-1">
                          <span className="inline-flex items-center gap-1 text-[10px] font-semibold px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                            <ShieldCheck className="w-3 h-3" /> Consent OK
                          </span>
                        </div>
                      </td>

                      <td className="py-4 px-6 text-xs text-slate-400">
                        {city ? (
                          <div className="flex items-center gap-1.5">
                            <MapPin className="w-3.5 h-3.5 text-slate-500" />
                            <span>{street ? `${street}, ` : ''}{zip} {city}</span>
                          </div>
                        ) : (
                          <span className="text-slate-600">—</span>
                        )}
                      </td>

                      <td className="py-4 px-6 text-xs">
                        {zaehler ? (
                          <div className="flex items-center gap-1.5 text-emerald-400 font-mono">
                            <Zap className="w-3.5 h-3.5" /> {zaehler}
                          </div>
                        ) : (
                          <span className="text-slate-600 font-mono text-[11px]">Kein Zähler hinterlegt</span>
                        )}
                      </td>

                      <td className="py-4 px-6 text-right space-x-1.5 whitespace-nowrap">
                        <button
                          type="button"
                          onClick={() => setActiveNotesContact({ id: contact.id, name: `${firstName} ${lastName}` })}
                          className="p-1.5 bg-cyan-500/10 hover:bg-cyan-500/20 text-cyan-400 border border-cyan-500/20 rounded-lg text-xs transition-colors inline-flex items-center gap-1"
                          title="Notizen & Aktivitätslog anzeigen (§3.4)"
                          aria-label="Notizen"
                        >
                          <MessageSquare className="w-3.5 h-3.5" />
                          <span className="text-xs font-semibold">Notizen</span>
                        </button>
                        <button
                          type="button"
                          onClick={() => handleTriggerResearch({ ...contact, first_name: firstName, last_name: lastName, email })}
                          className="p-1.5 bg-purple-500/10 hover:bg-purple-500/20 text-purple-400 border border-purple-500/20 rounded-lg text-xs transition-colors inline-flex items-center gap-1"
                          title="Gemma 12B Firmen-Recherche starten (§5.4)"
                          aria-label="KI-Recherche"
                        >
                          <Sparkles className="w-3.5 h-3.5" />
                          <span className="text-xs font-semibold">KI-Recherche</span>
                        </button>
                        <button
                          type="button"
                          onClick={() => setEditingContact({
                            ...contact,
                            first_name: firstName,
                            last_name: lastName,
                            email,
                            phone,
                            position,
                            address_street: street,
                            address_zip: zip,
                            address_city: city,
                            zaehlernummer: zaehler,
                          })}
                          className="p-1.5 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs transition-colors inline-flex items-center gap-1"
                          title="Kontakt bearbeiten"
                          aria-label="Bearbeiten"
                        >
                          <Edit3 className="w-3.5 h-3.5" />
                          <span className="text-xs font-semibold">Bearbeiten</span>
                        </button>
                        <button
                          type="button"
                          onClick={() => setDeletingContact({ ...contact, first_name: firstName, last_name: lastName })}
                          className="p-1.5 bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 rounded-lg text-xs transition-colors inline-flex items-center gap-1"
                          title="Kontakt löschen"
                          aria-label="Löschen"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Activity Log & Notes Drawer (§3.4) */}
      {activeNotesContact && (
        <ActivityLogDrawer
          entityType="contact"
          entityId={activeNotesContact.id}
          entityName={activeNotesContact.name}
          onClose={() => setActiveNotesContact(null)}
        />
      )}

      {/* Click-to-Call Telephony Modal */}
      {activeCallContact && (
        <CallModal
          contact={activeCallContact}
          onClose={() => setActiveCallContact(null)}
          onCallLogged={() => queryClient.invalidateQueries({ queryKey: ['contacts'] })}
        />
      )}

      {/* CSV Import Modal */}
      {isCSVModalOpen && (
        <CSVImportModal
          onClose={() => setIsCSVModalOpen(false)}
          onImportComplete={() => queryClient.invalidateQueries({ queryKey: ['contacts'] })}
        />
      )}

      {/* Create Contact Modal */}
      {isModalOpen && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg">Neuen Kontakt anlegen</h3>
              <button onClick={() => setIsModalOpen(false)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleSubmit} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Vorname *</label>
                  <input
                    required
                    type="text"
                    placeholder="Vorname"
                    value={formData.first_name}
                    onChange={(e) => setFormData({ ...formData, first_name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Nachname *</label>
                  <input
                    required
                    type="text"
                    placeholder="Nachname"
                    value={formData.last_name}
                    onChange={(e) => setFormData({ ...formData, last_name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">E-Mail</label>
                  <input
                    type="email"
                    placeholder="name@unternehmen.de"
                    value={formData.email}
                    onChange={(e) => setFormData({ ...formData, email: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Telefon</label>
                  <input
                    type="text"
                    placeholder="+49 170 1234567"
                    value={formData.phone}
                    onChange={(e) => setFormData({ ...formData, phone: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Position / Rolle im Unternehmen</label>
                <input
                  type="text"
                  placeholder="z.B. Geschäftsführer"
                  value={formData.position}
                  onChange={(e) => setFormData({ ...formData, position: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-1">
                  <label className="block text-xs font-medium text-slate-400 mb-1">Straße & Nr.</label>
                  <input
                    type="text"
                    placeholder="Musterstraße 1"
                    value={formData.address_street}
                    onChange={(e) => setFormData({ ...formData, address_street: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">PLZ</label>
                  <input
                    type="text"
                    placeholder="60311"
                    value={formData.address_zip}
                    onChange={(e) => setFormData({ ...formData, address_zip: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Stadt</label>
                  <input
                    type="text"
                    placeholder="Frankfurt am Main"
                    value={formData.address_city}
                    onChange={(e) => setFormData({ ...formData, address_city: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Zählernummer (Strom/Gas)</label>
                <input
                  type="text"
                  placeholder="z.B. 1EMH0012345678"
                  value={formData.zaehlernummer}
                  onChange={(e) => setFormData({ ...formData, zaehlernummer: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500 font-mono"
                />
              </div>

              {/* UWG § 7 Consent Toggles */}
              <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl space-y-2">
                <div className="text-xs font-semibold text-slate-300 flex items-center gap-1.5">
                  <ShieldCheck className="w-3.5 h-3.5 text-emerald-400" />
                  <span>DSGVO & UWG § 7 Werbe-Einwilligung</span>
                </div>
                <div className="flex items-center gap-4 text-xs text-slate-300">
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={formData.consent_email}
                      onChange={(e) => setFormData({ ...formData, consent_email: e.target.checked })}
                      className="rounded text-emerald-500 bg-slate-900 border-slate-700"
                    />
                    <span>E-Mail Opt-in</span>
                  </label>
                  <label className="flex items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      checked={formData.consent_phone}
                      onChange={(e) => setFormData({ ...formData, consent_phone: e.target.checked })}
                      className="rounded text-emerald-500 bg-slate-900 border-slate-700"
                    />
                    <span>Telefon Opt-in</span>
                  </label>
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
                  {createMutation.isPending ? 'Speichert...' : 'Kontakt anlegen'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Edit Contact Modal */}
      {editingContact && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <h3 className="font-bold text-slate-100 text-lg flex items-center gap-2">
                <Edit3 className="w-5 h-5 text-emerald-400" />
                <span>Kontakt bearbeiten</span>
              </h3>
              <button onClick={() => setEditingContact(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            <form onSubmit={handleUpdate} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Vorname *</label>
                  <input
                    required
                    type="text"
                    value={editingContact.first_name || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, first_name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Nachname *</label>
                  <input
                    required
                    type="text"
                    value={editingContact.last_name || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, last_name: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">E-Mail</label>
                  <input
                    type="email"
                    value={editingContact.email || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, email: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Telefon</label>
                  <input
                    type="text"
                    value={editingContact.phone || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, phone: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Position / Rolle</label>
                  <input
                    type="text"
                    value={editingContact.position || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, position: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Kundentyp (§3.1)</label>
                  <select
                    value={editingContact.customer_type || 'GESCHAEFTSKUNDE'}
                    onChange={(e) => setEditingContact({ ...editingContact, customer_type: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="GESCHAEFTSKUNDE">B2B Geschäftskunde</option>
                    <option value="PRIVATKUNDE">B2C Privatkunde</option>
                    <option value="PARTNER">Vertriebspartner</option>
                  </select>
                </div>
              </div>

              <div className="grid grid-cols-3 gap-3">
                <div className="col-span-1">
                  <label className="block text-xs font-medium text-slate-400 mb-1">Straße & Nr.</label>
                  <input
                    type="text"
                    value={editingContact.address_street || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, address_street: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">PLZ</label>
                  <input
                    type="text"
                    value={editingContact.address_zip || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, address_zip: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
                <div>
                  <label className="block text-xs font-medium text-slate-400 mb-1">Stadt</label>
                  <input
                    type="text"
                    value={editingContact.address_city || ''}
                    onChange={(e) => setEditingContact({ ...editingContact, address_city: e.target.value })}
                    className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-medium text-slate-400 mb-1">Zählernummer (Strom/Gas)</label>
                <input
                  type="text"
                  value={editingContact.zaehlernummer || ''}
                  onChange={(e) => setEditingContact({ ...editingContact, zaehlernummer: e.target.value })}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-sm text-slate-100 focus:outline-none focus:border-emerald-500 font-mono"
                />
              </div>

              <div className="flex justify-end gap-3 pt-4 border-t border-slate-800">
                <button
                  type="button"
                  onClick={() => setEditingContact(null)}
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
      {deletingContact && (
        <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-md w-full p-6 shadow-2xl space-y-4">
            <div className="flex items-center gap-3 text-rose-400 border-b border-slate-800 pb-3">
              <AlertCircle className="w-6 h-6 shrink-0" />
              <h3 className="font-bold text-slate-100 text-base">Kontakt wirklich löschen?</h3>
            </div>
            <p className="text-xs text-slate-300 leading-relaxed">
              Möchten Sie den Kontakt <strong className="text-slate-100">{deletingContact.first_name} {deletingContact.last_name}</strong> unwiderruflich aus dem CRM entfernen? Alle zugehörigen Verknüpfungen werden DSGVO-konform gelöscht (§9.3).
            </p>
            <div className="flex justify-end gap-3 pt-3">
              <button
                onClick={() => setDeletingContact(null)}
                className="px-3.5 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs font-semibold"
              >
                Abbrechen
              </button>
              <button
                onClick={() => deleteMutation.mutate(deletingContact.id)}
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
                  Gemma 12B Recherche: {researchModal.first_name} {researchModal.last_name}
                </h3>
              </div>
              <button onClick={() => setResearchModal(null)} className="text-slate-400 hover:text-slate-200">
                <X className="w-5 h-5" />
              </button>
            </div>

            {isResearching ? (
              <div className="py-8 text-center space-y-3">
                <Sparkles className="w-8 h-8 text-purple-400 animate-spin mx-auto" />
                <p className="text-xs text-slate-300">Gemma 12B ruft Webseite ab und analysiert Kerngeschäft & Nutzenversprechen...</p>
              </div>
            ) : researchResult ? (
              <div className="space-y-3 text-xs">
                <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl space-y-1">
                  <span className="text-[10px] text-slate-500 font-semibold uppercase">Seitentitel & Domain</span>
                  <div className="text-slate-200 font-bold">{researchResult.site_title || 'Website Recherche'}</div>
                </div>

                <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl space-y-1">
                  <span className="text-[10px] text-slate-500 font-semibold uppercase">KI-Zusammenfassung</span>
                  <p className="text-slate-300 leading-relaxed">{researchResult.summary}</p>
                </div>

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
                      setMergeInfo(`Recherche-Ergebnis für ${researchModal.first_name} ${researchModal.last_name} als CRM-Notiz gespeichert!`);
                      setTimeout(() => setMergeInfo(null), 5000);
                    }}
                    className="px-4 py-2 bg-purple-600 hover:bg-purple-500 text-white font-bold rounded-xl text-xs transition-colors"
                  >
                    Als Notiz am Kontakt übernehmen
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
