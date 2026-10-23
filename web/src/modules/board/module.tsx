import { BoardPage } from './BoardPage';
import { boardPaths } from './paths';

export const boardModule = { path: boardPaths.board, nav: { label: 'Board', path: boardPaths.board }, element: <BoardPage /> };
