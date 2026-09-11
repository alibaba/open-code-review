/*!
 * /**
 *  * Copyright (c) Meta Platforms, Inc. and affiliates.
 *  *
 *  * This source code is licensed under the MIT license found in the
 *  * LICENSE file in the root directory of this source tree.
 *  * /
 */
/******/ (() => { // webpackBootstrap
/******/ 	"use strict";
/******/ 	var __webpack_modules__ = ({

/***/ "./src/cleanupSemantic.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.cleanupSemantic = exports.Diff = exports.DIFF_INSERT = exports.DIFF_EQUAL = exports.DIFF_DELETE = void 0;
/**
 * Diff Match and Patch
 * Copyright 2018 The diff-match-patch Authors.
 * https://github.com/google/diff-match-patch
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * @fileoverview Computes the difference between two texts to create a patch.
 * Applies the patch onto another text, allowing for errors.
 * @author fraser@google.com (Neil Fraser)
 */

/**
 * CHANGES by pedrottimark to diff_match_patch_uncompressed.ts file:
 *
 * 1. Delete anything not needed to use diff_cleanupSemantic method
 * 2. Convert from prototype properties to var declarations
 * 3. Convert Diff to class from constructor and prototype
 * 4. Add type annotations for arguments and return values
 * 5. Add exports
 */

/**
 * The data structure representing a diff is an array of tuples:
 * [[DIFF_DELETE, 'Hello'], [DIFF_INSERT, 'Goodbye'], [DIFF_EQUAL, ' world.']]
 * which means: delete 'Hello', add 'Goodbye' and keep ' world.'
 */
var DIFF_DELETE = exports.DIFF_DELETE = -1;
var DIFF_INSERT = exports.DIFF_INSERT = 1;
var DIFF_EQUAL = exports.DIFF_EQUAL = 0;

/**
 * Class representing one diff tuple.
 * Attempts to look like a two-element array (which is what this used to be).
 * @param {number} op Operation, one of: DIFF_DELETE, DIFF_INSERT, DIFF_EQUAL.
 * @param {string} text Text to be deleted, inserted, or retained.
 * @constructor
 */
class Diff {
  0;
  1;
  constructor(op, text) {
    this[0] = op;
    this[1] = text;
  }
}

/**
 * Determine the common prefix of two strings.
 * @param {string} text1 First string.
 * @param {string} text2 Second string.
 * @return {number} The number of characters common to the start of each
 *     string.
 */
exports.Diff = Diff;
var diff_commonPrefix = function (text1, text2) {
  // Quick check for common null cases.
  if (!text1 || !text2 || text1.charAt(0) != text2.charAt(0)) {
    return 0;
  }
  // Binary search.
  // Performance analysis: https://neil.fraser.name/news/2007/10/09/
  var pointermin = 0;
  var pointermax = Math.min(text1.length, text2.length);
  var pointermid = pointermax;
  var pointerstart = 0;
  while (pointermin < pointermid) {
    if (text1.substring(pointerstart, pointermid) == text2.substring(pointerstart, pointermid)) {
      pointermin = pointermid;
      pointerstart = pointermin;
    } else {
      pointermax = pointermid;
    }
    pointermid = Math.floor((pointermax - pointermin) / 2 + pointermin);
  }
  return pointermid;
};

/**
 * Determine the common suffix of two strings.
 * @param {string} text1 First string.
 * @param {string} text2 Second string.
 * @return {number} The number of characters common to the end of each string.
 */
var diff_commonSuffix = function (text1, text2) {
  // Quick check for common null cases.
  if (!text1 || !text2 || text1.charAt(text1.length - 1) != text2.charAt(text2.length - 1)) {
    return 0;
  }
  // Binary search.
  // Performance analysis: https://neil.fraser.name/news/2007/10/09/
  var pointermin = 0;
  var pointermax = Math.min(text1.length, text2.length);
  var pointermid = pointermax;
  var pointerend = 0;
  while (pointermin < pointermid) {
    if (text1.substring(text1.length - pointermid, text1.length - pointerend) == text2.substring(text2.length - pointermid, text2.length - pointerend)) {
      pointermin = pointermid;
      pointerend = pointermin;
    } else {
      pointermax = pointermid;
    }
    pointermid = Math.floor((pointermax - pointermin) / 2 + pointermin);
  }
  return pointermid;
};

/**
 * Determine if the suffix of one string is the prefix of another.
 * @param {string} text1 First string.
 * @param {string} text2 Second string.
 * @return {number} The number of characters common to the end of the first
 *     string and the start of the second string.
 * @private
 */
var diff_commonOverlap_ = function (text1, text2) {
  // Cache the text lengths to prevent multiple calls.
  var text1_length = text1.length;
  var text2_length = text2.length;
  // Eliminate the null case.
  if (text1_length == 0 || text2_length == 0) {
    return 0;
  }
  // Truncate the longer string.
  if (text1_length > text2_length) {
    text1 = text1.substring(text1_length - text2_length);
  } else if (text1_length < text2_length) {
    text2 = text2.substring(0, text1_length);
  }
  var text_length = Math.min(text1_length, text2_length);
  // Quick check for the worst case.
  if (text1 == text2) {
    return text_length;
  }

  // Start by looking for a single character match
  // and increase length until no match is found.
  // Performance analysis: https://neil.fraser.name/news/2010/11/04/
  var best = 0;
  var length = 1;
  while (true) {
    var pattern = text1.substring(text_length - length);
    var found = text2.indexOf(pattern);
    if (found == -1) {
      return best;
    }
    length += found;
    if (found == 0 || text1.substring(text_length - length) == text2.substring(0, length)) {
      best = length;
      length++;
    }
  }
};

/**
 * Reduce the number of edits by eliminating semantically trivial equalities.
 * @param {!Array.<!diff_match_patch.Diff>} diffs Array of diff tuples.
 */
var diff_cleanupSemantic = function (diffs) {
  var changes = false;
  var equalities = []; // Stack of indices where equalities are found.
  var equalitiesLength = 0; // Keeping our own length var is faster in JS.
  /** @type {?string} */
  var lastEquality = null;
  // Always equal to diffs[equalities[equalitiesLength - 1]][1]
  var pointer = 0; // Index of current position.
  // Number of characters that changed prior to the equality.
  var length_insertions1 = 0;
  var length_deletions1 = 0;
  // Number of characters that changed after the equality.
  var length_insertions2 = 0;
  var length_deletions2 = 0;
  while (pointer < diffs.length) {
    if (diffs[pointer][0] == DIFF_EQUAL) {
      // Equality found.
      equalities[equalitiesLength++] = pointer;
      length_insertions1 = length_insertions2;
      length_deletions1 = length_deletions2;
      length_insertions2 = 0;
      length_deletions2 = 0;
      lastEquality = diffs[pointer][1];
    } else {
      // An insertion or deletion.
      if (diffs[pointer][0] == DIFF_INSERT) {
        length_insertions2 += diffs[pointer][1].length;
      } else {
        length_deletions2 += diffs[pointer][1].length;
      }
      // Eliminate an equality that is smaller or equal to the edits on both
      // sides of it.
      if (lastEquality && lastEquality.length <= Math.max(length_insertions1, length_deletions1) && lastEquality.length <= Math.max(length_insertions2, length_deletions2)) {
        // Duplicate record.
        diffs.splice(equalities[equalitiesLength - 1], 0, new Diff(DIFF_DELETE, lastEquality));
        // Change second copy to insert.
        diffs[equalities[equalitiesLength - 1] + 1][0] = DIFF_INSERT;
        // Throw away the equality we just deleted.
        equalitiesLength--;
        // Throw away the previous equality (it needs to be reevaluated).
        equalitiesLength--;
        pointer = equalitiesLength > 0 ? equalities[equalitiesLength - 1] : -1;
        length_insertions1 = 0; // Reset the counters.
        length_deletions1 = 0;
        length_insertions2 = 0;
        length_deletions2 = 0;
        lastEquality = null;
        changes = true;
      }
    }
    pointer++;
  }

  // Normalize the diff.
  if (changes) {
    diff_cleanupMerge(diffs);
  }
  diff_cleanupSemanticLossless(diffs);

  // Find any overlaps between deletions and insertions.
  // e.g: <del>abcxxx</del><ins>xxxdef</ins>
  //   -> <del>abc</del>xxx<ins>def</ins>
  // e.g: <del>xxxabc</del><ins>defxxx</ins>
  //   -> <ins>def</ins>xxx<del>abc</del>
  // Only extract an overlap if it is as big as the edit ahead or behind it.
  pointer = 1;
  while (pointer < diffs.length) {
    if (diffs[pointer - 1][0] == DIFF_DELETE && diffs[pointer][0] == DIFF_INSERT) {
      var deletion = diffs[pointer - 1][1];
      var insertion = diffs[pointer][1];
      var overlap_length1 = diff_commonOverlap_(deletion, insertion);
      var overlap_length2 = diff_commonOverlap_(insertion, deletion);
      if (overlap_length1 >= overlap_length2) {
        if (overlap_length1 >= deletion.length / 2 || overlap_length1 >= insertion.length / 2) {
          // Overlap found.  Insert an equality and trim the surrounding edits.
          diffs.splice(pointer, 0, new Diff(DIFF_EQUAL, insertion.substring(0, overlap_length1)));
          diffs[pointer - 1][1] = deletion.substring(0, deletion.length - overlap_length1);
          diffs[pointer + 1][1] = insertion.substring(overlap_length1);
          pointer++;
        }
      } else {
        if (overlap_length2 >= deletion.length / 2 || overlap_length2 >= insertion.length / 2) {
          // Reverse overlap found.
          // Insert an equality and swap and trim the surrounding edits.
          diffs.splice(pointer, 0, new Diff(DIFF_EQUAL, deletion.substring(0, overlap_length2)));
          diffs[pointer - 1][0] = DIFF_INSERT;
          diffs[pointer - 1][1] = insertion.substring(0, insertion.length - overlap_length2);
          diffs[pointer + 1][0] = DIFF_DELETE;
          diffs[pointer + 1][1] = deletion.substring(overlap_length2);
          pointer++;
        }
      }
      pointer++;
    }
    pointer++;
  }
};

/**
 * Look for single edits surrounded on both sides by equalities
 * which can be shifted sideways to align the edit to a word boundary.
 * e.g: The c<ins>at c</ins>ame. -> The <ins>cat </ins>came.
 * @param {!Array.<!diff_match_patch.Diff>} diffs Array of diff tuples.
 */
exports.cleanupSemantic = diff_cleanupSemantic;
var diff_cleanupSemanticLossless = function (diffs) {
  /**
   * Given two strings, compute a score representing whether the internal
   * boundary falls on logical boundaries.
   * Scores range from 6 (best) to 0 (worst).
   * Closure, but does not reference any external variables.
   * @param {string} one First string.
   * @param {string} two Second string.
   * @return {number} The score.
   * @private
   */
  function diff_cleanupSemanticScore_(one, two) {
    if (!one || !two) {
      // Edges are the best.
      return 6;
    }

    // Each port of this function behaves slightly differently due to
    // subtle differences in each language's definition of things like
    // 'whitespace'.  Since this function's purpose is largely cosmetic,
    // the choice has been made to use each language's native features
    // rather than force total conformity.
    var char1 = one.charAt(one.length - 1);
    var char2 = two.charAt(0);
    var nonAlphaNumeric1 = char1.match(nonAlphaNumericRegex_);
    var nonAlphaNumeric2 = char2.match(nonAlphaNumericRegex_);
    var whitespace1 = nonAlphaNumeric1 && char1.match(whitespaceRegex_);
    var whitespace2 = nonAlphaNumeric2 && char2.match(whitespaceRegex_);
    var lineBreak1 = whitespace1 && char1.match(linebreakRegex_);
    var lineBreak2 = whitespace2 && char2.match(linebreakRegex_);
    var blankLine1 = lineBreak1 && one.match(blanklineEndRegex_);
    var blankLine2 = lineBreak2 && two.match(blanklineStartRegex_);
    if (blankLine1 || blankLine2) {
      // Five points for blank lines.
      return 5;
    } else if (lineBreak1 || lineBreak2) {
      // Four points for line breaks.
      return 4;
    } else if (nonAlphaNumeric1 && !whitespace1 && whitespace2) {
      // Three points for end of sentences.
      return 3;
    } else if (whitespace1 || whitespace2) {
      // Two points for whitespace.
      return 2;
    } else if (nonAlphaNumeric1 || nonAlphaNumeric2) {
      // One point for non-alphanumeric.
      return 1;
    }
    return 0;
  }
  var pointer = 1;
  // Intentionally ignore the first and last element (don't need checking).
  while (pointer < diffs.length - 1) {
    if (diffs[pointer - 1][0] == DIFF_EQUAL && diffs[pointer + 1][0] == DIFF_EQUAL) {
      // This is a single edit surrounded by equalities.
      var equality1 = diffs[pointer - 1][1];
      var edit = diffs[pointer][1];
      var equality2 = diffs[pointer + 1][1];

      // First, shift the edit as far left as possible.
      var commonOffset = diff_commonSuffix(equality1, edit);
      if (commonOffset) {
        var commonString = edit.substring(edit.length - commonOffset);
        equality1 = equality1.substring(0, equality1.length - commonOffset);
        edit = commonString + edit.substring(0, edit.length - commonOffset);
        equality2 = commonString + equality2;
      }

      // Second, step character by character right, looking for the best fit.
      var bestEquality1 = equality1;
      var bestEdit = edit;
      var bestEquality2 = equality2;
      var bestScore = diff_cleanupSemanticScore_(equality1, edit) + diff_cleanupSemanticScore_(edit, equality2);
      while (edit.charAt(0) === equality2.charAt(0)) {
        equality1 += edit.charAt(0);
        edit = edit.substring(1) + equality2.charAt(0);
        equality2 = equality2.substring(1);
        var score = diff_cleanupSemanticScore_(equality1, edit) + diff_cleanupSemanticScore_(edit, equality2);
        // The >= encourages trailing rather than leading whitespace on edits.
        if (score >= bestScore) {
          bestScore = score;
          bestEquality1 = equality1;
          bestEdit = edit;
          bestEquality2 = equality2;
        }
      }
      if (diffs[pointer - 1][1] != bestEquality1) {
        // We have an improvement, save it back to the diff.
        if (bestEquality1) {
          diffs[pointer - 1][1] = bestEquality1;
        } else {
          diffs.splice(pointer - 1, 1);
          pointer--;
        }
        diffs[pointer][1] = bestEdit;
        if (bestEquality2) {
          diffs[pointer + 1][1] = bestEquality2;
        } else {
          diffs.splice(pointer + 1, 1);
          pointer--;
        }
      }
    }
    pointer++;
  }
};

// Define some regex patterns for matching boundaries.
var nonAlphaNumericRegex_ = /[^a-zA-Z0-9]/;
var whitespaceRegex_ = /\s/;
var linebreakRegex_ = /[\r\n]/;
var blanklineEndRegex_ = /\n\r?\n$/;
var blanklineStartRegex_ = /^\r?\n\r?\n/;

/**
 * Reorder and merge like edit sections.  Merge equalities.
 * Any edit section can move as long as it doesn't cross an equality.
 * @param {!Array.<!diff_match_patch.Diff>} diffs Array of diff tuples.
 */
var diff_cleanupMerge = function (diffs) {
  // Add a dummy entry at the end.
  diffs.push(new Diff(DIFF_EQUAL, ''));
  var pointer = 0;
  var count_delete = 0;
  var count_insert = 0;
  var text_delete = '';
  var text_insert = '';
  var commonlength;
  while (pointer < diffs.length) {
    switch (diffs[pointer][0]) {
      case DIFF_INSERT:
        count_insert++;
        text_insert += diffs[pointer][1];
        pointer++;
        break;
      case DIFF_DELETE:
        count_delete++;
        text_delete += diffs[pointer][1];
        pointer++;
        break;
      case DIFF_EQUAL:
        // Upon reaching an equality, check for prior redundancies.
        if (count_delete + count_insert > 1) {
          if (count_delete !== 0 && count_insert !== 0) {
            // Factor out any common prefixies.
            commonlength = diff_commonPrefix(text_insert, text_delete);
            if (commonlength !== 0) {
              if (pointer - count_delete - count_insert > 0 && diffs[pointer - count_delete - count_insert - 1][0] == DIFF_EQUAL) {
                diffs[pointer - count_delete - count_insert - 1][1] += text_insert.substring(0, commonlength);
              } else {
                diffs.splice(0, 0, new Diff(DIFF_EQUAL, text_insert.substring(0, commonlength)));
                pointer++;
              }
              text_insert = text_insert.substring(commonlength);
              text_delete = text_delete.substring(commonlength);
            }
            // Factor out any common suffixies.
            commonlength = diff_commonSuffix(text_insert, text_delete);
            if (commonlength !== 0) {
              diffs[pointer][1] = text_insert.substring(text_insert.length - commonlength) + diffs[pointer][1];
              text_insert = text_insert.substring(0, text_insert.length - commonlength);
              text_delete = text_delete.substring(0, text_delete.length - commonlength);
            }
          }
          // Delete the offending records and add the merged ones.
          pointer -= count_delete + count_insert;
          diffs.splice(pointer, count_delete + count_insert);
          if (text_delete.length) {
            diffs.splice(pointer, 0, new Diff(DIFF_DELETE, text_delete));
            pointer++;
          }
          if (text_insert.length) {
            diffs.splice(pointer, 0, new Diff(DIFF_INSERT, text_insert));
            pointer++;
          }
          pointer++;
        } else if (pointer !== 0 && diffs[pointer - 1][0] == DIFF_EQUAL) {
          // Merge this equality with the previous one.
          diffs[pointer - 1][1] += diffs[pointer][1];
          diffs.splice(pointer, 1);
        } else {
          pointer++;
        }
        count_insert = 0;
        count_delete = 0;
        text_delete = '';
        text_insert = '';
        break;
    }
  }
  if (diffs[diffs.length - 1][1] === '') {
    diffs.pop(); // Remove the dummy entry at the end.
  }

  // Second pass: look for single edits surrounded on both sides by equalities
  // which can be shifted sideways to eliminate an equality.
  // e.g: A<ins>BA</ins>C -> <ins>AB</ins>AC
  var changes = false;
  pointer = 1;
  // Intentionally ignore the first and last element (don't need checking).
  while (pointer < diffs.length - 1) {
    if (diffs[pointer - 1][0] == DIFF_EQUAL && diffs[pointer + 1][0] == DIFF_EQUAL) {
      // This is a single edit surrounded by equalities.
      if (diffs[pointer][1].substring(diffs[pointer][1].length - diffs[pointer - 1][1].length) == diffs[pointer - 1][1]) {
        // Shift the edit over the previous equality.
        diffs[pointer][1] = diffs[pointer - 1][1] + diffs[pointer][1].substring(0, diffs[pointer][1].length - diffs[pointer - 1][1].length);
        diffs[pointer + 1][1] = diffs[pointer - 1][1] + diffs[pointer + 1][1];
        diffs.splice(pointer - 1, 1);
        changes = true;
      } else if (diffs[pointer][1].substring(0, diffs[pointer + 1][1].length) == diffs[pointer + 1][1]) {
        // Shift the edit over the next equality.
        diffs[pointer - 1][1] += diffs[pointer + 1][1];
        diffs[pointer][1] = diffs[pointer][1].substring(diffs[pointer + 1][1].length) + diffs[pointer + 1][1];
        diffs.splice(pointer + 1, 1);
        changes = true;
      }
    }
    pointer++;
  }
  // If shifts were made, the diff needs reordering and another shift sweep.
  if (changes) {
    diff_cleanupMerge(diffs);
  }
};

/***/ },

/***/ "./src/constants.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.SIMILAR_MESSAGE = exports.NO_DIFF_MESSAGE = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const NO_DIFF_MESSAGE = exports.NO_DIFF_MESSAGE = 'Compared values have no visual difference.';
const SIMILAR_MESSAGE = exports.SIMILAR_MESSAGE = 'Compared values serialize to the same structure.\n' + 'Printing internal object structure without calling `toJSON` instead.';

/***/ },

/***/ "./src/diffLines.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.printDiffLines = exports.diffLinesUnified2 = exports.diffLinesUnified = exports.diffLinesRaw = void 0;
var _diffSequences = _interopRequireDefault(require("@jest/diff-sequences"));
var _cleanupSemantic = __webpack_require__("./src/cleanupSemantic.ts");
var _escapeControlCharacters = __webpack_require__("./src/escapeControlCharacters.ts");
var _joinAlignedDiffs = __webpack_require__("./src/joinAlignedDiffs.ts");
var _normalizeDiffOptions = __webpack_require__("./src/normalizeDiffOptions.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const isEmptyString = lines => lines.length === 1 && lines[0].length === 0;
const countChanges = diffs => {
  let a = 0;
  let b = 0;
  for (const diff of diffs) {
    switch (diff[0]) {
      case _cleanupSemantic.DIFF_DELETE:
        a += 1;
        break;
      case _cleanupSemantic.DIFF_INSERT:
        b += 1;
        break;
    }
  }
  return {
    a,
    b
  };
};
const printAnnotation = ({
  aAnnotation,
  aColor,
  aIndicator,
  bAnnotation,
  bColor,
  bIndicator,
  includeChangeCounts,
  omitAnnotationLines
}, changeCounts) => {
  if (omitAnnotationLines) {
    return '';
  }
  let aRest = '';
  let bRest = '';
  if (includeChangeCounts) {
    const aCount = String(changeCounts.a);
    const bCount = String(changeCounts.b);

    // Padding right aligns the ends of the annotations.
    const baAnnotationLengthDiff = bAnnotation.length - aAnnotation.length;
    const aAnnotationPadding = ' '.repeat(Math.max(0, baAnnotationLengthDiff));
    const bAnnotationPadding = ' '.repeat(Math.max(0, -baAnnotationLengthDiff));

    // Padding left aligns the ends of the counts.
    const baCountLengthDiff = bCount.length - aCount.length;
    const aCountPadding = ' '.repeat(Math.max(0, baCountLengthDiff));
    const bCountPadding = ' '.repeat(Math.max(0, -baCountLengthDiff));
    aRest = `${aAnnotationPadding}  ${aIndicator} ${aCountPadding}${aCount}`;
    bRest = `${bAnnotationPadding}  ${bIndicator} ${bCountPadding}${bCount}`;
  }
  const a = `${aIndicator} ${aAnnotation}${aRest}`;
  const b = `${bIndicator} ${bAnnotation}${bRest}`;
  return `${aColor(a)}\n${bColor(b)}\n\n`;
};
const printDiffLines = (diffs, options) => printAnnotation(options, countChanges(diffs)) + (options.expand ? (0, _joinAlignedDiffs.joinAlignedDiffsExpand)(diffs, options) : (0, _joinAlignedDiffs.joinAlignedDiffsNoExpand)(diffs, options));

// Compare two arrays of strings line-by-line. Format as comparison lines.
exports.printDiffLines = printDiffLines;
const diffLinesUnified = (aLines, bLines, options) => printDiffLines(diffLinesRaw(isEmptyString(aLines) ? [] : aLines.map(_escapeControlCharacters.escapeControlCharacters), isEmptyString(bLines) ? [] : bLines.map(_escapeControlCharacters.escapeControlCharacters)), (0, _normalizeDiffOptions.normalizeDiffOptions)(options));

// Given two pairs of arrays of strings:
// Compare the pair of comparison arrays line-by-line.
// Format the corresponding lines in the pair of displayable arrays.
exports.diffLinesUnified = diffLinesUnified;
const diffLinesUnified2 = (aLinesDisplay, bLinesDisplay, aLinesCompare, bLinesCompare, options) => {
  if (isEmptyString(aLinesDisplay) && isEmptyString(aLinesCompare)) {
    aLinesDisplay = [];
    aLinesCompare = [];
  }
  if (isEmptyString(bLinesDisplay) && isEmptyString(bLinesCompare)) {
    bLinesDisplay = [];
    bLinesCompare = [];
  }
  if (aLinesDisplay.length !== aLinesCompare.length || bLinesDisplay.length !== bLinesCompare.length) {
    // Fall back to diff of display lines.
    return diffLinesUnified(aLinesDisplay, bLinesDisplay, options);
  }
  const diffs = diffLinesRaw(aLinesCompare, bLinesCompare);

  // Replace comparison lines with displayable lines.
  let aIndex = 0;
  let bIndex = 0;
  for (const diff of diffs) {
    switch (diff[0]) {
      case _cleanupSemantic.DIFF_DELETE:
        diff[1] = aLinesDisplay[aIndex];
        aIndex += 1;
        break;
      case _cleanupSemantic.DIFF_INSERT:
        diff[1] = bLinesDisplay[bIndex];
        bIndex += 1;
        break;
      default:
        diff[1] = bLinesDisplay[bIndex];
        aIndex += 1;
        bIndex += 1;
    }
  }
  return printDiffLines(diffs, (0, _normalizeDiffOptions.normalizeDiffOptions)(options));
};

// Compare two arrays of strings line-by-line.
exports.diffLinesUnified2 = diffLinesUnified2;
const diffLinesRaw = (aLines, bLines) => {
  const aLength = aLines.length;
  const bLength = bLines.length;
  const isCommon = (aIndex, bIndex) => aLines[aIndex] === bLines[bIndex];
  const diffs = [];
  let aIndex = 0;
  let bIndex = 0;
  const foundSubsequence = (nCommon, aCommon, bCommon) => {
    for (; aIndex !== aCommon; aIndex += 1) {
      diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_DELETE, aLines[aIndex]));
    }
    for (; bIndex !== bCommon; bIndex += 1) {
      diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_INSERT, bLines[bIndex]));
    }
    for (; nCommon !== 0; nCommon -= 1, aIndex += 1, bIndex += 1) {
      diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_EQUAL, bLines[bIndex]));
    }
  };
  (0, _diffSequences.default)(aLength, bLength, isCommon, foundSubsequence);

  // After the last common subsequence, push remaining change items.
  for (; aIndex !== aLength; aIndex += 1) {
    diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_DELETE, aLines[aIndex]));
  }
  for (; bIndex !== bLength; bIndex += 1) {
    diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_INSERT, bLines[bIndex]));
  }
  return diffs;
};
exports.diffLinesRaw = diffLinesRaw;

