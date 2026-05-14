/*
Copyright (C) 2023-2026 QuantumNous

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
import type { StatusBadgeProps } from '@/components/status-badge'
import type { LogOtherData } from '../types'

type StreamStatus = NonNullable<LogOtherData['stream_status']>

export type StreamStatusDisplay = {
  labelKey: string
  variant: StatusBadgeProps['variant']
  iconClassName: string
}

function normalizeText(value: string | undefined): string {
  return (value || '').toLowerCase()
}

export function isClientDisconnectedStreamStatus(
  status: StreamStatus | undefined
): boolean {
  if (!status) return false

  const reason = normalizeText(status.end_reason)
  const statusText = normalizeText(status.status)
  const endError = normalizeText(status.end_error)
  const errors = Array.isArray(status.errors)
    ? status.errors.map(normalizeText).join('\n')
    : ''

  return (
    statusText === 'client_gone' ||
    reason === 'client_gone' ||
    endError.includes('context canceled') ||
    endError.includes('client disconnected') ||
    errors.includes('context canceled') ||
    errors.includes('client disconnected')
  )
}

export function getStreamStatusDisplay(
  status: StreamStatus | undefined
): StreamStatusDisplay {
  if (isClientDisconnectedStreamStatus(status)) {
    return {
      labelKey: 'Client Disconnected',
      variant: 'warning',
      iconClassName: 'text-amber-500',
    }
  }

  return {
    labelKey: 'Error',
    variant: 'danger',
    iconClassName: 'text-red-500',
  }
}
