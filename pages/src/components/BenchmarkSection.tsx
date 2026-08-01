import React, { useState, useMemo, useCallback } from 'react';
import { useTranslation } from '../i18n';
import { useResponsive } from '../hooks/useResponsive';
import { useSectionTitleStyle } from '../hooks/useResponsiveStyle';
import ocrSourceIcon from '../assets/images/icon-ocr-source.svg';
import claudeCodeIcon from '../assets/images/icon-claude-code.svg';
import codexIcon from '../assets/images/icon-codex.svg';
import sortIcon from '../assets/icons/icon-sort.svg';
import trophyGold from '../assets/icons/trophy-gold.svg';
import trophySilver from '../assets/icons/trophy-silver.svg';
import trophyBronze from '../assets/icons/trophy-bronze.svg';

const TROPHY_ICONS = [trophyGold, trophySilver, trophyBronze];

interface BenchmarkRow {
  model: string;
  provider: string;
  source: string;
  sourceIcon?: string;
  f1: string;
  f1Value: number;
  precision: string;
  precisionValue: number;
  precisionDetail: string;
  recall: string;
  recallValue: number;
  recallDetail: string;
  avgTime: string;
  avgToken: string;
  tokenDetail: string;
  precisionMedal?: number;  // 0=gold, 1=silver, 2=bronze
  recallMedal?: number;     // 0=gold, 1=silver, 2=bronze
}

type SortField = 'f1' | 'precision' | 'recall';
type SortOrder = 'desc' | 'asc';

