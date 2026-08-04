"use strict";

const SEMVER_RE = /^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/;

function parseVersionOutput(output) {
  const match = String(output || "").match(/v(\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.-]+)?)/);
  return match ? match[1] : null;
}

function semverGt(a, b) {
  const pa = a.replace(/-.*$/, "").split(".").map(Number);
  const pb = b.replace(/-.*$/, "").split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) > (pb[i] || 0)) return true;
    if ((pa[i] || 0) < (pb[i] || 0)) return false;
  }
  const aPre = a.includes("-");
  const bPre = b.includes("-");
  if (bPre && !aPre) return true;
  return false;
}

function shouldShowUpdateHint(hintVersion, installedVersion) {
  if (!SEMVER_RE.test(hintVersion)) return false;
  return !installedVersion || semverGt(hintVersion, installedVersion);
}

module.exports = {
  SEMVER_RE,
  parseVersionOutput,
  semverGt,
  shouldShowUpdateHint,
};
