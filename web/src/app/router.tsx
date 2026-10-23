import { Route, Routes } from 'react-router-dom';
import { registerHealth } from './register-health';
import { registerBoard } from './register-board';

export function AppRouter() {
  const health = registerHealth();
  const board = registerBoard();
  return <Routes><Route path={health.path} element={health.element} /><Route path={board.path} element={board.element} /></Routes>;
}
