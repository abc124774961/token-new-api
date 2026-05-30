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

import { useState, useCallback, useEffect } from 'react';
import { initVChartSemiTheme } from '@visactor/vchart-semi-theme';
import { renderNumber, renderQuota, getQuotaWithUnit } from '../../helpers';
import {
  processRawData,
  calculateTrendData,
  aggregateDataByTimeAndModel,
  generateChartTimePoints,
  updateChartSpec,
  updateMapValue,
  initializeMaps,
  processUserData,
} from '../../helpers/dashboard';

const DASHBOARD_MODEL_PALETTE = [
  '#8aa4c0',
  '#6f86a3',
  '#a0aec0',
  '#7891b2',
  '#94a3b8',
  '#64748b',
  '#9aa8b8',
  '#7c8fa6',
  '#b6c2d1',
  '#5f7590',
];

const DASHBOARD_MODEL_COLOR_RULES = [
  { pattern: /gpt[-_.\s]?5[-_.\s]?5|5\.5/i, color: '#8aa4c0' },
  { pattern: /gpt[-_.\s]?5[-_.\s]?4|5\.4/i, color: '#6f86a3' },
  { pattern: /gpt[-_.\s]?4|o[34]/i, color: '#a0aec0' },
  { pattern: /claude/i, color: '#c2a878' },
  { pattern: /gemini/i, color: '#9f96b5' },
  { pattern: /deepseek/i, color: '#8db9a4' },
  { pattern: /qwen|通义/i, color: '#7891b2' },
  { pattern: /llama|meta/i, color: '#9aa8b8' },
  { pattern: /mistral/i, color: '#c49b77' },
];

const USER_COLORS = [
  '#8aa4c0',
  '#6f86a3',
  '#a0aec0',
  '#7891b2',
  '#94a3b8',
  '#64748b',
  '#9aa8b8',
  '#7c8fa6',
  '#b6c2d1',
  '#5f7590',
];

const getStablePaletteColor = (value, palette = DASHBOARD_MODEL_PALETTE) => {
  const source = String(value || '');
  let hash = 0;
  for (let i = 0; i < source.length; i++) {
    hash = (hash << 5) - hash + source.charCodeAt(i);
    hash |= 0;
  }
  return palette[Math.abs(hash) % palette.length];
};

const getDashboardModelColor = (modelName, fallbackColor) => {
  if (fallbackColor) {
    return fallbackColor;
  }
  const matchedRule = DASHBOARD_MODEL_COLOR_RULES.find(({ pattern }) =>
    pattern.test(String(modelName || '')),
  );
  return matchedRule?.color || getStablePaletteColor(modelName);
};

const DASHBOARD_GRID_STYLE = {
  stroke: 'rgba(148, 163, 184, 0.07)',
  lineDash: [],
  lineWidth: 1,
};

const DASHBOARD_AXIS_LABEL = {
  visible: true,
  autoHide: true,
  autoRotate: true,
  style: {
    fontSize: 11,
    fontWeight: 500,
  },
};

const DASHBOARD_TITLE_STYLE = {
  padding: {
    bottom: 12,
  },
  textStyle: {
    fontSize: 14,
    fontWeight: 800,
    lineHeight: 20,
  },
  subtextStyle: {
    fontSize: 12,
    fontWeight: 560,
    lineHeight: 18,
  },
};

const DASHBOARD_LEGEND_STYLE = {
  visible: true,
  orient: 'bottom',
  position: 'middle',
  padding: {
    top: 12,
  },
  item: {
    spaceRow: 8,
    spaceCol: 14,
    shape: {
      style: {
        size: 8,
        symbolType: 'circle',
      },
    },
    label: {
      style: {
        fontSize: 11,
        fontWeight: 620,
      },
    },
  },
};

const DASHBOARD_TOOLTIP_STYLE = {
  panel: {
    backgroundColor: 'rgba(18, 25, 38, 0.96)',
    border: {
      color: 'rgba(148, 163, 184, 0.12)',
      width: 0,
    },
    borderRadius: 12,
    shadow: 'none',
  },
  titleLabel: {
    fill: '#f8fafc',
    fontSize: 12,
    fontWeight: 700,
  },
  keyLabel: {
    fill: '#cbd5e1',
    fontSize: 12,
  },
  valueLabel: {
    fill: '#f8fafc',
    fontSize: 12,
    fontWeight: 700,
  },
};

