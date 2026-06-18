#!/usr/bin/env node
"use strict";

const fs = require("fs");
const { CONFIG_DIR, CONFIG_PATH } = require("./config-paths");

function info(msg) {
  console.log(`[INFO]  ${msg}`);
}

function error(msg) {
  console.error(`[ERROR] ${msg}`);
}

function loadConfig() {
  fs.mkdirSync(CONFIG_DIR, { recursive: true });
  if (fs.existsSync(CONFIG_PATH)) {
    try {
      return JSON.parse(fs.readFileSync(CONFIG_PATH, "utf8"));
    } catch (e) {
      error(`Failed to parse config.json: ${e.message}`);
      error("Please fix or remove the corrupted file before retrying.");
      process.exit(1);
    }
  }
  return {};
}

function saveConfig(config) {
  fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 4) + "\n");
}

function show() {
  const config = loadConfig();
  if (config.proxy && config.proxy.url) {
    info(`Current proxy: ${config.proxy.url}`);
  } else {
    info("No proxy configured.");
    info("Set one with: node scripts/setup-proxy.js set http://127.0.0.1:7897");
  }
}

function set(proxyUrl) {
  // Validate URL
  try {
    new URL(proxyUrl);
  } catch (_) {
    error(`Invalid URL: ${proxyUrl}`);
    process.exit(1);
  }

  const config = loadConfig();
  config.proxy = { url: proxyUrl };
  saveConfig(config);
  info(`Proxy set to: ${proxyUrl}`);
  info("");
  info("The following will use this proxy:");
  info("  - npm postinstall (install.js)");
  info("  - Auto-update checks (update.js)");
  info("  - Manual downloads via install.js");
  info("");
  info("You can also override it with the HTTPS_PROXY env var.");
}

function unset() {
  const config = loadConfig();
  if (config.proxy) {
    delete config.proxy;
    saveConfig(config);
    info("Proxy configuration removed.");
  } else {
    info("No proxy was configured.");
  }
}

function main() {
  const cmd = process.argv[2];
  const value = process.argv[3];

  switch (cmd) {
    case "set":
      if (!value) {
        error("Usage: node scripts/setup-proxy.js set <proxy-url>");
        error("Example: node scripts/setup-proxy.js set http://127.0.0.1:7897");
        process.exit(1);
      }
      set(value);
      break;
    case "unset":
      unset();
      break;
    case "show":
    case undefined:
      show();
      break;
    default:
      error(`Unknown command: ${cmd}`);
      error("Commands: set <url>, unset, show");
      process.exit(1);
  }
}

main();
