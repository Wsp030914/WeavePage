import client from './client';

export const login = async (username, password) => {
    return client.post('/login', { username, password });
};

export const register = async (formData) => {
    // formData should be instance of FormData (multipart/form-data)
    return client.post('/register', formData, {
        headers: {
            'Content-Type': 'multipart/form-data'
        }
    });
};

export const getProfile = async () => {
    return client.get('/users/me');
};

export const updateProfile = async (formData) => {
    return client.patch('/users/me', formData, {
        headers: {
            'Content-Type': 'multipart/form-data'
        }
    });
};

export const logout = async () => {
    return client.post('/logout');
};
