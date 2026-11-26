// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

"use strict";

const assert = require("assert");
const EventEmitter = require("events");
const https = require("https");

const { download, downloadText } = require("./install");

function fakeRequest() {
  const req = new EventEmitter();
  req.destroy = (error) => {
    queueMicrotask(() => req.emit("error", error));
  };
  return req;
}

async function testRequestTimeoutRejectsDownload() {
  const originalGet = https.get;
  const req = fakeRequest();
  let options;
  https.get = (url, requestOptions, callback) => {
    assert.strictEqual(url, "https://example.test/opencodereview");
    options = requestOptions;
    assert.strictEqual(typeof callback, "function");
    return req;
  };
  try {
    const pending = download("https://example.test/opencodereview");
    assert.ok(Number.isFinite(options.timeout) && options.timeout > 0, "the request must carry a positive timeout");
    req.emit("timeout");
    await assert.rejects(pending, /Timed out fetching https:\/\/example\.test\/opencodereview/);
  } finally {
    https.get = originalGet;
  }
}

async function testResponseStallRejectsDownload() {
  const originalGet = https.get;
  const originalSetTimeout = global.setTimeout;
  const originalClearTimeout = global.clearTimeout;
  const req = fakeRequest();
  const res = new EventEmitter();
  const timers = [];
  let responseCallback;
  res.statusCode = 200;
  res.headers = {};
  res.resume = () => {};
  res.destroy = (error) => {
    queueMicrotask(() => res.emit("error", error));
  };
  https.get = (url, requestOptions, callback) => {
    responseCallback = callback;
    return req;
  };
  global.setTimeout = (callback) => {
    timers.push(callback);
    return { callback };
  };
  global.clearTimeout = () => {};
  let pending;
  try {
    pending = downloadText("https://example.test/opencodereview");
    responseCallback(res);
    assert.ok(timers.length > 0, "a successful response must carry an inactivity watchdog");
    timers[timers.length - 1]();
  } finally {
    https.get = originalGet;
    global.setTimeout = originalSetTimeout;
    global.clearTimeout = originalClearTimeout;
  }
  await assert.rejects(pending, /Stalled while downloading https:\/\/example\.test\/opencodereview/);
}

async function main() {
  await testRequestTimeoutRejectsDownload();
  await testResponseStallRejectsDownload();
  console.log("install tests passed");
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
