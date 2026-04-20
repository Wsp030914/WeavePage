import client from './client';

export const createProject = async (data) => {
    return client.post('/projects', data);
};

export const getProjects = async (params) => {
    return client.get('/projects', { params });
};

export const getProjectById = async (id) => {
    return client.get(`/projects/${id}`);
};

export const updateProject = async (id, data) => {
    return client.patch(`/projects/${id}`, data);
};

export const deleteProject = async (id) => {
    return client.delete(`/projects/${id}`);
};

export const getTrashSpaces = async (params) => {
    return client.get('/trash/spaces', { params });
};

export const restoreTrashSpace = async (id) => {
    return client.post(`/trash/spaces/${id}/restore`);
};

export const deleteTrashedSpace = async (id) => {
    return client.delete(`/trash/spaces/${id}`);
};
