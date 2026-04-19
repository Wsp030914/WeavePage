import * as Y from 'yjs';
import { cleanRealtimeError, createRealtimeURL, realtimeConnectionStatus } from './socket';

const TEXT_NAME = 'content_md';
const LOCAL_ORIGIN = 'local-editor';
const REMOTE_ORIGIN = 'remote-server';
const SEED_ORIGIN = 'initial-seed';

function bytesToBase64(bytes) {
    let binary = '';
    bytes.forEach((byte) => {
        binary += String.fromCharCode(byte);
    });
    return btoa(binary);
}

function base64ToBytes(value) {
    const binary = atob(value || '');
    const bytes = new Uint8Array(binary.length);
    for (let index = 0; index < binary.length; index += 1) {
        bytes[index] = binary.charCodeAt(index);
    }
    return bytes;
}

function websocketURL(taskId, cursor) {
    return createRealtimeURL(`/api/v1/tasks/${taskId}/content/ws`, {
        last_update_id: cursor > 0 ? cursor : undefined,
    });
}

function messageID(prefix, taskId) {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return `${prefix}-${crypto.randomUUID()}`;
    }
    return `${prefix}-${taskId}-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function cleanErrorMessage(message) {
    return cleanRealtimeError(message, '正文协同连接异常，请稍后重试');
}

export class YjsTaskContentProvider {
    constructor({ taskId, initialContent = '', onTextChange, onStatusChange, onError }) {
        this.taskId = taskId;
        this.initialContent = initialContent;
        this.onTextChange = onTextChange;
        this.onStatusChange = onStatusChange;
        this.onError = onError;
        this.cursor = 0;
        this.pendingUpdates = new Map();
        this.outbox = [];
        this.destroyed = false;
        this.seedMessageID = `task-content-seed-${taskId}`;

        this.createDoc();
        this.connect();
    }

    createDoc() {
        this.doc = new Y.Doc();
        this.text = this.doc.getText(TEXT_NAME);
        this.textObserver = () => {
            this.onTextChange?.(this.text.toString());
        };
        this.updateObserver = (update, origin) => {
            if (origin === REMOTE_ORIGIN || origin === SEED_ORIGIN) return;
            this.sendUpdate(update, this.text.toString());
        };
        this.text.observe(this.textObserver);
        this.doc.on('update', this.updateObserver);
    }

    connect() {
        if (this.destroyed) return;
        this.setStatus(realtimeConnectionStatus.CONNECTING);
        this.socket = new WebSocket(websocketURL(this.taskId, this.cursor));

        this.socket.addEventListener('open', () => {
            this.setStatus(realtimeConnectionStatus.CONNECTED);
            this.flushOutbox();
        });

        this.socket.addEventListener('message', (event) => {
            this.handleMessage(event.data);
        });

        this.socket.addEventListener('close', () => {
            if (this.destroyed) return;
            this.setStatus(realtimeConnectionStatus.DISCONNECTED);
        });

        this.socket.addEventListener('error', () => {
            if (this.destroyed) return;
            this.setStatus(realtimeConnectionStatus.ERROR);
            this.onError?.('正文协同连接异常，请稍后重试');
        });
    }

    destroy() {
        this.destroyed = true;
        this.socket?.close();
        this.doc?.off('update', this.updateObserver);
        this.text?.unobserve(this.textObserver);
        this.doc?.destroy();
        this.pendingUpdates.clear();
        this.outbox = [];
    }

    getDoc() {
        return this.doc;
    }

    getText() {
        return this.text;
    }

    setText(nextText) {
        if (!this.text) return;
        const current = this.text.toString();
        if (current === nextText) return;

        this.doc.transact(() => {
            this.text.delete(0, this.text.length);
            if (nextText) {
                this.text.insert(0, nextText);
            }
        }, LOCAL_ORIGIN);
    }

    handleMessage(raw) {
        let message;
        try {
            message = JSON.parse(raw);
        } catch {
            this.reportError('正文协同消息格式错误');
            return;
        }

        switch (message.type) {
            case 'CONTENT_INIT':
                this.applyInitialUpdates(message);
                break;
            case 'CONTENT_UPDATE':
                this.applyRemoteUpdate(message);
                break;
            case 'CONTENT_ACK':
                this.handleAck(message);
                break;
            case 'CONTENT_ERROR':
                this.reportError(message.error);
                break;
            case 'PONG':
                break;
            default:
                break;
        }
    }

    applyInitialUpdates(message) {
        const updates = Array.isArray(message.updates) ? message.updates : [];
        updates.forEach((item) => this.applyRemoteUpdate(item));
        if (typeof message.next_cursor === 'number') {
            this.cursor = Math.max(this.cursor, message.next_cursor);
        }

        if (updates.length === 0 && this.cursor === 0 && this.initialContent) {
            this.seedInitialContent();
        } else {
            this.onTextChange?.(this.text.toString());
        }
    }

    seedInitialContent() {
        this.doc.transact(() => {
            if (this.text.length === 0) {
                this.text.insert(0, this.initialContent);
            }
        }, SEED_ORIGIN);

        const update = Y.encodeStateAsUpdate(this.doc);
        this.sendUpdate(update, this.initialContent, this.seedMessageID);
    }

    applyRemoteUpdate(message) {
        if (!message?.update) return;
        Y.applyUpdate(this.doc, base64ToBytes(message.update), REMOTE_ORIGIN);
        if (typeof message.update_id === 'number') {
            this.cursor = Math.max(this.cursor, message.update_id);
        }
        if (typeof message.id === 'number') {
            this.cursor = Math.max(this.cursor, message.id);
        }
    }

    handleAck(message) {
        if (message.message_id) {
            this.pendingUpdates.delete(message.message_id);
        }
        if (typeof message.update_id === 'number') {
            this.cursor = Math.max(this.cursor, message.update_id);
        }
        if (message.duplicate && message.message_id === this.seedMessageID) {
            this.reloadFromServer();
        }
    }

    reloadFromServer() {
        this.doc.off('update', this.updateObserver);
        this.text.unobserve(this.textObserver);
        this.doc.destroy();
        this.cursor = 0;
        this.createDoc();
        this.send({
            type: 'CONTENT_SYNC',
            last_update_id: 0,
        });
    }

    sendUpdate(update, snapshot, forcedMessageID) {
        const id = forcedMessageID || messageID('content-update', this.taskId);
        this.pendingUpdates.set(id, update);
        this.send({
            type: 'CONTENT_UPDATE',
            message_id: id,
            update: bytesToBase64(update),
            content_snapshot: snapshot,
        });
    }

    send(message) {
        if (this.socket?.readyState !== WebSocket.OPEN) {
            this.outbox.push(message);
            return;
        }
        this.socket.send(JSON.stringify(message));
    }

    flushOutbox() {
        const messages = this.outbox.splice(0);
        messages.forEach((message) => this.send(message));
    }

    setStatus(status) {
        this.onStatusChange?.(status);
    }

    reportError(message) {
        const cleanMessage = cleanErrorMessage(message);
        this.setStatus(realtimeConnectionStatus.ERROR);
        this.onError?.(cleanMessage);
    }
}

export const contentConnectionStatus = {
    CONNECTING: realtimeConnectionStatus.CONNECTING,
    CONNECTED: realtimeConnectionStatus.CONNECTED,
    DISCONNECTED: realtimeConnectionStatus.DISCONNECTED,
    ERROR: realtimeConnectionStatus.ERROR,
};
