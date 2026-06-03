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

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { API } from '../../helpers/api';
import { showError } from '../../helpers';
import { REALTIME_STATES } from '../../services/realtime/RealtimeClient';
import { useRealtimeSubscription } from '../common/useRealtimeSubscription';

const FALLBACK_REFRESH_SECONDS = 30;
const RECENT_RECORD_LIMIT = 50;
const MANUAL_REFRESH_SOURCE = 'manual';
const FALLBACK_REFRESH_SOURCE = 'fallback';
const RECENT_USER_REQUEST_LIMIT = 50;
const MANUAL_REFRESH_DEBOUNCE_MS = 800;

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function mergeDelta(current, delta) {
  if (!current || !delta) return current;
  const recentRecords = Array.isArray(delta.recent_records)
    ? delta.recent_records
    : [];
  const seen = new Set();
  const mergedRecent =
    recentRecords.length > 0
      ? [...recentRecords, ...(current.recent_records || [])]
          .filter((record) => {
            const key = `${record.id || ''}:${record.request_id || ''}:${record.attempt_index || 0}:${record.created_at || ''}`;
            if (seen.has(key)) return false;
            seen.add(key);
            return true;
          })
          .slice(0, RECENT_RECORD_LIMIT)
      : current.recent_records;
  return {
    ...current,
    recent_records: mergedRecent,
    user_requests: mergeUserRequestDelta(
      current.user_requests,
      delta.user_requests_recent,
    ),
  };
}

function isUserRequestProcessingRecord(record) {
  return (
    String(record?.status || '').trim() === 'processing' &&
    !Number(record?.completed_at || 0)
  );
}

function isUserRequestTerminalRecord(record) {
  if (!record) return false;
  if (Number(record?.completed_at || 0) > 0) return true;
  const status = String(record?.status || '').trim();
  return [
    'success',
    'failed',
    'health_probe',
    'health_probe_failed',
    'client_aborted',
    'user_quota_exhausted',
  ].includes(status);
}

function userRequestDisplaySortTime(record) {
  if (isUserRequestProcessingRecord(record)) {
    return Number(record?.created_at || 0);
  }
  return Number(record?.completed_at || record?.created_at || 0);
}

function compareUserRequestRecordsForDisplay(left, right) {
  const leftProcessing = isUserRequestProcessingRecord(left);
  const rightProcessing = isUserRequestProcessingRecord(right);
  if (leftProcessing !== rightProcessing) {
    return leftProcessing ? -1 : 1;
  }
  const timeDiff =
    userRequestDisplaySortTime(right) - userRequestDisplaySortTime(left);
  if (timeDiff !== 0) return timeDiff;
  const idDiff = Number(right?.id || 0) - Number(left?.id || 0);
  if (idDiff !== 0) return idDiff;
  return String(right?.request_id || '').localeCompare(
    String(left?.request_id || ''),
  );
}

function userRequestKey(record) {
  return (
    record?.request_id || `${record?.id || ''}:${record?.created_at || ''}`
  );
}

function mergeUserRequestRecord(existing, incoming) {
  if (!existing) return incoming;
  if (!incoming) return existing;
  const existingTerminal = isUserRequestTerminalRecord(existing);
  const incomingTerminal = isUserRequestTerminalRecord(incoming);
  const base =
    existingTerminal && isUserRequestProcessingRecord(incoming)
      ? { ...incoming, ...existing }
      : { ...existing, ...incoming };
  const existingTTFT = Number(existing.ttft_ms || 0);
  const incomingTTFT = Number(incoming.ttft_ms || 0);
  return {
    ...base,
    updated_at: Math.max(
      Number(existing.updated_at || 0),
      Number(incoming.updated_at || 0),
      Number(existing.completed_at || 0),
      Number(incoming.completed_at || 0),
      Number(existing.created_at || 0),
      Number(incoming.created_at || 0),
    ),
    ttft_ms:
      incomingTTFT > 0
        ? incoming.ttft_ms
        : existingTTFT > 0
          ? existing.ttft_ms
          : base.ttft_ms,
    completed_at:
      existingTerminal && !incomingTerminal
        ? existing.completed_at
        : base.completed_at,
    status:
      existingTerminal && !incomingTerminal ? existing.status : base.status,
    final_success:
      existingTerminal && !incomingTerminal
        ? existing.final_success
        : base.final_success,
    final_status_code:
      existingTerminal && !incomingTerminal
        ? existing.final_status_code
        : base.final_status_code,
    final_error_category:
      existingTerminal && !incomingTerminal
        ? existing.final_error_category
        : base.final_error_category,
    client_aborted:
      existingTerminal && !incomingTerminal
        ? existing.client_aborted
        : base.client_aborted,
    billing: incoming.billing || existing.billing,
    dispatch_record: incoming.dispatch_record || existing.dispatch_record,
    actual_group: incoming.actual_group || existing.actual_group,
    actual_channel_cost:
      Number(incoming.actual_channel_cost || 0) > 0
        ? incoming.actual_channel_cost
        : existing.actual_channel_cost,
    actual_group_ratio:
      Number(incoming.actual_group_ratio || 0) > 0
        ? incoming.actual_group_ratio
        : existing.actual_group_ratio,
  };
}

