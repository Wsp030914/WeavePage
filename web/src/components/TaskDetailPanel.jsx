import React, { useCallback, useEffect, useRef, useState } from 'react';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { addTaskMember, removeTaskMember, saveDocumentContent, updateTask } from '../api/task';
import { contentConnectionStatus, YjsTaskContentProvider } from '../realtime/yjsTaskContentProvider';
import {
    METADATA_LOCK_FIELD,
    isLockHeldByCurrentUser,
    isLockHeldByOther,
} from '../store/collab-store';
import { getShanghaiParts, parseDateInShanghai } from '../utils/shanghaiTime';
import Avatar from './Avatar';
import Button from './Button';
import DocumentMarkdownEditor from './DocumentMarkdownEditor';
import Input from './Input';
import './TaskDetailPanel.css';

marked.setOptions({
    breaks: true,
    gfm: true,
});

function memberDisplay(member) {
    const username = member.user?.username || member.user_username || member.username || `User ${member.user_id}`;
    const avatarURL = member.user?.avatar_url || member.user_avatar_url || member.avatar_url || '';
    return { username, avatarURL };
}

function toShanghaiDateTimeInputValue(value) {
    const date = parseDateInShanghai(value);
    const parts = getShanghaiParts(date);
    if (!parts) return '';

    const month = String(parts.month + 1).padStart(2, '0');
    const day = String(parts.day).padStart(2, '0');
    const hour = String(parts.hour).padStart(2, '0');
    const minute = String(parts.minute).padStart(2, '0');
    return `${parts.year}-${month}-${day}T${hour}:${minute}`;
}

function formatDueForPanel(value) {
    const date = parseDateInShanghai(value);
    const parts = getShanghaiParts(date);
    if (!parts) return '';

    const month = String(parts.month + 1).padStart(2, '0');
    const day = String(parts.day).padStart(2, '0');
    const hour = String(parts.hour).padStart(2, '0');
    const minute = String(parts.minute).padStart(2, '0');
    const second = String(parts.second).padStart(2, '0');
    return `${parts.year}-${month}-${day} ${hour}:${minute}:${second}`;
}

function splitDueDateTimeInput(value) {
    const raw = String(value || '').trim();
    if (!raw) return { datePart: '', timePart: '' };
    const normalized = raw.replace(' ', 'T');
    if (normalized.length >= 16) {
        return {
            datePart: normalized.slice(0, 10),
            timePart: normalized.slice(11, 16),
        };
    }
    return { datePart: normalized.slice(0, 10), timePart: '' };
}

function mergeDueDateTimeInput(datePart, timePart) {
    const safeDate = String(datePart || '').trim();
    if (!safeDate) return '';
    const safeTime = String(timePart || '').trim() || '00:00';
    return `${safeDate}T${safeTime}`;
}

