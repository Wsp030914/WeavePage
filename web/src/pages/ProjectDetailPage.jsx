import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import { getProjectById } from '../api/project';
import { createTask, deleteTask, getTasks, updateTask } from '../api/task';
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

export default function ProjectDetailPage() {
    const { id } = useParams();
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

    const [newTitle, setNewTitle] = useState('');
    const [creating, setCreating] = useState(false);
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

    const { expired: expiredTasks, todo: todoTasks, done: doneTasks } = useMemo(
        () => splitTasksByLifecycle(tasks),
        [tasks],
    );

    const onlineUsers = useMemo(
        () => flattenPresenceUsers(presenceByNode),
        [presenceByNode],
    );

    const onCreateTask = async () => {
        if (!newTitle.trim() || creating) return;
        setCreating(true);
        try {
            const task = await createTask({
                title: newTitle.trim(),
                project_id: projectId,
                status: 'todo',
            });
            setNewTitle('');
            patchTask(task);
        } catch (err) {
            alert(err.message || 'Failed to create document');
        } finally {
            setCreating(false);
        }
    };

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

            <div className="yq-quick-create">
                <input
                    type="text"
                    className="yq-quick-create-input"
                    value={newTitle}
                    placeholder="Create a collaborative Markdown document"
                    onChange={(event) => setNewTitle(event.target.value)}
                    onKeyDown={(event) => {
                        if (event.key === 'Enter') {
                            event.preventDefault();
                            onCreateTask();
                        }
                    }}
                />
                <Button onClick={onCreateTask} disabled={!newTitle.trim() || creating}>
                    {creating ? 'Creating...' : 'New Document'}
                </Button>
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
                title="Active Documents"
                tasks={todoTasks}
                emptyText="No active documents in this space."
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
