import { parseDateInShanghai } from './shanghaiTime';

function parseDueDate(rawDueAt) {
    return parseDateInShanghai(rawDueAt);
}

export function filterTasksByDueRange(tasks, start, end) {
    if (!Array.isArray(tasks) || !start || !end) return [];
    const rangeStart = parseDateInShanghai(start);
    const rangeEnd = parseDateInShanghai(end);
    if (!rangeStart || !rangeEnd) return [];

    const startTime = rangeStart.getTime();
    const endTime = rangeEnd.getTime();

    return tasks.filter((task) => {
        const due = parseDueDate(task.due_at);
        if (!due) return false;
        const dueTime = due.getTime();
        return dueTime >= startTime && dueTime <= endTime;
    });
}
