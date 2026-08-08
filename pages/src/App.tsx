// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import React, { Suspense, useEffect, useState, useTransition } from 'react';
import { Routes, Route, useLocation } from 'react-router-dom';
import LandingPage from './components/LandingPage';
import ErrorBoundary from './components/ErrorBoundary';
import { useTranslation } from './i18n';
import FeaturesPage from './pages/FeaturesPage';
import FeaturesRoutePage from './pages/FeaturesRoutePage';

const BenchmarkPage = React.lazy(() => import(/* webpackChunkName: "benchmark-page" */ './pages/BenchmarkPage'));
const QuickStartPage = React.lazy(() => import(/* webpackChunkName: "quickstart-page" */ './pages/QuickStartPage'));
const DocsPage = React.lazy(() => import(/* webpackChunkName: "docs-page" */ './pages/DocsPage'));
const BlogPage = React.lazy(() => import(/* webpackChunkName: "blog-page" */ './pages/BlogPage'));

const ScrollToTop: React.FC = () => {
  const { pathname } = useLocation();
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [pathname]);
  return null;
};

const RouteErrorFallback: React.FC<{ reset: () => void }> = ({ reset }) => {
  const { t } = useTranslation();

  return (
    <div
      style={{
        minHeight: '100vh',
        background: '#000000',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
      }}
    >
      <div
        style={{
          width: '100%',
          maxWidth: 360,
          padding: 24,
          background: '#141414',
          border: '1px solid rgba(255,255,255,0.16)',
          borderRadius: 12,
          color: '#ffffff',
          textAlign: 'center',
        }}
      >
        <p style={{ margin: '0 0 16px', fontSize: 16 }}>{t('error.pageLoadFailed')}</p>
        <button
          type="button"
          onClick={reset}
          style={{
            border: 0,
            borderRadius: 6,
            padding: '10px 18px',
            background: '#ffffff',
            color: '#000000',
            fontSize: 14,
            fontWeight: 600,
            cursor: 'pointer',
          }}
        >
          {t('error.reload')}
        </button>
      </div>
    </div>
  );
};

const TopProgressBar: React.FC = () => (
  <div
    style={{
      position: 'fixed',
      top: 0,
      left: 0,
      width: '100%',
      height: 2,
      zIndex: 200,
      pointerEvents: 'none',
    }}
  >
    <div className="ocr-transition-bar" />
  </div>
);

const App: React.FC = () => {
  const [isPending, startTransition] = useTransition();
  const location = useLocation();
  const [displayLocation, setDisplayLocation] = useState(location);

  useEffect(() => {
    startTransition(() => {
      setDisplayLocation(location);
    });
  }, [location]);

  return (
    <>
      <ScrollToTop />
      {isPending && <TopProgressBar />}
      <ErrorBoundary reloadOnChunkError fallback={(_error, reset) => <RouteErrorFallback reset={reset} />}>
        <Suspense fallback={<div style={{ minHeight: '100vh', background: '#000000' }} />}>
          <Routes location={displayLocation}>
            <Route path="/" element={<LandingPage><FeaturesPage /></LandingPage>} />
            <Route path="/features" element={<LandingPage><FeaturesRoutePage /></LandingPage>} />
            <Route path="/benchmark" element={<LandingPage><BenchmarkPage /></LandingPage>} />
            <Route path="/quickstart" element={<LandingPage><QuickStartPage /></LandingPage>} />
            <Route path="/docs" element={<DocsPage />} />
            <Route path="/docs/:slug" element={<DocsPage />} />
            <Route path="/blog" element={<BlogPage />} />
            <Route path="/blog/:slug" element={<BlogPage />} />
          </Routes>
        </Suspense>
      </ErrorBoundary>
    </>
  );
};

export default App;
