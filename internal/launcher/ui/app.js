// OpenLocalCRM - Setup & Control Center Client Logic

let currentStep = 1;
let configuredPort = 80;
let savedAdminPassword = '';
let uploadedBackupFilename = null;
let healthCheckInterval = null;

let existingSystemConfig = null;

document.addEventListener('DOMContentLoaded', () => {
  generatePassword();
  loadAvailableVersions();
  checkInitialStatus();
  checkForUpdates(false);
});

async function checkInitialStatus() {
  try {
    const res = await fetch('/api/status');
    const data = await res.json();

    if (data.existing_config) {
      existingSystemConfig = data.existing_config;
      configuredPort = data.existing_config.port || 80;
    }

    const hasContainers = (data.containers || []).length > 0;
    const hasRunningContainers = (data.containers || []).some(c => 
      c.state === 'running' || (c.status && c.status.toLowerCase().includes('up'))
    );

    if (data.installed || hasRunningContainers || hasContainers) {
      switchToControlCenter();
      loadContainers(data.containers || []);

      const banner = document.getElementById('installed-detected-banner');
      const bannerText = document.getElementById('installed-detected-text');
      if (banner && bannerText) {
        banner.style.display = 'block';
        bannerText.textContent = `${data.reason || 'Bestehende Docker-Container erkannt'} | Port: ${configuredPort} | Admin: ${data.existing_config?.admin_email || 'admin@openlocalcrm.local'}`;
      }

      const wizBanner = document.getElementById('wizard-already-installed-banner');
      const wizBannerText = document.getElementById('wizard-already-installed-text');
      if (wizBanner && wizBannerText) {
        wizBanner.classList.remove('hidden');
        wizBannerText.textContent = `${data.reason || 'Installation aktiv'} auf diesem Rechner. Sie können die Einstellungen anpassen oder direkt ins CRM wechseln.`;
      }
    } else {
      runPreflight();
    }
  } catch (err) {
    console.error('Failed to get status:', err);
    runPreflight();
  }
}

function editExistingConfig() {
  if (existingSystemConfig) {
    if (existingSystemConfig.admin_email) {
      document.getElementById('admin-email').value = existingSystemConfig.admin_email;
    }
    if (existingSystemConfig.port) {
      document.getElementById('port-input').value = existingSystemConfig.port;
      configuredPort = existingSystemConfig.port;
    }
    if (existingSystemConfig.ai_provider) {
      document.getElementById('ai-provider').value = existingSystemConfig.ai_provider;
      onAIProviderChange();
    }
    if (existingSystemConfig.ai_base_url) {
      document.getElementById('ai-url').value = existingSystemConfig.ai_base_url;
    }
    if (existingSystemConfig.ai_model) {
      document.getElementById('ai-model').value = existingSystemConfig.ai_model;
    }
    if (existingSystemConfig.ai_api_key) {
      document.getElementById('ai-key').value = existingSystemConfig.ai_api_key;
    }
  }

  document.getElementById('control-section').classList.add('hidden');
  document.getElementById('wizard-section').classList.remove('hidden');
  goToStep(2);
}

