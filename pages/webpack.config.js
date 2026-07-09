const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');

module.exports = {
  mode: process.env.NODE_ENV === 'production' ? 'production' : 'development',
  entry: './src/index.tsx',
  output: {
    path: path.resolve(__dirname, 'dist'),
    filename: 'bundle.js',
    publicPath: process.env.NODE_ENV === 'production' ? '/open-code-review/' : '/'
  },
  module: {
    rules: [
      {
        test: /\.(ts|tsx)$/,
        exclude: /node_modules/,
        use: {
          loader: 'babel-loader',
          options: {
            presets: [
              ['@babel/preset-react', { development: process.env.NODE_ENV !== 'production' }],
              '@babel/preset-env',
              '@babel/preset-typescript'
            ]
          }
        }
      },
      {
        test: /\.css$/,
        use: ['style-loader', 'css-loader', 'postcss-loader']
      },
      {
        test: /\.svg$/,
        type: 'asset/resource'
      },
      {
        test: /\.(png|jpg|jpeg|gif)$/,
        type: 'asset/resource'
      },
      {
        test: /\.md$/,
        type: 'asset/source'
      }
    ]
  },
  resolve: {
    extensions: ['.ts', '.tsx', '.js', '.jsx']
  },
  devServer: {
    port: 3030,
    host: '0.0.0.0',
    allowedHosts: 'all',
    static: { directory: __dirname },
    historyApiFallback: {
      index: '/index.html',
      rewrites: [{ from: /^\/\_p\/\d+\//, to: '/index.html' }]
    }
  },
  plugins: [
    new HtmlWebpackPlugin({
      template: './index.html',
      inject: 'body'
    })
  ]
};
