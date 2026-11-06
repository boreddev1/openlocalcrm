import React, { useState, useEffect } from 'react';
import { Outlet } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { AIChatDrawer } from '../ai/AIChatDrawer';
import { CommandPalette } from '../common/CommandPalette';
import { DemoBanner } from '../common/DemoBanner';
import { PasswordNagBanner } from '../common/PasswordNagBanner';
import { apiFetch } from '../../api/client';

const SimulationWidget = React.lazy(() =>
  import('../../simulation/SimulationWidget').then((m) => ({ default: m.SimulationWidget })),
);

export const AppLayout: React.FC = () => {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
  const [isDemo, setIsDemo] = useState(false);

  useEffect(() => {
    apiFetch<{ demo_mode?: boolean }>('/api/v1/health')
      .then((res) => {
        if (res && res.demo_mode) {
          setIsDemo(true);
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setCommandPaletteOpen((prev) => !prev);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  return (
    <div className="flex h-screen overflow-hidden bg-slate-950 text-slate-100">
      <Sidebar isOpen={sidebarOpen} onClose={() => setSidebarOpen(false)} />

      <div className="flex-1 flex flex-col min-w-0 overflow-hidden relative">
        <DemoBanner />
        <PasswordNagBanner />
        <Header
          onMenuToggle={() => setSidebarOpen((prev) => !prev)}
          onOpenSearch={() => setCommandPaletteOpen(true)}
        />

        <main className="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8">
          <Outlet />
        </main>

        {/* Global Floating AI Assistant Chat Window */}
        <AIChatDrawer />

        {/* Global 10-Minute Live Simulation Widget (isolated to DEMO_MODE only) */}
        {isDemo && (
          <React.Suspense fallback={null}>
            <SimulationWidget />
          </React.Suspense>
        )}

        {/* Global Cmd+K Quick-Switcher Command Palette (§3.7) */}
        <CommandPalette isOpen={commandPaletteOpen} onClose={() => setCommandPaletteOpen(false)} />
      </div>
    </div>
  );
};