async function runPreflight() {
  const wslEl = document.getElementById('status-wsl');
  const wslDetailEl = document.getElementById('wsl-detail');
  const wslActionBox = document.getElementById('wsl-action-box');

  const dockerEl = document.getElementById('status-docker');
  const dockerDetailEl = document.getElementById('docker-detail');
  const dockerActionBox = document.getElementById('docker-action-box');
  const btnInstallWsl = document.getElementById('btn-install-wsl');
  const btnInstallDocker = document.getElementById('btn-install-docker');
  const btnStartDocker = document.getElementById('btn-start-docker');

  const portsEl = document.getElementById('status-ports');
  const portsDetailEl = document.getElementById('ports-detail');
  const feedbackEl = document.getElementById('system-action-feedback');

  wslEl.className = 'status-badge warning';
  wslEl.textContent = 'Prüfe...';
  dockerEl.className = 'status-badge warning';
  dockerEl.textContent = 'Prüfe...';

  try {
    const res = await fetch('/api/preflight', { method: 'POST' });
    const status = await res.json();

    // 0. OS & Badge adaptation
    const isWindows = status.os === 'windows';
    const isDarwin = status.os === 'darwin';
    const isLinux = status.os === 'linux';

    const badge = document.getElementById('launcher-version-badge');
    if (badge && status.os) {
      const osLabel = isDarwin ? 'macOS' : (isLinux ? 'Linux' : 'Windows');
      const archLabel = status.arch === 'arm64' ? 'ARM64' : 'x86_64';
      badge.textContent = `v3.0 ${osLabel} (${archLabel})`;
    }

    const introText = document.getElementById('preflight-intro-text');
    if (introText) {
      if (isDarwin) {
        introText.textContent = 'Wir prüfen Ihr Mac-System auf Docker und Port-Verfügbarkeit.';
      } else if (isLinux) {
        introText.textContent = 'Wir prüfen Ihr Linux-System auf Docker und Port-Verfügbarkeit.';
      } else {
        introText.textContent = 'Wir prüfen Ihr Windows-System auf WSL2, Docker Desktop und Port-Verfügbarkeit.';
      }
    }

    // 1. WSL Check (only visible on Windows)
    const wslCard = document.getElementById('status-wsl-card');
    if (!isWindows && wslCard) {
      wslCard.style.display = 'none';
    } else if (wslCard) {
      wslCard.style.display = '';
      if (status.wsl_installed) {
        wslEl.className = 'status-badge healthy';
        wslEl.textContent = 'Bereit';
        wslDetailEl.textContent = 'WSL 2 ist verfügbar und betriebsbereit.';
        wslActionBox.classList.remove('hidden');
        if (btnInstallWsl) {
          btnInstallWsl.disabled = true;
          btnInstallWsl.textContent = '✅ WSL 2 bereits installiert';
          btnInstallWsl.title = 'WSL 2 ist bereits auf diesem Rechner installiert.';
        }
      } else {
        wslEl.className = 'status-badge danger';
        wslEl.textContent = 'Nicht installiert';
        wslDetailEl.textContent = status.wsl_status || 'WSL 2 wird für Docker benötigt.';
        wslActionBox.classList.remove('hidden');
        if (btnInstallWsl) {
          btnInstallWsl.disabled = false;
          btnInstallWsl.textContent = '🚀 WSL2 jetzt installieren (Admin UAC)';
          btnInstallWsl.title = '';
        }
      }
    }

    // 2. Docker Check
    if (status.docker_running) {
      dockerEl.className = 'status-badge healthy';
      dockerEl.textContent = 'Bereit (v' + (status.docker_version || 'active') + ')';
      dockerDetailEl.textContent = 'Docker Engine läuft einwandfrei.';
      dockerActionBox.classList.remove('hidden');
      if (btnInstallDocker) {
        if (isWindows) {
          btnInstallDocker.classList.remove('hidden');
          btnInstallDocker.disabled = true;
          btnInstallDocker.textContent = '✅ Docker bereits installiert';
        } else {
          btnInstallDocker.classList.add('hidden');
        }
      }
      if (btnStartDocker) {
        btnStartDocker.classList.add('hidden');
      }
    } else if (status.docker_installed) {
      dockerEl.className = 'status-badge warning';
      dockerEl.textContent = 'Gestoppt';
      dockerActionBox.classList.remove('hidden');

      if (isDarwin) {
        dockerDetailEl.textContent = 'Docker Desktop / OrbStack ist installiert, läuft aber aktuell nicht.';
        if (btnInstallDocker) btnInstallDocker.classList.add('hidden');
        if (btnStartDocker) {
          btnStartDocker.classList.remove('hidden');
          btnStartDocker.disabled = false;
          btnStartDocker.textContent = '▶️ Docker starten';
        }
      } else if (isLinux) {
        dockerDetailEl.textContent = 'Docker Engine läuft nicht. Starten Sie den Dienst z. B. mit "sudo systemctl start docker" oder prüfen Sie "sudo usermod -aG docker $USER".';
        if (btnInstallDocker) btnInstallDocker.classList.add('hidden');
        if (btnStartDocker) {
          btnStartDocker.classList.remove('hidden');
          btnStartDocker.disabled = false;
          btnStartDocker.textContent = '▶️ Docker starten';
        }
      } else {
        dockerDetailEl.textContent = 'Docker Desktop ist installiert, läuft aber aktuell nicht.';
        if (btnInstallDocker) {
          btnInstallDocker.classList.remove('hidden');
          btnInstallDocker.disabled = true;
          btnInstallDocker.textContent = '✅ Docker bereits installiert';
        }
        if (btnStartDocker) {
          btnStartDocker.classList.remove('hidden');
          btnStartDocker.disabled = false;
          btnStartDocker.textContent = '▶️ Docker starten';
        }
      }
    } else {
      dockerEl.className = 'status-badge danger';
      dockerEl.textContent = 'Nicht installiert';
      dockerActionBox.classList.remove('hidden');

      if (isDarwin) {
        dockerDetailEl.textContent = 'Docker Desktop wurde nicht gefunden. Installieren Sie Docker z. B. via "brew install --cask docker" oder https://docker.com';
        if (btnInstallDocker) btnInstallDocker.classList.add('hidden');
      } else if (isLinux) {
        dockerDetailEl.textContent = 'Docker Engine wurde nicht gefunden. Installieren Sie Docker via Paketmanager oder "curl -fsSL https://get.docker.com | sh".';
        if (btnInstallDocker) btnInstallDocker.classList.add('hidden');
      } else {
        dockerDetailEl.textContent = 'Docker Desktop wurde nicht gefunden.';
        if (btnInstallDocker) {
          btnInstallDocker.classList.remove('hidden');
          btnInstallDocker.disabled = false;
          btnInstallDocker.textContent = '🐳 Docker Desktop installieren (Admin UAC)';
        }
      }
      if (btnStartDocker) {
        btnStartDocker.classList.add('hidden');
      }
    }

    // 3. Ports Check
    if (status.port_80_free) {
      portsEl.className = 'status-badge healthy';
      portsEl.textContent = 'Port 80 frei';
      portsDetailEl.textContent = 'Standard-Web-Port 80 ist verfügbar.';
      document.getElementById('web-port').value = 80;
    } else if (status.port_8080_free) {
      portsEl.className = 'status-badge warning';
      portsEl.textContent = 'Port 8080 (Ausweich-Port)';
      portsDetailEl.textContent = 'Port 80 ist belegt. Port 8080 wird genutzt.';
      document.getElementById('web-port').value = 8080;
    } else {
      portsEl.className = 'status-badge warning';
      portsEl.textContent = 'Port 8000';
      portsDetailEl.textContent = 'Ports 80 und 8080 belegt. Verwende Port 8000.';
      document.getElementById('web-port').value = 8000;
    }
  } catch (err) {
    dockerEl.className = 'status-badge danger';
    dockerEl.textContent = 'Fehler';
    dockerDetailEl.textContent = err.message;
  }
}

