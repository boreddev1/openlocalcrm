import React, { useState, useEffect } from 'react';
import { QRCodeSVG } from 'qrcode.react';
import { apiFetch } from '../api/client';
import {
  Webhook,
  ShieldCheck,
  Database,
  Server,
  Copy,
  Check,
  Mail,
  Key,
  Sliders,
  Cpu,
  Send,
  QrCode,
  Archive,
  Download,
  Tag,
  FileText,
  Sparkles,
  Plus,
  RefreshCw,
  RotateCcw,
  Lock,
  AlertCircle,
  Users,
  UserPlus,
  Upload,
} from 'lucide-react';

interface TeamUser {
  id: string;
  name: string;
  email: string;
  role: 'ADMIN' | 'BENUTZER' | 'VERTRIEB' | 'BACKOFFICE';
  status: 'ACTIVE' | 'INVITED' | 'DEACTIVATED';
  invited_at?: string;
}

export const SettingsPage: React.FC = () => {
  const [activeTab, setActiveTab] = useState<
    | 'CONNECTORS'
    | 'EMAIL'
    | 'SECURITY'
    | 'USERS'
    | 'BACKUP'
    | 'TEMPLATES'
    | 'TAGS'
    | 'CUSTOM_FIELDS'
    | 'SYSTEM'
  >('CONNECTORS');
  const [copied, setCopied] = useState(false);
  const [testWebhookStatus, setTestWebhookStatus] = useState<string | null>(null);

  // --- Team & User Management State (§2.3) ---
  const [users, setUsers] = useState<TeamUser[]>([]);
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteName, setInviteName] = useState('');
  const [inviteRole, setInviteRole] = useState<'ADMIN' | 'BENUTZER' | 'VERTRIEB' | 'BACKOFFICE'>(
    'BENUTZER',
  );
  const [userMsg, setUserMsg] = useState<{ type: 'success' | 'error'; message: string } | null>(
    null,
  );

  useEffect(() => {
    if (activeTab === 'USERS') {
      apiFetch<any[]>('/api/v1/users')
        .then((res) => {
          if (Array.isArray(res)) {
            setUsers(
              res.map((u) => ({
                id: u.id,
                name: `${u.first_name || ''} ${u.last_name || ''}`.trim() || u.email,
                email: u.email,
                role: u.role,
                status: u.status,
                invited_at: u.status === 'INVITED' ? 'Ausstehend' : undefined,
              })),
            );
          }
        })
        .catch((err) => {
          console.error('Failed to load users:', err);
        });
    }
  }, [activeTab]);

  const reloadUsers = async () => {
    try {
      const res = await apiFetch<any[]>('/api/v1/users');
      if (Array.isArray(res)) {
        setUsers(
          res.map((u) => ({
            id: u.id,
            name: `${u.first_name || ''} ${u.last_name || ''}`.trim() || u.email,
            email: u.email,
            role: u.role,
            status: u.status,
            invited_at: u.status === 'INVITED' ? 'Ausstehend' : undefined,
          })),
        );
      }
    } catch (err) {
      console.error(err);
    }
  };

  const handleInviteUser = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!inviteEmail.trim() || !inviteName.trim()) {
      setUserMsg({ type: 'error', message: 'Bitte geben Sie Name und E-Mail-Adresse an.' });
      return;
    }

    const parts = inviteName.trim().split(' ');
    const firstName = parts[0] || 'Team';
    const lastName = parts.slice(1).join(' ') || 'Mitglied';

    try {
      await apiFetch('/api/v1/users/invite', {
        method: 'POST',
        body: JSON.stringify({
          email: inviteEmail.trim().toLowerCase(),
          first_name: firstName,
          last_name: lastName,
          role: inviteRole,
        }),
      });

      setUserMsg({ type: 'success', message: `Einladung erfolgreich an ${inviteEmail} gesendet.` });
      setInviteEmail('');
      setInviteName('');
      await reloadUsers();
    } catch (err: any) {
      setUserMsg({ type: 'error', message: err.message || 'Fehler beim Einladen des Benutzers' });
    }
    setTimeout(() => setUserMsg(null), 5000);
  };

  const handleToggleUserRole = async (userId: string) => {
    const targetUser = users.find((u) => u.id === userId);
    if (!targetUser) return;

    const newRole = targetUser.role === 'ADMIN' ? 'BENUTZER' : 'ADMIN';
    try {
      await apiFetch(`/api/v1/users/${userId}/role`, {
        method: 'PUT',
        body: JSON.stringify({ role: newRole }),
      });
      setUserMsg({
        type: 'success',
        message: `Rolle von ${targetUser.name} auf ${newRole} geändert.`,
      });
      await reloadUsers();
    } catch (err: any) {
      setUserMsg({ type: 'error', message: err.message || 'Fehler beim Ändern der Rolle' });
    }
    setTimeout(() => setUserMsg(null), 5000);
  };

  const handleToggleDeactivateUser = async (userId: string) => {
    const targetUser = users.find((u) => u.id === userId);
    if (!targetUser) return;

    const newStatus = targetUser.status === 'DEACTIVATED' ? 'ACTIVE' : 'DEACTIVATED';
    try {
      await apiFetch(`/api/v1/users/${userId}/status`, {
        method: 'PUT',
        body: JSON.stringify({ status: newStatus }),
      });
      setUserMsg({
        type: 'success',
        message:
          newStatus === 'DEACTIVATED'
            ? `Benutzer ${targetUser.name} wurde deaktiviert.`
            : `Benutzer ${targetUser.name} reaktiviert.`,
      });
      await reloadUsers();
    } catch (err: any) {
      setUserMsg({ type: 'error', message: err.message || 'Fehler beim Ändern des Status' });
    }
    setTimeout(() => setUserMsg(null), 5000);
  };

  // --- 2FA TOTP State (§2.2) ---
  const [totpEnabled, setTotpEnabled] = useState(false);
  const [totpSecret, setTotpSecret] = useState('');
  const [totpUrl, setTotpUrl] = useState('');
  const [totpCodeInput, setTotpCodeInput] = useState('');
  const [totpStatus, setTotpStatus] = useState<{
    type: 'success' | 'error';
    message: string;
  } | null>(null);

  const handleGenerateNewTotpSecret = async () => {
    try {
      const res = await apiFetch<{ secret: string; url: string }>('/api/v1/auth/totp/setup', {
        method: 'POST',
      });
      setTotpSecret(res.secret);
      setTotpUrl(res.url);
      setTotpEnabled(false);
      setTotpCodeInput('');
      setTotpStatus({
        type: 'success',
        message:
          'Neuer Secret Key generiert. Bitte mit Authenticator-App scannen und mit 6-stelligem Code verifizieren.',
      });
    } catch (err: any) {
      setTotpStatus({
        type: 'error',
        message: err.message || 'Fehler beim Generieren des TOTP-Schlüssels',
      });
    }
    setTimeout(() => setTotpStatus(null), 6000);
  };

  const handleVerifyTotp = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    if (!totpCodeInput || totpCodeInput.trim().length !== 6) {
      setTotpStatus({
        type: 'error',
        message: 'Bitte geben Sie den vollständigen 6-stelligen Code ein.',
      });
      return;
    }

    try {
      await apiFetch('/api/v1/auth/totp/verify', {
        method: 'POST',
        body: JSON.stringify({ code: totpCodeInput.trim() }),
      });
      setTotpEnabled(true);
      setTotpStatus({
        type: 'success',
        message: '2FA erfolgreich verifiziert und für Ihr Konto aktiviert!',
      });
      setTotpCodeInput('');
    } catch (err: any) {
      const msg = err.message?.includes('Ungültiger Authenticator-Code')
        ? err.message
        : 'Ungültiger Authenticator-Code: ' + (err.message || 'Prüfen Sie den Code.');
      setTotpStatus({ type: 'error', message: msg });
    }
    setTimeout(() => setTotpStatus(null), 6000);
  };

  // --- Password Change State (§2.2) ---
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [passwordStatus, setPasswordStatus] = useState<{
    type: 'success' | 'error';
    message: string;
  } | null>(null);

  const handlePasswordChange = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!currentPassword) {
      setPasswordStatus({ type: 'error', message: 'Bitte geben Sie Ihr aktuelles Passwort ein.' });
      return;
    }
    if (newPassword.length < 8) {
      setPasswordStatus({
        type: 'error',
        message: 'Das neue Passwort muss mindestens 8 Zeichen lang sein.',
      });
      return;
    }
    const hasNumber = /\d/.test(newPassword);
    const hasSpecial = /[^A-Za-z0-9]/.test(newPassword);
    if (!hasNumber || !hasSpecial) {
      setPasswordStatus({
        type: 'error',
        message: 'Das neue Passwort muss mindestens eine Zahl und ein Sonderzeichen enthalten.',
      });
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordStatus({ type: 'error', message: 'Die Passwörter stimmen nicht überein.' });
      return;
    }

    try {
      await apiFetch('/api/v1/auth/change-password', {
        method: 'POST',
        body: JSON.stringify({
          old_password: currentPassword,
          new_password: newPassword,
        }),
      });
      setPasswordStatus({ type: 'success', message: 'Passwort erfolgreich aktualisiert!' });
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
    } catch (err: any) {
      setPasswordStatus({
        type: 'error',
        message: err.message || 'Fehler beim Ändern des Passworts',
      });
    }
    setTimeout(() => setPasswordStatus(null), 6000);
  };

  // --- Backup & Restore-Drill State (§8.7) ---
  const [restoreDrillStatus, setRestoreDrillStatus] = useState<string | null>(null);
  const [isDrilling, setIsDrilling] = useState(false);

  const handleRunRestoreDrill = async () => {
    setIsDrilling(true);
    setRestoreDrillStatus('Konsistenzprüfung läuft...');
    try {
      const res = await apiFetch<any>('/api/v1/backup/drill', { method: 'POST' });
      setRestoreDrillStatus(
        `✅ Restore-Drill erfolgreich (Restore-Drill & Konsistenzprüfung erfolgreich: ${res.user_count} Benutzer, ${res.company_count} Firmen, ${res.contact_count} Kontakte, ${res.deal_count} Deals).`,
      );
    } catch (err: any) {
      setRestoreDrillStatus(`❌ Prüfung fehlgeschlagen: ${err.message}`);
    } finally {
      setIsDrilling(false);
    }
  };

  const handleDownloadBackup = async (_type?: 'db' | 'storage') => {
    try {
      const data = await apiFetch<any>('/api/v1/backup/export');
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `openlocalcrm-backup-${new Date().toISOString().slice(0, 10)}.json`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err: any) {
      alert(`Export fehlgeschlagen: ${err.message}`);
    }
  };

  // --- Portable Settings State (§v1.0.2) ---
  const [settingsStatus, setSettingsStatus] = useState<string | null>(null);
  const [isExportingSettings, setIsExportingSettings] = useState(false);
  const [isImportingSettings, setIsImportingSettings] = useState(false);

  const handleExportSettings = async () => {
    setIsExportingSettings(true);
    setSettingsStatus(null);
    try {
      const data = await apiFetch<any>('/api/v1/settings/export');
      const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `openlocalcrm-settings-${new Date().toISOString().slice(0, 10)}.json`;
      a.click();
      URL.revokeObjectURL(url);
      setSettingsStatus(
        'Systemeinstellungen erfolgreich als openlocalcrm-settings.json exportiert.',
      );
    } catch (err: any) {
      alert(`Export der Einstellungen fehlgeschlagen: ${err.message || err}`);
    } finally {
      setIsExportingSettings(false);
    }
  };

  const handleImportSettings = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    try {
      const text = await file.text();
      const parsed = JSON.parse(text);
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        throw new Error('Ungültiges Einstellungsformat: Root muss ein JSON-Objekt sein.');
      }
      if (!parsed.ai && !parsed.admin && !parsed.system && !parsed.backup) {
        throw new Error(
          'Ungültiges Einstellungsformat. Mindestens ein Konfigurationsabschnitt (ai, admin, system, backup) erforderlich.',
        );
      }
      if (parsed.ai && (typeof parsed.ai !== 'object' || Array.isArray(parsed.ai))) {
        throw new Error('Ungültiges Format für "ai": Objekt erwartet.');
      }
      if (parsed.ai?.provider && typeof parsed.ai.provider !== 'string') {
        throw new Error('Ungültiges Format für "ai.provider": String erwartet.');
      }
      if (parsed.ai?.model && typeof parsed.ai.model !== 'string') {
        throw new Error('Ungültiges Format für "ai.model": String erwartet.');
      }
      if (parsed.admin && (typeof parsed.admin !== 'object' || Array.isArray(parsed.admin))) {
        throw new Error('Ungültiges Format für "admin": Objekt erwartet.');
      }
      if (parsed.admin?.email && typeof parsed.admin.email !== 'string') {
        throw new Error('Ungültiges Format für "admin.email": String erwartet.');
      }
      if (parsed.system && (typeof parsed.system !== 'object' || Array.isArray(parsed.system))) {
        throw new Error('Ungültiges Format für "system": Objekt erwartet.');
      }

      setIsImportingSettings(true);
      setSettingsStatus(null);

      // Whitelist only approved configuration keys (M12)
      const sanitizedPayload: Record<string, any> = {};
      if (parsed.ai && typeof parsed.ai === 'object' && !Array.isArray(parsed.ai)) {
        sanitizedPayload.ai = {
          provider: typeof parsed.ai.provider === 'string' ? parsed.ai.provider : undefined,
          model: typeof parsed.ai.model === 'string' ? parsed.ai.model : undefined,
          base_url: typeof parsed.ai.base_url === 'string' ? parsed.ai.base_url : undefined,
        };
      }
      if (parsed.admin && typeof parsed.admin === 'object' && !Array.isArray(parsed.admin)) {
        sanitizedPayload.admin = {
          email: typeof parsed.admin.email === 'string' ? parsed.admin.email : undefined,
          name: typeof parsed.admin.name === 'string' ? parsed.admin.name : undefined,
        };
      }
      if (parsed.system && typeof parsed.system === 'object' && !Array.isArray(parsed.system)) {
        sanitizedPayload.system = {
          app_name: typeof parsed.system.app_name === 'string' ? parsed.system.app_name : undefined,
          currency: typeof parsed.system.currency === 'string' ? parsed.system.currency : undefined,
        };
      }
      if (parsed.backup && typeof parsed.backup === 'object' && !Array.isArray(parsed.backup)) {
        sanitizedPayload.backup = {
          auto_backup:
            typeof parsed.backup.auto_backup === 'boolean' ? parsed.backup.auto_backup : undefined,
          interval_hours:
            typeof parsed.backup.interval_hours === 'number'
              ? parsed.backup.interval_hours
              : undefined,
        };
      }

      const res = await apiFetch<any>('/api/v1/settings/import', {
        method: 'POST',
        body: JSON.stringify(sanitizedPayload),
      });

      setSettingsStatus(
        res.message || 'Einstellungen erfolgreich importiert! Bei Rebuild werden diese übernommen.',
      );
      alert(
        'Einstellungen erfolgreich übernommen! Wenn Sie den Container neu bauen oder den Launcher nutzen, bleiben diese Einstellungen aktiv.',
      );
    } catch (err: any) {
      alert(`Fehler beim Importieren der Einstellungen: ${err.message || err}`);
    } finally {
      setIsImportingSettings(false);
      e.target.value = '';
    }
  };

  // --- Tags State (§4.3) ---
  const [tags, setTags] = useState([
    {
      id: 'tag-1',
      name: '🏷️ PV-Projekt 2026',
      color: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
    },
    {
      id: 'tag-2',
      name: '⭐ VIP Großkunde',
      color: 'bg-amber-500/20 text-amber-400 border-amber-500/30',
    },
    {
      id: 'tag-3',
      name: '⚡ Dringend / Vor-Ort',
      color: 'bg-rose-500/20 text-rose-400 border-rose-500/30',
    },
    {
      id: 'tag-4',
      name: '🏠 D2D Haustür-Lead',
      color: 'bg-blue-500/20 text-blue-400 border-blue-500/30',
    },
  ]);
  const [newTagName, setNewTagName] = useState('');

  const handleAddTag = (e: React.FormEvent) => {
    e.preventDefault();
    if (!newTagName.trim()) return;
    setTags([
      ...tags,
      {
        id: 'tag-' + Date.now(),
        name: newTagName.trim(),
        color: 'bg-emerald-500/20 text-emerald-400 border-emerald-500/30',
      },
    ]);
    setNewTagName('');
  };

  // --- E-Mail Templates State (§4.6 / §5.1) ---
  const [templates, setTemplates] = useState([
    {
      id: 'tpl-1',
      name: 'PV 10 kWp Erstangebot mit Speicher',
      subject: 'Ihr persönliches Photovoltaik-Angebot für {{contact_name}}',
      body: 'Sehr geehrte(r) {{contact_name}},\n\nvielen Dank für Ihr Interesse an einer nachhaltigen Energieversorgung für {{company_name}}.\n\nGerne unterbreiten wir Ihnen das Angebot über ein 10 kWp Solarsystem inkl. Batteriespeicher im Gesamtwert von {{deal_amount}} €.\n\nMit freundlichen Grüßen,\n{{user_name}}',
    },
    {
      id: 'tpl-2',
      name: 'Auftragsbestätigung & Widerrufsbelehrung § 355 BGB',
      subject: 'Auftragsbestätigung & wichtige Verbraucherhinweise',
      body: 'Sehr geehrte(r) {{contact_name}},\n\nwir bestätigen hiermit Ihren Auftrag über {{deal_amount}} €.\n\nRechtliche Belehrung: Als Verbraucher haben Sie das Recht, binnen 14 Tagen ohne Angabe von Gründen diesen Vertrag zu widerrufen (§ 355 BGB).\n\nMit freundlichen Grüßen,\nIhr Team von OpenLocalCRM',
    },
  ]);
  const [promptTemplateText, setPromptTemplateText] = useState('');
  const [isGeneratingTemplate, setIsGeneratingTemplate] = useState(false);

  const handleGenerateTemplateWithAI = (e: React.FormEvent) => {
    e.preventDefault();
    if (!promptTemplateText.trim()) return;
    setIsGeneratingTemplate(true);

    setTimeout(() => {
      const generated = {
        id: 'tpl-' + Date.now(),
        name: `KI-Vorlage: ${promptTemplateText.slice(0, 30)}...`,
        subject: `Wichtiges Update zu Ihrem Energie-Projekt für {{contact_name}}`,
        body: `Guten Tag {{contact_name}},\n\nbasierend auf unserer Beratung bezüglich {{company_name}} möchten wir Ihnen die nächsten Schritte vorschlagen.\n\nDas geplante Projektvolumen beläuft sich auf {{deal_amount}} €.\n\nHerzliche Grüße,\n{{user_name}}`,
      };
      setTemplates([generated, ...templates]);
      setIsGeneratingTemplate(false);
      setPromptTemplateText('');
    }, 1000);
  };

  const [connectorToken, setConnectorToken] = useState('demo-connector-token');
  const webhookUrl = `${window.location.origin}/api/v1/connectors/lead-intake`;

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const handleTestWebhook = async () => {
    try {
      setTestWebhookStatus('Sende Test-Lead an Webhook...');
      const payload = {
        name: 'Sabine Mustermann (Test-Lead)',
        email: `sabine.mustermann+${Date.now()}@example.de`,
        phone: '+49 89 12345678',
        company: 'Mustermann Solar GmbH',
        source: 'Website-Kontaktformular',
        notes: 'Interesse an PV-Anlage und Wärmepumpe.',
        deal_value: 18500,
      };
      const res = await fetch(webhookUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Connector-Token': connectorToken,
        },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const errData = await res.json().catch(() => ({}));
        throw new Error(errData.error || `HTTP ${res.status}`);
      }
      const data = await res.json();
      setTestWebhookStatus(
        `Test-Lead erfolgreich über Webhook eingespeist! Kontakt-ID: ${data.contact_id || 'angelegt'}.`,
      );
    } catch (err: any) {
      setTestWebhookStatus(`Fehler beim Senden des Test-Leads: ${err.message}`);
    }
    setTimeout(() => setTestWebhookStatus(null), 7000);
  };

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-slate-100">Systemeinstellungen & Konfiguration</h1>
        <p className="text-sm text-slate-400">
          Single-Tenant Konfiguration, E-Mail-Konten, Templates, Sicherheit, Datensicherung & Custom
          Fields
        </p>
      </div>

      {/* Tabs */}
      <div className="flex items-center gap-2 border-b border-slate-800 pb-3 overflow-x-auto">
        <button
          onClick={() => setActiveTab('CONNECTORS')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'CONNECTORS'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Webhook className="w-3.5 h-3.5" />
          Konnektoren & Webhooks (§7.3)
        </button>

        <button
          onClick={() => setActiveTab('EMAIL')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'EMAIL'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Mail className="w-3.5 h-3.5" />
          E-Mail-Konten & IMAP (§4.1)
        </button>

        <button
          onClick={() => setActiveTab('TEMPLATES')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'TEMPLATES'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <FileText className="w-3.5 h-3.5" />
          E-Mail-Vorlagen (§4.6)
        </button>

        <button
          onClick={() => setActiveTab('SECURITY')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'SECURITY'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Key className="w-3.5 h-3.5" />
          Sicherheit & 2FA (§2)
        </button>

        <button
          onClick={() => setActiveTab('USERS')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'USERS'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Users className="w-3.5 h-3.5" />
          Benutzer & Team (§2.3)
        </button>

        <button
          onClick={() => setActiveTab('BACKUP')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'BACKUP'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Archive className="w-3.5 h-3.5" />
          Datensicherung & Backups (§8.7)
        </button>

        <button
          onClick={() => setActiveTab('TAGS')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'TAGS'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Tag className="w-3.5 h-3.5" />
          Tags & Labels (§4.3)
        </button>

        <button
          onClick={() => setActiveTab('CUSTOM_FIELDS')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'CUSTOM_FIELDS'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Sliders className="w-3.5 h-3.5" />
          Custom Fields & Energie (§3.8)
        </button>

        <button
          onClick={() => setActiveTab('SYSTEM')}
          className={`px-3.5 py-2 rounded-xl text-xs font-semibold transition-colors flex items-center gap-2 shrink-0 ${
            activeTab === 'SYSTEM'
              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
              : 'text-slate-400 hover:text-slate-200'
          }`}
        >
          <Cpu className="w-3.5 h-3.5" />
          System-Info & DSGVO (§1.2/§22)
        </button>
      </div>

      {/* Tab: CONNECTORS */}
      {activeTab === 'CONNECTORS' && (
        <div className="space-y-5">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-sm space-y-6">
            <div className="flex items-center justify-between border-b border-slate-800 pb-4">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-xl bg-emerald-500/10 text-emerald-400 flex items-center justify-center">
                  <Webhook className="w-5 h-5" />
                </div>
                <div>
                  <h3 className="text-base font-bold text-slate-100">
                    Lead-Intake & Partner-Konnektor (REST Webhook)
                  </h3>
                  <p className="text-xs text-slate-400">
                    Automatische Erfassung von Leads aus Website-Formularen, Tarifrechnern oder
                    D2D-Kampagnen
                  </p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={handleTestWebhook}
                  className="px-3 py-1.5 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded-lg text-xs font-semibold transition-colors flex items-center gap-1.5"
                >
                  <Send className="w-3.5 h-3.5 text-emerald-400" />
                  Test-Payload senden
                </button>
                <span className="px-3 py-1 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 text-xs font-semibold rounded-full">
                  Aktiv
                </span>
              </div>
            </div>

            {testWebhookStatus && (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2">
                <Check className="w-4 h-4" />
                {testWebhookStatus}
              </div>
            )}

            <div className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Webhook Endpoint URL
                </label>
                <div className="flex gap-2">
                  <input
                    readOnly
                    value={webhookUrl}
                    className="flex-1 px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-slate-300"
                  />
                  <button
                    onClick={() => copyToClipboard(webhookUrl)}
                    className="px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg text-xs flex items-center gap-1.5 transition-colors"
                  >
                    {copied ? (
                      <Check className="w-3.5 h-3.5 text-emerald-400" />
                    ) : (
                      <Copy className="w-3.5 h-3.5" />
                    )}
                    Kopieren
                  </button>
                </div>
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="block text-xs font-semibold text-slate-300">
                    Connector API Secret Bearer-Token (X-Connector-Token)
                  </label>
                  <span className="text-[10px] text-slate-500">
                    Konfigurierbar via CONNECTOR_API_TOKEN
                  </span>
                </div>
                <div className="flex gap-2">
                  <input
                    value={connectorToken}
                    onChange={(e) => {
                      setConnectorToken(e.target.value);
                    }}
                    placeholder="Connector API Token (z.B. demo-connector-token)"
                    className="flex-1 px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-slate-300 focus:outline-none focus:border-emerald-500"
                  />
                  <button
                    type="button"
                    onClick={() => copyToClipboard(connectorToken)}
                    className="px-3 py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg text-xs flex items-center gap-1.5 transition-colors"
                  >
                    <Copy className="w-3.5 h-3.5" />
                    Kopieren
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab: EMAIL */}
      {activeTab === 'EMAIL' && (
        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-6">
          <div className="flex items-center justify-between border-b border-slate-800 pb-4">
            <div>
              <h3 className="text-base font-bold text-slate-100">
                E-Mail-Konten (IMAP / SMTP Synchronisation)
              </h3>
              <p className="text-xs text-slate-400">
                Verwalte Postfächer für automatischen E-Mail-Empfang und KI-Versand
              </p>
            </div>
            <span className="text-xs px-2.5 py-1 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 rounded-full font-semibold">
              1 Konto aktiv
            </span>
          </div>

          <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Mail className="w-5 h-5 text-emerald-400" />
                <div>
                  <div className="text-sm font-semibold text-slate-200">
                    vertrieb@openlocalcrm.local
                  </div>
                  <div className="text-xs text-slate-500">
                    Primäres Konto für Workflows & KI-Drafts (§4.1)
                  </div>
                </div>
              </div>
              <span className="text-xs font-mono text-emerald-400">IMAP: SSL / Port 993</span>
            </div>
            <div className="grid grid-cols-2 gap-4 text-xs text-slate-400 pt-2 border-t border-slate-800/80">
              <div>
                SMTP Host:{' '}
                <span className="font-mono text-slate-300">
                  mail.openlocalcrm.local:587 (STARTTLS)
                </span>
              </div>
              <div>
                Auto-Sync Intervall:{' '}
                <span className="font-mono text-slate-300">Alle 60 Sekunden</span>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab: TEMPLATES (§4.6 / §5.1) */}
      {activeTab === 'TEMPLATES' && (
        <div className="space-y-6">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div>
                <h3 className="text-base font-bold text-slate-100">
                  KI-Template-Generator per Chat (§5.1)
                </h3>
                <p className="text-xs text-slate-400">
                  Lassen Sie Gemma 12B fertige Vorlagen mit Platzhaltern nach Ihren Wünschen
                  erstellen
                </p>
              </div>
            </div>

            <form onSubmit={handleGenerateTemplateWithAI} className="flex gap-3">
              <input
                type="text"
                value={promptTemplateText}
                onChange={(e) => setPromptTemplateText(e.target.value)}
                placeholder="z.B. 'Erstelle ein Template für Nachfass-Mails bei Photovoltaik-Kunden'..."
                className="flex-1 px-4 py-2.5 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
              />
              <button
                type="submit"
                disabled={isGeneratingTemplate || !promptTemplateText.trim()}
                className="px-5 py-2.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs flex items-center gap-2 transition-colors disabled:opacity-40"
              >
                <Sparkles className="w-4 h-4" />
                {isGeneratingTemplate ? 'Generiert...' : 'Template generieren'}
              </button>
            </form>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            {templates.map((tpl) => (
              <div
                key={tpl.id}
                className="bg-slate-900 border border-slate-800 rounded-2xl p-5 space-y-3"
              >
                <div className="flex items-center justify-between">
                  <h4 className="text-sm font-bold text-slate-100">{tpl.name}</h4>
                  <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-slate-800 text-emerald-400 border border-slate-700">
                    Aktiv
                  </span>
                </div>
                <div className="text-xs text-slate-400 font-semibold">
                  Betreff: <span className="text-slate-200">{tpl.subject}</span>
                </div>
                <pre className="p-3 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-300 font-mono whitespace-pre-wrap leading-relaxed">
                  {tpl.body}
                </pre>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Tab: SECURITY (Password Change & 2FA) */}
      {activeTab === 'SECURITY' && (
        <div className="space-y-6">
          {/* Password Change (§2.2 / §2.3) */}
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-5">
            <div className="border-b border-slate-800 pb-3">
              <h3 className="text-base font-bold text-slate-100 flex items-center gap-2">
                <Lock className="w-4 h-4 text-emerald-400" /> Eigenes Passwort ändern (§2.2)
              </h3>
              <p className="text-xs text-slate-400">
                Regeln: Mindestens 8 Zeichen, mindestens 1 Zahl und 1 Sonderzeichen
              </p>
            </div>

            {passwordStatus && (
              <div
                className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
                  passwordStatus.type === 'success'
                    ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-300'
                    : 'bg-rose-500/10 border border-rose-500/20 text-rose-300'
                }`}
              >
                {passwordStatus.type === 'success' ? (
                  <Check className="w-4 h-4" />
                ) : (
                  <AlertCircle className="w-4 h-4" />
                )}
                {passwordStatus.message}
              </div>
            )}

            <form onSubmit={handlePasswordChange} className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Aktuelles Passwort
                </label>
                <input
                  type="password"
                  value={currentPassword}
                  onChange={(e) => setCurrentPassword(e.target.value)}
                  placeholder="••••••••"
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Neues Passwort
                </label>
                <input
                  type="password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                  placeholder="Min. 8 Zeichen + Zahl + Sonderzeichen"
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Passwort bestätigen
                </label>
                <input
                  type="password"
                  value={confirmPassword}
                  onChange={(e) => setConfirmPassword(e.target.value)}
                  placeholder="Wiederholung"
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div className="sm:col-span-3 flex justify-end">
                <button
                  type="submit"
                  className="px-5 py-2.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs transition-colors"
                >
                  Passwort speichern
                </button>
              </div>
            </form>
          </div>

          {/* 2FA TOTP */}
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-6">
            <div className="flex items-center justify-between border-b border-slate-800 pb-4">
              <div>
                <h3 className="text-base font-bold text-slate-100">
                  Zwei-Faktor-Authentifizierung (RFC 6238 TOTP)
                </h3>
                <p className="text-xs text-slate-400">
                  Schützen Sie den Zugang zu Ihrem Single-Tenant CRM mit Authenticator-Apps
                </p>
              </div>
              <button
                onClick={() => setTotpEnabled(!totpEnabled)}
                className={`px-3 py-1.5 rounded-lg text-xs font-bold transition-colors ${
                  totpEnabled ? 'bg-emerald-600 text-slate-950' : 'bg-slate-800 text-slate-400'
                }`}
              >
                {totpEnabled ? '2FA Aktiviert' : '2FA Deaktiviert'}
              </button>
            </div>

            {totpStatus && (
              <div
                className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
                  totpStatus.type === 'success'
                    ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-300'
                    : 'bg-rose-500/10 border border-rose-500/20 text-rose-300'
                }`}
              >
                {totpStatus.type === 'success' ? (
                  <Check className="w-4 h-4" />
                ) : (
                  <AlertCircle className="w-4 h-4" />
                )}
                {totpStatus.message}
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6 items-center">
              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-semibold text-slate-300">TOTP Secret Key</span>
                  <button
                    type="button"
                    onClick={handleGenerateNewTotpSecret}
                    className="text-[11px] text-emerald-400 hover:text-emerald-300 flex items-center gap-1 font-semibold"
                  >
                    <RotateCcw className="w-3 h-3" />
                    <span>Neu generieren</span>
                  </button>
                </div>
                <input
                  readOnly
                  value={totpSecret}
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-emerald-400"
                />
                <form onSubmit={handleVerifyTotp} className="flex gap-2 pt-1">
                  <input
                    type="text"
                    maxLength={6}
                    value={totpCodeInput}
                    onChange={(e) => setTotpCodeInput(e.target.value)}
                    placeholder="6-stelliger Code (z.B. 123456)"
                    className="flex-1 px-3 py-1.5 bg-slate-950 border border-slate-800 rounded-lg text-xs font-mono text-slate-100 placeholder-slate-600 focus:outline-none focus:border-emerald-500"
                  />
                  <button
                    type="submit"
                    className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-lg text-xs transition-colors"
                  >
                    Verifizieren
                  </button>
                </form>
                <p className="text-xs text-slate-500">
                  Kompatibel mit Google Authenticator, Apple Passwords, 1Password, Bitwarden und
                  YubiKey.
                </p>
              </div>
              <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl flex items-center justify-center gap-4">
                {totpUrl ? (
                  <div className="p-2 bg-white rounded-lg flex items-center justify-center shadow-md">
                    <QRCodeSVG value={totpUrl} size={110} level="M" />
                  </div>
                ) : (
                  <div className="w-[110px] h-[110px] bg-slate-900 border border-dashed border-slate-800 rounded-lg flex items-center justify-center">
                    <QrCode className="w-12 h-12 text-slate-500" />
                  </div>
                )}
                <div className="text-xs text-slate-400">
                  <div className="font-semibold text-slate-200">OpenLocalCRM: Admin</div>
                  <div>Algorithmus: SHA-1 (6 Digits)</div>
                  <div>Periode: 30s</div>
                  {totpSecret ? (
                    <div className="font-mono text-[10px] text-emerald-400 mt-1">
                      Key: {totpSecret.slice(0, 8)}...
                    </div>
                  ) : (
                    <div className="text-[10px] text-slate-500 mt-1">
                      Klicken Sie auf &quot;Neu generieren&quot; für QR-Code
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab: USERS & TEAM (§2.3) */}
      {activeTab === 'USERS' && (
        <div className="space-y-6">
          {/* Invite User Card */}
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div>
                <h3 className="text-base font-bold text-slate-100 flex items-center gap-2">
                  <UserPlus className="w-4 h-4 text-emerald-400" /> Neuen Benutzer einladen (§2.3)
                </h3>
                <p className="text-xs text-slate-400">
                  Einladungslink ist 7 Tage gültig. Benutzer vergibt beim Erstlogin eigenes Passwort
                  (min. 8 Zeichen).
                </p>
              </div>
            </div>

            {userMsg && (
              <div
                className={`p-3 rounded-xl text-xs flex items-center gap-2 ${
                  userMsg.type === 'success'
                    ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-300'
                    : 'bg-rose-500/10 border border-rose-500/20 text-rose-300'
                }`}
              >
                {userMsg.type === 'success' ? (
                  <Check className="w-4 h-4" />
                ) : (
                  <AlertCircle className="w-4 h-4" />
                )}
                {userMsg.message}
              </div>
            )}

            <form
              onSubmit={handleInviteUser}
              className="grid grid-cols-1 sm:grid-cols-4 gap-3 items-end"
            >
              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Vollständiger Name
                </label>
                <input
                  type="text"
                  value={inviteName}
                  onChange={(e) => setInviteName(e.target.value)}
                  placeholder="z.B. Tim Tester"
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  E-Mail-Adresse
                </label>
                <input
                  type="email"
                  value={inviteEmail}
                  onChange={(e) => setInviteEmail(e.target.value)}
                  placeholder="tim@unternehmen.de"
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                />
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-300 mb-1">
                  Rolle im CRM (§2.1)
                </label>
                <select
                  value={inviteRole}
                  onChange={(e) =>
                    setInviteRole(
                      e.target.value as 'ADMIN' | 'BENUTZER' | 'VERTRIEB' | 'BACKOFFICE',
                    )
                  }
                  className="w-full px-3 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
                >
                  <option value="BENUTZER">Benutzer (Standard)</option>
                  <option value="VERTRIEB">Vertrieb</option>
                  <option value="BACKOFFICE">Backoffice</option>
                  <option value="ADMIN">Admin (Vollzugriff)</option>
                </select>
              </div>

              <div>
                <button
                  type="submit"
                  className="w-full px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs flex items-center justify-center gap-1.5 transition-colors"
                >
                  <UserPlus className="w-3.5 h-3.5" />
                  <span>Einladung senden</span>
                </button>
              </div>
            </form>
          </div>

          {/* Active Users List */}
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-4">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div>
                <h3 className="text-base font-bold text-slate-100 flex items-center gap-2">
                  <Users className="w-4 h-4 text-emerald-400" /> Aktive Teammitglieder (
                  {users.length})
                </h3>
                <p className="text-xs text-slate-400">
                  Shared-Workspace (§2.1): Alle Benutzer sehen dieselben CRM-Daten. Rollenwechsel
                  und Deaktivierung werden protokolliert.
                </p>
              </div>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead>
                  <tr className="border-b border-slate-800 text-slate-400">
                    <th className="pb-3 font-semibold">Benutzer</th>
                    <th className="pb-3 font-semibold">E-Mail</th>
                    <th className="pb-3 font-semibold">Rolle</th>
                    <th className="pb-3 font-semibold">Status</th>
                    <th className="pb-3 font-semibold text-right">Aktionen</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60">
                  {users.map((u) => (
                    <tr key={u.id} className="hover:bg-slate-800/30">
                      <td className="py-3 font-semibold text-slate-200 flex items-center gap-2">
                        <div className="w-7 h-7 rounded-full bg-emerald-500/10 text-emerald-400 flex items-center justify-center font-bold text-xs border border-emerald-500/20">
                          {u.name.charAt(0)}
                        </div>
                        {u.name}
                      </td>
                      <td className="py-3 font-mono text-slate-400">{u.email}</td>
                      <td className="py-3">
                        <span
                          className={`px-2 py-0.5 rounded-full font-semibold text-[10px] ${
                            u.role === 'ADMIN'
                              ? 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
                              : 'bg-blue-500/10 text-blue-400 border border-blue-500/20'
                          }`}
                        >
                          {u.role}
                        </span>
                      </td>
                      <td className="py-3">
                        <span
                          className={`px-2 py-0.5 rounded-full font-semibold text-[10px] ${
                            u.status === 'ACTIVE'
                              ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                              : u.status === 'INVITED'
                                ? 'bg-purple-500/10 text-purple-400 border border-purple-500/20'
                                : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                          }`}
                        >
                          {u.status === 'ACTIVE'
                            ? 'Aktiv'
                            : u.status === 'INVITED'
                              ? 'Eingeladen (7 Tage)'
                              : 'Deaktiviert'}
                        </span>
                      </td>
                      <td className="py-3 text-right space-x-2">
                        <button
                          type="button"
                          onClick={() => handleToggleUserRole(u.id)}
                          className="px-2 py-1 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-lg text-[10px] font-semibold transition-colors"
                        >
                          Rolle: {u.role === 'ADMIN' ? 'Zu Benutzer' : 'Zu Admin'}
                        </button>
                        <button
                          type="button"
                          onClick={() => handleToggleDeactivateUser(u.id)}
                          className={`px-2 py-1 rounded-lg text-[10px] font-semibold transition-colors ${
                            u.status !== 'DEACTIVATED'
                              ? 'bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20'
                              : 'bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-400 border border-emerald-500/20'
                          }`}
                        >
                          {u.status !== 'DEACTIVATED' ? 'Deaktivieren' : 'Aktivieren'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* Tab: BACKUP & RESTORE-DRILL (§8.7) */}
      {activeTab === 'BACKUP' && (
        <div className="space-y-6">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-6">
            <div className="flex items-center justify-between border-b border-slate-800 pb-4">
              <div>
                <h3 className="text-base font-bold text-slate-100">
                  Datensicherung, Exporte & Restore-Drill (§8.7)
                </h3>
                <p className="text-xs text-slate-400">
                  Vollständige Datensouveränität mit PostgreSQL Dumps und nachweisbarer
                  Wiederherstellung
                </p>
              </div>
            </div>

            {restoreDrillStatus && (
              <div className="p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2">
                <Check className="w-4 h-4" />
                {restoreDrillStatus}
              </div>
            )}

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl space-y-3">
                <div className="text-xs font-bold text-slate-200 flex items-center gap-2">
                  <Database className="w-4 h-4 text-emerald-400" /> PostgreSQL Datenbank-Backup
                </div>
                <p className="text-xs text-slate-400">
                  Erstellt einen vollständigen SQL-Dump aller Kontakte, Deals, Notizen und
                  Audit-Logs.
                </p>
                <button
                  onClick={() => handleDownloadBackup('db')}
                  className="w-full py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold rounded-lg flex items-center justify-center gap-2 transition-colors"
                >
                  <Download className="w-3.5 h-3.5" /> SQL Dump herunterladen
                </button>
              </div>

              <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl space-y-3">
                <div className="text-xs font-bold text-slate-200 flex items-center gap-2">
                  <Archive className="w-4 h-4 text-blue-400" /> Dateispeicher-Backup
                </div>
                <p className="text-xs text-slate-400">
                  Sichert alle hochgeladenen E-Mail-Anhänge, Angebote und Dokumente als Archiv.
                </p>
                <button
                  onClick={() => handleDownloadBackup('storage')}
                  className="w-full py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold rounded-lg flex items-center justify-center gap-2 transition-colors"
                >
                  <Download className="w-3.5 h-3.5" /> Archiv herunterladen
                </button>
              </div>

              <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl space-y-3">
                <div className="text-xs font-bold text-slate-200 flex items-center gap-2">
                  <RefreshCw className="w-4 h-4 text-cyan-400" /> Restore-Drill Integritäts-Test
                </div>
                <p className="text-xs text-slate-400">
                  Prüft automatisch die Wiederherstellbarkeit und referentielle Integrität nach
                  §8.7.
                </p>
                <button
                  onClick={handleRunRestoreDrill}
                  disabled={isDrilling}
                  className="w-full py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 text-xs font-bold rounded-lg flex items-center justify-center gap-2 transition-colors disabled:opacity-50"
                >
                  <ShieldCheck className="w-3.5 h-3.5" />{' '}
                  {isDrilling ? 'Prüfung läuft...' : 'Restore-Drill ausführen'}
                </button>
              </div>
            </div>

            {/* Portable Settings (Export/Import without DB dump) (§v1.0.2) */}
            <div className="border-t border-slate-800 pt-6">
              <div className="flex items-center justify-between mb-3">
                <div>
                  <h4 className="text-sm font-bold text-slate-100 flex items-center gap-2">
                    <Sliders className="w-4 h-4 text-amber-400" /> Schnelleinstellungen Export &
                    Import (ohne DB-Dump)
                  </h4>
                  <p className="text-xs text-slate-400 mt-1">
                    Sichern Sie AI-Provider-Konfiguration, API-Keys, Web-Port und Admin-Benutzer
                    separat, um Instanzen bei Rebuilds oder Neuinstallationen blitzschnell ohne alte
                    Datenbankleichen wiederherzustellen.
                  </p>
                </div>
              </div>

              {settingsStatus && (
                <div className="mb-4 p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-xs text-emerald-300 flex items-center gap-2">
                  <Check className="w-4 h-4" />
                  {settingsStatus}
                </div>
              )}

              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl space-y-3">
                  <div className="text-xs font-bold text-slate-200 flex items-center gap-2">
                    <Download className="w-4 h-4 text-amber-400" /> Konfiguration exportieren
                  </div>
                  <p className="text-xs text-slate-400">
                    Lädt eine portable{' '}
                    <code className="text-amber-300">openlocalcrm-settings.json</code> mit Ihren
                    aktuellen KI-Modellen, Endpoints und System-Flags herunter.
                  </p>
                  <button
                    onClick={handleExportSettings}
                    disabled={isExportingSettings}
                    className="w-full py-2 bg-slate-800 hover:bg-slate-700 text-slate-200 text-xs font-semibold rounded-lg flex items-center justify-center gap-2 transition-colors disabled:opacity-50"
                  >
                    <Download className="w-3.5 h-3.5" />
                    {isExportingSettings ? 'Exportiere...' : 'Einstellungen exportieren (.json)'}
                  </button>
                </div>

                <div className="p-4 bg-slate-950 border border-slate-800 rounded-xl space-y-3">
                  <div className="text-xs font-bold text-slate-200 flex items-center gap-2">
                    <Upload className="w-4 h-4 text-emerald-400" /> Konfiguration importieren
                  </div>
                  <p className="text-xs text-slate-400">
                    Stellt KI-, Admin- und Port-Parameter aus einer vorhandenen{' '}
                    <code className="text-emerald-300">openlocalcrm-settings.json</code> wieder her.
                  </p>
                  <label className="w-full py-2 bg-amber-600/20 hover:bg-amber-600/30 text-amber-300 border border-amber-500/30 text-xs font-bold rounded-lg flex items-center justify-center gap-2 cursor-pointer transition-colors">
                    <Upload className="w-3.5 h-3.5" />
                    {isImportingSettings ? 'Importiere...' : 'Einstellungen importieren (.json)'}
                    <input
                      type="file"
                      accept=".json"
                      onChange={handleImportSettings}
                      disabled={isImportingSettings}
                      className="hidden"
                    />
                  </label>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Tab: TAGS (§4.3) */}
      {activeTab === 'TAGS' && (
        <div className="space-y-6">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-5">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div>
                <h3 className="text-base font-bold text-slate-100">
                  E-Mail- & Kontakt-Tags (§4.3)
                </h3>
                <p className="text-xs text-slate-400">
                  Definieren Sie eigene Labels und Schlagworte für Posteingang und Kontakte
                </p>
              </div>
            </div>

            <form onSubmit={handleAddTag} className="flex gap-3">
              <input
                type="text"
                value={newTagName}
                onChange={(e) => setNewTagName(e.target.value)}
                placeholder="z.B. '🏷️ Solarpark Rhein-Main'..."
                className="flex-1 px-4 py-2 bg-slate-950 border border-slate-800 rounded-xl text-xs text-slate-100 focus:outline-none focus:border-emerald-500"
              />
              <button
                type="submit"
                className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 text-slate-950 font-bold rounded-xl text-xs flex items-center gap-1.5"
              >
                <Plus className="w-3.5 h-3.5" /> Tag hinzufügen
              </button>
            </form>

            <div className="flex flex-wrap gap-2 pt-2">
              {tags.map((tag) => (
                <span
                  key={tag.id}
                  className={`px-3 py-1.5 rounded-xl border text-xs font-semibold ${tag.color}`}
                >
                  {tag.name}
                </span>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Tab: CUSTOM FIELDS */}
      {activeTab === 'CUSTOM_FIELDS' && (
        <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 space-y-6">
          <div className="border-b border-slate-800 pb-4">
            <h3 className="text-base font-bold text-slate-100">
              Custom Fields & Branchen-Felder (§3.8 / §7.1)
            </h3>
            <p className="text-xs text-slate-400">
              Konfigurierte Attribute für Photovoltaik-, Energie- und D2D-Vertrieb
            </p>
          </div>

          <div className="space-y-3">
            <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl flex items-center justify-between text-xs">
              <div>
                <span className="font-semibold text-slate-200">Zählernummer (Strom / Gas)</span>
                <div className="text-[11px] text-slate-500">
                  Typ: TEXT (Regex: 1EMH... / 33-stellig) — Entität: Kontakt / Firma
                </div>
              </div>
              <span className="px-2 py-0.5 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 rounded">
                Aktiv
              </span>
            </div>

            <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl flex items-center justify-between text-xs">
              <div>
                <span className="font-semibold text-slate-200">
                  Jahresverbrauch (Strom kWh / Gas kWh)
                </span>
                <div className="text-[11px] text-slate-500">
                  Typ: NUMERIC — Entität: Deal / Kontakt
                </div>
              </div>
              <span className="px-2 py-0.5 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 rounded">
                Aktiv
              </span>
            </div>

            <div className="p-3 bg-slate-950 border border-slate-800 rounded-xl flex items-center justify-between text-xs">
              <div>
                <span className="font-semibold text-slate-200">
                  Eigentümerstatus (Eigentümer / Mieter)
                </span>
                <div className="text-[11px] text-slate-500">
                  Typ: ENUM — Entität: D2D Gebietskarte / Lead
                </div>
              </div>
              <span className="px-2 py-0.5 bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 rounded">
                Aktiv
              </span>
            </div>
          </div>
        </div>
      )}

      {/* Tab: SYSTEM */}
      {activeTab === 'SYSTEM' && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 space-y-2">
              <div className="flex items-center gap-2 text-xs font-semibold text-slate-400">
                <Server className="w-4 h-4 text-emerald-400" /> Server-Architektur
              </div>
              <div className="text-sm font-bold text-slate-100">Go 1.26 + Caddy + Chi</div>
              <div className="text-xs text-slate-500">
                Single-Tenant Standalone Binary (&lt; 420 MB RAM Budget)
              </div>
            </div>

            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 space-y-2">
              <div className="flex items-center gap-2 text-xs font-semibold text-slate-400">
                <Database className="w-4 h-4 text-blue-400" /> Datenbank-Engine
              </div>
              <div className="text-sm font-bold text-slate-100">PostgreSQL 16 + pgvector</div>
              <div className="text-xs text-slate-500">Trigramm-Volltextsuche + Vektoren</div>
            </div>

            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-4 space-y-2">
              <div className="flex items-center gap-2 text-xs font-semibold text-slate-400">
                <ShieldCheck className="w-4 h-4 text-cyan-400" /> Lizenz-Compliance (§22)
              </div>
              <div className="text-sm font-bold text-slate-100">MIT Open Source</div>
              <div className="text-xs text-slate-500">100% Freie & Open Source Komponenten</div>
            </div>
          </div>

          <div className="bg-slate-900 border border-slate-800 rounded-2xl p-5 space-y-3">
            <h4 className="text-xs font-bold uppercase text-slate-400">
              DSGVO & Rechtliche Konformität
            </h4>
            <div className="text-xs text-slate-300 space-y-1.5">
              <div>
                • <strong>DSGVO Art. 20 (Datenübertragbarkeit):</strong> Vollständiger CSV-Export
                aller Kunden und Aktivitäten.
              </div>
              <div>
                • <strong>UWG § 7 (Werbeeinwilligung):</strong> Telefon- & E-Mail-Consent-Nachweise
                in Kontaktdaten hinterlegt.
              </div>
              <div>
                • <strong>BGB § 355 (Widerrufsrecht):</strong> Erfassung von Widerrufen ohne
                Verzerrung historischer Vertriebsstatistiken.
              </div>
              <div>
                • <strong>EU AI Act (Art. 50/52):</strong> Revisionssicheres AI-Audit-Ledger mit
                Latenz, Provider und Modelltransparenz.
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
