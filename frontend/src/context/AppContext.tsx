import React, { createContext, useContext, useEffect, useState } from 'react';
import { dict, Lang, ThemeMode, TranslationKey } from '../i18n/translations';
import { LiveEvent, nativeBridge } from '../lib/bridge';

interface AppContextValue {
  lang: Lang;
  setLang: (l: Lang) => void;
  t: (key: TranslationKey) => string;
  theme: ThemeMode;
  setTheme: (m: ThemeMode) => void;
  liveJob: LiveEvent | null;
  bannerAlert: { text: string; severity: 'info' | 'success' | 'warning' | 'error' } | null;
  notifyBanner: (text: string, severity?: 'info' | 'success' | 'warning' | 'error') => void;
  dismissBanner: () => void;
  eventTick: number;
}

const AppContext = createContext<AppContextValue | null>(null);

export const AppProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [lang, setLang] = useState<Lang>('es');
  const [theme, setTheme] = useState<ThemeMode>('system');
  const [liveJob, setLiveJob] = useState<LiveEvent | null>(null);
  const [bannerAlert, setBannerAlert] = useState<AppContextValue['bannerAlert']>(null);
  const [eventTick, setEventTick] = useState(0);

  useEffect(() => {
    const root = document.documentElement;
    const apply = () => {
      const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      const isDark = theme === 'dark' || (theme === 'system' && prefersDark);
      root.classList.toggle('dark', isDark);
    };
    apply();
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    media.addEventListener('change', apply);
    return () => media.removeEventListener('change', apply);
  }, [theme]);

  const notifyBanner = (text: string, severity: 'info' | 'success' | 'warning' | 'error' = 'info') => {
    setBannerAlert({ text, severity });
    const timeoutMs = severity === 'error' ? 12000 : 4500;
    setTimeout(() => {
      setBannerAlert((prev) => (prev?.text === text ? null : prev));
    }, timeoutMs);
  };

  const dismissBanner = () => setBannerAlert(null);

  useEffect(() => {
    const unsub = nativeBridge.onEvent((ev) => {
      setEventTick((n) => n + 1);
      if (ev.type === 'BACKUP_STARTED' || ev.type === 'BACKUP_PROGRESS') {
        setLiveJob(ev);
      } else if (ev.type === 'BACKUP_COMPLETED' || ev.type === 'BACKUP_FAILED' || ev.type === 'CLOUD_RETRYING') {
        setLiveJob(null);
        notifyBanner(`${ev.title}: ${ev.message}`, ev.severity);
      } else if (ev.type === 'CLOUD_COMPLETED') {
        notifyBanner(`${ev.title}: ${ev.message}`, 'success');
      }
    });
    return unsub;
  }, []);

  const t = (key: TranslationKey): string => dict[lang][key] ?? key;

  return (
    <AppContext.Provider
      value={{ lang, setLang, t, theme, setTheme, liveJob, bannerAlert, notifyBanner, dismissBanner, eventTick }}
    >
      {children}
    </AppContext.Provider>
  );
};

export function useApp() {
  const ctx = useContext(AppContext);
  if (!ctx) throw new Error('useApp must be used inside AppProvider');
  return ctx;
}