const DASHBOARD_CHART_SURFACE = {
  background: 'transparent',
  region: [
    {
      style: {
        fill: 'transparent',
        fillOpacity: 0,
      },
    },
  ],
};

const withTooltipStyle = (config) => ({
  ...config,
  style: DASHBOARD_TOOLTIP_STYLE,
});

const createDashboardTitle = (text, subtext = '') => ({
  visible: true,
  text,
  subtext,
  ...DASHBOARD_TITLE_STYLE,
});

const createDashboardLegend = (overrides = {}) => ({
  ...DASHBOARD_LEGEND_STYLE,
  ...overrides,
  item: {
    ...DASHBOARD_LEGEND_STYLE.item,
    ...(overrides.item || {}),
    shape: {
      ...DASHBOARD_LEGEND_STYLE.item.shape,
      ...(overrides.item?.shape || {}),
      style: {
        ...DASHBOARD_LEGEND_STYLE.item.shape.style,
        ...(overrides.item?.shape?.style || {}),
      },
    },
    label: {
      ...DASHBOARD_LEGEND_STYLE.item.label,
      ...(overrides.item?.label || {}),
      style: {
        ...DASHBOARD_LEGEND_STYLE.item.label.style,
        ...(overrides.item?.label?.style || {}),
      },
    },
  },
});

const createCartesianAxes = (bottomOverrides = {}, leftOverrides = {}) => [
  {
    orient: 'bottom',
    label: DASHBOARD_AXIS_LABEL,
    tick: { visible: false },
    domainLine: { visible: false },
    ...bottomOverrides,
  },
  {
    orient: 'left',
    label: DASHBOARD_AXIS_LABEL,
    grid: {
      visible: true,
      style: DASHBOARD_GRID_STYLE,
    },
    tick: { visible: false },
    domainLine: { visible: false },
    ...leftOverrides,
  },
];

const DASHBOARD_BAR_STYLE = {
  style: {
    cornerRadius: 5,
  },
  state: {
    hover: {
      lineWidth: 0,
      fillOpacity: 0.92,
    },
  },
};

