/*
	MIT License http://www.opensource.org/licenses/mit-license.php
	Author Tobias Koppers @sokra
*/
"use strict";

const path = require("path");

/**
 * @template T
 * @typedef {object} TreeNode
 * @property {string} target target
 * @property {TreeNode<T>} parent parent
 * @property {TreeNode<T>[]} children children
 * @property {number} entries number of entries
 * @property {boolean} active true when active, otherwise false
 * @property {T[] | T | undefined} value value
 */

/**
 * @template T
 * @param {Map<string, T[] | T>} plan plan
 * @param {number} limit limit
 * @returns {Map<string, Map<T, string>>} the new plan
 */
module.exports = (plan, limit) => {
	const treeMap = new Map();
	// Convert to tree
	for (const [target, value] of plan) {
		treeMap.set(target, {
			target,
			parent: undefined,
			children: undefined,
			entries: 1,
			active: true,
			value,
		});
	}
	let currentCount = treeMap.size;
	// Create parents and calculate sum of entries
	for (const node of treeMap.values()) {
		const parentPath = path.dirname(node.target);
		if (parentPath !== node.target) {
			let parent = treeMap.get(parentPath);
			if (parent === undefined) {
				parent = {
					target: parentPath,
					parent: undefined,
					children: [node],
					entries: node.entries,
					active: false,
					value: undefined,
				};
				treeMap.set(parentPath, parent);
				node.parent = parent;
			} else {
				node.parent = parent;
				if (parent.children === undefined) {
					parent.children = [node];
				} else {
					parent.children.push(node);
				}
				do {
					parent.entries += node.entries;
					parent = parent.parent;
				} while (parent);
			}
		}
	}
	// Reduce until limit reached. When no reduction is needed at all, skip
	// building the candidate set entirely to avoid paying for the setup on the
	// common fast path.
	if (currentCount > limit) {
		// Pre-filter candidate nodes so the inner selection loop skips structural
		// non-candidates entirely. `children` length and parent presence are
		// fixed after tree construction; only `entries` can change (it can only
		// decrease), so a node that fails the `entries` check in a later round
		// is simply skipped via `continue`. When we merge a subtree we drop the
		// descendants from the candidate set to keep it shrinking over
		// iterations.
		/** @type {Set<TreeNode<T>>} */
		const candidates = new Set();
		for (const node of treeMap.values()) {
			if (!node.parent || !node.children) continue;
			if (node.children.length === 0) continue;
			if (node.children.length === 1 && !node.value) continue;
			candidates.add(node);
		}
		const costBias = limit * 0.3;
		while (currentCount > limit) {
			// Select node that helps reaching the limit most effectively without overmerging
			const overLimit = currentCount - limit;
			let bestNode;
			let bestCost = Infinity;
			for (const node of candidates) {
				if (node.entries <= 1) continue;
				// Try to select the node with has just a bit more entries than we need to reduce
				// When just a bit more is over 30% over the limit,
				// also consider just a bit less entries then we need to reduce
				const diff = node.entries - 1 - overLimit;
				const cost = diff >= 0 ? diff : -diff + costBias;
				if (cost < bestCost) {
					bestNode = node;
					bestCost = cost;
					// A cost of 0 means the merge reduces exactly to the limit;
					// no further candidate can improve on that, so stop scanning.
					if (cost === 0) break;
				}
			}
			if (!bestNode) break;
			// Merge all children
			const reduction = bestNode.entries - 1;
			bestNode.active = true;
			bestNode.entries = 1;
			candidates.delete(bestNode);
			currentCount -= reduction;
			let { parent } = bestNode;
			while (parent) {
				parent.entries -= reduction;
				parent = parent.parent;
			}
			const queue = new Set(bestNode.children);
			for (const node of queue) {
				node.active = false;
				node.entries = 0;
				candidates.delete(node);
				if (node.children) {
					for (const child of node.children) queue.add(child);
				}
			}
		}
	}
	// Write down new plan
	const newPlan = new Map();
	for (const rootNode of treeMap.values()) {
		if (!rootNode.active) continue;
		const map = new Map();
		const queue = new Set([rootNode]);
		for (const node of queue) {
			if (node.active && node !== rootNode) continue;
			if (node.value) {
				if (Array.isArray(node.value)) {
					for (const item of node.value) {
						map.set(item, node.target);
					}
				} else {
					map.set(node.value, node.target);
				}
			}
			if (node.children) {
				for (const child of node.children) {
					queue.add(child);
				}
			}
		}
		newPlan.set(rootNode.target, map);
	}
	return newPlan;
};
