export type TickMode = "nextAsync" | "manual" | "interval";
export type NextAsyncTickMode = {
    mode: "nextAsync";
};
export type ManualTickMode = {
    mode: "manual";
};
export type IntervalTickMode = {
    mode: "interval";
    delta?: number;
};
export type TimerTickMode = IntervalTickMode | NextAsyncTickMode | ManualTickMode;
export type FakeTimersFunction = (...args: unknown[]) => unknown;
export type VoidVarArgsFunc = (...args: unknown[]) => void;
export type NextTick = (callback: VoidVarArgsFunc, ...args: unknown[]) => void;
export type SetImmediate = (callback: VoidVarArgsFunc, ...args: unknown[]) => NodeImmediate;
export type SetTimeout = (callback: VoidVarArgsFunc, delay?: number, ...args: unknown[]) => TimerId;
export type ClearTimeout = (id?: TimerId) => void;
export type SetInterval = (callback: VoidVarArgsFunc, delay?: number, ...args: unknown[]) => TimerId;
export type ClearInterval = (id?: TimerId) => void;
export type QueueMicrotask = (callback: VoidVarArgsFunc) => void;
export type TimeRemaining = () => number;
export type IdleDeadline = {
    didTimeout: boolean;
    timeRemaining: TimeRemaining;
};
export type RequestIdleCallbackCallback = (deadline: IdleDeadline) => any;
export type RequestIdleCallback = (callback: RequestIdleCallbackCallback, options?: {
    timeout?: number;
}) => number;
export type AnimationFrameCallback = (timestamp: number) => any;
export type RequestAnimationFrame = (callback: AnimationFrameCallback) => TimerId;
export type CancelAnimationFrame = (id: TimerId) => void;
export type CancelIdleCallback = (id: TimerId) => void;
export type ClearImmediate = (id: NodeImmediate) => void;
export type CountTimers = () => number;
export type RunMicrotasks = () => void;
export type TemporalDuration = {
    years: number;
    months: number;
    weeks: number;
    days: number;
    hours: number;
    minutes: number;
    seconds: number;
    milliseconds: number;
    microseconds: number;
    nanoseconds: number;
    total: (options: {
        unit: string;
        relativeTo?: unknown;
    }) => number;
};
export type TemporalTimelike = {
    epochMilliseconds: number;
};
export type Tick = (tickValue: number | string | TemporalDuration) => number;
export type TickAsync = (tickValue: number | string | TemporalDuration) => Promise<number>;
export type Next = () => number;
export type NextAsync = () => Promise<number>;
export type RunAll = () => number;
export type RunToFrame = () => number;
export type RunAllAsync = () => Promise<number>;
export type RunToLast = () => number;
export type RunToLastAsync = () => Promise<number>;
export type Reset = () => void;
export type SetSystemTime = (now?: number | Date | TemporalTimelike) => void;
export type Jump = (tickValue: number | string | TemporalDuration) => number;
export type Uninstall = () => void;
export type SetTickMode = (tickModeConfig: SetTickModeConfig) => void;
export type Hrtime = (prev?: Array<number>) => Array<number>;
export type WithGlobal = (_global: object) => FakeTimers;
export type FakeMethod = "setTimeout" | "clearTimeout" | "setImmediate" | "clearImmediate" | "setInterval" | "clearInterval" | "Date" | "nextTick" | "hrtime" | "requestAnimationFrame" | "cancelAnimationFrame" | "requestIdleCallback" | "cancelIdleCallback" | "performance" | "queueMicrotask" | "Intl" | "Temporal";
export type TimerId = number | NodeImmediate | Timer;
export type GlobalObject = Record<string, any> & {
    setTimeout?: SetTimeout;
    clearTimeout?: ClearTimeout;
    setInterval?: SetInterval;
    clearInterval?: ClearInterval;
    setImmediate?: SetImmediate;
    clearImmediate?: ClearImmediate;
    queueMicrotask?: QueueMicrotask;
    requestAnimationFrame?: RequestAnimationFrame;
    cancelAnimationFrame?: CancelAnimationFrame;
    requestIdleCallback?: RequestIdleCallback;
    cancelIdleCallback?: CancelIdleCallback;
    process?: any;
    performance?: any;
    Performance?: any;
    Intl?: any;
    Temporal?: any;
    Promise?: typeof Promise;
    Date: typeof Date & {
        isFake?: boolean;
        toSource?: () => string;
        clock?: any;
    };
};
export type TimerHeap = {
    timers: Timer[];
    peek: () => Timer | undefined;
    push: (timer: Timer) => void;
    pop: () => Timer | undefined;
    remove: (timer: Timer) => void;
};
export type ClockTickMode = {
    mode: TickMode;
    counter: number;
    delta?: number;
};
export type SetTickModeConfig = {
    mode: TickMode;
    delta?: number;
};
export type IntlWithClock = Record<string, any> & {
    clock: Clock;
};
export type PerformanceLike = Record<string, any> & {
    now: () => number;
};
export type Timers = {
    setTimeout: SetTimeout;
    clearTimeout: ClearTimeout;
    setInterval: SetInterval;
    clearInterval: ClearInterval;
    Date: typeof Date;
    Intl?: typeof Intl;
    Temporal?: any;
    setImmediate?: SetImmediate;
    clearImmediate?: ClearImmediate;
    hrtime?: Hrtime;
    nextTick?: NextTick;
    performance?: PerformanceLike;
    requestAnimationFrame?: RequestAnimationFrame;
    queueMicrotask?: QueueMicrotask;
    cancelAnimationFrame?: CancelAnimationFrame;
    requestIdleCallback?: RequestIdleCallback;
    cancelIdleCallback?: CancelIdleCallback;
};
export type ClockState = {
    tickFrom: number;
    tickTo: number;
    previous?: number;
    oldNow?: number | null;
    timer?: Timer;
    firstException?: unknown;
    nanosTotal?: number;
    msFloat?: number;
    ms?: number;
};
export type TimerInitialProps = {
    func: VoidVarArgsFunc;
    args?: unknown[];
    type?: 'Timeout' | 'Interval' | 'Immediate' | 'AnimationFrame' | 'IdleCallback';
    delay?: number;
    callAt?: number;
    createdAt?: number;
    immediate?: boolean;
    id?: number;
    error?: Error;
    interval?: number;
    animation?: boolean;
    requestIdleCallback?: boolean;
    order?: number;
    heapIndex?: number;
};
export type CreateClockCallback = (start?: number | Date | TemporalTimelike, loopLimit?: number) => Clock;
export type InstallCallback = (config?: Config) => Clock;
export type FakeTimers = {
    timers: Timers;
    createClock: CreateClockCallback;
    install: InstallCallback;
    withGlobal: WithGlobal;
};
export type Clock = {
    now: number;
    Date: typeof Date & {
        clock?: Clock;
        isFake?: boolean;
        toSource?: () => string;
    };
    loopLimit: number;
    requestIdleCallback: RequestIdleCallback;
    cancelIdleCallback: CancelIdleCallback;
    setTimeout: SetTimeout;
    clearTimeout: ClearTimeout;
    nextTick: NextTick;
    queueMicrotask: QueueMicrotask;
    setInterval: SetInterval;
    clearInterval: ClearInterval;
    setImmediate: SetImmediate;
    clearImmediate: ClearImmediate;
    countTimers: CountTimers;
    requestAnimationFrame: RequestAnimationFrame;
    cancelAnimationFrame: CancelAnimationFrame;
    runMicrotasks: RunMicrotasks;
    tick: Tick;
    tickAsync: TickAsync;
    next: Next;
    nextAsync: NextAsync;
    runAll: RunAll;
    runToFrame: RunToFrame;
    runAllAsync: RunAllAsync;
    runToLast: RunToLast;
    runToLastAsync: RunToLastAsync;
    reset: Reset;
    setSystemTime: SetSystemTime;
    jump: Jump;
    performance: any;
    hrtime: Hrtime;
    uninstall: Uninstall;
    methods: string[];
    shouldClearNativeTimers?: boolean;
    timersModuleMethods: {
        methodName: string;
        original: unknown;
    }[] | undefined;
    timersPromisesModuleMethods: {
        methodName: string;
        original: unknown;
    }[] | undefined;
    abortListenerMap: Map<VoidVarArgsFunc, AbortSignal>;
    setTickMode: SetTickMode;
    timers?: Map<number, Timer>;
    timerHeap?: TimerHeap;
    duringTick?: boolean;
    isNearInfiniteLimit: boolean;
    attachedInterval?: TimerId;
    tickMode?: ClockTickMode;
    jobs?: Timer[];
    Intl?: IntlWithClock;
    Temporal?: any;
};
export type Config = {
    now?: number | Date | TemporalTimelike;
    toFake?: FakeMethod[];
    toNotFake?: FakeMethod[];
    loopLimit?: number;
    shouldAdvanceTime?: boolean;
    advanceTimeDelta?: number;
    shouldClearNativeTimers?: boolean;
    ignoreMissingTimers?: boolean;
    target?: GlobalObject;
};
export type Timer = TimerInitialProps;
export type NodeImmediateHasRef = () => boolean;
export type NodeImmediateRef = () => NodeImmediate;
export type NodeImmediateUnref = () => NodeImmediate;
export type NodeImmediate = {
    hasRef: NodeImmediateHasRef;
    ref: NodeImmediateRef;
    unref: NodeImmediateUnref;
};
export declare var timers: Timers;
export declare var createClock: CreateClockCallback;
export declare var install: InstallCallback;
export declare var withGlobal: WithGlobal;
