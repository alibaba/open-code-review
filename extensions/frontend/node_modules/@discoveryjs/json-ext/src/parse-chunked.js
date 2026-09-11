import { isIterable } from './utils.js';

const NO_VALUE = Symbol('empty');
const STACK_OBJECT = 1;
const STACK_ARRAY = 2;
const MODE_JSON = 0;
const MODE_JSONL = 1;
const MODE_JSONL_AUTO = 2;
const decoder = new TextDecoder();

function adjustPosition(error, jsonParseOffset) {
    if (error.name === 'SyntaxError' && jsonParseOffset) {
        error.message = error.message.replace(/at position (\d+)/, (_, pos) =>
            'at position ' + (Number(pos) + jsonParseOffset)
        );
    }

    return error;
}

function append(array, elements) {
    // Note: Avoid using array.push(...elements) since it may lead to
    // "RangeError: Maximum call stack size exceeded" for long arrays
    const initialLength = array.length;
    array.length += elements.length;

    for (let i = 0; i < elements.length; i++) {
        array[initialLength + i] = elements[i];
    }
}

function resolveParseMode(mode) {
    switch (mode) {
        case 'json':
            return MODE_JSON;
        case 'jsonl':
            return MODE_JSONL;
        case 'auto':
            return MODE_JSONL_AUTO;
        default:
            throw new TypeError('Invalid options: `mode` should be "json", "jsonl", or "auto"');
    }
}

function parseChunkedOptions(value) {
    const options = typeof value === 'function'
        ? { reviver: value }
        : value || {};

    return {
        mode: resolveParseMode(options.mode ?? 'json'),
        reviver: options.reviver ?? null,
        onRootValue: options.onRootValue ?? null,
        onChunk: options.onChunk ?? null
    };
}

function applyReviver(value, reviver) {
    return walk({ '': value }, '', value);

    function walk(holder, key, value) {
        if (value && typeof value === 'object') {
            for (const childKey of Object.keys(value)) {
                const childValue = value[childKey];
                const newValue = walk(value, childKey, childValue);

                if (newValue === undefined) {
                    delete value[childKey];
                } else if (newValue !== childValue) {
                    value[childKey] = newValue;
                }
            }
        }

        return reviver.call(holder, key, value);
    }
}

export async function parseChunked(chunkEmitter, optionsOrReviver) {
    const { mode, reviver, onRootValue, onChunk } = parseChunkedOptions(optionsOrReviver);
    const iterable = typeof chunkEmitter === 'function'
        ? chunkEmitter()
        : chunkEmitter;

    if (isIterable(iterable)) {
        const parser = createChunkParser(mode, reviver, onRootValue, onChunk);

        try {
            for await (const chunk of iterable) {
                if (typeof chunk !== 'string' && !ArrayBuffer.isView(chunk)) {
                    throw new TypeError('Invalid chunk: Expected string, TypedArray or Buffer');
                }

                parser.push(chunk);
            }

            return parser.finish();
        } catch (e) {
            throw adjustPosition(e, parser.jsonParseOffset);
        }
    }

    throw new TypeError(
        'Invalid chunk emitter: Expected an Iterable, AsyncIterable, generator, ' +
        'async generator, or a function returning an Iterable or AsyncIterable'
    );
};

