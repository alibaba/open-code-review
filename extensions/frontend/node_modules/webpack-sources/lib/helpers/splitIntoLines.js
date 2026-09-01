/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/

"use strict";

/**
 * @param {string} str string
 * @returns {string[]} array of string separated by lines
 */
const splitIntoLines = (str) => {
	const results = [];
	const len = str.length;
	let i = 0;
	while (i < len) {
		// indexOf is implemented natively and is significantly faster than
		// scanning char-by-char with charCodeAt for long lines.
		const n = str.indexOf("\n", i);
		if (n === -1) {
			results.push(i === 0 ? str : str.slice(i));
			break;
		}
		if (n === i) {
			results.push("\n");
		} else {
			results.push(str.slice(i, n + 1));
		}
		i = n + 1;
	}
	return results;
};

module.exports = splitIntoLines;
