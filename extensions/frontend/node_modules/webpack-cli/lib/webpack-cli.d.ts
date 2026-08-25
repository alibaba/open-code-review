import { type Command as CommanderCommand, type CommandOptions as CommanderCommandOptions, type ParseOptions } from "commander";
import { type Colors as WebpackColors, type Compiler, type Configuration, type MultiCompiler, type MultiConfiguration, type MultiStats, type Stats, type WebpackError, default as webpack } from "webpack";
type RecordAny = Record<string, any>;
type PackageManager = "pnpm" | "yarn" | "npm";
type LogHandler = (value: any) => void;
interface Logger {
    error: LogHandler;
    warn: LogHandler;
    info: LogHandler;
    success: LogHandler;
    log: LogHandler;
    raw: LogHandler;
}
interface Colors extends WebpackColors {
    isColorSupported: boolean;
}
type Context = RecordAny;
interface Command extends CommanderCommand {
    pkg?: string;
    forHelp?: boolean;
    context: Context;
}
interface CommandOptions<A = void, O extends CommanderArgs = CommanderArgs, C extends Context = Context> extends CommanderCommandOptions {
    rawName: string;
    name: string;
    alias: string | string[];
    description?: string;
    usage?: string;
    dependencies?: string[];
    pkg?: string;
    preload?: () => Promise<C>;
    options?: CommandOption[] | ((command: Command & {
        context: C;
    }) => CommandOption[]) | ((command: Command & {
        context: C;
    }) => Promise<CommandOption[]>);
    action: A extends void ? (options: O, cmd: Command & {
        context: C;
    }) => void | Promise<void> : (args: A, options: O, cmd: Command & {
        context: C;
    }) => void | Promise<void>;
}
type CommanderArgs = Record<string, any>;
type BasicPrimitive = string | boolean | number;
type EnumValue = string | number | boolean | EnumValueObject | EnumValue[] | null;
interface EnumValueObject {
    [key: string]: EnumValue;
}
interface ArgumentConfig {
    description?: string;
    negatedDescription?: string;
    path?: string;
    multiple?: boolean;
    type: "enum" | "string" | "path" | "number" | "boolean" | "RegExp" | "reset";
    values?: EnumValue[];
}
interface CommandOption {
    name: string;
    alias?: string;
    type?: (value: string, previous: Record<string, BasicPrimitive | object>) => Record<string, BasicPrimitive | object>;
    configs?: ArgumentConfig[];
    negative?: boolean;
    multiple?: boolean;
    valueName?: string;
    description?: string;
    describe?: string;
    negatedDescription?: string;
    defaultValue?: string;
    hidden?: boolean;
    negativeHidden?: boolean;
}
interface Env {
    WEBPACK_BUNDLE?: boolean;
    WEBPACK_BUILD?: boolean;
    WEBPACK_WATCH?: boolean;
    WEBPACK_SERVE?: boolean;
}
interface Argv extends RecordAny {
    env: Env;
}
interface ConfigurationsAndPaths {
    options: Configuration | MultiConfiguration;
    path: WeakMap<object, string[]>;
}
declare interface WebpackCallback {
    (err: null | Error, result?: Stats): void;
    (err: null | Error, result?: MultiStats): void;
}
type Schema = Parameters<(typeof webpack)["cli"]["getArguments"]>[0];
interface KnownOptions {
    config?: string[];
    argv?: Argv;
    env?: Env;
    configNodeEnv?: string;
    watchOptionsStdin?: boolean;
    watch?: boolean;
    failOnWarnings?: boolean;
    isWatchingLikeCommand?: boolean;
    progress?: boolean | "profile";
    analyze?: boolean;
    json?: boolean;
    entry?: string | string[];
    merge?: boolean;
    configName?: string[];
    disableInterpret?: boolean;
    extends?: string[];
    webpack: typeof webpack;
}
type Options = KnownOptions & RecordAny;
export declare function distance(first: string, second: string): number;
declare class WebpackCLI {
    #private;
    get colors(): Colors;
    set colors(value: Colors);
    logger: Logger;
    program: Command;
    constructor();
    isPromise<T>(value: Promise<T>): value is Promise<T>;
    isFunction(value: unknown): value is CallableFunction;
    capitalizeFirstLetter(str: string): string;
    toKebabCase(str: string): string;
    getLogger(): Logger;
    getDefaultPackageManager(): Promise<PackageManager | undefined>;
    isPackageInstalled(packageName: string): Promise<boolean>;
    installPackage(packageName: string, options?: {
        preMessage?: () => void;
    }): Promise<string>;
    makeCommand<A = void, O extends CommanderArgs = CommanderArgs, C extends Context = Context>(options: CommandOptions<A, O, C>): Promise<Command>;
    makeOption(command: Command, option: CommandOption): void;
    isMultipleConfiguration(config: Configuration | MultiConfiguration): config is MultiConfiguration;
    isMultipleCompiler(compiler: Compiler | MultiCompiler): compiler is MultiCompiler;
    isValidationError(error: unknown): error is WebpackError;
    schemaToOptions(webpackMod: typeof webpack, schema?: Schema, additionalOptions?: CommandOption[], override?: Partial<CommandOption>): CommandOption[];
    loadWebpack(): Promise<typeof webpack>;
    loadWebpackDevServer(): Promise<typeof import("webpack-dev-server")>;
    run(args: readonly string[], parseOptions: ParseOptions): Promise<void>;
    loadConfig(options: Options): Promise<ConfigurationsAndPaths>;
    createCompiler(options: Options, callback?: WebpackCallback): Promise<Compiler | MultiCompiler>;
    runWebpack(options: Options, isWatchCommand: boolean): Promise<void>;
}
export default WebpackCLI;
