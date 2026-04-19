import React from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import useAuth from '../store/useAuth';
import useProjects from '../store/useProjects';
import Avatar from '../components/Avatar';
import Button from '../components/Button';
import './AppLayout.css';

function MainNavItem({ to, label, active, disabled = false, meta = '' }) {
    if (disabled) {
        return (
            <span className="yq-main-nav-item disabled" title={meta || 'Coming soon'}>
                <span>{label}</span>
                {meta ? <small>{meta}</small> : null}
            </span>
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
    const { projects } = useProjects();

    if (!user) return null;

    const inboxProject = projects.find((project) => {
        const name = (project.name || '').trim().toLowerCase();
        return name === 'inbox' || name === '\u6536\u96c6\u7bb1' || name === '\u6536\u4ef6\u7bb1';
    }) || null;
    const inboxPath = inboxProject ? `/projects/${inboxProject.id}` : '/';

    const onLogout = async () => {
        await logout();
        navigate('/login');
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
                    <MainNavItem disabled label="日记" meta="规划中" />
                    <MainNavItem disabled label="会议" meta="规划中" />
                    <MainNavItem disabled label="搜索" meta="规划中" />
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
