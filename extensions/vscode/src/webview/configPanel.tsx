import { render } from 'preact';
import { ConfigPanelApp } from './ConfigPanelApp';

const root = document.getElementById('root');
if (root) render(<ConfigPanelApp />, root);
