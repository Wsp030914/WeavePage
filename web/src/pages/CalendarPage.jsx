import React, { useCallback, useEffect, useMemo, useState } from 'react';
import Button from '../components/Button';
import TaskDetailPanel from '../components/TaskDetailPanel';
import { getTasksAcrossProjects } from '../api/task';
import {
    applySelectedTaskSnapshot,
    applyTaskSnapshot,
} from '../store/collab-store';
import useProjects from '../store/useProjects';
import {
    addShanghaiDays,
    addShanghaiMonths,
    addShanghaiYears,
    createShanghaiDate,
    formatShanghaiDateTime,
    getShanghaiDateKey,
    getShanghaiMonthGrid,
    getShanghaiParts,
    getShanghaiWeekStart,
    parseDateInShanghai,
    isSameShanghaiDay,
    startOfShanghaiDay,
} from '../utils/shanghaiTime';
import { isTodoTask } from '../utils/taskTypes';
import './CalendarPage.css';

const VIEW_OPTIONS = [
    { key: 'year', label: 'Year' },
    { key: 'month', label: 'Month' },
    { key: 'week', label: 'Week' },
    { key: 'day', label: 'Day' },
    { key: 'agenda', label: 'Agenda' },
];

const STATUS_OPTIONS = [
    { key: 'all', label: 'All' },
    { key: 'todo', label: 'Open' },
    { key: 'done', label: 'Done' },
];

const WEEKDAY_LABELS = [
    '周一',
    '周二',
    '周三',
    '周四',
    '周五',
    '周六',
    '周日',
];

function titleForView(view, currentDate) {
    const parts = getShanghaiParts(currentDate);
    if (!parts) return '';

    if (view === 'year') return `${parts.year} 年`;
    if (view === 'month') return `${parts.year} 年 ${parts.month + 1} 月`;
    if (view === 'day') {
        return formatShanghaiDateTime(currentDate, {
            year: 'numeric',
            month: 'long',
            day: 'numeric',
            weekday: 'long',
        });
    }
    if (view === 'week') {
        const start = getShanghaiWeekStart(currentDate);
        const end = addShanghaiDays(start, 6);
        return `${formatShanghaiDateTime(start, { month: 'numeric', day: 'numeric' })} - ${formatShanghaiDateTime(end, { month: 'numeric', day: 'numeric' })}`;
    }
    return `${parts.year} 年 ${parts.month + 1} 月 日程`;
}

function getTodoScheduleDate(task) {
    return parseDateInShanghai(task?.due_at || task?.created_at || null);
}

function getTaskMapByDate(tasks) {
    const map = new Map();

    tasks.forEach((task) => {
        const date = getTodoScheduleDate(task);
        if (!date) return;
        const key = getShanghaiDateKey(date);
        if (!map.has(key)) map.set(key, []);
        map.get(key).push({ ...task, __calendarDate: date });
    });

    map.forEach((list) => {
        list.sort((a, b) => new Date(a.__calendarDate).getTime() - new Date(b.__calendarDate).getTime());
    });

    return map;
}

function TaskChip({ task, onOpen }) {
    return (
        <button
            type="button"
            className={`yq-cal-task-chip ${task.status === 'done' ? 'done' : 'todo'}`}
            onClick={() => onOpen(task)}
            title={task.title}
        >
            <span className="yq-cal-task-dot" />
            <span className="yq-cal-task-text">{task.title}</span>
        </button>
    );
}

