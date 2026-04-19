export const realtimeConnectionStatus = {
    CONNECTING: 'connecting',
    CONNECTED: 'connected',
    DISCONNECTED: 'disconnected',
    ERROR: 'error',
};

export function tokenFromStorage() {
    return localStorage.getItem('access_token') || '';
}

export function createRealtimeURL(path, params = {}) {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const search = new URLSearchParams();
    const token = tokenFromStorage();
    if (token) search.set('token', token);

    Object.entries(params).forEach(([key, value]) => {
        if (value === undefined || value === null || value === '') return;
        search.set(key, String(value));
    });

    const query = search.toString();
    return `${protocol}//${window.location.host}${path}${query ? `?${query}` : ''}`;
}

export function cleanRealtimeError(message, fallback = '实时连接异常，请稍后重试') {
    const text = String(message || '').trim();
    return text || fallback;
}
