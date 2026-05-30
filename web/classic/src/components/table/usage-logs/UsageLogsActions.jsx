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

import React from 'react';
import { Button } from '@douyinfe/semi-ui';
import CompactModeToggle from '../../common/ui/CompactModeToggle';
import { PiPulseDuotone } from 'react-icons/pi';

const LogsActions = ({
  compactMode,
  setCompactMode,
  isAdminUser,
  openChannelAffinityDiagnosticsModal,
  t,
}) => {
  return (
    <div className='ct-usage-logs-actions'>
      <div className='ct-usage-logs-action-group'>
        <span>{t('快速操作')}</span>
        <CompactModeToggle
          compactMode={compactMode}
          setCompactMode={setCompactMode}
          t={t}
        />
      </div>
      {isAdminUser ? (
        <Button
          className='ct-usage-logs-diagnostics-button'
          theme='borderless'
          type='tertiary'
          icon={<PiPulseDuotone size={16} />}
          onClick={openChannelAffinityDiagnosticsModal}
        >
          {t('缓存诊断')}
        </Button>
      ) : null}
    </div>
  );
};

export default LogsActions;
