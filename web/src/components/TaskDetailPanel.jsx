import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { marked } from 'marked';
import DOMPurify from 'dompurify';
import { generateMeetingPreview, streamContinuePreview, streamDraftPreview } from '../api/ai';
import {
    addTaskMember,
    createTaskComment,
    deleteTaskComment,
    getTaskComments,
    removeTaskMember,
    saveDocumentContent,
    updateTask,
    updateTaskComment,
} from '../api/task';
import { createMeetingActionTodo } from '../api/meeting';
import { openDiaryDate } from '../api/diary';
import { contentConnectionStatus, YjsTaskContentProvider } from '../realtime/yjsTaskContentProvider';
import {
    METADATA_LOCK_FIELD,
    isLockHeldByCurrentUser,
    isLockHeldByOther,
} from '../store/collab-store';
import useAuth from '../store/useAuth';
import { getShanghaiParts, parseDateInShanghai } from '../utils/shanghaiTime';
import { getTaskStatusLabel, isDiaryTask, isMeetingTask, isTodoTask } from '../utils/taskTypes';
import AIPreviewPanel from './AIPreviewPanel';
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

function commentDisplay(comment) {
    const username = comment.user?.username || comment.user_username || comment.username || `User ${comment.user_id}`;
    const avatarURL = comment.user?.avatar_url || comment.user_avatar_url || comment.avatar_url || '';
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

function formatCommentTime(value) {
    const date = parseDateInShanghai(value);
    const parts = getShanghaiParts(date);
    if (!parts) return '';

    const month = String(parts.month + 1).padStart(2, '0');
    const day = String(parts.day).padStart(2, '0');
    const hour = String(parts.hour).padStart(2, '0');
    const minute = String(parts.minute).padStart(2, '0');
    return `${parts.year}-${month}-${day} ${hour}:${minute}`;
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
    viewers = [],
    onLockRequest,
    onLockRelease,
}) {
    const navigate = useNavigate();
    const { user } = useAuth();
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
    const [comments, setComments] = useState([]);
    const [commentsLoading, setCommentsLoading] = useState(false);
    const [commentsError, setCommentsError] = useState('');
    const [commentDraft, setCommentDraft] = useState('');
    const [commentAnchorText, setCommentAnchorText] = useState('');
    const [commentSubmitting, setCommentSubmitting] = useState(false);
    const [commentActionID, setCommentActionID] = useState(0);
    const [meetingActionTitle, setMeetingActionTitle] = useState('');
    const [meetingActionSaving, setMeetingActionSaving] = useState(false);
    const [diaryNavBusy, setDiaryNavBusy] = useState('');
    const [aiInstruction, setAIInstruction] = useState('');
    const [aiBusy, setAIBusy] = useState(false);
    const [aiMode, setAIMode] = useState('');
    const [aiError, setAIError] = useState('');
    const [aiPreviewText, setAIPreviewText] = useState('');
    const [aiMeetingPreview, setAIMeetingPreview] = useState(null);
    const [aiSelection, setAISelection] = useState({ from: 0, to: 0, text: '' });
    const [editorCommand, setEditorCommand] = useState(null);
    const aiAbortRef = useRef(null);
    const resolvedCurrentUserID = Number(currentUserID || user?.id || user?.user_id || 0);

    const releaseMetadataLock = useCallback(() => {
        const held = editingLockRef.current;
        if (held?.taskID) {
            onLockRelease?.(held.taskID, held.field);
        }
        editingLockRef.current = null;
    }, [onLockRelease]);

    useEffect(() => {
        if (!task) return;
        aiAbortRef.current?.abort();
        aiAbortRef.current = null;
        setEditData({
            title: task.title || '',
            content_md: task.content_md || '',
            priority: task.priority || 0,
            due_at: toShanghaiDateTimeInputValue(task.due_at),
        });
        setIsEditing(false);
        setShowMemberForm(false);
        setAIBusy(false);
        setAIMode('');
        setAIError('');
        setAIPreviewText('');
        setAIMeetingPreview(null);
        setAIInstruction('');
        setAISelection({ from: 0, to: 0, text: '' });
        setEditorCommand(null);
    }, [task]);

    useEffect(() => {
        setComments([]);
        setCommentsError('');
        setCommentDraft('');
        setCommentActionID(0);

        if (!isOpen || !task?.id || isDiaryTask(task)) return undefined;

        let cancelled = false;
        setCommentsLoading(true);
        getTaskComments(task.id)
            .then((list) => {
                if (!cancelled) {
                    setComments(Array.isArray(list) ? list : []);
                }
            })
            .catch((err) => {
                if (!cancelled) {
                    setCommentsError(err.message || 'Failed to load comments');
                }
            })
            .finally(() => {
                if (!cancelled) {
                    setCommentsLoading(false);
                }
            });

        return () => {
            cancelled = true;
        };
    }, [isOpen, task]);

    useEffect(() => () => {
        releaseMetadataLock();
    }, [task?.id, releaseMetadataLock]);

    useEffect(() => () => {
        aiAbortRef.current?.abort();
    }, []);

    useEffect(() => {
        contentProviderRef.current?.destroy();
        contentProviderRef.current = null;
        setContentProvider(null);
        setContentStatus(contentConnectionStatus.DISCONNECTED);
        setContentError('');

        if (!isOpen || !task?.id || isDiaryTask(task)) return undefined;

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
    }, [isOpen, task]);

    useEffect(() => {
        if (lockError && editingLockRef.current?.taskID === task?.id) {
            editingLockRef.current = null;
        }
    }, [lockError, task?.id]);

    const onDocumentContentChange = useCallback((nextContent) => {
        setEditData((prev) => (prev ? { ...prev, content_md: nextContent } : prev));
    }, []);

    const cancelAIRequest = useCallback(() => {
        aiAbortRef.current?.abort();
        aiAbortRef.current = null;
        setAIBusy(false);
    }, []);

    const runAIStream = useCallback(async (mode, runner) => {
        aiAbortRef.current?.abort();
        const controller = new AbortController();
        aiAbortRef.current = controller;
        setAIBusy(true);
        setAIMode(mode);
        setAIError('');
        setAIPreviewText('');
        setAIMeetingPreview(null);

        try {
            await runner(controller.signal);
        } catch (err) {
            if (err?.name !== 'AbortError') {
                setAIError(err.message || 'AI generation failed');
            }
        } finally {
            if (aiAbortRef.current === controller) {
                aiAbortRef.current = null;
            }
            setAIBusy(false);
        }
    }, []);

    const onGenerateDraft = useCallback(() => {
        runAIStream('draft', (signal) => streamDraftPreview({
            task_id: task?.id || 0,
            title: editData?.title || task?.title || '',
            instruction: aiInstruction,
            doc_type: task?.doc_type || '',
        }, {
            signal,
            onChunk: (chunk) => setAIPreviewText((prev) => prev + chunk),
        }));
    }, [runAIStream, task, editData, aiInstruction]);

    const onGenerateContinue = useCallback(() => {
        runAIStream('continue', (signal) => streamContinuePreview({
            task_id: task?.id || 0,
            title: editData?.title || task?.title || '',
            selected_text: aiSelection.text || '',
            full_context: editData?.content_md || '',
            instruction: aiInstruction || '请继续写或改写这段内容，保持与上下文一致。',
        }, {
            signal,
            onChunk: (chunk) => setAIPreviewText((prev) => prev + chunk),
        }));
    }, [runAIStream, task, editData, aiInstruction, aiSelection]);

    const onGenerateMeeting = useCallback(async () => {
        setAIBusy(true);
        setAIMode('meeting');
        setAIError('');
        setAIPreviewText('');
        setAIMeetingPreview(null);
        try {
            const preview = await generateMeetingPreview({
                task_id: task?.id || 0,
                title: editData?.title || task?.title || '',
                notes: editData?.content_md || '',
                instruction: aiInstruction,
            });
            setAIMeetingPreview(preview);
        } catch (err) {
            setAIError(err.message || 'Meeting preview failed');
        } finally {
            setAIBusy(false);
        }
    }, [task, editData, aiInstruction]);

    const queueEditorCommand = useCallback((type, text) => {
        setEditorCommand({
            id: `${type}-${Date.now()}-${Math.random().toString(16).slice(2)}`,
            type,
            text,
            from: aiSelection.from,
            to: aiSelection.to,
        });
    }, [aiSelection]);

    const onApplyInsert = useCallback(() => {
        const text = aiMeetingPreview?.minutes_markdown || aiPreviewText;
        if (!text) return;
        queueEditorCommand('insert_after_selection', text);
    }, [aiMeetingPreview, aiPreviewText, queueEditorCommand]);

    const onApplyReplaceSelection = useCallback(() => {
        if (!aiPreviewText) return;
        queueEditorCommand('replace_selection', aiPreviewText);
    }, [aiPreviewText, queueEditorCommand]);

    const onApplyReplaceAll = useCallback(() => {
        const text = aiMeetingPreview?.minutes_markdown || aiPreviewText;
        if (!text) return;
        queueEditorCommand('replace_all', text);
    }, [aiMeetingPreview, aiPreviewText, queueEditorCommand]);

    const onUseMeetingAction = useCallback((action) => {
        const nextTitle = String(action?.title || '').trim();
        if (!nextTitle) return;
        setMeetingActionTitle(nextTitle);
    }, []);

    if (!isOpen || !task || !editData) return null;
    const dueParts = splitDueDateTimeInput(editData.due_at);
    const diaryDocument = isDiaryTask(task);
    const meetingDocument = isMeetingTask(task);
    const todoDocument = isTodoTask(task);
    const privateDocument = task.collaboration_mode === 'private';
    const entityLabel = todoDocument ? 'todo' : 'document';
    const entityLabelTitle = todoDocument ? 'Todo' : 'Document';
    const statusLabel = todoDocument ? 'State' : 'Status';
    const reminderLabel = todoDocument ? 'Reminder' : 'Reminder';
    const reminderEmptyText = todoDocument ? 'No reminder set' : 'No reminder';
    const membersTitle = todoDocument ? 'Participants' : 'Collaborators';
    const addMemberLabel = todoDocument ? 'Add Participant' : 'Add Member';
    const addMemberButtonLabel = todoDocument ? 'Add participant' : 'Add';
    const privateEntityTitle = todoDocument ? 'Private todo' : 'Private document';
    const privateEntityBody = todoDocument
        ? 'This todo stays in your personal block. Other collaborators cannot be added.'
        : 'This document stays in your personal block. Collaborators cannot be added.';
    const contentLabel = diaryDocument
        ? 'Diary note (plain Markdown)'
        : todoDocument
            ? 'Todo note (Markdown)'
            : privateDocument
                ? 'Private document (Markdown)'
                : 'Collaborative document (Markdown)';
    const contentHelpText = diaryDocument
        ? 'Diary content is saved with the Save button through the plain Markdown API, without Yjs collaboration.'
        : todoDocument
            ? 'Todo notes are saved live through CodeMirror + Yjs; the Save button only updates todo metadata.'
            : privateDocument
                ? 'Private content is stored on the same Markdown document path, without collaborator access.'
                : 'Document content is saved live through CodeMirror + Yjs; the Save button only updates metadata.';
    const metadataLockEnabled = !diaryDocument && typeof onLockRequest === 'function' && typeof onLockRelease === 'function';
    const lockedBySelf = metadataLockEnabled && isLockHeldByCurrentUser(taskLock, currentUserID);
    const lockedByOther = metadataLockEnabled && isLockHeldByOther(taskLock, currentUserID);
    const awaitingMetadataLock = metadataLockEnabled && isEditing && !lockedBySelf && !lockedByOther && !lockError;
    const metadataLockBlocked = metadataLockEnabled && (lockedByOther || awaitingMetadataLock || Boolean(lockError));
    const lockHolder = taskLock?.holder_username || `User ${taskLock?.holder_user_id || ''}`.trim();
    const lockStatusText = (metadataLockEnabled && lockError)
        || (lockedByOther ? `Metadata is locked by ${lockHolder}. You can still edit the live ${todoDocument ? 'todo note' : 'document body'}.` : '')
        || (lockedBySelf ? `${entityLabelTitle} metadata lock held by you.` : '')
        || (awaitingMetadataLock ? 'Waiting for metadata lock...' : '');
    const canModerateComments = Number(task.user_id) === resolvedCurrentUserID;

    const canManageComment = (comment) => (
        resolvedCurrentUserID > 0
        && (Number(comment?.user_id) === resolvedCurrentUserID || canModerateComments)
    );

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
        cancelAIRequest();
        setIsEditing(false);
        releaseMetadataLock();
    };

    const closePanel = () => {
        cancelAIRequest();
        releaseMetadataLock();
        onClose?.();
    };

    const onSave = async () => {
        if (metadataLockBlocked) {
            alert(lockStatusText || `${entityLabelTitle} metadata is locked.`);
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
            alert(err.message || `Failed to save ${entityLabel} metadata`);
        } finally {
            setSaving(false);
        }
    };

    const onStatusToggle = async (checked) => {
        if (lockedByOther) {
            alert(lockStatusText || `${entityLabelTitle} metadata is locked.`);
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
            alert(err.message || `Failed to update ${entityLabel} status`);
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

    const onCreateComment = async () => {
        const content = commentDraft.trim();
        if (!content) return;

        setCommentSubmitting(true);
        setCommentsError('');
        try {
            const created = await createTaskComment(task.id, {
                content_md: content,
                anchor_type: commentAnchorText.trim() ? 'selection' : 'document',
                anchor_text: commentAnchorText.trim(),
            });
            setComments((prev) => [...prev, created]);
            setCommentDraft('');
            setCommentAnchorText('');
        } catch (err) {
            setCommentsError(err.message || 'Failed to create comment');
        } finally {
            setCommentSubmitting(false);
        }
    };

    const onToggleCommentResolved = async (comment) => {
        if (!comment?.id) return;

        setCommentActionID(comment.id);
        setCommentsError('');
        try {
            const updated = await updateTaskComment(comment.id, { resolved: !comment.resolved });
            setComments((prev) => prev.map((item) => (item.id === comment.id ? updated : item)));
        } catch (err) {
            setCommentsError(err.message || 'Failed to update comment');
        } finally {
            setCommentActionID(0);
        }
    };

    const onDeleteComment = async (comment) => {
        if (!comment?.id) return;
        if (!window.confirm('Delete this comment?')) return;

        setCommentActionID(comment.id);
        setCommentsError('');
        try {
            await deleteTaskComment(comment.id);
            setComments((prev) => prev.filter((item) => item.id !== comment.id));
        } catch (err) {
            setCommentsError(err.message || 'Failed to delete comment');
        } finally {
            setCommentActionID(0);
        }
    };

    const onCreateMeetingActionTodo = async () => {
        const title = meetingActionTitle.trim();
        if (!title || !task?.id || meetingActionSaving) return;
        setMeetingActionSaving(true);
        try {
            await createMeetingActionTodo(task.id, { title });
            setMeetingActionTitle('');
            if (onTaskUpdated) await onTaskUpdated();
        } catch (err) {
            alert(err.message || 'Failed to create todo from action item');
        } finally {
            setMeetingActionSaving(false);
        }
    };

    const onOpenAdjacentDiary = async (direction) => {
        if (!task?.title || diaryNavBusy) return;
        const match = String(task.title).match(/^(\d{4}-\d{2}-\d{2})\.md$/);
        if (!match) return;
        const date = new Date(`${match[1]}T00:00:00+08:00`);
        date.setUTCDate(date.getUTCDate() + direction);
        const nextDate = new Intl.DateTimeFormat('en-CA', {
            timeZone: 'Asia/Shanghai',
            year: 'numeric',
            month: '2-digit',
            day: '2-digit',
        }).format(date);
        setDiaryNavBusy(direction < 0 ? 'prev' : 'next');
        try {
            const result = await openDiaryDate(nextDate);
            if (result?.project?.id && result?.task?.id) {
                navigate(`/projects/${result.project.id}?task=${result.task.id}`);
            }
        } catch (err) {
            alert(err.message || 'Failed to open diary');
        } finally {
            setDiaryNavBusy('');
        }
    };
    const emptyContentText = diaryDocument
        ? '*No diary content yet*'
        : todoDocument
            ? '*No todo note yet*'
            : '*No collaborative document content yet*';
    const markdownHTML = DOMPurify.sanitize(marked.parse(editData.content_md || emptyContentText));
    const contentStatusLabel = diaryDocument ? 'Plain Markdown' : ({
        [contentConnectionStatus.CONNECTING]: 'Connecting',
        [contentConnectionStatus.CONNECTED]: 'Live',
        [contentConnectionStatus.DISCONNECTED]: 'Offline',
        [contentConnectionStatus.ERROR]: 'Error',
    }[contentStatus] || 'Offline');
    const contentStatusPrefix = todoDocument ? 'Todo note sync' : 'Document sync';
    const commentsSection = diaryDocument ? null : (
        <div className="yq-panel-section yq-comments-section">
            <div className="yq-panel-section-header">
                <h3>Comments</h3>
                <span className="yq-panel-section-count">{comments.length}</span>
            </div>

            <div className="yq-comment-compose">
                <textarea
                    className="yq-input yq-comment-input"
                    value={commentDraft}
                    onChange={(event) => setCommentDraft(event.target.value)}
                    placeholder="Leave a Markdown comment"
                    rows={4}
                    disabled={commentSubmitting}
                />
                <Input
                    label="Anchor text (optional)"
                    value={commentAnchorText}
                    onChange={(event) => setCommentAnchorText(event.target.value)}
                    placeholder="Paste the text this comment refers to"
                    disabled={commentSubmitting}
                />
                <div className="yq-comment-compose-footer">
                    <span className="yq-input-help">Markdown supported. Optional anchor text is stored for future inline comments.</span>
                    <Button onClick={onCreateComment} disabled={commentSubmitting || !commentDraft.trim()}>
                        {commentSubmitting ? 'Posting...' : 'Comment'}
                    </Button>
                </div>
            </div>

            {commentsError ? <div className="yq-comment-error">{commentsError}</div> : null}

            {commentsLoading ? (
                <div className="yq-comment-empty">Loading comments...</div>
            ) : comments.length > 0 ? (
                <div className="yq-comment-list">
                    {comments.map((comment) => {
                        const { username, avatarURL } = commentDisplay(comment);
                        const commentHTML = DOMPurify.sanitize(marked.parse(comment.content_md || ''));
                        const busy = commentActionID === comment.id;
                        return (
                            <div key={comment.id} className={`yq-comment-item ${comment.resolved ? 'resolved' : ''}`}>
                                <Avatar src={avatarURL} alt={username} size={28} />
                                <div className="yq-comment-card">
                                    <div className="yq-comment-meta">
                                        <div className="yq-comment-author-row">
                                            <strong>{username}</strong>
                                            {comment.resolved ? <span className="yq-comment-state">Resolved</span> : null}
                                        </div>
                                        <span>{formatCommentTime(comment.created_at)}</span>
                                    </div>
                                    {comment.anchor_type === 'selection' && comment.anchor_text ? (
                                        <blockquote className="yq-comment-anchor">{comment.anchor_text}</blockquote>
                                    ) : null}
                                    <div className="yq-markdown-body yq-comment-content" dangerouslySetInnerHTML={{ __html: commentHTML }} />
                                    {canManageComment(comment) ? (
                                        <div className="yq-comment-actions">
                                            <button
                                                type="button"
                                                className="yq-comment-action"
                                                onClick={() => onToggleCommentResolved(comment)}
                                                disabled={busy}
                                            >
                                                {busy ? 'Saving...' : comment.resolved ? 'Reopen' : 'Resolve'}
                                            </button>
                                            <button
                                                type="button"
                                                className="yq-comment-action danger"
                                                onClick={() => onDeleteComment(comment)}
                                                disabled={busy}
                                            >
                                                Delete
                                            </button>
                                        </div>
                                    ) : null}
                                </div>
                            </div>
                        );
                    })}
                </div>
            ) : (
                <div className="yq-comment-empty">No comments yet.</div>
            )}
        </div>
    );
    const meetingActionsSection = meetingDocument ? (
        <div className="yq-panel-section">
            <div className="yq-panel-section-header">
                <h3>Action item to todo</h3>
            </div>
            <div className="yq-member-form">
                <Input
                    placeholder="Action item title"
                    value={meetingActionTitle}
                    onChange={(event) => setMeetingActionTitle(event.target.value)}
                    disabled={meetingActionSaving}
                />
                <Button onClick={onCreateMeetingActionTodo} disabled={meetingActionSaving || !meetingActionTitle.trim()}>
                    {meetingActionSaving ? 'Creating...' : 'Create todo'}
                </Button>
            </div>
        </div>
    ) : null;
    const aiPanelSection = (
        <AIPreviewPanel
            busy={aiBusy}
            mode={aiMode}
            error={aiError}
            instruction={aiInstruction}
            selectedText={aiSelection.text}
            previewText={aiPreviewText}
            meetingPreview={aiMeetingPreview}
            canGenerateMeeting={meetingDocument}
            onInstructionChange={setAIInstruction}
            onGenerateDraft={onGenerateDraft}
            onGenerateContinue={onGenerateContinue}
            onGenerateMeeting={onGenerateMeeting}
            onCancel={cancelAIRequest}
            onApplyInsert={onApplyInsert}
            onApplyReplaceSelection={onApplyReplaceSelection}
            onApplyReplaceAll={onApplyReplaceAll}
            onUseMeetingAction={onUseMeetingAction}
        />
    );
    const diaryNavSection = diaryDocument ? (
        <div className="yq-panel-section">
            <div className="yq-panel-section-header">
                <h3>Daily navigation</h3>
            </div>
            <div className="yq-panel-actions">
                <Button variant="secondary" onClick={() => onOpenAdjacentDiary(-1)} disabled={Boolean(diaryNavBusy)}>
                    {diaryNavBusy === 'prev' ? 'Opening...' : 'Previous day'}
                </Button>
                <Button variant="secondary" onClick={() => onOpenAdjacentDiary(1)} disabled={Boolean(diaryNavBusy)}>
                    {diaryNavBusy === 'next' ? 'Opening...' : 'Next day'}
                </Button>
            </div>
        </div>
    ) : null;

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
                                    <span className="yq-meta-label">{statusLabel}</span>
                                    <span className={`yq-status-badge ${task.status}`}>{getTaskStatusLabel(task)}</span>
                                </div>
                                <div className="yq-meta-item">
                                    <span className="yq-meta-label">Priority</span>
                                    <span>{task.priority > 0 ? `P${task.priority}` : 'None'}</span>
                                </div>
                                <div className="yq-meta-item">
                                    <span className="yq-meta-label">{reminderLabel}</span>
                                    <span>
                                        {task.due_at
                                            ? formatDueForPanel(task.due_at)
                                            : reminderEmptyText}
                                    </span>
                                </div>
                            </div>

                            <div className="yq-panel-content yq-markdown-body" dangerouslySetInnerHTML={{ __html: markdownHTML }} />
                            <div className={`yq-content-sync-status ${contentStatus}`}>
                                {contentStatusPrefix}: {contentStatusLabel}
                                {contentError ? <span>{contentError}</span> : null}
                            </div>
                            {viewers.length > 0 ? (
                                <div className="yq-document-viewers">
                                    Viewing: {viewers.map((viewer) => viewer.username || `User ${viewer.user_id}`).join(', ')}
                                </div>
                            ) : null}

                            {!privateDocument ? (
                            <div className="yq-panel-section">
                                <div className="yq-panel-section-header">
                                    <h3>{membersTitle}</h3>
                                    <Button variant="secondary" onClick={() => setShowMemberForm((prev) => !prev)}>
                                        {showMemberForm ? 'Close' : addMemberLabel}
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
                                                {addingMember ? 'Adding...' : addMemberButtonLabel}
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
                                    <h3>{privateEntityTitle}</h3>
                                    <p>{privateEntityBody}</p>
                                </div>
                            )}

                            {diaryNavSection}
                            {meetingActionsSection}
                            {commentsSection}
                        </>
                    ) : (
                        <div className="yq-panel-edit-form">
                            {lockStatusText ? (
                                <div className={`yq-metadata-lock-banner ${metadataLockBlocked ? 'blocked' : 'active'}`}>
                                    {lockStatusText}
                                </div>
                            ) : null}
                            <Input
                                label={todoDocument ? 'Todo title' : 'Document title'}
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
                                    <label className="yq-input-label">{contentLabel}</label>
                                    <span className={`yq-content-sync-status ${contentStatus}`}>
                                        {contentStatusPrefix}: {contentStatusLabel}
                                    </span>
                                </div>
                                {contentError ? <div className="yq-content-sync-error">{contentError}</div> : null}
                                <DocumentMarkdownEditor
                                    key={`${diaryDocument ? 'plain' : 'collab'}-${task.id}`}
                                    provider={contentProvider}
                                    value={editData.content_md}
                                    onChange={onDocumentContentChange}
                                    collaborative={!diaryDocument}
                                    onSelectionChange={setAISelection}
                                    command={editorCommand}
                                />
                                <span className="yq-input-help">{contentHelpText}</span>
                            </div>

                            {aiPanelSection}
                            {diaryNavSection}
                            {meetingActionsSection}
                            {commentsSection}
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}


