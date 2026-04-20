import client from './client';

export const openTodayDiary = async () => {
    return client.post('/diary/today');
};

export const openDiaryDate = async (date) => {
    return client.post(`/diary/${date}`);
};
