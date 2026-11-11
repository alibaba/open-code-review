// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { Link } from 'react-router-dom';
import { useTranslation } from '../i18n';
import anthropic from '../assets/providers/anthropic.svg';
import openai from '../assets/providers/openai.svg';
import gemini from '../assets/providers/gemini.svg';
import qwen from '../assets/providers/qwen.svg';
import deepseek from '../assets/providers/deepseek.svg';
import kimi from '../assets/providers/kimi.svg';
import zai from '../assets/providers/zai.svg';
import minimax from '../assets/providers/minimax.svg';
import mistral from '../assets/providers/mistral.svg';
import xai from '../assets/providers/xai.svg';
import './ProvidersSection.css';

const providers = [
  { name: 'Anthropic', icon: anthropic },
  { name: 'OpenAI', icon: openai },
  { name: 'Gemini', icon: gemini },
  { name: 'Qwen', icon: qwen },
  { name: 'DeepSeek', icon: deepseek },
  { name: 'Kimi', icon: kimi },
  { name: 'Z.ai', icon: zai },
  { name: 'MiniMax', icon: minimax },
  { name: 'Mistral', icon: mistral },
  { name: 'xAI', icon: xai },
];

const ProvidersSection: React.FC = () => {
  const { t } = useTranslation();

  return (
    <section id="providers" className="providers-section" aria-labelledby="providers-heading">
      <div className="providers-inner">
        <div className="providers-heading">
          <h2 id="providers-heading">{t('providers.title')}</h2>
          <p>{t('providers.description')}</p>
        </div>

        <div className="providers-viewport">
          <div className="providers-track">
            {[false, true].map((isDuplicate) => (
              <ul
                key={String(isDuplicate)}
                className="providers-list"
                role="list"
                aria-label={isDuplicate ? undefined : t('providers.listLabel')}
                aria-hidden={isDuplicate ? true : undefined}
              >
                {providers.map((provider) => (
                  <li key={provider.name} className="providers-item">
                    <img src={provider.icon} alt="" width={32} height={32} />
                    <span>{provider.name}</span>
                  </li>
                ))}
              </ul>
            ))}
          </div>
        </div>

        <div className="providers-footer">
          <Link to="/docs/configuration" className="providers-link">
            {t('providers.explore')}
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" aria-hidden="true">
              <path d="M5 12h14m-6-6 6 6-6 6" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </Link>
        </div>
      </div>
    </section>
  );
};

export default ProvidersSection;
