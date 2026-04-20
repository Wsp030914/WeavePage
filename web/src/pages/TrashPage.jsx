import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Button from '../components/Button';
import { deleteTrashedTask, getTrashTasks, restoreTrashTask } from '../api/task';
import { deleteTrashedSpace, getTrashSpaces, restoreTrashSpace } from '../api/project';
import useProjects from '../store/useProjects';
import { getTaskDocTypeLabel, getTaskStatusLabel } from '../utils/taskTypes';
import './TrashPage.css';

function formatShanghaiDateTime(value) {
    if (!value) return 'Unknown time';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return 'Unknown time';
    return new Intl.DateTimeFormat('zh-CN', {
        timeZone: 'Asia/Shanghai',
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
    }).format(date);
}

export default function TrashPage() {
    const navigate = useNavigate();
    const { projects } = useProjects();

    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [busyKey, setBusyKey] = useState('');

    const projectNameByID = useMemo(() => {
        const map = new Map();
        (projects || []).forEach((project) => {
            if (!project?.id) return;
            map.set(project.id, project.name || `Space #${project.id}`);
        });
        return map;
    }, [projects]);

    const loadTrash = useCallback(async () => {
        setLoading(true);
        try {
            const [taskResult, spaceResult] = await Promise.all([
                getTrashTasks({ page: 1, size: 100 }),
                getTrashSpaces({ page: 1, size: 100 }),
            ]);
            const taskItems = (Array.isArray(taskResult?.list) ? taskResult.list : [])
                .map((item) => ({ ...item, trash_kind: 'task' }));
            const spaceItems = (Array.isArray(spaceResult?.list) ? spaceResult.list : [])
                .map((item) => ({ ...item, trash_kind: 'space', title: item.name, project_id: item.id }));
            setItems([...spaceItems, ...taskItems].sort((a, b) => new Date(b.deleted_at || 0) - new Date(a.deleted_at || 0)));
            setError('');
        } catch (err) {
            setError(err.message || 'Failed to load trash');
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadTrash();
    }, [loadTrash]);

    const onRestore = async (item) => {
        if (!item?.id || busyKey) return;
        setBusyKey(`restore:${item.trash_kind}:${item.id}`);
        try {
            if (item.trash_kind === 'space') {
                const restoredSpace = await restoreTrashSpace(item.id);
                setItems((prev) => prev.filter((current) => current.id !== item.id || current.trash_kind !== item.trash_kind));
                if (restoredSpace?.id) navigate(`/projects/${restoredSpace.id}`);
                return;
            }
            const result = await restoreTrashTask(item.id);
            const restoredTask = result?.task || null;
            setItems((prev) => prev.filter((current) => current.id !== item.id || current.trash_kind !== item.trash_kind));
            if (restoredTask?.project_id && restoredTask?.id) {
                navigate(`/projects/${restoredTask.project_id}?task=${restoredTask.id}`);
                return;
            }
        } catch (err) {
            alert(err.message || 'Failed to restore item');
        } finally {
            setBusyKey('');
        }
    };

    const onDeletePermanently = async (item) => {
        if (!item?.id || busyKey) return;
        if (!window.confirm(`Permanently delete "${item.title}"? This cannot be undone.`)) return;

        setBusyKey(`delete:${item.trash_kind}:${item.id}`);
        try {
            if (item.trash_kind === 'space') {
                await deleteTrashedSpace(item.id);
            } else {
                await deleteTrashedTask(item.id);
            }
            setItems((prev) => prev.filter((current) => current.id !== item.id || current.trash_kind !== item.trash_kind));
        } catch (err) {
            alert(err.message || 'Failed to permanently delete item');
        } finally {
            setBusyKey('');
        }
    };

    if (loading) {
        return <div className="yq-page-container">Loading archive...</div>;
    }
    if (error) {
        return <div className="yq-page-container yq-error">{error}</div>;
    }

    return (
        <div className="yq-page-container yq-trash-page">
            <div className="yq-page-header">
                <div>
                    <span className="yq-kicker">Archive</span>
                    <h1>Trash</h1>
                </div>
                <Button variant="secondary" onClick={loadTrash} disabled={Boolean(busyKey)}>
                    Refresh
                </Button>
            </div>

            {items.length === 0 ? (
                <div className="yq-empty-state">
                    Trash is empty. Deleted docs, meeting notes, daily notes, and todos will appear here.
                </div>
            ) : (
                <div className="yq-trash-grid">
                    {items.map((item) => {
                        const key = `${item.trash_kind}:${item.id}`;
                        const restoreBusy = busyKey === `restore:${key}`;
                        const deleteBusy = busyKey === `delete:${key}`;
                        const projectName = projectNameByID.get(item.project_id) || `Space #${item.project_id}`;

                        return (
                            <article key={key} className="yq-trash-card">
                                <div className="yq-trash-card-header">
                                    <div className="yq-trash-title-wrap">
                                        <strong className="yq-trash-title">{item.title}</strong>
                                        <span className="yq-trash-type">{item.trash_kind === 'space' ? 'Space' : getTaskDocTypeLabel(item)}</span>
                                    </div>
                                    <span className="yq-trash-project">{item.trash_kind === 'space' ? 'Workspace' : projectName}</span>
                                </div>

                                <div className="yq-trash-meta">
                                    <span>Deleted {formatShanghaiDateTime(item.deleted_at)}</span>
                                    {item.due_at ? <span>Reminder {formatShanghaiDateTime(item.due_at)}</span> : null}
                                    {item.status ? <span>Status {getTaskStatusLabel(item)}</span> : null}
                                </div>

                                <div className="yq-trash-actions">
                                    <Button variant="secondary" onClick={() => onRestore(item)} disabled={Boolean(busyKey)}>
                                        {restoreBusy ? 'Restoring...' : 'Restore'}
                                    </Button>
                                    <Button variant="danger" onClick={() => onDeletePermanently(item)} disabled={Boolean(busyKey)}>
                                        {deleteBusy ? 'Deleting...' : 'Delete forever'}
                                    </Button>
                                </div>
                            </article>
                        );
                    })}
                </div>
            )}
        </div>
    );
}
