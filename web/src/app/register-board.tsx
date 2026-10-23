import { boardModule } from '../modules/board/module';
import { paths } from './paths';

export function registerBoard() {
  paths.board = boardModule.path;
  return boardModule;
}