function retryPreflight() {
  runPreflight();
}

async function installWSL() {
  const feedback = document.getElementById('system-action-feedback');
  feedback.style.color = '#38bdf8';
  feedback.textContent = '🚀 WSL2-Installation wird gestartet... Bitte bestätigen Sie den Windows Administrator-Dialog (UAC).';
  try {
    const res = await fetch('/api/install/wsl', { method: 'POST' });
    const data = await res.json();
    if (data.success) {
      feedback.textContent = '✅ WSL2-Befehl übergeben. Nach Abschluss Windows ggf. neu starten und erneut prüfen.';
      setTimeout(runPreflight, 5000);
    } else {
      feedback.style.color = '#f87171';
      feedback.textContent = '❌ ' + (data.error || 'WSL-Installation fehlgeschlagen.');
    }
  } catch (err) {
    feedback.style.color = '#f87171';
    feedback.textContent = '❌ Fehler: ' + err.message;
  }
}

async function installDocker() {
  const feedback = document.getElementById('system-action-feedback');
  feedback.style.color = '#38bdf8';
  feedback.textContent = '🐳 Docker Desktop Installation über winget gestartet... Bitte UAC-Dialog bestätigen.';
  try {
    const res = await fetch('/api/install/docker', { method: 'POST' });
    const data = await res.json();
    if (data.success) {
      feedback.textContent = '✅ winget-Installation läuft im Hintergrund. Nach Fertigstellung erneut prüfen.';
      setTimeout(runPreflight, 8000);
    } else {
      feedback.style.color = '#f87171';
      feedback.textContent = '❌ ' + (data.error || 'Docker-Installation fehlgeschlagen.');
    }
  } catch (err) {
    feedback.style.color = '#f87171';
    feedback.textContent = '❌ Fehler: ' + err.message;
  }
}

async function startDocker() {
  const feedback = document.getElementById('system-action-feedback');
  feedback.style.color = '#38bdf8';
  feedback.textContent = '▶️ Starte Docker Desktop... Bitte warten...';
  try {
    await fetch('/api/install/start-docker', { method: 'POST' });
    setTimeout(runPreflight, 6000);
  } catch (err) {
    feedback.style.color = '#f87171';
    feedback.textContent = '❌ Fehler: ' + err.message;
  }
}

function generatePassword() {
  const chars = 'abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!#%*';
  let pw = '';
  for (let i = 0; i < 16; i++) {
    pw += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  document.getElementById('admin-password').value = pw;
  savedAdminPassword = pw;
}

function goToStep(step) {
  for (let i = 1; i <= 4; i++) {
    const nav = document.getElementById('step-nav-' + i);
    const content = document.getElementById('step-content-' + i);
    if (nav) {
      if (i === step) nav.className = 'step-item active';
      else if (i < step) nav.className = 'step-item completed';
      else nav.className = 'step-item';
    }
    if (content) {
      if (i === step) content.classList.remove('hidden');
      else content.classList.add('hidden');
    }
  }
  currentStep = step;
}

async function uploadSetupBackup() {
  const fileInput = document.getElementById('setup-backup-file');
  const statusEl = document.getElementById('setup-backup-status');
  if (!fileInput.files || fileInput.files.length === 0) {
    statusEl.style.color = '#fbbf24';
    statusEl.textContent = 'Bitte wählen Sie zuerst eine .sql Datei aus.';
    return;
  }

  const formData = new FormData();
  formData.append('backup_file', fileInput.files[0]);

  statusEl.style.color = '#38bdf8';
  statusEl.textContent = 'Lade Backup hoch...';

  try {
    const res = await fetch('/api/backup/upload', {
      method: 'POST',
      body: formData
    });
    const data = await res.json();
    if (data.success) {
      uploadedBackupFilename = data.filename;
      statusEl.style.color = '#34d399';
      statusEl.textContent = `✅ Backup "${data.filename}" erfolgreich hochgeladen und für Import vorgemerkt!`;
    } else {
      statusEl.style.color = '#f87171';
      statusEl.textContent = '❌ Upload fehlgeschlagen: ' + (data.error || 'Unbekannter Fehler');
    }
  } catch (err) {
    statusEl.style.color = '#f87171';
    statusEl.textContent = '❌ Fehler beim Upload: ' + err.message;
  }
}

function onAIProviderChange() {
  const provider = document.getElementById('ai-provider').value;
  const box = document.getElementById('ai-settings-box');
  const keyGroup = document.getElementById('ai-key-group');
  const urlInput = document.getElementById('ai-url');
  const modelInput = document.getElementById('ai-model');

  if (provider === 'none') {
    box.classList.add('hidden');
  } else if (provider === 'ollama') {
    box.classList.remove('hidden');
    keyGroup.classList.add('hidden');
    urlInput.value = 'http://localhost:11434';
    modelInput.value = 'gemma2:12b';
  } else {
    box.classList.remove('hidden');
    keyGroup.classList.remove('hidden');
    urlInput.value = 'https://api.openai.com/v1';
    modelInput.value = 'gpt-4o-mini';
  }
}

async function testAIConnection() {
  const statusEl = document.getElementById('ai-probe-status');
  statusEl.style.color = '#fbbf24';
  statusEl.textContent = '⏳ Teste Verbindung...';

  const payload = {
    provider: document.getElementById('ai-provider').value,
    base_url: document.getElementById('ai-url').value,
    api_key: document.getElementById('ai-key').value,
    model: document.getElementById('ai-model').value
  };

  try {
    const res = await fetch('/api/ai/test', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });
    const data = await res.json();
    if (data.success) {
      statusEl.style.color = '#34d399';
      statusEl.textContent = '✅ ' + data.message;
    } else {
      statusEl.style.color = '#f87171';
      statusEl.textContent = '❌ ' + data.message;
    }
  } catch (err) {
    statusEl.style.color = '#f87171';
    statusEl.textContent = '❌ Fehler: ' + err.message;
  }
}

