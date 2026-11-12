import React, { useEffect, useState } from 'react';
import {
  ShieldCheck,
  Database,
  HardDrive,
  Cloud,
  Bell,
  CalendarClock,
  CheckCircle2,
  ChevronRight,
  ChevronLeft,
  FolderOpen,
  RefreshCw,
} from 'lucide-react';
import { apiCall, DriveStatus, formatBytes, nativeBridge, ServerUserSession, setAuthToken } from '../lib/bridge';
import { useApp } from '../context/AppContext';
import { DriveFolderModal } from '../components/DriveFolderModal';

interface Props {
  onComplete: () => void;
}

export const SetupWizard: React.FC<Props> = ({ onComplete }) => {
  const { lang, setLang, theme, setTheme, t } = useApp();
  const [step, setStep] = useState(1);
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);

  // Step 1: Security
  const [pin, setPin] = useState('');
  const [pinConfirm, setPinConfirm] = useState('');

  // Step 2: Postgres
  const [pgHost, setPgHost] = useState('127.0.0.1');
  const [pgPort, setPgPort] = useState(5432);
  const [pgUser, setPgUser] = useState('postgres');
  const [pgPassword, setPgPassword] = useState('');
  const [pgDumpPath, setPgDumpPath] = useState('');
  const [detectedBins, setDetectedBins] = useState<{ path: string; version: string }[]>([]);
  const [pgTestedVersion, setPgTestedVersion] = useState('');
  const [previewDBs, setPreviewDBs] = useState<{ name: string; sizeBytes: number }[]>([]);

  // Step 3: Local storage
  const [localRoot, setLocalRoot] = useState('C:\\ProgramData\\PGuardian\\backups');
  const [minFreeGB, setMinFreeGB] = useState(5);

  // Step 4: Google Drive
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [driveStatus, setDriveStatus] = useState<DriveStatus | null>(null);
  const [folderModalOpen, setFolderModalOpen] = useState(false);

  // Step 5: Alerts & RDP
  const [tgToken, setTgToken] = useState('');
  const [tgChatId, setTgChatId] = useState('');
  const [tgChatLabel, setTgChatLabel] = useState('Admin');
  const [detectedChats, setDetectedChats] = useState<{ chatId: string; title: string }[]>([]);
  const [winUsers, setWinUsers] = useState<ServerUserSession[]>([]);
  const [selectedWinUsers, setSelectedWinUsers] = useState<string[]>([]);

  // Step 6: Initial Backup Profile
  const [profName, setProfName] = useState('Daily Nightly Backup');
  const [profCron, setProfCron] = useState('0 2 * * *');
  const [profFormat, setProfFormat] = useState<'custom' | 'plain_gz'>('custom');
  const [selMode, setSelMode] = useState<'manual' | 'all_auto'>('manual');
  const [selectedDBs, setSelectedDBs] = useState<string[]>([]);
  const [localKeep, setLocalKeep] = useState(7);
  const [driveKeep, setDriveKeep] = useState(14);

  useEffect(() => {
    void (async () => {
      try {
        const bins = await apiCall<{ path: string; version: string }[]>('GET', '/api/v1/postgres/discover');
        setDetectedBins(bins);
        if (bins.length > 0) setPgDumpPath(bins[0].path);
      } catch {}
      try {
        const users = await apiCall<ServerUserSession[]>('GET', '/api/v1/windows-users');
        setWinUsers(users);
        const activeAdmins = users.filter((u) => u.connectionStat === 'Active').map((u) => u.username);
        setSelectedWinUsers(activeAdmins);
      } catch {}
      try {
        const ds = await apiCall<DriveStatus>('GET', '/api/v1/cloud/status');
        setDriveStatus(ds);
      } catch {}
    })();
  }, []);

  const refreshDriveStatus = async () => {
    try {
      const ds = await apiCall<DriveStatus>('GET', '/api/v1/cloud/status');
      setDriveStatus(ds);
    } catch {}
  };

  const testPGAndLoadDBs = async () => {
    setBusy(true);
    setErr('');
    try {
      const res = await apiCall<{ version: string }>('POST', '/api/v1/postgres/test', {
        host: pgHost,
        port: pgPort,
        username: pgUser,
        password: pgPassword,
      });
      setPgTestedVersion(res.version);
      const dbs = await apiCall<{ name: string; sizeBytes: number }[]>('POST', '/api/v1/postgres/databases-preview', {
        host: pgHost,
        port: pgPort,
        username: pgUser,
        password: pgPassword,
      });
      setPreviewDBs(dbs);
      setSelectedDBs(dbs.filter((d) => d.name !== 'postgres').map((d) => d.name));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleConnectDrive = async () => {
    setBusy(true);
    setErr('');
    try {
      await apiCall('POST', '/api/v1/cloud/credentials', { clientId, clientSecret });
      const res = await apiCall<{ authUrl: string }>('POST', '/api/v1/cloud/oauth-start');
      await nativeBridge.openURL(res.authUrl);
      // Poll briefly for browser completion
      const poll = setInterval(async () => {
        const ds = await apiCall<DriveStatus>('GET', '/api/v1/cloud/status');
        setDriveStatus(ds);
        if (ds.connected) clearInterval(poll);
      }, 2000);
      setTimeout(() => clearInterval(poll), 180000);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const handleDetectTelegram = async () => {
    setErr('');
    try {
      const chats = await apiCall<{ chatId: string; title: string }[]>('POST', '/api/v1/notifications/telegram-detect', {
        botToken: tgToken,
      });
      setDetectedChats(chats);
      if (chats.length > 0) {
        setTgChatId(chats[0].chatId);
        setTgChatLabel(chats[0].title || 'Admin');
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  const handleNext = () => {
    setErr('');
    if (step === 1) {
      if (pin.trim().length < 4 || pin !== pinConfirm) {
        setErr(t('pinMismatch'));
        return;
      }
    }
    setStep((s) => Math.min(6, s + 1));
  };

  const handleFinish = async () => {
    setBusy(true);
    setErr('');
    try {
      const res = await apiCall<{ token: string }>('POST', '/api/v1/setup/complete', {
        language: lang,
        theme,
        masterPassword: pin,
        localBackupRoot: localRoot,
        minFreeDiskGb: minFreeGB,
        server: {
          name: 'PostgreSQL Principal',
          host: pgHost,
          port: pgPort,
          username: pgUser,
          password: pgPassword,
          pgDumpPath,
          isDefault: true,
        },
        initialProfile: {
          name: profName,
          enabled: true,
          scheduleCron: profCron,
          scheduleLabel: profCron,
          format: profFormat,
          selectionMode: selMode,
          includedDatabases: selMode === 'manual' ? selectedDBs : [],
          excludedDatabases: [],
          localRetentionMode: 'count',
          localRetentionValue: localKeep,
          driveEnabled: Boolean(driveStatus?.connected),
          driveFolderId: driveStatus?.rootFolderId || '',
          driveFolderPath: driveStatus?.rootFolderName || '',
          driveRetentionMode: 'count',
          driveRetentionValue: driveKeep,
        },
        telegramBotToken: tgToken,
        telegramChatId: tgChatId,
        telegramChatLabel: tgChatLabel,
        windowsUsers: selectedWinUsers,
      });
      setAuthToken(res.token);
      onComplete();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const stepsMeta = [
    { num: 1, label: t('wizStep1'), icon: ShieldCheck },
    { num: 2, label: t('wizStep2'), icon: Database },
    { num: 3, label: t('wizStep3'), icon: HardDrive },
    { num: 4, label: t('wizStep4'), icon: Cloud },
    { num: 5, label: t('wizStep5'), icon: Bell },
    { num: 6, label: t('wizStep6'), icon: CalendarClock },
  ];

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950 p-6 flex flex-col items-center justify-center">
      <div className="w-full max-w-4xl rounded-3xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 shadow-2xl overflow-hidden">
        <div className="px-8 py-6 bg-gradient-to-r from-slate-900 via-slate-900 to-cyan-950 text-white flex items-center justify-between">
          <div className="flex items-center gap-4">
            <img src="/logo.png" alt="PGuardian" className="w-12 h-12 rounded-xl shadow" />
            <div>
              <h1 className="text-xl font-bold">{t('wizTitle')}</h1>
              <p className="text-xs text-cyan-300/80">{t('appSubtitle')}</p>
            </div>
          </div>
          <span className="text-xs px-3 py-1 rounded-full bg-white/10 font-medium">
            {step} / 6
          </span>
        </div>

        <div className="grid grid-cols-6 border-b border-slate-200 dark:border-slate-800 bg-slate-50/80 dark:bg-slate-950/50">
          {stepsMeta.map((s) => {
            const Icon = s.icon;
            const active = step === s.num;
            const done = step > s.num;
            return (
              <div
                key={s.num}
                className={`px-3 py-3 flex items-center gap-2 text-xs font-medium border-b-2 transition ${
                  active
                    ? 'border-cyan-500 text-cyan-600 dark:text-cyan-400'
                    : done
                    ? 'border-emerald-500 text-emerald-600 dark:text-emerald-400'
                    : 'border-transparent text-slate-400'
                }`}
              >
                <Icon className="w-4 h-4 shrink-0" />
                <span className="truncate">{s.label}</span>
              </div>
            );
          })}
        </div>

        <div className="p-8 min-h-[370px]">
          {err && (
            <div className="mb-5 p-3.5 rounded-xl bg-red-500/10 border border-red-500/30 text-red-500 text-xs font-medium">
              {err}
            </div>
          )}

          {step === 1 && (
            <div className="space-y-6 max-w-lg mx-auto">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-semibold mb-1.5">Idioma / Language</label>
                  <select
                    value={lang}
                    onChange={(e) => setLang(e.target.value as 'es' | 'en')}
                    className="w-full px-3.5 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                  >
                    <option value="es">Español</option>
                    <option value="en">English</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-semibold mb-1.5">Tema Visual / Theme</label>
                  <select
                    value={theme}
                    onChange={(e) => setTheme(e.target.value as 'system' | 'dark' | 'light')}
                    className="w-full px-3.5 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                  >
                    <option value="system">{t('themeSystem')}</option>
                    <option value="dark">{t('themeDark')}</option>
                    <option value="light">{t('themeLight')}</option>
                  </select>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold mb-1.5">{t('masterPinLabel')}</label>
                <input
                  type="password"
                  value={pin}
                  onChange={(e) => setPin(e.target.value)}
                  className="w-full px-3.5 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                />
              </div>
              <div>
                <label className="block text-xs font-semibold mb-1.5">{t('confirmPinLabel')}</label>
                <input
                  type="password"
                  value={pinConfirm}
                  onChange={(e) => setPinConfirm(e.target.value)}
                  className="w-full px-3.5 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                />
              </div>
            </div>
          )}

          {step === 2 && (
            <div className="space-y-4">
              <div className="grid grid-cols-3 gap-4">
                <div className="col-span-2">
                  <label className="block text-xs font-semibold mb-1">Host</label>
                  <input
                    value={pgHost}
                    onChange={(e) => setPgHost(e.target.value)}
                    className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold mb-1">Port</label>
                  <input
                    type="number"
                    value={pgPort}
                    onChange={(e) => setPgPort(Number(e.target.value))}
                    className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                  />
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs font-semibold mb-1">Username</label>
                  <input
                    value={pgUser}
                    onChange={(e) => setPgUser(e.target.value)}
                    className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold mb-1">Password</label>
                  <input
                    type="password"
                    value={pgPassword}
                    onChange={(e) => setPgPassword(e.target.value)}
                    className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                  />
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold mb-1">Ruta de pg_dump.exe</label>
                <div className="flex gap-2">
                  <input
                    value={pgDumpPath}
                    onChange={(e) => setPgDumpPath(e.target.value)}
                    className="flex-1 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
                  />
                  {detectedBins.length > 0 && (
                    <select
                      onChange={(e) => setPgDumpPath(e.target.value)}
                      value={pgDumpPath}
                      className="px-2 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-slate-100 dark:bg-slate-800 text-xs"
                    >
                      {detectedBins.map((b) => (
                        <option key={b.path} value={b.path}>
                          {b.version}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              </div>

              <div className="flex items-center gap-4 pt-2">
                <button
                  type="button"
                  onClick={testPGAndLoadDBs}
                  disabled={busy}
                  className="px-4 py-2.5 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold"
                >
                  {t('srvTestConn')}
                </button>
                {pgTestedVersion && (
                  <span className="text-xs text-emerald-600 dark:text-emerald-400 flex items-center gap-1.5 font-medium">
                    <CheckCircle2 className="w-4 h-4" /> {pgTestedVersion.slice(0, 55)} ({previewDBs.length} DBs)
                  </span>
                )}
              </div>
            </div>
          )}

          {step === 3 && (
            <div className="space-y-5 max-w-xl mx-auto">
              <div>
                <label className="block text-xs font-semibold mb-1.5">
                  {lang === 'es' ? 'Carpeta Raíz para Copias Locales' : 'Root Directory for Local Backups'}
                </label>
                <div className="flex gap-2">
                  <input
                    value={localRoot}
                    onChange={(e) => setLocalRoot(e.target.value)}
                    className="flex-1 px-3.5 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm font-mono"
                  />
                  <button
                    type="button"
                    onClick={async () => {
                      const picked = await nativeBridge.pickDirectory('Backup Directory', localRoot);
                      if (picked) setLocalRoot(picked);
                    }}
                    className="px-3.5 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium flex items-center gap-1.5"
                  >
                    <FolderOpen className="w-4 h-4" /> {t('browse')}
                  </button>
                </div>
              </div>
              <div>
                <label className="block text-xs font-semibold mb-1.5">
                  {lang === 'es' ? 'Umbral de alerta por poco espacio libre (GB)' : 'Low Free Disk Alert Threshold (GB)'}
                </label>
                <input
                  type="number"
                  min={1}
                  value={minFreeGB}
                  onChange={(e) => setMinFreeGB(Number(e.target.value))}
                  className="w-36 px-3.5 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                />
              </div>
            </div>
          )}

          {step === 4 && (
            <div className="space-y-4 max-w-xl mx-auto">
              <p className="text-xs text-slate-500">
                {lang === 'es'
                  ? 'Ingresa tu Client ID y Client Secret de Google Cloud Console (tipo Aplicación de Escritorio). También puedes configurarlo más tarde.'
                  : 'Enter your Google Cloud Console Desktop OAuth Client ID and Client Secret. You can also configure this later.'}
              </p>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-semibold mb-1">Google OAuth Client ID</label>
                  <input
                    value={clientId}
                    onChange={(e) => setClientId(e.target.value)}
                    placeholder="...apps.googleusercontent.com"
                    className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                  />
                </div>
                <div>
                  <label className="block text-xs font-semibold mb-1">Client Secret</label>
                  <input
                    type="password"
                    value={clientSecret}
                    onChange={(e) => setClientSecret(e.target.value)}
                    placeholder="GOCSPX-..."
                    className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                  />
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-3 pt-1">
                <button
                  type="button"
                  disabled={!clientId || !clientSecret || busy}
                  onClick={handleConnectDrive}
                  className="px-4 py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold disabled:opacity-50"
                >
                  {t('cloudLoginBrowser')}
                </button>
                <button
                  type="button"
                  onClick={refreshDriveStatus}
                  className="p-2 rounded-xl border border-slate-300 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800"
                >
                  <RefreshCw className="w-4 h-4" />
                </button>
                {driveStatus?.connected && (
                  <span className="text-xs text-emerald-600 dark:text-emerald-400 font-semibold flex items-center gap-1.5">
                    <CheckCircle2 className="w-4 h-4" /> {driveStatus.accountEmail}
                  </span>
                )}
              </div>

              {driveStatus?.connected && (
                <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-950 border border-slate-200 dark:border-slate-800 flex items-center justify-between">
                  <div className="text-xs">
                    <span className="text-slate-500 block">Drive Folder:</span>
                    <span className="font-semibold">{driveStatus.rootFolderName || '/My Drive (Root)'}</span>
                  </div>
                  <button
                    type="button"
                    onClick={() => setFolderModalOpen(true)}
                    className="px-3.5 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold"
                  >
                    {t('cloudChooseFolder')}
                  </button>
                </div>
              )}
            </div>
          )}

          {step === 5 && (
            <div className="grid grid-cols-2 gap-6">
              <div className="space-y-3">
                <h3 className="text-sm font-semibold">{t('notifTelegramCard')}</h3>
                <input
                  type="password"
                  value={tgToken}
                  onChange={(e) => setTgToken(e.target.value)}
                  placeholder="Telegram Bot Token (123456:ABC...)"
                  className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                />
                <div className="flex gap-2">
                  <input
                    value={tgChatId}
                    onChange={(e) => setTgChatId(e.target.value)}
                    placeholder="Chat ID (ej. 123456789 o -100...)"
                    className="flex-1 px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                  />
                  <button
                    type="button"
                    disabled={!tgToken}
                    onClick={handleDetectTelegram}
                    className="px-3 py-2 rounded-xl bg-slate-200 dark:bg-slate-800 text-xs font-medium disabled:opacity-50"
                  >
                    {t('notifDetectChats')}
                  </button>
                </div>
                {detectedChats.length > 0 && (
                  <div className="text-xs text-emerald-500">
                    Chat detectado: {detectedChats[0].title} ({detectedChats[0].chatId})
                  </div>
                )}
              </div>

              <div className="space-y-2">
                <h3 className="text-sm font-semibold">{t('notifRdpUsersCard')}</h3>
                <p className="text-[11px] text-slate-500">{t('notifRdpDesc')}</p>
                <div className="max-h-44 overflow-y-auto rounded-xl border border-slate-200 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800">
                  {winUsers.map((u) => {
                    const checked = selectedWinUsers.includes(u.username);
                    return (
                      <label key={u.username} className="px-3 py-2 flex items-center justify-between text-xs cursor-pointer hover:bg-slate-50 dark:hover:bg-slate-800/50">
                        <span>
                          <b>{u.shortName}</b>{' '}
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-800 ml-1">
                            {u.sessionType} • {u.connectionStat}
                          </span>
                        </span>
                        <input
                          type="checkbox"
                          checked={checked}
                          onChange={(e) =>
                            setSelectedWinUsers((prev) =>
                              e.target.checked ? [...prev, u.username] : prev.filter((x) => x !== u.username),
                            )
                          }
                          className="rounded accent-cyan-600"
                        />
                      </label>
                    );
                  })}
                </div>
              </div>
            </div>
          )}

          {step === 6 && (
            <div className="grid grid-cols-2 gap-6">
              <div className="space-y-3">
                <div>
                  <label className="block text-xs font-semibold mb-1">{t('profName')}</label>
                  <input
                    value={profName}
                    onChange={(e) => setProfName(e.target.value)}
                    className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-sm"
                  />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-semibold mb-1">{t('profSchedulePreset')}</label>
                    <select
                      value={profCron}
                      onChange={(e) => setProfCron(e.target.value)}
                      className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                    >
                      <option value="0 */2 * * *">Cada 2 horas / Every 2h</option>
                      <option value="0 */6 * * *">Cada 6 horas / Every 6h</option>
                      <option value="0 2 * * *">Diario 02:00 AM / Daily 02:00</option>
                      <option value="0 13,21 * * *">13:00 & 21:00</option>
                    </select>
                  </div>
                  <div>
                    <label className="block text-xs font-semibold mb-1">{t('profFormat')}</label>
                    <select
                      value={profFormat}
                      onChange={(e) => setProfFormat(e.target.value as 'custom' | 'plain_gz')}
                      className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                    >
                      <option value="custom">Custom (-Fc .backup)</option>
                      <option value="plain_gz">SQL Gzip (.sql.gz)</option>
                    </select>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <label className="block text-xs font-semibold mb-1">{t('profLocalRet')} (copias)</label>
                    <input
                      type="number"
                      min={1}
                      value={localKeep}
                      onChange={(e) => setLocalKeep(Number(e.target.value))}
                      className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                    />
                  </div>
                  <div>
                    <label className="block text-xs font-semibold mb-1">{t('profDriveRet')} (copias)</label>
                    <input
                      type="number"
                      min={1}
                      value={driveKeep}
                      onChange={(e) => setDriveKeep(Number(e.target.value))}
                      className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                    />
                  </div>
                </div>
              </div>

              <div className="space-y-2">
                <label className="block text-xs font-semibold">{t('profSelectionMode')}</label>
                <select
                  value={selMode}
                  onChange={(e) => setSelMode(e.target.value as 'manual' | 'all_auto')}
                  className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs"
                >
                  <option value="manual">{t('profModeManual')}</option>
                  <option value="all_auto">{t('profModeAutoAll')}</option>
                </select>

                {selMode === 'manual' && (
                  <div className="max-h-40 overflow-y-auto rounded-xl border border-slate-200 dark:border-slate-800 divide-y divide-slate-100 dark:divide-slate-800">
                    {previewDBs.length === 0 ? (
                      <div className="p-3 text-xs text-slate-400">
                        {lang === 'es' ? 'Prueba la conexión en el Paso 2 para listar bases.' : 'Test connection in Step 2 to list databases.'}
                      </div>
                    ) : (
                      previewDBs.map((d) => (
                        <label key={d.name} className="px-3 py-2 flex items-center justify-between text-xs cursor-pointer">
                          <span>
                            <b>{d.name}</b> <span className="text-slate-400">({formatBytes(d.sizeBytes)})</span>
                          </span>
                          <input
                            type="checkbox"
                            checked={selectedDBs.includes(d.name)}
                            onChange={(e) =>
                              setSelectedDBs((prev) =>
                                e.target.checked ? [...prev, d.name] : prev.filter((x) => x !== d.name),
                              )
                            }
                            className="accent-cyan-600"
                          />
                        </label>
                      ))
                    )}
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        <div className="px-8 py-4 bg-slate-50 dark:bg-slate-950/60 border-t border-slate-200 dark:border-slate-800 flex items-center justify-between">
          <button
            type="button"
            disabled={step === 1}
            onClick={() => setStep((s) => s - 1)}
            className="px-4 py-2.5 rounded-xl border border-slate-300 dark:border-slate-700 text-xs font-semibold flex items-center gap-1.5 disabled:opacity-40"
          >
            <ChevronLeft className="w-4 h-4" /> {t('wizBack')}
          </button>

          {step < 6 ? (
            <button
              type="button"
              onClick={handleNext}
              className="px-5 py-2.5 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold flex items-center gap-1.5 shadow"
            >
              {t('wizNext')} <ChevronRight className="w-4 h-4" />
            </button>
          ) : (
            <button
              type="button"
              disabled={busy}
              onClick={handleFinish}
              className="px-6 py-2.5 rounded-xl bg-gradient-to-r from-emerald-600 to-cyan-600 hover:from-emerald-500 hover:to-cyan-500 text-white text-xs font-bold flex items-center gap-2 shadow-lg"
            >
              <CheckCircle2 className="w-4 h-4" /> {t('wizFinish')}
            </button>
          )}
        </div>
      </div>

      <DriveFolderModal
        open={folderModalOpen}
        onClose={() => setFolderModalOpen(false)}
        onSelect={async (id, path) => {
          const ds = await apiCall<DriveStatus>('POST', '/api/v1/cloud/root-folder', {
            folderId: id,
            folderName: path,
          });
          setDriveStatus(ds);
        }}
      />
    </div>
  );
};
