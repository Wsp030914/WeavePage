export const taskDocTypes = {
    DOCUMENT: 'document',
    MEETING: 'meeting',
    DIARY: 'diary',
    TODO: 'todo',
};

export function getTaskDocType(task) {
    return String(task?.doc_type || taskDocTypes.DOCUMENT).trim().toLowerCase();
}

export function isTodoTask(task) {
    return getTaskDocType(task) === taskDocTypes.TODO;
}

export function isDiaryTask(task) {
    return getTaskDocType(task) === taskDocTypes.DIARY;
}

export function isMeetingTask(task) {
    return getTaskDocType(task) === taskDocTypes.MEETING;
}

export function getTaskDocTypeLabel(task) {
    const docType = getTaskDocType(task);

    if (docType === taskDocTypes.TODO) return 'Todo';
    if (docType === taskDocTypes.DIARY) return 'Daily Note';
    if (docType === taskDocTypes.MEETING) return 'Meeting Note';
    return 'Document';
}

export function getTaskStatusLabel(task) {
    if (task?.status === 'done') {
        return isTodoTask(task) ? 'Done' : 'Archived';
    }
    return isTodoTask(task) ? 'Open' : 'Active';
}
