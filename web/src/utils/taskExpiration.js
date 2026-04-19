import { parseDateInShanghai } from './shanghaiTime';

export function isTaskExpired(task, now = new Date()) {
    if (!task || task.status === 'done' || !task.due_at) return false;
    const due = parseDateInShanghai(task.due_at);
    if (!due) return false;
    return due.getTime() < now.getTime();
}

export function splitTasksByLifecycle(tasks, now = new Date()) {
    const expired = [];
    const todo = [];
    const done = [];

    (tasks || []).forEach((task) => {
        if (task?.status === 'done') {
            done.push(task);
            return;
        }
        if (isTaskExpired(task, now)) {
            expired.push(task);
            return;
        }
        todo.push(task);
    });

    return { expired, todo, done };
}

