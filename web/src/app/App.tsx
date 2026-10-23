import { BrowserRouter } from 'react-router-dom';
import { AppLayout } from './layout/AppLayout';
import { AppRouter } from './router';

export function App() {
  return <BrowserRouter><AppLayout><AppRouter /></AppLayout></BrowserRouter>;
}
