import React, { useState, useEffect } from 'react';
import { Outlet } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { Header } from './Header';
import { AIChatDrawer } from '../ai/AIChatDrawer';
import { CommandPalette } from '../common/CommandPalette';
import { SimulationWidget } from '../../simulation/SimulationWidget';
import { DemoBanner } from '../common/DemoBanner';

export const AppLayout: React.FC = () => {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);

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
        <Header
          onMenuToggle={() => setSidebarOpen((prev) => !prev)}
          onOpenSearch={() => setCommandPaletteOpen(true)}
        />

        <main className="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8">
          <Outlet />
        </main>

        {/* Global Floating AI Assistant Chat Window */}
        <AIChatDrawer />

        {/* Global 10-Minute Live Simulation Widget */}
        <SimulationWidget />

        {/* Global Cmd+K Quick-Switcher Command Palette (§3.7) */}
        <CommandPalette isOpen={commandPaletteOpen} onClose={() => setCommandPaletteOpen(false)} />
      </div>
    </div>
  );
};
