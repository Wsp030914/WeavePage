const SHANGHAI_OFFSET_MS = 8 * 60 * 60 * 1000;
const SHANGHAI_LOCAL_RE = /^(\d{4})-(\d{1,2})-(\d{1,2})(?:[T\s](\d{1,2})(?::(\d{1,2})(?::(\d{1,2})(?:\.(\d{1,3}))?)?)?)?$/;

function toValidDate(value) {
    if (!value) return null;
    const date = value instanceof Date ? value : new Date(value);
    if (Number.isNaN(date.getTime())) return null;
    return date;
}

function toShanghaiWallClock(date) {
    return new Date(date.getTime() + SHANGHAI_OFFSET_MS);
}

function parseShanghaiLocal(raw) {
    if (!raw) return null;
    const match = raw.match(SHANGHAI_LOCAL_RE);
    if (!match) return null;

    const year = Number(match[1]);
    const month = Number(match[2]);
    const day = Number(match[3]);
    const hour = Number(match[4] || 0);
    const minute = Number(match[5] || 0);
    const second = Number(match[6] || 0);
    const millisecond = Number((match[7] || '').padEnd(3, '0') || 0);

    if (month < 1 || month > 12) return null;
    if (day < 1 || day > 31) return null;
    if (hour < 0 || hour > 23) return null;
    if (minute < 0 || minute > 59) return null;
    if (second < 0 || second > 59) return null;

    return createShanghaiDate(year, month - 1, day, hour, minute, second, millisecond);
}

export function parseDateInShanghai(value) {
    if (value instanceof Date || typeof value === 'number') {
        return toValidDate(value);
    }
    if (typeof value !== 'string') return null;

    const raw = value.trim();
    if (!raw) return null;

    const normalized = raw.replace(/\//g, '-').replace(' ', 'T');
    const hasExplicitZone = /(?:[zZ]|[+-]\d{2}:?\d{2})$/.test(normalized);
    if (hasExplicitZone) {
        return toValidDate(normalized) || toValidDate(raw);
    }

    const localDate = parseShanghaiLocal(normalized) || parseShanghaiLocal(raw.replace(/\//g, '-'));
    if (localDate) return localDate;

    return toValidDate(raw);
}

export function getShanghaiParts(value) {
    const date = toValidDate(value);
    if (!date) return null;
    const wall = toShanghaiWallClock(date);
    return {
        year: wall.getUTCFullYear(),
        month: wall.getUTCMonth(),
        day: wall.getUTCDate(),
        hour: wall.getUTCHours(),
        minute: wall.getUTCMinutes(),
        second: wall.getUTCSeconds(),
        millisecond: wall.getUTCMilliseconds(),
        weekday: wall.getUTCDay(),
    };
}

export function createShanghaiDate(year, month, day, hour = 0, minute = 0, second = 0, millisecond = 0) {
    return new Date(Date.UTC(year, month, day, hour, minute, second, millisecond) - SHANGHAI_OFFSET_MS);
}

export function startOfShanghaiDay(value) {
    const parts = getShanghaiParts(value);
    if (!parts) return null;
    return createShanghaiDate(parts.year, parts.month, parts.day, 0, 0, 0, 0);
}

export function endOfShanghaiDay(value) {
    const parts = getShanghaiParts(value);
    if (!parts) return null;
    return createShanghaiDate(parts.year, parts.month, parts.day, 23, 59, 59, 999);
}

export function addShanghaiDays(value, days) {
    const parts = getShanghaiParts(value);
    if (!parts) return null;
    return createShanghaiDate(parts.year, parts.month, parts.day + days, parts.hour, parts.minute, parts.second, parts.millisecond);
}

export function getDaysInShanghaiMonth(year, month) {
    return new Date(Date.UTC(year, month + 1, 0)).getUTCDate();
}

export function addShanghaiMonths(value, months) {
    const parts = getShanghaiParts(value);
    if (!parts) return null;

    const baseMonth = parts.month + months;
    const targetYear = parts.year + Math.floor(baseMonth / 12);
    const targetMonth = ((baseMonth % 12) + 12) % 12;
    const maxDay = getDaysInShanghaiMonth(targetYear, targetMonth);
    const day = Math.min(parts.day, maxDay);

    return createShanghaiDate(targetYear, targetMonth, day, parts.hour, parts.minute, parts.second, parts.millisecond);
}

export function addShanghaiYears(value, years) {
    return addShanghaiMonths(value, years * 12);
}

export function getShanghaiDateKey(value) {
    const parts = getShanghaiParts(value);
    if (!parts) return '';
    const y = String(parts.year);
    const m = String(parts.month + 1).padStart(2, '0');
    const d = String(parts.day).padStart(2, '0');
    return `${y}-${m}-${d}`;
}

export function isSameShanghaiDay(a, b) {
    return getShanghaiDateKey(a) === getShanghaiDateKey(b);
}

export function getShanghaiWeekStart(value) {
    const parts = getShanghaiParts(value);
    if (!parts) return null;
    const mondayOffset = (parts.weekday + 6) % 7;
    return startOfShanghaiDay(addShanghaiDays(value, -mondayOffset));
}

export function getShanghaiMonthGrid(value) {
    const parts = getShanghaiParts(value);
    if (!parts) return [];

    const firstDay = createShanghaiDate(parts.year, parts.month, 1, 0, 0, 0, 0);
    const firstWeekday = getShanghaiParts(firstDay)?.weekday ?? 1;
    const mondayOffset = (firstWeekday + 6) % 7;
    const gridStart = addShanghaiDays(firstDay, -mondayOffset);

    const cells = [];
    for (let i = 0; i < 42; i += 1) {
        cells.push(addShanghaiDays(gridStart, i));
    }
    return cells;
}

export function formatShanghaiDateTime(value, options = {}) {
    const date = parseDateInShanghai(value) || toValidDate(value);
    if (!date) return '';
    return new Intl.DateTimeFormat('zh-CN', {
        timeZone: 'Asia/Shanghai',
        ...options,
    }).format(date);
}

export function getTaskCreateDate(task) {
    return parseDateInShanghai(task?.created_at || null);
}
