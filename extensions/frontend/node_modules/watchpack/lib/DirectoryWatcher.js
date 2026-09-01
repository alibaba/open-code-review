/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/
"use strict";

const { EventEmitter } = require("events");
const path = require("path");
const fs = require("graceful-fs");

const watchEventSource = require("./watchEventSource");

/** @typedef {import("./index").IgnoredFunction} IgnoredFunction */
/** @typedef {import("./index").EventType} EventType */
/** @typedef {import("./index").TimeInfoEntries} TimeInfoEntries */
/** @typedef {import("./index").Entry} Entry */
/** @typedef {import("./index").ExistenceOnlyTimeEntry} ExistenceOnlyTimeEntry */
/** @typedef {import("./index").OnlySafeTimeEntry} OnlySafeTimeEntry */
/** @typedef {import("./index").EventMap} EventMap */
/** @typedef {import("./getWatcherManager").WatcherManager} WatcherManager */
/** @typedef {import("./watchEventSource").Watcher} EventSourceWatcher */

/** @type {ExistenceOnlyTimeEntry} */
const EXISTANCE_ONLY_TIME_ENTRY = Object.freeze({});

let FS_ACCURACY = 2000;

const IS_OSX = require("os").platform() === "darwin";
const IS_WIN = require("os").platform() === "win32";

const { WATCHPACK_POLLING, WATCHPACK_RETRIES } = process.env;
const FORCE_POLLING =
	// @ts-expect-error avoid additional checks
	`${+WATCHPACK_POLLING}` === WATCHPACK_POLLING
		? +WATCHPACK_POLLING
		: Boolean(WATCHPACK_POLLING) && WATCHPACK_POLLING !== "false";

// Number of retries (and delay between retries, in ms) when an fs operation
// returns EBUSY. EBUSY is transient on Windows when an AV scanner, indexer,
// or another process briefly locks a file; retrying avoids incorrectly
// reporting the file as removed (see webpack/watchpack#223, #44).
//
// Configurable via `WATCHPACK_RETRIES` env var: an integer >= 0, or "false"
// to disable retries entirely. Unset / invalid values fall back to 3.
const BUSY_RETRIES = (() => {
	if (WATCHPACK_RETRIES === undefined) return 3;
	if (WATCHPACK_RETRIES === "false") return 0;
	const n = Number(WATCHPACK_RETRIES);
	return Number.isFinite(n) && n >= 0 ? Math.floor(n) : 3;
})();
const BUSY_RETRY_DELAY = 100;

/**
 * @param {string} str string
 * @returns {string} lower cased string
 */
function withoutCase(str) {
	return str.toLowerCase();
}

/**
 * @param {number} times times
 * @param {() => void} callback callback
 * @returns {() => void} result
 */
function needCalls(times, callback) {
	return function needCallsCallback() {
		if (--times === 0) {
			return callback();
		}
	};
}

/**
 * @param {Entry} entry entry
 */
function fixupEntryAccuracy(entry) {
	if (entry.accuracy > FS_ACCURACY) {
		entry.safeTime = entry.safeTime - entry.accuracy + FS_ACCURACY;
		entry.accuracy = FS_ACCURACY;
	}
}

/**
 * @param {number=} mtime mtime
 */
function ensureFsAccuracy(mtime) {
	if (!mtime) return;
	if (FS_ACCURACY > 1 && mtime % 1 !== 0) FS_ACCURACY = 1;
	else if (FS_ACCURACY > 10 && mtime % 10 !== 0) FS_ACCURACY = 10;
	else if (FS_ACCURACY > 100 && mtime % 100 !== 0) FS_ACCURACY = 100;
	else if (FS_ACCURACY > 1000 && mtime % 1000 !== 0) FS_ACCURACY = 1000;
}

/**
 * Call `fs.lstat` with retries on EBUSY. Transient EBUSY errors are common
 * on Windows when another process (AV scanner, indexer, editor) holds an
 * open handle on the file. See webpack/watchpack#223, #44.
 *
 * The retry count is taken from the `WATCHPACK_RETRIES` env var (default
 * 3, set to "0" or "false" to disable retrying). The hot path is a single
 * `fs.lstat` call with one inline callback; the timer and the recursive
 * call are only scheduled when an EBUSY is actually observed.
 * @param {string} target target path
 * @param {{ closed: boolean }} watcher owning watcher (checked between retries)
 * @param {(err: NodeJS.ErrnoException | null, stats: import("fs").Stats) => void} callback callback
 * @param {number=} remaining retries remaining (defaults to `BUSY_RETRIES`)
 */
