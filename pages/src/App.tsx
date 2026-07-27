import React, { Suspense, useEffect } from 'react';
import { Routes, Route, useLocation } from 'react-router-dom';
import LandingPage from './components/LandingPage';
import ErrorBoundary from './components/ErrorBoundary';
import { useTranslation } from './i18n';
import FeaturesPage from './pages/FeaturesPage';

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

const App: React.FC = () => {
  return (
    <>
      <ScrollToTop />
      <ErrorBoundary reloadOnChunkError fallback={(_error, reset) => <RouteErrorFallback reset={reset} />}>
        <Suspense fallback={<div style={{ minHeight: '100vh', background: '#000000' }} />}>
          <Routes>
            <Route path="/" element={<LandingPage><FeaturesPage /></LandingPage>} />
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
