import { Lang } from '../i18n/translations';

export type ScheduleMode =
  | 'minutes'
  | 'hourly'
  | 'daily'
  | 'multi_daily'
  | 'weekly'
  | 'monthly'
  | 'advanced';

export interface ParsedSchedule {
  mode: ScheduleMode;
  everyMinutes: number;       // 5..59 for 'minutes'
  everyHours: number;         // 1..23 for 'hourly'
  minuteAtHour: number;       // 0..59 for 'hourly'
  dailyTime: string;          // 'HH:MM' for 'daily', 'weekly', 'monthly'
  multiTimes: string[];       // ['13:00', '21:00'] for 'multi_daily' (all share minute or same minute preferred; if different minutes wait - cron only supports multiple hours sharing the same minute in a single 5-field cron! Let's handle same minute or explain/normalize)
  daysOfWeek: number[];       // 0..6 (0 = Sunday, 1 = Monday, ..., 6 = Saturday) for 'weekly'
  dayOfMonth: number;         // 1..28 (or 1..31) for 'monthly'
  rawCron: string;            // fallback / current cron
}

const DEFAULT_PARSED: ParsedSchedule = {
  mode: 'daily',
  everyMinutes: 30,
  everyHours: 4,
  minuteAtHour: 0,
  dailyTime: '02:00',
  multiTimes: ['13:00', '21:00'],
  daysOfWeek: [1, 2, 3, 4, 5],
  dayOfMonth: 1,
  rawCron: '0 2 * * *',
};

function pad2(n: number): string {
  return String(Math.max(0, n)).padStart(2, '0');
}

function isIntInRange(s: string, min: number, max: number): boolean {
  if (!/^\d+$/.test(s)) return false;
  const n = Number(s);
  return Number.isInteger(n) && n >= min && n <= max;
}

/**
 * Parses a standard 5-field cron expression into a friendly UI state.
 * Falls back to 'advanced' mode if the expression uses syntax outside the visual modes.
 */
export function parseCronToSchedule(cronExpr: string): ParsedSchedule {
  const trimmed = (cronExpr || '').trim();
  const base: ParsedSchedule = {
    ...DEFAULT_PARSED,
    multiTimes: [...DEFAULT_PARSED.multiTimes],
    daysOfWeek: [...DEFAULT_PARSED.daysOfWeek],
    rawCron: trimmed || '0 2 * * *',
  };

  if (!trimmed) {
    return base;
  }

  const parts = trimmed.split(/\s+/);
  if (parts.length !== 5) {
    return { ...base, mode: 'advanced' };
  }

  const [minPart, hourPart, domPart, monPart, dowPart] = parts;

  // All visual modes use '*' for month
  if (monPart !== '*') {
    return { ...base, mode: 'advanced' };
  }

  // 1. Every N minutes: */N * * * *
  const everyMinMatch = minPart.match(/^\*\/(\d+)$/);
  if (everyMinMatch && hourPart === '*' && domPart === '*' && dowPart === '*') {
    const mins = Number(everyMinMatch[1]);
    if (mins >= 1 && mins <= 59) {
      return {
        ...base,
        mode: 'minutes',
        everyMinutes: mins,
      };
    }
  }

  // 2. Hourly / Every N hours: M * * * * or M */H * * *
  if (isIntInRange(minPart, 0, 59) && domPart === '*' && dowPart === '*') {
    const m = Number(minPart);
    if (hourPart === '*' || hourPart === '*/1') {
      return {
        ...base,
        mode: 'hourly',
        everyHours: 1,
        minuteAtHour: m,
      };
    }
    const everyHourMatch = hourPart.match(/^\*\/(\d+)$/);
    if (everyHourMatch) {
      const h = Number(everyHourMatch[1]);
      if (h >= 1 && h <= 23) {
        return {
          ...base,
          mode: 'hourly',
          everyHours: h,
          minuteAtHour: m,
        };
      }
    }
  }

  // 3. Daily once: M H * * *
  if (isIntInRange(minPart, 0, 59) && isIntInRange(hourPart, 0, 23) && domPart === '*' && dowPart === '*') {
    const m = Number(minPart);
    const h = Number(hourPart);
    return {
      ...base,
      mode: 'daily',
      dailyTime: `${pad2(h)}:${pad2(m)}`,
    };
  }

  // 4. Multiple times per day (same minute across hours): M H1,H2,... * * *
  if (isIntInRange(minPart, 0, 59) && /^\d+(,\d+)+$/.test(hourPart) && domPart === '*' && dowPart === '*') {
    const m = Number(minPart);
    const hours = hourPart.split(',').map(Number);
    if (hours.every((h) => h >= 0 && h <= 23)) {
      const uniqueSorted = Array.from(new Set(hours)).sort((a, b) => a - b);
      return {
        ...base,
        mode: 'multi_daily',
        minuteAtHour: m,
        multiTimes: uniqueSorted.map((h) => `${pad2(h)}:${pad2(m)}`),
      };
    }
  }

  // 5. Weekly on specific days: M H * * D1,D2... or D1-D2
  if (isIntInRange(minPart, 0, 59) && isIntInRange(hourPart, 0, 23) && domPart === '*' && dowPart !== '*') {
    const parsedDays = parseDowField(dowPart);
    if (parsedDays && parsedDays.length > 0) {
      const m = Number(minPart);
      const h = Number(hourPart);
      return {
        ...base,
        mode: 'weekly',
        dailyTime: `${pad2(h)}:${pad2(m)}`,
        daysOfWeek: parsedDays,
      };
    }
  }

  // 6. Monthly on day D: M H D * *
  if (isIntInRange(minPart, 0, 59) && isIntInRange(hourPart, 0, 23) && isIntInRange(domPart, 1, 31) && dowPart === '*') {
    const m = Number(minPart);
    const h = Number(hourPart);
    const dom = Number(domPart);
    return {
      ...base,
      mode: 'monthly',
      dailyTime: `${pad2(h)}:${pad2(m)}`,
      dayOfMonth: dom,
    };
  }

  return { ...base, mode: 'advanced' };
}