async function startSetup() {
  goToStep(3);

  configuredPort = parseInt(document.getElementById('web-port').value, 10) || 80;
  savedAdminPassword = document.getElementById('admin-password').value;

  const versionSelect = document.getElementById('install-version');
  const selectedVersion = versionSelect ? versionSelect.value : 'v1.0.0';

  const payload = {
    admin_email: document.getElementById('admin-email').value,
    admin_password: savedAdminPassword,
    port: configuredPort,
    is_demo_mode: false,
    ai_provider: document.getElementById('ai-provider').value,
    ai_base_url: document.getElementById('ai-url').value,
    ai_api_key: document.getElementById('ai-key').value,
    ai_model: document.getElementById('ai-model').value,
    version: selectedVersion
  };

  const terminal = document.getElementById('deployment-terminal');
  terminal.textContent = 'Sende Konfiguration & Starte Docker Compose...\n';
  startLogPolling();

  try {
    const res = await fetch('/api/setup', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    });

    if (!res.ok) {
      const err = await res.json();
      terminal.textContent += '\nFEHLER: ' + (err.error || 'Setup fehlgeschlagen');
      return;
    }

    terminal.textContent += '\nContainer-Erstellung im Gange. Prüfe Health-Status...\n';
    pollHealth(configuredPort);
  } catch (err) {
    terminal.textContent += '\nNetzwerkfehler: ' + err.message;
  }
}

let logPollInterval = null;

function startLogPolling() {
  if (logPollInterval) clearInterval(logPollInterval);
  const terminal = document.getElementById('deployment-terminal');
  const controlTerminal = document.getElementById('control-terminal');

  logPollInterval = setInterval(async () => {
    try {
      const res = await fetch('/api/logs');
      const data = await res.json();
      if (data.lines && data.lines.length > 0) {
        const text = data.lines.join('\n');
        if (terminal) {
          terminal.textContent = text;
          terminal.scrollTop = terminal.scrollHeight;
        }
        if (controlTerminal) {
          controlTerminal.textContent = text;
          controlTerminal.scrollTop = controlTerminal.scrollHeight;
        }
      }
    } catch (e) {
      // ignore
    }
  }, 1000);
}

function pollHealth(port) {
  if (healthCheckInterval) clearInterval(healthCheckInterval);
  let attempts = 0;
  const maxAttempts = 60; // 60 * 2s = 120s
  const info = document.getElementById('deploy-health-info');
  const spinner = document.getElementById('deploy-spinner');
  const errorActions = document.getElementById('deploy-error-actions');

  spinner.className = 'status-badge warning';
  spinner.textContent = '⏳ Startet Dienste...';
  errorActions.classList.add('hidden');

  healthCheckInterval = setInterval(async () => {
    attempts++;
    info.textContent = `Warte auf Server-Bereitschaft (${attempts * 2}s / 120s)...`;

    try {
      const res = await fetch('/api/status');
      const data = await res.json();

      const healthyServer = (data.containers || []).some(c => 
        (c.service === 'server' || c.service === 'crm-server' || c.name.includes('server')) &&
        (c.health === 'healthy' || c.state === 'running')
      );

      if (healthyServer) {
        clearInterval(healthCheckInterval);
        if (uploadedBackupFilename) {
          info.textContent = `Importiere Datenbank-Backup "${uploadedBackupFilename}"...`;
          await triggerRestoreAfterSetup(uploadedBackupFilename);
        }
        onSetupComplete(port);
        return;
      }
    } catch (e) {
      // Ignore network flutter during container setup
    }

    if (attempts >= maxAttempts) {
      clearInterval(healthCheckInterval);
      spinner.className = 'status-badge danger';
      spinner.textContent = '⚠️ Start verzögert / Fehler aufgetreten';
      info.textContent = 'Die Container konnten innerhalb von 120s nicht als betriebsbereit gemeldet werden. Bitte prüfen Sie die Logs oben.';
      errorActions.classList.remove('hidden');
    }
  }, 2000);
}

function retryHealthCheck() {
  pollHealth(configuredPort);
}

async function triggerRestoreAfterSetup(filename) {
  try {
    await fetch('/api/backup/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ filename })
    });
  } catch (e) {
    console.error('Failed to trigger auto-restore:', e);
  }
}

