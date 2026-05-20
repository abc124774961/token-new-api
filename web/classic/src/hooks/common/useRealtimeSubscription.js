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

import { useEffect, useMemo, useState } from 'react';
import {
  realtimeClient,
  REALTIME_STATES,
} from '../../services/realtime/RealtimeClient';

export function useRealtimeSubscription({
  topic,
  params,
  enabled = true,
  onSnapshot,
  onDelta,
  onStatus,
  onError,
  onDisconnect,
}) {
  const [connectionState, setConnectionState] = useState(
    REALTIME_STATES.DISCONNECTED,
  );
  const paramsKey = useMemo(() => JSON.stringify(params || {}), [params]);

  useEffect(() => realtimeClient.onStateChange(setConnectionState), []);

  useEffect(() => {
    if (!enabled || !topic) return undefined;
    const parsedParams = JSON.parse(paramsKey);
    return realtimeClient.subscribe(topic, parsedParams, {
      onSnapshot,
      onDelta,
      onStatus,
      onError,
      onDisconnect,
    });
  }, [
    enabled,
    topic,
    paramsKey,
    onSnapshot,
    onDelta,
    onStatus,
    onError,
    onDisconnect,
  ]);

  return { connectionState };
}
