export const parseChannelRuntimeInfo = (value) => {
  if (!value) return {};
  if (typeof value === 'object') return value;
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (error) {
    return {};
  }
};

export const flattenChannelRows = (rows = []) => {
  const flattened = [];
  rows.forEach((row) => {
    if (Array.isArray(row?.children)) {
      row.children.forEach((child) => flattened.push(child));
      return;
    }
    flattened.push(row);
  });
  return flattened;
};

export const isBalanceInsufficientChannel = (record) => {
  if (record?.balance_insufficient === true) return true;
  if (Number(record?.runtime_balance_insufficient_count || 0) > 0) return true;
  const otherInfo = parseChannelRuntimeInfo(record?.other_info);
  const statusReason = String(
    record?.status_reason ||
      otherInfo.status_reason ||
      otherInfo.pause_type ||
      '',
  )
    .trim()
    .toLowerCase();
  return (
    statusReason === 'balance_insufficient' || statusReason.includes('余额不足')
  );
};

const hasActiveTimedStatus = (status) => {
  if (!status?.active) return false;
  if (status.until) {
    return Number(status.until) > Math.floor(Date.now() / 1000);
  }
  return Number(status.remaining_seconds || 0) > 0 || status.active === true;
};

export const isRecoverableHealthChannel = (record) => {
  if (!record || record.children !== undefined) return false;
  const runtimeCircuit = record?.runtime_circuit || {};
  return (
    isBalanceInsufficientChannel(record) ||
    hasActiveTimedStatus(record?.failure_avoidance) ||
    hasActiveTimedStatus(record?.concurrency_cooldown) ||
    Number(runtimeCircuit.open_runtime_keys || 0) > 0 ||
    Number(runtimeCircuit.half_open_runtime_keys || 0) > 0
  );
};
