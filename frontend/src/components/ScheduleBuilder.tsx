import React, { useEffect, useMemo, useState } from 'react';
import { CalendarClock, Clock, Code2, AlertCircle } from 'lucide-react';
import { useApp } from '../context/AppContext';
import {
  buildCronFromSchedule,
  describeCron,
  getDayShortLabels,
  getNextCronRuns,
  isValidFiveFieldCron,
  parseCronToSchedule,
  ParsedSchedule,
  ScheduleMode,
} from '../lib/scheduleUtils';

interface ScheduleBuilderProps {
  value: string;
  onChange: (cronExpr: string, humanLabel: string) => void;
  compact?: boolean;
}

const MINUTE_INTERVALS = [5, 10, 15, 20, 30, 45];
const HOUR_INTERVALS = [1, 2, 3, 4, 6, 8, 12];

export const ScheduleBuilder: React.FC<ScheduleBuilderProps> = ({ value, onChange, compact = false }) => {
  const { lang, t } = useApp();
  const [sched, setSched] = useState<ParsedSchedule>(() => parseCronToSchedule(value));

  // Sync external changes (e.g. switching edited profile) if cron doesn't match current state
  useEffect(() => {
    const currentBuilt = buildCronFromSchedule(sched);
    if (value.trim() !== currentBuilt.trim()) {
      setSched(parseCronToSchedule(value));
    }
  }, [value]);

  const emitChange = (next: ParsedSchedule) => {
    const cron = buildCronFromSchedule(next);
    const nextWithRaw = { ...next, rawCron: cron };
    setSched(nextWithRaw);
    onChange(cron, describeCron(cron, lang));
  };

  const handleModeSwitch = (mode: ScheduleMode) => {
    const next: ParsedSchedule = { ...sched, mode };
    if (mode === 'advanced') {
      next.rawCron = value || buildCronFromSchedule(sched);
      setSched(next);
      return;
    }
    emitChange(next);
  };

  const activeCron = sched.mode === 'advanced' ? sched.rawCron : buildCronFromSchedule(sched);
  const valid = isValidFiveFieldCron(activeCron);
  const summaryText = valid ? describeCron(activeCron, lang) : t('schedInvalidCron');

  const nextRuns = useMemo(() => {
    if (!valid) return [];
    return getNextCronRuns(activeCron, compact ? 2 : 3);
  }, [activeCron, valid, compact]);

  const modes: { id: ScheduleMode; label: string }[] = [
    { id: 'minutes', label: t('schedModeMinutes') },
    { id: 'hourly', label: t('schedModeHourly') },
    { id: 'daily', label: t('schedModeDaily') },
    { id: 'multi_daily', label: t('schedModeMultiDaily') },
    { id: 'weekly', label: t('schedModeWeekly') },
    { id: 'monthly', label: t('schedModeMonthly') },
    { id: 'advanced', label: t('schedModeAdvanced') },
  ];

  const selectedHoursSet = useMemo(() => {
    const set = new Set<number>();
    for (const tm of sched.multiTimes) {
      const hr = Number(tm.split(':')[0]);
      if (!Number.isNaN(hr)) set.add(hr);
    }
    return set;
  }, [sched.multiTimes]);

  const toggleHourSlot = (hr: number) => {
    const nextHours = new Set(selectedHoursSet);
    if (nextHours.has(hr)) {
      if (nextHours.size <= 1) return; // keep at least 1 selected hour
      nextHours.delete(hr);
    } else {
      nextHours.add(hr);
    }
    const minStr = String(sched.minuteAtHour).padStart(2, '0');
    const multiTimes = Array.from(nextHours)
      .sort((a, b) => a - b)
      .map((h) => `${String(h).padStart(2, '0')}:${minStr}`);
    emitChange({ ...sched, multiTimes });
  };

  const toggleDayOfWeek = (day: number) => {
    const exists = sched.daysOfWeek.includes(day);
    let updated: number[];
    if (exists) {
      if (sched.daysOfWeek.length <= 1) return; // keep at least 1 day selected
      updated = sched.daysOfWeek.filter((d) => d !== day);
    } else {
      updated = [...sched.daysOfWeek, day];
    }
    emitChange({ ...sched, daysOfWeek: updated });
  };

  const formatShortRun = (d: Date) => {
    return d.toLocaleString(lang === 'es' ? 'es-AR' : 'en-US', {
      weekday: 'short',
      day: '2-digit',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    });
  };

  return (
    <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-slate-50/70 dark:bg-slate-950/60 p-4 space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <label className="text-xs font-semibold flex items-center gap-1.5">
          <CalendarClock className="w-4 h-4 text-cyan-600 dark:text-cyan-400" />
          {t('profSchedulePreset')}
        </label>
        <span className="text-[11px] font-mono px-2 py-0.5 rounded bg-slate-200/70 dark:bg-slate-800 text-slate-600 dark:text-slate-400">
          cron: {activeCron || '—'}
        </span>
      </div>

      {/* Frequency mode pills */}
      <div className="flex flex-wrap gap-1.5">
        {modes.map((m) => {
          const isActive = sched.mode === m.id;
          return (
            <button
              key={m.id}
              type="button"
              onClick={() => handleModeSwitch(m.id)}
              className={`px-3 py-1.5 rounded-lg text-xs font-semibold transition flex items-center gap-1 ${
                isActive
                  ? 'bg-cyan-600 text-white shadow-sm'
                  : 'bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:border-cyan-500/50'
              }`}
            >
              {m.id === 'advanced' && <Code2 className="w-3.5 h-3.5" />}
              {m.label}
            </button>
          );
        })}
      </div>

      {/* Mode-specific controls */}
      <div className="pt-1">
        {sched.mode === 'minutes' && (
          <div className="space-y-2.5">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-300">{t('schedEvery')}</span>
              <div className="flex flex-wrap gap-1.5">
                {MINUTE_INTERVALS.map((mins) => (
                  <button
                    key={mins}
                    type="button"
                    onClick={() => emitChange({ ...sched, everyMinutes: mins })}
                    className={`px-2.5 py-1 rounded-lg text-xs font-semibold border transition ${
                      sched.everyMinutes === mins
                        ? 'border-cyan-500 bg-cyan-500/15 text-cyan-700 dark:text-cyan-300'
                        : 'border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800'
                    }`}
                  >
                    {mins} min
                  </button>
                ))}
              </div>
              <div className="flex items-center gap-1.5 ml-1">
                <input
                  type="number"
                  min={1}
                  max={59}
                  value={sched.everyMinutes}
                  onChange={(e) => {
                    const v = Math.min(59, Math.max(1, Number(e.target.value) || 1));
                    emitChange({ ...sched, everyMinutes: v });
                  }}
                  className="w-16 px-2.5 py-1 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-semibold text-center"
                />
                <span className="text-xs text-slate-500">{t('schedMinutesUnit')}</span>
              </div>
            </div>
          </div>
        )}

        {sched.mode === 'hourly' && (
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-300">{t('schedEvery')}</span>
              <div className="flex flex-wrap gap-1">
                {HOUR_INTERVALS.map((hrs) => (
                  <button
                    key={hrs}
                    type="button"
                    onClick={() => emitChange({ ...sched, everyHours: hrs })}
                    className={`px-2.5 py-1 rounded-lg text-xs font-semibold border transition ${
                      sched.everyHours === hrs
                        ? 'border-cyan-500 bg-cyan-500/15 text-cyan-700 dark:text-cyan-300'
                        : 'border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 hover:bg-slate-100 dark:hover:bg-slate-800'
                    }`}
                  >
                    {hrs}h
                  </button>
                ))}
              </div>
              <input
                type="number"
                min={1}
                max={23}
                value={sched.everyHours}
                onChange={(e) => {
                  const v = Math.min(23, Math.max(1, Number(e.target.value) || 1));
                  emitChange({ ...sched, everyHours: v });
                }}
                className="w-16 px-2 py-1 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-semibold text-center"
              />
              <span className="text-xs text-slate-500">{t('schedHoursUnit')}</span>
            </div>

            <div className="flex items-center gap-2">
              <span className="text-xs text-slate-500">{t('schedAtMinute')}</span>
              <select
                value={sched.minuteAtHour}
                onChange={(e) => emitChange({ ...sched, minuteAtHour: Number(e.target.value) })}
                className="px-2.5 py-1 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-mono"
              >
                {[0, 5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55].map((m) => (
                  <option key={m} value={m}>
                    :{String(m).padStart(2, '0')}
                  </option>
                ))}
              </select>
            </div>
          </div>
        )}

        {sched.mode === 'daily' && (
          <div className="flex flex-wrap items-center gap-3">
            <label className="text-xs font-medium text-slate-600 dark:text-slate-300 flex items-center gap-1.5">
              <Clock className="w-3.5 h-3.5 text-slate-400" />
              {t('schedAtTime')}:
            </label>
            <input
              type="time"
              value={sched.dailyTime}
              onChange={(e) => emitChange({ ...sched, dailyTime: e.target.value || '02:00' })}
              className="px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-mono font-semibold"
            />
            <div className="flex flex-wrap gap-1.5">
              {['00:00', '02:00', '04:00', '13:00', '21:00'].map((presetTime) => (
                <button
                  key={presetTime}
                  type="button"
                  onClick={() => emitChange({ ...sched, dailyTime: presetTime })}
                  className={`px-2 py-1 rounded-md text-[11px] font-mono border transition ${
                    sched.dailyTime === presetTime
                      ? 'border-cyan-500 bg-cyan-500/15 text-cyan-700 dark:text-cyan-300 font-bold'
                      : 'border-slate-200 dark:border-slate-800 bg-white dark:bg-slate-900 text-slate-500 hover:text-slate-800 dark:hover:text-slate-200'
                  }`}
                >
                  {presetTime}
                </button>
              ))}
            </div>
          </div>
        )}

        {sched.mode === 'multi_daily' && (
          <div className="space-y-2.5">
            <div className="flex items-center justify-between flex-wrap gap-2">
              <span className="text-xs font-medium text-slate-600 dark:text-slate-300">{t('schedHoursOfDay')}</span>
              <div className="flex items-center gap-1.5 text-xs">
                <span className="text-slate-500">{t('schedAtMinute')}:</span>
                <select
                  value={sched.minuteAtHour}
                  onChange={(e) => {
                    const nextMin = Number(e.target.value);
                    const minStr = String(nextMin).padStart(2, '0');
                    const updatedTimes = Array.from(selectedHoursSet)
                      .sort((a, b) => a - b)
                      .map((h) => `${String(h).padStart(2, '0')}:${minStr}`);
                    emitChange({ ...sched, minuteAtHour: nextMin, multiTimes: updatedTimes });
                  }}
                  className="px-2 py-1 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-mono"
                >
                  {[0, 15, 30, 45].map((m) => (
                    <option key={m} value={m}>
                      :{String(m).padStart(2, '0')}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            <div className="grid grid-cols-6 sm:grid-cols-8 md:grid-cols-12 gap-1.5">
              {Array.from({ length: 24 }, (_, hr) => {
                const selected = selectedHoursSet.has(hr);
                const label = `${String(hr).padStart(2, '0')}:${String(sched.minuteAtHour).padStart(2, '0')}`;
                return (
                  <button
                    key={hr}
                    type="button"
                    onClick={() => toggleHourSlot(hr)}
                    className={`py-1.5 px-1 rounded-lg text-[11px] font-mono font-semibold border transition ${
                      selected
                        ? 'bg-cyan-600 border-cyan-600 text-white shadow-sm'
                        : 'bg-white dark:bg-slate-900 border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-400 hover:border-cyan-500/40'
                    }`}
                  >
                    {label}
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {sched.mode === 'weekly' && (
          <div className="space-y-3">
            <div className="space-y-1.5">
              <div className="flex items-center justify-between flex-wrap gap-2">
                <span className="text-xs font-medium text-slate-600 dark:text-slate-300">{t('schedDaysOfWeek')}</span>
                <div className="flex gap-1.5">
                  <button
                    type="button"
                    onClick={() => emitChange({ ...sched, daysOfWeek: [1, 2, 3, 4, 5] })}
                    className="px-2 py-0.5 rounded bg-slate-200/80 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-[11px] font-medium"
                  >
                    {t('schedWeekdaysBtn')}
                  </button>
                  <button
                    type="button"
                    onClick={() => emitChange({ ...sched, daysOfWeek: [6, 0] })}
                    className="px-2 py-0.5 rounded bg-slate-200/80 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-[11px] font-medium"
                  >
                    {t('schedWeekendsBtn')}
                  </button>
                  <button
                    type="button"
                    onClick={() => emitChange({ ...sched, daysOfWeek: [1, 2, 3, 4, 5, 6, 0] })}
                    className="px-2 py-0.5 rounded bg-slate-200/80 dark:bg-slate-800 hover:bg-slate-300 dark:hover:bg-slate-700 text-[11px] font-medium"
                  >
                    {t('schedAllDaysBtn')}
                  </button>
                </div>
              </div>
              <div className="flex flex-wrap gap-1.5">
                {getDayShortLabels(lang).map(({ day, label }) => {
                  const active = sched.daysOfWeek.includes(day);
                  return (
                    <button
                      key={day}
                      type="button"
                      onClick={() => toggleDayOfWeek(day)}
                      className={`px-3 py-1.5 rounded-lg text-xs font-semibold border transition ${
                        active
                          ? 'bg-cyan-600 border-cyan-600 text-white shadow-sm'
                          : 'bg-white dark:bg-slate-900 border-slate-200 dark:border-slate-800 text-slate-600 dark:text-slate-300 hover:border-cyan-500/40'
                      }`}
                    >
                      {label}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="flex items-center gap-3">
              <label className="text-xs font-medium text-slate-600 dark:text-slate-300 flex items-center gap-1.5">
                <Clock className="w-3.5 h-3.5 text-slate-400" />
                {t('schedAtTime')}:
              </label>
              <input
                type="time"
                value={sched.dailyTime}
                onChange={(e) => emitChange({ ...sched, dailyTime: e.target.value || '03:00' })}
                className="px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-mono font-semibold"
              />
            </div>
          </div>
        )}

        {sched.mode === 'monthly' && (
          <div className="flex flex-wrap items-center gap-4">
            <div className="flex items-center gap-2">
              <label className="text-xs font-medium text-slate-600 dark:text-slate-300">{t('schedDayOfMonth')}:</label>
              <select
                value={sched.dayOfMonth}
                onChange={(e) => emitChange({ ...sched, dayOfMonth: Number(e.target.value) })}
                className="px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-semibold"
              >
                {Array.from({ length: 28 }, (_, idx) => idx + 1).map((d) => (
                  <option key={d} value={d}>
                    {d}
                  </option>
                ))}
              </select>
            </div>

            <div className="flex items-center gap-2">
              <label className="text-xs font-medium text-slate-600 dark:text-slate-300 flex items-center gap-1.5">
                <Clock className="w-3.5 h-3.5 text-slate-400" />
                {t('schedAtTime')}:
              </label>
              <input
                type="time"
                value={sched.dailyTime}
                onChange={(e) => emitChange({ ...sched, dailyTime: e.target.value || '03:00' })}
                className="px-3 py-1.5 rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-900 text-xs font-mono font-semibold"
              />
            </div>
          </div>
        )}

        {sched.mode === 'advanced' && (
          <div className="space-y-1.5">
            <label className="block text-xs font-medium text-slate-600 dark:text-slate-300">
              {t('profCronCustom')}
            </label>
            <input
              required
              value={sched.rawCron}
              onChange={(e) => {
                const raw = e.target.value;
                setSched({ ...sched, rawCron: raw });
                onChange(raw, isValidFiveFieldCron(raw) ? describeCron(raw, lang) : raw);
              }}
              placeholder="0 2 * * *"
              className={`w-full px-3 py-2 rounded-lg border bg-white dark:bg-slate-900 text-xs font-mono ${
                valid
                  ? 'border-slate-300 dark:border-slate-700'
                  : 'border-red-500 focus:outline-none focus:ring-1 focus:ring-red-500'
              }`}
            />
          </div>
        )}
      </div>

      {/* Live summary & Next execution preview */}
      <div
        className={`rounded-lg px-3 py-2.5 text-xs flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 border ${
          valid
            ? 'bg-cyan-500/5 border-cyan-500/20 text-slate-700 dark:text-slate-200'
            : 'bg-red-500/10 border-red-500/30 text-red-600 dark:text-red-400'
        }`}
      >
        <div className="flex items-center gap-2 font-medium">
          {!valid && <AlertCircle className="w-4 h-4 shrink-0 text-red-500" />}
          <span>
            <span className="text-slate-400 dark:text-slate-500 mr-1">{t('schedSummaryLabel')}:</span>
            <b className={valid ? 'text-cyan-700 dark:text-cyan-300' : ''}>{summaryText}</b>
          </span>
        </div>

        {valid && nextRuns.length > 0 && (
          <div className="text-[11px] text-slate-500 dark:text-slate-400 flex flex-wrap items-center gap-1.5">
            <span>{t('schedNextRunsLabel')}:</span>
            {nextRuns.map((dt, idx) => (
              <span
                key={idx}
                className="px-1.5 py-0.5 rounded bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 font-mono text-[10px]"
              >
                {formatShortRun(dt)}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};
