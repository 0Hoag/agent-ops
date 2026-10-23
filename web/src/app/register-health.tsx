import { paths } from './paths';
import { healthModule } from '../modules/health/module';

export function registerHealth() {
  paths.health = healthModule.path;
  return healthModule;
}
