import React, { useState, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { apiFetch } from '../../api/client';
import {
  X,
  Sparkles,
  Zap,
  Calculator,
  ShieldCheck,
  CheckCircle2,
  FileText,
  Sun,
  Battery,
  ThumbsUp,
  Star,
  Wrench,
  Percent,
} from 'lucide-react';

interface OfferCalculatorModalProps {
  initialCustomerName?: string;
  onClose: () => void;
}

export const OfferCalculatorModal: React.FC<OfferCalculatorModalProps> = ({
  initialCustomerName = 'Familie Müller',
  onClose,
}) => {
  const queryClient = useQueryClient();
  const [activeTab, setActiveTab] = useState<'consumption' | 'hardware' | 'pricing'>('consumption');

  // Extraction & Customer State
  const [isExtracting, setIsExtracting] = useState(false);
  const [customerName, setCustomerName] = useState(initialCustomerName);
  const [meterNumber, setMeterNumber] = useState('');
  const [consumption, setConsumption] = useState(6500); // kWh/a
  const [pricePerKwh, setPricePerKwh] = useState(0.385); // €/kWh
  const [roofArea, setRoofArea] = useState(75);
  const [roofOrientation, setRoofOrientation] = useState('Süd (35° Dachneigung)');

  // Hardware & Engineering Adjustments
  const [kwp, setKwp] = useState(14.5);
  const [storageKwh, setStorageKwh] = useState(12.0);
  const [moduleType, setModuleType] = useState('440W Glas-Glas Biaxial');
  const [moduleCount, setModuleCount] = useState(33);
  const [inverterBrand, setInverterBrand] = useState('Fronius Symo GEN24 + BYD HVS');

  // Economic Adjustments (Sales Rep Overrides)
  const [feedInRate, setFeedInRate] = useState(0.081); // EEG 8.1 ct/kWh
  const [electricityInflation, setElectricityInflation] = useState(2.5); // % p.a.
  const [manualDiscountEuro, setManualDiscountEuro] = useState(0);
  const [additionalServicesEuro, setAdditionalServicesEuro] = useState(0);
  const [additionalServicesNotes, setAdditionalServicesNotes] = useState(
    'Inkl. Gerüst & Zählerschrank-Anschluss',
  );
  const [customPriceOverride, setCustomPriceOverride] = useState<number | null>(null);

  // Calculation State
  const [calcResult, setCalcResult] = useState<any | null>(null);
  const [userRating, setUserRating] = useState<number>(5);
  const [userComment, setUserComment] = useState('');
  const [isApproved, setIsApproved] = useState(true);
  const [feedbackBanner, setFeedbackBanner] = useState<string | null>(null);

  // Synchronize Module Count when kWp changes
  const handleKwpChange = (val: number) => {
    setKwp(val);
    setModuleCount(Math.round((val * 1000) / 440));
  };

  const handleModuleCountChange = (count: number) => {
    setModuleCount(count);
    const newKwp = Number(((count * 440) / 1000).toFixed(2));
    setKwp(newKwp);
  };

  const calculateSolar = async () => {
    try {
      const res = await apiFetch<any>('/api/v1/calculator/solar', {
        method: 'POST',
        body: JSON.stringify({
          kwp,
          consumption,
          pricePerKwh,
          storageKwh,
        }),
      });

      // Apply manual adjustments (Discount, Additional services, Price Override)
      const baseSystemCost = kwp * 1050 + storageKwh * 650;
      const finalPrice =
        customPriceOverride !== null
          ? customPriceOverride
          : baseSystemCost + additionalServicesEuro - manualDiscountEuro;

      const directSavings = res.directSavingsEuro || res.yearlySavingsEuro * 0.8;
      const feedInIncome = (res.yearlyGenerationKwh - consumption * 0.68) * feedInRate;
      const totalYearlySavings = Math.round(directSavings + Math.max(0, feedInIncome));
      const paybackPeriodYears = Number((finalPrice / Math.max(1, totalYearlySavings)).toFixed(1));
      const twentyYearSavings = Math.round(totalYearlySavings * 20 - finalPrice);

      setCalcResult({
        ...res,
        systemCostGrossEuro: Math.round(finalPrice),
        systemCostNetEuro: Math.round(finalPrice),
        yearlySavingsEuro: totalYearlySavings,
        paybackPeriodYears,
        twentyYearSavingsEuro: twentyYearSavings,
      });
    } catch {
      // Fallback
    }
  };

  useEffect(() => {
    calculateSolar();
  }, [
    kwp,
    consumption,
    pricePerKwh,
    storageKwh,
    feedInRate,
    electricityInflation,
    manualDiscountEuro,
    additionalServicesEuro,
    customPriceOverride,
  ]);

  const handleUploadDemoBill = async () => {
    setIsExtracting(true);
    try {
      const res = await apiFetch<any>('/api/v1/ai/parse-bill', {
        method: 'POST',
        body: JSON.stringify({ customer_name: customerName }),
      });
      if (res.extracted) {
        setCustomerName(res.extracted.customer_name);
        setConsumption(res.extracted.yearly_consumption);
        setPricePerKwh(res.extracted.current_electricity_price);
        setMeterNumber(res.extracted.meter_number);
        setRoofArea(res.extracted.roof_area_sqm);
        setRoofOrientation(res.extracted.roof_orientation);
        handleKwpChange(res.extracted.recommended_kwp);
        setStorageKwh(res.extracted.recommended_storage_kwh);
        setFeedbackBanner('Gemma 12B OCR: Stromrechnung & Zählerdaten erfolgreich eingelesen!');
        setTimeout(() => setFeedbackBanner(null), 4000);
      }
    } finally {
      setIsExtracting(false);
    }
  };

  const handleCreateDealFromCalc = async () => {
    if (!calcResult) return;

    await apiFetch('/api/v1/deals', {
      method: 'POST',
      body: JSON.stringify({
        title: `PV-Angebot ${kwp} kWp + ${storageKwh} kWh Speicher (${customerName})`,
        value: calcResult.systemCostGrossEuro.toString(),
        currency: 'EUR',
        stage: 'OFFER_SENT',
        probability: 75,
        contact_name: customerName,
        notes: `Deterministische Berechnung vom Vertriebler manuell angepasst & freigegeben (${userRating}/5 Sterne).\nHardware: ${moduleCount}x ${moduleType} an ${inverterBrand}.\nRabatt: ${manualDiscountEuro} €, Zusatzleistungen: ${additionalServicesEuro} € (${additionalServicesNotes}).\nAutarkie: ${calcResult.autarkyRatePercent}%, Ersparnis: ${calcResult.yearlySavingsEuro} €/a, Amortisation: ${calcResult.paybackPeriodYears} Jahre.\nVertriebsnotiz: ${userComment || 'Geprüft und kaufmännisch freigegeben.'}`,
      }),
    });

    queryClient.invalidateQueries({ queryKey: ['deals'] });
    setFeedbackBanner(
      'Angebot erfolgreich mit individuellen Anpassungen als Deal in der Pipeline angelegt!',
    );
    setTimeout(() => {
      onClose();
    }, 1500);
  };

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-md flex items-center justify-center p-4">
      <div className="bg-slate-900 border border-slate-800 rounded-3xl max-w-5xl w-full max-h-[92vh] flex flex-col shadow-2xl overflow-hidden">
        {/* Header */}
        <div className="p-6 border-b border-slate-800 flex items-center justify-between bg-slate-950/60 shrink-0">
          <div className="flex items-center gap-3">
            <div className="p-2.5 bg-emerald-500/10 border border-emerald-500/20 rounded-2xl">
              <Calculator className="w-6 h-6 text-emerald-400" />
            </div>
            <div>
              <h2 className="text-lg font-bold text-slate-100 flex items-center gap-2">
                KI-Dokumentenextraktion & Manuell anpassbarer Angebotsrechner (§5.2 / §5.6 / §6.3)
              </h2>
              <p className="text-xs text-slate-400">
                OCR-Extraktion mit Gemma 12B ➔ Deterministischer Rechner ➔ Volle manuelle
                Anpassungskontrolle für Vertriebler
              </p>
            </div>
          </div>
          <button
            onClick={onClose}
            className="p-2 text-slate-400 hover:text-slate-200 rounded-xl hover:bg-slate-800 transition-colors"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {feedbackBanner && (
          <div className="p-3.5 bg-emerald-500/10 border-b border-emerald-500/20 text-xs text-emerald-300 flex items-center gap-2 shrink-0">
            <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />
            <span>{feedbackBanner}</span>
          </div>
        )}

        {/* Modal Body: Split View */}
        <div className="flex-1 overflow-y-auto p-6 grid grid-cols-1 lg:grid-cols-12 gap-6">
          {/* Left Column: Editable Parameters & Tab Controls (6 cols) */}
          <div className="lg:col-span-6 space-y-4">
            {/* OCR Document Upload Banner */}
            <div className="p-3.5 bg-slate-950 border border-slate-800 rounded-2xl flex items-center justify-between gap-3">
              <div className="flex items-center gap-2 text-xs text-slate-300">
                <FileText className="w-4 h-4 text-purple-400 shrink-0" />
                <span>Stromrechnung / Lastgang-OCR:</span>
              </div>
              <button
                type="button"
                onClick={handleUploadDemoBill}
                disabled={isExtracting}
                className="px-3 py-1.5 bg-purple-600 hover:bg-purple-500 text-white font-bold rounded-xl text-xs transition-colors inline-flex items-center gap-1.5 shadow-lg shadow-purple-600/20 cursor-pointer"
              >
                <Sparkles className="w-3.5 h-3.5" />
                <span>{isExtracting ? 'Analysiert...' : '📄 Demo-Rechnung einlesen'}</span>
              </button>
            </div>

            {/* Adjustment Tabs */}
            <div className="flex items-center gap-1.5 bg-slate-950 p-1.5 rounded-2xl border border-slate-800">
              <button
                type="button"
                onClick={() => setActiveTab('consumption')}
                className={`flex-1 py-1.5 px-2 rounded-xl text-xs font-semibold transition-colors flex items-center justify-center gap-1.5 ${activeTab === 'consumption' ? 'bg-emerald-600 text-slate-950' : 'text-slate-400 hover:text-slate-200'}`}
              >
                <Zap className="w-3.5 h-3.5" />
                <span>1. Verbrauch & Dach</span>
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('hardware')}
                className={`flex-1 py-1.5 px-2 rounded-xl text-xs font-semibold transition-colors flex items-center justify-center gap-1.5 ${activeTab === 'hardware' ? 'bg-emerald-600 text-slate-950' : 'text-slate-400 hover:text-slate-200'}`}
              >
                <Wrench className="w-3.5 h-3.5" />
                <span>2. Hardware</span>
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('pricing')}
                className={`flex-1 py-1.5 px-2 rounded-xl text-xs font-semibold transition-colors flex items-center justify-center gap-1.5 ${activeTab === 'pricing' ? 'bg-emerald-600 text-slate-950' : 'text-slate-400 hover:text-slate-200'}`}
              >
                <Percent className="w-3.5 h-3.5" />
                <span>3. Preis & Rabatt</span>
              </button>
            </div>

            {/* TAB 1: Consumption & Dach */}
            {activeTab === 'consumption' && (
              <div className="p-4 bg-slate-950 border border-slate-800 rounded-2xl space-y-3.5">
                <span className="text-xs font-bold text-slate-200 flex items-center gap-1.5 border-b border-slate-800 pb-2">
                  <Zap className="w-4 h-4 text-amber-400" />
                  Kunden- & Verbrauchsdaten (manuell anpassbar)
                </span>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Kundenname / Objekt
                    </label>
                    <input
                      type="text"
                      value={customerName}
                      onChange={(e) => setCustomerName(e.target.value)}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Zählernummer
                    </label>
                    <input
                      type="text"
                      placeholder="1EMH..."
                      value={meterNumber}
                      onChange={(e) => setMeterNumber(e.target.value)}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 font-mono focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Jahresstromverbrauch (kWh/a)
                    </label>
                    <input
                      type="number"
                      value={consumption}
                      onChange={(e) => setConsumption(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Aktueller Strompreis (€/kWh)
                    </label>
                    <input
                      type="number"
                      step="0.01"
                      value={pricePerKwh}
                      onChange={(e) => setPricePerKwh(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Nutzbare Dachfläche (m²)
                    </label>
                    <input
                      type="number"
                      value={roofArea}
                      onChange={(e) => setRoofArea(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Dachausrichtung / Neigung
                    </label>
                    <input
                      type="text"
                      value={roofOrientation}
                      onChange={(e) => setRoofOrientation(e.target.value)}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                </div>
              </div>
            )}

            {/* TAB 2: Hardware & Komponenten */}
            {activeTab === 'hardware' && (
              <div className="p-4 bg-slate-950 border border-slate-800 rounded-2xl space-y-3.5">
                <span className="text-xs font-bold text-slate-200 flex items-center gap-1.5 border-b border-slate-800 pb-2">
                  <Wrench className="w-4 h-4 text-emerald-400" />
                  Hardware-Konfiguration & Leistungsdimensionierung
                </span>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      PV-Modultyp
                    </label>
                    <select
                      value={moduleType}
                      onChange={(e) => setModuleType(e.target.value)}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    >
                      <option value="440W Glas-Glas Biaxial">440W Glas-Glas Biaxial</option>
                      <option value="420W Full-Black">420W Full-Black Premium</option>
                      <option value="450W TOPCon N-Type">450W TOPCon N-Type</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Anzahl Solarmodule
                    </label>
                    <input
                      type="number"
                      value={moduleCount}
                      onChange={(e) => handleModuleCountChange(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 font-bold text-amber-400 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                    Wechselrichter & Hybrid-System
                  </label>
                  <select
                    value={inverterBrand}
                    onChange={(e) => setInverterBrand(e.target.value)}
                    className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  >
                    <option value="Fronius Symo GEN24 + BYD HVS">
                      Fronius Symo GEN24 + BYD HVS
                    </option>
                    <option value="SMA Sunny Tripower + SMA Home Storage">
                      SMA Sunny Tripower + SMA Home Storage
                    </option>
                    <option value="Huawei SUN2000 + LUNA2000">Huawei SUN2000 + LUNA2000</option>
                    <option value="SolarEdge Home Hub + Energy Bank">
                      SolarEdge Home Hub + Energy Bank
                    </option>
                  </select>
                </div>

                {/* Fine-Tuning Sliders */}
                <div className="space-y-1 pt-1">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-slate-300 font-medium flex items-center gap-1">
                      <Sun className="w-3.5 h-3.5 text-amber-400" /> PV-Gesamtleistung:
                    </span>
                    <span className="font-bold text-amber-400">
                      {kwp} kWp ({moduleCount} Module)
                    </span>
                  </div>
                  <input
                    type="range"
                    min="4"
                    max="30"
                    step="0.44"
                    value={kwp}
                    onChange={(e) => handleKwpChange(Number(e.target.value))}
                    className="w-full accent-amber-500 h-1.5 bg-slate-800 rounded-lg cursor-pointer"
                  />
                </div>

                <div className="space-y-1 pt-1">
                  <div className="flex items-center justify-between text-xs">
                    <span className="text-slate-300 font-medium flex items-center gap-1">
                      <Battery className="w-3.5 h-3.5 text-emerald-400" />{' '}
                      Batteriespeicherkapazität:
                    </span>
                    <span className="font-bold text-emerald-400">{storageKwh} kWh</span>
                  </div>
                  <input
                    type="range"
                    min="0"
                    max="25"
                    step="1"
                    value={storageKwh}
                    onChange={(e) => setStorageKwh(Number(e.target.value))}
                    className="w-full accent-emerald-500 h-1.5 bg-slate-800 rounded-lg cursor-pointer"
                  />
                </div>
              </div>
            )}

            {/* TAB 3: Preisanpassung & Vertriebler-Rabatte */}
            {activeTab === 'pricing' && (
              <div className="p-4 bg-slate-950 border border-slate-800 rounded-2xl space-y-3.5">
                <span className="text-xs font-bold text-slate-200 flex items-center gap-1.5 border-b border-slate-800 pb-2">
                  <Percent className="w-4 h-4 text-purple-400" />
                  Kaufmännische Preisanpassung, Rabatte & Sonderpositionen
                </span>

                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Manueller Rabatt (€)
                    </label>
                    <input
                      type="number"
                      placeholder="0"
                      value={manualDiscountEuro || ''}
                      onChange={(e) => setManualDiscountEuro(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-rose-400 font-bold focus:outline-none focus:border-rose-500"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Zusatzleistungen (€)
                    </label>
                    <input
                      type="number"
                      placeholder="0"
                      value={additionalServicesEuro || ''}
                      onChange={(e) => setAdditionalServicesEuro(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 font-bold focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                    Beschreibung der Zusatzleistungen
                  </label>
                  <input
                    type="text"
                    value={additionalServicesNotes}
                    onChange={(e) => setAdditionalServicesNotes(e.target.value)}
                    className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                  />
                </div>

                <div className="grid grid-cols-2 gap-3 pt-1">
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      EEG-Einspeisevergütung (€/kWh)
                    </label>
                    <input
                      type="number"
                      step="0.001"
                      value={feedInRate}
                      onChange={(e) => setFeedInRate(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                  <div>
                    <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                      Erwartete Strompreissteigerung (% p.a.)
                    </label>
                    <input
                      type="number"
                      step="0.1"
                      value={electricityInflation}
                      onChange={(e) => setElectricityInflation(Number(e.target.value))}
                      className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                    />
                  </div>
                </div>

                <div>
                  <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                    Festpreis-Override (Optional: Überschreibt Gesamtsumme)
                  </label>
                  <input
                    type="number"
                    placeholder="Leer lassen für automatische Kalkulation..."
                    value={customPriceOverride !== null ? customPriceOverride : ''}
                    onChange={(e) =>
                      setCustomPriceOverride(e.target.value ? Number(e.target.value) : null)
                    }
                    className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-emerald-400 font-bold focus:outline-none focus:border-emerald-500"
                  />
                </div>
              </div>
            )}
          </div>

          {/* Right Column: Deterministic Calculations & Human Evaluation (6 cols) */}
          <div className="lg:col-span-6 space-y-4">
            {/* Calculation Output Cards */}
            {calcResult && (
              <div className="p-5 bg-slate-950 border border-slate-800 rounded-2xl space-y-4">
                <div className="flex items-center justify-between border-b border-slate-800 pb-3">
                  <div className="flex items-center gap-2">
                    <ShieldCheck className="w-5 h-5 text-emerald-400" />
                    <span className="text-xs font-bold text-slate-100 uppercase tracking-wider">
                      Deterministische Ertrags- & Wirtschaftlichkeitskalkulation
                    </span>
                  </div>
                  <span className="text-[10px] px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 font-bold">
                    Zero Hallucination
                  </span>
                </div>

                {/* Key Metrics Grid */}
                <div className="grid grid-cols-2 sm:grid-cols-3 gap-3">
                  <div className="p-3 bg-slate-900/90 border border-slate-800 rounded-xl">
                    <div className="text-[11px] text-slate-400 font-medium">Autarkiegrad</div>
                    <div className="text-xl font-bold text-emerald-400 mt-0.5">
                      {calcResult.autarkyRatePercent}%
                    </div>
                  </div>
                  <div className="p-3 bg-slate-900/90 border border-slate-800 rounded-xl">
                    <div className="text-[11px] text-slate-400 font-medium">Eigenverbrauch</div>
                    <div className="text-xl font-bold text-blue-400 mt-0.5">
                      {calcResult.selfConsumptionRatePercent}%
                    </div>
                  </div>
                  <div className="p-3 bg-slate-900/90 border border-slate-800 rounded-xl">
                    <div className="text-[11px] text-slate-400 font-medium">Jahresertrag</div>
                    <div className="text-xl font-bold text-amber-400 mt-0.5">
                      {calcResult.yearlyGenerationKwh.toLocaleString('de-DE')} kWh/a
                    </div>
                  </div>
                  <div className="p-3 bg-slate-900/90 border border-slate-800 rounded-xl">
                    <div className="text-[11px] text-slate-400 font-medium">Ersparnis pro Jahr</div>
                    <div className="text-xl font-bold text-emerald-400 mt-0.5">
                      {calcResult.yearlySavingsEuro.toLocaleString('de-DE')} €/a
                    </div>
                  </div>
                  <div className="p-3 bg-slate-900/90 border border-slate-800 rounded-xl">
                    <div className="text-[11px] text-slate-400 font-medium">Amortisationszeit</div>
                    <div className="text-xl font-bold text-purple-400 mt-0.5">
                      {calcResult.paybackPeriodYears} Jahre
                    </div>
                  </div>
                  <div className="p-3 bg-slate-900/90 border border-slate-800 rounded-xl">
                    <div className="text-[11px] text-slate-400 font-medium">20-Jahre Ersparnis</div>
                    <div className="text-xl font-bold text-emerald-400 mt-0.5">
                      {calcResult.twentyYearSavingsEuro.toLocaleString('de-DE')} €
                    </div>
                  </div>
                </div>

                {/* Offer Pricing Summary */}
                <div className="p-4 bg-emerald-950/20 border border-emerald-500/30 rounded-xl flex items-center justify-between">
                  <div>
                    <span className="text-xs text-slate-300 font-semibold block">
                      Kalkulierter Endpreis für Kunden:
                    </span>
                    <span className="text-[11px] text-emerald-400">
                      Gemäß § 12 Abs. 3 UStG (0% MwSt){' '}
                      {manualDiscountEuro > 0 && `· Inkl. ${manualDiscountEuro} € Rabatt`}
                    </span>
                  </div>
                  <div className="text-2xl font-bold text-emerald-400">
                    {calcResult.systemCostGrossEuro.toLocaleString('de-DE')} €
                  </div>
                </div>
              </div>
            )}

            {/* Human-in-the-Loop Evaluation & Approval (§6.3) */}
            <div className="p-4 bg-slate-950 border border-purple-500/30 rounded-2xl space-y-3">
              <div className="flex items-center justify-between border-b border-slate-800 pb-2">
                <span className="text-xs font-bold text-purple-300 flex items-center gap-1.5">
                  <ThumbsUp className="w-4 h-4 text-purple-400" />
                  3. Human-in-the-Loop Bewertung & Freigabe durch Vertriebler (§6.3)
                </span>
                <div className="flex items-center gap-1">
                  {[1, 2, 3, 4, 5].map((star) => (
                    <button
                      key={star}
                      type="button"
                      onClick={() => setUserRating(star)}
                      className={`p-0.5 ${star <= userRating ? 'text-amber-400' : 'text-slate-700'} hover:text-amber-300 transition-colors cursor-pointer`}
                    >
                      <Star className="w-4 h-4 fill-current" />
                    </button>
                  ))}
                </div>
              </div>

              <div>
                <label className="block text-[11px] font-semibold text-slate-400 mb-1">
                  Freigabekommentar / Vertriebsnotiz:
                </label>
                <input
                  type="text"
                  placeholder="z.B. Dachstatik geprüft, 33 Module passen perfekt, Rabatt mit Verkaufsleiter abgestimmt"
                  value={userComment}
                  onChange={(e) => setUserComment(e.target.value)}
                  className="w-full px-3 py-1.5 bg-slate-900 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-purple-500"
                />
              </div>

              <div className="flex items-center justify-between pt-2">
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="approvalCheckbox"
                    checked={isApproved}
                    onChange={(e) => setIsApproved(e.target.checked)}
                    className="w-4 h-4 accent-emerald-500 rounded cursor-pointer"
                  />
                  <label
                    htmlFor="approvalCheckbox"
                    className="text-xs text-slate-300 font-medium cursor-pointer"
                  >
                    Ergebnis als plausibel & fachlich korrekt freigegeben
                  </label>
                </div>

                <button
                  type="button"
                  disabled={!isApproved}
                  onClick={handleCreateDealFromCalc}
                  className="px-4 py-2.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-slate-950 font-bold rounded-xl text-xs transition-colors shadow-lg shadow-emerald-600/20 inline-flex items-center gap-2 cursor-pointer"
                >
                  <CheckCircle2 className="w-4 h-4" />
                  <span>Berechnung freigeben & Deal anlegen</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