/***/ },

/***/ "./src/diffStrings.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _diffSequences = _interopRequireDefault(require("@jest/diff-sequences"));
var _cleanupSemantic = __webpack_require__("./src/cleanupSemantic.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const diffStrings = (a, b) => {
  const isCommon = (aIndex, bIndex) => a[aIndex] === b[bIndex];
  let aIndex = 0;
  let bIndex = 0;
  const diffs = [];
  const foundSubsequence = (nCommon, aCommon, bCommon) => {
    if (aIndex !== aCommon) {
      diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_DELETE, a.slice(aIndex, aCommon)));
    }
    if (bIndex !== bCommon) {
      diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_INSERT, b.slice(bIndex, bCommon)));
    }
    aIndex = aCommon + nCommon; // number of characters compared in a
    bIndex = bCommon + nCommon; // number of characters compared in b
    diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_EQUAL, b.slice(bCommon, bIndex)));
  };
  (0, _diffSequences.default)(a.length, b.length, isCommon, foundSubsequence);

  // After the last common subsequence, push remaining change items.
  if (aIndex !== a.length) {
    diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_DELETE, a.slice(aIndex)));
  }
  if (bIndex !== b.length) {
    diffs.push(new _cleanupSemantic.Diff(_cleanupSemantic.DIFF_INSERT, b.slice(bIndex)));
  }
  return diffs;
};
var _default = exports["default"] = diffStrings;