function lstatWithRetry(target, watcher, callback, remaining = BUSY_RETRIES) {
	fs.lstat(target, (err, stats) => {
		if (
			err &&
			/** @type {NodeJS.ErrnoException} */ (err).code === "EBUSY" &&
			remaining > 0 &&
			!watcher.closed
		) {
			setTimeout(
				() => lstatWithRetry(target, watcher, callback, remaining - 1),
				BUSY_RETRY_DELAY,
			);
			return;
		}
		callback(err, stats);
	});
}

/**
 * @typedef {object} FileWatcherEvents
 * @property {(type: EventType) => void} initial-missing initial missing event
 * @property {(mtime: number, type: EventType, initial: boolean) => void} change change event
 * @property {(type: EventType) => void} remove remove event
 * @property {() => void} closed closed event
 */

/**
 * @typedef {object} DirectoryWatcherEvents
 * @property {(type: EventType) => void} initial-missing initial missing event
 * @property {((file: string, mtime: number, type: EventType, initial: boolean) => void)} change change event
 * @property {(type: EventType) => void} remove remove event
 * @property {() => void} closed closed event
 */

/**
 * @template {EventMap} T
 * @extends {EventEmitter<{ [K in keyof T]: Parameters<T[K]> }>}
 */
class Watcher extends EventEmitter {
	/**
	 * @param {DirectoryWatcher} directoryWatcher a directory watcher
	 * @param {string} target a target to watch
	 * @param {number=} startTime start time
	 */
	constructor(directoryWatcher, target, startTime) {
		super();
		this.directoryWatcher = directoryWatcher;
		this.path = target;
		this.startTime = startTime && +startTime;
	}

	/**
	 * @param {number} mtime mtime
	 * @param {boolean} initial true when initial, otherwise false
	 * @returns {boolean} true of start time less than mtile, otherwise false
	 */
	checkStartTime(mtime, initial) {
		const { startTime } = this;
		if (typeof startTime !== "number") return !initial;
		return startTime <= mtime;
	}

	close() {
		// @ts-expect-error bad typing in EventEmitter
		this.emit("closed");
	}
}

/** @typedef {Set<string>} InitialScanRemoved */

/**
 * @typedef {object} WatchpackEvents
 * @property {(target: string, mtime: string, type: EventType, initial: boolean) => void} change change event
 * @property {() => void} closed closed event
 */

/**
 * @typedef {object} DirectoryWatcherOptions
 * @property {boolean=} followSymlinks true when need to resolve symlinks and watch symlink and real file, otherwise false
 * @property {IgnoredFunction=} ignored ignore some files from watching (glob pattern or regexp)
 * @property {number | boolean=} poll true when need to enable polling mode for watching, otherwise false
 */

/**
 * @extends {EventEmitter<{ [K in keyof WatchpackEvents]: Parameters<WatchpackEvents[K]> }>}
 */
class DirectoryWatcher extends EventEmitter {
	/**
	 * @param {WatcherManager} watcherManager a watcher manager
	 * @param {string} directoryPath directory path
	 * @param {DirectoryWatcherOptions=} options options
	 */
	constructor(watcherManager, directoryPath, options = {}) {
		super();
		if (FORCE_POLLING) {
			options.poll = FORCE_POLLING;
		}
		this.watcherManager = watcherManager;
		this.options = options;
		this.path = directoryPath;
		// safeTime is the point in time after which reading is safe to be unchanged
		// timestamp is a value that should be compared with another timestamp (mtime)
		/** @type {Map<string, Entry>} */
		this.files = new Map();
		/** @type {Map<string, number>} */
		this.filesWithoutCase = new Map();
		/** @type {Map<string, Watcher<DirectoryWatcherEvents> | boolean>} */
		this.directories = new Map();
		/** @type {Map<string, Watcher<FileWatcherEvents>>} */
		this._symlinkTargetWatchers = new Map();
		this.lastWatchEvent = 0;
		this.initialScan = true;
		this.ignored = options.ignored || (() => false);
		this.nestedWatching = false;
		/** @type {number | false} */
		this.polledWatching =
			typeof options.poll === "number"
				? options.poll
				: options.poll
					? 5007
					: false;
		/** @type {undefined | NodeJS.Timeout} */
		this.timeout = undefined;
		/** @type {null | InitialScanRemoved} */
		this.initialScanRemoved = new Set();
		/** @type {undefined | number} */
		this.initialScanFinished = undefined;
		/** @type {Map<string, Set<Watcher<DirectoryWatcherEvents> | Watcher<FileWatcherEvents>>>} */
		this.watchers = new Map();
		/** @type {Watcher<FileWatcherEvents> | null} */
		this.parentWatcher = null;
		this.refs = 0;
		/** @type {Map<string, boolean>} */
		this._activeEvents = new Map();
		this.closed = false;
		this.scanning = false;
		this.scanAgain = false;
		this.scanAgainInitial = false;

		this.createWatcher();
		this.doScan(true);
	}

