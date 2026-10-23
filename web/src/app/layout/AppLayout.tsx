import type { ReactNode } from 'react';
import { NavLink } from 'react-router-dom';
import { paths } from '../paths';

export function AppLayout({ children }: { children: ReactNode }) {
  return <div className="flex min-h-screen"><aside className="w-56 bg-slate-900 p-6 text-white"><h1 className="mb-8 text-xl font-bold">Agent Ops</h1><NavLink to={paths.health} className="rounded px-3 py-2 hover:bg-slate-700">Health</NavLink></aside><main className="flex-1 p-10">{children}</main></div>;
}
