import React, { useDeferredValue, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Button from '../components/Button';
import Input from '../components/Input';
import { searchWorkspace } from '../api/search';
import useProjects from '../store/useProjects';
import { getTaskDocTypeLabel, getTaskStatusLabel, isTodoTask } from '../utils/taskTypes';
import './SearchPage.css';

function matchesQuery(value, query) {
    return String(value || '').toLowerCase().includes(query);
}

export default function SearchPage() {
    const navigate = useNavigate();
    const { projects, loading: projectsLoading, error: projectsError } = useProjects();
    const [query, setQuery] = useState('');
    const [searchedQuery, setSearchedQuery] = useState('');
    const [tasks, setTasks] = useState([]);
    const [taskLoading, setTaskLoading] = useState(false);
    const [taskError, setTaskError] = useState('');
    const deferredQuery = useDeferredValue(query);

    const normalizedQuery = deferredQuery.trim().toLowerCase();
    const normalizedSearchedQuery = searchedQuery.trim().toLowerCase();

    const projectByID = useMemo(() => {
        const map = new Map();
        projects.forEach((project) => map.set(project.id, project));
        return map;
    }, [projects]);

    const spaceResults = useMemo(() => {
        if (!normalizedQuery) return [];
        return projects.filter((project) => matchesQuery(project.name, normalizedQuery));
    }, [normalizedQuery, projects]);

    const taskResults = useMemo(() => {
        if (!normalizedSearchedQuery) return [];
        return tasks
            .filter((task) => (
                matchesQuery(task.title, normalizedSearchedQuery)
                || matchesQuery(task.content_md, normalizedSearchedQuery)
                || matchesQuery(getTaskDocTypeLabel(task), normalizedSearchedQuery)
                || matchesQuery(projectByID.get(task.project_id)?.name, normalizedSearchedQuery)
            ))
            .slice(0, 80);
    }, [normalizedSearchedQuery, projectByID, tasks]);

    const onSearchDocuments = async () => {
        const nextQuery = query.trim();
        if (nextQuery.length < 2 || taskLoading) return;
        setTaskLoading(true);
        setTaskError('');
        try {
            const result = await searchWorkspace(nextQuery, 80);
            setTasks(Array.isArray(result?.documents) ? result.documents : []);
            setSearchedQuery(nextQuery);
        } catch (err) {
            setTaskError(err.message || 'Failed to search documents');
        } finally {
            setTaskLoading(false);
        }
    };

    const openTask = (task) => {
        if (!task?.project_id || !task?.id) return;
        navigate(`/projects/${task.project_id}?task=${task.id}`);
    };

    const canSearchDocuments = query.trim().length >= 2 && projects.length > 0;

    return (
        <div className="yq-page-container yq-search-page">
            <div className="yq-search-hero">
                <span className="yq-kicker">Search</span>
                <h1>Find spaces, documents, meetings, and todos</h1>
                <p>
                    Search starts with local space names, then asks the backend for matching documents when you run a
                    document search.
                </p>
            </div>

            <div className="yq-search-command">
                <Input
                    value={query}
                    onChange={(event) => setQuery(event.target.value)}
                    onKeyDown={(event) => {
                        if (event.key === 'Enter') {
                            event.preventDefault();
                            onSearchDocuments();
                        }
                    }}
                    placeholder="Search by title, content, space, or type"
                    style={{ marginBottom: 0 }}
                    autoFocus
                />
                <Button onClick={onSearchDocuments} disabled={!canSearchDocuments || taskLoading}>
                    {taskLoading ? 'Searching...' : 'Search Documents'}
                </Button>
            </div>

            {projectsError ? <div className="yq-error">{projectsError}</div> : null}
            {taskError ? <div className="yq-error">{taskError}</div> : null}

            <div className="yq-search-grid">
                <section className="yq-search-panel">
                    <div className="yq-search-panel-head">
                        <span className="yq-kicker">Spaces</span>
                        <strong>{projectsLoading ? 'Loading' : `${spaceResults.length} matches`}</strong>
                    </div>
                    <div className="yq-search-results">
                        {spaceResults.map((project) => (
                            <button
                                key={project.id}
                                type="button"
                                className="yq-search-result"
                                onClick={() => navigate(`/projects/${project.id}`)}
                            >
                                <span className="yq-project-dot" style={{ backgroundColor: project.color }} />
                                <span>
                                    <strong>{project.name}</strong>
                                    <small>Space</small>
                                </span>
                            </button>
                        ))}
                        {!normalizedQuery && <div className="yq-empty-state">Type a query to search spaces.</div>}
                        {normalizedQuery && !projectsLoading && spaceResults.length === 0 && (
                            <div className="yq-empty-state">No matching spaces.</div>
                        )}
                    </div>
                </section>

                <section className="yq-search-panel">
                    <div className="yq-search-panel-head">
                        <span className="yq-kicker">Documents</span>
                        <strong>{taskLoading ? 'Loading' : `${taskResults.length} matches`}</strong>
                    </div>
                    <div className="yq-search-results">
                        {taskResults.map((task) => {
                            const project = projectByID.get(task.project_id);
                            return (
                                <button
                                    key={task.id}
                                    type="button"
                                    className={`yq-search-result ${isTodoTask(task) ? 'is-todo' : ''}`}
                                    onClick={() => openTask(task)}
                                >
                                    <span className="yq-search-type">{getTaskDocTypeLabel(task)}</span>
                                    <span>
                                        <strong>{task.title}</strong>
                                        <small>
                                            {project?.name || 'Unknown space'} - {getTaskStatusLabel(task)}
                                        </small>
                                    </span>
                                </button>
                            );
                        })}
                        {!searchedQuery && (
                            <div className="yq-empty-state">Run a document search to find docs, meetings, and todos.</div>
                        )}
                        {searchedQuery && !taskLoading && taskResults.length === 0 && (
                            <div className="yq-empty-state">No matching documents for "{searchedQuery}".</div>
                        )}
                    </div>
                </section>
            </div>
        </div>
    );
}
