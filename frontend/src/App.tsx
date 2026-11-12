import React, { useEffect, useState } from 'react';
import {
  LayoutDashboard,
  Database,
  CalendarClock,
  Server,
  Cloud,
  Bell,
  DownloadCloud,
  Settings,
  Lock,
  Globe,
  Sun,
  Moon,
  Monitor,
  WifiOff,
  X,
} from 'lucide-react';
import { apiCall, setAuthToken } from './lib/bridge';
import { AppProvider, useApp } from './context/AppContext';
import { Lang, ThemeMode } from './i18n/translations';
import { LockScreen } from './pages/LockScreen';
import { SetupWizard } from './pages/SetupWizard';
import { DashboardView } from './pages/DashboardView';
import { DatabasesView } from './pages/DatabasesView';
import { ProfilesView } from './pages/ProfilesView';
import { ServersView } from './pages/ServersView';
import { CloudView } from './pages/CloudView';
import { NotificationsView } from './pages/NotificationsView';
import { UpdatesView } from './pages/UpdatesView';
import { SettingsView } from './pages/SettingsView';

type Tab = 'dashboard' | 'databases' | 'profiles' | 'servers' | 'cloud' | 'notifications' | 'updates' | 'settings';

const Shell: React.FC = () => {
  const { lang, setLang, theme, setTheme, t, bannerAlert, dismissBanner } = useApp();
  const [bootState, setBootState] = useState<'loading' | 'offline' | 'wizard' | 'locked' | 'unlocked'>('loading');
  const [version, setVersion] = useState('0.1.0');
  const [tab, setTab] = useState<Tab>('dashboard');

  const checkServiceHealth = async () => {
    setBootState('loading');
    try {
      const h = await apiCall<{
        status: string;
        version: string;
        setupCompleted: boolean;
        language: Lang;
        theme: ThemeMode;
      }>('GET', '/api/v1/health');
      setVersion(h.version);
      if (h.language) setLang(h.language);
      if (h.theme) setTheme(h.theme);
      setBootState(h.setupCompleted ? 'locked' : 'wizard');
    } catch {
      setBootState('offline');
    }
  };

  useEffect(() => {
    void checkServiceHealth();
  }, []);

  if (bootState === 'loading') {
    return (
      <div className="min-h-screen flex items-center justify-center bg-slate-950 text-slate-300 text-sm">
        Connecting to PGuardianService...
      </div>
    );
  }

  if (bootState === 'offline') {
    return (
      <div className="min-h-screen flex items-center justify-center p-6 bg-slate-50 dark:bg-slate-950">
        <div className="max-w-md w-full p-8 rounded-3xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-center space-y-4 shadow-xl">
          <WifiOff className="w-12 h-12 text-red-500 mx-auto" />
          <h2 className="text-lg font-bold">{t('serviceOfflineTitle')}</h2>
          <p className="text-xs text-slate-500">{t('serviceOfflineDesc')}</p>
          <code className="block p-3 rounded-xl bg-slate-100 dark:bg-slate-950 text-xs font-mono">
            sc start PGuardianService
          </code>
          <button
            onClick={checkServiceHealth}
            className="px-5 py-2.5 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold"
          >
            {t('retry')}
          </button>
        </div>
      </div>
    );
  }

  if (bootState === 'wizard') {
    return <SetupWizard onComplete={() => setBootState('unlocked')} />;
  }

  if (bootState === 'locked') {
    return <LockScreen version={version} onUnlocked={() => setBootState('unlocked')} />;
  }

  const navItems: { id: Tab; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
    { id: 'dashboard', label: t('navDashboard'), icon: LayoutDashboard },
    { id: 'databases', label: t('navDatabases'), icon: Database },
    { id: 'profiles', label: t('navProfiles'), icon: CalendarClock },
    { id: 'servers', label: t('navServers'), icon: Server },
    { id: 'cloud', label: t('navCloud'), icon: Cloud },
    { id: 'notifications', label: t('navNotifications'), icon: Bell },
    { id: 'updates', label: t('navUpdates'), icon: DownloadCloud },
    { id: 'settings', label: t('navSettings'), icon: Settings },
  ];

  return (
    <div className="h-screen flex overflow-hidden bg-slate-50 dark:bg-slate-950">
      <aside className="w-64 shrink-0 border-r border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 flex flex-col justify-between p-4">
        <div className="space-y-6">
          <div className="flex items-center gap-3 px-2 pt-1">
            <img src="/logo.png" alt="PGuardian" className="w-10 h-10 rounded-xl shadow" />
            <div>
              <div className="font-bold text-base leading-tight">PGuardian</div>
              <div className="text-[11px] text-cyan-600 dark:text-cyan-400 font-mono">v{version}</div>
            </div>
          </div>

          <nav className="space-y-1">
            {navItems.map((item) => {
              const Icon = item.icon;
              const active = tab === item.id;
              return (
                <button
                  key={item.id}
                  onClick={() => setTab(item.id)}
                  className={`w-full px-3.5 py-2.5 rounded-xl text-xs font-semibold flex items-center gap-3 transition ${
                    active
                      ? 'bg-gradient-to-r from-cyan-600 to-blue-600 text-white shadow-md shadow-cyan-600/15'
                      : 'text-slate-600 dark:text-slate-300 hover:bg-slate-100 dark:hover:bg-slate-800/70'
                  }`}
                >
                  <Icon className="w-4 h-4 shrink-0" />
                  <span className="truncate">{item.label}</span>
                </button>
              );
            })}
          </nav>
        </div>

        <div className="space-y-3 pt-4 border-t border-slate-200 dark:border-slate-800">
          <div className="flex items-center justify-between gap-2">
            <button
              onClick={() => setLang(lang === 'es' ? 'en' : 'es')}
              className="px-2.5 py-1.5 rounded-lg border border-slate-200 dark:border-slate-800 text-xs font-medium flex items-center gap-1.5 hover:bg-slate-100 dark:hover:bg-slate-800"
            >
              <Globe className="w-3.5 h-3.5 text-cyan-500" />
              {lang.toUpperCase()}
            </button>
            <div className="flex rounded-lg border border-slate-200 dark:border-slate-800 p-0.5">
              {(['system', 'light', 'dark'] as const).map((m) => (
                <button
                  key={m}
                  onClick={() => setTheme(m)}
                  className={`p-1.5 rounded-md ${
                    theme === m ? 'bg-cyan-500 text-white' : 'text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'
                  }`}
                >
                  {m === 'system' ? <Monitor className="w-3.5 h-3.5" /> : m === 'light' ? <Sun className="w-3.5 h-3.5" /> : <Moon className="w-3.5 h-3.5" />}
                </button>
              ))}
            </div>
          </div>

          <button
            onClick={() => {
              setAuthToken('');
              setBootState('locked');
            }}
            className="w-full py-2 px-3 rounded-xl border border-slate-200 dark:border-slate-800 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-medium flex items-center justify-center gap-2 text-slate-500"
          >
            <Lock className="w-3.5 h-3.5" /> {t('logout')}
          </button>
        </div>
      </aside>

      <main className="flex-1 overflow-y-auto p-8 relative">
        {bannerAlert && (
          <div
            className={`mb-5 px-4 py-3 rounded-xl border text-xs font-semibold shadow-sm flex items-start justify-between gap-3 ${
              bannerAlert.severity === 'error'
                ? 'bg-red-500/10 border-red-500/30 text-red-500'
                : bannerAlert.severity === 'success'
                ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-600 dark:text-emerald-400'
                : bannerAlert.severity === 'warning'
                ? 'bg-amber-500/10 border-amber-500/30 text-amber-500'
                : 'bg-cyan-500/10 border-cyan-500/30 text-cyan-600 dark:text-cyan-400'
            }`}
          >
            <div className="max-h-28 overflow-y-auto break-words flex-1 leading-relaxed">{bannerAlert.text}</div>
            <button
              type="button"
              onClick={dismissBanner}
              className="shrink-0 p-1 rounded-lg hover:bg-black/10 dark:hover:bg-white/10 transition"
              title={t('cancel')}
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        )}

        {tab === 'dashboard' && <DashboardView />}
        {tab === 'databases' && <DatabasesView />}
        {tab === 'profiles' && <ProfilesView />}
        {tab === 'servers' && <ServersView />}
        {tab === 'cloud' && <CloudView />}
        {tab === 'notifications' && <NotificationsView />}
        {tab === 'updates' && <UpdatesView />}
        {tab === 'settings' && <SettingsView />}
      </main>
    </div>
  );
};

export default function App() {
  return (
    <AppProvider>
      <Shell />
    </AppProvider>
  );
}