function onSetupComplete(port) {
  goToStep(4);
  const url = port === 80 ? 'http://localhost' : `http://localhost:${port}`;
  document.getElementById('success-url').textContent = url;
  document.getElementById('success-email').textContent = document.getElementById('admin-email').value;
  document.getElementById('success-password').textContent = savedAdminPassword;
}

function openCRM() {
  const base = configuredPort === 80 ? 'http://localhost' : `http://localhost:${configuredPort}`;
  const url = `${base}/login`;
  window.open(url, '_blank');
}

function switchToControlCenter() {
  document.getElementById('wizard-section').classList.add('hidden');
  document.getElementById('control-section').classList.remove('hidden');
  refreshControlStatus();
  fetchBackups();
  startLogPolling();
}

async function refreshControlStatus() {
  try {
    const res = await fetch('/api/status');
    const data = await res.json();
    if (data.existing_config) {
      existingSystemConfig = data.existing_config;
      configuredPort = data.existing_config.port || 80;
    }
    loadContainers(data.containers || []);
    if (data.installed) {
      const banner = document.getElementById('installed-detected-banner');
      const bannerText = document.getElementById('installed-detected-text');
      if (banner && bannerText) {
        banner.style.display = 'block';
        bannerText.textContent = `${data.reason || 'Installation aktiv'} | Port: ${configuredPort} | Admin: ${data.existing_config?.admin_email || 'admin@openlocalcrm.local'}`;
      }
    }
  } catch (e) {
    console.error('Failed refreshing control status', e);
  }
}

function loadContainers(containers) {
  const grid = document.getElementById('control-containers-grid');
  grid.innerHTML = '';

  if (!containers || containers.length === 0) {
    grid.innerHTML = '<p class="helper-text">Keine aktiven Container gefunden.</p>';
    return;
  }

  if (containers.length > 0 && !containers.some(c => c.state === 'running')) {
    const notice = document.createElement('div');
    notice.style.cssText = 'grid-column: 1 / -1; background: rgba(245, 158, 11, 0.1); border: 1px solid rgba(245, 158, 11, 0.3); border-radius: 8px; padding: 0.75rem 1rem; color: #f59e0b; margin-bottom: 0.5rem; font-size: 0.9rem;';
    notice.innerHTML = '⚠️ <strong>Container angehalten:</strong> Die OpenLocalCRM Container existieren, sind aber aktuell gestoppt. Klicken Sie unten auf <strong>"▶️ Stack starten"</strong>.';
    grid.appendChild(notice);
  }

  containers.forEach(c => {
    const card = document.createElement('div');
    card.className = 'status-card';
    const isHealthy = c.health === 'healthy' || c.state === 'running';
    card.innerHTML = `
      <h4>${c.service || c.name}</h4>
      <div class="status-badge ${isHealthy ? 'healthy' : 'danger'}">
        ● ${c.state} (${c.health || c.status})
      </div>
      <p class="helper-text">${c.name}</p>
    `;
    grid.appendChild(card);
  });
}

async function controlAction(action) {
  try {
    await fetch(`/api/control/${action}`, { method: 'POST' });
    setTimeout(refreshControlStatus, 2000);
  } catch (err) {
    alert(`Fehler bei Aktion ${action}: ` + err.message);
  }
}

async function updateContainers() {
  if (!confirm('Container-Update starten?\n\nEs wird automatisch vorab ein Sicherheits-Backup der Datenbank erstellt und anschließend werden alle Container neu gebaut (docker compose up -d --build).')) {
    return;
  }
  try {
    await fetch('/api/control/update', { method: 'POST' });
    setTimeout(refreshControlStatus, 3000);
    setTimeout(fetchBackups, 4000);
  } catch (err) {
    alert('Fehler beim Container-Update: ' + err.message);
  }
}

async function createBackup() {
  const resEl = document.getElementById('backup-result');
  resEl.textContent = 'Erstelle Sicherheits-Backup...';
  try {
    const res = await fetch('/api/backup', { method: 'POST' });
    const data = await res.json();
    if (data.success) {
      resEl.textContent = '✅ Backup erfolgreich erstellt: ' + data.filename;
      resEl.style.color = '#34d399';
      fetchBackups();
    } else {
      resEl.textContent = '❌ ' + data.error;
      resEl.style.color = '#f87171';
    }
  } catch (err) {
    resEl.textContent = '❌ Fehler: ' + err.message;
    resEl.style.color = '#f87171';
  }
}

async function fetchBackups() {
  const tbody = document.getElementById('backups-tbody');
  try {
    const res = await fetch('/api/backups');
    const data = await res.json();

    if (!data.backups || data.backups.length === 0) {
      tbody.innerHTML = '<tr><td colspan="4" class="helper-text">Noch keine Backups vorhanden. Klicken Sie oben auf "1-Klick Backup erstellen".</td></tr>';
      return;
    }

    tbody.innerHTML = '';
    data.backups.forEach(b => {
      const tr = document.createElement('tr');
      const dateStr = new Date(b.created_at).toLocaleString('de-DE');
      const schemaTag = b.schema_version ? `<br><span style="color: #94a3b8; font-size: 0.75rem;">Schema: ${b.schema_version}</span>` : '';
      tr.innerHTML = `
        <td style="font-family: monospace; font-size: 0.8rem;">${b.filename}${schemaTag}</td>
        <td>${b.size_formatted}</td>
        <td>${dateStr}</td>
        <td style="text-align: right; white-space: nowrap;">
          <a class="btn btn-outline btn-sm" href="/api/backup/download?file=${encodeURIComponent(b.filename)}" download="${b.filename}" style="text-decoration: none; margin-right: 0.35rem;">⬇️ Download</a>
          <button class="btn btn-secondary btn-sm" onclick="restoreBackup('${b.filename}')">Wiederherstellen</button>
        </td>
      `;
      tbody.appendChild(tr);
    });
  } catch (e) {
    tbody.innerHTML = '<tr><td colspan="4" style="color: #f87171;">Fehler beim Laden der Backups.</td></tr>';
  }
}

