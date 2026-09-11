/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Ivan Kopeykin @vankop
*/

"use strict";

const { isInside, normalize } = require("./util/path");

/** @typedef {import("./Resolver")} Resolver */
/** @typedef {import("./Resolver").ResolveStepHook} ResolveStepHook */

/**
 * @typedef {object} PathRestriction
 * @property {"path"} type type of the restriction
 * @property {string} rule normalized path the request has to be inside of
 */

/**
 * @typedef {object} RegExpRestriction
 * @property {"regexp"} type type of the restriction
 * @property {RegExp} rule pattern the request has to match
 */

/** @typedef {PathRestriction | RegExpRestriction} Restriction */

module.exports = class RestrictionsPlugin {
	/**
	 * @param {string | ResolveStepHook} source source
	 * @param {Set<string | RegExp>} restrictions restrictions
	 */
	constructor(source, restrictions) {
		this.source = source;
		this.restrictions = restrictions;
		// Restrictions never change, so bringing them into the shape requests
		// arrive in is done once here instead of on every request.
		/** @type {Restriction[]} */
		this._restrictions = [];
		for (const rule of restrictions) {
			this._restrictions.push(
				typeof rule === "string"
					? { type: "path", rule: normalize(rule) }
					: { type: "regexp", rule },
			);
		}
	}

	/**
	 * @param {Resolver} resolver the resolver
	 * @returns {void}
	 */
	apply(resolver) {
		resolver
			.getHook(this.source)
			.tapAsync("RestrictionsPlugin", (request, resolveContext, callback) => {
				if (typeof request.path === "string") {
					const { path } = request;
					for (const restriction of this._restrictions) {
						if (restriction.type === "path") {
							if (isInside(restriction.rule, path)) continue;
							if (resolveContext.log) {
								resolveContext.log(
									`${path} is not inside of the restriction ${restriction.rule}`,
								);
							}
						} else {
							if (restriction.rule.test(path)) continue;
							if (resolveContext.log) {
								resolveContext.log(
									`${path} doesn't match the restriction ${restriction.rule}`,
								);
							}
						}
						// Target existed (FileExistsPlugin already passed) but is
						// outside the jail; signal ExportsFieldPlugin to fall back.
						if (request.__restrictionsMarker) {
							request.__restrictionsMarker.blocked = true;
						}
						return callback(null, null);
					}
				}

				callback();
			});
	}
};