function MonthCell({ date, currentMonth, taskMap, onOpenTask, onPickDay }) {
    const key = getShanghaiDateKey(date);
    const tasks = taskMap.get(key) || [];
    const parts = getShanghaiParts(date);
    const isCurrentMonth = parts?.month === currentMonth;
    const isToday = isSameShanghaiDay(date, new Date());

    return (
        <div className={`yq-cal-cell ${isCurrentMonth ? '' : 'other'} ${isToday ? 'today' : ''}`}>
            <button type="button" className="yq-cal-day-number" onClick={() => onPickDay(date)}>
                {parts?.day || ''}
            </button>
            <div className="yq-cal-day-tasks">
                {tasks.slice(0, 3).map((task) => (
                    <TaskChip key={task.id} task={task} onOpen={onOpenTask} />
                ))}
                {tasks.length > 3 && (
                    <button type="button" className="yq-cal-more" onClick={() => onPickDay(date)}>
                        +{tasks.length - 3} 更多
                    </button>
                )}
            </div>
        </div>
    );
}

function WeekColumn({ date, taskMap, onOpenTask, onPickDay }) {
    const key = getShanghaiDateKey(date);
    const tasks = taskMap.get(key) || [];
    const isToday = isSameShanghaiDay(date, new Date());

    return (
        <div className={`yq-cal-week-col ${isToday ? 'today' : ''}`}>
            <button type="button" className="yq-cal-week-head" onClick={() => onPickDay(date)}>
                <strong>{formatShanghaiDateTime(date, { weekday: 'short' })}</strong>
                <span>{formatShanghaiDateTime(date, { month: 'numeric', day: 'numeric' })}</span>
            </button>
            <div className="yq-cal-week-tasks">
                {tasks.length === 0 ? (
                    <span className="yq-cal-empty">No reminders</span>
                ) : (
                    tasks.map((task) => <TaskChip key={task.id} task={task} onOpen={onOpenTask} />)
                )}
            </div>
        </div>
    );
}