/***/ },

/***/ "./src/escapeControlCharacters.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.escapeControlCharacters = void 0;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Escape control characters to make them visible in diffs
const escapeControlCharacters = str => str.replaceAll(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F-\u009F]/g, match => {
  switch (match) {
    case '\b':
      return '\\b';
    case '\f':
      return '\\f';
    case '\v':
      return '\\v';
    default:
      {
        const code = match.codePointAt(0);
        return `\\x${code.toString(16).padStart(2, '0')}`;
      }
  }
});
exports.escapeControlCharacters = escapeControlCharacters;

/***/ },

/***/ "./src/getAlignedDiffs.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports["default"] = void 0;
var _cleanupSemantic = __webpack_require__("./src/cleanupSemantic.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Given change op and array of diffs, return concatenated string:
// * include common strings
// * include change strings which have argument op with changeColor
// * exclude change strings which have opposite op
const concatenateRelevantDiffs = (op, diffs, changeColor) => diffs.reduce((reduced, diff) => reduced + (diff[0] === _cleanupSemantic.DIFF_EQUAL ? diff[1] : diff[0] === op && diff[1].length > 0 // empty if change is newline
? changeColor(diff[1]) : ''), '');

// Encapsulate change lines until either a common newline or the end.
class ChangeBuffer {
  op;
  line; // incomplete line
  lines; // complete lines
  changeColor;
  constructor(op, changeColor) {
    this.op = op;
    this.line = [];
    this.lines = [];
    this.changeColor = changeColor;
  }
  pushSubstring(substring) {
    this.pushDiff(new _cleanupSemantic.Diff(this.op, substring));
  }
  pushLine() {
    // Assume call only if line has at least one diff,
    // therefore an empty line must have a diff which has an empty string.

    // If line has multiple diffs, then assume it has a common diff,
    // therefore change diffs have change color;
    // otherwise then it has line color only.
    this.lines.push(this.line.length === 1 ? this.line[0][0] === this.op ? this.line[0] // can use instance
    : new _cleanupSemantic.Diff(this.op, this.line[0][1]) : new _cleanupSemantic.Diff(this.op, concatenateRelevantDiffs(this.op, this.line, this.changeColor)) // was common diff
    );
    this.line.length = 0;
  }
  isLineEmpty() {
    return this.line.length === 0;
  }

  // Minor input to buffer.
  pushDiff(diff) {
    this.line.push(diff);
  }

  // Main input to buffer.
  align(diff) {
    const string = diff[1];
    if (string.includes('\n')) {
      const substrings = string.split('\n');
      const iLast = substrings.length - 1;
      for (const [i, substring] of substrings.entries()) {
        if (i < iLast) {
          // The first substring completes the current change line.
          // A middle substring is a change line.
          this.pushSubstring(substring);
          this.pushLine();
        } else if (substring.length > 0) {
          // The last substring starts a change line, if it is not empty.
          // Important: This non-empty condition also automatically omits
          // the newline appended to the end of expected and received strings.
          this.pushSubstring(substring);
        }
      }
    } else {
      // Append non-multiline string to current change line.
      this.pushDiff(diff);
    }
  }

  // Output from buffer.
  moveLinesTo(lines) {
    if (!this.isLineEmpty()) {
      this.pushLine();
    }
    lines.push(...this.lines);
    this.lines.length = 0;
  }
}

// Encapsulate common and change lines.
class CommonBuffer {
  deleteBuffer;
  insertBuffer;
  lines;
  constructor(deleteBuffer, insertBuffer) {
    this.deleteBuffer = deleteBuffer;
    this.insertBuffer = insertBuffer;
    this.lines = [];
  }
  pushDiffCommonLine(diff) {
    this.lines.push(diff);
  }
  pushDiffChangeLines(diff) {
    const isDiffEmpty = diff[1].length === 0;

    // An empty diff string is redundant, unless a change line is empty.
    if (!isDiffEmpty || this.deleteBuffer.isLineEmpty()) {
      this.deleteBuffer.pushDiff(diff);
    }
    if (!isDiffEmpty || this.insertBuffer.isLineEmpty()) {
      this.insertBuffer.pushDiff(diff);
    }
  }
  flushChangeLines() {
    this.deleteBuffer.moveLinesTo(this.lines);
    this.insertBuffer.moveLinesTo(this.lines);
  }

  // Input to buffer.
  align(diff) {
    const op = diff[0];
    const string = diff[1];
    if (string.includes('\n')) {
      const substrings = string.split('\n');
      const iLast = substrings.length - 1;
      for (const [i, substring] of substrings.entries()) {
        if (i === 0) {
          const subdiff = new _cleanupSemantic.Diff(op, substring);
          if (this.deleteBuffer.isLineEmpty() && this.insertBuffer.isLineEmpty()) {
            // If both current change lines are empty,
            // then the first substring is a common line.
            this.flushChangeLines();
            this.pushDiffCommonLine(subdiff);
          } else {
            // If either current change line is non-empty,
            // then the first substring completes the change lines.
            this.pushDiffChangeLines(subdiff);
            this.flushChangeLines();
          }
        } else if (i < iLast) {
          // A middle substring is a common line.
          this.pushDiffCommonLine(new _cleanupSemantic.Diff(op, substring));
        } else if (substring.length > 0) {
          // The last substring starts a change line, if it is not empty.
          // Important: This non-empty condition also automatically omits
          // the newline appended to the end of expected and received strings.
          this.pushDiffChangeLines(new _cleanupSemantic.Diff(op, substring));
        }
      }
    } else {
      // Append non-multiline string to current change lines.
      // Important: It cannot be at the end following empty change lines,
      // because newline appended to the end of expected and received strings.
      this.pushDiffChangeLines(diff);
    }
  }

  // Output from buffer.
  getLines() {
    this.flushChangeLines();
    return this.lines;
  }
}

// Given diffs from expected and received strings,
// return new array of diffs split or joined into lines.
//
// To correctly align a change line at the end, the algorithm:
// * assumes that a newline was appended to the strings
// * omits the last newline from the output array
//
// Assume the function is not called:
// * if either expected or received is empty string
// * if neither expected nor received is multiline string
const getAlignedDiffs = (diffs, changeColor) => {
  const deleteBuffer = new ChangeBuffer(_cleanupSemantic.DIFF_DELETE, changeColor);
  const insertBuffer = new ChangeBuffer(_cleanupSemantic.DIFF_INSERT, changeColor);
  const commonBuffer = new CommonBuffer(deleteBuffer, insertBuffer);
  for (const diff of diffs) {
    switch (diff[0]) {
      case _cleanupSemantic.DIFF_DELETE:
        deleteBuffer.align(diff);
        break;
      case _cleanupSemantic.DIFF_INSERT:
        insertBuffer.align(diff);
        break;
      default:
        commonBuffer.align(diff);
    }
  }
  return commonBuffer.getLines();
};
var _default = exports["default"] = getAlignedDiffs;

/***/ },

