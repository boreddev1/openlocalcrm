import React, { useState, useEffect } from 'react';
import { Phone, PhoneOff, Clock, CheckCircle2, X, Sparkles } from 'lucide-react';
import { apiFetch } from '../../api/client';

interface CallModalProps {
  contact: {
    id: string;
    first_name: string;
    last_name: string;
    phone?: string;
  };
  onClose: () => void;
  onCallLogged: () => void;
}

export const CallModal: React.FC<CallModalProps> = ({ contact, onClose, onCallLogged }) => {
  const [seconds, setSeconds] = useState(0);
  const [isActive, setIsActive] = useState(true);
  const [disposition, setDisposition] = useState<'REACHED' | 'NO_ANSWER' | 'BUSY' | 'WRONG_NUMBER'>(
    'REACHED',
  );
  const [notes, setNotes] = useState('');
  const [isSaving, setIsSaving] = useState(false);

  useEffect(() => {
    let interval: any = null;
    if (isActive) {
      interval = setInterval(() => {
        setSeconds((prev) => prev + 1);
      }, 1000);
    }
    return () => clearInterval(interval);
  }, [isActive]);

  const formatTimer = (totalSeconds: number) => {
    const mins = Math.floor(totalSeconds / 60);
    const secs = totalSeconds % 60;
    return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSaving(true);
    try {
      await apiFetch('/api/v1/telephony/calls', {
        method: 'POST',
        body: JSON.stringify({
          contact_id: contact.id,
          duration_seconds: seconds,
          disposition,
          notes,
        }),
      });
      onCallLogged();
      onClose();
    } catch {
      onClose();
    } finally {
      setIsSaving(false);
    }
  };

  const quickChips = [
    'Interesse an 10 kWp PV-Anlage',
    'Vor-Ort-Termin vereinbart',
    'Wünscht Rückruf nächste Woche',
    'Aktuell kein Interesse',
  ];

  return (
    <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-lg w-full p-6 shadow-2xl space-y-5">
        {/* Header */}
        <div className="flex items-center justify-between border-b border-slate-800 pb-3">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-400 flex items-center justify-center border border-emerald-500/20">
              <Phone className="w-5 h-5 animate-pulse" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h3 className="font-bold text-slate-100 text-base">
                  Anruf bei {contact.first_name} {contact.last_name}
                </h3>
                <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-medium">
                  Click-to-Call Telefonie (§7.1a)
                </span>
              </div>
              <div className="text-xs text-slate-400 font-mono">
                {contact.phone || 'Keine Nummer'}
              </div>
            </div>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-200">
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Timer Bar */}
        <div className="flex items-center justify-between p-4 bg-slate-950 rounded-xl border border-slate-800">
          <div className="flex items-center gap-2">
            <Clock className="w-4 h-4 text-emerald-400" />
            <span className="text-xs text-slate-400">Gesprächsdauer:</span>
            <span className="text-base font-bold font-mono text-slate-100">
              {formatTimer(seconds)}
            </span>
          </div>

          <button
            type="button"
            onClick={() => setIsActive(!isActive)}
            className={`px-3 py-1.5 rounded-lg text-xs font-semibold flex items-center gap-1.5 transition-colors ${
              isActive
                ? 'bg-rose-500/10 text-rose-400 border border-rose-500/20 hover:bg-rose-500/20'
                : 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 hover:bg-emerald-500/20'
            }`}
          >
            {isActive ? <PhoneOff className="w-3.5 h-3.5" /> : <Phone className="w-3.5 h-3.5" />}
            {isActive ? 'Auflegen' : 'Fortsetzen'}
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSave} className="space-y-4">
          <div>
            <label className="block text-xs font-semibold text-slate-300 mb-1.5">
              Gesprächs-Ergebnis
            </label>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs">
              {[
                { key: 'REACHED', label: 'Erreicht' },
                { key: 'NO_ANSWER', label: 'Nicht erreicht' },
                { key: 'BUSY', label: 'Besetzt' },
                { key: 'WRONG_NUMBER', label: 'Falsche Nr.' },
              ].map((d) => (
                <button
                  type="button"
                  key={d.key}
                  onClick={() => setDisposition(d.key as any)}
                  className={`py-2 px-2.5 rounded-xl border font-semibold text-center transition-all ${
                    disposition === d.key
                      ? 'bg-emerald-600 text-slate-950 border-emerald-500 shadow-md shadow-emerald-600/20'
                      : 'bg-slate-950 text-slate-400 border-slate-800 hover:border-slate-700'
                  }`}
                >
                  {d.label}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-xs font-semibold text-slate-300 mb-1">
              Gesprächsnotiz / Zusammenfassung
            </label>
            <textarea
              rows={3}
              placeholder="Notizen zum Telefonat festhalten..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="w-full p-3 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 placeholder-slate-500 focus:outline-none focus:border-emerald-500 leading-relaxed"
            />
          </div>

          {/* Quick Note Chips */}
          <div className="space-y-1.5">
            <div className="text-[11px] text-slate-500 font-semibold flex items-center gap-1">
              <Sparkles className="w-3 h-3 text-emerald-400" /> Schnellbausteine:
            </div>
            <div className="flex flex-wrap gap-1.5">
              {quickChips.map((chip, idx) => (
                <button
                  type="button"
                  key={idx}
                  onClick={() => setNotes((prev) => (prev ? `${prev}, ${chip}` : chip))}
                  className="px-2.5 py-1 rounded-lg bg-slate-950 hover:bg-slate-800 text-[11px] text-slate-400 border border-slate-800 transition-colors"
                >
                  + {chip}
                </button>
              ))}
            </div>
          </div>

          <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-xs font-medium transition-colors"
            >
              Abbrechen
            </button>
            <button
              type="submit"
              disabled={isSaving}
              className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-lg text-xs transition-colors flex items-center gap-1.5"
            >
              <CheckCircle2 className="w-4 h-4" />
              {isSaving ? 'Speichert...' : 'Anruf protokollieren'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
