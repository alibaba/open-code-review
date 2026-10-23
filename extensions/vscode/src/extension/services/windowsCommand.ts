// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import * as fs from 'fs';
import * as path from 'path';

export interface ResolvedCommand {
  executable: string;
  args: string[];
}

export interface ParsedWindowsCmdShim {
  jsEntry: string;
  nodeHint?: string;
}

const win32 = path.win32;
const DEFAULT_PATH_EXTENSIONS = ['.COM', '.EXE', '.BAT', '.CMD'];

function getEnvironmentValue(env: NodeJS.ProcessEnv, name: string): string | undefined {
  const key = Object.keys(env).find((candidate) => candidate.toUpperCase() === name.toUpperCase());
  return key ? env[key] : undefined;
}

function stripQuotes(value: string): string {
  const trimmed = value.trim();
  if (trimmed.length >= 2 && trimmed.startsWith('"') && trimmed.endsWith('"')) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}

function pathExtensions(env: NodeJS.ProcessEnv): string[] {
  const configured = getEnvironmentValue(env, 'PATHEXT');
  const extensions = configured
    ? configured.split(';').map((entry) => entry.trim()).filter(Boolean)
    : DEFAULT_PATH_EXTENSIONS;
  return Array.from(new Set(extensions.map((entry) => entry.toLowerCase())));
}

function commandCandidates(command: string, env: NodeJS.ProcessEnv): string[] {
  const cleaned = stripQuotes(command);
  if (!cleaned) return [];
  if (win32.extname(cleaned)) return [cleaned];
  return [cleaned, ...pathExtensions(env).map((extension) => `${cleaned}${extension}`)];
}

function resolveCommandPath(command: string, env: NodeJS.ProcessEnv): string | null {
  const cleaned = stripQuotes(command);
  if (!cleaned) return null;

  if (win32.isAbsolute(cleaned) || cleaned.includes('/') || cleaned.includes('\\')) {
    for (const candidate of commandCandidates(cleaned, env)) {
      if (fs.existsSync(candidate)) return candidate;
    }
    return null;
  }

  const pathValue = getEnvironmentValue(env, 'PATH') ?? '';
  for (const directory of pathValue.split(';')) {
    if (!directory.trim()) continue;
    const normalizedDirectory = stripQuotes(directory);
    for (const candidate of commandCandidates(cleaned, env)) {
      const fullPath = win32.join(normalizedDirectory, candidate);
      if (fs.existsSync(fullPath)) return fullPath;
    }
  }
  return null;
}

interface VariableResolver {
  resolve(name: string, stack?: Set<string>): string | undefined;
}

function createVariableResolver(
  assignments: Map<string, string[]>,
  shimDirectory: string,
  env: NodeJS.ProcessEnv,
): VariableResolver {
  const resolveValue = (rawValue: string, stack: Set<string>): string => {
    let value = stripQuotes(rawValue);
    value = value.replace(/%~dp0/gi, shimDirectory);
    value = value.replace(/%([^%:]+)%/g, (match, name: string) => {
      const resolved = resolve(name, stack);
      return resolved ?? getEnvironmentValue(env, name) ?? match;
    });
    return value;
  };

  const resolve = (name: string, stack = new Set<string>()): string | undefined => {
    const normalizedName = name.toUpperCase();
    if (stack.has(normalizedName)) return undefined;
    const values = assignments.get(normalizedName);
    if (!values?.length) return undefined;

    const nextStack = new Set(stack);
    nextStack.add(normalizedName);
    let fallback: string | undefined;
    for (let index = values.length - 1; index >= 0; index -= 1) {
      const resolved = resolveValue(values[index], nextStack);
      if (!resolved || resolved.includes('%')) continue;
      if (win32.isAbsolute(resolved) && fs.existsSync(resolved)) return resolved;
      if (fallback === undefined) fallback = resolved;
    }
    return fallback;
  };

  return { resolve };
}

function tokenizeCommandLine(line: string): string[] {
  const tokens: string[] = [];
  const tokenPattern = /"([^"]*)"|(\S+)/g;
  let match: RegExpExecArray | null;
  while ((match = tokenPattern.exec(line)) !== null) {
    tokens.push(match[1] ?? match[2]);
  }
  return tokens;
}

