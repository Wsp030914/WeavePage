import React, { useState, useEffect, useCallback } from 'react';
import { getProjects, createProject, deleteProject, updateProject } from '../api/project';
import useAuth from './useAuth';
import { ProjectContext } from './project-context';

export const ProjectProvider = ({ children }) => {
    const { user } = useAuth();
    const [projects, setProjects] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    const fetchProjects = useCallback(async (searchQuery = '') => {
        if (!user) return;
        setLoading(true);
        try {
            const data = await getProjects({ size: 100, name: searchQuery });
            // The API returns { list: [...], total: ... }
            setProjects(data.list || []);
            setError(null);
        } catch (err) {
            setError(err.message);
        } finally {
            setLoading(false);
        }
    }, [user]);

    useEffect(() => {
        fetchProjects();
    }, [fetchProjects]);

    const addProject = async (name, color) => {
        const newProj = await createProject({ name, color });
        setProjects(prev => [newProj, ...prev]);
        return newProj;
    };

    const editProject = async (id, data) => {
        await updateProject(id, data);
        setProjects(prev => prev.map(p => p.id === id ? { ...p, ...data } : p));
    };

    const removeProject = async (id) => {
        await deleteProject(id);
        setProjects(prev => prev.filter(p => p.id !== id));
    };

    return (
        <ProjectContext.Provider value={{
            projects,
            loading,
            error,
            fetchProjects,
            addProject,
            editProject,
            removeProject
        }}>
            {children}
        </ProjectContext.Provider>
    );
};
