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
const DEFAULT_RECENT_USER_REQUEST_LIMIT = 50;
const MANUAL_REFRESH_DEBOUNCE_MS = 800;
const USER_REQUEST_VIEW_MODE = 'user_requests';
const USER_REQUEST_STATS_REFRESH_DELAY_MS = 600;
const USER_REQUEST_STATS_REFRESH_MIN_INTERVAL_MS = 15000;

function unwrapApiData(response) {
  return response?.data?.data || response?.data || {};
}

function mergeDelta(
  current,
  delta,
  userRequestLimit = DEFAULT_RECENT_USER_REQUEST_LIMIT,
) {
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
      userRequestLimit,
    ),
  };
}

function isUserRequestProcessingRecord(record) {
  return (
    String(record?.status || '').trim() === 'processing' &&
    !Number(record?.completed_at || 0) &&
    !record?.final_success &&
    !record?.client_aborted &&
    !record?.final_error_category &&
    !Number(record?.final_status_code || 0)
  );
}

function isUserRequestTerminalRecord(record) {
  if (!record) return false;
  if (Number(record?.completed_at || 0) > 0) return true;
  if (
    record?.final_success ||
    record?.client_aborted ||
    record?.final_error_category ||
    Number(record?.final_status_code || 0) > 0
  ) {
    return true;
  }
  const status = String(record?.status || '').trim();
  return [
    'success',
    'failed',
    'health_probe',
    'health_probe_failed',
    'client_aborted',
    'user_quota_exhausted',
    'settling',
    'settlement_timeout',
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

function userRequestHasBilling(record) {
  return Boolean(record?.billing);
}

function userRequestSuccessStatus(record) {
  const status = String(record?.status || '').trim();
  if (status === 'settling' || status === 'settlement_timeout') return status;
  if (record?.is_health_probe || record?.request_meta?.is_health_probe) {
    return 'health_probe';
  }
  return 'success';
}

function mergeUserRequestAttemptRecords(existing, incoming) {
  const existingAttempts = Array.isArray(existing?.attempt_records)
    ? existing.attempt_records
    : [];
  const incomingAttempts = Array.isArray(incoming?.attempt_records)
    ? incoming.attempt_records
    : [];
  const attempts = incomingAttempts.length ? incomingAttempts : existingAttempts;
  return attempts.length ? attempts : undefined;
}

function mergeUserRequestRecord(existing, incoming) {
  if (!existing) return incoming;
  if (!incoming) return existing;
  const existingTerminal = isUserRequestTerminalRecord(existing);
  const incomingTerminal = isUserRequestTerminalRecord(incoming);
  const keepExistingSettledDisplay =
    existingTerminal &&
    incomingTerminal &&
    userRequestHasBilling(existing) &&
    !userRequestHasBilling(incoming);
  const base =
    keepExistingSettledDisplay ||
    (existingTerminal && isUserRequestProcessingRecord(incoming))
      ? { ...incoming, ...existing }
      : { ...existing, ...incoming };
  const existingTTFT = Number(existing.ttft_ms || 0);
  const incomingTTFT = Number(incoming.ttft_ms || 0);
  const merged = {
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
    attempt_records: mergeUserRequestAttemptRecords(existing, incoming),
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
  const hasFinalSuccess =
    existing.final_success === true ||
    incoming.final_success === true ||
    merged.final_success === true;
  if (!hasFinalSuccess) return merged;
  return {
    ...merged,
    final_success: true,
    status: userRequestSuccessStatus(merged),
    final_status_code: 0,
    final_error_category: '',
    client_aborted: false,
    stream_interrupted: false,
  };
}

function mergeUserRequestSnapshot(
  currentUserRequests,
  snapshotUserRequests,
  limit = DEFAULT_RECENT_USER_REQUEST_LIMIT,
) {
  if (!snapshotUserRequests) return snapshotUserRequests;
  const snapshotRecent = Array.isArray(snapshotUserRequests.recent_requests)
    ? snapshotUserRequests.recent_requests
    : [];
  const currentRecent = Array.isArray(currentUserRequests?.recent_requests)
    ? currentUserRequests.recent_requests
    : [];
  if (!snapshotRecent.length || !currentRecent.length) {
    return snapshotUserRequests;
  }
  const currentByKey = new Map();
  currentRecent.forEach((record) => {
    const key = userRequestKey(record);
    if (key && !currentByKey.has(key)) {
      currentByKey.set(key, record);
    }
  });
  const normalizedLimit =
    Number.isFinite(Number(limit)) && Number(limit) > 0
      ? Number(limit)
      : DEFAULT_RECENT_USER_REQUEST_LIMIT;
  return {
    ...snapshotUserRequests,
    recent_requests: snapshotRecent
      .map((record) =>
        mergeUserRequestRecord(currentByKey.get(userRequestKey(record)), record),
      )
      .sort(compareUserRequestRecordsForDisplay)
      .slice(0, normalizedLimit),
  };
}

function mergePartialUserRequestSnapshot(current, snapshot, userRequestLimit) {
  if (!current || !snapshot) return snapshot || null;
  const mergedUserRequests = mergeUserRequestSnapshot(
    current.user_requests,
    snapshot.user_requests,
    userRequestLimit,
  );
  return {
    ...current,
    summary: {
      ...(current.summary || {}),
      end_time: snapshot.summary?.end_time || current.summary?.end_time,
    },
    user_requests: {
      ...(current.user_requests || {}),
      recent_requests:
        mergedUserRequests?.recent_requests ||
        current.user_requests?.recent_requests ||
        [],
    },
    partial: false,
  };
}

function mergeSnapshot(current, snapshot, userRequestLimit) {
  if (!current || !snapshot) return snapshot || null;
  if (snapshot.partial && current.partial !== true) {
    return mergePartialUserRequestSnapshot(current, snapshot, userRequestLimit);
  }
  return {
    ...snapshot,
    user_requests: mergeUserRequestSnapshot(
      current.user_requests,
      snapshot.user_requests,
      userRequestLimit,
    ),
  };
}

function mergeUserRequestDelta(
  userRequests,
  recentDelta,
  limit = DEFAULT_RECENT_USER_REQUEST_LIMIT,
) {
  if (
    !userRequests ||
    !Array.isArray(recentDelta) ||
    recentDelta.length === 0
  ) {
    return userRequests;
  }
  const normalizedLimit =
    Number.isFinite(Number(limit)) && Number(limit) > 0
      ? Number(limit)
      : DEFAULT_RECENT_USER_REQUEST_LIMIT;
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
    .slice(0, normalizedLimit);
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

  const fullRequestParams = useMemo(
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
  const isUserRequestMode = viewMode === USER_REQUEST_VIEW_MODE;
  const requestParams = useMemo(() => {
    if (!isUserRequestMode) return fullRequestParams;
    return {
      ...fullRequestParams,
      scan_limit: Math.max(1, recentLimit || DEFAULT_RECENT_USER_REQUEST_LIMIT),
      top_n: 1,
      lite: true,
      include_dispatch: true,
      recent_only: true,
    };
  }, [fullRequestParams, isUserRequestMode, recentLimit]);
  const requestKey = useMemo(
    () => JSON.stringify(requestParams),
    [requestParams],
  );
  const fullRequestKey = useMemo(
    () => JSON.stringify(fullRequestParams),
    [fullRequestParams],
  );
  const latestRequestKeyRef = useRef(requestKey);
  const latestFullRequestKeyRef = useRef(fullRequestKey);
  const hasSnapshotRef = useRef(false);
  const activeRequestRef = useRef(null);
  const activeFullRequestRef = useRef(null);
  const statsRefreshTimerRef = useRef(null);
  const lastFullStatsRefreshRef = useRef({ key: '', at: 0 });
  const lastManualRefreshAtRef = useRef(0);

  const abortActiveRequest = useCallback(() => {
    const activeRequest = activeRequestRef.current;
    if (!activeRequest?.controller) return;
    activeRequest.controller.abort();
    activeRequestRef.current = null;
  }, []);

  const abortActiveFullRequest = useCallback(() => {
    const activeRequest = activeFullRequestRef.current;
    if (!activeRequest?.controller) return;
    activeRequest.controller.abort();
    activeFullRequestRef.current = null;
  }, []);

  const clearStatsRefreshTimer = useCallback(() => {
    if (!statsRefreshTimerRef.current) return;
    window.clearTimeout(statsRefreshTimerRef.current);
    statsRefreshTimerRef.current = null;
  }, []);

  const isAbortError = useCallback((err) => {
    return (
      err?.name === 'CanceledError' ||
      err?.name === 'AbortError' ||
      err?.code === 'ERR_CANCELED' ||
      err?.message === 'canceled'
    );
  }, []);

  const loadFullStats = useCallback(
    async ({ force = false } = {}) => {
      if (!isUserRequestMode) return;
      const activeRequestKey = fullRequestKey;
      const now = Date.now();
      const lastRefresh = lastFullStatsRefreshRef.current;
      if (
        !force &&
        lastRefresh.key === activeRequestKey &&
        now - lastRefresh.at < USER_REQUEST_STATS_REFRESH_MIN_INTERVAL_MS
      ) {
        return;
      }
      const activeRequest = activeFullRequestRef.current;
      if (activeRequest?.key === activeRequestKey) {
        return activeRequest.promise;
      }
      abortActiveFullRequest();
      const controller = new AbortController();
      const requestPromise = (async () => {
        const response = await API.get(
          '/api/model_gateway/observability/summary',
          {
            params: fullRequestParams,
            disableDuplicate: true,
            skipErrorHandler: true,
            signal: controller.signal,
          },
        );
        if (latestFullRequestKeyRef.current !== activeRequestKey) return;
        const payload = unwrapApiData(response);
        setData((current) =>
          mergeSnapshot(current, payload || null, recentLimit),
        );
        lastFullStatsRefreshRef.current = {
          key: activeRequestKey,
          at: Date.now(),
        };
      })();
      activeFullRequestRef.current = {
        key: activeRequestKey,
        controller,
        promise: requestPromise,
      };
      try {
        await requestPromise;
      } catch (err) {
        if (isAbortError(err)) return;
      } finally {
        if (activeFullRequestRef.current?.promise === requestPromise) {
          activeFullRequestRef.current = null;
        }
      }
    },
    [
      abortActiveFullRequest,
      fullRequestKey,
      fullRequestParams,
      isAbortError,
      isUserRequestMode,
      recentLimit,
    ],
  );

  const scheduleFullStatsRefresh = useCallback(
    (force = false) => {
      if (!isUserRequestMode) return;
      clearStatsRefreshTimer();
      statsRefreshTimerRef.current = window.setTimeout(() => {
        statsRefreshTimerRef.current = null;
        loadFullStats({ force });
      }, USER_REQUEST_STATS_REFRESH_DELAY_MS);
    },
    [clearStatsRefreshTimer, isUserRequestMode, loadFullStats],
  );

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
        const payload = unwrapApiData(response);
        setData((current) =>
          mergeSnapshot(current, payload || null, recentLimit),
        );
        if (isUserRequestMode) {
          scheduleFullStatsRefresh(
            lastFullStatsRefreshRef.current.key !== fullRequestKey,
          );
        }
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
    [
      abortActiveRequest,
      isAbortError,
      isUserRequestMode,
      fullRequestKey,
      recentLimit,
      requestKey,
      requestParams,
      scheduleFullStatsRefresh,
      t,
    ],
  );

  const handleSnapshot = useCallback(
    (payload) => {
      hasSnapshotRef.current = true;
      setData((current) => mergeSnapshot(current, payload || null, recentLimit));
      if (isUserRequestMode) {
        scheduleFullStatsRefresh(
          lastFullStatsRefreshRef.current.key !== fullRequestKey,
        );
      }
      setError('');
      setLoading(false);
      setRefreshing(false);
      setFallbackMode(false);
      setFallbackCountdown(FALLBACK_REFRESH_SECONDS);
    },
    [
      fullRequestKey,
      isUserRequestMode,
      recentLimit,
      scheduleFullStatsRefresh,
    ],
  );

  const handleDelta = useCallback(
    (payload) => {
      setData((current) => mergeDelta(current, payload, recentLimit));
    },
    [recentLimit],
  );

  const handleRealtimeError = useCallback((message) => {
    if (!message) return;
    setError(message);
  }, []);

  const handleDisconnect = useCallback(() => {
    setFallbackMode(true);
  }, []);

  useEffect(() => {
    abortActiveRequest();
    abortActiveFullRequest();
    clearStatsRefreshTimer();
    latestRequestKeyRef.current = requestKey;
    latestFullRequestKeyRef.current = fullRequestKey;
    hasSnapshotRef.current = false;
    setData(null);
    setLoading(true);
    setRefreshing(false);
    setError('');
    setFallbackMode(false);
    setFallbackCountdown(FALLBACK_REFRESH_SECONDS);
  }, [
    abortActiveFullRequest,
    abortActiveRequest,
    clearStatsRefreshTimer,
    fullRequestKey,
    requestKey,
  ]);

  useEffect(
    () => () => {
      abortActiveRequest();
      abortActiveFullRequest();
      clearStatsRefreshTimer();
    },
    [abortActiveFullRequest, abortActiveRequest, clearStatsRefreshTimer],
  );

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
