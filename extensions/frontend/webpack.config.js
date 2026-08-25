const path = require('path');
const webpack = require('webpack');

const isIdea = process.env.OCR_TARGET === 'idea';
const bridge = isIdea ? './bridge.idea' : './bridge.vsc';
const outDir = isIdea
  ? path.resolve(__dirname, '../idea/src/main/resources/webview')
  : path.resolve(__dirname, '../vscode/out');

function webConfig(name, entry, mode) {
  return {
    name,
    mode,
    target: 'web',
    entry: { [name]: `./src/webview/${entry}` },
    output: {
      path: outDir,
      filename: '[name].js',
      clean: false,
    },
    resolve: { extensions: ['.ts', '.tsx', '.js'] },
    module: {
      rules: [
        {
          test: /\.tsx?$/,
          exclude: /node_modules/,
          use: { loader: 'ts-loader', options: { configFile: 'tsconfig.json', transpileOnly: true } },
        },
        { test: /\.css$/, use: ['style-loader', 'css-loader'] },
      ],
    },
    plugins: [
      new webpack.NormalModuleReplacementPlugin(/bridge$/, bridge),
    ],
    devtool: mode === 'production' ? false : 'inline-source-map',
    performance: { hints: false },
  };
}

module.exports = (_env, argv) => {
  const mode = argv.mode === 'production' ? 'production' : 'development';
  return [
    webConfig('webview', 'index.tsx', mode),
    webConfig('configPanel', 'configPanel.tsx', mode),
  ];
};