const rawRows: BenchmarkRow[] = [
  { model: 'Claude-4.6-Opus', provider: 'Anthropic', source: 'Open Code Review', sourceIcon: ocrSourceIcon, f1: '25.10%', f1Value: 25.10, precision: '33.90%', precisionValue: 33.90, precisionDetail: '301/889', recall: '20.00%', recallValue: 20.00, recallDetail: '301/1505', avgTime: '1m23s', avgToken: '385K', tokenDetail: '375K / 10K', precisionMedal: 1 },
  { model: 'GLM-5.2', provider: 'Zhipu AI', source: 'Open Code Review', sourceIcon: ocrSourceIcon, f1: '21.30%', f1Value: 21.30, precision: '32.30%', precisionValue: 32.30, precisionDetail: '239/741', recall: '15.90%', recallValue: 15.90, recallDetail: '239/1505', avgTime: '7m58s', avgToken: '682K', tokenDetail: '624K / 58K', precisionMedal: 2 },
  { model: 'Qwen3.7-Max', provider: 'Alibaba', source: 'Open Code Review', sourceIcon: ocrSourceIcon, f1: '21.20%', f1Value: 21.20, precision: '25.20%', precisionValue: 25.20, precisionDetail: '276/1096', recall: '18.30%', recallValue: 18.30, recallDetail: '276/1505', avgTime: '4m41s', avgToken: '625K', tokenDetail: '587K / 38K' },
  { model: 'GPT-5.5', provider: 'OpenAI', source: 'Open Code Review', sourceIcon: ocrSourceIcon, f1: '21.00%', f1Value: 21.00, precision: '32.10%', precisionValue: 32.10, precisionDetail: '234/728', recall: '15.50%', recallValue: 15.50, recallDetail: '234/1505', avgTime: '2m51s', avgToken: '422K', tokenDetail: '409K / 13K' },
  { model: 'GLM-5.1', provider: 'Zhipu AI', source: 'Open Code Review', sourceIcon: ocrSourceIcon, f1: '20.40%', f1Value: 20.40, precision: '28.90%', precisionValue: 28.90, precisionDetail: '237/820', recall: '15.70%', recallValue: 15.70, recallDetail: '237/1505', avgTime: '4m11s', avgToken: '743K', tokenDetail: '707K / 36K' },
  { model: 'Claude-4.8-Opus', provider: 'Anthropic', source: 'Open Code Review', sourceIcon: ocrSourceIcon, f1: '17.90%', f1Value: 17.90, precision: '37.80%', precisionValue: 37.80, precisionDetail: '176/465', recall: '11.70%', recallValue: 11.70, recallDetail: '176/1505', avgTime: '1m6s', avgToken: '352K', tokenDetail: '342K / 11K', precisionMedal: 0 },
  { model: 'Deepseek-V4-Pro', provider: 'DeepSeek', source: 'Open Code Review', sourceIcon: ocrSourceIcon, f1: '17.90%', f1Value: 17.90, precision: '30.60%', precisionValue: 30.60, precisionDetail: '191/624', recall: '12.70%', recallValue: 12.70, recallDetail: '191/1505', avgTime: '6m28s', avgToken: '394K', tokenDetail: '350K / 44K' },
  { model: 'Claude-4.8-Opus', provider: 'Anthropic', source: 'Claude Code', sourceIcon: claudeCodeIcon, f1: '14.13%', f1Value: 14.13, precision: '15.93%', precisionValue: 15.93, precisionDetail: '191/1200', recall: '12.70%', recallValue: 12.70, recallDetail: '191/1505', avgTime: '5m38s', avgToken: '2,062K', tokenDetail: '2,039K / 23K' },
  { model: 'Qwen3.7-Max', provider: 'Alibaba', source: 'Claude Code', sourceIcon: claudeCodeIcon, f1: '12.17%', f1Value: 12.17, precision: '8.23%', precisionValue: 8.23, precisionDetail: '351/4260', recall: '23.37%', recallValue: 23.37, recallDetail: '351/1505', avgTime: '8m6s', avgToken: '5,153K', tokenDetail: '5,108K / 44K', recallMedal: 1 },
  { model: 'GLM-5.1', provider: 'Zhipu AI', source: 'Claude Code', sourceIcon: claudeCodeIcon, f1: '11.93%', f1Value: 11.93, precision: '8.37%', precisionValue: 8.37, precisionDetail: '313/3742', recall: '20.80%', recallValue: 20.80, recallDetail: '313/1505', avgTime: '14m10s', avgToken: '4,038K', tokenDetail: '3,998K / 39K', recallMedal: 2 },
  { model: 'Claude-4.6-Opus', provider: 'Anthropic', source: 'Claude Code', sourceIcon: claudeCodeIcon, f1: '11.57%', f1Value: 11.57, precision: '7.23%', precisionValue: 7.23, precisionDetail: '435/5980', recall: '28.90%', recallValue: 28.90, recallDetail: '435/1505', avgTime: '13m6s', avgToken: '5,664K', tokenDetail: '5,603K / 60K', recallMedal: 0 },
  { model: 'Deepseek-V4-Pro', provider: 'DeepSeek', source: 'Claude Code', sourceIcon: claudeCodeIcon, f1: '10.93%', f1Value: 10.93, precision: '8.27%', precisionValue: 8.27, precisionDetail: '243/2945', recall: '16.13%', recallValue: 16.13, recallDetail: '243/1505', avgTime: '14m24s', avgToken: '5,450K', tokenDetail: '5,389K / 60K' },
  { model: 'GPT-5.5', provider: 'OpenAI', source: 'Codex', sourceIcon: codexIcon, f1: '8.36%', f1Value: 8.36, precision: '27.82%', precisionValue: 27.82, precisionDetail: '74/266', recall: '4.92%', recallValue: 4.92, recallDetail: '74/1505', avgTime: '2m58s', avgToken: '525K', tokenDetail: '520K / 5K' },
];

const MEDAL_RANKS = ['🥇', '🥈', '🥉'];

