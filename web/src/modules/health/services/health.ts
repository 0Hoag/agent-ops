import { useQuery } from '@tanstack/react-query';
import { http } from '../../../shared/http';
import type { HealthResponse } from './types';

export async function getHealth(): Promise<HealthResponse> {
  const response = await http.get<HealthResponse>('/healthz');
  return response.data;
}
export function useHealth() { return useQuery({ queryKey: ['health'], queryFn: getHealth }); }
