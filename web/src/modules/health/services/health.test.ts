import { describe, expect, it, vi } from 'vitest';
import { http } from '../../../shared/http';
import { getHealth } from './health';

describe('getHealth', () => {
  it('returns the API health contract', async () => {
    const get = vi.spyOn(http, 'get').mockResolvedValueOnce({ data: { status: 'ok' } });
    await expect(getHealth()).resolves.toEqual({ status: 'ok' });
    expect(get).toHaveBeenCalledWith('/healthz');
  });
});
