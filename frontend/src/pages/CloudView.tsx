import React, { useEffect, useState } from 'react';
import { Cloud, CheckCircle2, Folder, LogOut, ExternalLink } from 'lucide-react';
import { apiCall, DriveStatus, nativeBridge } from '../lib/bridge';
import { useApp } from '../context/AppContext';
import { DriveFolderModal } from '../components/DriveFolderModal';

export const CloudView: React.FC = () => {
  const { lang, t, notifyBanner, eventTick } = useApp();
  const [status, setStatus] = useState<DriveStatus | null>(null);
  const [clientId, setClientId] = useState('');
  const [clientSecret, setClientSecret] = useState('');
  const [folderModalOpen, setFolderModalOpen] = useState(false);

  const load = async () => {
    try {
      const ds = await apiCall<DriveStatus>('GET', '/api/v1/cloud/status');
      setStatus(ds);
      if (ds.clientId && !clientId) setClientId(ds.clientId);
    } catch (e) {
      notifyBanner(e instanceof Error ? e.message : String(e), 'error');
    }
  };

  useEffect(() => {
    void load();
  }, [eventTick]);

  const handleSaveCreds = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const ds = await apiCall<DriveStatus>('POST', '/api/v1/cloud/credentials', { clientId, clientSecret });
      setStatus(ds);
      setClientSecret('');
      notifyBanner(t('savedOk'), 'success');
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const handleLoginBrowser = async () => {
    try {
      if (clientId && clientSecret) {
        await apiCall('POST', '/api/v1/cloud/credentials', { clientId, clientSecret });
        setClientSecret('');
      }
      const res = await apiCall<{ authUrl: string }>('POST', '/api/v1/cloud/oauth-start');
      await nativeBridge.openURL(res.authUrl);
    } catch (err) {
      notifyBanner(err instanceof Error ? err.message : String(err), 'error');
    }
  };

  const handleDisconnect = async () => {
    await apiCall('POST', '/api/v1/cloud/disconnect');
    await load();
  };

  return (
    <div className="space-y-6 max-w-3xl">
      <div>
        <h2 className="text-xl font-bold">{t('cloudTitle')}</h2>
        <p className="text-xs text-slate-500">{t('cloudSubtitle')}</p>
      </div>

      <form
        onSubmit={handleSaveCreds}
        className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4"
      >
        <div className="flex items-center justify-between flex-wrap gap-2">
          <h3 className="font-bold text-sm">{t('cloudCredsCard')}</h3>
          <div className="flex items-center gap-3">
            <button
              type="button"
              onClick={() => nativeBridge.openURL('https://console.cloud.google.com/apis/library/drive.googleapis.com')}
              className="text-xs text-emerald-600 dark:text-emerald-400 hover:underline flex items-center gap-1 font-medium"
            >
              {lang === 'es' ? 'Habilitar Drive API' : 'Enable Drive API'} <ExternalLink className="w-3 h-3" />
            </button>
            <button
              type="button"
              onClick={() => nativeBridge.openURL('https://console.cloud.google.com/apis/credentials')}
              className="text-xs text-cyan-500 hover:underline flex items-center gap-1"
            >
              Google Cloud Console <ExternalLink className="w-3 h-3" />
            </button>
          </div>
        </div>

        <p className="text-xs text-slate-500">
          {lang === 'es'
            ? 'Habilita "Google Drive API" en tu proyecto de Google Cloud y crea un ID de cliente OAuth 2.0 de tipo "Aplicación de escritorio". Se cifrará localmente mediante Windows DPAPI.'
            : 'Enable "Google Drive API" in your Google Cloud project and create an OAuth 2.0 Client ID of type "Desktop App". Stored encrypted via Windows DPAPI.'}
        </p>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-xs font-semibold mb-1">{t('cloudClientId')}</label>
            <input
              required
              value={clientId}
              onChange={(e) => setClientId(e.target.value)}
              placeholder="123456-abc.apps.googleusercontent.com"
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
            />
          </div>
          <div>
            <label className="block text-xs font-semibold mb-1">
              {t('cloudClientSecret')} {status?.hasSecret ? '(DPAPI ✓)' : ''}
            </label>
            <input
              type="password"
              value={clientSecret}
              onChange={(e) => setClientSecret(e.target.value)}
              placeholder={status?.hasSecret ? '••••••••••••••••' : 'GOCSPX-...'}
              className="w-full px-3 py-2 rounded-xl border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-950 text-xs font-mono"
            />
          </div>
        </div>

        <button
          type="submit"
          className="px-4 py-2 rounded-xl bg-slate-900 dark:bg-slate-100 text-white dark:text-slate-900 text-xs font-semibold"
        >
          {t('cloudSaveCreds')}
        </button>
      </form>

      <div className="p-6 rounded-2xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 space-y-4">
        <h3 className="font-bold text-sm">{t('cloudAccountCard')}</h3>

        {status?.connected ? (
          <div className="space-y-4">
            <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/30 flex items-center justify-between">
              <div className="flex items-center gap-3">
                <CheckCircle2 className="w-5 h-5 text-emerald-500" />
                <div>
                  <div className="text-xs font-bold text-emerald-600 dark:text-emerald-400">
                    {status.accountEmail || 'Connected'}
                  </div>
                  <div className="text-[11px] text-slate-500">OAuth 2.0 Refresh Token (Windows DPAPI Protected)</div>
                </div>
              </div>
              <button
                onClick={handleDisconnect}
                className="px-3 py-1.5 rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-500 text-xs font-medium flex items-center gap-1.5"
              >
                <LogOut className="w-3.5 h-3.5" /> {t('cloudDisconnect')}
              </button>
            </div>

            <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-950/60 border border-slate-200 dark:border-slate-800 flex items-center justify-between">
              <div>
                <div className="text-[11px] text-slate-400">
                  {lang === 'es' ? 'Carpeta Raíz Seleccionada en Google Drive' : 'Selected Root Folder in Google Drive'}
                </div>
                <div className="text-sm font-semibold flex items-center gap-2 mt-0.5">
                  <Folder className="w-4 h-4 text-amber-500" />
                  {status.rootFolderName || '/My Drive'}
                </div>
              </div>
              <button
                onClick={() => setFolderModalOpen(true)}
                className="px-4 py-2 rounded-xl bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-semibold"
              >
                {t('cloudChooseFolder')}
              </button>
            </div>
          </div>
        ) : (
          <button
            onClick={handleLoginBrowser}
            disabled={!status?.configured && (!clientId || !clientSecret)}
            className="px-5 py-2.5 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-xs font-semibold flex items-center gap-2 disabled:opacity-50"
          >
            <Cloud className="w-4 h-4" /> {t('cloudLoginBrowser')}
          </button>
        )}
      </div>

      <DriveFolderModal
        open={folderModalOpen}
        onClose={() => setFolderModalOpen(false)}
        onSelect={async (id, path) => {
          const ds = await apiCall<DriveStatus>('POST', '/api/v1/cloud/root-folder', {
            folderId: id,
            folderName: path,
          });
          setStatus(ds);
          notifyBanner(t('savedOk'), 'success');
        }}
      />
    </div>
  );
};