/***/ "./src/joinAlignedDiffs.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.joinAlignedDiffsNoExpand = exports.joinAlignedDiffsExpand = void 0;
var _cleanupSemantic = __webpack_require__("./src/cleanupSemantic.ts");
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const formatTrailingSpaces = (line, trailingSpaceFormatter) => line.replace(/\s+$/, match => trailingSpaceFormatter(match));
const printDiffLine = (line, isFirstOrLast, color, indicator, trailingSpaceFormatter, emptyFirstOrLastLinePlaceholder) => line.length === 0 ? indicator === ' ' ? isFirstOrLast && emptyFirstOrLastLinePlaceholder.length > 0 ? color(`${indicator} ${emptyFirstOrLastLinePlaceholder}`) : '' : color(indicator) : color(`${indicator} ${formatTrailingSpaces(line, trailingSpaceFormatter)}`);
const printDeleteLine = (line, isFirstOrLast, {
  aColor,
  aIndicator,
  changeLineTrailingSpaceColor,
  emptyFirstOrLastLinePlaceholder
}) => printDiffLine(line, isFirstOrLast, aColor, aIndicator, changeLineTrailingSpaceColor, emptyFirstOrLastLinePlaceholder);
const printInsertLine = (line, isFirstOrLast, {
  bColor,
  bIndicator,
  changeLineTrailingSpaceColor,
  emptyFirstOrLastLinePlaceholder
}) => printDiffLine(line, isFirstOrLast, bColor, bIndicator, changeLineTrailingSpaceColor, emptyFirstOrLastLinePlaceholder);
const printCommonLine = (line, isFirstOrLast, {
  commonColor,
  commonIndicator,
  commonLineTrailingSpaceColor,
  emptyFirstOrLastLinePlaceholder
}) => printDiffLine(line, isFirstOrLast, commonColor, commonIndicator, commonLineTrailingSpaceColor, emptyFirstOrLastLinePlaceholder);

