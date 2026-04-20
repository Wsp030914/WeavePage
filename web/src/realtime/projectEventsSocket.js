import { projectMessageTypes, isProjectTaskMessage } from './protocol';
import { cleanRealtimeError, createRealtimeURL, realtimeConnectionStatus } from './socket';

const RECONNECT_DELAY_MS = 1200;

function projectEventsURL(projectId, cursor) {
    return createRealtimeURL(`/api/v1/projects/${projectId}/ws`, {
        cursor: cursor > 0 ? cursor : undefined,
    });
}

export class ProjectEventsSocket {
    constructor({
        projectId,
        cursor = 0,
        onInit,
        onEvent,
        onPresence,
        onLock,
        onCursorChange,
        onStatusChange,
        onError,
    }) {
        this.projectId = projectId;
        this.cursor = cursor;
        this.onInit = onInit;
        this.onEvent = onEvent;
        this.onPresence = onPresence;
        this.onLock = onLock;
        this.onCursorChange = onCursorChange;
        this.onStatusChange = onStatusChange;
        this.onError = onError;
        this.destroyed = false;
        this.reconnectTimer = null;

        this.connect();
    }

    connect() {
        if (this.destroyed) return;
        this.setStatus(realtimeConnectionStatus.CONNECTING);
        this.socket = new WebSocket(projectEventsURL(this.projectId, this.cursor));

        this.socket.addEventListener('open', () => {
            this.setStatus(realtimeConnectionStatus.CONNECTED);
        });

        this.socket.addEventListener('message', (event) => {
            this.handleMessage(event.data);
        });

        this.socket.addEventListener('close', () => {
            if (this.destroyed) return;
            this.setStatus(realtimeConnectionStatus.DISCONNECTED);
            this.scheduleReconnect();
        });

        this.socket.addEventListener('error', () => {
            if (this.destroyed) return;
            this.setStatus(realtimeConnectionStatus.ERROR);
            this.reportError('项目实时连接异常，正在尝试重连');
        });
    }

    destroy() {
        this.destroyed = true;
        if (this.reconnectTimer) {
            clearTimeout(this.reconnectTimer);
        }
        this.socket?.close();
    }

    handleMessage(raw) {
        let message;
        try {
            message = JSON.parse(raw);
        } catch {
            this.reportError('项目实时消息格式错误');
            return;
        }

        switch (message.type) {
            case projectMessageTypes.INIT:
                this.applyInit(message);
                break;
            case projectMessageTypes.ERROR:
                this.reportError(message.error);
                break;
            case projectMessageTypes.PRESENCE_SNAPSHOT:
                this.onPresence?.(message.server_node_id || 'default', Array.isArray(message.presence) ? message.presence : []);
                break;
            case projectMessageTypes.TASK_LOCKED:
            case projectMessageTypes.TASK_UNLOCKED:
            case projectMessageTypes.LOCK_ERROR:
                this.onLock?.(message);
                break;
            case projectMessageTypes.PONG:
                break;
            default:
                if (isProjectTaskMessage(message.type)) {
                    this.applyTaskEvent(message);
                }
                break;
        }
    }

    applyInit(message) {
        const events = Array.isArray(message.events) ? message.events : [];
        this.onInit?.(events);
        this.updateCursor(message.next_cursor);

        if (message.has_more) {
            this.send({
                type: projectMessageTypes.SYNC,
                cursor: this.cursor,
            });
        }
    }

    applyTaskEvent(message) {
        if (!message.event) return;
        this.onEvent?.(message.event);
        this.updateCursor(message.cursor || message.event.id);
    }

    updateCursor(cursor) {
        if (typeof cursor !== 'number') return;
        const nextCursor = Math.max(this.cursor, cursor);
        if (nextCursor === this.cursor) return;
        this.cursor = nextCursor;
        this.onCursorChange?.(nextCursor);
    }

    send(message) {
        if (this.socket?.readyState !== WebSocket.OPEN) return false;
        this.socket.send(JSON.stringify(message));
        return true;
    }

    requestLock(taskId, field = 'metadata') {
        return this.send({
            type: projectMessageTypes.LOCK_REQUEST,
            task_id: taskId,
            field,
        });
    }

    releaseLock(taskId, field = 'metadata') {
        return this.send({
            type: projectMessageTypes.LOCK_RELEASE,
            task_id: taskId,
            field,
        });
    }

    viewDocument(taskId = 0) {
        return this.send({
            type: projectMessageTypes.VIEW_DOCUMENT,
            task_id: taskId,
        });
    }

    scheduleReconnect() {
        if (this.destroyed || this.reconnectTimer) return;
        this.reconnectTimer = window.setTimeout(() => {
            this.reconnectTimer = null;
            this.connect();
        }, RECONNECT_DELAY_MS);
    }

    setStatus(status) {
        this.onStatusChange?.(status);
    }

    reportError(message) {
        this.onError?.(cleanRealtimeError(message, '项目实时连接异常，请稍后重试'));
    }
}

export const projectConnectionStatus = realtimeConnectionStatus;
