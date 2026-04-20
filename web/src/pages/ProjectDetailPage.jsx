import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useParams, useSearchParams } from 'react-router-dom';
import {
    abortDocumentImport,
    completeDocumentImport,
    createDocumentImportSession,
    uploadDocumentImportAsset,
    uploadDocumentImportPart,
} from '../api/documentImport';
import { getProjectById } from '../api/project';
import { createTask, deleteTask, getProjectActivities, getTaskById, getTasks, updateTask } from '../api/task';
import Button from '../components/Button';
import ProjectActivityPanel from '../components/ProjectActivityPanel';
import TaskDetailPanel from '../components/TaskDetailPanel';
import TaskSection from '../components/TaskSection';
import { projectMessageTypes } from '../realtime/protocol';
import { ProjectEventsSocket, projectConnectionStatus } from '../realtime/projectEventsSocket';
import {
    METADATA_LOCK_FIELD,
    applyProjectLockMessage,
    applyProjectTaskEvent,
    applyProjectTaskEvents,
    applySelectedTaskEvent,
    flattenPresenceUsers,
    metadataLockForTask,
    mergePresenceSnapshot,
    optimisticTaskUpdate,
    projectLockKey,
    removeTask,
    sortProjectTasks,
    upsertTask,
} from '../store/collab-store';
import useAuth from '../store/useAuth';
import { splitTasksByLifecycle } from '../utils/taskExpiration';
import { isTodoTask, taskDocTypes } from '../utils/taskTypes';
import './ProjectDetailPage.css';

const documentModes = {
    COLLABORATIVE: 'collaborative',
    PRIVATE: 'private',
};

const importChunkSize = 1024 * 1024;
const activityPageSize = 12;

function isPrivateDocument(task) {
    return task?.collaboration_mode === documentModes.PRIVATE;
}

function titleFromMarkdownFile(file) {
    return (file?.name || '').replace(/\.(md|markdown)$/i, '').trim();
}

function originalAssetPath(file) {
    return file?.webkitRelativePath || file?.name || '';
}

function mergeActivityEntries(previous, incoming) {
    const nextMap = new Map();
    [...previous, ...(incoming || [])].forEach((activity) => {
        if (!activity?.id) return;
        nextMap.set(activity.id, activity);
    });
    return Array.from(nextMap.values()).sort((left, right) => Number(right.id || 0) - Number(left.id || 0));
}

