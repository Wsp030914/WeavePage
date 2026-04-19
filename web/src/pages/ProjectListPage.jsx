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
        if (!window.confirm(`Delete space "${project.name}"?`)) return;
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
                <div>
                    <span className="yq-kicker">Workspace</span>
                    <h1>空间</h1>
                </div>
                <Button onClick={() => setIsModalOpen(true)}>新建空间</Button>
            </div>

            <div className="yq-list-toolbar">
                <Input
                    value={search}
                    onChange={(event) => setSearch(event.target.value)}
                    placeholder="搜索空间"
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
                                    <strong>{project.name}</strong>
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
                            还没有空间。先创建一个空间，用它组织文档、会议和轻量待办。
                        </div>
                    )}
                </div>
            )}

            <Modal
                isOpen={isModalOpen}
                onClose={() => setIsModalOpen(false)}
                title="创建新空间"
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
                    label="空间名称"
                    value={form.name}
                    onChange={(event) => setForm((prev) => ({ ...prev, name: event.target.value }))}
                    autoFocus
                />
                <div className="yq-color-picker">
                    <label className="yq-input-label">空间颜色</label>
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


