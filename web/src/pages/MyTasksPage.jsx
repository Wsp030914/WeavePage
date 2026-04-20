import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { deleteTask, getTasksAcrossProjects, updateTask } from '../api/task';
import useProjects from '../store/useProjects';
import Button from '../components/Button';
import TaskSection from '../components/TaskSection';
import TaskDetailPanel from '../components/TaskDetailPanel';
import {
    applySelectedTaskSnapshot,
    applyTaskSnapshot,
    optimisticTaskUpdate,
    removeTask,
} from '../store/collab-store';
import { splitTasksByLifecycle } from '../utils/taskExpiration';
import { filterTasksByDueRange } from '../utils/taskDueRange';
import { endOfShanghaiDay, startOfShanghaiDay } from '../utils/shanghaiTime';
import { isTodoTask } from '../utils/taskTypes';
import './ProjectDetailPage.css';

function getTodayRange() {
    const now = new Date();
    const start = startOfShanghaiDay(now) || now;
    const end = endOfShanghaiDay(now) || now;
    return { start, end };
}

export default function MyTasksPage() {
    const navigate = useNavigate();
    const { projects } = useProjects();

    const [tasks, setTasks] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [selectedTask, setSelectedTask] = useState(null);

    const loadTasks = useCallback(async () => {
        setLoading(true);
        try {
            const { start, end } = getTodayRange();
            const allTasks = await getTasksAcrossProjects(projects.map((project) => project.id), 100);
            const list = filterTasksByDueRange(allTasks.filter(isTodoTask), start, end);
            setTasks(list);
            setError('');
        } catch (err) {
            setError(err.message || 'Failed to load todos');
        } finally {
            setLoading(false);
        }
    }, [projects]);

    useEffect(() => {
        loadTasks();
    }, [loadTasks]);

    const isTaskVisible = useCallback((task) => {
        const { start, end } = getTodayRange();
        return isTodoTask(task) && filterTasksByDueRange([task], start, end).length > 0;
    }, []);

    const patchTask = useCallback((task) => {
        if (!task?.id) return;
        setTasks((prev) => applyTaskSnapshot(prev, task, isTaskVisible));
        setSelectedTask((prev) => applySelectedTaskSnapshot(prev, task));
    }, [isTaskVisible]);

    const { expired: expiredTasks, todo: todoTasks, done: doneTasks } = useMemo(
        () => splitTasksByLifecycle(tasks),
        [tasks],
    );

    const onToggleTask = async (task) => {
        const nextStatus = task.status === 'done' ? 'todo' : 'done';
        try {
            const updatedTask = await updateTask(task.project_id, task.id, { status: nextStatus }, task.version);
            patchTask(updatedTask || optimisticTaskUpdate(task, { status: nextStatus }));
        } catch (err) {
            alert(err.message || 'Failed to update todo');
            await loadTasks();
        }
    };

    const onDeleteTask = async (task) => {
        if (!window.confirm(`Move todo "${task.title}" to trash?`)) return;
        try {
            await deleteTask(task.id);
            setTasks((prev) => removeTask(prev, task.id));
            setSelectedTask((prev) => (prev?.id === task.id ? null : prev));
        } catch (err) {
            alert(err.message || 'Failed to move todo to trash');
            await loadTasks();
        }
    };

    const onPanelTaskUpdated = useCallback(async (task) => {
        if (task?.id) {
            patchTask(task);
            return;
        }
        await loadTasks();
    }, [loadTasks, patchTask]);

    const selectedProject = selectedTask
        ? projects.find((project) => project.id === selectedTask.project_id) || null
        : null;

    if (loading) {
        return <div className="yq-page-container">Loading today's todos...</div>;
    }
    if (error) {
        return <div className="yq-page-container yq-error">{error}</div>;
    }

    return (
        <div className="yq-page-container yq-board-page">
            <div className="yq-page-header yq-board-header">
                <div>
                    <span className="yq-kicker">Todos</span>
                    <h1>Today</h1>
                </div>
                <div className="yq-board-tools">
                    <Button variant="secondary" onClick={loadTasks}>Refresh</Button>
                </div>
            </div>

            <TaskSection
                title="Overdue"
                tasks={expiredTasks}
                emptyText="No overdue todos today."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                onOpenProject={(task) => navigate(`/projects/${task.project_id}`)}
                completeLabel="Done"
                restoreLabel="Undo"
                projectLabel="Space"
                detailsLabel="Open"
                completeAriaLabel="Mark todo as done"
                restoreAriaLabel="Reopen todo"
                expiredLabel="Overdue"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Open"
                tasks={todoTasks}
                emptyText="No active todos due today."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                onOpenProject={(task) => navigate(`/projects/${task.project_id}`)}
                completeLabel="Done"
                restoreLabel="Undo"
                projectLabel="Space"
                detailsLabel="Open"
                completeAriaLabel="Mark todo as done"
                restoreAriaLabel="Reopen todo"
                expiredLabel="Overdue"
                dueLabel="Reminder"
            />

            <TaskSection
                title="Completed"
                tasks={doneTasks}
                emptyText="No completed todos today."
                onToggleStatus={onToggleTask}
                onOpenDetails={setSelectedTask}
                onDeleteTask={onDeleteTask}
                onOpenProject={(task) => navigate(`/projects/${task.project_id}`)}
                completeLabel="Done"
                restoreLabel="Undo"
                projectLabel="Space"
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
                project={selectedProject}
                onTaskUpdated={onPanelTaskUpdated}
            />
        </div>
    );
}
