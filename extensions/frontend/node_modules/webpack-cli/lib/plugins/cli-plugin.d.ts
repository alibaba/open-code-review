import { type Compiler } from "webpack";
interface CLIPluginOptions {
    isMultiCompiler?: boolean;
    configPath?: string[];
    helpfulOutput: boolean;
    hot?: boolean | "only";
    progress?: boolean | "profile";
    prefetch?: string;
    analyze?: boolean;
}
export default class CLIPlugin {
    #private;
    logger: ReturnType<Compiler["getInfrastructureLogger"]>;
    options: CLIPluginOptions;
    constructor(options: CLIPluginOptions);
    setupBundleAnalyzerPlugin(compiler: Compiler): Promise<void>;
    setupProgressPlugin(compiler: Compiler): void;
    setupHelpfulOutput(compiler: Compiler): void;
    apply(compiler: Compiler): void;
}
export {};