export default function CalendarPage() {
    const { projects } = useProjects();
    const [view, setView] = useState('month');
    const [status, setStatus] = useState('all');
    const [anchorDate, setAnchorDate] = useState(new Date());
    const [tasks, setTasks] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState('');
    const [selectedTask, setSelectedTask] = useState(null);

    const loadTasks = useCallback(async () => {
        setLoading(true);
        try {
            const allTasks = await getTasksAcrossProjects(projects.map((project) => project.id), 100);
            setTasks(allTasks.filter(isTodoTask));
            setError('');
        } catch (err) {
            setError(err.message || '加载待办日历失败');
        } finally {
            setLoading(false);
        }
    }, [projects]);

    useEffect(() => {
        loadTasks();
    }, [loadTasks]);

    const patchTask = useCallback((task) => {
        if (!task?.id) return;
        setTasks((prev) => applyTaskSnapshot(prev, task, isTodoTask));
        setSelectedTask((prev) => applySelectedTaskSnapshot(prev, task));
    }, []);

    const onPanelTaskUpdated = useCallback(async (task) => {
        if (task?.id) {
            patchTask(task);
            return;
        }
        await loadTasks();
    }, [loadTasks, patchTask]);

    const visibleTasks = useMemo(() => {
        if (status === 'all') return tasks;
        return tasks.filter((task) => task.status === status);
    }, [status, tasks]);

    const taskMap = useMemo(() => getTaskMapByDate(visibleTasks), [visibleTasks]);

    const projectMap = useMemo(() => {
        const map = new Map();
        projects.forEach((project) => map.set(project.id, project));
        return map;
    }, [projects]);

    const selectedProject = selectedTask ? projectMap.get(selectedTask.project_id) || null : null;

    const onPrev = () => {
        setAnchorDate((prev) => {
            if (view === 'year') return addShanghaiYears(prev, -1);
            if (view === 'month' || view === 'agenda') return addShanghaiMonths(prev, -1);
            if (view === 'week') return addShanghaiDays(prev, -7);
            return addShanghaiDays(prev, -1);
        });
    };

    const onNext = () => {
        setAnchorDate((prev) => {
            if (view === 'year') return addShanghaiYears(prev, 1);
            if (view === 'month' || view === 'agenda') return addShanghaiMonths(prev, 1);
            if (view === 'week') return addShanghaiDays(prev, 7);
            return addShanghaiDays(prev, 1);
        });
    };

    const onToday = () => setAnchorDate(new Date());

    const openDayView = (date) => {
        setAnchorDate(startOfShanghaiDay(date) || date);
        setView('day');
    };

    const monthGrid = getShanghaiMonthGrid(anchorDate);
    const monthParts = getShanghaiParts(anchorDate) || getShanghaiParts(new Date());
    const weekStart = getShanghaiWeekStart(anchorDate);

    const weekDays = useMemo(() => {
        if (!weekStart) return [];
        return Array.from({ length: 7 }, (_, idx) => addShanghaiDays(weekStart, idx));
    }, [weekStart]);

    const dayKey = getShanghaiDateKey(anchorDate);
    const dayTasks = taskMap.get(dayKey) || [];

    const agendaGroups = useMemo(() => {
        if (!monthParts) return [];
        const firstDay = createShanghaiDate(monthParts.year, monthParts.month, 1);
        const days = [];
        const dayCount = new Date(Date.UTC(monthParts.year, monthParts.month + 1, 0)).getUTCDate();

        for (let i = 0; i < dayCount; i += 1) {
            const date = addShanghaiDays(firstDay, i);
            const key = getShanghaiDateKey(date);
            const list = taskMap.get(key) || [];
            if (list.length > 0) days.push({ date, tasks: list });
        }
        return days;
    }, [monthParts, taskMap]);

    if (loading) return <div className="yq-page-container">Loading todo calendar...</div>;
    if (error) return <div className="yq-page-container yq-error">{error}</div>;

    return (
        <div className="yq-page-container yq-calendar-page">
            <div className="yq-calendar-toolbar">
                <div className="yq-calendar-title-wrap">
                    <span className="yq-kicker">Todos</span>
                    <h1>{titleForView(view, anchorDate)}</h1>
                    <div className="yq-calendar-nav-btns">
                        <Button variant="secondary" data-testid="calendar-prev" onClick={onPrev}>
                            Previous
                        </Button>
                        <Button variant="secondary" data-testid="calendar-today" onClick={onToday}>
                            Today
                        </Button>
                        <Button variant="secondary" data-testid="calendar-next" onClick={onNext}>
                            Next
                        </Button>
                    </div>
                </div>

                <div className="yq-calendar-actions">
                    <div className="yq-view-switch">
                        {VIEW_OPTIONS.map((item) => (
                            <button
                                key={item.key}
                                type="button"
                                data-testid={`calendar-view-${item.key}`}
                                className={`yq-view-btn ${view === item.key ? 'active' : ''}`}
                                onClick={() => setView(item.key)}
                            >
                                {item.label}
                            </button>
                        ))}
                    </div>
                    <select
                        data-testid="calendar-status-filter"
                        className="yq-input yq-calendar-filter"
                        value={status}
                        onChange={(event) => setStatus(event.target.value)}
                    >
                        {STATUS_OPTIONS.map((item) => (
                            <option key={item.key} value={item.key}>{item.label}</option>
                        ))}
                    </select>
                </div>
            </div>

            {view === 'year' && (
                <div className="yq-calendar-year-grid">
                    {Array.from({ length: 12 }, (_, monthIdx) => {
                        const monthDate = createShanghaiDate(monthParts.year, monthIdx, 1);
                        const cells = getShanghaiMonthGrid(monthDate).slice(0, 35);
                        return (
                            <article key={monthIdx} className="yq-year-month">
                                <button
                                    type="button"
                                    className="yq-year-month-title"
                                    onClick={() => {
                                        setAnchorDate(monthDate);
                                        setView('month');
                                    }}
                                >
                                    {monthIdx + 1} 月
                                </button>
                                <div className="yq-year-weekday-row">
                                    {WEEKDAY_LABELS.map((label) => <span key={label}>{label.slice(1)}</span>)}
                                </div>
                                <div className="yq-year-month-grid">
                                    {cells.map((date) => {
                                        const key = getShanghaiDateKey(date);
                                        const count = (taskMap.get(key) || []).length;
                                        const parts = getShanghaiParts(date);
                                        const classes = ['yq-year-day'];
                                        if (parts?.month !== monthIdx) classes.push('other');
                                        if (count > 0) classes.push('has-tasks');
                                        return (
                                            <button
                                                type="button"
                                                key={key}
                                                className={classes.join(' ')}
                                                onClick={() => openDayView(date)}
                                            >
                                                <span>{parts?.day}</span>
                                                {count > 0 && <i>{count}</i>}
                                            </button>
                                        );
                                    })}
                                </div>
                            </article>
                        );
                    })}
                </div>
            )}

            {view === 'month' && (
                <div className="yq-calendar-month">
                    <div className="yq-cal-weekday-head">
                        {WEEKDAY_LABELS.map((label) => <span key={label}>{label}</span>)}
                    </div>
                    <div className="yq-cal-month-grid">
                        {monthGrid.map((date) => (
                            <MonthCell
                                key={getShanghaiDateKey(date)}
                                date={date}
                                currentMonth={monthParts.month}
                                taskMap={taskMap}
                                onOpenTask={setSelectedTask}
                                onPickDay={openDayView}
                            />
                        ))}
                    </div>
                </div>
            )}

            {view === 'week' && (
                <div className="yq-cal-week-grid">
                    {weekDays.map((date) => (
                        <WeekColumn
                            key={getShanghaiDateKey(date)}
                            date={date}
                            taskMap={taskMap}
                            onOpenTask={setSelectedTask}
                            onPickDay={openDayView}
                        />
                    ))}
                </div>
            )}

            {view === 'day' && (
                <div className="yq-cal-day-view">
                    <h3>{formatShanghaiDateTime(anchorDate, { month: 'long', day: 'numeric', weekday: 'long' })}</h3>
                    <div className="yq-cal-day-list">
                        {dayTasks.length === 0 ? (
                            <div className="yq-empty-state">No reminders on this day.</div>
                        ) : (
                            dayTasks.map((task) => (
                                <button
                                    type="button"
                                    key={task.id}
                                    className={`yq-cal-day-item ${task.status === 'done' ? 'done' : ''}`}
                                    onClick={() => setSelectedTask(task)}
                                >
                                    <div>
                                        <strong>{task.title}</strong>
                                        <p>{task.content_md ? task.content_md.slice(0, 80) : 'No notes yet.'}</p>
                                    </div>
                                    <time>
                                        {formatShanghaiDateTime(task.__calendarDate, {
                                            hour: '2-digit',
                                            minute: '2-digit',
                                            hour12: false,
                                        })}
                                    </time>
                                </button>
                            ))
                        )}
                    </div>
                </div>
            )}

            {view === 'agenda' && (
                <div className="yq-cal-agenda">
                    {agendaGroups.length === 0 ? (
                        <div className="yq-empty-state">No reminders this month.</div>
                    ) : (
                        agendaGroups.map((group) => (
                            <section key={getShanghaiDateKey(group.date)} className="yq-agenda-group">
                                <h3>{formatShanghaiDateTime(group.date, { month: 'long', day: 'numeric', weekday: 'long' })}</h3>
                                <div className="yq-agenda-items">
                                    {group.tasks.map((task) => (
                                        <button
                                            key={task.id}
                                            type="button"
                                            className={`yq-agenda-item ${task.status === 'done' ? 'done' : ''}`}
                                            onClick={() => setSelectedTask(task)}
                                        >
                                            <span>{task.title}</span>
                                            <time>
                                                {formatShanghaiDateTime(task.__calendarDate, {
                                                    hour: '2-digit',
                                                    minute: '2-digit',
                                                    hour12: false,
                                                })}
                                            </time>
                                        </button>
                                    ))}
                                </div>
                            </section>
                        ))
                    )}
                </div>
            )}

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

