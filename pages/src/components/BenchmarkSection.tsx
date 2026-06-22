import React, { useState } from 'react';
import { useTranslation } from '../i18n';
import { OcrIcon, ClaudeIcon, OpenAIIcon } from './icons';

interface BenchmarkEntry {
  model: string;
  company: string;
  sourceType: 'ocr' | 'cc' | 'codex';
  version: string;
  precision?: number;
  precisionDetail?: string;
  recall?: number;
  recallDetail?: string;
  f1?: number;
  avgTime?: string;
  avgInputToken?: string;
  avgOutputToken?: string;
  avgTotalToken?: string;
}

const OCR_VERSION = 'v1.3.1';
const CC_VERSION = 'v2.1.169';
const CODEX_VERSION = 'v0.140.0';

const sourceColorMap: Record<string, string> = {
  ocr: 'text-brand-400',
  cc: 'text-[#D97757]',
  codex: 'text-[#10a37f]',
};
const sourceNameMap: Record<string, string> = {
  ocr: 'Open Code Review',
  cc: 'Claude Code',
  codex: 'Codex',
};

const benchmarkData: BenchmarkEntry[] = [
  {
    model: 'Claude-4.6-Opus',
    company: 'Anthropic',
    sourceType: 'ocr',
    version: OCR_VERSION,
    precision: 33.90,
    precisionDetail: '301/889',
    recall: 20.00,
    recallDetail: '301/1505',
    f1: 25.10,
    avgTime: '1m23s',
    avgInputToken: '375K',
    avgOutputToken: '10K',
    avgTotalToken: '385K',
  },
  {
    model: 'Qwen3.7-Max',
    company: 'Alibaba',
    sourceType: 'ocr',
    version: OCR_VERSION,
    precision: 25.20,
    precisionDetail: '276/1096',
    recall: 18.30,
    recallDetail: '276/1505',
    f1: 21.20,
    avgTime: '4m41s',
    avgInputToken: '587K',
    avgOutputToken: '38K',
    avgTotalToken: '625K',
  },
  {
    model: 'GPT-5.5',
    company: 'OpenAI',
    sourceType: 'ocr',
    version: OCR_VERSION,
    precision: 32.10,
    precisionDetail: '234/728',
    recall: 15.50,
    recallDetail: '234/1505',
    f1: 21.00,
    avgTime: '2m51s',
    avgInputToken: '409K',
    avgOutputToken: '13K',
    avgTotalToken: '422K',
  },
  {
    model: 'Claude-4.8-Opus',
    company: 'Anthropic',
    sourceType: 'ocr',
    version: OCR_VERSION,
    precision: 37.80,
    precisionDetail: '176/465',
    recall: 11.70,
    recallDetail: '176/1505',
    f1: 17.90,
    avgTime: '1m6s',
    avgInputToken: '342K',
    avgOutputToken: '11K',
    avgTotalToken: '352K',
  },
  {
    model: 'Deepseek-V4-Pro',
    company: 'DeepSeek',
    sourceType: 'ocr',
    version: OCR_VERSION,
    precision: 30.60,
    precisionDetail: '191/624',
    recall: 12.70,
    recallDetail: '191/1505',
    f1: 17.90,
    avgTime: '6m28s',
    avgInputToken: '350K',
    avgOutputToken: '44K',
    avgTotalToken: '394K',
  },
  {
    model: 'GLM-5.1',
    company: 'Zhipu AI',
    sourceType: 'ocr',
    version: OCR_VERSION,
    precision: 28.90,
    precisionDetail: '237/820',
    recall: 15.70,
    recallDetail: '237/1505',
    f1: 20.40,
    avgTime: '4m11s',
    avgInputToken: '707K',
    avgOutputToken: '36K',
    avgTotalToken: '743K',
  },
  {
    model: 'Claude-4.6-Opus',
    company: 'Anthropic',
    sourceType: 'cc',
    version: CC_VERSION,
    precision: 7.23,
    precisionDetail: '435/5980',
    recall: 28.90,
    recallDetail: '435/1505',
    f1: 11.57,
    avgTime: '13m6s',
    avgInputToken: '5603K',
    avgOutputToken: '60K',
    avgTotalToken: '5664K',
  },
  {
    model: 'Qwen3.7-Max',
    company: 'Alibaba',
    sourceType: 'cc',
    version: CC_VERSION,
    precision: 8.23,
    precisionDetail: '351/4260',
    recall: 23.37,
    recallDetail: '351/1505',
    f1: 12.17,
    avgTime: '8m6s',
    avgInputToken: '5108K',
    avgOutputToken: '44K',
    avgTotalToken: '5153K',
  },
  {
    model: 'Claude-4.8-Opus',
    company: 'Anthropic',
    sourceType: 'cc',
    version: CC_VERSION,
    precision: 15.93,
    precisionDetail: '191/1200',
    recall: 12.70,
    recallDetail: '191/1505',
    f1: 14.13,
    avgTime: '5m38s',
    avgInputToken: '2,039K',
    avgOutputToken: '23K',
    avgTotalToken: '2,062K',
  },
  {
    model: 'Deepseek-V4-Pro',
    company: 'DeepSeek',
    sourceType: 'cc',
    version: CC_VERSION,
    precision: 8.27,
    precisionDetail: '243/2945',
    recall: 16.13,
    recallDetail: '243/1505',
    f1: 10.93,
    avgTime: '14m24s',
    avgInputToken: '5389K',
    avgOutputToken: '60K',
    avgTotalToken: '5450K',
  },
  {
    model: 'GLM-5.1',
    company: 'Zhipu AI',
    sourceType: 'cc',
    version: CC_VERSION,
    precision: 8.37,
    precisionDetail: '313/3742',
    recall: 20.80,
    recallDetail: '313/1505',
    f1: 11.93,
    avgTime: '14m10s',
    avgInputToken: '3,998K',
    avgOutputToken: '39K',
    avgTotalToken: '4,038K',
  },
  {
    model: 'GPT-5.5',
    company: 'OpenAI',
    sourceType: 'codex',
    version: CODEX_VERSION,
    precision: 27.82,
    precisionDetail: '74/266',
    recall: 4.92,
    recallDetail: '74/1505',
    f1: 8.36,
    avgTime: '2m58s',
    avgInputToken: '520K',
    avgOutputToken: '5K',
    avgTotalToken: '525K',
  },
];

