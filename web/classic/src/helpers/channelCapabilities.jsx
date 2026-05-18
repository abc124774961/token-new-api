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
import { Tag, Tooltip } from '@douyinfe/semi-ui';

export const CHANNEL_CAPABILITY_ORDER = [
  'chat',
  'responses',
  'compact',
  'image_api',
  'codex_tool:image_generation',
];

const parseJsonObject = (value) => {
  if (!value) return {};
  if (typeof value === 'object') return value;
  if (typeof value !== 'string') return {};
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (error) {
    return {};
  }
};

const getChannelModelsForCapability = (record) => {
  const models = String(record?.models || '')
    .split(',')
    .map((model) => model.trim())
    .filter(Boolean);
  const modelMapping = parseJsonObject(record?.model_mapping);
  Object.values(modelMapping).forEach((mappedModel) => {
    if (typeof mappedModel === 'string' && mappedModel.trim()) {
      models.push(mappedModel.trim());
    }
  });
  return models;
};

const isImageGenerationModel = (modelName) => {
  const model = String(modelName || '').trim().toLowerCase();
  if (!model) return false;
  return (
    model.includes('dall-e-3') ||
    model.includes('dall-e-2') ||
    model.startsWith('gpt-image-') ||
    model.startsWith('imagen-') ||
    model.includes('flux-') ||
    model.includes('flux.1-')
  );
};

const getCodexSupportedTools = (type, settings) => {
  if (type === 57) {
    return ['image_generation'];
  }
  if (type !== 1 || settings.codex_compatibility_mode !== true) {
    return [];
  }
  if (Array.isArray(settings.codex_supported_tools)) {
    const tools = settings.codex_supported_tools
      .map((tool) => String(tool || '').trim())
      .filter(Boolean);
    if (tools.length > 0) {
      return Array.from(new Set(tools));
    }
  }
  return settings.codex_image_generation_tool_supported === true
    ? ['image_generation']
    : [];
};

const usesResponsesWireAPI = (settings) => {
  const wireAPI = String(settings?.wire_api || '')
    .trim()
    .replace(/^\/+|\/+$/g, '')
    .toLowerCase();
  return (
    wireAPI === 'responses' ||
    wireAPI.startsWith('responses/') ||
    wireAPI.endsWith('/responses') ||
    wireAPI.includes('/responses/')
  );
};

export const sortChannelCapabilityKeys = (keys = []) => {
  const orderMap = CHANNEL_CAPABILITY_ORDER.reduce((acc, key, index) => {
    acc[key] = index;
    return acc;
  }, {});
  return [...keys].sort((a, b) => {
    const orderA = orderMap[a] ?? 1000;
    const orderB = orderMap[b] ?? 1000;
    if (orderA !== orderB) return orderA - orderB;
    return String(a).localeCompare(String(b));
  });
};

export const buildChannelCapabilityKeys = (record) => {
  const rows =
    Array.isArray(record?.children) && record.children.length > 0
      ? record.children
      : [record];
  const keys = new Set();

  rows.forEach((row) => {
    if (!row) return;
    const type = Number(row.type);
    const settings = parseJsonObject(row.settings);
    const endpointTypes = Array.isArray(row.supported_endpoint_types)
      ? row.supported_endpoint_types
      : [];
    const codexCompatible =
      type === 57 ||
      (type === 1 && settings.codex_compatibility_mode === true);
    const codexTools = getCodexSupportedTools(type, settings);
    const models = getChannelModelsForCapability(row);
    const hasImageApi = models.some((model) => {
      if (!isImageGenerationModel(model)) return false;
      return true;
    });

    if (
      codexCompatible ||
      usesResponsesWireAPI(settings) ||
      type === 48 ||
      endpointTypes.includes('openai-response')
    ) {
      keys.add('responses');
    }
    if (codexCompatible || endpointTypes.includes('openai-response-compact')) {
      keys.add('compact');
    }
    codexTools.forEach((tool) => {
      keys.add(`codex_tool:${tool}`);
    });
    if (
      hasImageApi ||
      endpointTypes.includes('image-generation') ||
      endpointTypes.includes('image-edit')
    ) {
      keys.add('image_api');
    }
  });

  return keys.size > 0 ? sortChannelCapabilityKeys(Array.from(keys)) : ['chat'];
};

export const getChannelCapabilityMeta = (key, t) => {
  const tagMap = {
    chat: { label: 'Chat', color: 'grey' },
    responses: { label: 'Responses', color: 'light-blue' },
    compact: { label: 'Compact', color: 'teal' },
    image_api: { label: t('图片API'), color: 'purple' },
  };
  if (key?.startsWith('codex_tool:')) {
    return { label: key.slice('codex_tool:'.length), color: 'violet' };
  }
  return tagMap[key] || { label: key, color: 'grey' };
};

export const renderChannelCapabilities = (
  capabilitySource,
  t,
  className = 'inline-flex max-w-[260px] flex-wrap gap-1',
) => {
  const capabilityKeys = Array.isArray(capabilitySource)
    ? sortChannelCapabilityKeys(capabilitySource)
    : buildChannelCapabilityKeys(capabilitySource);
  const capabilities = capabilityKeys
    .map((key) => getChannelCapabilityMeta(key, t))
    .filter(Boolean);

  return (
    <Tooltip
      content={capabilities.map((item) => item.label).join(' / ')}
      trigger='hover'
    >
      <div className={className}>
        {capabilities.map((item) => (
          <Tag
            key={item.label}
            color={item.color}
            type='light'
            shape='circle'
            size='small'
          >
            {item.label}
          </Tag>
        ))}
      </div>
    </Tooltip>
  );
};
