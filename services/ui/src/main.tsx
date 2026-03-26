import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { installAuthInterceptor } from './lib/api';
import App from './App.tsx';
import './index.css';
import './styles.css';

installAuthInterceptor();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