	createWatcher() {
		try {
			if (this.polledWatching) {
				/** @type {EventSourceWatcher} */
				(this.watcher) = /** @type {EventSourceWatcher} */ ({
					close: () => {
						if (this.timeout) {
							clearTimeout(this.timeout);
							this.timeout = undefined;
						}
					},
				});
			} else {
				if (IS_OSX) {
					this.watchInParentDirectory();
				}
				this.watcher =
					/** @type {EventSourceWatcher} */
					(watchEventSource.watch(this.path));
				this.watcher.on("change", this.onWatchEvent.bind(this));
				this.watcher.on("error", this.onWatcherError.bind(this));
			}
		} catch (err) {
			this.onWatcherError(err);
		}
	}

	/**
	 * @template {(watcher: Watcher<EventMap>) => void} T
	 * @param {string} path path
	 * @param {T} fn function
	 */
	forEachWatcher(path, fn) {
		const watchers = this.watchers.get(withoutCase(path));
		if (watchers !== undefined) {
			for (const w of watchers) {
				fn(w);
			}
		}
	}

	/**
	 * @param {string} itemPath an item path
	 * @param {boolean} initial true when initial, otherwise false
	 * @param {EventType} type even type
	 */
	setMissing(itemPath, initial, type) {
		if (this.initialScan) {
			/** @type {InitialScanRemoved} */
			(this.initialScanRemoved).add(itemPath);
		}

		const oldDirectory = this.directories.get(itemPath);
		if (oldDirectory) {
			if (this.nestedWatching) {
				/** @type {Watcher<DirectoryWatcherEvents>} */
				(oldDirectory).close();
			}
			this.directories.delete(itemPath);
			this.forEachWatcher(itemPath, (w) => w.emit("remove", type));
			if (!initial) {
				this.forEachWatcher(this.path, (w) =>
					w.emit("change", itemPath, null, type, initial),
				);
			}
		}

		const oldFile = this.files.get(itemPath);
		if (oldFile) {
			this.files.delete(itemPath);
			const key = withoutCase(itemPath);
			const count = /** @type {number} */ (this.filesWithoutCase.get(key)) - 1;
			if (count <= 0) {
				this.filesWithoutCase.delete(key);
				this.forEachWatcher(itemPath, (w) => w.emit("remove", type));
			} else {
				this.filesWithoutCase.set(key, count);
			}

			if (!initial) {
				this.forEachWatcher(this.path, (w) =>
					w.emit("change", itemPath, null, type, initial),
				);
			}
		}
	}

	/**
	 * @param {string} target a target to set file time
	 * @param {number} mtime mtime
	 * @param {boolean} initial true when initial, otherwise false
	 * @param {boolean} ignoreWhenEqual true to ignore when equal, otherwise false
	 * @param {EventType} type type
	 */
	setFileTime(target, mtime, initial, ignoreWhenEqual, type) {
		const now = Date.now();

		if (this.ignored(target)) return;

		const old = this.files.get(target);

		let safeTime;
		let accuracy;
		if (initial) {
			safeTime = Math.min(now, mtime) + FS_ACCURACY;
			accuracy = FS_ACCURACY;
		} else {
			safeTime = now;
			accuracy = 0;

			if (old && old.timestamp === mtime && mtime + FS_ACCURACY < now) {
				// We are sure that mtime is untouched
				// This can be caused by some file attribute change
				// e. g. when access time has been changed
				// but the file content is untouched
				return;
			}
		}

		if (ignoreWhenEqual && old && old.timestamp === mtime) return;

		this.files.set(target, {
			safeTime,
			accuracy,
			timestamp: mtime,
		});

		if (!old) {
			const key = withoutCase(target);
			const count = this.filesWithoutCase.get(key);
			this.filesWithoutCase.set(key, (count || 0) + 1);
			if (count !== undefined) {
				// There is already a file with case-insensitive-equal name
				// On a case-insensitive filesystem we may miss the renaming
				// when only casing is changed.
				// To be sure that our information is correct
				// we trigger a rescan here
				this.doScan(false);
			}

			this.forEachWatcher(target, (w) => {
				if (!initial || w.checkStartTime(safeTime, initial)) {
					w.emit("change", mtime, type);
				}
			});
		} else if (!initial) {
			this.forEachWatcher(target, (w) => w.emit("change", mtime, type));
		}
		this.forEachWatcher(this.path, (w) => {
			if (!initial || w.checkStartTime(safeTime, initial)) {
				w.emit("change", target, safeTime, type, initial);
			}
		});
	}

