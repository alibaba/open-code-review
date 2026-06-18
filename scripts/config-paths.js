"use strict";

const path = require("path");
const os = require("os");

const CONFIG_DIR = path.join(os.homedir(), ".opencodereview");
const CONFIG_PATH = path.join(CONFIG_DIR, "config.json");

module.exports = { CONFIG_DIR, CONFIG_PATH };
