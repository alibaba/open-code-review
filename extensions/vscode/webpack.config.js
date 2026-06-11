const path = require('path');

/** @type {import('webpack').Configuration} */
const extensionConfig = {
  name: 'extension',
  target: 'node',
  entry: { extension: './src/extension/extension.ts' },
  output: {
    path: path.resolve(__dirname, 'out'),
    filename: '[name].js',
    libraryTarget: 'commonjs2',
  },
  externals: { vscode: 'commonjs vscode' },
  resolve: { extensions: ['.ts', '.js'] },
  module: {
    rules: [
      {
        test: /\.ts$/,
        exclude: /node_modules/,
        use: { loader: 'ts-loader', options: { configFile: 'tsconfig.extension.json' } },
      },
    ],
  },
  devtool: 'source-map',
};

/** @type {import('webpack').Configuration} */
const webviewConfig = {
  name: 'webview',
  target: 'web',
  entry: { webview: './src/webview/index.tsx' },
  output: {
    path: path.resolve(__dirname, 'out'),
    filename: '[name].js',
  },
  resolve: { extensions: ['.ts', '.tsx', '.js'] },
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        exclude: /node_modules/,
        use: { loader: 'ts-loader', options: { configFile: 'tsconfig.webview.json' } },
      },
      { test: /\.css$/, use: ['style-loader', 'css-loader'] },
    ],
  },
  devtool: 'source-map',
};

module.exports = [extensionConfig, webviewConfig];