async function restoreBackup(filename) {
  if (!confirm(`Möchten Sie das Backup "${filename}" wirklich wiederherstellen?\n\nVor der Wiederherstellung wird zur Sicherheit automatisch ein aktueller Snapshot erstellt.`)) {
    return;
  }

  try {
    const res = await fetch('/api/backup/restore', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ filename })
    });
    const data = await res.json();
    if (data.success) {
      alert(`Wiederherstellung von "${filename}" gestartet! Verfolgen Sie den Fortschritt in den Live-Logs.`);
      setTimeout(refreshControlStatus, 3000);
      setTimeout(fetchBackups, 4000);
    } else {
      alert('Fehler: ' + (data.error || 'Restore fehlgeschlagen'));
    }
  } catch (err) {
    alert('Fehler beim Senden des Restore-Befehls: ' + err.message);
  }
}

async function uploadControlBackup() {
  const fileInput = document.getElementById('control-backup-file');
  if (!fileInput.files || fileInput.files.length === 0) return;

  const formData = new FormData();
  formData.append('backup_file', fileInput.files[0]);

  try {
    const res = await fetch('/api/backup/upload', {
      method: 'POST',
      body: formData
    });
    const data = await res.json();
    if (data.success) {
      alert(`Backup "${data.filename}" erfolgreich hochgeladen!`);
      fileInput.value = '';
      fetchBackups();
    } else {
      alert('Fehler beim Upload: ' + (data.error || 'Unbekannt'));
    }
  } catch (err) {
    alert('Fehler beim Upload: ' + err.message);
  }
}

function openResetPasswordModal() {
  const modal = document.getElementById('reset-password-modal');
  const emailInput = document.getElementById('reset-admin-email');
  const feedback = document.getElementById('reset-password-feedback');
  
  if (emailInput) {
    emailInput.value = existingSystemConfig?.admin_email || 'admin@openlocalcrm.local';
  }
  generateResetPassword();
  if (feedback) {
    feedback.textContent = '';
    feedback.style.color = '';
  }
  if (modal) {
    modal.style.display = 'flex';
  }
}

function closeResetPasswordModal() {
  const modal = document.getElementById('reset-password-modal');
  if (modal) {
    modal.style.display = 'none';
  }
}

function generateResetPassword() {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%&*';
  let pw = '';
  const cryptoObj = window.crypto || window.msCrypto;
  const values = new Uint8Array(14);
  if (cryptoObj) {
    cryptoObj.getRandomValues(values);
    for (let i = 0; i < 14; i++) {
      pw += chars[values[i] % chars.length];
    }
  } else {
    for (let i = 0; i < 14; i++) {
      pw += chars.charAt(Math.floor(Math.random() * chars.length));
    }
  }
  const input = document.getElementById('reset-admin-password');
  if (input) {
    input.value = pw;
  }
}

async function submitResetPassword() {
  const email = document.getElementById('reset-admin-email').value.trim();
  const password = document.getElementById('reset-admin-password').value.trim();
  const feedback = document.getElementById('reset-password-feedback');
  const submitBtn = document.getElementById('btn-submit-reset-password');

  if (!email) {
    feedback.textContent = '❌ Bitte geben Sie eine gültige Administrator-E-Mail ein.';
    feedback.style.color = '#f87171';
    return;
  }
  if (!password || password.length < 6) {
    feedback.textContent = '❌ Das Passwort muss mindestens 6 Zeichen lang sein.';
    feedback.style.color = '#f87171';
    return;
  }

  submitBtn.disabled = true;
  feedback.textContent = '⏳ Setze Passwort in PostgreSQL zurück...';
  feedback.style.color = '#38bdf8';

  try {
    const res = await fetch('/api/admin/reset-password', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email, password })
    });
    const data = await res.json();
    submitBtn.disabled = false;

    if (data.success) {
      feedback.innerHTML = `✅ <strong>Erfolgreich zurückgesetzt!</strong><br>Konto: <code>${email}</code><br>Neues Kennwort: <code style="font-size:1.05rem; color:#34d399;">${password}</code><br><span style="font-size:0.8rem; color:#94a3b8;">Sie können sich jetzt mit diesen Daten im CRM anmelden.</span>`;
      feedback.style.color = '#34d399';
      if (existingSystemConfig) {
        existingSystemConfig.admin_email = email;
        existingSystemConfig.admin_password = password;
      }
      setTimeout(refreshControlStatus, 1500);
    } else {
      feedback.textContent = '❌ ' + (data.error || 'Fehler beim Zurücksetzen.');
      feedback.style.color = '#f87171';
    }
  } catch (err) {
    submitBtn.disabled = false;
    feedback.textContent = '❌ Netzwerkfehler: ' + err.message;
    feedback.style.color = '#f87171';
  }
}