function parseDowField(dowPart: string): number[] | null {
  const days = new Set<number>();
  const tokens = dowPart.split(',');
  for (const tok of tokens) {
    if (/^\d+$/.test(tok)) {
      let d = Number(tok);
      if (d === 7) d = 0; // cron allows 7 for Sunday
      if (d < 0 || d > 6) return null;
      days.add(d);
    } else {
      const rangeMatch = tok.match(/^(\d+)-(\d+)$/);
      if (!rangeMatch) return null;
      const start = Number(rangeMatch[1]);
      const end = Number(rangeMatch[2]);
      if (start > end || start < 0 || end > 7) return null;
      for (let i = start; i <= end; i++) {
        days.add(i === 7 ? 0 : i);
      }
    }
  }
  return Array.from(days).sort((a, b) => a - b);
}

function parseHHMM(timeStr: string): { hour: number; minute: number } {
  const [hStr, mStr] = (timeStr || '02:00').split(':');
  const hour = Math.min(23, Math.max(0, Number(hStr) || 0));
  const minute = Math.min(59, Math.max(0, Number(mStr) || 0));
  return { hour, minute };
}

/**
 * Converts a ParsedSchedule state back into a valid 5-field cron expression.
 */
export function buildCronFromSchedule(sched: ParsedSchedule): string {
  switch (sched.mode) {
    case 'minutes': {
      const m = Math.min(59, Math.max(1, Math.round(sched.everyMinutes || 15)));
      return m === 1 ? '* * * * *' : `*/${m} * * * *`;
    }
    case 'hourly': {
      const h = Math.min(23, Math.max(1, Math.round(sched.everyHours || 1)));
      const min = Math.min(59, Math.max(0, Math.round(sched.minuteAtHour || 0)));
      return h === 1 ? `${min} * * * *` : `${min} */${h} * * *`;
    }
    case 'daily': {
      const { hour, minute } = parseHHMM(sched.dailyTime);
      return `${minute} ${hour} * * *`;
    }
    case 'multi_daily': {
      const validTimes = sched.multiTimes.length > 0 ? sched.multiTimes : ['13:00', '21:00'];
      const parsed = validTimes.map(parseHHMM);
      const sharedMin = Math.min(59, Math.max(0, sched.minuteAtHour ?? parsed[0].minute));
      const uniqueHours = Array.from(new Set(parsed.map((p) => p.hour))).sort((a, b) => a - b);
      if (uniqueHours.length === 1) {
        return `${sharedMin} ${uniqueHours[0]} * * *`;
      }
      return `${sharedMin} ${uniqueHours.join(',')} * * *`;
    }
    case 'weekly': {
      const { hour, minute } = parseHHMM(sched.dailyTime);
      const days = sched.daysOfWeek.length > 0
        ? Array.from(new Set(sched.daysOfWeek)).sort((a, b) => a - b)
        : [0];
      return `${minute} ${hour} * * ${days.join(',')}`;
    }
    case 'monthly': {
      const { hour, minute } = parseHHMM(sched.dailyTime);
      const dom = Math.min(31, Math.max(1, Math.round(sched.dayOfMonth || 1)));
      return `${minute} ${hour} ${dom} * *`;
    }
    case 'advanced':
    default:
      return sched.rawCron.trim();
  }
}

