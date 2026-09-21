import React, { useState } from 'react';
import { X, Upload, Check, FileSpreadsheet, AlertCircle } from 'lucide-react';
import { apiFetch } from '../../api/client';

interface CSVImportModalProps {
  onClose: () => void;
  onImportComplete: () => void;
}

export const CSVImportModal: React.FC<CSVImportModalProps> = ({ onClose, onImportComplete }) => {
  const [csvContent, setCsvContent] = useState<string>('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [fileName, setFileName] = useState<string | null>(null);
  const [previewRows, setPreviewRows] = useState<any[]>([]);
  const [importStatus, setImportStatus] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isProcessing, setIsProcessing] = useState(false);

  const defaultSampleCSV = `Vorname,Nachname,Email,Telefon,Zaehlernummer,Jahresverbrauch_kWh,Strasse,PLZ,Ort
Dr. Michael,Weber,weber@energie-dach.de,+49 69 12345678,1EMH0012345678,45000,Kaiserstrasse 14,60311,Frankfurt
Sabine,Mustermann,sabine@mustermann.de,+49 171 9876543,1EMH0099887766,4800,Goethestrasse 8,60313,Frankfurt
Thomas,Becker,becker@bau-solar.de,+49 69 55443322,1EMH0033445566,12000,Mainzer Landstrasse 40,60325,Frankfurt`;

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setSelectedFile(file);
      setFileName(file.name);
      setErrorMessage(null);
      const reader = new FileReader();
      reader.onload = (event) => {
        const text = event.target?.result as string;
        setCsvContent(text);
        parseCSVPreview(text);
      };
      reader.readAsText(file);
    }
  };

  const handleUseSample = () => {
    setSelectedFile(null);
    setFileName('beispiel_kunden_leads.csv');
    setCsvContent(defaultSampleCSV);
    setErrorMessage(null);
    parseCSVPreview(defaultSampleCSV);
  };

  const parseCSVPreview = (text: string) => {
    const lines = text.trim().split('\n');
    if (lines.length < 2) return;
    const headers = lines[0].split(',').map((h) => h.trim());
    const rows = lines.slice(1, 4).map((line) => {
      const values = line.split(',').map((v) => v.trim());
      const rowObj: any = {};
      headers.forEach((h, i) => {
        rowObj[h] = values[i] || '';
      });
      return rowObj;
    });
    setPreviewRows(rows);
  };

  const handleExecuteImport = async () => {
    setIsProcessing(true);
    setErrorMessage(null);
    try {
      const blob = selectedFile || new Blob([csvContent], { type: 'text/csv' });
      const formData = new FormData();
      formData.append('file', blob, fileName || 'import.csv');

      const res = await apiFetch<{ status: string; imported_count: number }>(
        '/api/v1/import/contacts',
        {
          method: 'POST',
          body: formData,
        },
      );

      setImportStatus(
        `✅ Import erfolgreich: ${res.imported_count} Kontakt(e) erfolgreich in Datenbank importiert.`,
      );
      setTimeout(() => {
        onImportComplete();
        onClose();
      }, 1500);
    } catch (err: any) {
      setErrorMessage(err.message || 'Fehler beim CSV-Import');
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/60 backdrop-blur-sm flex items-center justify-center p-4">
      <div className="bg-slate-900 border border-slate-800 rounded-2xl max-w-2xl w-full p-6 shadow-2xl space-y-5">
        <div className="flex items-center justify-between border-b border-slate-800 pb-3">
          <div className="flex items-center gap-2">
            <FileSpreadsheet className="w-5 h-5 text-emerald-400" />
            <h3 className="font-bold text-slate-100 text-base">
              CSV Kontakte- & Lead-Import (§6.2)
            </h3>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-200">
            <X className="w-5 h-5" />
          </button>
        </div>

        {errorMessage && (
          <div className="p-4 bg-rose-500/10 border border-rose-500/20 rounded-xl text-xs text-rose-300 flex items-center gap-2">
            <AlertCircle className="w-4 h-4 text-rose-400" />
            {errorMessage}
          </div>
        )}

        {importStatus ? (
          <div className="p-4 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2">
            <Check className="w-4 h-4" />
            {importStatus}
          </div>
        ) : (
          <div className="space-y-4">
            <div className="border-2 border-dashed border-slate-800 hover:border-slate-700 rounded-2xl p-6 text-center space-y-3 bg-slate-950/60">
              <Upload className="w-8 h-8 mx-auto text-slate-500" />
              <div>
                <label className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs cursor-pointer inline-block transition-colors">
                  CSV-Datei auswählen
                  <input type="file" accept=".csv" onChange={handleFileUpload} className="hidden" />
                </label>
                <button
                  type="button"
                  onClick={handleUseSample}
                  className="ml-3 text-xs text-slate-400 hover:text-emerald-400 font-semibold underline"
                >
                  Beispiel-CSV laden
                </button>
              </div>
              {fileName && (
                <div className="text-xs text-emerald-400 font-mono font-semibold">
                  {fileName} geladen
                </div>
              )}
            </div>

            {previewRows.length > 0 && (
              <div className="space-y-2">
                <div className="text-xs font-bold text-slate-300">
                  Vorschau & Spalten-Mapping (erste 3 Datensätze):
                </div>
                <div className="overflow-x-auto border border-slate-800 rounded-xl bg-slate-950">
                  <table className="w-full text-left text-[11px] text-slate-300">
                    <thead className="bg-slate-900 text-slate-400 uppercase border-b border-slate-800">
                      <tr>
                        {Object.keys(previewRows[0]).map((header, idx) => (
                          <th key={idx} className="py-2 px-3 font-mono font-semibold">
                            {header}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-800">
                      {previewRows.map((row, rIdx) => (
                        <tr key={rIdx}>
                          {Object.values(row).map((val: any, cIdx) => (
                            <td key={cIdx} className="py-2 px-3 whitespace-nowrap">
                              {val}
                            </td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>
        )}

        <div className="flex justify-end gap-3 pt-3 border-t border-slate-800">
          <button
            type="button"
            onClick={onClose}
            className="px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-xl text-xs font-semibold"
          >
            Abbrechen
          </button>
          <button
            type="button"
            disabled={!csvContent || isProcessing || !!importStatus}
            onClick={handleExecuteImport}
            className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs disabled:opacity-40 transition-colors shadow-lg shadow-emerald-600/20"
          >
            {isProcessing ? 'Importiere & Dedupliziere...' : 'Import jetzt ausführen'}
          </button>
        </div>
      </div>
    </div>
  );
};
