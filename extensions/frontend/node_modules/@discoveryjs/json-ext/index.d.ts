declare module '@discoveryjs/json-ext' {
    export type Chunk = string | Uint8Array | Buffer;

    export type Reviver = (this: any, key: string, value: any) => any;
    export type ParseChunkedState = {
        readonly mode: 'json' | 'jsonl';
        readonly returnValue: any;
        readonly currentRootValue: any;
        readonly rootValuesCount: number;
        readonly consumed: number;
        readonly parsed: number;
    };
    export type OnRootValue = (value: any, state: ParseChunkedState) => void;
    export type OnChunk = (chunkParsed: number, chunk: string | null, pending: string | null, state: ParseChunkedState) => void;
    export type ParseOptions = {
        reviver?: Reviver;
        mode?: 'json' | 'jsonl' | 'auto';
        onRootValue?: OnRootValue;
        onChunk?: OnChunk;
    };

    export type Replacer =
        | ((this: any, key: string, value: any) => any)
        | (string | number)[]
        | null;
    export type Space = string | number | null;
    export type StringifyOptions = {
        replacer?: Replacer;
        space?: Space;
        mode?: 'json' | 'jsonl';
        highWaterMark?: number;
    };
    export type StringifyInfoOptions = {
        replacer?: Replacer;
        space?: Space;
        mode?: 'json' | 'jsonl';
        continueOnCircular?: boolean;
    };
    export type StringifyInfoResult = {
        bytes: number;
        spaceBytes: number;
        circular: object[];
    };

    export function parseChunked(input: Iterable<Chunk> | AsyncIterable<Chunk>, reviver?: Reviver): Promise<any>;
    export function parseChunked(input: Iterable<Chunk> | AsyncIterable<Chunk>, options: ParseOptions & { onRootValue: OnRootValue }): Promise<number>;
    export function parseChunked(input: Iterable<Chunk> | AsyncIterable<Chunk>, options?: ParseOptions): Promise<any>;
    export function parseChunked(input: () => (Iterable<Chunk> | AsyncIterable<Chunk>), reviver?: Reviver): Promise<any>;
    export function parseChunked(input: () => (Iterable<Chunk> | AsyncIterable<Chunk>), options: ParseOptions & { onRootValue: OnRootValue }): Promise<number>;
    export function parseChunked(input: () => (Iterable<Chunk> | AsyncIterable<Chunk>), options?: ParseOptions): Promise<any>;

    export function stringifyChunked(value: any, replacer?: Replacer, space?: Space): Generator<string>;
    export function stringifyChunked(value: any, options: StringifyOptions): Generator<string>;

    export function stringifyInfo(value: any, replacer?: Replacer, space?: Space): StringifyInfoResult;
    export function stringifyInfo(value: any, options?: StringifyInfoOptions): StringifyInfoResult;

    // Web streams
    export function parseFromWebStream(stream: ReadableStream<Chunk>): Promise<any>;
    export function createStringifyWebStream(value: any, replacer?: Replacer, space?: Space): ReadableStream<string>;
    export function createStringifyWebStream(value: any, options: StringifyOptions): ReadableStream<string>;
}