	/**
	 * @param {string} directoryPath directory path
	 * @param {number} birthtime birthtime
	 * @param {boolean} initial true when initial, otherwise false
	 * @param {EventType} type even type
	 */
	setDirectory(directoryPath, birthtime, initial, type) {
		if (this.ignored(directoryPath)) return;
		if (directoryPath === this.path) {
			if (!initial) {
				this.forEachWatcher(this.path, (w) =>
					w.emit("change", directoryPath, birthtime, type, initial),
				);
			}
		} else {
			const old = this.directories.get(directoryPath);
			if (!old) {
				const now = Date.now();

				if (this.nestedWatching) {
					this.createNestedWatcher(directoryPath);
				} else {
					this.directories.set(directoryPath, true);
				}

				const safeTime = initial ? Math.min(now, birthtime) + FS_ACCURACY : now;

				this.forEachWatcher(directoryPath, (w) => {
					if (!initial || w.checkStartTime(safeTime, false)) {
						w.emit("change", birthtime, type);
					}
				});
				this.forEachWatcher(this.path, (w) => {
					if (!initial || w.checkStartTime(safeTime, initial)) {
						w.emit("change", directoryPath, safeTime, type, initial);
					}
				});
			}
		}
	}

	/**
	 * @param {string} directoryPath directory path
	 */
	createNestedWatcher(directoryPath) {
		const watcher = this.watcherManager.watchDirectory(directoryPath, 1);
		watcher.on("change", (target, mtime, type, initial) => {
			this.forEachWatcher(this.path, (w) => {
				if (!initial || w.checkStartTime(mtime, initial)) {
					w.emit("change", target, mtime, type, initial);
				}
			});
		});
		this.directories.set(directoryPath, watcher);
	}

	/**
	 * @param {boolean} flag true when nested, otherwise false
	 */
	setNestedWatching(flag) {
		if (this.nestedWatching !== Boolean(flag)) {
			this.nestedWatching = Boolean(flag);
			if (this.nestedWatching) {
				for (const directory of this.directories.keys()) {
					this.createNestedWatcher(directory);
				}
			} else {
				for (const [directory, watcher] of this.directories) {
					/** @type {Watcher<DirectoryWatcherEvents>} */
					(watcher).close();
					this.directories.set(directory, true);
				}
				for (const w of this._symlinkTargetWatchers.values()) {
					w.close();
				}
				this._symlinkTargetWatchers.clear();
			}
		}
	}

