import React, { useState } from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { openTodayDiary } from '../api/diary';
import { createMeetingNote } from '../api/meeting';
import useAuth from '../store/useAuth';
import useProjects from '../store/useProjects';
import Avatar from '../components/Avatar';
import Button from '../components/Button';
import './AppLayout.css';

function MainNavItem({ to, label, active, disabled = false, meta = '', onClick, busy = false }) {
    if (disabled) {
        return (
            <span className="yq-main-nav-item disabled" title={meta || 'Coming soon'}>
                <span>{label}</span>
                {meta ? <small>{meta}</small> : null}
            </span>
        );
    }

    if (onClick) {
        return (
            <button type="button" className={`yq-main-nav-item action ${active ? 'active' : ''}`} onClick={onClick} disabled={busy}>
                <span>{label}</span>
                {meta ? <small>{meta}</small> : null}
            </button>
        );
    }

    return (
        <Link to={to} className={`yq-main-nav-item ${active ? 'active' : ''}`}>
            <span>{label}</span>
        </Link>
    );
}

export default function AppLayout() {
    const location = useLocation();
    const navigate = useNavigate();
    const { user, logout } = useAuth();
    const { projects, fetchProjects } = useProjects();
    const [openingDiary, setOpeningDiary] = useState(false);
    const [creatingMeeting, setCreatingMeeting] = useState(false);
    const [diaryError, setDiaryError] = useState('');
    const [meetingError, setMeetingError] = useState('');

    if (!user) return null;

    const inboxProject = projects.find((project) => {
        const name = (project.name || '').trim().toLowerCase();
        return name === 'inbox' || name === '\u6536\u96c6\u7bb1' || name === '\u6536\u4ef6\u7bb1';
    }) || null;
    const inboxPath = inboxProject ? `/projects/${inboxProject.id}` : '/';
    const diaryProject = projects.find((project) => (project.name || '').trim() === '日记') || null;
    const meetingProject = projects.find((project) => (project.name || '').trim() === '会议') || null;
    const diaryPath = diaryProject ? `/projects/${diaryProject.id}` : '';
    const meetingPath = meetingProject ? `/projects/${meetingProject.id}` : '';

    const onLogout = async () => {
        await logout();
        navigate('/login');
    };

    const onOpenTodayDiary = async () => {
        if (openingDiary) return;
        setOpeningDiary(true);
        setDiaryError('');
        try {
            const result = await openTodayDiary();
            if (result?.project?.id && result?.task?.id) {
                await fetchProjects();
                navigate(`/projects/${result.project.id}?task=${result.task.id}`);
            }
        } catch (err) {
            setDiaryError(err.message || '打开日记失败');
        } finally {
            setOpeningDiary(false);
        }
    };

    const onCreateMeeting = async () => {
        if (creatingMeeting) return;
        setCreatingMeeting(true);
        setMeetingError('');
        try {
            const result = await createMeetingNote();
            if (result?.project?.id && result?.task?.id) {
                await fetchProjects();
                navigate(`/projects/${result.project.id}?task=${result.task.id}`);
            }
        } catch (err) {
            setMeetingError(err.message || '创建会议纪要失败');
        } finally {
            setCreatingMeeting(false);
        }
    };

    return (
        <div className="yq-layout">
            <aside className="yq-sidebar">
                <div className="yq-sidebar-header" onClick={() => navigate('/profile')}>
                    <Avatar src={user.avatar_url} alt={user.username} size={38} />
                    <div className="yq-sidebar-user-meta">
                        <strong>{user.username}</strong>
                        <span>Personal Workspace</span>
                    </div>
                </div>

                <div className="yq-sidebar-section yq-primary-nav">
                    <MainNavItem to="/" label="空间" active={location.pathname === '/'} />
                    <MainNavItem
                        label="日记"
                        active={Boolean(diaryPath && location.pathname === diaryPath)}
                        meta={openingDiary ? '打开中' : '今天'}
                        onClick={onOpenTodayDiary}
                        busy={openingDiary}
                    />
                    <MainNavItem
                        label="会议"
                        active={Boolean(meetingPath && location.pathname === meetingPath)}
                        meta={creatingMeeting ? '创建中' : '新建'}
                        onClick={onCreateMeeting}
                        busy={creatingMeeting}
                    />
                    <MainNavItem disabled label="搜索" meta="规划中" />
                    {diaryError ? <div className="yq-sidebar-inline-error">{diaryError}</div> : null}
                    {meetingError ? <div className="yq-sidebar-inline-error">{meetingError}</div> : null}
                </div>

                <div className="yq-sidebar-section">
                    <div className="yq-list-header">
                        <span>待办</span>
                    </div>
                    <MainNavItem to="/tasks/me" label="今日" active={location.pathname === '/tasks/me'} />
                    <MainNavItem to="/tasks/next7" label="未来 7 天" active={location.pathname === '/tasks/next7'} />
                    <MainNavItem to="/calendar" label="日历" active={location.pathname === '/calendar'} />
                    <MainNavItem to={inboxPath} label="收件箱" active={inboxProject ? location.pathname === inboxPath : false} />
                </div>

                <div className="yq-sidebar-section yq-list-section">
                    <div className="yq-list-header">
                        <span>空间列表</span>
                        <Button variant="secondary" className="yq-mini-btn" onClick={() => navigate('/')}>管理</Button>
                    </div>
                    <div className="yq-project-links">
                        {projects.map((project) => {
                            const to = `/projects/${project.id}`;
                            const active = location.pathname === to;
                            return (
                                <Link key={project.id} to={to} className={`yq-project-link ${active ? 'active' : ''}`}>
                                    <span className="yq-project-dot" style={{ backgroundColor: project.color }} />
                                    <span className="yq-project-name">{project.name}</span>
                                </Link>
                            );
                        })}
                    </div>
                </div>

                <div className="yq-sidebar-footer">
                    <Button variant="secondary" onClick={() => navigate('/profile')}>个人资料</Button>
                    <Button variant="danger" onClick={onLogout}>退出</Button>
                </div>
            </aside>

            <main className="yq-main">
                <header className="yq-header">
                    <div className="yq-header-title">
                        <strong>协同文档工作台</strong>
                    </div>
                    <div className="yq-header-actions">
                        <Link to="/profile" className="yq-avatar-link">
                            <Avatar src={user.avatar_url} alt={user.username} size={32} />
                        </Link>
                    </div>
                </header>
                <section className="yq-content-wrapper">
                    <Outlet />
                </section>
            </main>
        </div>
    );
}
