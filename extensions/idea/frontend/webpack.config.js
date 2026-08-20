const path = require('path');

// 产物直接落到插件的资源目录：Kotlin 侧会把 JS 内联进 HTML 再交给 JCEF。
// 之所以内联而不是 <script src="...">：JBCefBrowser.loadHTML 没有 base URL，
// 相对路径的 script 请求会落到 about:blank 上，永远 404。
const OUT_DIR = path.resolve(__dirname, '../src/main/resources/webview');

/** @param {'development'|'production'} mode */
function webConfig(name, entry, mode) {
  return {
    name,
    target: 'web',
    entry: { [name]: entry },
    output: {
      path: OUT_DIR,
      filename: '[name].js',
      // 内联进 HTML 之后独立的 .map 文件取不到，所以生产不出 sourcemap，
      // 开发模式用 inline-source-map，devtools 里才能看到 TSX 源码。
      clean: false,
    },
    resolve: { extensions: ['.ts', '.tsx', '.js'] },
    module: {
      rules: [
        {
          test: /\.tsx?$/,
          exclude: /node_modules/,
          use: { loader: 'ts-loader', options: { configFile: 'tsconfig.json' } },
        },
        { test: /\.css$/, use: ['style-loader', 'css-loader'] },
      ],
    },
    devtool: mode === 'production' ? false : 'inline-source-map',
    performance: { hints: false },
  };
}

module.exports = (_env, argv) => {
  const mode = argv.mode === 'production' ? 'production' : 'development';
  return [
    webConfig('webview', './src/webview/index.tsx', mode),
    webConfig('configPanel', './src/webview/configPanel.tsx', mode),
  ];
};
