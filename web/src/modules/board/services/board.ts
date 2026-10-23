import { useQuery } from '@tanstack/react-query';
import { http } from '../../../shared/http';
import type { BoardIssue, IssueStatus, Page, Repository } from './types';

export const statuses: IssueStatus[] = ['todo', 'in_progress', 'review', 'blocked', 'done'];
export function groupIssues(issues: BoardIssue[]): Record<IssueStatus, BoardIssue[]> {
  return Object.fromEntries(statuses.map((status) => [status, issues.filter((issue) => issue.status === status)])) as Record<IssueStatus, BoardIssue[]>;
}
export function ciLabel(run: { status: string; conclusion: string }): string {
  if (run.status.toLowerCase() !== 'completed') return 'Running';
  const result = run.conclusion.toLowerCase();
  return result === 'success' ? 'Passed' : result === 'failure' ? 'Failed' : result || 'Completed';
}
export async function getRepositories(): Promise<Page<Repository>> {
  return (await http.get<Page<Repository>>('/api/v1/repositories', { params: { page: 1, per_page: 100 } })).data;
}
export async function getBoard(repositoryId: string, page: number): Promise<Page<BoardIssue>> {
  return (await http.get<Page<BoardIssue>>('/api/v1/board', { params: { repository_id: repositoryId, page, per_page: 100 } })).data;
}
export function useRepositories() { return useQuery({ queryKey: ['board', 'repositories'], queryFn: getRepositories }); }
export function useBoard(repositoryId: string, page: number) {
  return useQuery({ queryKey: ['board', repositoryId, page], queryFn: () => getBoard(repositoryId, page), enabled: Boolean(repositoryId) });
}