const DAY_NAMES_SHORT: Record<Lang, string[]> = {
  es: ['Dom', 'Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb'],
  en: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'],
};

const DAY_NAMES_FULL: Record<Lang, string[]> = {
  es: ['Domingo', 'Lunes', 'Martes', 'Miércoles', 'Jueves', 'Viernes', 'Sábado'],
  en: ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'],
};

export function getDayShortLabels(lang: Lang): { day: number; label: string }[] {
  // Order starting Monday (1..6, 0) for natural calendar feel
  const order = [1, 2, 3, 4, 5, 6, 0];
  return order.map((d) => ({ day: d, label: DAY_NAMES_SHORT[lang][d] }));
}

/**
 * Generates a clear human-readable description of any cron expression in ES or EN.
 */
export function describeCron(cronExpr: string, lang: Lang): string {
  const trimmed = (cronExpr || '').trim();
  if (!trimmed) return '—';

  if (trimmed === '* * * * *') {
    return lang === 'es' ? 'Cada minuto' : 'Every minute';
  }

  const parsed = parseCronToSchedule(trimmed);
  switch (parsed.mode) {
    case 'minutes':
      return lang === 'es'
        ? `Cada ${parsed.everyMinutes} minutos`
        : `Every ${parsed.everyMinutes} minutes`;

    case 'hourly': {
      const minStr = pad2(parsed.minuteAtHour);
      if (parsed.everyHours === 1) {
        return parsed.minuteAtHour === 0
          ? lang === 'es'
            ? 'Cada hora en punto'
            : 'Every hour on the hour'
          : lang === 'es'
          ? `Cada hora en el minuto :${minStr}`
          : `Every hour at minute :${minStr}`;
      }
      return parsed.minuteAtHour === 0
        ? lang === 'es'
          ? `Cada ${parsed.everyHours} horas`
          : `Every ${parsed.everyHours} hours`
        : lang === 'es'
        ? `Cada ${parsed.everyHours} horas (en el minuto :${minStr})`
        : `Every ${parsed.everyHours} hours (at minute :${minStr})`;
    }

    case 'daily':
      return lang === 'es'
        ? `Todos los días a las ${parsed.dailyTime}`
        : `Every day at ${parsed.dailyTime}`;

    case 'multi_daily': {
      const formattedTimes = parsed.multiTimes
        .map((t) => {
          const { hour } = parseHHMM(t);
          return `${pad2(hour)}:${pad2(parsed.minuteAtHour)}`;
        })
        .join(', ');
      return lang === 'es'
        ? `Todos los días a las ${formattedTimes}`
        : `Every day at ${formattedTimes}`;
    }

    case 'weekly': {
      const sorted = [...parsed.daysOfWeek].sort((a, b) => {
        const oa = a === 0 ? 7 : a;
        const ob = b === 0 ? 7 : b;
        return oa - ob;
      });
      const isWeekdays =
        sorted.length === 5 && sorted.every((v, idx) => v === idx + 1);
      const isWeekends =
        sorted.length === 2 && sorted[0] === 6 && sorted[1] === 0;

      if (isWeekdays) {
        return lang === 'es'
          ? `Lunes a Viernes a las ${parsed.dailyTime}`
          : `Monday to Friday at ${parsed.dailyTime}`;
      }
      if (isWeekends) {
        return lang === 'es'
          ? `Fines de semana (Sáb y Dom) a las ${parsed.dailyTime}`
          : `Weekends (Sat & Sun) at ${parsed.dailyTime}`;
      }
      if (sorted.length === 1) {
        const dayName = DAY_NAMES_FULL[lang][sorted[0]];
        return lang === 'es'
          ? `Cada ${dayName} a las ${parsed.dailyTime}`
          : `Every ${dayName} at ${parsed.dailyTime}`;
      }
      const names = sorted.map((d) => DAY_NAMES_SHORT[lang][d]).join(', ');
      return lang === 'es'
        ? `Semanal (${names}) a las ${parsed.dailyTime}`
        : `Weekly (${names}) at ${parsed.dailyTime}`;
    }

    case 'monthly':
      return lang === 'es'
        ? `El día ${parsed.dayOfMonth} de cada mes a las ${parsed.dailyTime}`
        : `On day ${parsed.dayOfMonth} of every month at ${parsed.dailyTime}`;

    case 'advanced':
    default:
      return lang === 'es'
        ? `Programación personalizada (${trimmed})`
        : `Custom schedule (${trimmed})`;
  }
}

