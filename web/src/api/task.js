import client from './client';

export const createTask = async (data) => {
    return client.post('/tasks', data);
};

export const getTasks = async (params) => {
    return client.get('/tasks', { params });
};

export const syncProjectEvents = async (projectId, cursor = 0, limit = 100) => {
    return client.get(`/projects/${projectId}/sync`, {
        params: { cursor, limit },
    });
};

export const getMyTasks = async (params) => {
    return client.get('/tasks/me', { params });
};

export const getAllProjectTasks = async (projectId, pageSize = 100) => {
    const size = pageSize > 0 && pageSize <= 100 ? pageSize : 100;
    let page = 1;
    const all = [];

    while (true) {
        const data = await getTasks({ project_id: projectId, page, size });
        const list = Array.isArray(data.list) ? data.list : [];
        all.push(...list);

        const total = Number(data.total) || 0;
        if (list.length < size) break;
        if (total > 0 && all.length >= total) break;
        page += 1;
        if (page > 50) break;
    }

    return all;
};

export const getTasksAcrossProjects = async (projectIds, pageSize = 100) => {
    const ids = Array.from(
        new Set(
            (projectIds || [])
                .map((id) => Number(id))
                .filter((id) => Number.isFinite(id) && id > 0),
        ),
    );
    if (ids.length === 0) return [];

    const grouped = await Promise.all(ids.map((id) => getAllProjectTasks(id, pageSize)));
    const merged = new Map();
    grouped.flat().forEach((task) => {
        if (!task || typeof task.id === 'undefined') return;
        merged.set(task.id, task);
    });
    return Array.from(merged.values());
};

export const getTaskById = async (id) => {
    return client.get(`/tasks/${id}`);
};

export const updateTask = async (projectId, taskId, data, expectedVersion) => {
    const payload = typeof expectedVersion === 'number'
        ? { ...data, expected_version: expectedVersion }
        : data;

    return client.patch(`/projects/${projectId}/tasks/${taskId}`, payload);
};

export const deleteTask = async (id) => {
    return client.delete(`/tasks/${id}`);
};

export const addTaskMember = async (projectId, taskId, data) => {
    return client.post(`/projects/${projectId}/tasks/${taskId}/members`, data);
};

export const removeTaskMember = async (projectId, taskId, data) => {
    return client.delete(`/projects/${projectId}/tasks/${taskId}/members`, { data });
};