function isNodeExecutableToken(token: string): boolean {
  return /(^|[\\/])node(?:\.exe)?$/i.test(token);
}

function toAbsoluteEntry(entry: string, shimDirectory: string): string {
  const normalized = entry.trim();
  return win32.isAbsolute(normalized)
    ? win32.normalize(normalized)
    : win32.resolve(shimDirectory, normalized);
}

/**
 * Parse the Node.js entry point and node executable hint from an npm-generated .cmd shim.
 * This reads the shim as data only; it never executes the batch file or invokes cmd.exe.
 */
export function parseWindowsCmdShim(
  contents: string,
  shimPath: string,
  env: NodeJS.ProcessEnv = process.env,
): ParsedWindowsCmdShim {
  const shimDirectory = win32.dirname(shimPath);
  const assignments = new Map<string, string[]>();

  for (const line of contents.split(/\r?\n/)) {
    const match = line.match(/^\s*@?\s*SET\s+(?:"([^"]+)=([^"]*)"|([^=\s]+)=(.*))\s*$/i);
    if (!match) continue;
    const name = (match[1] ?? match[3] ?? '').toUpperCase();
    const value = match[2] ?? match[4] ?? '';
    if (!name) continue;
    const values = assignments.get(name) ?? [];
    values.push(value);
    assignments.set(name, values);
  }

  const variables = createVariableResolver(assignments, shimDirectory, env);
  const invocationLines = contents
    .split(/\r?\n/)
    .filter((line) => /%(\*|[1-9])/.test(line) && !/^\s*@?\s*(?:REM|::)/i.test(line));

  for (const invocationLine of invocationLines) {
    let jsEntry: string | undefined;
    let nodeHint: string | undefined;
    for (const rawToken of tokenizeCommandLine(invocationLine)) {
      if (/^%[*1-9]$/.test(rawToken)) continue;
      let token = rawToken.replace(/%~dp0/gi, shimDirectory);
      token = token.replace(
        /%([^%:]+)%/g,
        (match, name: string) => variables.resolve(name) ?? match,
      );
      if (!token || token.includes('%')) continue;
      if (/\.js$/i.test(token)) jsEntry = toAbsoluteEntry(token, shimDirectory);
      if (isNodeExecutableToken(token)) nodeHint = token;
    }
    if (jsEntry) return { jsEntry, nodeHint };
  }

  throw new Error(`Windows command shim "${shimPath}" does not invoke a Node.js entry point.`);
}

function resolveNodeExecutable(
  nodeHint: string | undefined,
  shimDirectory: string,
  env: NodeJS.ProcessEnv,
): string {
  const candidates: string[] = [win32.join(shimDirectory, 'node.exe')];
  if (nodeHint) {
    candidates.push(
      win32.isAbsolute(nodeHint)
        ? nodeHint
        : win32.resolve(shimDirectory, nodeHint),
    );
  }

  for (const candidate of candidates) {
    if (fs.existsSync(candidate)) return candidate;
  }

  const fromPath = resolveCommandPath('node.exe', env);
  if (fromPath) return fromPath;

  throw new Error(`Unable to locate node.exe while resolving a Windows command shim in "${shimDirectory}".`);
}

/**
 * Resolve a Windows command to an executable and argument vector that can be spawned
 * without shell expansion. npm and ocr expose .cmd shims; their Node.js entry point is
 * invoked directly so user-controlled review arguments are never interpreted by cmd.exe.
 */
export function resolveWindowsCommand(
  command: string,
  args: string[],
  env: NodeJS.ProcessEnv,
): ResolvedCommand {
  const executable = resolveCommandPath(command, env);
  if (!executable) {
    throw new Error(`Unable to resolve Windows command "${command}" on PATH.`);
  }

  const extension = win32.extname(executable).toLowerCase();
  if (extension !== '.cmd' && extension !== '.bat') {
    return { executable, args };
  }

  const shim = fs.readFileSync(executable, 'utf8');
  const parsed = parseWindowsCmdShim(shim, executable, env);
  if (!fs.existsSync(parsed.jsEntry)) {
    throw new Error(`Windows command shim "${executable}" points to a missing entry point: ${parsed.jsEntry}`);
  }

  return {
    executable: resolveNodeExecutable(parsed.nodeHint, win32.dirname(executable), env),
    args: [parsed.jsEntry, ...args],
  };
}