function matchCronField(val: number, field: string, isDow = false): boolean {
  if (field === '*') return true;

  for (const part of field.split(',')) {
    const stepParts = part.split('/');
    const rangePart = stepParts[0];
    const step = stepParts.length > 1 ? Number(stepParts[1]) : 1;
    if (!Number.isInteger(step) || step <= 0) return false;

    if (rangePart === '*') {
      if (val % step === 0) return true;
      continue;
    }

    if (rangePart.includes('-')) {
      const [sStr, eStr] = rangePart.split('-');
      const start = Number(sStr);
      const end = Number(eStr);
      if (Number.isNaN(start) || Number.isNaN(end)) return false;
      const checkVal = isDow && val === 0 && end === 7 ? 7 : val;
      if (checkVal >= start && checkVal <= end && (checkVal - start) % step === 0) {
        return true;
      }
    } else {
      let target = Number(rangePart);
      if (Number.isNaN(target)) return false;
      if (isDow && target === 7) target = 0;
      if (val === target) return true;
    }
  }
  return false;
}

export function isValidFiveFieldCron(cronExpr: string): boolean {
  const trimmed = (cronExpr || '').trim();
  const parts = trimmed.split(/\s+/);
  if (parts.length !== 5) return false;

  const ranges: [number, number][] = [
    [0, 59], // minute
    [0, 23], // hour
    [1, 31], // dom
    [1, 12], // month
    [0, 7],  // dow
  ];

  for (let i = 0; i < 5; i++) {
    const field = parts[i];
    const [min, max] = ranges[i];
    if (!field) return false;
    for (const token of field.split(',')) {
      const stepMatch = token.match(/^(\*|\d+(?:-\d+)?)(?:\/(\d+))?$/);
      if (!stepMatch) return false;
      const base = stepMatch[1];
      const stepStr = stepMatch[2];
      if (stepStr !== undefined) {
        const step = Number(stepStr);
        if (step < 1 || step > max) return false;
      }
      if (base !== '*') {
        if (base.includes('-')) {
          const [a, b] = base.split('-').map(Number);
          if (a < min || b > max || a > b) return false;
        } else {
          const n = Number(base);
          if (n < min || n > max) return false;
        }
      }
    }
  }
  return true;
}

/**
 * Computes the next `count` upcoming execution Date objects for a valid 5-field cron expression.
 */
export function getNextCronRuns(cronExpr: string, count = 3, fromDate = new Date()): Date[] {
  if (!isValidFiveFieldCron(cronExpr)) return [];
  const [minF, hourF, domF, monF, dowF] = cronExpr.trim().split(/\s+/);

  const results: Date[] = [];
  const cursor = new Date(fromDate.getTime());
  cursor.setSeconds(0, 0);
  cursor.setMinutes(cursor.getMinutes() + 1);

  // Cap iterations (up to ~60 days in minutes = 86400; for monthly schedules check up to 120 days = 172800)
  const maxChecks = 180000;
  for (let i = 0; i < maxChecks && results.length < count; i++) {
    const mon = cursor.getMonth() + 1;
    if (!matchCronField(mon, monF)) {
      // Jump to start of next month
      cursor.setMonth(cursor.getMonth() + 1, 1);
      cursor.setHours(0, 0, 0, 0);
      continue;
    }

    const dom = cursor.getDate();
    const dow = cursor.getDay();
    // Standard cron rule: if both DOM and DOW are restricted (not *), match if EITHER matches; otherwise match both
    const domRestricted = domF !== '*';
    const dowRestricted = dowF !== '*';
    const dayMatches =
      domRestricted && dowRestricted
        ? matchCronField(dom, domF) || matchCronField(dow, dowF, true)
        : matchCronField(dom, domF) && matchCronField(dow, dowF, true);

    if (!dayMatches) {
      cursor.setDate(cursor.getDate() + 1);
      cursor.setHours(0, 0, 0, 0);
      continue;
    }

    const hr = cursor.getHours();
    if (!matchCronField(hr, hourF)) {
      cursor.setHours(cursor.getHours() + 1, 0, 0, 0);
      continue;
    }

    const mn = cursor.getMinutes();
    if (matchCronField(mn, minF)) {
      results.push(new Date(cursor.getTime()));
    }
    cursor.setMinutes(cursor.getMinutes() + 1);
  }

  return results;
}