	/**
	 * @param {string} target a target to watch
	 * @param {number=} startTime start time
	 * @returns {Watcher<DirectoryWatcherEvents> | Watcher<FileWatcherEvents>} watcher
	 */
	watch(target, startTime) {
		const key = withoutCase(target);
		let watchers = this.watchers.get(key);
		if (watchers === undefined) {
			watchers = new Set();
			this.watchers.set(key, watchers);
		}
		this.refs++;
		const watcher =
			/** @type {Watcher<DirectoryWatcherEvents> | Watcher<FileWatcherEvents>} */
			(new Watcher(this, target, startTime));
		watcher.on("closed", () => {
			if (--this.refs <= 0) {
				this.close();
				return;
			}
			watchers.delete(watcher);
			if (watchers.size === 0) {
				this.watchers.delete(key);
				if (this.path === target) this.setNestedWatching(false);
			}
		});
		watchers.add(watcher);
		let safeTime;
		if (target === this.path) {
			this.setNestedWatching(true);
			safeTime = this.lastWatchEvent;
			for (const entry of this.files.values()) {
				fixupEntryAccuracy(entry);
				safeTime = Math.max(safeTime, entry.safeTime);
			}
		} else {
			const entry = this.files.get(target);
			if (entry) {
				fixupEntryAccuracy(entry);
				safeTime = entry.safeTime;
			} else {
				safeTime = 0;
			}
		}
		if (safeTime) {
			if (startTime && safeTime >= startTime) {
				process.nextTick(() => {
					if (this.closed) return;
					if (target === this.path) {
						/** @type {Watcher<DirectoryWatcherEvents>} */
						(watcher).emit(
							"change",
							target,
							safeTime,
							"watch (outdated on attach)",
							true,
						);
					} else {
						/** @type {Watcher<FileWatcherEvents>} */
						(watcher).emit(
							"change",
							safeTime,
							"watch (outdated on attach)",
							true,
						);
					}
				});
			}
		} else if (this.initialScan) {
			if (
				/** @type {InitialScanRemoved} */
				(this.initialScanRemoved).has(target)
			) {
				process.nextTick(() => {
					if (this.closed) return;
					watcher.emit("remove");
				});
			}
		} else if (
			target !== this.path &&
			!this.directories.has(target) &&
			watcher.checkStartTime(
				/** @type {number} */
				(this.initialScanFinished),
				false,
			)
		) {
			process.nextTick(() => {
				if (this.closed) return;
				watcher.emit("initial-missing", "watch (missing on attach)");
			});
		}
		return watcher;
	}

	/**
	 * @param {EventType} eventType event type
	 * @param {string=} filename filename
	 */
	onWatchEvent(eventType, filename) {
		if (this.closed) return;
		if (!filename) {
			// In some cases no filename is provided
			// This seem to happen on windows
			// So some event happened but we don't know which file is affected
			// We have to do a full scan of the directory
			this.doScan(false);
			return;
		}

		const target = path.join(this.path, filename);
		if (this.ignored(target)) return;

		if (this._activeEvents.get(filename) === undefined) {
			this._activeEvents.set(filename, false);
			const checkStats = () => {
				if (this.closed) return;
				this._activeEvents.set(filename, false);
				lstatWithRetry(target, this, (err, stats) => {
					if (this.closed) return;
					if (this._activeEvents.get(filename) === true) {
						process.nextTick(checkStats);
						return;
					}
					this._activeEvents.delete(filename);
					// ENOENT happens when the file/directory doesn't exist
					// EPERM happens when the containing directory doesn't exist
					// EBUSY happens when another process has the file locked (e.g.
					// Windows AV scanner). lstatWithRetry already retried before
					// giving up here.
					if (err) {
						if (
							err.code !== "ENOENT" &&
							err.code !== "EPERM" &&
							err.code !== "EBUSY"
						) {
							this.onStatsError(err);
						} else if (
							filename === path.basename(this.path) && // This may indicate that the directory itself was removed
							!fs.existsSync(this.path)
						) {
							this.onDirectoryRemoved("stat failed");
						}
					}
					this.lastWatchEvent = Date.now();
					if (!stats) {
						// On EBUSY we keep the tracked state: the file is likely still
						// there and a later event or scan will reconcile. Emitting a
						// remove here would make the watch appear to stop after a
						// transient lock (webpack/watchpack#223, #44).
						if (err && err.code === "EBUSY" && BUSY_RETRIES > 0) {
							return;
						}
						this.setMissing(target, false, eventType);
					} else if (stats.isDirectory()) {
						this.setDirectory(target, +stats.birthtime || 1, false, eventType);
					} else if (stats.isFile() || stats.isSymbolicLink()) {
						if (stats.mtime) {
							ensureFsAccuracy(+stats.mtime);
						}
						this.setFileTime(
							target,
							+stats.mtime || +stats.ctime || 1,
							false,
							false,
							eventType,
						);
					}
				});
			};
			process.nextTick(checkStats);
		} else {
			this._activeEvents.set(filename, true);
		}
	}

	/**
	 * @param {unknown=} err error
	 */
	onWatcherError(err) {
		if (this.closed) return;
		if (err) {
			if (
				/** @type {NodeJS.ErrnoException} */
				(err).code !== "EPERM" &&
				/** @type {NodeJS.ErrnoException} */
				(err).code !== "ENOENT"
			) {
				// eslint-disable-next-line no-console
				console.error(`Watchpack Error (watcher): ${err}`);
			}
			this.onDirectoryRemoved("watch error");
		}
	}