// FACTORY RESET MODAL & ACTION
function openResetModal() {
  const modal = document.getElementById('reset-modal');
  if (!modal) return;
  modal.style.display = 'flex';
  modal.classList.remove('hidden');

  const checkbox = document.getElementById('reset-confirm-checkbox');
  if (checkbox) checkbox.checked = false;

  const btnConfirm = document.getElementById('btn-confirm-reset');
  if (btnConfirm) btnConfirm.disabled = true;

  const btnCancel = document.getElementById('btn-cancel-reset');
  if (btnCancel) btnCancel.disabled = false;

  const feedback = document.getElementById('reset-progress-feedback');
  if (feedback) {
    feedback.textContent = '';
    feedback.style.color = '';
  }
}

function closeResetModal() {
  const modal = document.getElementById('reset-modal');
  if (!modal) return;
  modal.style.display = 'none';
  modal.classList.add('hidden');
}

function onResetCheckboxChange(checked) {
  const btnConfirm = document.getElementById('btn-confirm-reset');
  if (btnConfirm) {
    btnConfirm.disabled = !checked;
  }
}

async function executeFactoryReset() {
  const btnConfirm = document.getElementById('btn-confirm-reset');
  const btnCancel = document.getElementById('btn-cancel-reset');
  const feedback = document.getElementById('reset-progress-feedback');

  if (btnConfirm) btnConfirm.disabled = true;
  if (btnCancel) btnCancel.disabled = true;
  if (feedback) {
    feedback.style.color = '#fbbf24';
    feedback.textContent = '⏳ Lösche alle Docker-Container, Volumes und Konfigurationsdateien...';
  }

  try {
    const res = await fetch('/api/control/reset', { method: 'POST' });
    const data = await res.json();

    if (data.success) {
      if (feedback) {
        feedback.style.color = '#34d399';
        feedback.textContent = '✅ Factory Reset erfolgreich! Setze System sauber neu auf...';
      }

      setTimeout(() => {
        closeResetModal();
        existingSystemConfig = null;

        // Hide Control Center, show Wizard
        document.getElementById('control-section').classList.add('hidden');
        document.getElementById('wizard-section').classList.remove('hidden');

        // Hide existing installation banners
        const wizBanner = document.getElementById('wizard-already-installed-banner');
        if (wizBanner) wizBanner.classList.add('hidden');
        const instBanner = document.getElementById('installed-detected-banner');
        if (instBanner) instBanner.style.display = 'none';

        // Reset inputs to default values
        document.getElementById('admin-email').value = 'admin@openlocalcrm.local';
        document.getElementById('web-port').value = 80;
        document.getElementById('ai-provider').value = 'ollama';
        onAIProviderChange();
        generatePassword();

        // Clear deployment terminal
        const term = document.getElementById('deployment-terminal');
        if (term) term.textContent = 'System wurde vollständig zurückgesetzt. Bereit zur Neuinstallation.\n';

        // Go to Step 1 and run preflight
        goToStep(1);
        runPreflight();
      }, 1200);
    } else {
      if (feedback) {
        feedback.style.color = '#f87171';
        feedback.textContent = '❌ Fehler beim Zurücksetzen: ' + (data.error || 'Unbekannter Fehler');
      }
      if (btnCancel) btnCancel.disabled = false;
      if (btnConfirm) btnConfirm.disabled = false;
    }
  } catch (err) {
    if (feedback) {
      feedback.style.color = '#f87171';
      feedback.textContent = '❌ Netzwerkfehler: ' + err.message;
    }
    if (btnCancel) btnCancel.disabled = false;
    if (btnConfirm) btnConfirm.disabled = false;
  }
}

// ==========================================
// GITHUB AUTO-UPDATE & HOT-SWAP LOGIC
// ==========================================

let updateInfo = null;

async function checkUpdatesManually() {
  await checkForUpdates(true);
}

async function checkForUpdates(manual = false) {
  const btn = document.getElementById('btn-check-updates');
  const badgeBtn = document.getElementById('btn-update-available');
  if (btn && manual) {
    btn.disabled = true;
    btn.textContent = '⏳ Prüfe GitHub auf Updates...';
  }

  try {
    const res = await fetch('/api/update/check');
    const data = await res.json();
    updateInfo = data;

    if (data.has_update) {
      if (badgeBtn) {
        badgeBtn.classList.remove('hidden');
        badgeBtn.style.display = 'inline-block';
      }
      if (manual) {
        openUpdateModal();
      }
    } else {
      if (badgeBtn) {
        badgeBtn.classList.add('hidden');
        badgeBtn.style.display = 'none';
      }
      if (manual) {
        alert(data.rate_limited 
          ? 'GitHub Rate-Limit erreicht. Bitte versuchen Sie es in wenigen Minuten erneut.' 
          : 'OpenLocalCRM ist bereits auf dem neuesten Stand (' + (data.current_commit || 'aktuell') + ').');
      }
    }
  } catch (err) {
    console.error('Update check failed:', err);
    if (manual) {
      alert('Konnte GitHub nicht erreichen: ' + err.message);
    }
  } finally {
    if (btn && manual) {
      btn.disabled = false;
      btn.textContent = '⚡ Nach GitHub-Updates suchen';
    }
  }
}

