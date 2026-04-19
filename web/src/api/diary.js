import client from './client';

export const openTodayDiary = async () => {
    return client.post('/diary/today');
};
