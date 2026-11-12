import React, { useEffect, useState } from 'react';
import { DownloadCloud, PackageOpen, RefreshCw, CheckCircle2, ShieldAlert } from 'lucide-react';
import { apiCall, formatBytes, nativeBridge } from '../lib/bridge';
import { useApp } from '../context/AppContext';

interface ReleaseInfo {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
  releaseName: string;
  releaseNotes: string;
  publishedAt: string;
  assetName: string;
  assetSizeBytes: number;
  htmlUrl: string;
}

export const UpdatesView: React.FC = () => {
  const { lang, t, notifyBanner } = useApp();
  const [repo, setRepo] = useState('nico9julio/PGuardian');
  const [ghToken, setGhToken] = useState('');
  const [hasGhToken, setHasGhToken] = useState(false);
  const [release, setRelease] = useState<ReleaseInfo | null>(null);
  const [checking, setChecking] = useState(false);
  const [applying, setApplying] = useState(false);
  const [forceUpdate, setForceUpdate] = useState(false);

  useEffect(() => {
    void (async () => {
      try {
        const st = await apiCall<{ githubRepo: string; hasGithubToken: boolean }>('GET', '/api/v1/settings');
        if (st.githubRepo) setRepo(st.githubRepo);
        setHasGhToken(st.hasGithubToken);
      } catch {}
      void checkGitHub();
    })();
  }, []);

  const checkGitHub = async () => {
    setChecking(true);
    try {
      const res = await apiCall<ReleaseInfo>('GET', '/api/v1/updates/check');
      setRelease(res);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    } finally {
      setChecking(false);
    }
  };

  const saveRepoConfig = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await apiCall('POST', '/api/v1/updates/github-config', { repo, token: ghToken });
      setGhToken('');
      notifyBanner(t('savedOk'), 'success');
      await checkGitHub();
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const applyGitHubUpdate = async () => {
    setApplying(true);
    try {
      const res = await apiCall<{ updatingTo: string }>('POST', '/api/v1/updates/apply-github', {
        force: forceUpdate,
      });
      notifyBanner(
        lang === 'es'
          ? `Aplicando versión ${res.updatingTo}. La ventana se reabrirá automáticamente...`
          : `Applying version ${res.updatingTo}. Console will reopen automatically...`,
        'info',
      );
    } catch (err) {
      setApplying(false);
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const applyLocalZip = async () => {
    const zipPath = await nativeBridge.pickUpdateZip('Select PGuardianUpdate_vX.Y.Z.zip');
    if (!zipPath) return;
    setApplying(true);
    try {
      const res = await apiCall<{ updatingTo: string }>('POST', '/api/v1/updates/apply-zip', {
        zipPath,
        force: forceUpdate,
      });
      notifyBanner(
        lang === 'es'
          ? `Actualizando a ${res.updatingTo}...`
          : `Updating to ${res.updatingTo}...`,
        'info',
      );
    } catch (err) {
      setApplying(false);
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <h2 className="text-xl font-bold">{t('updTitle')}</h2>
        <p className="text-xs text-slate-500">{t('updSubtitle')}</p>
      </div>

      <div className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4">
        <div className="flex items-center justify-between">
          <div>
            <span className="text-xs text-slate-400 block">Versión Actual / Current Version</span>
            <span className="text-lg font-bold font-mono">v{release?.currentVersion || '0.1.0'}</span>
          </div>
          <button
            onClick={checkGitHub}
            disabled={checking}
            className="px-4 py-2 rounded-xl border border-slate-300 dark:border-slate-700 hover:bg-slate-100 dark:hover:bg-slate-800 text-xs font-semibold flex items-center gap-1.5"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${checking ? 'animate-spin' : ''}`} />
            {t('updCheckBtn')}
          </button>
        </div>

        {release && (
          <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800 space-y-3">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2 text-xs font-semibold">
                <CheckCircle2
                  className={`w-4 h-4 ${release.updateAvailable ? 'text-cyan-500' : 'text-emerald-500'}`}
                />
                {release.updateAvailable
                  ? `Nueva versión disponible / New release: v${release.latestVersion}`
                  : lang === 'es'
                  ? 'Estás en la última versión publicada en GitHub'
                  : 'You are running the latest GitHub release'}
              </div>
              {release.assetSizeBytes > 0 && (
                <span className="text-[11px] font-mono text-slate-400">
                  {release.assetName} ({formatBytes(release.assetSizeBytes)})
                </span>
              )}
            </div>

            {release.releaseNotes && (
              <pre className="text-xs text-slate-600 dark:text-slate-300 whitespace-pre-wrap font-sans max-h-40 overflow-y-auto p-3 rounded-lg bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800">
                {release.releaseNotes}
              </pre>
            )}

            {release.assetName && (
              <button
                onClick={applyGitHubUpdate}
                disabled={applying}
                className="px-5 py-2.5 rounded-xl bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white text-xs font-bold flex items-center gap-2 shadow"
              >
                <DownloadCloud className="w-4 h-4" />
                {t('updApplyGithub')} (v{release.latestVersion})
              </button>
            )}
          </div>
        )}

        <form onSubmit={saveRepoConfig} className="pt-2 grid grid-cols-1 md:grid-cols-3 gap-3 items-end">
          <div>
            <label className="block text-[11px] font-semibold text-slate-500 mb-1">GitHub Repository (owner/repo)</label>
            <input
              value={repo}
              onChange={(e) => setRepo(e.target.value)}
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
            />
          </div>
          <div>
            <label className="block text-[11px] font-semibold text-slate-500 mb-1">
              GitHub Token {hasGhToken ? '(Optional DPAPI ✓)' : '(Optional for private repos)'}
            </label>
            <input
              type="password"
              value={ghToken}
              onChange={(e) => setGhToken(e.target.value)}
              placeholder="github_pat_..."
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
            />
          </div>
          <button
            type="submit"
            className="px-4 py-2 rounded-xl bg-slate-200 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-xs font-semibold"
          >
            {t('save')}
          </button>
        </form>
      </div>

      <div className="p-6 rounded-2xl border border-cyan-500/30 bg-cyan-500/5 space-y-4">
        <div className="flex items-start gap-3">
          <PackageOpen className="w-6 h-6 text-cyan-500 shrink-0 mt-0.5" />
          <div>
            <h3 className="font-bold text-sm">{t('updLocalCard')}</h3>
            <p className="text-xs text-slate-500 mt-1">{t('updLocalDesc')}</p>
          </div>
        </div>

        <div className="flex flex-wrap items-center justify-between gap-4 pt-1">
          <label className="flex items-center gap-2 text-xs text-slate-500 cursor-pointer">
            <input
              type="checkbox"
              checked={forceUpdate}
              onChange={(e) => setForceUpdate(e.target.checked)}
              className="accent-amber-500"
            />
            <ShieldAlert className="w-3.5 h-3.5 text-amber-500" />
            {lang === 'es' ? 'Forzar sin esperar backups largos' : 'Force immediately without waiting'}
          </label>

          <button
            onClick={applyLocalZip}
            disabled={applying}
            className="px-5 py-2.5 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-bold flex items-center gap-2 shadow"
          >
            <PackageOpen className="w-4 h-4" />
            {t('updPickZip')}
          </button>
        </div>
      </div>
    </div>
  );
};
