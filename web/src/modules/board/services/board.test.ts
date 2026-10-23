import { describe, expect, it } from 'vitest';
import { ciLabel, groupIssues } from './board';
import type { BoardIssue } from './types';

const issue = (status: BoardIssue['status'], number: number): BoardIssue => ({ id: String(number), repository_id: 'r', number, title: `Issue ${number}`, state: 'open', status, labels: [], assignee: '', pull_requests: [] });
describe('board mappings', () => {
  it('groups every status without moving records', () => {
    const grouped = groupIssues(['todo', 'in_progress', 'review', 'blocked', 'done'].map((status, index) => issue(status as BoardIssue['status'], index)));
    expect(Object.values(grouped).map((items) => items[0].status)).toEqual(['todo', 'in_progress', 'review', 'blocked', 'done']);
    expect(Object.values(grouped).flat().map((item) => item.number)).toEqual([0, 1, 2, 3, 4]);
  });
  it('maps CI success, failure and running states', () => {
    expect(ciLabel({ status: 'completed', conclusion: 'success' })).toBe('Passed');
    expect(ciLabel({ status: 'completed', conclusion: 'failure' })).toBe('Failed');
    expect(ciLabel({ status: 'in_progress', conclusion: '' })).toBe('Running');
  });
});
