import React, { useState } from 'react';
import { Lock, ArrowRight, Globe, Sun, Moon, Monitor } from 'lucide-react';
import { apiCall, setAuthToken } from '../lib/bridge';
import { useApp } from '../context/AppContext';

interface Props {
  version: string;
  onUnlocked: () => void;
}

export const LockScreen: React.FC<Props> = ({ version, onUnlocked }) => {
  const { lang, setLang, theme, setTheme, t } = useApp();
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password.trim()) return;
    setLoading(true);
    setError('');
    try {
      const res = await apiCall<{ token: string }>('POST', '/api/v1/auth/login', { password });
      setAuthToken(res.token);
      onUnlocked();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex flex-col justify-between bg-gradient-to-br from-slate-100 via-slate-50 to-cyan-50/40 dark:from-slate-950 dark:via-slate-900 dark:to-cyan-950/30 p-6">
      <div className="flex justify-end items-center gap-2">
        <button
          onClick={() => setLang(lang === 'es' ? 'en' : 'es')}
          className="px-3 py-1.5 rounded-lg border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/80 text-xs font-medium flex items-center gap-1.5 hover:border-cyan-500/50 transition"
        >
          <Globe className="w-3.5 h-3.5 text-cyan-500" />
          {lang.toUpperCase()}
        </button>
        <div className="flex rounded-lg border border-slate-200 dark:border-slate-800 bg-white/80 dark:bg-slate-900/80 p-0.5">
          {(['system', 'light', 'dark'] as const).map((m) => (
            <button
              key={m}
              onClick={() => setTheme(m)}
              className={`p-1.5 rounded-md transition ${
                theme === m ? 'bg-cyan-500 text-white' : 'text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
              }`}
              title={m}
            >
              {m === 'system' ? <Monitor className="w-3.5 h-3.5" /> : m === 'light' ? <Sun className="w-3.5 h-3.5" /> : <Moon className="w-3.5 h-3.5" />}
            </button>
          ))}
        </div>
      </div>

      <div className="mx-auto w-full max-w-md">
        <div className="rounded-3xl border border-slate-200/80 dark:border-slate-800 bg-white/90 dark:bg-slate-900/90 backdrop-blur-xl shadow-2xl p-8">
          <div className="flex flex-col items-center text-center mb-7">
            <img src="/logo.png" alt="PGuardian" className="w-20 h-20 rounded-2xl shadow-lg mb-4" />
            <h1 className="text-2xl font-bold tracking-tight">PGuardian</h1>
            <p className="text-xs text-cyan-600 dark:text-cyan-400 font-medium mt-0.5">{t('appSubtitle')}</p>
            <p className="text-xs text-slate-500 dark:text-slate-400 mt-3">{t('lockSubtitle')}</p>
          </div>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="relative">
              <Lock className="w-4 h-4 text-slate-400 absolute left-3.5 top-1/2 -translate-y-1/2" />
              <input
                type="password"
                autoFocus
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={t('lockPlaceholder')}
                className="w-full pl-10 pr-4 py-3 rounded-xl border border-slate-300 dark:border-slate-700 bg-slate-50 dark:bg-slate-950 text-sm focus:outline-none focus:ring-2 focus:ring-cyan-500"
              />
            </div>

            {error && (
              <div className="p-3 rounded-xl bg-red-500/10 border border-red-500/30 text-red-500 text-xs text-center">
                {error}
              </div>
            )}

            <button
              type="submit"
              disabled={loading || !password.trim()}
              className="w-full py-3 rounded-xl bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white font-semibold text-sm flex items-center justify-center gap-2 shadow-lg shadow-cyan-600/20 disabled:opacity-50 transition"
            >
              {t('unlockBtn')}
              <ArrowRight className="w-4 h-4" />
            </button>
          </form>
        </div>
      </div>

      <div className="text-center text-xs text-slate-400">PGuardian v{version} • Windows Server & RDP Edition</div>
    </div>
  );
};
