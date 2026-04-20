import React from 'react';
import { formatShanghaiDateTime } from '../utils/shanghaiTime';
import { getTaskDocTypeLabel, getTaskStatusLabel } from '../utils/taskTypes';
import Button from './Button';
import './ProjectActivityPanel.css';

function fallbackTask(activity) {
    if (activity?.task) return activity.task;
    if (!activity?.task_id) return null;
    return {
        id: activity.task_id,
        title: `Document #${activity.task_id}`,
        doc_type: 'document',
        status: 'todo',
    };
}

function formatActivityTime(value) {
    return formatShanghaiDateTime(value, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        hour12: false,
    });
}

export default function ProjectActivityPanel({
    activities,
    loading,
    loadingMore,
    error,
    hasMore,
    onRefresh,
    onLoadMore,
    onOpenTask,
}) {
    return (
        <section className="yq-activity-panel">
            <div className="yq-activity-panel-header">
                <div>
                    <span className="yq-kicker">Activity</span>
                    <h3>Recent Space Activity</h3>
                </div>
                <Button variant="secondary" onClick={onRefresh} disabled={loading}>
                    {loading ? 'Refreshing...' : 'Refresh'}
                </Button>
            </div>

            {error ? <div className="yq-activity-error">{error}</div> : null}

            {loading && activities.length === 0 ? (
                <div className="yq-activity-empty">Loading activity...</div>
            ) : null}

            {!loading && activities.length === 0 ? (
                <div className="yq-activity-empty">
                    No activity yet. Creating, updating, or archiving documents will appear here.
                </div>
            ) : null}

            {activities.length > 0 ? (
                <div className="yq-activity-list">
                    {activities.map((activity) => {
                        const task = fallbackTask(activity);
                        const openable = activity?.event_type !== 'task.deleted' && Number(activity?.task_id) > 0;

                        return (
                            <article
                                key={activity.id}
                                className={`yq-activity-item ${openable ? 'is-openable' : ''}`}
                                onClick={openable ? () => onOpenTask(activity) : undefined}
                            >
                                <div className="yq-activity-item-top">
                                    <span className="yq-activity-time">{formatActivityTime(activity.created_at)}</span>
                                    {task ? (
                                        <span className="yq-activity-type">
                                            {getTaskDocTypeLabel(task)}
                                        </span>
                                    ) : null}
                                </div>

                                <div className="yq-activity-summary">{activity.summary || 'Updated activity'}</div>

                                <div className="yq-activity-title-row">
                                    <strong>{task?.title || `Document #${activity.task_id}`}</strong>
                                    <span className="yq-activity-version">v{activity.task_version || 0}</span>
                                </div>

                                <div className="yq-activity-meta">
                                    <span>{task ? getTaskStatusLabel(task) : 'Unavailable'}</span>
                                    <span>Actor #{activity.actor_id || 0}</span>
                                </div>

                                {openable ? (
                                    <div className="yq-activity-actions">
                                        <Button variant="secondary" onClick={(event) => {
                                            event.stopPropagation();
                                            onOpenTask(activity);
                                        }}
                                        >
                                            Open
                                        </Button>
                                    </div>
                                ) : null}
                            </article>
                        );
                    })}
                </div>
            ) : null}

            {hasMore ? (
                <div className="yq-activity-footer">
                    <Button variant="secondary" onClick={onLoadMore} disabled={loadingMore}>
                        {loadingMore ? 'Loading...' : 'Load Older Activity'}
                    </Button>
                </div>
            ) : null}
        </section>
    );
}