	/**
	 * @param {Error | NodeJS.ErrnoException=} err error
	 */
	onStatsError(err) {
		if (err) {
			// eslint-disable-next-line no-console
			console.error(`Watchpack Error (stats): ${err}`);
		}
	}

	/**
	 * @param {Error | NodeJS.ErrnoException=} err error
	 */
	onScanError(err) {
		if (err) {
			// eslint-disable-next-line no-console
			console.error(`Watchpack Error (initial scan): ${err}`);
		}
		this.onScanFinished();
	}

	onScanFinished() {
		if (this.polledWatching) {
			this.timeout = setTimeout(() => {
				if (this.closed) return;
				this.doScan(false);
			}, this.polledWatching);
		}
	}

	/**
	 * @param {string} reason a reason
	 */
	onDirectoryRemoved(reason) {
		if (this.watcher) {
			this.watcher.close();
			this.watcher = null;
		}
		this.watchInParentDirectory();
		const type = /** @type {EventType} */ (`directory-removed (${reason})`);
		for (const directory of this.directories.keys()) {
			this.setMissing(directory, false, type);
		}
		for (const file of this.files.keys()) {
			this.setMissing(file, false, type);
		}
	}

	watchInParentDirectory() {
		if (!this.parentWatcher) {
			const parentDir = path.dirname(this.path);
			// avoid watching in the root directory
			// removing directories in the root directory is not supported
			if (path.dirname(parentDir) === parentDir) return;

			this.parentWatcher = this.watcherManager.watchFile(this.path, 1);
			/** @type {Watcher<FileWatcherEvents>} */
			(this.parentWatcher).on("change", (mtime, type) => {
				if (this.closed) return;

				// On non-osx platforms we don't need this watcher to detect
				// directory removal, as an EPERM error indicates that
				if ((!IS_OSX || this.polledWatching) && this.parentWatcher) {
					this.parentWatcher.close();
					this.parentWatcher = null;
				}
				// Try to create the watcher when parent directory is found
				if (!this.watcher) {
					this.createWatcher();
					this.doScan(false);

					// directory was created so we emit an event
					this.forEachWatcher(this.path, (w) =>
						w.emit("change", this.path, mtime, type, false),
					);
				}
			});
			/** @type {Watcher<FileWatcherEvents>} */
			(this.parentWatcher).on("remove", () => {
				this.onDirectoryRemoved("parent directory removed");
			});
		}
	}

