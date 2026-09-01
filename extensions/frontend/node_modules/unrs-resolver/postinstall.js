const { checkAndPreparePackage } = require("napi-postinstall");

const packageJson = require("./package.json");

checkAndPreparePackage(packageJson, true);