export default function ProjectDetailPage() {
    const { id } = useParams();
    const [searchParams, setSearchParams] = useSearchParams();
    const { user } = useAuth();
    const projectId = Number.parseInt(id || '', 10);
    const currentUserID = Number(user?.id || user?.user_id || 0);

    const [project, setProject] = useState(null);
    const [tasks, setTasks] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [realtimeStatus, setRealtimeStatus] = useState(projectConnectionStatus.DISCONNECTED);
    const [realtimeError, setRealtimeError] = useState('');
    const [lastCursor, setLastCursor] = useState(0);
    const [presenceByNode, setPresenceByNode] = useState({});
    const cursorRef = useRef(0);
    const socketRef = useRef(null);
    const [locksByKey, setLocksByKey] = useState({});
    const [lockErrors, setLockErrors] = useState({});

    const [draftTitles, setDraftTitles] = useState({
        [documentModes.COLLABORATIVE]: '',
        [documentModes.PRIVATE]: '',
    });
    const [creatingMode, setCreatingMode] = useState('');
    const [assetFilesByMode, setAssetFilesByMode] = useState({
        [documentModes.COLLABORATIVE]: [],
        [documentModes.PRIVATE]: [],
    });
    const [importingMode, setImportingMode] = useState('');
    const [importMessages, setImportMessages] = useState({});
    const [todoDraftTitle, setTodoDraftTitle] = useState('');
    const [creatingTodo, setCreatingTodo] = useState(false);
    const [selectedTask, setSelectedTask] = useState(null);
    const [activities, setActivities] = useState([]);
    const [activityCursor, setActivityCursor] = useState(0);
    const [activityHasMore, setActivityHasMore] = useState(false);
    const [activityLoading, setActivityLoading] = useState(true);
    const [activityLoadingMore, setActivityLoadingMore] = useState(false);
    const [activityError, setActivityError] = useState('');

    const applyRealtimeEvent = useCallback((event) => {
        setTasks((prev) => applyProjectTaskEvent(prev, event));
        setSelectedTask((prev) => applySelectedTaskEvent(prev, event));
    }, []);

    const applyRealtimeEvents = useCallback((events) => {
        setTasks((prev) => applyProjectTaskEvents(prev, events));
        setSelectedTask((prev) => events.reduce((next, event) => applySelectedTaskEvent(next, event), prev));
    }, []);

    const patchTask = useCallback((task) => {
        if (!task?.id) return;
        setTasks((prev) => upsertTask(prev, task));
        setSelectedTask((prev) => (prev?.id === task.id ? { ...prev, ...task } : prev));
    }, []);

    const onRealtimeCursor = useCallback((cursor) => {
        cursorRef.current = cursor;
        setLastCursor(cursor);
    }, []);

    const onPresenceSnapshot = useCallback((nodeID, presence) => {
        setPresenceByNode((prev) => mergePresenceSnapshot(prev, nodeID, presence));
    }, []);

    const clearLockError = useCallback((taskID, field = METADATA_LOCK_FIELD) => {
        const key = projectLockKey(taskID, field);
        setLockErrors((prev) => {
            if (!prev[key]) return prev;
            const next = { ...prev };
            delete next[key];
            return next;
        });
    }, []);

    const onProjectLockMessage = useCallback((message) => {
        const lock = message?.lock;
        if (!lock?.task_id) return;
        const key = projectLockKey(lock.task_id, lock.field);

        if (message.type === projectMessageTypes.LOCK_ERROR) {
            setLockErrors((prev) => ({
                ...prev,
                [key]: message.error || 'Failed to update task lock.',
            }));
            return;
        }

        setLocksByKey((prev) => applyProjectLockMessage(prev, message));
        clearLockError(lock.task_id, lock.field);
    }, [clearLockError]);

    const requestTaskLock = useCallback((taskID, field = METADATA_LOCK_FIELD) => {
        clearLockError(taskID, field);
        return socketRef.current?.requestLock(taskID, field) || false;
    }, [clearLockError]);

    const releaseTaskLock = useCallback((taskID, field = METADATA_LOCK_FIELD) => {
        clearLockError(taskID, field);
        return socketRef.current?.releaseLock(taskID, field) || false;
    }, [clearLockError]);

    const loadData = useCallback(async () => {
        if (!Number.isFinite(projectId) || projectId <= 0) {
            setError('Invalid space id.');
            return;
        }

        setLoading(true);
        try {
            const [projData, taskData] = await Promise.all([
                getProjectById(projectId),
                getTasks({ project_id: projectId, size: 200 }),
            ]);
            setProject(projData);
            setTasks(sortProjectTasks(taskData.list || []));
            setError('');
        } catch (err) {
            setError(err.message || 'Failed to load space');
        } finally {
            setLoading(false);
        }
    }, [projectId]);

    const loadActivities = useCallback(async ({ cursor = 0, append = false, silent = false } = {}) => {
        if (!Number.isFinite(projectId) || projectId <= 0) return;

        if (append) {
            setActivityLoadingMore(true);
        } else if (!silent) {
            setActivityLoading(true);
        }

        try {
            const data = await getProjectActivities(projectId, {
                cursor,
                limit: activityPageSize,
            });
            const nextActivities = Array.isArray(data?.activities) ? data.activities : [];
            setActivities((prev) => (append ? mergeActivityEntries(prev, nextActivities) : nextActivities));
            setActivityCursor(Number(data?.next_cursor || 0));
            setActivityHasMore(Boolean(data?.has_more));
            setActivityError('');
        } catch (err) {
            if (!silent || !append) {
                setActivityError(err.message || 'Failed to load activity');
            }
        } finally {
            if (append) {
                setActivityLoadingMore(false);
            } else if (!silent) {
                setActivityLoading(false);
            }
        }
    }, [projectId]);

    useEffect(() => {
        cursorRef.current = 0;
        setLastCursor(0);
        setPresenceByNode({});
        setLocksByKey({});
        setLockErrors({});
        loadData();
        setActivities([]);
        setActivityCursor(0);
        setActivityHasMore(false);
        setActivityError('');
        setActivityLoading(true);
    }, [loadData]);

    useEffect(() => {
        loadActivities();
    }, [loadActivities]);

    useEffect(() => {
        if (!project?.id) return undefined;

        setRealtimeError('');
        const socket = new ProjectEventsSocket({
            projectId: project.id,
            cursor: cursorRef.current,
            onInit: applyRealtimeEvents,
            onEvent: applyRealtimeEvent,
            onPresence: onPresenceSnapshot,
            onLock: onProjectLockMessage,
            onCursorChange: onRealtimeCursor,
            onStatusChange: setRealtimeStatus,
            onError: setRealtimeError,
        });
        socketRef.current = socket;

        return () => {
            if (socketRef.current === socket) {
                socketRef.current = null;
            }
            socket.destroy();
        };
    }, [project?.id, applyRealtimeEvent, applyRealtimeEvents, onPresenceSnapshot, onProjectLockMessage, onRealtimeCursor]);

    useEffect(() => {
        if (!project?.id || lastCursor <= 0) return undefined;

        const timer = window.setTimeout(() => {
            loadActivities({ silent: true });
        }, 300);

        return () => window.clearTimeout(timer);
    }, [project?.id, lastCursor, loadActivities]);

    useEffect(() => {
        const rawTaskID = searchParams.get('task');
        const taskID = Number.parseInt(rawTaskID || '', 10);
        if (!project?.id || !Number.isFinite(taskID) || taskID <= 0) return undefined;

        let cancelled = false;
        const openRequestedTask = async () => {
            const existing = tasks.find((item) => item.id === taskID);
            if (existing) {
                setSelectedTask(existing);
            } else {
                try {
                    const loaded = await getTaskById(taskID);
                    if (!cancelled && loaded?.project_id === project.id) {
                        patchTask(loaded);
                        setSelectedTask(loaded);
                    }
                } catch (err) {
                    setError(err.message || 'Failed to open document');
                }
            }

            if (!cancelled) {
                setSearchParams((prev) => {
                    const next = new URLSearchParams(prev);
                    next.delete('task');
                    return next;
                }, { replace: true });
            }
        };

        openRequestedTask();
        return () => {
            cancelled = true;
        };
    }, [project?.id, searchParams, setSearchParams, tasks, patchTask]);

    const { expired: expiredTasks, todo: todoTasks, done: doneTasks } = useMemo(
        () => splitTasksByLifecycle(tasks),
        [tasks],
    );

    const attentionDocuments = useMemo(
        () => expiredTasks.filter((task) => !isTodoTask(task)),
        [expiredTasks],
    );

    const attentionTodos = useMemo(
        () => expiredTasks.filter(isTodoTask),
        [expiredTasks],
    );

    const collaborativeTasks = useMemo(
        () => todoTasks.filter((task) => !isPrivateDocument(task) && !isTodoTask(task)),
        [todoTasks],
    );

    const privateTasks = useMemo(
        () => todoTasks.filter((task) => isPrivateDocument(task) && !isTodoTask(task)),
        [todoTasks],
    );

    const todoItems = useMemo(
        () => todoTasks.filter(isTodoTask),
        [todoTasks],
    );

    const archivedDocuments = useMemo(
        () => doneTasks.filter((task) => !isTodoTask(task)),
        [doneTasks],
    );

    const completedTodos = useMemo(
        () => doneTasks.filter(isTodoTask),
        [doneTasks],
    );

    const onlineUsers = useMemo(
        () => flattenPresenceUsers(presenceByNode),
        [presenceByNode],
    );

    const selectedTaskViewers = useMemo(() => {
        if (!selectedTask?.id) return [];
        return onlineUsers.filter((user) => (user.viewing_task_ids || []).includes(selectedTask.id));
    }, [onlineUsers, selectedTask?.id]);

    useEffect(() => {
        socketRef.current?.viewDocument(selectedTask?.id || 0);
    }, [selectedTask?.id]);

    const updateDraftTitle = useCallback((mode, value) => {
        setDraftTitles((prev) => ({ ...prev, [mode]: value }));
    }, []);

    const setImportMessage = useCallback((mode, message) => {
        setImportMessages((prev) => ({ ...prev, [mode]: message }));
    }, []);

    const onCreateDocument = async (mode) => {
        const title = (draftTitles[mode] || '').trim();
        if (!title || creatingMode) return;
        setCreatingMode(mode);
        try {
            const task = await createTask({
                title,
                project_id: projectId,
                doc_type: 'document',
                collaboration_mode: mode,
                status: 'todo',
            });
            updateDraftTitle(mode, '');
            patchTask(task);
            loadActivities({ silent: true });
        } catch (err) {
            alert(err.message || 'Failed to create document');
        } finally {
            setCreatingMode('');
        }
    };

    const onCreateTodo = async () => {
        const title = todoDraftTitle.trim();
        if (!title || creatingTodo) return;

        setCreatingTodo(true);
        try {
            const task = await createTask({
                title,
                project_id: projectId,
                doc_type: taskDocTypes.TODO,
                collaboration_mode: documentModes.COLLABORATIVE,
                status: 'todo',
                priority: 0,
            });
            setTodoDraftTitle('');
            patchTask(task);
            setSelectedTask(task);
            loadActivities({ silent: true });
        } catch (err) {
            alert(err.message || 'Failed to create todo');
        } finally {
            setCreatingTodo(false);
        }
    };

    const onAssetFilesSelected = useCallback((mode, files) => {
        const nextFiles = Array.from(files || []);
        setAssetFilesByMode((prev) => ({ ...prev, [mode]: nextFiles }));
        setImportMessage(mode, nextFiles.length > 0 ? `${nextFiles.length} image reference(s) ready.` : '');
    }, [setImportMessage]);

    const onImportMarkdown = useCallback(async (mode, file) => {
        if (!file || importingMode) return;
        if (!/\.(md|markdown)$/i.test(file.name || '')) {
            setImportMessage(mode, 'Please choose a .md or .markdown file.');
            return;
        }
        if (file.size <= 0) {
            setImportMessage(mode, 'Markdown file is empty.');
            return;
        }

        let uploadID = '';
        setImportingMode(mode);
        setImportMessage(mode, 'Creating import session...');
        try {
            const totalParts = Math.ceil(file.size / importChunkSize);
            const session = await createDocumentImportSession({
                project_id: projectId,
                file_name: file.name,
                title: titleFromMarkdownFile(file),
                total_size: file.size,
                total_parts: totalParts,
                chunk_size: importChunkSize,
                collaboration_mode: mode,
            });
            uploadID = session.upload_id;

            const assets = assetFilesByMode[mode] || [];
            for (let index = 0; index < assets.length; index += 1) {
                const asset = assets[index];
                setImportMessage(mode, `Uploading image ${index + 1}/${assets.length}...`);
                await uploadDocumentImportAsset(uploadID, asset, originalAssetPath(asset));
            }

            const chunkSize = Number(session.chunk_size) || importChunkSize;
            const partCount = Number(session.total_parts) || totalParts;
            for (let partNo = 1; partNo <= partCount; partNo += 1) {
                const start = (partNo - 1) * chunkSize;
                const end = Math.min(start + chunkSize, file.size);
                setImportMessage(mode, `Uploading Markdown chunk ${partNo}/${partCount}...`);
                await uploadDocumentImportPart(uploadID, partNo, file.slice(start, end));
            }

            setImportMessage(mode, 'Assembling document...');
            const result = await completeDocumentImport(uploadID);
            if (result?.task) {
                patchTask(result.task);
                setSelectedTask(result.task);
            }
            setAssetFilesByMode((prev) => ({ ...prev, [mode]: [] }));
            setImportMessage(mode, `Imported "${result?.task?.title || file.name}".`);
            loadActivities({ silent: true });
        } catch (err) {
            if (uploadID) {
                await abortDocumentImport(uploadID).catch(() => undefined);
            }
            setImportMessage(mode, err.message || 'Failed to import Markdown document.');
        } finally {
            setImportingMode('');
        }
    }, [assetFilesByMode, importingMode, loadActivities, patchTask, projectId, setImportMessage]);

    const onToggleTask = async (task) => {
        const nextStatus = task.status === 'done' ? 'todo' : 'done';
        const entityLabel = isTodoTask(task) ? 'todo' : 'document';
        try {
            await updateTask(task.project_id, task.id, { status: nextStatus }, task.version);
            patchTask(optimisticTaskUpdate(task, { status: nextStatus }));
            loadActivities({ silent: true });
        } catch (err) {
            alert(err.message || `Failed to update ${entityLabel}`);
            await loadData();
        }
    };

    const onDeleteTask = async (task) => {
        const entityLabel = isTodoTask(task) ? 'todo' : 'document';
        if (!window.confirm(`Move ${entityLabel} "${task.title}" to trash?`)) return;
        try {
            await deleteTask(task.id);
            setTasks((prev) => removeTask(prev, task.id));
            setSelectedTask((prev) => (prev?.id === task.id ? null : prev));
            loadActivities({ silent: true });
        } catch (err) {
            alert(err.message || `Failed to move ${entityLabel} to trash`);
            await loadData();
        }
    };

    const onPanelTaskUpdated = useCallback(async (task) => {
        if (task?.id) {
            patchTask(task);
            loadActivities({ silent: true });
            return;
        }
        await loadData();
        loadActivities({ silent: true });
    }, [loadActivities, loadData, patchTask]);

    const onOpenActivityTask = useCallback(async (activity) => {
        if (activity?.event_type === 'task.deleted' || !Number(activity?.task_id)) return;

        const existing = tasks.find((item) => item.id === activity.task_id);
        if (existing) {
            setSelectedTask(existing);
            return;
        }

        if (activity?.task?.project_id === projectId) {
            patchTask(activity.task);
            setSelectedTask(activity.task);
            return;
        }

        try {
            const loaded = await getTaskById(activity.task_id);
            if (loaded?.project_id === projectId) {
                patchTask(loaded);
                setSelectedTask(loaded);
            }
        } catch (err) {
            alert(err.message || 'Failed to open document');
        }
    }, [patchTask, projectId, tasks]);

    const refreshSpace = useCallback(async () => {
        await Promise.all([
            loadData(),
            loadActivities(),
        ]);
    }, [loadActivities, loadData]);

    const realtimeStatusLabel = {
        [projectConnectionStatus.CONNECTING]: 'Connecting',
        [projectConnectionStatus.CONNECTED]: 'Live',
        [projectConnectionStatus.DISCONNECTED]: 'Offline',
        [projectConnectionStatus.ERROR]: 'Reconnecting',
    }[realtimeStatus] || 'Offline';

    const renderDocumentBlock = (mode) => {
        const isPrivate = mode === documentModes.PRIVATE;
        const draftTitle = draftTitles[mode] || '';
        const selectedAssets = assetFilesByMode[mode] || [];
        const isCreating = creatingMode === mode;
        const isImporting = importingMode === mode;
        const title = isPrivate ? 'Private Documents' : 'Collaborative Documents';
        const eyebrow = isPrivate ? 'Personal block' : 'Shared block';
        const description = isPrivate
            ? 'A quiet Notion-style block for private Markdown drafts. No collaborators can be added.'
            : 'A live block for team Markdown docs with WebSocket sync, presence, and metadata locks.';

        return (
            <section className={`yq-document-block ${isPrivate ? 'is-private' : 'is-collab'}`}>
                <div>
                    <span className="yq-document-block-eyebrow">{eyebrow}</span>
                    <h2>{title}</h2>
                    <p>{description}</p>
                </div>

                <div className="yq-document-block-actions">
                    <input
                        type="text"
                        className="yq-document-block-input"
                        value={draftTitle}
                        placeholder={isPrivate ? 'Name a private document' : 'Name a collaborative document'}
                        onChange={(event) => updateDraftTitle(mode, event.target.value)}
                        onKeyDown={(event) => {
                            if (event.key === 'Enter') {
                                event.preventDefault();
                                onCreateDocument(mode);
                            }
                        }}
                    />
                    <Button onClick={() => onCreateDocument(mode)} disabled={!draftTitle.trim() || Boolean(creatingMode)}>
                        {isCreating ? 'Creating...' : 'New'}
                    </Button>
                </div>

                <div className="yq-import-row">
                    <label className="yq-import-picker">
                        <span>{selectedAssets.length > 0 ? `${selectedAssets.length} image(s)` : 'Attach images'}</span>
                        <input
                            type="file"
                            accept="image/png,image/jpeg,image/webp,image/gif"
                            multiple
                            onChange={(event) => onAssetFilesSelected(mode, event.target.files)}
                            disabled={isImporting}
                        />
                    </label>
                    <label className="yq-import-picker yq-import-picker-primary">
                        <span>{isImporting ? 'Importing...' : 'Import .md'}</span>
                        <input
                            type="file"
                            accept=".md,.markdown,text/markdown,text/plain"
                            onChange={(event) => {
                                const file = event.target.files?.[0];
                                event.target.value = '';
                                onImportMarkdown(mode, file);
                            }}
                            disabled={isImporting}
                        />
                    </label>
                </div>

                {importMessages[mode] ? (
                    <div className="yq-import-message">{importMessages[mode]}</div>
                ) : null}
            </section>
        );
    };

    const renderTodoBlock = () => (
        <section className="yq-document-block is-todo">
            <div>
                <span className="yq-document-block-eyebrow">Todo lane</span>
                <h2>Lightweight Todos</h2>
                <p>
                    Capture action items inside the current space without pulling the main experience away from docs.
                    These items stay visible in the secondary todo views and can still be discussed in comments.
                </p>
            </div>

            <div className="yq-document-block-actions">
                <input
                    type="text"
                    className="yq-document-block-input"
                    value={todoDraftTitle}
                    placeholder="Capture a todo title"
                    onChange={(event) => setTodoDraftTitle(event.target.value)}
                    onKeyDown={(event) => {
                        if (event.key === 'Enter') {
                            event.preventDefault();
                            onCreateTodo();
                        }
                    }}
                />
                <Button onClick={onCreateTodo} disabled={!todoDraftTitle.trim() || creatingTodo}>
                    {creatingTodo ? 'Creating...' : 'New'}
                </Button>
            </div>

            <div className="yq-import-message">
                Use this for reminders and action items. Collaborative docs and meetings stay in the document blocks.
            </div>
        </section>
    );

    if (loading) {
        return <div className="yq-page-container">Loading space...</div>;
    }
    if (error) {
        return <div className="yq-page-container yq-error">{error}</div>;
    }
    if (!project) {
        return <div className="yq-page-container yq-error">Space not found.</div>;
    }

    return (
        <div className="yq-page-container yq-board-page">
            <div className="yq-page-header yq-board-header">
                <div className="yq-board-title-wrap">
                    <span className="yq-project-color" style={{ backgroundColor: project.color }} />
                    <div>
                        <span className="yq-kicker">Space</span>
                        <h1>{project.name}</h1>
                    </div>
                </div>
                <div className="yq-board-tools">
                    <span className="yq-presence-pill" title={onlineUsers.map((user) => user.username).join(', ')}>
                        Online: {onlineUsers.length}
                    </span>
                    <span className={`yq-realtime-status ${realtimeStatus}`}>
                        Space sync: {realtimeStatusLabel}
                        {lastCursor > 0 ? ` #${lastCursor}` : ''}
                    </span>
                    <Button variant="secondary" onClick={refreshSpace}>Refresh</Button>
                </div>
            </div>

            {realtimeError ? <div className="yq-realtime-error">{realtimeError}</div> : null}

            <div className="yq-document-block-grid">
                {renderDocumentBlock(documentModes.COLLABORATIVE)}
                {renderDocumentBlock(documentModes.PRIVATE)}
                {renderTodoBlock()}
            </div>

            <ProjectActivityPanel
                activities={activities}
                loading={activityLoading}
                loadingMore={activityLoadingMore}
                error={activityError}
                hasMore={activityHasMore}
                onRefresh={() => loadActivities()}
                onLoadMore={() => loadActivities({ cursor: activityCursor, append: true })}
                onOpenTask={onOpenActivityTask}
            />

            <TaskSection
                title="Documents Needing Attention"
                tasks={attentionDocuments}
                emptyText="No documents need attention."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
                completeAriaLabel="Archive document"
                restoreAriaLabel="Restore document"
                expiredLabel="Needs attention"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Todos Needing Attention"
                tasks={attentionTodos}
                emptyText="No todo reminders are overdue."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
                completeLabel="Done"
                restoreLabel="Undo"
                detailsLabel="Open"
                completeAriaLabel="Mark todo as done"
                restoreAriaLabel="Reopen todo"
                expiredLabel="Overdue"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Collaborative Documents"
                tasks={collaborativeTasks}
                emptyText="No collaborative documents in this space."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
                completeAriaLabel="Archive document"
                restoreAriaLabel="Restore document"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Private Documents"
                tasks={privateTasks}
                emptyText="No private documents yet."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
                completeAriaLabel="Archive document"
                restoreAriaLabel="Restore document"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Archived Documents"
                tasks={archivedDocuments}
                emptyText="No archived documents."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
                completeAriaLabel="Archive document"
                restoreAriaLabel="Restore document"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Todo Lane"
                tasks={todoItems}
                emptyText="No open todos in this space."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
                completeLabel="Done"
                restoreLabel="Undo"
                detailsLabel="Open"
                completeAriaLabel="Mark todo as done"
                restoreAriaLabel="Reopen todo"
                expiredLabel="Overdue"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Completed Todos"
                tasks={completedTodos}
                emptyText="No completed todos yet."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
                completeLabel="Done"
                restoreLabel="Undo"
                detailsLabel="Open"
                completeAriaLabel="Mark todo as done"
                restoreAriaLabel="Reopen todo"
                expiredLabel="Overdue"
                dueLabel="Reminder"
            />

            <TaskDetailPanel
                isOpen={Boolean(selectedTask)}
                onClose={() => setSelectedTask(null)}
                task={selectedTask}
                viewers={selectedTaskViewers}
                project={project}
                onTaskUpdated={onPanelTaskUpdated}
                currentUserID={currentUserID}
                taskLock={selectedTask ? metadataLockForTask(locksByKey, selectedTask.id) : null}
                lockError={selectedTask ? lockErrors[projectLockKey(selectedTask.id, METADATA_LOCK_FIELD)] : ''}
                onLockRequest={requestTaskLock}
                onLockRelease={releaseTaskLock}
            />
        </div>
    );
}
