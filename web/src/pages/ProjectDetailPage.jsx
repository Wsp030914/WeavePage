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
import { createTask, deleteTask, getTaskById, getTasks, updateTask } from '../api/task';
import Button from '../components/Button';
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
import './ProjectDetailPage.css';

const documentModes = {
    COLLABORATIVE: 'collaborative',
    PRIVATE: 'private',
};

const importChunkSize = 1024 * 1024;

function isPrivateDocument(task) {
    return task?.collaboration_mode === documentModes.PRIVATE;
}

function titleFromMarkdownFile(file) {
    return (file?.name || '').replace(/\.(md|markdown)$/i, '').trim();
}

function originalAssetPath(file) {
    return file?.webkitRelativePath || file?.name || '';
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
    const [selectedTask, setSelectedTask] = useState(null);

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

    useEffect(() => {
        cursorRef.current = 0;
        setLastCursor(0);
        setPresenceByNode({});
        setLocksByKey({});
        setLockErrors({});
        loadData();
    }, [loadData]);

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

    const collaborativeTasks = useMemo(
        () => todoTasks.filter((task) => !isPrivateDocument(task)),
        [todoTasks],
    );

    const privateTasks = useMemo(
        () => todoTasks.filter(isPrivateDocument),
        [todoTasks],
    );

    const onlineUsers = useMemo(
        () => flattenPresenceUsers(presenceByNode),
        [presenceByNode],
    );

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
        } catch (err) {
            alert(err.message || 'Failed to create document');
        } finally {
            setCreatingMode('');
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
        } catch (err) {
            if (uploadID) {
                await abortDocumentImport(uploadID).catch(() => undefined);
            }
            setImportMessage(mode, err.message || 'Failed to import Markdown document.');
        } finally {
            setImportingMode('');
        }
    }, [assetFilesByMode, importingMode, patchTask, projectId, setImportMessage]);

    const onToggleTask = async (task) => {
        const nextStatus = task.status === 'done' ? 'todo' : 'done';
        try {
            await updateTask(task.project_id, task.id, { status: nextStatus }, task.version);
            patchTask(optimisticTaskUpdate(task, { status: nextStatus }));
        } catch (err) {
            alert(err.message || 'Failed to update document');
            await loadData();
        }
    };

    const onDeleteTask = async (task) => {
        if (!window.confirm(`Delete document "${task.title}"?`)) return;
        try {
            await deleteTask(task.id);
            setTasks((prev) => removeTask(prev, task.id));
            setSelectedTask((prev) => (prev?.id === task.id ? null : prev));
        } catch (err) {
            alert(err.message || 'Failed to delete document');
            await loadData();
        }
    };

    const onPanelTaskUpdated = useCallback(async (task) => {
        if (task?.id) {
            patchTask(task);
            return;
        }
        await loadData();
    }, [loadData, patchTask]);

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
                    <Button variant="secondary" onClick={loadData}>Refresh</Button>
                </div>
            </div>

            {realtimeError ? <div className="yq-realtime-error">{realtimeError}</div> : null}

            <div className="yq-document-block-grid">
                {renderDocumentBlock(documentModes.COLLABORATIVE)}
                {renderDocumentBlock(documentModes.PRIVATE)}
            </div>

            <TaskSection
                title="Needs Attention"
                tasks={expiredTasks}
                emptyText="No documents need attention."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
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
            />

            <TaskSection
                title="Archived Documents"
                tasks={doneTasks}
                emptyText="No archived documents."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                locksByKey={locksByKey}
                currentUserID={currentUserID}
            />

            <TaskDetailPanel
                isOpen={Boolean(selectedTask)}
                onClose={() => setSelectedTask(null)}
                task={selectedTask}
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
