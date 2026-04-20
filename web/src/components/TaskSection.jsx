import React from 'react';
import { isLockHeldByCurrentUser, isLockHeldByOther, metadataLockForTask } from '../store/collab-store';
import { isTaskExpired } from '../utils/taskExpiration';
import { getShanghaiParts, parseDateInShanghai } from '../utils/shanghaiTime';
import Button from './Button';
import './TaskSection.css';

function formatDueDate(rawDue) {
    if (!rawDue) return null;
    const date = parseDateInShanghai(rawDue);
    const parts = getShanghaiParts(date);
    if (!parts) return null;

    const month = String(parts.month + 1).padStart(2, '0');
    const day = String(parts.day).padStart(2, '0');
    const hour = String(parts.hour).padStart(2, '0');
    const minute = String(parts.minute).padStart(2, '0');
    return `${parts.year}-${month}-${day} ${hour}:${minute}`;
}

export default function TaskSection({
    title,
    tasks,
    emptyText,
    onToggleStatus,
    onOpenDetails,
    onDeleteTask,
    onOpenProject,
    locksByKey = {},
    currentUserID = 0,
    completeLabel = 'Archive',
    restoreLabel = 'Restore',
    projectLabel = 'Space',
    detailsLabel = 'Open',
    completeAriaLabel = 'Mark as done',
    restoreAriaLabel = 'Mark as todo',
    expiredLabel = 'Expired',
    dueLabel = 'Due',
    deleteLabel = 'Trash',
}) {
    return (
        <section className="yq-task-section">
            <div className="yq-task-section-header">
                <h3>{title}</h3>
                <span>{tasks.length}</span>
            </div>

            {tasks.length === 0 ? (
                <div className="yq-empty-state">{emptyText}</div>
            ) : (
                <div className="yq-task-stack">
                    {tasks.map((task) => {
                        const metadataLock = metadataLockForTask(locksByKey, task.id);
                        const lockedByOther = isLockHeldByOther(metadataLock, currentUserID);
                        const lockedBySelf = isLockHeldByCurrentUser(metadataLock, currentUserID);
                        const lockHolder = metadataLock?.holder_username || `User ${metadataLock?.holder_user_id || ''}`.trim();
                        const lockTitle = lockedByOther ? `Locked by ${lockHolder}` : 'Locked by you';

                        return (
                            <article
                                key={task.id}
                                className={`yq-task-row ${task.status === 'done' ? 'is-done' : ''} ${isTaskExpired(task) ? 'is-expired' : ''} ${metadataLock ? 'is-locked' : ''}`}
                            >
                                <button
                                    type="button"
                                    className="yq-task-toggle"
                                    aria-label={task.status === 'done' ? restoreAriaLabel : completeAriaLabel}
                                    onClick={() => onToggleStatus(task)}
                                    disabled={lockedByOther}
                                    title={lockedByOther ? lockTitle : undefined}
                                >
                                    {task.status === 'done' ? restoreLabel : completeLabel}
                                </button>

                                <div className="yq-task-main" onClick={() => onOpenDetails(task)}>
                                    <div className="yq-task-title">{task.title}</div>
                                    <div className="yq-task-meta">
                                        {isTaskExpired(task) && <span className="yq-tag yq-tag-expired">{expiredLabel}</span>}
                                        {task.priority > 0 && <span className="yq-tag">P{task.priority}</span>}
                                        {metadataLock && (
                                            <span className={`yq-tag yq-tag-lock ${lockedBySelf ? 'self' : 'other'}`} title={lockTitle}>
                                                {lockedBySelf ? 'Locked by you' : `Locked by ${lockHolder}`}
                                            </span>
                                        )}
                                        {task.project_id > 0 && <span>Space #{task.project_id}</span>}
                                        {formatDueDate(task.due_at) && <span>{dueLabel} {formatDueDate(task.due_at)}</span>}
                                    </div>
                                </div>

                                <div className="yq-task-actions">
                                    {onOpenProject && (
                                        <Button variant="secondary" onClick={() => onOpenProject(task)}>
                                            {projectLabel}
                                        </Button>
                                    )}
                                    <Button variant="secondary" onClick={() => onOpenDetails(task)}>
                                        {detailsLabel}
                                    </Button>
                                    <Button
                                        variant="danger"
                                        onClick={() => onDeleteTask(task)}
                                        disabled={lockedByOther}
                                        title={lockedByOther ? lockTitle : undefined}
                                    >
                                        {deleteLabel}
                                    </Button>
                                </div>
                            </article>
                        );
                    })}
                </div>
            )}
        </section>
    );
}
