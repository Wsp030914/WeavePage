export const projectMessageTypes = {
    SYNC: 'PROJECT_SYNC',
    INIT: 'PROJECT_INIT',
    ERROR: 'PROJECT_ERROR',
    PING: 'PING',
    PONG: 'PONG',
    TASK_CREATED: 'TASK_CREATED',
    TASK_UPDATED: 'TASK_UPDATED',
    TASK_DELETED: 'TASK_DELETED',
    PRESENCE_SNAPSHOT: 'PRESENCE_SNAPSHOT',
    LOCK_REQUEST: 'LOCK_REQUEST',
    LOCK_RELEASE: 'LOCK_RELEASE',
    VIEW_DOCUMENT: 'VIEW_DOCUMENT',
    TASK_LOCKED: 'TASK_LOCKED',
    TASK_UNLOCKED: 'TASK_UNLOCKED',
    LOCK_ERROR: 'LOCK_ERROR',
};

export const taskEventTypes = {
    CREATED: 'task.created',
    UPDATED: 'task.updated',
    DELETED: 'task.deleted',
};

export function isProjectTaskMessage(type) {
    return type === projectMessageTypes.TASK_CREATED
        || type === projectMessageTypes.TASK_UPDATED
        || type === projectMessageTypes.TASK_DELETED;
}
