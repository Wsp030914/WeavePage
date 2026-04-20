import React, { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import Button from '../components/Button';
import Input from '../components/Input';
import Modal from '../components/Modal';
import useProjects from '../store/useProjects';
import './ProjectListPage.css';

export default function ProjectListPage() {
    const navigate = useNavigate();
    const { projects, loading, error, addProject, editProject, removeProject } = useProjects();

    const [search, setSearch] = useState('');
    const [creating, setCreating] = useState(false);
    const [isModalOpen, setIsModalOpen] = useState(false);
    const [form, setForm] = useState({ name: '', color: '#e0b882' });
    const [editingProjectID, setEditingProjectID] = useState(null);
    const [renameValue, setRenameValue] = useState('');
    const [renameSaving, setRenameSaving] = useState(false);
    const [renameError, setRenameError] = useState('');

    const filtered = useMemo(() => {
        const keyword = search.trim().toLowerCase();
        if (!keyword) return projects;
        return projects.filter((project) => project.name.toLowerCase().includes(keyword));
    }, [projects, search]);

    const diaryProject = useMemo(
        () => projects.find((project) => (project.name || '').trim() === '日记') || null,
        [projects],
    );

    const meetingProject = useMemo(
        () => projects.find((project) => (project.name || '').trim() === '会议') || null,
        [projects],
    );

    const inboxProject = useMemo(() => (
        projects.find((project) => {
            const name = (project.name || '').trim().toLowerCase();
            return name === 'inbox' || name === '收件箱' || name === '收集箱';
        }) || null
    ), [projects]);

    const onCreateProject = async () => {
        if (!form.name.trim() || creating) return;
        setCreating(true);
        try {
            await addProject(form.name.trim(), form.color);
            setForm({ name: '', color: '#e0b882' });
            setIsModalOpen(false);
        } catch (err) {
            alert(err.message || 'Failed to create space');
        } finally {
            setCreating(false);
        }
    };

    const onStartRenameProject = (project) => {
        if (!project) return;
        setEditingProjectID(project.id);
        setRenameValue(project.name || '');
        setRenameSaving(false);
        setRenameError('');
    };

    const onCancelRenameProject = () => {
        setEditingProjectID(null);
        setRenameValue('');
        setRenameSaving(false);
        setRenameError('');
    };

    const onSubmitRenameProject = async (project) => {
        if (!project || renameSaving) return;
        const nextName = renameValue.trim();
        if (!nextName) {
            setRenameError('Space name is required');
            return;
        }
        if (nextName === project.name) {
            onCancelRenameProject();
            return;
        }
        setRenameSaving(true);
        setRenameError('');
        try {
            await editProject(project.id, { name: nextName });
            onCancelRenameProject();
        } catch (err) {
            setRenameError(err.message || 'Failed to rename space');
        } finally {
            setRenameSaving(false);
        }
    };

    const onDeleteProject = async (project) => {
        if (!window.confirm(`Move space "${project.name}" to trash? You can restore it from Trash.`)) return;
        try {
            await removeProject(project.id);
        } catch (err) {
            alert(err.message || 'Failed to delete space');
        }
    };

    const colors = ['#e0b882', '#4f86f7', '#61c78e', '#dd5a5a', '#b67cf4', '#f49d50'];

    return (
        <div className="yq-page-container yq-list-page">
            <div className="yq-page-header">
                <div className="yq-space-hero-copy">
                    <span className="yq-kicker">Spaces</span>
                    <h1>Workspace Spaces</h1>
                    <p>
                        Organize collaborative docs, private drafts, daily notes, meeting notes, and lightweight todos
                        around clear space boundaries.
                    </p>
                </div>
                <Button onClick={() => setIsModalOpen(true)}>New Space</Button>
            </div>

            <div className="yq-space-summary-grid">
                <article className="yq-space-summary-card">
                    <span className="yq-space-summary-label">Total Spaces</span>
                    <strong>{projects.length}</strong>
                    <p>Shared working areas for docs, meetings, and focused planning.</p>
                </article>
                <article className="yq-space-summary-card">
                    <span className="yq-space-summary-label">Daily Notes</span>
                    <strong>{diaryProject ? 'Ready' : 'Not started'}</strong>
                    <p>{diaryProject ? `Stored in ${diaryProject.name}` : 'Open the Daily Notes entry in the sidebar to create today\'s note.'}</p>
                </article>
                <article className="yq-space-summary-card">
                    <span className="yq-space-summary-label">Meetings</span>
                    <strong>{meetingProject ? 'Ready' : 'Not started'}</strong>
                    <p>{meetingProject ? `Meeting notes are grouped in ${meetingProject.name}.` : 'Use the Meetings entry to create a collaborative meeting note.'}</p>
                </article>
                <article className="yq-space-summary-card">
                    <span className="yq-space-summary-label">Capture</span>
                    <strong>{inboxProject ? 'Inbox ready' : 'Create a lane'}</strong>
                    <p>{inboxProject ? 'Inbox can absorb quick documents and action items before they are sorted.' : 'Create an Inbox-style space when you want a quick capture lane.'}</p>
                </article>
            </div>

            <div className="yq-list-toolbar">
                <Input
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder="Search spaces"
                    style={{ marginBottom: 0 }}
                />
            </div>

            {loading && <div>Loading spaces...</div>}
            {!loading && error && <div className="yq-error">{error}</div>}
            {!loading && !error && (
                <div className="yq-project-grid">
                    {filtered.map((project) => (
                        <article key={project.id} className="yq-project-card">
                            {editingProjectID === project.id ? (
                                <div className="yq-project-open yq-project-open-editing">
                                    <span className="yq-project-dot" style={{ backgroundColor: project.color }} />
                                    <input
                                        className="yq-project-inline-input"
                                        value={renameValue}
                                        maxLength={64}
                                        autoFocus
                                        onChange={(event) => {
                                            setRenameValue(event.target.value);
                                            if (renameError) setRenameError('');
                                        }}
                                        onKeyDown={(event) => {
                                            if (event.key === 'Enter') {
                                                event.preventDefault();
                                                onSubmitRenameProject(project);
                                            }
                                            if (event.key === 'Escape') {
                                                event.preventDefault();
                                                onCancelRenameProject();
                                            }
                                        }}
                                    />
                                </div>
                            ) : (
                                <button
                                    type="button"
                                    className="yq-project-open"
                                    onClick={() => navigate(`/projects/${project.id}`)}
                                >
                                    <span className="yq-project-dot" style={{ backgroundColor: project.color }} />
                                    <div className="yq-project-card-main">
                                        <strong>{project.name}</strong>
                                        <span className="yq-project-card-meta">
                                            {project.id === diaryProject?.id
                                                ? 'Daily Notes'
                                                : project.id === meetingProject?.id
                                                    ? 'Meetings'
                                                    : project.id === inboxProject?.id
                                                        ? 'Inbox'
                                                        : 'Space'}
                                        </span>
                                    </div>
                                </button>
                            )}
                            {editingProjectID === project.id && renameError && (
                                <div className="yq-inline-error">{renameError}</div>
                            )}
                            <div className="yq-project-actions">
                                {editingProjectID === project.id ? (
                                    <>
                                        <Button variant="secondary" onClick={onCancelRenameProject} disabled={renameSaving}>Cancel</Button>
                                        <Button onClick={() => onSubmitRenameProject(project)} disabled={renameSaving || !renameValue.trim()}>
                                            {renameSaving ? 'Saving...' : 'Save'}
                                        </Button>
                                    </>
                                ) : (
                                    <>
                                        <Button variant="secondary" onClick={() => onStartRenameProject(project)}>Rename</Button>
                                        <Button variant="danger" onClick={() => onDeleteProject(project)}>Delete</Button>
                                    </>
                                )}
                            </div>
                        </article>
                    ))}

                    {filtered.length === 0 && (
                        <div className="yq-empty-state">
                            No spaces found. Create one to organize docs, meetings, and lightweight todos.
                        </div>
                    )}
                </div>
            )}

            <Modal
                isOpen={isModalOpen}
                onClose={() => setIsModalOpen(false)}
                title="Create New Space"
                footer={
                    <>
                        <Button variant="secondary" onClick={() => setIsModalOpen(false)}>Cancel</Button>
                        <Button onClick={onCreateProject} disabled={creating || !form.name.trim()}>
                            {creating ? 'Creating...' : 'Create'}
                        </Button>
                    </>
                }
            >
                <Input
                    label="Space name"
                    value={form.name}
                    onChange={(event) => setForm((prev) => ({ ...prev, name: event.target.value }))}
                    autoFocus
                />
                <div className="yq-color-picker">
                    <label className="yq-input-label">Space color</label>
                    <div className="yq-color-options">
                        {colors.map((color) => (
                            <button
                                key={color}
                                type="button"
                                className={`yq-color-option ${form.color === color ? 'selected' : ''}`}
                                style={{ backgroundColor: color }}
                                onClick={() => setForm((prev) => ({ ...prev, color }))}
                                aria-label={`Pick ${color}`}
                            />
                        ))}
                    </div>
                </div>
            </Modal>
        </div>
    );
}


