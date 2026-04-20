import client from './client';

export const searchWorkspace = async (query, limit = 50) => {
    return client.get('/search', {
        params: { q: query, limit },
    });
};