export default function TaskDetailPanel({
    isOpen,
    onClose,
    task,
    project,
    onTaskUpdated,
    currentUserID = 0,
    taskLock = null,
    lockError = '',
    onLockRequest,
    onLockRelease,
}) {
    const [isEditing, setIsEditing] = useState(false);
    const [saving, setSaving] = useState(false);
    const [editData, setEditData] = useState(null);
    const [contentStatus, setContentStatus] = useState(contentConnectionStatus.DISCONNECTED);
    const [contentError, setContentError] = useState('');
    const [contentProvider, setContentProvider] = useState(null);
    const contentProviderRef = useRef(null);
    const editingLockRef = useRef(null);

    const [showMemberForm, setShowMemberForm] = useState(false);
    const [memberEmail, setMemberEmail] = useState('');
    const [memberRole, setMemberRole] = useState('viewer');
    const [addingMember, setAddingMember] = useState(false);

    const releaseMetadataLock = useCallback(() => {
        const held = editingLockRef.current;
        if (held?.taskID) {
            onLockRelease?.(held.taskID, held.field);
        }
        editingLockRef.current = null;
    }, [onLockRelease]);

    useEffect(() => {
        if (!task) return;
        setEditData({
            title: task.title || '',
            content_md: task.content_md || '',
            priority: task.priority || 0,
            due_at: toShanghaiDateTimeInputValue(task.due_at),
        });
        setIsEditing(false);
        setShowMemberForm(false);
    }, [task]);

    useEffect(() => () => {
        releaseMetadataLock();
    }, [task?.id, releaseMetadataLock]);

    useEffect(() => {
        contentProviderRef.current?.destroy();
        contentProviderRef.current = null;
        setContentProvider(null);
        setContentStatus(contentConnectionStatus.DISCONNECTED);
        setContentError('');

        if (!isOpen || !task?.id || task.doc_type === 'diary') return undefined;

        const provider = new YjsTaskContentProvider({
            taskId: task.id,
            initialContent: task.content_md || '',
            onTextChange: (text) => {
                setEditData((prev) => (prev ? { ...prev, content_md: text } : prev));
            },
            onStatusChange: setContentStatus,
            onError: setContentError,
        });
        contentProviderRef.current = provider;
        setContentProvider(provider);

        return () => {
            provider.destroy();
            if (contentProviderRef.current === provider) {
                contentProviderRef.current = null;
            }
            setContentProvider((prev) => (prev === provider ? null : prev));
        };
    }, [isOpen, task?.id, task?.content_md, task?.doc_type]);

    useEffect(() => {
        if (lockError && editingLockRef.current?.taskID === task?.id) {
            editingLockRef.current = null;
        }
    }, [lockError, task?.id]);

    const onDocumentContentChange = useCallback((nextContent) => {
        setEditData((prev) => (prev ? { ...prev, content_md: nextContent } : prev));
    }, []);

    if (!isOpen || !task || !editData) return null;
    const dueParts = splitDueDateTimeInput(editData.due_at);
    const diaryDocument = task.doc_type === 'diary';
    const privateDocument = task.collaboration_mode === 'private';
    const metadataLockEnabled = !diaryDocument && typeof onLockRequest === 'function' && typeof onLockRelease === 'function';
    const lockedBySelf = metadataLockEnabled && isLockHeldByCurrentUser(taskLock, currentUserID);
    const lockedByOther = metadataLockEnabled && isLockHeldByOther(taskLock, currentUserID);
    const awaitingMetadataLock = metadataLockEnabled && isEditing && !lockedBySelf && !lockedByOther && !lockError;
    const metadataLockBlocked = metadataLockEnabled && (lockedByOther || awaitingMetadataLock || Boolean(lockError));
    const lockHolder = taskLock?.holder_username || `User ${taskLock?.holder_user_id || ''}`.trim();
    const lockStatusText = (metadataLockEnabled && lockError)
        || (lockedByOther ? `Metadata is locked by ${lockHolder}. You can still edit the live document body.` : '')
        || (lockedBySelf ? 'Metadata lock held by you.' : '')
        || (awaitingMetadataLock ? 'Waiting for metadata lock...' : '');

    const beginEdit = () => {
        if (metadataLockEnabled && !lockedByOther && !lockedBySelf) {
            const sent = onLockRequest(task.id, METADATA_LOCK_FIELD);
            if (!sent) {
                alert('Space realtime connection is not ready. Please retry after it reconnects.');
                return;
            }
            editingLockRef.current = { taskID: task.id, field: METADATA_LOCK_FIELD };
        } else if (lockedBySelf) {
            editingLockRef.current = { taskID: task.id, field: METADATA_LOCK_FIELD };
        }
        setIsEditing(true);
    };

    const cancelEdit = () => {
        setIsEditing(false);
        releaseMetadataLock();
    };

    const closePanel = () => {
        releaseMetadataLock();
        onClose?.();
    };

    const onSave = async () => {
        if (metadataLockBlocked) {
            alert(lockStatusText || 'Document metadata is locked.');
            return;
        }
        setSaving(true);
        try {
            const payload = {
                title: editData.title.trim(),
                priority: Number(editData.priority) || 0,
            };
            const nextTask = {
                ...task,
                title: payload.title,
                priority: payload.priority,
            };

            if (editData.due_at) {
                const dueDate = parseDateInShanghai(editData.due_at);
                if (!dueDate) {
                    alert('Invalid due date');
                    return;
                }
                payload.due_at = dueDate.toISOString();
                nextTask.due_at = payload.due_at;
            } else {
                payload.clear_due_at = true;
                nextTask.due_at = null;
            }
            let expectedVersion = typeof task.version === 'number' ? task.version : undefined;
            await updateTask(task.project_id, task.id, payload, expectedVersion);
            if (typeof expectedVersion === 'number') {
                expectedVersion += 1;
                nextTask.version = expectedVersion;
            }

            if (diaryDocument) {
                const savedTask = await saveDocumentContent(task.id, {
                    content_md: editData.content_md,
                }, expectedVersion);
                nextTask.content_md = savedTask?.content_md ?? editData.content_md;
                if (typeof savedTask?.version === 'number') {
                    nextTask.version = savedTask.version;
                } else if (typeof expectedVersion === 'number') {
                    nextTask.version = expectedVersion + 1;
                }
            }

            setIsEditing(false);
            releaseMetadataLock();
            if (onTaskUpdated) await onTaskUpdated(nextTask);
        } catch (err) {
            alert(err.message || 'Failed to save document metadata');
        } finally {
            setSaving(false);
        }
    };

    const onStatusToggle = async (checked) => {
        if (lockedByOther) {
            alert(lockStatusText || 'Document metadata is locked.');
            return;
        }
        const nextStatus = checked ? 'done' : 'todo';
        try {
            await updateTask(task.project_id, task.id, { status: nextStatus }, task.version);
            const nextTask = {
                ...task,
                status: nextStatus,
            };
            if (typeof task.version === 'number') {
                nextTask.version = task.version + 1;
            }
            if (onTaskUpdated) await onTaskUpdated(nextTask);
        } catch (err) {
            alert(err.message || 'Failed to update document status');
        }
    };

    const onAddMember = async () => {
        if (!memberEmail.trim()) return;
        setAddingMember(true);
        try {
            await addTaskMember(task.project_id, task.id, {
                email: memberEmail.trim(),
                role: memberRole,
            });
            setMemberEmail('');
            setMemberRole('viewer');
            setShowMemberForm(false);
            if (onTaskUpdated) await onTaskUpdated();
        } catch (err) {
            alert(err.message || 'Failed to add member');
        } finally {
            setAddingMember(false);
        }
    };

    const onRemoveMember = async (userID) => {
        if (!window.confirm('Remove this member?')) return;
        try {
            await removeTaskMember(task.project_id, task.id, { user_id: userID });
            if (onTaskUpdated) await onTaskUpdated();
        } catch (err) {
            alert(err.message || 'Failed to remove member');
        }
    };

    const emptyContentText = diaryDocument ? '*No diary content yet*' : '*No collaborative document content yet*';
    const markdownHTML = DOMPurify.sanitize(marked.parse(editData.content_md || emptyContentText));
    const contentStatusLabel = diaryDocument ? 'Plain Markdown' : ({
        [contentConnectionStatus.CONNECTING]: 'Connecting',
        [contentConnectionStatus.CONNECTED]: 'Live',
        [contentConnectionStatus.DISCONNECTED]: 'Offline',
        [contentConnectionStatus.ERROR]: 'Error',
    }[contentStatus] || 'Offline');

    return (
        <div className={`yq-task-panel-overlay ${isOpen ? 'open' : ''}`} onClick={closePanel}>
            <div className={`yq-task-panel ${isOpen ? 'open' : ''}`} onClick={(event) => event.stopPropagation()}>
                <div className="yq-panel-header">
                    <div className="yq-panel-project-info">
                        {project && (
                            <span className="yq-project-tag" style={{ backgroundColor: project.color }}>
                                {project.name}
                            </span>
                        )}
                        {lockStatusText ? (
                            <span className={`yq-metadata-lock-pill ${lockedByOther || lockError ? 'blocked' : 'active'}`}>
                                {lockStatusText}
                            </span>
                        ) : null}
                    </div>
                    <div className="yq-panel-actions">
                        {!isEditing ? (
                            <Button variant="secondary" onClick={beginEdit}>Edit</Button>
                        ) : (
                            <>
                                <Button variant="secondary" onClick={cancelEdit}>Cancel</Button>
                                <Button onClick={onSave} disabled={saving || metadataLockBlocked}>
                                    {saving ? 'Saving...' : awaitingMetadataLock ? 'Locking...' : 'Save'}
                                </Button>
                            </>
                        )}
                        <button className="yq-panel-close" type="button" aria-label="Close details" onClick={closePanel}>
                            x
                        </button>
                    </div>
                </div>

                <div className="yq-panel-body">
                    {!isEditing ? (
                        <>
                            <div className="yq-panel-title-area">
                                <input
                                    type="checkbox"
                                    checked={task.status === 'done'}
                                    className="yq-task-checkbox-lg"
                                    onChange={(event) => onStatusToggle(event.target.checked)}
                                    disabled={lockedByOther}
                                    title={lockedByOther ? lockStatusText : undefined}
                                />
                                <h2 className={`yq-panel-title ${task.status === 'done' ? 'completed' : ''}`}>{task.title}</h2>
                            </div>

                            <div className="yq-panel-meta">
                                <div className="yq-meta-item">
                                    <span className="yq-meta-label">Status</span>
                                    <span className={`yq-status-badge ${task.status}`}>{task.status}</span>
                                </div>
                                <div className="yq-meta-item">
                                    <span className="yq-meta-label">Priority</span>
                                    <span>{task.priority > 0 ? `P${task.priority}` : 'None'}</span>
                                </div>
                                <div className="yq-meta-item">
                                    <span className="yq-meta-label">Reminder</span>
                                    <span>
                                        {task.due_at
                                            ? formatDueForPanel(task.due_at)
                                            : 'No reminder'}
                                    </span>
                                </div>
                            </div>

                            <div className="yq-panel-content yq-markdown-body" dangerouslySetInnerHTML={{ __html: markdownHTML }} />
                            <div className={`yq-content-sync-status ${contentStatus}`}>
                                Document sync: {contentStatusLabel}
                                {contentError ? <span>{contentError}</span> : null}
                            </div>

                            {!privateDocument ? (
                            <div className="yq-panel-section">
                                <div className="yq-panel-section-header">
                                    <h3>Collaborators</h3>
                                    <Button variant="secondary" onClick={() => setShowMemberForm((prev) => !prev)}>
                                        {showMemberForm ? 'Close' : 'Add Member'}
                                    </Button>
                                </div>

                                {showMemberForm && (
                                    <div className="yq-member-form">
                                        <Input
                                            placeholder="User email"
                                            value={memberEmail}
                                            onChange={(event) => setMemberEmail(event.target.value)}
                                            style={{ marginBottom: 8 }}
                                        />
                                        <div className="yq-member-form-row">
                                            <select
                                                className="yq-input"
                                                value={memberRole}
                                                onChange={(event) => setMemberRole(event.target.value)}
                                            >
                                                <option value="viewer">Viewer</option>
                                                <option value="editor">Editor</option>
                                            </select>
                                            <Button onClick={onAddMember} disabled={addingMember || !memberEmail.trim()}>
                                                {addingMember ? 'Adding...' : 'Add'}
                                            </Button>
                                        </div>
                                    </div>
                                )}

                                <div className="yq-member-list">
                                    {Array.isArray(task.members) && task.members.length > 0 ? (
                                        task.members.map((member) => {
                                            const { username, avatarURL } = memberDisplay(member);
                                            return (
                                                <div key={member.user_id} className="yq-member-badge" title={member.role}>
                                                    <Avatar src={avatarURL} alt={username} size={24} />
                                                    <span>{username}</span>
                                                    {member.user_id !== task.user_id && (
                                                        <button
                                                            type="button"
                                                            className="yq-member-remove"
                                                            onClick={() => onRemoveMember(member.user_id)}
                                                        >
                                                            x
                                                        </button>
                                                    )}
                                                </div>
                                            );
                                        })
                                    ) : (
                                        <span className="yq-empty-member">No members yet</span>
                                    )}
                                </div>
                            </div>
                            ) : (
                                <div className="yq-panel-section yq-private-doc-note">
                                    <h3>Private document</h3>
                                    <p>This document stays in your personal block. Collaborators cannot be added.</p>
                                </div>
                            )}
                        </>
                    ) : (
                        <div className="yq-panel-edit-form">
                            {lockStatusText ? (
                                <div className={`yq-metadata-lock-banner ${metadataLockBlocked ? 'blocked' : 'active'}`}>
                                    {lockStatusText}
                                </div>
                            ) : null}
                            <Input
                                label="Document title"
                                value={editData.title}
                                onChange={(event) => setEditData((prev) => ({ ...prev, title: event.target.value }))}
                                disabled={metadataLockBlocked}
                            />

                            <div className="yq-form-row">
                                <div className="yq-form-group">
                                    <label className="yq-input-label">Priority</label>
                                    <select
                                        className="yq-input"
                                        value={editData.priority}
                                        onChange={(event) => setEditData((prev) => ({ ...prev, priority: Number(event.target.value) }))}
                                        disabled={metadataLockBlocked}
                                    >
                                        <option value={0}>None</option>
                                        <option value={1}>P1</option>
                                        <option value={2}>P2</option>
                                        <option value={3}>P3</option>
                                        <option value={4}>P4</option>
                                        <option value={5}>P5</option>
                                    </select>
                                </div>

                                <div className="yq-form-group">
                                    <label className="yq-input-label">Reminder time</label>
                                    <div className="yq-due-picker-row">
                                        <input
                                            type="date"
                                            className="yq-input"
                                            value={dueParts.datePart}
                                            onClick={(event) => event.target.showPicker?.()}
                                            onFocus={(event) => event.target.showPicker?.()}
                                            disabled={metadataLockBlocked}
                                            onChange={(event) => setEditData((prev) => ({
                                                ...prev,
                                                due_at: mergeDueDateTimeInput(event.target.value, splitDueDateTimeInput(prev.due_at).timePart),
                                            }))}
                                        />
                                        <input
                                            type="time"
                                            className="yq-input"
                                            step={60}
                                            value={dueParts.timePart}
                                            onClick={(event) => event.target.showPicker?.()}
                                            onFocus={(event) => event.target.showPicker?.()}
                                            disabled={metadataLockBlocked}
                                            onChange={(event) => setEditData((prev) => ({
                                                ...prev,
                                                due_at: mergeDueDateTimeInput(splitDueDateTimeInput(prev.due_at).datePart, event.target.value),
                                            }))}
                                        />
                                        <Button
                                            variant="secondary"
                                            type="button"
                                            onClick={() => setEditData((prev) => ({ ...prev, due_at: '' }))}
                                            disabled={metadataLockBlocked}
                                        >
                                            Clear
                                        </Button>
                                    </div>
                                    <span className="yq-input-help">Optional reminder metadata. Asia/Shanghai (UTC+08:00), minute precision.</span>
                                </div>
                            </div>

                            <div className="yq-form-group">
                                <div className="yq-content-editor-header">
                                    <label className="yq-input-label">
                                        {diaryDocument
                                            ? 'Diary note (plain Markdown)'
                                            : privateDocument ? 'Private document (Markdown)' : 'Collaborative document (Markdown)'}
                                    </label>
                                    <span className={`yq-content-sync-status ${contentStatus}`}>
                                        Document sync: {contentStatusLabel}
                                    </span>
                                </div>
                                {contentError ? <div className="yq-content-sync-error">{contentError}</div> : null}
                                <DocumentMarkdownEditor
                                    key={`${diaryDocument ? 'plain' : 'collab'}-${task.id}`}
                                    provider={contentProvider}
                                    value={editData.content_md}
                                    onChange={onDocumentContentChange}
                                    collaborative={!diaryDocument}
                                />
                                <span className="yq-input-help">
                                    {diaryDocument
                                        ? 'Diary content is saved with the Save button through the plain Markdown API, without Yjs collaboration.'
                                        : privateDocument
                                        ? 'Private content is stored on the same Markdown document path, without collaborator access.'
                                        : 'Document content is saved live through CodeMirror + Yjs; the Save button only updates metadata.'}
                                </span>
                            </div>
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}