	/**
	 * @param {boolean} initial true when initial, otherwise false
	 */
	doScan(initial) {
		if (this.scanning) {
			if (this.scanAgain) {
				if (!initial) this.scanAgainInitial = false;
			} else {
				this.scanAgain = true;
				this.scanAgainInitial = initial;
			}
			return;
		}
		this.scanning = true;
		if (this.timeout) {
			clearTimeout(this.timeout);
			this.timeout = undefined;
		}
		process.nextTick(() => {
			if (this.closed) return;
			fs.readdir(this.path, (err, items) => {
				if (this.closed) return;
				if (err) {
					// Mirror the lstat error handling below: treat permission /
					// invalid-argument / no-device errors on the directory itself
					// as removed rather than logging "Watchpack Error (initial
					// scan)". These surface for unreadable mounts (WSL `/mnt/c`,
					// fuse mounts), unmounted devices (`/efi`), and libuv's
					// post-Node 22.17 `EINVAL` on protected Windows paths
					// (see #187).
					if (
						err.code === "ENOENT" ||
						err.code === "EPERM" ||
						err.code === "EACCES" ||
						err.code === "ENODEV" ||
						(err.code === "EINVAL" && IS_WIN)
					) {
						this.onDirectoryRemoved("scan readdir failed");
					} else {
						this.onScanError(err);
					}
					this.initialScan = false;
					this.initialScanFinished = Date.now();
					if (initial) {
						for (const watchers of this.watchers.values()) {
							for (const watcher of watchers) {
								if (watcher.checkStartTime(this.initialScanFinished, false)) {
									watcher.emit(
										"initial-missing",
										"scan (parent directory missing in initial scan)",
									);
								}
							}
						}
					}
					if (this.scanAgain) {
						this.scanAgain = false;
						this.doScan(this.scanAgainInitial);
					} else {
						this.scanning = false;
					}
					return;
				}
				const itemPaths = new Set(
					items.map((item) => path.join(this.path, item.normalize("NFC"))),
				);
				for (const file of this.files.keys()) {
					if (!itemPaths.has(file)) {
						this.setMissing(file, initial, "scan (missing)");
					}
				}
				for (const directory of this.directories.keys()) {
					if (!itemPaths.has(directory)) {
						this.setMissing(directory, initial, "scan (missing)");
					}
				}
				if (this.scanAgain) {
					// Early repeat of scan
					this.scanAgain = false;
					this.doScan(initial);
					return;
				}
				const itemFinished = needCalls(itemPaths.size + 1, () => {
					if (this.closed) return;
					this.initialScan = false;
					this.initialScanRemoved = null;
					this.initialScanFinished = Date.now();
					if (initial) {
						const missingWatchers = new Map(this.watchers);
						missingWatchers.delete(withoutCase(this.path));
						for (const item of itemPaths) {
							missingWatchers.delete(withoutCase(item));
						}
						for (const watchers of missingWatchers.values()) {
							for (const watcher of watchers) {
								if (watcher.checkStartTime(this.initialScanFinished, false)) {
									watcher.emit(
										"initial-missing",
										"scan (missing in initial scan)",
									);
								}
							}
						}
					}
					if (this.scanAgain) {
						this.scanAgain = false;
						this.doScan(this.scanAgainInitial);
					} else {
						this.scanning = false;
						this.onScanFinished();
					}
				});
				for (const itemPath of itemPaths) {
					lstatWithRetry(itemPath, this, (err2, stats) => {
						if (this.closed) return;
						if (err2) {
							if (
								err2.code === "ENOENT" ||
								err2.code === "EPERM" ||
								err2.code === "EACCES" ||
								err2.code === "EBUSY" ||
								err2.code === "ENODEV" ||
								// TODO https://github.com/libuv/libuv/pull/4566
								(err2.code === "EINVAL" && IS_WIN)
							) {
								// readdir saw the entry but we can't stat it due to a
								// transient lock — keep the previously-known entry instead
								// of incorrectly flagging it as missing.
								if (
									!(
										err2.code === "EBUSY" &&
										BUSY_RETRIES > 0 &&
										this.files.has(itemPath)
									)
								) {
									this.setMissing(itemPath, initial, `scan (${err2.code})`);
								}
							} else {
								this.onScanError(err2);
							}
							itemFinished();
							return;
						}
						/**
						 * @param {string | null} realPath resolved real path for an outside-dir symlink target
						 */
						const apply = (realPath) => {
							if (stats.isFile() || stats.isSymbolicLink()) {
								if (stats.mtime) ensureFsAccuracy(+stats.mtime);
								this.setFileTime(
									itemPath,
									+stats.mtime || +stats.ctime || 1,
									initial,
									true,
									"scan (file)",
								);
								if (realPath && !this._symlinkTargetWatchers.has(itemPath)) {
									const w = this.watcherManager.watchFile(realPath, Date.now());
									if (w) {
										w.on("change", (mtime, type, wInitial) => {
											if (wInitial) return;
											this.setFileTime(itemPath, mtime, false, false, type);
										});
										this._symlinkTargetWatchers.set(itemPath, w);
									}
								}
							} else if (
								stats.isDirectory() &&
								(!initial || !this.directories.has(itemPath))
							) {
								this.setDirectory(
									itemPath,
									+stats.birthtime || 1,
									initial,
									"scan (dir)",
								);
							}
							itemFinished();
						};
						if (
							this.options.followSymlinks &&
							this.nestedWatching &&
							stats.isSymbolicLink()
						) {
							fs.realpath(itemPath, (err3, realPath) => {
								if (this.closed) return;
								if (
									err3 ||
									!realPath ||
									withoutCase(path.dirname(realPath)) === withoutCase(this.path)
								) {
									apply(null);
									return;
								}
								// Cycle protection: when the symlink's target is the symlink
								// path itself or one of its ancestors, descending would
								// create an unbounded chain of `DirectoryWatcher`s as we walk
								// back into territory we are already watching. Treat the
								// symlink as a plain entry instead so the symlink itself is
								// still tracked but no recursion happens.
								const rel = path.relative(realPath, itemPath);
								if (!path.isAbsolute(rel) && !rel.startsWith("..")) {
									apply(null);
									return;
								}
								fs.stat(realPath, (err4, targetStats) => {
									if (this.closed) return;
									if (err4 || !targetStats) {
										apply(null);
									} else if (targetStats.isFile()) {
										apply(realPath);
									} else if (targetStats.isDirectory()) {
										// Treat a symlink whose target is a directory as a
										// nested watched directory so files inside the target
										// propagate change events to the symlink path.
										this.setDirectory(
											itemPath,
											+targetStats.birthtime || +stats.birthtime || 1,
											initial,
											"scan (dir)",
										);
										itemFinished();
									} else {
										apply(null);
									}
								});
							});
							return;
						}
						apply(null);
					});
				}
				itemFinished();
			});
		});
	}