const medalIcons: Record<string, string> = {
  gold: '🥇',
  silver: '🥈',
  bronze: '🥉',
};

function computeMedals(
  entries: BenchmarkEntry[],
  field: 'precision' | 'recall'
): Map<number, string> {
  const indexed = entries
    .map((e, i) => ({ index: i, value: e[field] }))
    .filter((e): e is { index: number; value: number } => e.value !== undefined)
    .sort((a, b) => b.value - a.value);

  const medals = new Map<number, string>();
  const types = ['gold', 'silver', 'bronze'];
  indexed.slice(0, 3).forEach((item, i) => {
    medals.set(item.index, types[i]);
  });
  return medals;
}

type SortField = 'f1' | 'precision' | 'recall';

const BenchmarkSection: React.FC = () => {
  const [hoveredRow, setHoveredRow] = useState<number | null>(null);
  const [sortField, setSortField] = useState<SortField>('f1');
  const { t } = useTranslation();

  const sortedData = [...benchmarkData].sort((a, b) => {
    const av = a[sortField];
    const bv = b[sortField];
    if (av == null && bv == null) return 0;
    if (av == null) return 1;
    if (bv == null) return -1;
    return bv - av;
  });

  const precisionMedals = computeMedals(sortedData, 'precision');
  const recallMedals = computeMedals(sortedData, 'recall');

  const ranks = new Map<number, number>();
  let rank = 0;
  sortedData.forEach((entry, index) => {
    if (entry[sortField] != null) {
      rank++;
      ranks.set(index, rank);
    }
  });

  return (
    <section id="benchmark" className="py-24 relative noise-overlay">
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 w-[800px] h-[600px] rounded-full bg-brand-500/[0.02] blur-[140px] pointer-events-none"></div>

      <div className="relative z-10">
        <div className="section-divider mb-24"></div>
        <div className="max-w-7xl mx-auto px-6">
          <div className="text-center mb-12">
            <p className="text-slate-500 text-sm font-mono uppercase tracking-widest mb-3">
              {t('benchmark.sectionLabel')}
            </p>
            <h2 className="text-4xl font-bold text-white mb-4">
              {t('benchmark.title')}
            </h2>
            <p className="text-slate-400 max-w-4xl mx-auto">
              {t('benchmark.subtitlePreRepos')}
              <span className="text-white font-semibold">50</span>
              {t('benchmark.subtitlePrePRs')}
              <span className="text-white font-semibold">200</span>
              {t('benchmark.subtitlePreLangs')}
              <span className="text-white font-semibold">10</span>
              {t('benchmark.subtitleEnd')}
            </p>
          </div>

          {/* Legend */}
          <div className="mb-6 flex items-center gap-5 flex-wrap">
            <div className="flex items-center gap-1.5 text-xs">
              <OcrIcon className="w-4 h-4 rounded-sm" />
              <span className="text-brand-400 font-medium">Open Code Review · {OCR_VERSION}</span>
            </div>
            <div className="flex items-center gap-1.5 text-xs">
              <ClaudeIcon className="w-4 h-4" />
              <span className="text-[#D97757] font-medium">Claude Code · {CC_VERSION} · /code-review</span>
            </div>
            <div className="flex items-center gap-1.5 text-xs">
              <OpenAIIcon className="w-4 h-4" />
              <span className="text-[#10a37f] font-medium">Codex · {CODEX_VERSION} · /review</span>
            </div>
          </div>

          {/* Table */}
          <div className="rounded-2xl overflow-hidden glass-strong gradient-border shadow-2xl shadow-black/30">
            {/* Header */}
            <div className="grid grid-cols-[2.5rem_10rem_11rem_repeat(5,1fr)] gap-2 px-6 py-3 bg-dark-700/60 text-xs font-medium text-slate-500 uppercase tracking-wider">
              <div>{t('benchmark.colRank')}</div>
              <div>{t('benchmark.colModel')}</div>
              <div>{t('benchmark.colSource')}</div>
              {(['f1', 'precision', 'recall'] as const).map((field) => {
                const labels: Record<SortField, string> = {
                  f1: 'F1',
                  precision: t('benchmark.colPrecision'),
                  recall: t('benchmark.colRecall'),
                };
                return (
                  <div
                    key={field}
                    className={`cursor-pointer select-none transition-colors hover:text-slate-300 ${sortField === field ? 'text-brand-400' : ''}`}
                    onClick={() => setSortField(field)}
                  >
                    {labels[field]}
                    <span className={`ml-1 ${sortField === field ? 'opacity-100' : 'opacity-30'}`}>▼</span>
                  </div>
                );
              })}
              <div>{t('benchmark.colAvgTime')}</div>
              <div>{t('benchmark.colAvgToken')}</div>
            </div>

            {/* Rows */}
            {sortedData.map((entry, index) => {
              const entryRank = ranks.get(index);
              const hasData = entry.f1 != null;
              const pMedal = precisionMedals.get(index);
              const rMedal = recallMedals.get(index);

              return (
                <div
                  key={`${entry.model}-${entry.sourceType}`}
                  className={`leaderboard-row grid grid-cols-[2.5rem_10rem_11rem_repeat(5,1fr)] gap-2 px-6 py-4 items-center cursor-default ${
                    hasData && entry.sourceType === 'ocr' ? 'bg-brand-500/3' : ''
                  } ${hoveredRow === index ? 'bg-brand-500/6' : ''}`}
                  onMouseEnter={() => setHoveredRow(index)}
                  onMouseLeave={() => setHoveredRow(null)}
                >
                  {/* Rank */}
                  <div>
                    {entryRank != null ? (
                      <span className="text-lg w-6 inline-block text-center">
                        {entryRank <= 3 ? medalIcons[['gold', 'silver', 'bronze'][entryRank - 1]] : (
                          <span className="text-slate-500 font-mono text-sm">{entryRank}</span>
                        )}
                      </span>
                    ) : (
                      <span className="w-6 inline-flex items-center justify-center">
                        <span className="w-3.5 h-3.5 rounded-full border-2 border-slate-600 border-t-brand-400 animate-spin" />
                      </span>
                    )}
                  </div>

                  {/* Model + Company */}
                  <div>
                    <div className="text-white text-sm font-medium">{entry.model}</div>
                    <div className="text-slate-500 text-xs mt-0.5">{entry.company}</div>
                  </div>

                  {/* Source */}
                  <div className="flex items-center gap-2">
                    {entry.sourceType === 'ocr' && <OcrIcon className="w-5 h-5 rounded shrink-0" />}
                    {entry.sourceType === 'cc' && <ClaudeIcon className="w-5 h-5 shrink-0" />}
                    {entry.sourceType === 'codex' && <OpenAIIcon className="w-5 h-5 shrink-0" />}
                    <span className={`text-xs whitespace-nowrap ${sourceColorMap[entry.sourceType]}`}>
                      {sourceNameMap[entry.sourceType]}
                    </span>
                  </div>

                  {/* F1 */}
                  <div>
                    {entry.f1 != null ? (
                      <span
                        className={`text-sm font-bold ${
                          sortField === 'f1' && entryRank != null && entryRank <= 3 ? 'text-brand-400' : 'text-white'
                        }`}
                      >
                        {entry.f1.toFixed(2)}%
                      </span>
                    ) : (
                      <span className="text-slate-500 text-xs font-medium flex items-center gap-1.5">
                        <span className="inline-block w-1.5 h-1.5 rounded-full bg-brand-400/60 animate-pulse" />
                        Running
                      </span>
                    )}
                  </div>

                  {/* Precision with medal */}
                  <div>
                    {entry.precision != null ? (
                      <div>
                        <div className="inline-flex items-center gap-1">
                          <span className="text-slate-300 text-sm">{entry.precision.toFixed(2)}%</span>
                          <span className="w-5 inline-block text-center text-lg leading-none">
                            {pMedal ? medalIcons[pMedal] : ''}
                          </span>
                        </div>
                        {entry.precisionDetail && (
                          <div className="text-slate-600 text-xs font-mono mt-0.5">{entry.precisionDetail}</div>
                        )}
                      </div>
                    ) : (
                      <div className="h-4 w-16 rounded bg-slate-800 overflow-hidden relative">
                        <div className="absolute inset-0 bg-gradient-to-r from-transparent via-slate-700/50 to-transparent animate-[shimmer_2s_ease-in-out_infinite]" />
                      </div>
                    )}
                  </div>

                  {/* Recall with medal */}
                  <div>
                    {entry.recall != null ? (
                      <div>
                        <div className="inline-flex items-center gap-1">
                          <span className="text-slate-300 text-sm">{entry.recall.toFixed(2)}%</span>
                          <span className="w-5 inline-block text-center text-lg leading-none">
                            {rMedal ? medalIcons[rMedal] : ''}
                          </span>
                        </div>
                        {entry.recallDetail && (
                          <div className="text-slate-600 text-xs font-mono mt-0.5">{entry.recallDetail}</div>
                        )}
                      </div>
                    ) : (
                      <div className="h-4 w-16 rounded bg-slate-800 overflow-hidden relative">
                        <div className="absolute inset-0 bg-gradient-to-r from-transparent via-slate-700/50 to-transparent animate-[shimmer_2s_ease-in-out_infinite]" />
                      </div>
                    )}
                  </div>

                  {/* Avg Time */}
                  <div>
                    {entry.avgTime ? (
                      <span className="text-slate-400 text-sm font-mono">{entry.avgTime}</span>
                    ) : (
                      <div className="h-4 w-14 rounded bg-slate-800 overflow-hidden relative">
                        <div className="absolute inset-0 bg-gradient-to-r from-transparent via-slate-700/50 to-transparent animate-[shimmer_2s_ease-in-out_infinite]" />
                      </div>
                    )}
                  </div>

                  {/* Avg Token */}
                  <div>
                    {entry.avgTotalToken ? (
                      <div>
                        <span className="text-slate-400 text-sm font-mono">{entry.avgTotalToken}</span>
                        {entry.avgInputToken && (
                          <div className="text-slate-600 text-xs font-mono mt-0.5">{entry.avgInputToken} / {entry.avgOutputToken}</div>
                        )}
                      </div>
                    ) : (
                      <div className="h-4 w-14 rounded bg-slate-800 overflow-hidden relative">
                        <div className="absolute inset-0 bg-gradient-to-r from-transparent via-slate-700/50 to-transparent animate-[shimmer_2s_ease-in-out_infinite]" />
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>

        </div>
      </div>

    </section>
  );
};

export default BenchmarkSection;
