import { rspack } from '@rspack/core';
import type { Configuration } from '@rspack/core';

const config: Configuration = {
  entry: './src/main.tsx',
  output: { publicPath: '/' },
  resolve: { extensions: ['.tsx', '.ts', '.js'] },
  module: { rules: [{ test: /\.tsx?$/, use: 'builtin:swc-loader', exclude: /node_modules/ }, { test: /\.css$/, type: 'css' }] },
  plugins: [new rspack.HtmlRspackPlugin({ template: './index.html' })],
  devServer: { historyApiFallback: true, proxy: [{ context: ['/healthz'], target: 'http://localhost:8080' }] },
};
export default config;
