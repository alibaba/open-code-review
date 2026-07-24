import React, { Suspense, useEffect } from 'react';
import { Routes, Route, useLocation } from 'react-router-dom';
import LandingPage from './components/LandingPage';
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

const App: React.FC = () => {
  return (
    <>
      <ScrollToTop />
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
    </>
  );
};

export default App;
