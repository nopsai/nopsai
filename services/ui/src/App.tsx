import { useEffect } from 'react';
import { HashRouter, useLocation } from 'react-router-dom';
import AppShell from './app/AppShell';
import { canonicalizeHashRouterURL } from './app/hashRouterUrl';
import { AuthProvider } from './auth/AuthProvider';

function App() {
  return (
    <HashRouter>
      <CanonicalHashRouterURL />
      <AuthProvider>
        <AppShell />
      </AuthProvider>
    </HashRouter>
  );
}

function CanonicalHashRouterURL() {
  const location = useLocation();
  useEffect(() => {
    canonicalizeHashRouterURL();
  }, [location.key, location.pathname, location.search]);
  return null;
}

export default App;
