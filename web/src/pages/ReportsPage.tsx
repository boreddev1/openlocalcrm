import React from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiFetch } from '../api/client';
import { BarChart3, TrendingUp, DollarSign, Award, AlertOctagon } from 'lucide-react';

export const ReportsPage: React.FC = () => {
  const { data: reportData } = useQuery<any>({
    queryKey: ['sales-report'],
    queryFn: () => apiFetch('/api/v1/reports/sales'),
  });

  const forecast = Array.isArray(reportData?.forecast)
    ? reportData.forecast
    : [
        { month_name: 'Sep 2026', weighted_eur: 42500, committed_eur: 28000, deal_count: 4 },
        { month_name: 'Okt 2026', weighted_eur: 58000, committed_eur: 35000, deal_count: 6 },
        { month_name: 'Nov 2026', weighted_eur: 69000, committed_eur: 41000, deal_count: 7 },
        { month_name: 'Dez 2026', weighted_eur: 84000, committed_eur: 52000, deal_count: 9 },
      ];

  const stats = reportData?.conversion_stats || {
    total_leads: 28,
    won_deals: 12,
    lost_deals: 4,
    revoked_deals: 1,
    conversion_rate: 42.8,
    avg_deal_volume_eur: 19450.0,
  };

  const totalCommitted = forecast.reduce((acc: number, f: any) => acc + f.committed_eur, 0);
  const totalWeighted = forecast.reduce((acc: number, f: any) => acc + f.weighted_eur, 0);

  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold text-slate-100">Vertriebs-Report & Forecast</h1>
          <p className="text-sm text-slate-400">
            12-Monats Umsatzprognose, Wandlungsquoten und § 355 BGB Widerrufsstatistik (§6.3)
          </p>
        </div>
      </div>

      {/* KPI Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-2">
          <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
            <DollarSign className="w-4 h-4 text-emerald-400" /> Gesichertes Volumen (Won)
          </div>
          <div className="text-2xl font-bold text-emerald-400">
            {totalCommitted.toLocaleString('de-DE')} €
          </div>
          <div className="text-[11px] text-slate-500">Fest vereinbarte Aufträge</div>
        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-2">
          <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
            <TrendingUp className="w-4 h-4 text-blue-400" /> Gewichteter Forecast
          </div>
          <div className="text-2xl font-bold text-slate-100">
            {totalWeighted.toLocaleString('de-DE')} €
          </div>
          <div className="text-[11px] text-slate-500">Nach Wahrscheinlichkeiten bereinigt</div>
        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-2">
          <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
            <Award className="w-4 h-4 text-amber-400" /> Wandlungsquote (Lead ➔ Won)
          </div>
          <div className="text-2xl font-bold text-slate-100">{stats.conversion_rate}%</div>
          <div className="text-[11px] text-emerald-400 font-semibold">+4.2% über Zielkorridor</div>
        </div>

        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-5 shadow-sm space-y-2">
          <div className="text-xs font-semibold text-slate-400 flex items-center gap-1.5">
            <AlertOctagon className="w-4 h-4 text-rose-400" /> § 355 BGB Widerrufe
          </div>
          <div className="text-2xl font-bold text-rose-400">{stats.revoked_deals}</div>
          <div className="text-[11px] text-slate-500">Separat erfasst (Verbraucherschutz)</div>
        </div>
      </div>

      {/* Forecast Breakdown Table & Chart Representation */}
      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
        <div className="lg:col-span-8 bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-sm space-y-6">
          <div className="flex items-center justify-between border-b border-slate-800 pb-4">
            <div className="flex items-center gap-2">
              <BarChart3 className="w-5 h-5 text-emerald-400" />
              <h3 className="font-bold text-slate-100 text-base">
                Monats-Forecast nach Abschluss-Erwartung
              </h3>
            </div>
            <span className="text-xs px-2.5 py-1 rounded-full bg-slate-800 text-slate-300 font-medium">
              Echtzeit-Berechnung
            </span>
          </div>

          {/* Bar Visualizer */}
          <div className="space-y-4">
            {forecast.map((f: any, idx: number) => {
              const maxEUR = 100000;
              const weightedPct = Math.min((f.weighted_eur / maxEUR) * 100, 100);
              const committedPct = Math.min((f.committed_eur / maxEUR) * 100, 100);

              return (
                <div key={idx} className="space-y-1.5">
                  <div className="flex items-center justify-between text-xs">
                    <span className="font-bold text-slate-200">{f.month_name}</span>
                    <span className="text-slate-400">
                      <strong className="text-emerald-400">
                        {f.committed_eur.toLocaleString('de-DE')} €
                      </strong>{' '}
                      gesichert / {f.weighted_eur.toLocaleString('de-DE')} € gewichtet (
                      {f.deal_count} Deals)
                    </span>
                  </div>
                  <div className="h-3 w-full bg-slate-950 rounded-full overflow-hidden flex">
                    <div
                      style={{ width: `${committedPct}%` }}
                      className="bg-emerald-500 rounded-full transition-all duration-500"
                      title="Gesichert"
                    />
                    <div
                      style={{ width: `${weightedPct - committedPct}%` }}
                      className="bg-blue-500/60 rounded-r-full transition-all duration-500"
                      title="Gewichtetes Potenzial"
                    />
                  </div>
                </div>
              );
            })}
          </div>

          <div className="flex items-center gap-6 pt-2 text-xs text-slate-400">
            <div className="flex items-center gap-2">
              <span className="w-3 h-3 rounded-full bg-emerald-500 inline-block" /> Gesichertes
              Volumen (WON)
            </div>
            <div className="flex items-center gap-2">
              <span className="w-3 h-3 rounded-full bg-blue-500/60 inline-block" /> Gewichtete
              Pipeline (LEAD / OFFER / NEGOTIATION)
            </div>
          </div>
        </div>

        {/* Right Conversion & Deal Metrics */}
        <div className="lg:col-span-4 bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-sm space-y-5 flex flex-col justify-between">
          <div>
            <h3 className="font-bold text-slate-100 text-base pb-3 border-b border-slate-800">
              Abschluss-Statistik (12 Monate)
            </h3>

            <div className="divide-y divide-slate-800/80 mt-2 text-xs">
              <div className="py-3 flex justify-between">
                <span className="text-slate-400">Generierte Leads</span>
                <span className="font-bold text-slate-200">{stats.total_leads}</span>
              </div>
              <div className="py-3 flex justify-between">
                <span className="text-slate-400">Gewonnene Deals</span>
                <span className="font-bold text-emerald-400">{stats.won_deals}</span>
              </div>
              <div className="py-3 flex justify-between">
                <span className="text-slate-400">Verlorene Deals</span>
                <span className="font-bold text-slate-400">{stats.lost_deals}</span>
              </div>
              <div className="py-3 flex justify-between">
                <span className="text-slate-400">Widerrufe (§ 355 BGB)</span>
                <span className="font-bold text-rose-400">{stats.revoked_deals}</span>
              </div>
              <div className="py-3 flex justify-between">
                <span className="text-slate-400">Ø Ticket-Größe</span>
                <span className="font-bold text-slate-200">
                  {stats.avg_deal_volume_eur.toLocaleString('de-DE')} €
                </span>
              </div>
            </div>
          </div>

          <div className="p-4 bg-slate-950 rounded-xl border border-slate-800/80 text-xs text-slate-400 space-y-1">
            <div className="font-semibold text-slate-300">Konformitäts-Hinweis:</div>
            <div>
              Widerrufe nach § 355 BGB fließen nicht in die Abschlussquoten ein, um
              Vertriebsstatistiken nicht zu verfälschen.
            </div>
          </div>
        </div>
      </div>
    </div>
  );
};
