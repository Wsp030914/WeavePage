import { projectMessageTypes, taskEventTypes } from '../realtime/protocol';

export const METADATA_LOCK_FIELD = 'metadata';

function eventPayload(event) {
    if (!event?.payload) return {};
    if (typeof event.payload === 'string') {
        try {
            return JSON.parse(event.payload);
        } catch {
            return {};
        }
    }
    return event.payload;
}

export function taskFromProjectEvent(event) {
    const payload = eventPayload(event);
    return payload.task || null;
}

function taskSortValue(task) {
    const sortOrder = Number(task?.sort_order);
    if (Number.isFinite(sortOrder)) return sortOrder;
    const createdAt = Date.parse(task?.created_at || '');
    return Number.isFinite(createdAt) ? createdAt : 0;
}

export function sortProjectTasks(tasks) {
    return [...tasks].sort((a, b) => {
        const scoreDiff = taskSortValue(b) - taskSortValue(a);
        if (scoreDiff !== 0) return scoreDiff;
        return Number(b?.id || 0) - Number(a?.id || 0);
    });
}

export function upsertTask(tasks, task) {
    if (!task || typeof task.id === 'undefined') return tasks;

    const next = [];
    let found = false;
    tasks.forEach((item) => {
        if (item.id === task.id) {
            next.push({ ...item, ...task });
            found = true;
        } else {
            next.push(item);
        }
    });

    if (!found) {
        next.push(task);
    }

    return sortProjectTasks(next);
}

export function removeTask(tasks, taskID) {
    return tasks.filter((task) => task.id !== taskID);
}

export function applyProjectTaskEvent(tasks, event) {
    if (!event) return tasks;

    const task = taskFromProjectEvent(event);
    switch (event.event_type) {
        case taskEventTypes.CREATED:
        case taskEventTypes.UPDATED:
            return task ? upsertTask(tasks, task) : tasks;
        case taskEventTypes.DELETED:
            return removeTask(tasks, event.task_id);
        default:
            return tasks;
    }
}

export function applyProjectTaskEvents(tasks, events) {
    if (!Array.isArray(events) || events.length === 0) return tasks;
    return events.reduce((nextTasks, event) => applyProjectTaskEvent(nextTasks, event), tasks);
}

export function applySelectedTaskEvent(selectedTask, event) {
    if (!selectedTask || !event) return selectedTask;
    if (event.task_id !== selectedTask.id) return selectedTask;

    if (event.event_type === taskEventTypes.DELETED) {
        return null;
    }

    const task = taskFromProjectEvent(event);
    return task ? { ...selectedTask, ...task } : selectedTask;
}

export function optimisticTaskUpdate(task, patch) {
    if (!task) return task;
    const version = Number(task.version);
    return {
        ...task,
        ...patch,
        version: Number.isFinite(version) && version > 0 ? version + 1 : task.version,
    };
}

export function applyTaskSnapshot(tasks, task, shouldKeepTask = () => true) {
    if (!task || typeof task.id === 'undefined') return tasks;
    if (!shouldKeepTask(task)) return removeTask(tasks, task.id);
    return upsertTask(tasks, task);
}

export function applySelectedTaskSnapshot(selectedTask, task) {
    if (!selectedTask || !task || selectedTask.id !== task.id) return selectedTask;
    return { ...selectedTask, ...task };
}

export function mergePresenceSnapshot(presenceByNode, nodeID, presence) {
    const key = String(nodeID || 'default');
    return {
        ...presenceByNode,
        [key]: Array.isArray(presence) ? presence : [],
    };
}

export function flattenPresenceUsers(presenceByNode) {
    const byUserID = new Map();
    Object.values(presenceByNode || {}).forEach((items) => {
        (Array.isArray(items) ? items : []).forEach((item) => {
            if (!item || typeof item.user_id === 'undefined') return;
            const existing = byUserID.get(item.user_id);
            const connections = Number(item.connections) || 0;
            if (existing) {
                existing.connections += connections;
                const nextTaskIDs = Array.isArray(item.viewing_task_ids) ? item.viewing_task_ids : [];
                existing.viewing_task_ids = Array.from(new Set([...(existing.viewing_task_ids || []), ...nextTaskIDs]));
                if (!existing.username && item.username) {
                    existing.username = item.username;
                }
            } else {
                byUserID.set(item.user_id, {
                    user_id: item.user_id,
                    username: item.username || `User ${item.user_id}`,
                    connections,
                    viewing_task_ids: Array.isArray(item.viewing_task_ids) ? item.viewing_task_ids : [],
                });
            }
        });
    });

    return Array.from(byUserID.values()).sort((a, b) => Number(a.user_id) - Number(b.user_id));
}

export function normalizeLockField(field = METADATA_LOCK_FIELD) {
    const normalized = String(field || '').trim().toLowerCase().replaceAll(':', '_');
    return normalized || METADATA_LOCK_FIELD;
}

export function projectLockKey(taskID, field = METADATA_LOCK_FIELD) {
    return `${Number(taskID) || 0}:${normalizeLockField(field)}`;
}

export function applyProjectLockMessage(locksByKey, message) {
    const lock = message?.lock;
    if (!lock || !lock.task_id) return locksByKey;

    const key = projectLockKey(lock.task_id, lock.field);
    if (message.type === projectMessageTypes.TASK_LOCKED) {
        return {
            ...locksByKey,
            [key]: {
                ...lock,
                field: normalizeLockField(lock.field),
            },
        };
    }

    if (message.type === projectMessageTypes.TASK_UNLOCKED) {
        const next = { ...locksByKey };
        delete next[key];
        return next;
    }

    return locksByKey;
}

export function metadataLockForTask(locksByKey, taskID) {
    return locksByKey?.[projectLockKey(taskID, METADATA_LOCK_FIELD)] || null;
}

export function isLockHeldByCurrentUser(lock, currentUserID) {
    if (!lock || !currentUserID) return false;
    return Number(lock.holder_user_id) === Number(currentUserID);
}

export function isLockHeldByOther(lock, currentUserID) {
    if (!lock) return false;
    if (!currentUserID) return true;
    return !isLockHeldByCurrentUser(lock, currentUserID);
}