// In GNU diff format, indexes are one-based instead of zero-based.
const createPatchMark = (aStart, aEnd, bStart, bEnd, {
  patchColor
}) => patchColor(`@@ -${aStart + 1},${aEnd - aStart} +${bStart + 1},${bEnd - bStart} @@`);

// jest --no-expand
//
// Given array of aligned strings with inverse highlight formatting,
// return joined lines with diff formatting (and patch marks, if needed).
const joinAlignedDiffsNoExpand = (diffs, options) => {
  const iLength = diffs.length;
  const nContextLines = options.contextLines;
  const nContextLines2 = nContextLines + nContextLines;

  // First pass: count output lines and see if it has patches.
  let jLength = iLength;
  let hasExcessAtStartOrEnd = false;
  let nExcessesBetweenChanges = 0;
  let i = 0;
  while (i !== iLength) {
    const iStart = i;
    while (i !== iLength && diffs[i][0] === _cleanupSemantic.DIFF_EQUAL) {
      i += 1;
    }
    if (iStart !== i) {
      if (iStart === 0) {
        // at start
        if (i > nContextLines) {
          jLength -= i - nContextLines; // subtract excess common lines
          hasExcessAtStartOrEnd = true;
        }
      } else if (i === iLength) {
        // at end
        const n = i - iStart;
        if (n > nContextLines) {
          jLength -= n - nContextLines; // subtract excess common lines
          hasExcessAtStartOrEnd = true;
        }
      } else {
        // between changes
        const n = i - iStart;
        if (n > nContextLines2) {
          jLength -= n - nContextLines2; // subtract excess common lines
          nExcessesBetweenChanges += 1;
        }
      }
    }
    while (i !== iLength && diffs[i][0] !== _cleanupSemantic.DIFF_EQUAL) {
      i += 1;
    }
  }
  const hasPatch = nExcessesBetweenChanges !== 0 || hasExcessAtStartOrEnd;
  if (nExcessesBetweenChanges !== 0) {
    jLength += nExcessesBetweenChanges + 1; // add patch lines
  } else if (hasExcessAtStartOrEnd) {
    jLength += 1; // add patch line
  }
  const jLast = jLength - 1;
  const lines = [];
  let jPatchMark = 0; // index of placeholder line for current patch mark
  if (hasPatch) {
    lines.push(''); // placeholder line for first patch mark
  }

  // Indexes of expected or received lines in current patch:
  let aStart = 0;
  let bStart = 0;
  let aEnd = 0;
  let bEnd = 0;
  const pushCommonLine = line => {
    const j = lines.length;
    lines.push(printCommonLine(line, j === 0 || j === jLast, options));
    aEnd += 1;
    bEnd += 1;
  };
  const pushDeleteLine = line => {
    const j = lines.length;
    lines.push(printDeleteLine(line, j === 0 || j === jLast, options));
    aEnd += 1;
  };
  const pushInsertLine = line => {
    const j = lines.length;
    lines.push(printInsertLine(line, j === 0 || j === jLast, options));
    bEnd += 1;
  };

  // Second pass: push lines with diff formatting (and patch marks, if needed).
  i = 0;
  while (i !== iLength) {
    let iStart = i;
    while (i !== iLength && diffs[i][0] === _cleanupSemantic.DIFF_EQUAL) {
      i += 1;
    }
    if (iStart !== i) {
      if (iStart === 0) {
        // at beginning
        if (i > nContextLines) {
          iStart = i - nContextLines;
          aStart = iStart;
          bStart = iStart;
          aEnd = aStart;
          bEnd = bStart;
        }
        for (let iCommon = iStart; iCommon !== i; iCommon += 1) {
          pushCommonLine(diffs[iCommon][1]);
        }
      } else if (i === iLength) {
        // at end
        const iEnd = i - iStart > nContextLines ? iStart + nContextLines : i;
        for (let iCommon = iStart; iCommon !== iEnd; iCommon += 1) {
          pushCommonLine(diffs[iCommon][1]);
        }
      } else {
        // between changes
        const nCommon = i - iStart;
        if (nCommon > nContextLines2) {
          const iEnd = iStart + nContextLines;
          for (let iCommon = iStart; iCommon !== iEnd; iCommon += 1) {
            pushCommonLine(diffs[iCommon][1]);
          }
          lines[jPatchMark] = createPatchMark(aStart, aEnd, bStart, bEnd, options);
          jPatchMark = lines.length;
          lines.push(''); // placeholder line for next patch mark

          const nOmit = nCommon - nContextLines2;
          aStart = aEnd + nOmit;
          bStart = bEnd + nOmit;
          aEnd = aStart;
          bEnd = bStart;
          for (let iCommon = i - nContextLines; iCommon !== i; iCommon += 1) {
            pushCommonLine(diffs[iCommon][1]);
          }
        } else {
          for (let iCommon = iStart; iCommon !== i; iCommon += 1) {
            pushCommonLine(diffs[iCommon][1]);
          }
        }
      }
    }
    while (i !== iLength && diffs[i][0] === _cleanupSemantic.DIFF_DELETE) {
      pushDeleteLine(diffs[i][1]);
      i += 1;
    }
    while (i !== iLength && diffs[i][0] === _cleanupSemantic.DIFF_INSERT) {
      pushInsertLine(diffs[i][1]);
      i += 1;
    }
  }
  if (hasPatch) {
    lines[jPatchMark] = createPatchMark(aStart, aEnd, bStart, bEnd, options);
  }
  return lines.join('\n');
};

