"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
class CLIPlugin {
    logger;
    options;
    constructor(options) {
        this.options = options;
    }
    async setupBundleAnalyzerPlugin(compiler) {
        // @ts-expect-error No types right now
        const { BundleAnalyzerPlugin } = (await import("webpack-bundle-analyzer")).default;
        const bundleAnalyzerPlugin = compiler.options.plugins.some((plugin) => plugin instanceof BundleAnalyzerPlugin);
        if (!bundleAnalyzerPlugin) {
            new BundleAnalyzerPlugin().apply(compiler);
        }
    }
    static #progressStates = [];
    setupProgressPlugin(compiler) {
        const { ProgressPlugin } = compiler.webpack;
        const progressPlugin = compiler.options.plugins.some((plugin) => plugin instanceof ProgressPlugin);
        if (progressPlugin) {
            return;
        }
        const isProfile = this.options.progress === "profile";
        const options = {
            profile: isProfile,
        };
        if (this.options.isMultiCompiler && ProgressPlugin.createDefaultHandler) {
            const handler = ProgressPlugin.createDefaultHandler(isProfile, compiler.getInfrastructureLogger("webpack.Progress"));
            const idx = CLIPlugin.#progressStates.length;
            CLIPlugin.#progressStates[idx] = [0];
            options.handler = (progress, msg, ...args) => {
                CLIPlugin.#progressStates[idx] = [progress, msg, ...args];
                let sum = 0;
                for (const [progress] of CLIPlugin.#progressStates) {
                    sum += progress;
                }
                handler(sum / CLIPlugin.#progressStates.length, `[${compiler.name || idx}] ${msg}`, ...args);
            };
        }
        new ProgressPlugin(options).apply(compiler);
    }
    setupHelpfulOutput(compiler) {
        const pluginName = "webpack-cli";
        const getCompilationName = () => (compiler.name ? ` '${compiler.name}'` : "");
        const logCompilation = (message) => {
            if (process.env.WEBPACK_CLI_START_FINISH_FORCE_LOG) {
                process.stderr.write(message);
            }
            else {
                this.logger.log(message);
            }
        };
        const { configPath } = this.options;
        compiler.hooks.run.tap(pluginName, () => {
            const name = getCompilationName();
            logCompilation(`Compiler${name} starting... `);
            if (configPath) {
                this.logger.log(`Compiler${name} is using config: ${configPath.map((path) => `'${path}'`).join(", ")}`);
            }
        });
        compiler.hooks.watchRun.tap(pluginName, (compiler) => {
            const { bail, watch } = compiler.options;
            if (bail && watch) {
                this.logger.warn('You are using "bail" with "watch". "bail" will still exit webpack when the first error is found.');
            }
            const name = getCompilationName();
            logCompilation(`Compiler${name} starting... `);
            if (configPath) {
                this.logger.log(`Compiler${name} is using config: '${configPath}'`);
            }
        });
        compiler.hooks.invalid.tap(pluginName, (filename, changeTime) => {
            const date = new Date(changeTime);
            this.logger.log(`File '${filename}' was modified`);
            this.logger.log(`Changed time is ${date} (timestamp is ${changeTime})`);
        });
        compiler.hooks.afterDone.tap(pluginName, () => {
            const name = getCompilationName();
            logCompilation(`Compiler${name} finished`);
            process.nextTick(() => {
                if (compiler.watchMode) {
                    this.logger.log(`Compiler${name} is watching files for updates...`);
                }
            });
        });
    }
    apply(compiler) {
        this.logger = compiler.getInfrastructureLogger("webpack-cli");
        if (this.options.progress) {
            this.setupProgressPlugin(compiler);
        }
        if (this.options.analyze) {
            this.setupBundleAnalyzerPlugin(compiler);
        }
        this.setupHelpfulOutput(compiler);
    }
}
exports.default = CLIPlugin;