function openUpdateModal() {
  const modal = document.getElementById('update-modal');
  if (!modal) return;
  modal.classList.remove('hidden');
  modal.style.display = 'flex';

  const curCommitEl = document.getElementById('update-current-commit');
  const newCommitEl = document.getElementById('update-latest-commit');
  const msgEl = document.getElementById('update-commit-msg');
  const feedback = document.getElementById('update-progress-feedback');

  if (curCommitEl) curCommitEl.textContent = updateInfo?.current_commit || 'Lokal';
  if (newCommitEl) newCommitEl.textContent = updateInfo?.latest_commit ? updateInfo.latest_commit.substring(0, 7) : 'Neueste Version';
  if (msgEl) msgEl.textContent = updateInfo?.commit_message || 'Neues Update von GitHub';
  if (feedback) {
    feedback.textContent = '';
    feedback.style.color = '';
  }

  const btn = document.getElementById('btn-confirm-update');
  if (btn) btn.disabled = false;
}

function closeUpdateModal() {
  const modal = document.getElementById('update-modal');
  if (!modal) return;
  modal.classList.add('hidden');
  modal.style.display = 'none';
}

async function executeSystemUpdate() {
  const btn = document.getElementById('btn-confirm-update');
  const cancelBtn = document.getElementById('btn-cancel-update');
  const feedback = document.getElementById('update-progress-feedback');

  if (btn) btn.disabled = true;
  if (cancelBtn) cancelBtn.disabled = true;
  if (feedback) {
    feedback.textContent = '🚀 Update wird ausgeführt... Bitte warten (Archiv wird heruntergeladen und entpackt)...';
    feedback.style.color = '#38bdf8';
  }

  try {
    const res = await fetch('/api/update/execute', { method: 'POST' });
    const data = await res.json();
    if (!data.success) {
      throw new Error(data.error || 'Update konnte nicht gestartet werden');
    }

    if (feedback) {
      feedback.textContent = '🔄 Systemdateien und Launcher aktualisiert! Launcher startet neu, verbinde neu...';
      feedback.style.color = '#34d399';
    }

    // Wait 2.5 seconds, then poll for restart
    setTimeout(pollForRestartAndReload, 2500);
  } catch (err) {
    if (feedback) {
      feedback.textContent = 'Fehler beim Starten des Updates: ' + err.message;
      feedback.style.color = '#f87171';
    }
    if (btn) btn.disabled = false;
    if (cancelBtn) cancelBtn.disabled = false;
  }
}

function pollForRestartAndReload() {
  const feedback = document.getElementById('update-progress-feedback');
  let attempts = 0;
  const maxAttempts = 30;

  const interval = setInterval(async () => {
    attempts++;
    if (feedback) {
      feedback.textContent = `Warte auf Neustart des Launchers (Versuch ${attempts}/${maxAttempts})...`;
    }

    try {
      const res = await fetch('/api/status', { cache: 'no-store' });
      if (res.ok) {
        clearInterval(interval);
        if (feedback) {
          feedback.textContent = '✅ Launcher erfolgreich neu gestartet! Lade Seite neu...';
        }
        setTimeout(() => {
          window.location.reload();
        }, 800);
      }
    } catch (e) {
      // Still restarting, retry
    }

    if (attempts >= maxAttempts) {
      clearInterval(interval);
      if (feedback) {
        feedback.textContent = 'Der Neustart dauert länger als erwartet. Bitte aktualisieren Sie die Seite manuell.';
        feedback.style.color = '#f59e0b';
      }
    }
  }, 1500);
}

async function loadAvailableVersions() {
  const select = document.getElementById('install-version');
  const helper = document.getElementById('version-helper-text');
  if (!select) return;

  try {
    const res = await fetch('/api/versions');
    if (!res.ok) return;
    const data = await res.json();
    if (!data.versions || data.versions.length === 0) return;

    const currentVal = select.value;
    select.innerHTML = '';

    data.versions.forEach(v => {
      const opt = document.createElement('option');
      opt.value = v.tag;
      opt.textContent = v.name;
      select.appendChild(opt);
    });

    const preferred = existingSystemConfig?.version || currentVal || data.current_version;
    if (preferred && Array.from(select.options).some(o => o.value === preferred)) {
      select.value = preferred;
    } else if (data.versions.length > 0) {
      select.value = data.versions[0].tag;
    }

    if (helper) {
      updateVersionHelperText();
    }
  } catch (err) {
    console.warn('Fehler beim Laden der Versionen:', err);
  }
}

function updateVersionHelperText() {
  const select = document.getElementById('install-version');
  const helper = document.getElementById('version-helper-text');
  if (!select || !helper) return;

  const raw = (select.value || '').trim().toLowerCase();
  const val = raw.replace(/^v/, '');
  const major = parseInt(val.split('.')[0], 10);

  if (raw === 'main' || raw === 'master' || raw === 'edge' || (!isNaN(major) && major >= 1)) {
    helper.textContent = '⚡ Schnelle Bereitstellung: Vorkompilierte GitHub Container Images (GHCR) verfügbar.';
    helper.style.color = '#38bdf8';
  } else {
    helper.textContent = '🔨 Legacy-Release (< v1.0): Lokaler Container-Build aus Quellcode erforderlich.';
    helper.style.color = '#fbbf24';
  }
}

