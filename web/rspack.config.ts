import { rspack } from '@rspack/core';
import type { Configuration } from '@rspack/core';

const config: Configuration = {
  entry: './src/main.tsx',
  experiments: { css: true },
  output: { publicPath: '/' },
  resolve: { extensions: ['.tsx', '.ts', '.js'] },
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        use: {
          loader: 'builtin:swc-loader',
          options: { jsc: { transform: { react: { runtime: 'automatic' } } } },
        },
        exclude: /node_modules/,
      },
      {
        test: /\.css$/,
        type: 'css',
        use: [{ loader: 'postcss-loader', options: { postcssOptions: { config: './postcss.config.mjs' } } }],
      },
    ],
  },
  plugins: [new rspack.HtmlRspackPlugin({ template: './index.html' })],
  devServer: { historyApiFallback: true, proxy: [{ context: ['/healthz'], target: 'http://localhost:8080' }] },
};
export default config;
