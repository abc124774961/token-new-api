/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import { getUserIdFromLocalStorage } from '../../helpers';

export const REALTIME_STATES = {
  CONNECTING: 'connecting',
  CONNECTED: 'connected',
  RECONNECTING: 'reconnecting',
  DISCONNECTED: 'disconnected',
  FALLBACK: 'fallback',
};

const DEFAULT_RECONNECT_DELAYS = [1000, 2000, 5000, 10000, 15000];
const DEFAULT_PING_INTERVAL = 25000;

export class RealtimeClient {
  constructor(options = {}) {
    this.path = options.path || '/api/realtime/ws';
    this.reconnectDelays = options.reconnectDelays || DEFAULT_RECONNECT_DELAYS;
    this.pingInterval = options.pingInterval || DEFAULT_PING_INTERVAL;
    this.subscriptions = new Map();
    this.listeners = new Set();
    this.socket = null;
    this.state = REALTIME_STATES.DISCONNECTED;
    this.sequence = 0;
    this.reconnectAttempt = 0;
    this.reconnectTimer = null;
    this.pingTimer = null;
    this.manuallyClosed = false;
  }

  subscribe(topic, params, handlers = {}) {
    const id =
      handlers.id ||
      `${topic}:${Date.now()}:${Math.random().toString(36).slice(2)}`;
    this.subscriptions.set(id, {
      id,
      topic,
      params: params || {},
      handlers,
      lastSequence: 0,
    });
    this.connect();
    this.sendSubscribe(id);
    return () => this.unsubscribe(id);
  }

  onStateChange(listener) {
    if (typeof listener !== 'function') return () => {};
    this.listeners.add(listener);
    listener(this.state);
    return () => this.listeners.delete(listener);
  }

  connect() {
    if (
      this.socket &&
      (this.socket.readyState === WebSocket.OPEN ||
        this.socket.readyState === WebSocket.CONNECTING)
    ) {
      return;
    }
    this.manuallyClosed = false;
    this.setState(
      this.reconnectAttempt > 0
        ? REALTIME_STATES.RECONNECTING
        : REALTIME_STATES.CONNECTING,
    );
    try {
      this.socket = new WebSocket(this.buildUrl());
    } catch (error) {
      this.scheduleReconnect(error);
      return;
    }
    const socket = this.socket;
    socket.onopen = () => this.handleOpen(socket);
    socket.onmessage = (event) => this.handleMessage(event);
    socket.onerror = (error) => this.handleDisconnect(socket, error);
    socket.onclose = () => this.handleDisconnect(socket);
  }

  close() {
    this.manuallyClosed = true;
    this.clearTimers();
    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }
    this.setState(REALTIME_STATES.DISCONNECTED);
  }

  unsubscribe(id) {
    const subscription = this.subscriptions.get(id);
    if (!subscription) return;
    this.send({
      type: 'unsubscribe',
      id,
      topic: subscription.topic,
    });
    this.subscriptions.delete(id);
    if (this.subscriptions.size === 0) {
      this.close();
    }
  }

  sendSubscribe(id) {
    const subscription = this.subscriptions.get(id);
    if (!subscription || !this.isOpen()) return;
    this.send({
      type: 'subscribe',
      id,
      topic: subscription.topic,
      params: subscription.params,
    });
  }

  send(payload) {
    if (!this.isOpen()) return false;
    this.socket.send(JSON.stringify(payload));
    return true;
  }

  isOpen() {
    return this.socket?.readyState === WebSocket.OPEN;
  }

  handleOpen(socket) {
    if (socket && socket !== this.socket) return;
    this.reconnectAttempt = 0;
    this.setState(REALTIME_STATES.CONNECTED);
    this.startPing();
    this.subscriptions.forEach((_, id) => this.sendSubscribe(id));
  }

  handleMessage(event) {
    let message;
    try {
      message = JSON.parse(event.data);
    } catch (error) {
      return;
    }
    if (message?.type === 'pong') return;
    const subscription = this.subscriptions.get(message?.id);
    if (!subscription) return;
    const sequence = Number(message.sequence || 0);
    if (sequence && sequence <= subscription.lastSequence) return;
    if (sequence) subscription.lastSequence = sequence;

    if (message.type === 'snapshot') {
      subscription.handlers.onSnapshot?.(message.data, message);
      return;
    }
    if (message.type === 'delta') {
      subscription.handlers.onDelta?.(message.data, message);
      return;
    }
    if (message.type === 'status') {
      subscription.handlers.onStatus?.(message.data, message);
      return;
    }
    if (message.type === 'error') {
      subscription.handlers.onError?.(message.message, message);
    }
  }

  handleDisconnect(socket, error) {
    if (socket && socket !== this.socket) return;
    if (this.socket === socket) {
      this.socket = null;
    }
    this.clearPing();
    if (this.manuallyClosed) return;
    this.scheduleReconnect(error);
  }

  scheduleReconnect(error) {
    if (this.manuallyClosed) return;
    this.setState(REALTIME_STATES.RECONNECTING);
    this.subscriptions.forEach((subscription) => {
      subscription.handlers.onDisconnect?.(error);
    });
    window.clearTimeout(this.reconnectTimer);
    const delay =
      this.reconnectDelays[
        Math.min(this.reconnectAttempt, this.reconnectDelays.length - 1)
      ];
    this.reconnectAttempt += 1;
    this.reconnectTimer = window.setTimeout(() => this.connect(), delay);
  }

  startPing() {
    this.clearPing();
    this.pingTimer = window.setInterval(() => {
      this.send({ type: 'ping', id: `ping-${Date.now()}` });
    }, this.pingInterval);
  }

  clearPing() {
    window.clearInterval(this.pingTimer);
    this.pingTimer = null;
  }

  clearTimers() {
    window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
    this.clearPing();
  }

  setState(state) {
    if (this.state === state) return;
    this.state = state;
    this.listeners.forEach((listener) => listener(state));
  }

  buildUrl() {
    const base = import.meta.env.VITE_REACT_APP_SERVER_URL || window.location.origin;
    const url = new URL(this.path, base);
    url.searchParams.set('user_id', String(getUserIdFromLocalStorage()));
    if (url.protocol === 'http:') url.protocol = 'ws:';
    if (url.protocol === 'https:') url.protocol = 'wss:';
    return url.toString();
  }
}

export const realtimeClient = new RealtimeClient();