// jest --expand
//
// Given array of aligned strings with inverse highlight formatting,
// return joined lines with diff formatting.
exports.joinAlignedDiffsNoExpand = joinAlignedDiffsNoExpand;
const joinAlignedDiffsExpand = (diffs, options) => diffs.map((diff, i, diffs) => {
  const line = diff[1];
  const isFirstOrLast = i === 0 || i === diffs.length - 1;
  switch (diff[0]) {
    case _cleanupSemantic.DIFF_DELETE:
      return printDeleteLine(line, isFirstOrLast, options);
    case _cleanupSemantic.DIFF_INSERT:
      return printInsertLine(line, isFirstOrLast, options);
    default:
      return printCommonLine(line, isFirstOrLast, options);
  }
}).join('\n');
exports.joinAlignedDiffsExpand = joinAlignedDiffsExpand;

/***/ },

/***/ "./src/normalizeDiffOptions.ts"
(__unused_webpack_module, exports) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.normalizeDiffOptions = exports.noColor = void 0;
var _chalk = _interopRequireDefault(require("chalk"));
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const noColor = string => string;
exports.noColor = noColor;
const DIFF_CONTEXT_DEFAULT = 5;
const OPTIONS_DEFAULT = {
  aAnnotation: 'Expected',
  aColor: _chalk.default.green,
  aIndicator: '-',
  bAnnotation: 'Received',
  bColor: _chalk.default.red,
  bIndicator: '+',
  changeColor: _chalk.default.inverse,
  changeLineTrailingSpaceColor: noColor,
  commonColor: _chalk.default.dim,
  commonIndicator: ' ',
  commonLineTrailingSpaceColor: noColor,
  compareKeys: undefined,
  contextLines: DIFF_CONTEXT_DEFAULT,
  emptyFirstOrLastLinePlaceholder: '',
  expand: true,
  includeChangeCounts: false,
  omitAnnotationLines: false,
  patchColor: _chalk.default.yellow
};
const getCompareKeys = compareKeys => compareKeys && typeof compareKeys === 'function' ? compareKeys : OPTIONS_DEFAULT.compareKeys;
const getContextLines = contextLines => typeof contextLines === 'number' && Number.isSafeInteger(contextLines) && contextLines >= 0 ? contextLines : DIFF_CONTEXT_DEFAULT;

