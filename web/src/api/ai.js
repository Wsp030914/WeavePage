import client from './client';

function authHeaders() {
    const token = localStorage.getItem('access_token');
    if (!token) return {};
    return { Authorization: `Bearer ${token}` };
}

async function readErrorMessage(response) {
    try {
        const data = await response.json();
        return data?.message || 'AI request failed';
    } catch {
        return 'AI request failed';
    }
}

function handleUnauthorized() {
    localStorage.removeItem('access_token');
    if (window.location.pathname !== '/login' && window.location.pathname !== '/register') {
        window.location.href = '/login';
    }
}

async function streamText(path, payload, options = {}) {
    const { signal, onChunk } = options;
    const response = await fetch(`/api/v1${path}`, {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            Accept: 'text/plain',
            ...authHeaders(),
        },
        body: JSON.stringify(payload),
        signal,
    });

    if (!response.ok) {
        if (response.status === 401) {
            handleUnauthorized();
            throw new Error('当前账号无权限访问或登录已过期');
        }
        if (response.status >= 500) {
            const dateStr = new Date().toISOString().replace(/[-:T.]/g, '').slice(0, 14);
            throw new Error(`AI 服务暂时不可用。参考码：E${dateStr}`);
        }
        throw new Error(await readErrorMessage(response));
    }

    if (!response.body) {
        return '';
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let output = '';

    while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        const chunk = decoder.decode(value, { stream: true });
        if (!chunk) continue;
        output += chunk;
        onChunk?.(chunk);
    }

    const tail = decoder.decode();
    if (tail) {
        output += tail;
        onChunk?.(tail);
    }

    return output;
}

export function streamDraftPreview(payload, options) {
    return streamText('/ai/draft/stream', payload, options);
}

export function streamContinuePreview(payload, options) {
    return streamText('/ai/continue/stream', payload, options);
}

export function generateMeetingPreview(payload) {
    return client.post('/ai/meetings/generate', payload);
}
