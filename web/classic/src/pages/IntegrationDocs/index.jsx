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

import React, { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
  ArrowRight,
  BookOpenCheck,
  CheckCircle2,
  Clipboard,
  FileTerminal,
  KeyRound,
  Layers3,
  PlugZap,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import { copy, getServerAddress, showSuccess } from '../../helpers';
import './integration-docs.css';

const ACCESS_DOC_PROVIDERS = [
  {
    key: 'codex',
    name: 'Codex',
    titleKey: 'Codex 接入本站 API',
    categoryKey: '代码代理',
    statusKey: '已开放',
    summaryKey:
      '面向 Codex 的 Responses API 接入说明，适合代码生成、项目修改、长上下文任务和工具调用场景。',
    endpointSuffix: '/v1',
    tokenRoute: '/console/token',
    logRoute: '/console/log',
    apiFormatKey: 'Responses API / OpenAI Compatible',
    recommendedMethodKey: 'CC Switch 一键导入',
    highlights: [
      {
        icon: PlugZap,
        labelKey: '接入端点',
        valueKey: 'Base URL 只填到 /v1',
      },
      {
        icon: Layers3,
        labelKey: '推荐方式',
        valueKey: 'CC Switch 一键导入',
      },
      {
        icon: FileTerminal,
        labelKey: '支持接口',
        valueKey: 'Responses API',
      },
    ],
    steps: [
      {
        titleKey: '创建或复制令牌',
        descriptionKey: '进入控制台令牌页面，确认令牌状态正常且账户余额充足。',
      },
      {
        titleKey: '选择 Codex 模型',
        descriptionKey:
          '从令牌可用模型中选择 Codex 可用模型，模型名必须和本站显示一致。',
      },
      {
        titleKey: '一键导入或手动填写',
        descriptionKey:
          '优先使用令牌页的 CC Switch 导入；手动配置时按配置表填写。',
      },
      {
        titleKey: '发起测试请求',
        descriptionKey:
          '在 Codex 客户端发送简单请求，然后回到本站用量日志确认记录。',
      },
    ],
    configRows: [
      {
        labelKey: 'Provider Name',
        valueKey: '自定义名称，例如 My Codex',
      },
      {
        labelKey: 'API Key',
        valueKey: '本站令牌，例如 sk-xxxx',
      },
      {
        labelKey: 'Base URL',
        valueKey: '当前站点域名 + /v1',
        copyType: 'baseUrl',
      },
      {
        labelKey: 'Model',
        valueKey: '选择本站支持的 Codex 模型',
      },
      {
        labelKey: 'API Format',
        valueKey: 'Responses API / OpenAI Compatible',
      },
      {
        labelKey: '主要端点',
        valueKey: '/v1/responses, /v1/responses/compact',
      },
    ],
    checklist: [
      '令牌未禁用，且账户余额充足。',
      'Base URL 使用当前站点域名并以 /v1 结尾。',
      '模型名来自本站模型列表或令牌可用模型。',
      '如使用图片、工具等能力，请确认所选模型和渠道支持。',
    ],
    faq: [
      {
        title: '401 Unauthorized',
        answerKey: 'API Key 错误或未带 sk- 前缀，重新复制令牌。',
      },
      {
        title: '403 Forbidden',
        answerKey: '令牌、分组或模型权限不足，检查令牌限制和账户状态。',
      },
      {
        title: 'model not found',
        answerKey: '模型名不匹配，使用本站显示的完整模型名。',
      },
      {
        title: 'insufficient quota',
        answerKey: '余额不足，充值后重试。',
      },
    ],
  },
  {
    key: 'claude-code',
    name: 'Claude Code',
    titleKey: 'Claude Code 接入本站 API',
    categoryKey: '代码代理',
    statusKey: '已开放',
    summaryKey:
      '面向 Claude Code 的 Anthropic Messages 接入说明，适合项目级代码生成、重构和长会话开发。',
    endpointSuffix: '',
    tokenRoute: '/console/token',
    logRoute: '/console/log',
    apiFormatKey: 'Anthropic Messages',
    recommendedMethodKey: 'CC Switch 一键导入',
    highlights: [
      {
        icon: PlugZap,
        labelKey: '接入端点',
        valueKey: 'Base URL 填站点根地址',
      },
      {
        icon: Layers3,
        labelKey: '推荐方式',
        valueKey: 'CC Switch 一键导入',
      },
      {
        icon: FileTerminal,
        labelKey: '支持接口',
        valueKey: 'Anthropic Messages',
      },
    ],
    steps: [
      {
        titleKey: '创建或复制令牌',
        descriptionKey: '进入控制台令牌页面，确认令牌状态正常且账户余额充足。',
      },
      {
        titleKey: '选择 Claude 模型',
        descriptionKey:
          '从令牌可用模型中选择 Claude 可用模型，建议区分主模型、Haiku、Sonnet 和 Opus。',
      },
      {
        titleKey: '一键导入或手动填写',
        descriptionKey:
          '优先使用令牌页的 CC Switch 导入；手动配置时 Base URL 填站点根地址。',
      },
      {
        titleKey: '发起测试请求',
        descriptionKey:
          '在 Claude Code 中发送简单任务，然后回到本站用量日志确认 /v1/messages 记录。',
      },
    ],
    configRows: [
      {
        labelKey: 'Provider Name',
        valueKey: '自定义名称，例如 My Claude Code',
      },
      {
        labelKey: 'API Key',
        valueKey: '本站令牌，例如 sk-xxxx',
      },
      {
        labelKey: 'Base URL',
        valueKey: '当前站点根地址',
        copyType: 'baseUrl',
      },
      {
        labelKey: 'Model',
        valueKey: '选择本站支持的 Claude 模型',
      },
      {
        labelKey: 'API Format',
        valueKey: 'Anthropic Messages',
      },
      {
        labelKey: '主要端点',
        valueKey: '/v1/messages',
      },
    ],
    checklist: [
      '令牌未禁用，且账户余额充足。',
      'Base URL 使用当前站点根地址，不要手动追加 /v1/messages。',
      '模型名来自本站模型列表或令牌可用模型。',
      '如使用长缓存、思考模型等能力，请确认所选模型和渠道支持。',
    ],
    faq: [
      {
        title: '401 Unauthorized',
        answerKey: 'API Key 错误或未带 sk- 前缀，重新复制令牌。',
      },
      {
        title: '403 Forbidden',
        answerKey: '令牌、分组或模型权限不足，检查令牌限制和账户状态。',
      },
      {
        title: 'model not found',
        answerKey: '模型名不匹配，使用本站显示的完整模型名。',
      },
      {
        title: 'invalid x-api-key',
        answerKey: 'Claude 客户端未正确读取令牌，重新保存配置后再测试。',
      },
    ],
  },
  {
    key: 'openai-sdk',
    name: 'OpenAI SDK',
    titleKey: 'OpenAI SDK 接入本站 API',
    categoryKey: 'SDK',
    statusKey: '已开放',
    summaryKey:
      '面向 OpenAI 官方 SDK 和兼容 SDK 的接入说明，适合在后端服务、脚本和自动化任务中统一调用本站中转。',
    endpointSuffix: '/v1',
    tokenRoute: '/console/token',
    logRoute: '/console/log',
    apiFormatKey: 'OpenAI Compatible',
    recommendedMethodKey: '环境变量或 SDK 配置',
    highlights: [
      {
        icon: PlugZap,
        labelKey: '接入端点',
        valueKey: 'Base URL 只填到 /v1',
      },
      {
        icon: Layers3,
        labelKey: '推荐方式',
        valueKey: '环境变量或 SDK 配置',
      },
      {
        icon: FileTerminal,
        labelKey: '支持接口',
        valueKey: 'Chat Completions / Responses',
      },
    ],
    steps: [
      {
        titleKey: '创建或复制令牌',
        descriptionKey: '进入控制台令牌页面，确认令牌状态正常且账户余额充足。',
      },
      {
        titleKey: '设置 SDK 参数',
        descriptionKey:
          '在 SDK 中设置 apiKey 和 baseURL，baseURL 使用当前站点域名加 /v1。',
      },
      {
        titleKey: '填写模型名称',
        descriptionKey:
          '模型名必须使用本站模型列表或令牌可用模型中的完整名称。',
      },
      {
        titleKey: '查看调用日志',
        descriptionKey:
          '请求完成后到本站用量日志查看消耗、模型、分组和请求状态。',
      },
    ],
    configRows: [
      {
        labelKey: 'API Key',
        valueKey: '本站令牌，例如 sk-xxxx',
      },
      {
        labelKey: 'Base URL',
        valueKey: '当前站点域名 + /v1',
        copyType: 'baseUrl',
      },
      {
        labelKey: 'Model',
        valueKey: '选择本站支持的 OpenAI 兼容模型',
      },
      {
        labelKey: 'API Format',
        valueKey: 'OpenAI Compatible',
      },
      {
        labelKey: '主要端点',
        valueKey: '/v1/chat/completions, /v1/responses, /v1/embeddings',
      },
    ],
    checklist: [
      '令牌未禁用，且账户余额充足。',
      'Base URL 使用当前站点域名并以 /v1 结尾。',
      '模型名来自本站模型列表或令牌可用模型。',
      '如果 SDK 报连接失败，请确认网络环境可以访问当前站点。',
    ],
    faq: [
      {
        title: '401 Unauthorized',
        answerKey: 'API Key 错误或未带 sk- 前缀，重新复制令牌。',
      },
      {
        title: '404 Not Found',
        answerKey: 'Base URL 填写错误，确认只填到指定的接入地址。',
      },
      {
        title: 'model not found',
        answerKey: '模型名不匹配，使用本站显示的完整模型名。',
      },
      {
        title: 'timeout',
        answerKey: '上游或网络波动导致超时，可稍后重试或切换模型。',
      },
    ],
  },
  {
    key: 'openai-compatible',
    name: 'OpenAI Compatible',
    titleKey: 'OpenAI 兼容客户端接入本站 API',
    categoryKey: '通用客户端',
    statusKey: '已开放',
    summaryKey:
      '面向任意支持 OpenAI Compatible 的客户端，统一使用本站 API Key、Base URL、模型列表和用量日志。',
    endpointSuffix: '/v1',
    tokenRoute: '/console/token',
    logRoute: '/console/log',
    apiFormatKey: 'OpenAI Compatible',
    recommendedMethodKey: '手动填写配置',
    highlights: [
      {
        icon: PlugZap,
        labelKey: '接入端点',
        valueKey: 'Base URL 只填到 /v1',
      },
      {
        icon: Layers3,
        labelKey: '推荐方式',
        valueKey: '手动填写配置',
      },
      {
        icon: FileTerminal,
        labelKey: '支持接口',
        valueKey: 'OpenAI Compatible',
      },
    ],
    steps: [
      {
        titleKey: '创建或复制令牌',
        descriptionKey: '进入控制台令牌页面，确认令牌状态正常且账户余额充足。',
      },
      {
        titleKey: '选择 OpenAI Compatible',
        descriptionKey:
          '在客户端供应商或接口类型中选择 OpenAI、OpenAI Compatible 或自定义 OpenAI。',
      },
      {
        titleKey: '填写 Base URL 和 API Key',
        descriptionKey: 'Base URL 填当前站点域名加 /v1，API Key 填本站令牌。',
      },
      {
        titleKey: '选择模型并测试',
        descriptionKey:
          '选择本站支持的模型发起测试，成功后可在用量日志中确认记录。',
      },
    ],
    configRows: [
      {
        labelKey: 'Provider Type',
        valueKey: 'OpenAI Compatible / Custom OpenAI',
      },
      {
        labelKey: 'API Key',
        valueKey: '本站令牌，例如 sk-xxxx',
      },
      {
        labelKey: 'Base URL',
        valueKey: '当前站点域名 + /v1',
        copyType: 'baseUrl',
      },
      {
        labelKey: 'Model',
        valueKey: '选择本站支持的 OpenAI 兼容模型',
      },
      {
        labelKey: 'API Format',
        valueKey: 'OpenAI Compatible',
      },
    ],
    checklist: [
      '令牌未禁用，且账户余额充足。',
      'Base URL 使用当前站点域名并以 /v1 结尾。',
      '模型名来自本站模型列表或令牌可用模型。',
      '客户端如果有单独的路径配置，不要重复填写 /chat/completions。',
    ],
    faq: [
      {
        title: '401 Unauthorized',
        answerKey: 'API Key 错误或未带 sk- 前缀，重新复制令牌。',
      },
      {
        title: '404 Not Found',
        answerKey: 'Base URL 填写错误，确认只填到指定的接入地址。',
      },
      {
        title: 'model not found',
        answerKey: '模型名不匹配，使用本站显示的完整模型名。',
      },
      {
        title: 'no usage log',
        answerKey:
          '客户端可能没有请求到本站，请检查 Base URL 是否仍指向官方地址。',
      },
    ],
  },
  {
    key: 'cursor-continue',
    name: 'Cursor / Continue',
    titleKey: 'Cursor / Continue 接入本站 API',
    categoryKey: '编辑器插件',
    statusKey: '已开放',
    summaryKey:
      '面向 Cursor、Continue 等编辑器插件，适合补全、对话、代码解释和小型重构等 OpenAI 兼容场景。',
    endpointSuffix: '/v1',
    tokenRoute: '/console/token',
    logRoute: '/console/log',
    apiFormatKey: 'OpenAI Compatible',
    recommendedMethodKey: '自定义 OpenAI 供应商',
    highlights: [
      {
        icon: PlugZap,
        labelKey: '接入端点',
        valueKey: 'Base URL 只填到 /v1',
      },
      {
        icon: Layers3,
        labelKey: '推荐方式',
        valueKey: '自定义 OpenAI 供应商',
      },
      {
        icon: FileTerminal,
        labelKey: '支持接口',
        valueKey: 'Chat Completions',
      },
    ],
    steps: [
      {
        titleKey: '创建或复制令牌',
        descriptionKey: '进入控制台令牌页面，确认令牌状态正常且账户余额充足。',
      },
      {
        titleKey: '新增自定义供应商',
        descriptionKey:
          '在编辑器插件中新增 OpenAI Compatible 或自定义 OpenAI 供应商。',
      },
      {
        titleKey: '填写模型和接入地址',
        descriptionKey:
          'Base URL 填当前站点域名加 /v1，模型名使用本站可用模型。',
      },
      {
        titleKey: '运行一次编辑器任务',
        descriptionKey:
          '用解释代码或生成测试等小任务验证连接，然后检查用量日志。',
      },
    ],
    configRows: [
      {
        labelKey: 'Provider Type',
        valueKey: 'OpenAI Compatible / Custom OpenAI',
      },
      {
        labelKey: 'API Key',
        valueKey: '本站令牌，例如 sk-xxxx',
      },
      {
        labelKey: 'Base URL',
        valueKey: '当前站点域名 + /v1',
        copyType: 'baseUrl',
      },
      {
        labelKey: 'Model',
        valueKey: '选择本站支持的 OpenAI 兼容模型',
      },
      {
        labelKey: '主要端点',
        valueKey: '/v1/chat/completions',
      },
    ],
    checklist: [
      '令牌未禁用，且账户余额充足。',
      'Base URL 使用当前站点域名并以 /v1 结尾。',
      '模型名来自本站模型列表或令牌可用模型。',
      '如果插件区分补全模型和聊天模型，请分别填写本站支持的模型。',
    ],
    faq: [
      {
        title: '401 Unauthorized',
        answerKey: 'API Key 错误或未带 sk- 前缀，重新复制令牌。',
      },
      {
        title: 'model not found',
        answerKey: '模型名不匹配，使用本站显示的完整模型名。',
      },
      {
        title: 'empty response',
        answerKey:
          '编辑器插件可能选择了不兼容的接口类型，切换到 OpenAI Compatible 后重试。',
      },
      {
        title: 'timeout',
        answerKey: '上游或网络波动导致超时，可稍后重试或切换模型。',
      },
    ],
  },
  {
    key: 'chat-clients',
    name: 'Cherry Studio / LobeChat',
    titleKey: '聊天客户端接入本站 API',
    categoryKey: '聊天客户端',
    statusKey: '已开放',
    summaryKey:
      '面向 Cherry Studio、LobeChat 等聊天客户端，适合把本站作为统一模型供应商来管理 Key、模型和费用。',
    endpointSuffix: '/v1',
    tokenRoute: '/console/token',
    logRoute: '/console/log',
    apiFormatKey: 'OpenAI Compatible',
    recommendedMethodKey: '新增 OpenAI 兼容供应商',
    highlights: [
      {
        icon: PlugZap,
        labelKey: '接入端点',
        valueKey: 'Base URL 只填到 /v1',
      },
      {
        icon: Layers3,
        labelKey: '推荐方式',
        valueKey: '新增 OpenAI 兼容供应商',
      },
      {
        icon: FileTerminal,
        labelKey: '支持接口',
        valueKey: 'Chat Completions / Responses',
      },
    ],
    steps: [
      {
        titleKey: '创建或复制令牌',
        descriptionKey: '进入控制台令牌页面，确认令牌状态正常且账户余额充足。',
      },
      {
        titleKey: '新增 OpenAI 兼容供应商',
        descriptionKey:
          '在聊天客户端中新增 OpenAI Compatible、OpenAI API 或自定义供应商。',
      },
      {
        titleKey: '同步或手动填写模型',
        descriptionKey:
          '客户端支持模型同步时可从本站拉取模型；不支持时手动填写模型名。',
      },
      {
        titleKey: '发送测试对话',
        descriptionKey:
          '发送一句简单对话，确认客户端返回正常并在本站生成用量日志。',
      },
    ],
    configRows: [
      {
        labelKey: 'Provider Type',
        valueKey: 'OpenAI Compatible / Custom Provider',
      },
      {
        labelKey: 'API Key',
        valueKey: '本站令牌，例如 sk-xxxx',
      },
      {
        labelKey: 'Base URL',
        valueKey: '当前站点域名 + /v1',
        copyType: 'baseUrl',
      },
      {
        labelKey: 'Model',
        valueKey: '选择本站支持的 OpenAI 兼容模型',
      },
      {
        labelKey: '模型列表',
        valueKey: '/v1/models',
      },
    ],
    checklist: [
      '令牌未禁用，且账户余额充足。',
      'Base URL 使用当前站点域名并以 /v1 结尾。',
      '模型名来自本站模型列表或令牌可用模型。',
      '如果客户端缓存了旧模型列表，请刷新供应商模型或重新导入。',
    ],
    faq: [
      {
        title: '401 Unauthorized',
        answerKey: 'API Key 错误或未带 sk- 前缀，重新复制令牌。',
      },
      {
        title: 'models empty',
        answerKey: '令牌可能没有可用模型，检查令牌限制或手动填写模型名。',
      },
      {
        title: 'model not found',
        answerKey: '模型名不匹配，使用本站显示的完整模型名。',
      },
      {
        title: 'no usage log',
        answerKey:
          '客户端可能没有请求到本站，请检查 Base URL 是否仍指向官方地址。',
      },
    ],
  },
  {
    key: 'gemini-cli',
    name: 'Gemini CLI',
    titleKey: 'Gemini CLI 接入本站 API',
    categoryKey: '命令行工具',
    statusKey: '已开放',
    summaryKey:
      '面向 Gemini API 兼容命令行或工具，适合使用 /v1beta/models 路径的 Gemini 格式请求。',
    endpointSuffix: '/v1beta',
    tokenRoute: '/console/token',
    logRoute: '/console/log',
    apiFormatKey: 'Gemini API',
    recommendedMethodKey: '手动填写配置',
    highlights: [
      {
        icon: PlugZap,
        labelKey: '接入端点',
        valueKey: 'Base URL 只填到 /v1beta',
      },
      {
        icon: Layers3,
        labelKey: '推荐方式',
        valueKey: '手动填写配置',
      },
      {
        icon: FileTerminal,
        labelKey: '支持接口',
        valueKey: 'Gemini API',
      },
    ],
    steps: [
      {
        titleKey: '创建或复制令牌',
        descriptionKey: '进入控制台令牌页面，确认令牌状态正常且账户余额充足。',
      },
      {
        titleKey: '选择 Gemini 模型',
        descriptionKey:
          '从令牌可用模型中选择 Gemini 可用模型，模型名以本站显示为准。',
      },
      {
        titleKey: '填写 Gemini 接入地址',
        descriptionKey:
          'Base URL 填当前站点域名加 /v1beta，API Key 使用本站令牌。',
      },
      {
        titleKey: '发起测试请求',
        descriptionKey:
          '运行一次 Gemini 格式请求，然后回到本站用量日志确认记录。',
      },
    ],
    configRows: [
      {
        labelKey: 'API Key',
        valueKey: '本站令牌，例如 sk-xxxx',
      },
      {
        labelKey: 'Base URL',
        valueKey: '当前站点域名 + /v1beta',
        copyType: 'baseUrl',
      },
      {
        labelKey: 'Model',
        valueKey: '选择本站支持的 Gemini 模型',
      },
      {
        labelKey: 'API Format',
        valueKey: 'Gemini API',
      },
      {
        labelKey: '主要端点',
        valueKey: '/v1beta/models/{model}:generateContent',
      },
    ],
    checklist: [
      '令牌未禁用，且账户余额充足。',
      'Base URL 使用当前站点域名并以 /v1beta 结尾。',
      '模型名来自本站模型列表或令牌可用模型。',
      '如果工具只支持 OpenAI 格式，请改用 OpenAI 兼容接入方式。',
    ],
    faq: [
      {
        title: 'API key not valid',
        answerKey: 'API Key 错误或未带 sk- 前缀，重新复制令牌。',
      },
      {
        title: '404 Not Found',
        answerKey: 'Base URL 填写错误，确认只填到指定的接入地址。',
      },
      {
        title: 'model not found',
        answerKey: '模型名不匹配，使用本站显示的完整模型名。',
      },
      {
        title: 'unsupported format',
        answerKey:
          '客户端请求格式与当前接入类型不一致，请切换工具的 API 类型。',
      },
    ],
  },
];

const getProviderBaseURL = (provider) => {
  const serverAddress = getServerAddress().replace(/\/+$/, '');
  return `${serverAddress}${provider.endpointSuffix}`;
};

const hasCJKText = (value) => /[\u3400-\u9fff]/.test(String(value || ''));

const IntegrationDocs = () => {
  const { t } = useTranslation();
  const [activeProviderKey, setActiveProviderKey] = useState(
    ACCESS_DOC_PROVIDERS[0].key,
  );

  const activeProvider = useMemo(
    () =>
      ACCESS_DOC_PROVIDERS.find(
        (provider) => provider.key === activeProviderKey,
      ) || ACCESS_DOC_PROVIDERS[0],
    [activeProviderKey],
  );

  const baseURL = getProviderBaseURL(activeProvider);
  const docText = (value) => (hasCJKText(value) ? t(value) : value);

  const handleCopy = async (text) => {
    if (await copy(text)) {
      showSuccess(t('已复制到剪贴板'));
    }
  };

  return (
    <main className='integration-docs-page'>
      <div className='integration-docs-shell'>
        <div className='integration-docs-layout'>
          <aside className='integration-docs-sidebar'>
            <div className='integration-docs-provider-heading'>
              <span>{t('工具列表')}</span>
              <strong>{t('接入文档')}</strong>
            </div>
            <div className='integration-docs-provider-tabs'>
              {ACCESS_DOC_PROVIDERS.map((provider) => (
                <button
                  key={provider.key}
                  type='button'
                  className={
                    provider.key === activeProviderKey
                      ? 'integration-docs-provider-tab active'
                      : 'integration-docs-provider-tab'
                  }
                  aria-pressed={provider.key === activeProviderKey}
                  onClick={() => setActiveProviderKey(provider.key)}
                >
                  <span>
                    <strong>{provider.name}</strong>
                    <em>{docText(provider.categoryKey)}</em>
                  </span>
                  <small>{docText(provider.statusKey)}</small>
                </button>
              ))}
            </div>
            <div className='integration-docs-base-url'>
              <span>{t('Base URL')}</span>
              <code>{baseURL}</code>
              <button
                type='button'
                aria-label={t('复制 Base URL')}
                onClick={() => handleCopy(baseURL)}
              >
                <Clipboard size={15} />
              </button>
            </div>
          </aside>

          <div className='integration-docs-main'>
            <section className='integration-docs-hero'>
              <div className='integration-docs-hero-copy'>
                <div className='integration-docs-eyebrow'>
                  <BookOpenCheck size={16} />
                  <span>{t('接入文档')}</span>
                </div>
                <h1>{docText(activeProvider.titleKey)}</h1>
                <p>{docText(activeProvider.summaryKey)}</p>
                <div className='integration-docs-meta-strip'>
                  <span>{docText(activeProvider.categoryKey)}</span>
                  <span>{docText(activeProvider.apiFormatKey)}</span>
                  <span>{docText(activeProvider.recommendedMethodKey)}</span>
                </div>
                <div className='integration-docs-actions'>
                  <Link
                    className='integration-docs-button primary'
                    to={activeProvider.tokenRoute}
                  >
                    <KeyRound size={16} />
                    <span>{t('打开令牌页面')}</span>
                    <ArrowRight size={16} />
                  </Link>
                  <button
                    className='integration-docs-button secondary'
                    type='button'
                    onClick={() => handleCopy(baseURL)}
                  >
                    <Clipboard size={16} />
                    <span>{t('复制 Base URL')}</span>
                  </button>
                </div>
              </div>
            </section>

            <section className='integration-docs-highlight-grid'>
              {activeProvider.highlights.map((item) => {
                const Icon = item.icon;
                return (
                  <div
                    className='integration-docs-highlight'
                    key={item.labelKey}
                  >
                    <div className='integration-docs-highlight-icon'>
                      <Icon size={18} />
                    </div>
                    <span>{docText(item.labelKey)}</span>
                    <strong>{docText(item.valueKey)}</strong>
                  </div>
                );
              })}
            </section>

            <section className='integration-docs-content-grid'>
              <div className='integration-docs-panel'>
                <div className='integration-docs-section-title'>
                  <Sparkles size={18} />
                  <h2>{t('推荐接入流程')}</h2>
                </div>
                <div className='integration-docs-steps'>
                  {activeProvider.steps.map((step, index) => (
                    <div className='integration-docs-step' key={step.titleKey}>
                      <div className='integration-docs-step-index'>
                        {String(index + 1).padStart(2, '0')}
                      </div>
                      <div>
                        <h3>{docText(step.titleKey)}</h3>
                        <p>{docText(step.descriptionKey)}</p>
                      </div>
                    </div>
                  ))}
                </div>
              </div>

              <div className='integration-docs-panel'>
                <div className='integration-docs-section-title'>
                  <FileTerminal size={18} />
                  <h2>{t('配置填写')}</h2>
                </div>
                <div className='integration-docs-config-table'>
                  {activeProvider.configRows.map((row) => {
                    const value =
                      row.copyType === 'baseUrl'
                        ? baseURL
                        : docText(row.valueKey);
                    return (
                      <div
                        className={
                          row.copyType === 'baseUrl'
                            ? 'integration-docs-config-row has-action'
                            : 'integration-docs-config-row'
                        }
                        key={row.labelKey}
                      >
                        <span>{docText(row.labelKey)}</span>
                        <code>{value}</code>
                        {row.copyType === 'baseUrl' && (
                          <button
                            type='button'
                            aria-label={t('复制 Base URL')}
                            onClick={() => handleCopy(value)}
                          >
                            <Clipboard size={14} />
                          </button>
                        )}
                      </div>
                    );
                  })}
                </div>
              </div>
            </section>

            <section className='integration-docs-content-grid compact'>
              <div className='integration-docs-panel'>
                <div className='integration-docs-section-title'>
                  <ShieldCheck size={18} />
                  <h2>{t('接入检查清单')}</h2>
                </div>
                <div className='integration-docs-check-list'>
                  {activeProvider.checklist.map((item) => (
                    <div className='integration-docs-check-item' key={item}>
                      <CheckCircle2 size={16} />
                      <span>{docText(item)}</span>
                    </div>
                  ))}
                </div>
              </div>

              <div className='integration-docs-panel'>
                <div className='integration-docs-section-title'>
                  <PlugZap size={18} />
                  <h2>{t('常见问题')}</h2>
                </div>
                <div className='integration-docs-faq-list'>
                  {activeProvider.faq.map((item) => (
                    <div className='integration-docs-faq-item' key={item.title}>
                      <code>{item.title}</code>
                      <span>{docText(item.answerKey)}</span>
                    </div>
                  ))}
                </div>
              </div>
            </section>

            <section className='integration-docs-footer-note'>
              <div>
                <strong>{t('注意事项')}</strong>
                <p>
                  {t(
                    'API Key 等同调用权限，请勿公开分享；泄露后请立即删除或重置令牌。',
                  )}
                </p>
              </div>
              <div>
                <strong>{t('扩展说明')}</strong>
                <p>
                  {t(
                    '新增工具时只需追加客户端配置、接入步骤和翻译，页面结构无需重写。',
                  )}
                </p>
              </div>
              <Link
                className='integration-docs-log-link'
                to={activeProvider.logRoute}
              >
                <span>{t('查看用量日志')}</span>
                <ArrowRight size={15} />
              </Link>
            </section>
          </div>
        </div>
      </div>
    </main>
  );
};

export default IntegrationDocs;
