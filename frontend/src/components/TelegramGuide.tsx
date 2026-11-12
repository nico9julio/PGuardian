import React, { useState } from 'react';
import { HelpCircle, ChevronDown, ChevronUp, ExternalLink } from 'lucide-react';
import { nativeBridge } from '../lib/bridge';
import { useApp } from '../context/AppContext';

interface Props {
  defaultOpen?: boolean;
  compact?: boolean;
}

export const TelegramGuide: React.FC<Props> = ({ defaultOpen = false, compact = false }) => {
  const { lang } = useApp();
  const [open, setOpen] = useState(defaultOpen);
  const es = lang === 'es';

  return (
    <div className="rounded-xl border border-sky-500/30 bg-sky-500/5 overflow-hidden text-xs">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="w-full px-3.5 py-2.5 flex items-center justify-between gap-2 text-left font-semibold text-sky-700 dark:text-sky-300 hover:bg-sky-500/10 transition"
      >
        <span className="flex items-center gap-2">
          <HelpCircle className="w-4 h-4 text-sky-500 shrink-0" />
          {es ? '¿Cómo crear y vincular tu Bot de Telegram? (Paso a paso)' : 'How to create & link your Telegram Bot (Step-by-step)'}
        </span>
        {open ? <ChevronUp className="w-4 h-4 shrink-0" /> : <ChevronDown className="w-4 h-4 shrink-0" />}
      </button>

      {open && (
        <div className={`px-4 pb-3.5 pt-1 space-y-2.5 text-slate-600 dark:text-slate-300 border-t border-sky-500/20 ${compact ? 'max-h-52 overflow-y-auto' : ''}`}>
          <ol className="list-decimal list-inside space-y-2 leading-relaxed pt-1.5">
            <li>
              {es ? (
                <>
                  Abre{' '}
                  <button
                    type="button"
                    onClick={() => void nativeBridge.openURL('https://t.me/BotFather')}
                    className="text-sky-600 dark:text-sky-400 font-semibold hover:underline inline-flex items-center gap-0.5"
                  >
                    @BotFather <ExternalLink className="w-3 h-3" />
                  </button>{' '}
                  en Telegram y envía el comando <code className="px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">/newbot</code>.
                </>
              ) : (
                <>
                  Open{' '}
                  <button
                    type="button"
                    onClick={() => void nativeBridge.openURL('https://t.me/BotFather')}
                    className="text-sky-600 dark:text-sky-400 font-semibold hover:underline inline-flex items-center gap-0.5"
                  >
                    @BotFather <ExternalLink className="w-3 h-3" />
                  </button>{' '}
                  in Telegram and send <code className="px-1.5 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">/newbot</code>.
                </>
              )}
            </li>
            <li>
              {es ? (
                <>
                  Elige un nombre visible (ej. <b>PGuardian Alertas</b>) y un usuario terminado en <code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">bot</code> (ej. <code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">mi_pguardian_bot</code>).
                </>
              ) : (
                <>
                  Choose a display name (e.g. <b>PGuardian Alerts</b>) and a username ending in <code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">bot</code> (e.g. <code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">my_pguardian_bot</code>).
                </>
              )}
            </li>
            <li>
              {es ? (
                <>
                  Copia el <b>HTTP API Token</b> que te entrega (<code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">123456789:ABC...</code>), pégalo abajo y pulsa <b>Guardar</b>.
                </>
              ) : (
                <>
                  Copy the <b>HTTP API Token</b> (<code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">123456789:ABC...</code>), paste it below, and click <b>Save</b>.
                </>
              )}
            </li>
            <li>
              {es ? (
                <>
                  <b>¡Importante!</b> Abre el chat de tu nuevo bot en Telegram y pulsa <b>Iniciar</b> (<code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">/start</code>) o envíale cualquier mensaje.{' '}
                  <span className="text-slate-500 dark:text-slate-400">
                    (Si quieres alertas en un <b>grupo</b>, agrega el bot al grupo y envía un mensaje como <code className="font-mono">/start</code> allí).
                  </span>
                </>
              ) : (
                <>
                  <b>Important!</b> Open a chat with your new bot in Telegram and press <b>Start</b> (<code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">/start</code>) or send any message.{' '}
                  <span className="text-slate-500 dark:text-slate-400">
                    (For a <b>group</b>, add the bot to the group and send a message like <code className="font-mono">/start</code> there).
                  </span>
                </>
              )}
            </li>
            <li>
              {es ? (
                <>
                  Haz clic en <b>Autodetectar</b> para rellenar tu <code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">Chat ID</code> automáticamente, pulsa <b>+</b> para agregarlo a la lista y prueba con <b>Probar Telegram</b>.
                </>
              ) : (
                <>
                  Click <b>Detect</b> to auto-fill your <code className="px-1 py-0.5 rounded bg-slate-200 dark:bg-slate-800 font-mono">Chat ID</code>, click <b>+</b> to add the recipient, and verify with <b>Test Telegram</b>.
                </>
              )}
            </li>
          </ol>
        </div>
      )}
    </div>
  );
};