function createChunkParser(parseMode, reviver, onRootValue, onChunk) {
    let rootValues = parseMode === MODE_JSONL ? [] : null;
    let rootValuesCount = 0;
    let currentRootValue = NO_VALUE;
    let currentRootValueCursor = null;
    let consumedChunkLength = 0;
    let parsedChunkLength = 0;

    let prevArray = null;
    let prevArraySlices = [];

    let stack = new Array(100);
    let lastFlushDepth = 0;
    let flushDepth = 0;
    let stateString = false;
    let stateStringEscape = false;
    let seenNonWhiteSpace = false;
    let allowNewRootValue = true;
    let pendingByteSeq = null;
    let pendingChunk = null;
    let jsonParseOffset = 0;

    const state = Object.freeze({
        get mode() {
            return parseMode === MODE_JSONL ? 'jsonl' : 'json';
        },
        get returnValue() {
            return typeof onRootValue === 'function'
                ? rootValuesCount
                : rootValues !== null
                    ? rootValues
                    : currentRootValue !== NO_VALUE
                        ? currentRootValue
                        : undefined;
        },
        get currentRootValue() {
            return currentRootValue !== NO_VALUE ? currentRootValue : undefined;
        },
        get rootValuesCount() {
            return rootValuesCount;
        },
        get consumed() {
            return consumedChunkLength;
        },
        get parsed() {
            return parsedChunkLength;
        }
    });

    return {
        push,
        finish,
        state,
        get jsonParseOffset() {
            return jsonParseOffset;
        }
    };

    function startRootValue(fragment) {
        // Extra non-whitespace after complete root value should fail to parse
        if (!allowNewRootValue) {
            jsonParseOffset -= 2;
            JSON.parse('[]' + fragment);
        }

        // In "auto" mode, switch to JSONL when a second root value is starting after a newline
        if (currentRootValue !== NO_VALUE && parseMode === MODE_JSONL_AUTO) {
            parseMode = MODE_JSONL;
            rootValues = [currentRootValue];
        }

        // Block parsing of an additional root value until a newline is encountered
        allowNewRootValue = false;

        // Parse fragment as a new root value
        currentRootValue = JSON.parse(fragment);
    }

    function finishRootValue() {
        rootValuesCount++;

        if (typeof reviver === 'function') {
            currentRootValue = applyReviver(currentRootValue, reviver);
        }

        if (typeof onRootValue === 'function') {
            onRootValue(currentRootValue, state);
        } else if (parseMode === MODE_JSONL) {
            rootValues.push(currentRootValue);
        }
    }

    function mergeArraySlices() {
        if (prevArray === null) {
            return;
        }

        if (prevArraySlices.length !== 0) {
            const newArray = prevArraySlices.length === 1
                ? prevArray.concat(prevArraySlices[0])
                : prevArray.concat(...prevArraySlices);

            if (currentRootValueCursor.prev !== null) {
                currentRootValueCursor.prev.value[currentRootValueCursor.key] = newArray;
            } else {
                currentRootValue = newArray;
            }

            currentRootValueCursor.value = newArray;
            prevArraySlices = [];
        }

        prevArray = null;
    }

    function parseAndAppend(fragment, wrap) {
        // Append new entries or elements
        if (stack[lastFlushDepth - 1] === STACK_OBJECT) {
            if (wrap) {
                jsonParseOffset--;
                fragment = '{' + fragment + '}';
            }

            Object.assign(currentRootValueCursor.value, JSON.parse(fragment));
        } else {
            if (wrap) {
                jsonParseOffset--;
                fragment = '[' + fragment + ']';
            }

            if (prevArray === currentRootValueCursor.value) {
                prevArraySlices.push(JSON.parse(fragment));
            } else {
                append(currentRootValueCursor.value, JSON.parse(fragment));
                prevArray = currentRootValueCursor.value;
            }
        }
    }

    function prepareAddition(fragment) {
        const { value } = currentRootValueCursor;
        const expectComma = Array.isArray(value)
            ? value.length !== 0
            : Object.keys(value).length !== 0;

        if (expectComma) {
            // Skip a comma at the beginning of fragment, otherwise it would
            // fail to parse
            if (fragment[0] === ',') {
                jsonParseOffset++;
                return fragment.slice(1);
            }

            // When value (an object or array) is not empty and a fragment
            // doesn't start with a comma, a single valid fragment starting
            // is a closing bracket. If it's not, a prefix is adding to fail
            // parsing. Otherwise, the sequence of chunks can be successfully
            // parsed, although it should not, e.g. ["[{}", "{}]"]
            if (fragment[0] !== '}' && fragment[0] !== ']') {
                jsonParseOffset -= 3;
                return '[[]' + fragment;
            }
        }

        return fragment;
    }

    function flush(chunk, start, end) {
        let fragment = chunk.slice(start, end);

        // Save position correction for an error in JSON.parse() if any
        jsonParseOffset = consumedChunkLength + start;
        parsedChunkLength += end - start;

        // Prepend pending chunk if any
        if (pendingChunk !== null) {
            fragment = pendingChunk + fragment;
            jsonParseOffset -= pendingChunk.length;
            parsedChunkLength += pendingChunk.length;
            pendingChunk = null;
        }

        if (flushDepth === lastFlushDepth) {
            // Depth didn't change, so it's a continuation of the current value or entire value if it's a root one
            if (lastFlushDepth === 0) {
                startRootValue(fragment);
            } else {
                parseAndAppend(prepareAddition(fragment), true);
            }
        } else if (flushDepth > lastFlushDepth) {
            // Add missed closing brackets/parentheses
            for (let i = flushDepth - 1; i >= lastFlushDepth; i--) {
                fragment += stack[i] === STACK_OBJECT ? '}' : ']';
            }

            if (lastFlushDepth === 0) {
                startRootValue(fragment);
                currentRootValueCursor = {
                    value: currentRootValue,
                    key: null,
                    prev: null
                };
            } else {
                parseAndAppend(prepareAddition(fragment), true);
                mergeArraySlices();
            }

            // Move down to the depths to the last object/array, which is current now
            for (let i = lastFlushDepth || 1; i < flushDepth; i++) {
                let { value } = currentRootValueCursor;
                let key = null;

                if (stack[i - 1] === STACK_OBJECT) {
                    // Find last entry
                    // eslint-disable-next-line curly
                    for (key in value);
                    value = value[key];
                } else {
                    // Last element
                    key = value.length - 1;
                    value = value[key];
                }

                currentRootValueCursor = {
                    value,
                    key,
                    prev: currentRootValueCursor
                };
            }
        } else /* flushDepth < lastFlushDepth */ {
            fragment = prepareAddition(fragment);

            // Add missed opening brackets/parentheses
            for (let i = lastFlushDepth - 1; i >= flushDepth; i--) {
                jsonParseOffset--;
                fragment = (stack[i] === STACK_OBJECT ? '{' : '[') + fragment;
            }

            parseAndAppend(fragment, false);
            mergeArraySlices();

            for (let i = lastFlushDepth - 1; i >= flushDepth; i--) {
                currentRootValueCursor = currentRootValueCursor.prev;
            }
        }

        if (flushDepth === 0) {
            finishRootValue();
        }

        lastFlushDepth = flushDepth;
        seenNonWhiteSpace = false;
    }

    function ensureChunkString(chunk) {
        if (typeof chunk !== 'string') {
            // Suppose chunk is Buffer or Uint8Array

            // Prepend uncompleted byte sequence if any
            if (pendingByteSeq !== null) {
                const origRawChunk = chunk;
                chunk = new Uint8Array(pendingByteSeq.length + origRawChunk.length);
                chunk.set(pendingByteSeq);
                chunk.set(origRawChunk, pendingByteSeq.length);
                pendingByteSeq = null;
            }

            // In case Buffer/Uint8Array, an input is encoded in UTF8
            // Seek for parts of uncompleted UTF8 symbol on the ending
            // This makes sense only if we expect more chunks and last char is not multi-bytes
            if (chunk[chunk.length - 1] > 127) {
                for (let seqLength = 0; seqLength < chunk.length; seqLength++) {
                    const byte = chunk[chunk.length - 1 - seqLength];

                    // 10xxxxxx - 2nd, 3rd or 4th byte
                    // 110xxxxx – first byte of 2-byte sequence
                    // 1110xxxx - first byte of 3-byte sequence
                    // 11110xxx - first byte of 4-byte sequence
                    if (byte >> 6 === 3) {
                        seqLength++;

                        // If the sequence is really incomplete, then preserve it
                        // for the future chunk and cut off it from the current chunk
                        if ((seqLength !== 4 && byte >> 3 === 0b11110) ||
                            (seqLength !== 3 && byte >> 4 === 0b1110) ||
                            (seqLength !== 2 && byte >> 5 === 0b110)) {
                            pendingByteSeq = chunk.slice(chunk.length - seqLength); // use slice to avoid tying chunk
                            chunk = chunk.subarray(0, -seqLength); // use subarray to avoid buffer copy
                        }

                        break;
                    }
                }
            }

            // Convert chunk to a string, since single decode per chunk
            // is much effective than decode multiple small substrings
            chunk = decoder.decode(chunk);
        }

        return chunk;
    }

    function push(chunk) {
        chunk = ensureChunkString(chunk);

        const chunkLength = chunk.length;
        const prevParsedChunkLength = parsedChunkLength;
        let lastFlushPoint = 0;
        let flushPoint = 0;

        // Main scan loop
        scan: for (let i = 0; i < chunkLength; i++) {
            if (stateString) {
                for (; i < chunkLength; i++) {
                    if (stateStringEscape) {
                        stateStringEscape = false;
                    } else {
                        switch (chunk.charCodeAt(i)) {
                            case 0x22: /* " */
                                stateString = false;
                                continue scan;

                            case 0x5C: /* \ */
                                stateStringEscape = true;
                        }
                    }
                }

                break;
            }

            switch (chunk.charCodeAt(i)) {
                case 0x22: /* " */
                    stateString = true;
                    stateStringEscape = false;
                    seenNonWhiteSpace = true;
                    break;

                case 0x2C: /* , */
                    flushPoint = i;
                    break;

                case 0x7B: /* { */
                    // Open an object
                    flushPoint = i + 1;
                    stack[flushDepth++] = STACK_OBJECT;
                    seenNonWhiteSpace = true;
                    break;

                case 0x5B: /* [ */
                    // Open an array
                    flushPoint = i + 1;
                    stack[flushDepth++] = STACK_ARRAY;
                    seenNonWhiteSpace = true;
                    break;

                case 0x5D: /* ] */
                case 0x7D: /* } */
                    // Close an object or array
                    flushPoint = i + 1;

                    if (flushDepth === 0) {
                        // Unmatched closing bracket/brace at top level, should fail to parse
                        break scan;
                    }

                    flushDepth--;

                    // Flush on depth decrease related to last flush, otherwise wait for more chunks to flush together
                    if (flushDepth < lastFlushDepth) {
                        flush(chunk, lastFlushPoint, flushPoint);
                        lastFlushPoint = flushPoint;
                    }

                    break;

                case 0x09: /* \t */
                case 0x0A: /* \n */
                case 0x0D: /* \r */
                case 0x20: /* space */
                    if (flushDepth === 0) {
                        if (seenNonWhiteSpace) {
                            flushPoint = i;
                            flush(chunk, lastFlushPoint, flushPoint);
                            lastFlushPoint = flushPoint;
                        }

                        if (parseMode !== MODE_JSON &&
                            allowNewRootValue === false &&
                            (chunk.charCodeAt(i) === 0x0A || chunk.charCodeAt(i) === 0x0D)
                        ) {
                            allowNewRootValue = true;
                        }

                        if (flushPoint === i) {
                            parsedChunkLength++;
                        }
                    }

                    // Move points forward when they point to current position and it's a whitespace
                    if (lastFlushPoint === i) {
                        lastFlushPoint++;
                    }

                    if (flushPoint === i) {
                        flushPoint++;
                    }

                    break;

                default:
                    seenNonWhiteSpace = true;
            }
        }

        if (flushPoint > lastFlushPoint) {
            flush(chunk, lastFlushPoint, flushPoint);
        }

        // Produce pendingChunk if something left
        if (flushPoint < chunkLength) {
            if (pendingChunk !== null) {
                // When there is already a pending chunk then no flush happened,
                // appending entire chunk to pending one
                pendingChunk += chunk;
            } else {
                // Create a pending chunk, it will start with non-whitespace since
                // flushPoint was moved forward away from whitespaces on scan
                pendingChunk = chunk.slice(flushPoint, chunkLength);
            }
        }

        consumedChunkLength += chunkLength;

        if (typeof onChunk === 'function') {
            onChunk(parsedChunkLength - prevParsedChunkLength, chunk, pendingChunk, state);
        }
    }

    function finish() {
        if (pendingChunk !== null || (currentRootValue === NO_VALUE && parseMode !== MODE_JSONL)) {
            // Force the `flushDepth < lastFlushDepth` branch in flush() to prepend missed
            // opening brackets/parentheses and produce a natural JSON.parse() EOF error
            flushDepth = 0;
            flush('', 0, 0);
        }

        if (typeof onChunk === 'function') {
            parsedChunkLength = consumedChunkLength;
            onChunk(0, null, null, state);
        }

        const result = state.returnValue;

        rootValues = null;
        currentRootValue = NO_VALUE;

        return result;
    }
}
