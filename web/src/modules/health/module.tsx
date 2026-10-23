import { HealthPage } from './HealthPage';
import { healthPaths } from './paths';

export const healthModule = {
  path: healthPaths.health,
  nav: { label: 'Health', path: healthPaths.health },
  element: <HealthPage />,
};