const BenchmarkSection: React.FC = () => {
  const { t } = useTranslation();
  const { isMobile, isTablet } = useResponsive();
  const titleStyle = useSectionTitleStyle();
  const [sortField, setSortField] = useState<SortField>('f1');
  const [sortOrder, setSortOrder] = useState<SortOrder>('desc');

  const sortedRows = useMemo(() => {
    const key = `${sortField}Value` as keyof BenchmarkRow;
    const sorted = [...rawRows].sort((a, b) => {
      const aVal = a[key] as number;
      const bVal = b[key] as number;
      return sortOrder === 'desc' ? bVal - aVal : aVal - bVal;
    });
    return sorted;
  }, [sortField, sortOrder]);

  const handleSort = useCallback((field: SortField) => {
    if (sortField === field) {
      setSortOrder(prev => prev === 'desc' ? 'asc' : 'desc');
    } else {
      setSortField(field);
      setSortOrder('desc');
    }
  }, [sortField]);

  const sortableHeaders: { key: string; label: string; field?: SortField }[] = [
    { key: 'rank', label: t('benchmark.colRank') },
    { key: 'model', label: t('benchmark.colModel') },
    { key: 'source', label: t('benchmark.colSource') },
    { key: 'f1', label: t('benchmark.colF1'), field: 'f1' },
    { key: 'precision', label: t('benchmark.colPrecision'), field: 'precision' },
    { key: 'recall', label: t('benchmark.colRecall'), field: 'recall' },
    { key: 'avgTime', label: t('benchmark.colAvgTime') },
    { key: 'avgToken', label: t('benchmark.colAvgToken') },
  ];

  return (
    <section
      id="benchmark"
      style={{ width: '100%', display: 'flex', justifyContent: 'center', padding: isMobile ? '60px 20px' : isTablet ? '80px 40px' : '80px 0', overflow: 'hidden' }}
    >
      <div style={{ width: '100%', maxWidth: 1200, display: 'flex', flexDirection: 'column', alignItems: 'center', gap: isMobile ? 32 : 48 }}>
        {/* Header */}
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12 }}>
          <span style={{ color: '#2BDE5E', fontSize: 16, fontWeight: 500, letterSpacing: '0.48px' }}>
            {t('benchmark.sectionLabel')}
          </span>
          <h2 style={{ color: '#FFFFFF', fontSize: titleStyle.fontSize, fontWeight: 500, textAlign: 'center', lineHeight: titleStyle.lineHeight, letterSpacing: '0.96px', margin: 0, maxWidth: 758 }}>
            {t('benchmark.title')}
          </h2>
          <p style={{ color: 'rgba(255,255,255,0.5)', fontSize: 16, textAlign: 'center', lineHeight: '24px', margin: 0, maxWidth: 646 }}>
            {t('benchmark.subtitle')}
          </p>
        </div>

        {/* Table */}
        <div style={{ width: '100%', display: 'flex', flexDirection: 'column', border: '1px solid rgba(255,255,255,0.16)', borderRadius: 8, overflow: isMobile ? 'auto' : 'hidden' }}>
          {/* Table Header */}
          <div style={{
            width: 1200,
            minWidth: 1200,
            display: 'flex',
            alignItems: 'flex-start',
            borderStyle: 'solid',
            borderColor: 'rgba(255,255,255,0.16)',
            borderTopWidth: 0,
            borderBottomWidth: 1,
            borderRightWidth: 0,
            borderLeftWidth: 0,
          }}>
            {sortableHeaders.map((col, i) => {
              const isActive = col.field === sortField;
              const isSortable = !!col.field;
              return (
                <div
                  key={col.key}
                  onClick={isSortable ? () => handleSort(col.field!) : undefined}
                  style={{
                    width: i === 0 ? 120 : i === 1 ? 200 : i === 2 ? 200 : undefined,
                    flex: i > 2 ? 1 : undefined,
                    height: 44,
                    display: 'flex',
                    alignItems: 'center',
                    gap: 4,
                    padding: i === 0 ? '10px 20px' : '10px 12px',
                    cursor: isSortable ? 'pointer' : 'default',
                    userSelect: 'none',
                  }}
                >
                  <span style={{
                    color: isActive ? '#2BDE5E' : 'rgba(255,255,255,0.5)',
                    fontSize: 12,
                    fontWeight: 500,
                    letterSpacing: '0.5px',
                    transition: 'color 0.2s ease',
                  }}>
                    {col.label}
                  </span>
                  {isSortable && (
                    <img
                      src={sortIcon}
                      alt=""
                      style={{
                        width: 14,
                        height: 14,
                        opacity: isActive ? 1 : 0.5,
                        transform: isActive && sortOrder === 'asc' ? 'rotate(180deg)' : 'none',
                        transition: 'transform 0.2s ease, opacity 0.2s ease',
                      }}
                    />
                  )}
                </div>
              );
            })}
          </div>

          {/* Table Rows */}
          {sortedRows.map((row, idx) => {
            const bgOpacity = idx === 0 ? 0.25 : idx === 1 ? 0.18 : idx === 2 ? 0.1 : 0;
            const rankDisplay = idx < 3 ? MEDAL_RANKS[idx] : String(idx + 1);
            return (
              <div
                key={`${row.model}-${row.source}`}
                style={{
                  width: 1200,
                  minWidth: 1200,
                  display: 'flex',
                  alignItems: 'center',
                  background: bgOpacity > 0 ? `rgba(43,222,94,${bgOpacity})` : 'transparent',
                  borderStyle: 'solid',
                  borderColor: 'rgba(255,255,255,0.16)',
                  borderTopWidth: 0,
                  borderBottomWidth: idx === sortedRows.length - 1 ? 0 : 1,
                  borderRightWidth: 0,
                  borderLeftWidth: 0,
                  overflow: 'hidden',
                }}
              >
                {/* Rank */}
                <div style={{ width: 120, padding: '10px 20px' }}>
                  <span style={{ color: 'rgba(255,255,255,0.8)', fontSize: idx < 3 ? 18 : 14, fontFamily: 'Menlo, monospace' }}>
                    {rankDisplay}
                  </span>
                </div>
                {/* Model */}
                <div style={{ width: 200, padding: '10px 12px', display: 'flex', flexDirection: 'column' }}>
                  <span style={{ color: '#FFFFFF', fontSize: 14, fontWeight: 500 }}>{row.model}</span>
                  <span style={{ color: 'rgba(255,255,255,0.4)', fontSize: 12 }}>{row.provider}</span>
                </div>
                {/* Source */}
                <div style={{ width: 200, padding: '10px 12px', display: 'flex', alignItems: 'center', gap: 4 }}>
                  {row.sourceIcon && <img src={row.sourceIcon} alt="" style={{ width: 20, height: 20 }} />}
                  <span style={{ color: 'rgba(255,255,255,0.7)', fontSize: 13 }}>{row.source}</span>
                </div>
                {/* F1 */}
                <div style={{ flex: 1, padding: '10px 12px' }}>
                  <span style={{ color: sortField === 'f1' ? '#2BDE5E' : '#FFFFFF', fontSize: 14, fontWeight: sortField === 'f1' ? 600 : 400 }}>
                    {row.f1}
                  </span>
                </div>
                {/* Precision */}
                <div style={{ flex: 1, padding: '10px 12px', display: 'flex', flexDirection: 'column' }}>
                  <div style={{ display: 'flex', alignItems: 'center' }}>
                    <span style={{ color: sortField === 'precision' ? '#2BDE5E' : '#FFFFFF', fontSize: 14, fontWeight: sortField === 'precision' ? 600 : 400 }}>
                      {row.precision}
                    </span>
                    {row.precisionMedal !== undefined && <img src={TROPHY_ICONS[row.precisionMedal]} alt="" style={{ width: 16, height: 16, marginLeft: 4 }} />}
                  </div>
                  <span style={{ color: 'rgba(255,255,255,0.4)', fontSize: 11 }}>{row.precisionDetail}</span>
                </div>
                {/* Recall */}
                <div style={{ flex: 1, padding: '10px 12px', display: 'flex', flexDirection: 'column' }}>
                  <div style={{ display: 'flex', alignItems: 'center' }}>
                    <span style={{ color: sortField === 'recall' ? '#2BDE5E' : '#FFFFFF', fontSize: 14, fontWeight: sortField === 'recall' ? 600 : 400 }}>
                      {row.recall}
                    </span>
                    {row.recallMedal !== undefined && <img src={TROPHY_ICONS[row.recallMedal]} alt="" style={{ width: 16, height: 16, marginLeft: 4 }} />}
                  </div>
                  <span style={{ color: 'rgba(255,255,255,0.4)', fontSize: 11 }}>{row.recallDetail}</span>
                </div>
                {/* Avg Time */}
                <div style={{ flex: 1, padding: '10px 12px' }}>
                  <span style={{ color: '#FFFFFF', fontSize: 14 }}>{row.avgTime}</span>
                </div>
                {/* Avg Token */}
                <div style={{ flex: 1, padding: '10px 12px', display: 'flex', flexDirection: 'column' }}>
                  <span style={{ color: '#FFFFFF', fontSize: 14 }}>{row.avgToken}</span>
                  <span style={{ color: 'rgba(255,255,255,0.4)', fontSize: 11 }}>{row.tokenDetail}</span>
                </div>
              </div>
            );
          })}
        </div>

        {/* Footer note */}
        <p style={{ color: 'rgba(255,255,255,0.3)', fontSize: 12, textAlign: 'center', margin: 0, marginTop: -24 }}>
          Open Code Review · v1.3.1 ｜ Claude Code · v2.1.169 · /code-review ｜ Codex · v0.140.0 · /review
        </p>
      </div>
    </section>
  );
};

export default BenchmarkSection;
