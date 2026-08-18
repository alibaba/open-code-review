// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React from 'react';
import { useNavigate } from 'react-router-dom';
import Footer from '../components/Footer';
import { useTranslation } from '../i18n';

const NotFoundPage: React.FC = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  return (
    <div
      style={{
        minHeight: '100vh',
        paddingTop: 72,
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <div
        style={{
          flex: 1,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          padding: '80px 24px',
        }}
      >
        <div
          style={{
            width: '100%',
            maxWidth: 480,
            textAlign: 'center',
            color: '#ffffff',
          }}
        >
          <h1
            style={{
              margin: '0 0 16px',
              fontSize: 48,
              fontWeight: 700,
              lineHeight: 1.1,
            }}
          >
            {t('notFound.title')}
          </h1>
          <p
            style={{
              margin: '0 0 32px',
              fontSize: 16,
              lineHeight: 1.6,
              color: 'rgba(255,255,255,0.65)',
            }}
          >
            {t('notFound.description')}
          </p>
          <button
            type="button"
            onClick={() => navigate('/')}
            style={{
              border: 0,
              borderRadius: 8,
              padding: '12px 22px',
              background: '#ffffff',
              color: '#000000',
              fontSize: 15,
              fontWeight: 600,
              cursor: 'pointer',
            }}
          >
            {t('notFound.backHome')}
          </button>
        </div>
      </div>
      <Footer />
    </div>
  );
};

export default NotFoundPage;