// Pure function returns options with all properties.
const normalizeDiffOptions = (options = {}) => ({
  ...OPTIONS_DEFAULT,
  ...options,
  compareKeys: getCompareKeys(options.compareKeys),
  contextLines: getContextLines(options.contextLines)
});
exports.normalizeDiffOptions = normalizeDiffOptions;

/***/ },

/***/ "./src/printDiffs.ts"
(__unused_webpack_module, exports, __webpack_require__) {



Object.defineProperty(exports, "__esModule", ({
  value: true
}));
exports.diffStringsUnified = exports.diffStringsRaw = void 0;
var _cleanupSemantic = __webpack_require__("./src/cleanupSemantic.ts");
var _diffLines = __webpack_require__("./src/diffLines.ts");
var _diffStrings = _interopRequireDefault(__webpack_require__("./src/diffStrings.ts"));
var _getAlignedDiffs = _interopRequireDefault(__webpack_require__("./src/getAlignedDiffs.ts"));
var _normalizeDiffOptions = __webpack_require__("./src/normalizeDiffOptions.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

const hasCommonDiff = (diffs, isMultiline) => {
  if (isMultiline) {
    // Important: Ignore common newline that was appended to multiline strings!
    const iLast = diffs.length - 1;
    return diffs.some((diff, i) => diff[0] === _cleanupSemantic.DIFF_EQUAL && (i !== iLast || diff[1] !== '\n'));
  }
  return diffs.some(diff => diff[0] === _cleanupSemantic.DIFF_EQUAL);
};

// Compare two strings character-by-character.
// Format as comparison lines in which changed substrings have inverse colors.
const diffStringsUnified = (a, b, options) => {
  if (a !== b && a.length > 0 && b.length > 0) {
    const isMultiline = a.includes('\n') || b.includes('\n');

    // getAlignedDiffs assumes that a newline was appended to the strings.
    const diffs = diffStringsRaw(isMultiline ? `${a}\n` : a, isMultiline ? `${b}\n` : b, true // cleanupSemantic
    );
    if (hasCommonDiff(diffs, isMultiline)) {
      const optionsNormalized = (0, _normalizeDiffOptions.normalizeDiffOptions)(options);
      const lines = (0, _getAlignedDiffs.default)(diffs, optionsNormalized.changeColor);
      return (0, _diffLines.printDiffLines)(lines, optionsNormalized);
    }
  }

  // Fall back to line-by-line diff.
  return (0, _diffLines.diffLinesUnified)(a.split('\n'), b.split('\n'), options);
};

// Compare two strings character-by-character.
// Optionally clean up small common substrings, also known as chaff.
exports.diffStringsUnified = diffStringsUnified;
const diffStringsRaw = (a, b, cleanup) => {
  const diffs = (0, _diffStrings.default)(a, b);
  if (cleanup) {
    (0, _cleanupSemantic.cleanupSemantic)(diffs); // impure function
  }
  return diffs;
};
exports.diffStringsRaw = diffStringsRaw;

/***/ }

/******/ 	});
/************************************************************************/
/******/ 	// The module cache
/******/ 	var __webpack_module_cache__ = {};
/******/ 	
/******/ 	// The require function
/******/ 	function __webpack_require__(moduleId) {
/******/ 		// Check if module is in cache
/******/ 		var cachedModule = __webpack_module_cache__[moduleId];
/******/ 		if (cachedModule !== undefined) {
/******/ 			return cachedModule.exports;
/******/ 		}
/******/ 		// Create a new module (and put it into the cache)
/******/ 		var module = __webpack_module_cache__[moduleId] = {
/******/ 			// no module.id needed
/******/ 			// no module.loaded needed
/******/ 			exports: {}
/******/ 		};
/******/ 	
/******/ 		// Execute the module function
/******/ 		__webpack_modules__[moduleId](module, module.exports, __webpack_require__);
/******/ 	
/******/ 		// Return the exports of the module
/******/ 		return module.exports;
/******/ 	}
/******/ 	
/************************************************************************/
var __webpack_exports__ = {};
// This entry needs to be wrapped in an IIFE because it uses a non-standard name for the exports (exports).
(() => {
var exports = __webpack_exports__;


Object.defineProperty(exports, "__esModule", ({
  value: true
}));
Object.defineProperty(exports, "DIFF_DELETE", ({
  enumerable: true,
  get: function () {
    return _cleanupSemantic.DIFF_DELETE;
  }
}));
Object.defineProperty(exports, "DIFF_EQUAL", ({
  enumerable: true,
  get: function () {
    return _cleanupSemantic.DIFF_EQUAL;
  }
}));
Object.defineProperty(exports, "DIFF_INSERT", ({
  enumerable: true,
  get: function () {
    return _cleanupSemantic.DIFF_INSERT;
  }
}));
Object.defineProperty(exports, "Diff", ({
  enumerable: true,
  get: function () {
    return _cleanupSemantic.Diff;
  }
}));
exports.diff = diff;
Object.defineProperty(exports, "diffLinesRaw", ({
  enumerable: true,
  get: function () {
    return _diffLines.diffLinesRaw;
  }
}));
Object.defineProperty(exports, "diffLinesUnified", ({
  enumerable: true,
  get: function () {
    return _diffLines.diffLinesUnified;
  }
}));
Object.defineProperty(exports, "diffLinesUnified2", ({
  enumerable: true,
  get: function () {
    return _diffLines.diffLinesUnified2;
  }
}));
Object.defineProperty(exports, "diffStringsRaw", ({
  enumerable: true,
  get: function () {
    return _printDiffs.diffStringsRaw;
  }
}));
Object.defineProperty(exports, "diffStringsUnified", ({
  enumerable: true,
  get: function () {
    return _printDiffs.diffStringsUnified;
  }
}));
var _chalk = _interopRequireDefault(require("chalk"));
var _getType = require("@jest/get-type");
var _prettyFormat = require("pretty-format");
var _cleanupSemantic = __webpack_require__("./src/cleanupSemantic.ts");
var _constants = __webpack_require__("./src/constants.ts");
var _diffLines = __webpack_require__("./src/diffLines.ts");
var _escapeControlCharacters = __webpack_require__("./src/escapeControlCharacters.ts");
var _normalizeDiffOptions = __webpack_require__("./src/normalizeDiffOptions.ts");
var _printDiffs = __webpack_require__("./src/printDiffs.ts");
function _interopRequireDefault(e) { return e && e.__esModule ? e : { default: e }; }
var src_Symbol = globalThis['jest-symbol-do-not-touch'] || globalThis.Symbol;
/**
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */
const getCommonMessage = (message, options) => {
  const {
    commonColor
  } = (0, _normalizeDiffOptions.normalizeDiffOptions)(options);
  return commonColor(message);
};
const {
  AsymmetricMatcher,
  DOMCollection,
  DOMElement,
  Immutable,
  ReactElement,
  ReactTestComponent
} = _prettyFormat.plugins;
const PLUGINS = [ReactTestComponent, ReactElement, DOMElement, DOMCollection, Immutable, AsymmetricMatcher];
const FORMAT_OPTIONS = {
  plugins: PLUGINS
};
const FALLBACK_FORMAT_OPTIONS = {
  callToJSON: false,
  maxDepth: 10,
  plugins: PLUGINS
};

// Generate a string that will highlight the difference between two values
// with green and red. (similar to how github does code diffing)
// eslint-disable-next-line @typescript-eslint/explicit-module-boundary-types
function diff(a, b, options) {
  if (Object.is(a, b)) {
    return getCommonMessage(_constants.NO_DIFF_MESSAGE, options);
  }
  const aType = (0, _getType.getType)(a);
  let expectedType = aType;
  let omitDifference = false;
  if (aType === 'object' && typeof a.asymmetricMatch === 'function') {
    if (a.$$typeof !== src_Symbol.for('jest.asymmetricMatcher')) {
      // Do not know expected type of user-defined asymmetric matcher.
      return null;
    }
    if (typeof a.getExpectedType !== 'function') {
      // For example, expect.anything() matches either null or undefined
      return null;
    }
    expectedType = a.getExpectedType();
    // Primitive types boolean and number omit difference below.
    // For example, omit difference for expect.stringMatching(regexp)
    omitDifference = expectedType === 'string';
  }
  if (expectedType !== (0, _getType.getType)(b)) {
    return '  Comparing two different types of values.' + ` Expected ${_chalk.default.green(expectedType)} but ` + `received ${_chalk.default.red((0, _getType.getType)(b))}.`;
  }
  if (omitDifference) {
    return null;
  }
  switch (aType) {
    case 'string':
      return (0, _diffLines.diffLinesUnified)((0, _escapeControlCharacters.escapeControlCharacters)(a).split('\n'), (0, _escapeControlCharacters.escapeControlCharacters)(b).split('\n'), options);
    case 'boolean':
    case 'number':
      return comparePrimitive(a, b, options);
    case 'map':
      return compareObjects(sortMap(a), sortMap(b), options);
    case 'set':
      return compareObjects(sortSet(a), sortSet(b), options);
    default:
      return compareObjects(a, b, options);
  }
}
function comparePrimitive(a, b, options) {
  const aFormat = (0, _prettyFormat.format)(a, FORMAT_OPTIONS);
  const bFormat = (0, _prettyFormat.format)(b, FORMAT_OPTIONS);
  return aFormat === bFormat ? getCommonMessage(_constants.NO_DIFF_MESSAGE, options) : (0, _diffLines.diffLinesUnified)(aFormat.split('\n'), bFormat.split('\n'), options);
}
function sortMap(map) {
  return new Map([...map].sort());
}
function sortSet(set) {
  return new Set([...set].sort());
}
function compareObjects(a, b, options) {
  let difference;
  let hasThrown = false;
  try {
    const formatOptions = getFormatOptions(FORMAT_OPTIONS, options);
    difference = getObjectsDifference(a, b, formatOptions, options);
  } catch {
    hasThrown = true;
  }
  const noDiffMessage = getCommonMessage(_constants.NO_DIFF_MESSAGE, options);
  // If the comparison yields no results, compare again but this time
  // without calling `toJSON`. It's also possible that toJSON might throw.
  if (difference === undefined || difference === noDiffMessage) {
    const formatOptions = getFormatOptions(FALLBACK_FORMAT_OPTIONS, options);
    difference = getObjectsDifference(a, b, formatOptions, options);
    if (difference !== noDiffMessage && !hasThrown) {
      difference = `${getCommonMessage(_constants.SIMILAR_MESSAGE, options)}\n\n${difference}`;
    }
  }
  return difference;
}
function getFormatOptions(formatOptions, options) {
  const {
    compareKeys
  } = (0, _normalizeDiffOptions.normalizeDiffOptions)(options);
  return {
    ...formatOptions,
    compareKeys
  };
}
function getObjectsDifference(a, b, formatOptions, options) {
  const formatOptionsZeroIndent = {
    ...formatOptions,
    indent: 0
  };
  const aCompare = (0, _prettyFormat.format)(a, formatOptionsZeroIndent);
  const bCompare = (0, _prettyFormat.format)(b, formatOptionsZeroIndent);
  if (aCompare === bCompare) {
    return getCommonMessage(_constants.NO_DIFF_MESSAGE, options);
  } else {
    const aDisplay = (0, _prettyFormat.format)(a, formatOptions);
    const bDisplay = (0, _prettyFormat.format)(b, formatOptions);
    return (0, _diffLines.diffLinesUnified2)(aDisplay.split('\n'), bDisplay.split('\n'), aCompare.split('\n'), bCompare.split('\n'), options);
  }
}
})();

module.exports = __webpack_exports__;
/******/ })()
;