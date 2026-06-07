import { HashRouter } from 'react-router-dom';
import AppShell from './app/AppShell';
import { AuthProvider } from './auth/AuthProvider';

function App() {
  return (
    <HashRouter>
      <AuthProvider>
        <AppShell />
      </AuthProvider>
    </HashRouter>
  );
}

export default App;
