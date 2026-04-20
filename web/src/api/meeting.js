import client from './client';

export const createMeetingNote = async (data = {}) => {
    return client.post('/meetings', data);
};

export const createMeetingActionTodo = async (meetingId, data) => {
    return client.post(`/meetings/${meetingId}/actions`, data);
};
