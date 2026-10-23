import { Route, Routes } from 'react-router-dom';
import { registerHealth } from './register-health';

export function AppRouter() {
  const health = registerHealth();
  return <Routes><Route path={health.path} element={health.element} /></Routes>;
}