export const useDashboardCharts = (
  dataExportDefaultTime,
  setTrendData,
  setConsumeQuota,
  setTimes,
  setConsumeTokens,
  setPieData,
  setLineData,
  setModelColors,
  t,
) => {
  // ========== 图表规格状态 ==========
  const [spec_pie, setSpecPie] = useState({
    type: 'pie',
    ...DASHBOARD_CHART_SURFACE,
    data: [
      {
        id: 'id0',
        values: [{ type: 'null', value: '0' }],
      },
    ],
    outerRadius: 0.8,
    innerRadius: 0.5,
    padAngle: 0.6,
    valueField: 'value',
    categoryField: 'type',
    pie: {
      style: {
        cornerRadius: 10,
      },
      state: {
        hover: {
          outerRadius: 0.85,
          stroke: '#5eead4',
          lineWidth: 1,
        },
        selected: {
          outerRadius: 0.85,
          stroke: '#5eead4',
          lineWidth: 1,
        },
      },
    },
    title: createDashboardTitle(
      t('模型调用次数占比'),
      `${t('总计')}：${renderNumber(0)}`,
    ),
    legends: createDashboardLegend({
      orient: 'right',
      position: 'middle',
      padding: {
        left: 14,
      },
    }),
    label: {
      visible: true,
      style: {
        fontSize: 11,
        fontWeight: 620,
      },
    },
    tooltip: withTooltipStyle({
      mark: {
        content: [
          {
            key: (datum) => datum['type'],
            value: (datum) => renderNumber(datum['value']),
          },
        ],
      },
    }),
    color: {
      type: 'ordinal',
      range: DASHBOARD_MODEL_PALETTE,
    },
  });

  const [spec_line, setSpecLine] = useState({
    type: 'bar',
    ...DASHBOARD_CHART_SURFACE,
    data: [
      {
        id: 'barData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Usage',
    seriesField: 'Model',
    stack: true,
    padding: {
      top: 8,
      right: 18,
      bottom: 4,
      left: 4,
    },
    axes: createCartesianAxes(),
    legends: createDashboardLegend({
      selectMode: 'single',
    }),
    title: createDashboardTitle(
      t('模型消耗分布'),
      `${t('总计')}：${renderQuota(0, 2)}`,
    ),
    bar: DASHBOARD_BAR_STYLE,
    tooltip: withTooltipStyle({
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => datum['rawQuota'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            if (array[i].key == '其他') {
              continue;
            }
            let value = parseFloat(array[i].value);
            if (isNaN(value)) {
              value = 0;
            }
            if (array[i].datum && array[i].datum.TimeSum) {
              sum = array[i].datum.TimeSum;
            }
            array[i].value = renderQuota(value, 4);
          }
          array.unshift({
            key: t('总计'),
            value: renderQuota(sum, 4),
          });
          return array;
        },
      },
    }),
    color: {
      type: 'ordinal',
      range: DASHBOARD_MODEL_PALETTE,
    },
  });

  const [spec_model_line, setSpecModelLine] = useState({
    type: 'line',
    ...DASHBOARD_CHART_SURFACE,
    data: [
      {
        id: 'lineData',
        values: [],
      },
    ],
    xField: 'Time',
    yField: 'Count',
    seriesField: 'Model',
    padding: {
      top: 8,
      right: 18,
      bottom: 4,
      left: 4,
    },
    axes: createCartesianAxes(),
    legends: createDashboardLegend({
      selectMode: 'single',
    }),
    title: createDashboardTitle(t('调用趋势')),
    line: {
      style: {
        lineWidth: 2.4,
      },
      state: {
        hover: {
          lineWidth: 3,
        },
      },
    },
    point: {
      visible: false,
      state: {
        hover: {
          size: 7,
          lineWidth: 2,
          stroke: '#0f172a',
        },
      },
    },
    tooltip: withTooltipStyle({
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => datum['Count'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            let value = parseFloat(array[i].value);
            if (isNaN(value)) value = 0;
            sum += value;
            array[i].value = renderNumber(value);
          }
          array.unshift({
            key: t('总计'),
            value: renderNumber(sum),
          });
          return array;
        },
      },
    }),
    color: {
      type: 'ordinal',
      range: DASHBOARD_MODEL_PALETTE,
    },
  });

  const [spec_rank_bar, setSpecRankBar] = useState({
    type: 'bar',
    ...DASHBOARD_CHART_SURFACE,
    data: [
      {
        id: 'rankData',
        values: [],
      },
    ],
    xField: 'Model',
    yField: 'Count',
    seriesField: 'Model',
    padding: {
      top: 8,
      right: 18,
      bottom: 4,
      left: 4,
    },
    axes: createCartesianAxes(
      {
        label: {
          ...DASHBOARD_AXIS_LABEL,
          style: {
            ...DASHBOARD_AXIS_LABEL.style,
            fontSize: 10,
          },
        },
      },
      {},
    ),
    legends: { visible: false },
    title: createDashboardTitle(t('模型调用次数排行')),
    bar: DASHBOARD_BAR_STYLE,
    tooltip: withTooltipStyle({
      mark: {
        content: [
          {
            key: (datum) => datum['Model'],
            value: (datum) => renderNumber(datum['Count']),
          },
        ],
      },
    }),
    color: {
      type: 'ordinal',
      range: DASHBOARD_MODEL_PALETTE,
    },
  });

  // ========== Admin: 用户消耗排行 ==========
  const [spec_user_rank, setSpecUserRank] = useState({
    type: 'bar',
    ...DASHBOARD_CHART_SURFACE,
    data: [{ id: 'userRankData', values: [] }],
    xField: 'rawQuota',
    yField: 'User',
    seriesField: 'User',
    direction: 'horizontal',
    padding: {
      top: 8,
      right: 18,
      bottom: 4,
      left: 4,
    },
    legends: { visible: false },
    title: createDashboardTitle(t('用户消耗排行')),
    bar: DASHBOARD_BAR_STYLE,
    label: {
      visible: true,
      position: 'outside',
      formatMethod: (value, datum) => renderQuota(datum['rawQuota'] || 0, 2),
    },
    axes: [
      {
        orient: 'left',
        type: 'band',
        label: DASHBOARD_AXIS_LABEL,
        tick: { visible: false },
        domainLine: { visible: false },
      },
      {
        orient: 'bottom',
        type: 'linear',
        visible: false,
      },
    ],
    tooltip: withTooltipStyle({
      mark: {
        content: [
          {
            key: (datum) => datum['User'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
    }),
    color: { type: 'ordinal', range: USER_COLORS },
  });

  // ========== Admin: 用户消耗趋势 ==========
  const [spec_user_trend, setSpecUserTrend] = useState({
    type: 'area',
    ...DASHBOARD_CHART_SURFACE,
    data: [{ id: 'userTrendData', values: [] }],
    xField: 'Time',
    yField: 'rawQuota',
    seriesField: 'User',
    stack: false,
    padding: {
      top: 8,
      right: 18,
      bottom: 4,
      left: 4,
    },
    legends: createDashboardLegend({ selectMode: 'single' }),
    title: createDashboardTitle(t('用户消耗趋势')),
    axes: createCartesianAxes(
      {},
      {
        label: {
          ...DASHBOARD_AXIS_LABEL,
          formatMethod: (value) => renderQuota(value, 2),
        },
      },
    ),
    area: { style: { fillOpacity: 0.12 } },
    line: { style: { lineWidth: 2.4 } },
    point: { visible: false },
    tooltip: withTooltipStyle({
      mark: {
        content: [
          {
            key: (datum) => datum['User'],
            value: (datum) => renderQuota(datum['rawQuota'] || 0, 4),
          },
        ],
      },
      dimension: {
        content: [
          {
            key: (datum) => datum['User'],
            value: (datum) => datum['rawQuota'] || 0,
          },
        ],
        updateContent: (array) => {
          array.sort((a, b) => b.value - a.value);
          let sum = 0;
          for (let i = 0; i < array.length; i++) {
            let value = parseFloat(array[i].value);
            if (isNaN(value)) value = 0;
            sum += value;
            array[i].value = renderQuota(value, 4);
          }
          array.unshift({
            key: t('总计'),
            value: renderQuota(sum, 4),
          });
          return array;
        },
      },
    }),
    color: { type: 'ordinal', range: USER_COLORS },
  });

  // ========== 数据处理函数 ==========
  const generateModelColors = useCallback((uniqueModels, modelColors) => {
    const newModelColors = {};
    Array.from(uniqueModels).forEach((modelName) => {
      newModelColors[modelName] =
        modelColors[modelName] || getDashboardModelColor(modelName);
    });
    return newModelColors;
  }, []);

  const updateChartData = useCallback(
    (data, overrideDefaultTime) => {
      const currentDefaultTime = overrideDefaultTime || dataExportDefaultTime;
      const processedData = processRawData(
        data,
        currentDefaultTime,
        initializeMaps,
        updateMapValue,
      );

      const {
        totalQuota,
        totalTimes,
        totalTokens,
        uniqueModels,
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
      } = processedData;

      const trendDataResult = calculateTrendData(
        timePoints,
        timeQuotaMap,
        timeTokensMap,
        timeCountMap,
        currentDefaultTime,
      );
      setTrendData(trendDataResult);

      const newModelColors = generateModelColors(uniqueModels, {});
      newModelColors[t('其他')] = '#94a3b8';
      setModelColors(newModelColors);

      const aggregatedData = aggregateDataByTimeAndModel(
        data,
        currentDefaultTime,
      );

      const modelTotals = new Map();
      for (let [_, value] of aggregatedData) {
        updateMapValue(modelTotals, value.model, value.count);
      }

      const newPieData = Array.from(modelTotals)
        .map(([model, count]) => ({
          type: model,
          value: count,
        }))
        .sort((a, b) => b.value - a.value);

      const chartTimePoints = generateChartTimePoints(
        aggregatedData,
        data,
        dataExportDefaultTime,
      );

      let newLineData = [];

      chartTimePoints.forEach((time) => {
        let timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            rawQuota: aggregated?.quota || 0,
            Usage: aggregated?.quota
              ? getQuotaWithUnit(aggregated.quota, 4)
              : 0,
          };
        });

        const timeSum = timeData.reduce((sum, item) => sum + item.rawQuota, 0);
        timeData.sort((a, b) => b.rawQuota - a.rawQuota);
        timeData = timeData.map((item) => ({ ...item, TimeSum: timeSum }));
        newLineData.push(...timeData);
      });

      newLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      updateChartSpec(
        setSpecPie,
        newPieData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'id0',
      );

      updateChartSpec(
        setSpecLine,
        newLineData,
        `${t('总计')}：${renderQuota(totalQuota, 2)}`,
        newModelColors,
        'barData',
      );

      // ===== 模型调用次数折线图 =====
      let modelLineData = [];
      chartTimePoints.forEach((time) => {
        const timeData = Array.from(uniqueModels).map((model) => {
          const key = `${time}-${model}`;
          const aggregated = aggregatedData.get(key);
          return {
            Time: time,
            Model: model,
            Count: aggregated?.count || 0,
          };
        });
        modelLineData.push(...timeData);
      });
      modelLineData.sort((a, b) => a.Time.localeCompare(b.Time));

      // ===== 模型调用次数排行柱状图 =====
      const MAX_RANK_MODELS = 20;
      const allRankData = Array.from(modelTotals)
        .map(([model, count]) => ({
          Model: model,
          Count: count,
        }))
        .sort((a, b) => b.Count - a.Count);

      let rankData;
      if (allRankData.length > MAX_RANK_MODELS) {
        const topModels = allRankData.slice(0, MAX_RANK_MODELS);
        const otherCount = allRankData
          .slice(MAX_RANK_MODELS)
          .reduce((sum, item) => sum + item.Count, 0);
        rankData = [...topModels, { Model: t('其他'), Count: otherCount }];
      } else {
        rankData = allRankData;
      }

      updateChartSpec(
        setSpecModelLine,
        modelLineData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'lineData',
      );

      updateChartSpec(
        setSpecRankBar,
        rankData,
        `${t('总计')}：${renderNumber(totalTimes)}`,
        newModelColors,
        'rankData',
      );

      setPieData(newPieData);
      setLineData(newLineData);
      setConsumeQuota(totalQuota);
      setTimes(totalTimes);
      setConsumeTokens(totalTokens);
    },
    [
      dataExportDefaultTime,
      setTrendData,
      generateModelColors,
      setModelColors,
      setPieData,
      setLineData,
      setConsumeQuota,
      setTimes,
      setConsumeTokens,
      t,
    ],
  );

  // ========== 用户维度图表数据处理 ==========
  const updateUserChartData = useCallback(
    (data, overrideDefaultTime) => {
      const currentDefaultTime = overrideDefaultTime || dataExportDefaultTime;
      const { rankingData, trendData: userTrend } = processUserData(
        data,
        currentDefaultTime,
        10,
      );

      const userRankValues = rankingData
        .map((item) => ({
          User: item.User,
          rawQuota: item.Quota,
          Quota: getQuotaWithUnit(item.Quota, 4),
        }))
        .sort((a, b) => b.rawQuota - a.rawQuota);

      const totalUserQuota = rankingData.reduce((s, i) => s + i.Quota, 0);

      setSpecUserRank((prev) => ({
        ...prev,
        data: [{ id: 'userRankData', values: userRankValues }],
        title: {
          ...prev.title,
          subtext: `${t('总计')}：${renderQuota(totalUserQuota, 2)}`,
        },
      }));

      const userTrendValues = userTrend.map((item) => ({
        Time: item.Time,
        User: item.User,
        rawQuota: item.Quota,
        Usage: item.Quota ? getQuotaWithUnit(item.Quota, 4) : 0,
      }));

      setSpecUserTrend((prev) => ({
        ...prev,
        data: [{ id: 'userTrendData', values: userTrendValues }],
        title: {
          ...prev.title,
          subtext: `${t('总计')}：${renderQuota(totalUserQuota, 2)}`,
        },
      }));
    },
    [dataExportDefaultTime, t],
  );

  // ========== 初始化图表主题 ==========
  useEffect(() => {
    initVChartSemiTheme({
      isWatchingThemeSwitch: true,
    });
  }, []);

  return {
    spec_pie,
    spec_line,
    spec_model_line,
    spec_rank_bar,
    spec_user_rank,
    spec_user_trend,
    updateChartData,
    updateUserChartData,
    generateModelColors,
  };
};
