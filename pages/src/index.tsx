import React from 'react';
import ReactDOM from 'react-dom/client';
import { HashRouter } from 'react-router-dom';
import App from './App';
import { LanguageProvider } from './i18n';
import './styles/index.css';

if ('scrollRestoration' in history) {
  history.scrollRestoration = 'manual';
}
window.scrollTo(0, 0);

const rootElement = document.getElementById('root') as HTMLElement;
const root = ReactDOM.createRoot(rootElement);

root.render(
  <React.StrictMode>
    <LanguageProvider>
      <HashRouter>
        <App />
      </HashRouter>
    </LanguageProvider>
  </React.StrictMode>
);
