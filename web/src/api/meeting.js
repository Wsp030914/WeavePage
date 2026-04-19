import client from './client';

export const createMeetingNote = async (data = {}) => {
    return client.post('/meetings', data);
};