function mergeUserRequestDelta(userRequests, recentDelta) {
  if (
    !userRequests ||
    !Array.isArray(recentDelta) ||
    recentDelta.length === 0
  ) {
    return userRequests;
  }
  const mergedByKey = new Map();
  (userRequests.recent_requests || []).forEach((record) => {
    const key = userRequestKey(record);
    if (!key || mergedByKey.has(key)) return;
    mergedByKey.set(key, record);
  });
  recentDelta.forEach((record) => {
    const key = userRequestKey(record);
    if (!key) return;
    mergedByKey.set(key, mergeUserRequestRecord(mergedByKey.get(key), record));
  });
  const recentRequests = [...mergedByKey.values()]
    .sort(compareUserRequestRecordsForDisplay)
    .slice(0, RECENT_USER_REQUEST_LIMIT);
  return {
    ...userRequests,
    recent_requests: recentRequests,
  };
}

export function useModelGatewayObservabilityData({
  hours,
  trendBucket,
  defaultTrendBucket,
  recentLimit,
  topN,
  appliedFilters,
  viewMode,
  t,
}) {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState('');
  const [fallbackCountdown, setFallbackCountdown] = useState(
    FALLBACK_REFRESH_SECONDS,
  );
  const [fallbackMode, setFallbackMode] = useState(false);

  const requestParams = useMemo(
    () => ({
      hours,
      recent_limit: recentLimit,
      top_n: topN,
      trend_bucket_seconds:
        trendBucket === defaultTrendBucket ? undefined : trendBucket,
      view_mode: viewMode || undefined,
      model: appliedFilters.model || undefined,
      group: appliedFilters.group || undefined,
      channel_id: appliedFilters.channel_id || undefined,
      request_id: appliedFilters.request_id || undefined,
    }),
    [
      appliedFilters,
      defaultTrendBucket,
      hours,
      recentLimit,
      topN,
      trendBucket,
      viewMode,
    ],
  );
  const requestKey = useMemo(
    () => JSON.stringify(requestParams),
    [requestParams],
  );
  const latestRequestKeyRef = useRef(requestKey);
  const hasSnapshotRef = useRef(false);
  const activeRequestRef = useRef(null);
  const lastManualRefreshAtRef = useRef(0);

  const abortActiveRequest = useCallback(() => {
    const activeRequest = activeRequestRef.current;
    if (!activeRequest?.controller) return;
    activeRequest.controller.abort();
    activeRequestRef.current = null;
  }, []);

  const isAbortError = useCallback((err) => {
    return (
      err?.name === 'CanceledError' ||
      err?.name === 'AbortError' ||
      err?.code === 'ERR_CANCELED' ||
      err?.message === 'canceled'
    );
  }, []);

  const loadSummary = useCallback(
    async (silent = false, source = MANUAL_REFRESH_SOURCE) => {
      const activeRequestKey = requestKey;
      const isActiveRequest = () =>
        latestRequestKeyRef.current === activeRequestKey;
      const activeRequest = activeRequestRef.current;
      if (activeRequest?.key === activeRequestKey) {
        if (silent) {
          setRefreshing(true);
        }
        return activeRequest.promise;
      }
      abortActiveRequest();
      const controller = new AbortController();
      if (silent) {
        setRefreshing(true);
      } else {
        setLoading(true);
      }
      setError('');
      const requestPromise = (async () => {
        const response = await API.get(
          '/api/model_gateway/observability/summary',
          {
            params: requestParams,
            disableDuplicate: true,
            skipErrorHandler: true,
            signal: controller.signal,
          },
        );
        if (!isActiveRequest()) return;
        if (source === FALLBACK_REFRESH_SOURCE && hasSnapshotRef.current)
          return;
        setData(unwrapApiData(response));
        setFallbackCountdown(FALLBACK_REFRESH_SECONDS);
      })();
      activeRequestRef.current = {
        key: activeRequestKey,
        controller,
        promise: requestPromise,
      };
      try {
        await requestPromise;
      } catch (err) {
        if (isAbortError(err)) return;
        if (!isActiveRequest()) return;
        if (source === FALLBACK_REFRESH_SOURCE && hasSnapshotRef.current)
          return;
        const message =
          err?.response?.data?.message || err?.message || t('加载观测数据失败');
        setError(message);
        if (source !== FALLBACK_REFRESH_SOURCE) {
          showError(message);
        }
      } finally {
        if (activeRequestRef.current?.promise === requestPromise) {
          activeRequestRef.current = null;
        }
        if (!isActiveRequest()) return;
        setFallbackCountdown(FALLBACK_REFRESH_SECONDS);
        setLoading(false);
        setRefreshing(false);
      }
    },
    [abortActiveRequest, isAbortError, requestKey, requestParams, t],
  );

  const handleSnapshot = useCallback((payload) => {
    hasSnapshotRef.current = true;
    setData(payload || null);
    setError('');
    setLoading(false);
    setRefreshing(false);
    setFallbackMode(false);
    setFallbackCountdown(FALLBACK_REFRESH_SECONDS);
  }, []);

  const handleDelta = useCallback((payload) => {
    setData((current) => mergeDelta(current, payload));
  }, []);

  const handleRealtimeError = useCallback((message) => {
    if (!message) return;
    setError(message);
  }, []);

  const handleDisconnect = useCallback(() => {
    setFallbackMode(true);
  }, []);

  useEffect(() => {
    abortActiveRequest();
    latestRequestKeyRef.current = requestKey;
    hasSnapshotRef.current = false;
    setData(null);
    setLoading(true);
    setRefreshing(false);
    setError('');
    setFallbackMode(false);
    setFallbackCountdown(FALLBACK_REFRESH_SECONDS);
  }, [abortActiveRequest, requestKey]);

  useEffect(() => () => abortActiveRequest(), [abortActiveRequest]);

  const { connectionState } = useRealtimeSubscription({
    topic: 'model_gateway.observability',
    params: requestParams,
    enabled: true,
    onSnapshot: handleSnapshot,
    onDelta: handleDelta,
    onError: handleRealtimeError,
    onDisconnect: handleDisconnect,
  });

  useEffect(() => {
    loadSummary(false, FALLBACK_REFRESH_SOURCE);
  }, [loadSummary]);

  useEffect(() => {
    if (connectionState === REALTIME_STATES.CONNECTED) {
      setFallbackMode(false);
      return undefined;
    }
    if (!fallbackMode || loading || refreshing) return undefined;
    if (fallbackCountdown <= 0) {
      loadSummary(true, FALLBACK_REFRESH_SOURCE);
      return undefined;
    }
    const timer = window.setTimeout(() => {
      setFallbackCountdown((value) => Math.max(0, value - 1));
    }, 1000);
    return () => window.clearTimeout(timer);
  }, [
    connectionState,
    fallbackCountdown,
    fallbackMode,
    loadSummary,
    loading,
    refreshing,
  ]);

  const refresh = useCallback(() => {
    const now = Date.now();
    if (now - lastManualRefreshAtRef.current < MANUAL_REFRESH_DEBOUNCE_MS) {
      return;
    }
    lastManualRefreshAtRef.current = now;
    loadSummary(true, MANUAL_REFRESH_SOURCE);
  }, [loadSummary]);

  return {
    data,
    loading,
    refreshing,
    error,
    refresh,
    connectionState,
    fallbackMode,
    fallbackCountdown,
  };
}
