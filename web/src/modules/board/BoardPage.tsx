import { useState } from 'react';
import { ciLabel, groupIssues, statuses, useBoard, useRepositories } from './services/board';
import type { BoardIssue, IssueStatus, PullRequest } from './services/types';

const titles: Record<IssueStatus, string> = { todo: 'Todo', in_progress: 'In progress', review: 'Review', blocked: 'Blocked', done: 'Done' };
function IssueCard({ issue }: { issue: BoardIssue }) {
  return <article className="rounded border border-slate-200 bg-white p-3 shadow-sm"><div className="text-xs text-slate-500">#{issue.number} · {issue.state}</div><h3 className="my-2 text-sm font-semibold">{issue.title}</h3>{issue.assignee && <div className="text-xs text-slate-600">Assignee: {issue.assignee}</div>}{issue.labels.length > 0 && <div className="my-2 flex flex-wrap gap-1">{issue.labels.map((label) => <span key={label} className="rounded bg-slate-100 px-2 py-1 text-xs">{label}</span>)}</div>}{issue.pull_requests.map((pr) => <PullRequestCard key={pr.number} pr={pr} />)}</article>;
}
function PullRequestCard({ pr }: { pr: PullRequest }) {
  return <div className="mt-3 border-t border-slate-100 pt-2 text-xs"><div className="font-medium">PR #{pr.number} · {pr.state} — {pr.title}</div>{pr.pipeline_runs.map((run) => { const label = ciLabel(run); const color = label === 'Passed' ? 'text-emerald-700' : label === 'Failed' ? 'text-rose-700' : 'text-amber-700'; return <div key={`${run.external_id}-${run.sha}`} className={color}>CI {run.external_id}: {label}</div>; })}</div>;
}
export function BoardPage() {
  const repositories = useRepositories();
  const [repositoryId, setRepositoryId] = useState('');
  const [page, setPage] = useState(1);
  const board = useBoard(repositoryId, page);
  const allIssues = board.data?.data ?? [];
  const grouped = groupIssues(allIssues);
  return <section><header className="mb-5 flex flex-wrap items-end justify-between gap-4"><div><h1 className="text-2xl font-semibold">Issue board</h1><p className="text-sm text-slate-600">Repository work and linked pull request checks</p></div><label className="grid gap-1 text-sm">Repository{repositories.isError ? <span role="alert" className="text-rose-700">Unable to load repositories: {repositories.error.message}</span> : <select aria-label="Repository" className="min-w-64 rounded border bg-white p-2" value={repositoryId} onChange={(event) => { setRepositoryId(event.target.value); setPage(1); }} disabled={repositories.isPending || !repositories.data?.data.length}><option value="">Select a repository</option>{repositories.data?.data.map((repo) => <option key={repo.id} value={repo.id}>{repo.full_name}</option>)}</select>}</label></header>
    {repositories.isPending && <div aria-label="Loading repositories" className="grid grid-cols-2 gap-3 md:grid-cols-5">{statuses.map((status) => <div key={status} className="h-48 animate-pulse rounded bg-slate-200" />)}</div>}
    {!repositories.isPending && !repositories.isError && !repositories.data?.data.length && <p className="rounded bg-white p-6 text-slate-600">No repositories available.</p>}
    {repositoryId && board.isPending && <p role="status" className="p-6">Loading board…</p>}
    {repositoryId && board.isError && <p role="alert" className="rounded bg-rose-50 p-4 text-rose-800">Unable to load board: {board.error.message}</p>}
    {repositoryId && board.data && allIssues.length === 0 && <p className="rounded bg-white p-6 text-slate-600">No issues on this board.</p>}
    {repositoryId && board.data && allIssues.length > 0 && <><div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-5">{statuses.map((status) => <div key={status} className="min-w-0 rounded bg-slate-100 p-2"><h2 className="mb-2 flex justify-between text-sm font-semibold"><span>{titles[status]}</span><span>{grouped[status].length}</span></h2><div className="grid content-start gap-2">{grouped[status].map((issue) => <IssueCard key={issue.id} issue={issue} />)}</div></div>)}</div><div className="mt-4 flex items-center justify-between text-sm"><span>Page {board.data.meta.page} · {board.data.meta.total} issues</span><div className="flex gap-2"><button className="rounded border bg-white px-3 py-1 disabled:opacity-40" disabled={page <= 1 || board.isFetching} onClick={() => setPage(page - 1)}>Previous</button><button className="rounded border bg-white px-3 py-1 disabled:opacity-40" disabled={board.data.meta.page * board.data.meta.per_page >= board.data.meta.total || board.isFetching} onClick={() => setPage(page + 1)}>Next</button></div></div></>}
  </section>;
}