	/**
	 * @returns {Record<string, number>} times
	 */
	getTimes() {
		const obj = Object.create(null);
		let safeTime = this.lastWatchEvent;
		for (const [file, entry] of this.files) {
			fixupEntryAccuracy(entry);
			safeTime = Math.max(safeTime, entry.safeTime);
			obj[file] = Math.max(entry.safeTime, entry.timestamp);
		}
		if (this.nestedWatching) {
			for (const w of this.directories.values()) {
				const times =
					/** @type {Watcher<DirectoryWatcherEvents>} */
					(w).directoryWatcher.getTimes();
				for (const file of Object.keys(times)) {
					const time = times[file];
					safeTime = Math.max(safeTime, time);
					obj[file] = time;
				}
			}
			obj[this.path] = safeTime;
		}
		if (!this.initialScan) {
			for (const watchers of this.watchers.values()) {
				for (const watcher of watchers) {
					const { path } = watcher;
					if (!Object.prototype.hasOwnProperty.call(obj, path)) {
						obj[path] = null;
					}
				}
			}
		}
		return obj;
	}

	/**
	 * @param {TimeInfoEntries} fileTimestamps file timestamps
	 * @param {TimeInfoEntries} directoryTimestamps directory timestamps
	 * @returns {number} safe time
	 */
	collectTimeInfoEntries(fileTimestamps, directoryTimestamps) {
		let safeTime = this.lastWatchEvent;
		for (const [file, entry] of this.files) {
			fixupEntryAccuracy(entry);
			safeTime = Math.max(safeTime, entry.safeTime);
			fileTimestamps.set(file, entry);
		}
		if (this.nestedWatching) {
			for (const w of this.directories.values()) {
				safeTime = Math.max(
					safeTime,
					/** @type {Watcher<DirectoryWatcherEvents>} */
					(w).directoryWatcher.collectTimeInfoEntries(
						fileTimestamps,
						directoryTimestamps,
					),
				);
			}
			fileTimestamps.set(this.path, EXISTANCE_ONLY_TIME_ENTRY);
			directoryTimestamps.set(this.path, {
				safeTime,
			});
		} else {
			for (const dir of this.directories.keys()) {
				// No additional info about this directory
				// but maybe another DirectoryWatcher has info
				fileTimestamps.set(dir, EXISTANCE_ONLY_TIME_ENTRY);
				if (!directoryTimestamps.has(dir)) {
					directoryTimestamps.set(dir, EXISTANCE_ONLY_TIME_ENTRY);
				}
			}
			fileTimestamps.set(this.path, EXISTANCE_ONLY_TIME_ENTRY);
			directoryTimestamps.set(this.path, EXISTANCE_ONLY_TIME_ENTRY);
		}
		if (!this.initialScan) {
			for (const watchers of this.watchers.values()) {
				for (const watcher of watchers) {
					const { path } = watcher;
					if (!fileTimestamps.has(path)) {
						fileTimestamps.set(path, null);
					}
				}
			}
		}
		return safeTime;
	}

	close() {
		if (this.closed) return;
		this.closed = true;
		this.initialScan = false;
		if (this.watcher) {
			this.watcher.close();
			this.watcher = null;
		}
		if (this.nestedWatching) {
			for (const w of this.directories.values()) {
				/** @type {Watcher<DirectoryWatcherEvents>} */
				(w).close();
			}
			this.directories.clear();
		}
		for (const w of this._symlinkTargetWatchers.values()) {
			w.close();
		}
		this._symlinkTargetWatchers.clear();
		if (this.parentWatcher) {
			this.parentWatcher.close();
			this.parentWatcher = null;
		}
		this.emit("closed");
	}
}

module.exports = DirectoryWatcher;
module.exports.EXISTANCE_ONLY_TIME_ENTRY = EXISTANCE_ONLY_TIME_ENTRY;
module.exports.Watcher = Watcher;
