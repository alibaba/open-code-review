var concatMap = require('concat-map');
var balanced = require('balanced-match');

module.exports = expandTop;

var escSlash = '\0SLASH'+Math.random()+'\0';
var escOpen = '\0OPEN'+Math.random()+'\0';
var escClose = '\0CLOSE'+Math.random()+'\0';
var escComma = '\0COMMA'+Math.random()+'\0';
var escPeriod = '\0PERIOD'+Math.random()+'\0';

var EXPANSION_MAX = 100000

// `EXPANSION_MAX` caps the *number* of expansions, but not their length. An
// input like `'{a,b}'.repeat(1500)` stays under that count - its output is
// truncated to 100k results - while making every result ~1500 characters
// long. The result set, and the intermediate arrays built while combining
// brace sets, then grow large enough to exhaust memory and crash the process
// (CVE-2026-14257). `EXPANSION_MAX_LENGTH` bounds the total number of
// characters the accumulator may hold at any point, so memory stays flat no
// matter how many brace groups are chained. The limit sits well above any
// realistic expansion (100k results hitting `EXPANSION_MAX` measure ~1M
// characters) so legitimate input is unaffected.
var EXPANSION_MAX_LENGTH = 4000000

function numeric(str) {
  return parseInt(str, 10) == str
    ? parseInt(str, 10)
    : str.charCodeAt(0);
}

function escapeBraces(str) {
  return str.split('\\\\').join(escSlash)
            .split('\\{').join(escOpen)
            .split('\\}').join(escClose)
            .split('\\,').join(escComma)
            .split('\\.').join(escPeriod);
}

function unescapeBraces(str) {
  return str.split(escSlash).join('\\')
            .split(escOpen).join('{')
            .split(escClose).join('}')
            .split(escComma).join(',')
            .split(escPeriod).join('.');
}


// Basically just str.split(","), but handling cases
// where we have nested braced sections, which should be
// treated as individual members, like {a,{b,c},d}
function parseCommaParts(str) {
  if (!str)
    return [''];

  var parts = [];
  var m = balanced('{', '}', str);

  if (!m)
    return str.split(',');

  var pre = m.pre;
  var body = m.body;
  var post = m.post;
  var p = pre.split(',');

  p[p.length-1] += '{' + body + '}';
  var postParts = parseCommaParts(post);
  if (post.length) {
    p[p.length-1] += postParts.shift();
    p.push.apply(p, postParts);
  }

  parts.push.apply(parts, p);

  return parts;
}

function expandTop(str, options) {
  if (!str)
    return [];

  options = options || {};
  var max = options.max == null ? EXPANSION_MAX : options.max;
  var maxLength = options.maxLength == null ? EXPANSION_MAX_LENGTH : options.maxLength;

  // I don't know why Bash 4.3 does this, but it does.
  // Anything starting with {} will have the first two bytes preserved
  // but *only* at the top level, so {},a}b will not expand to anything,
  // but a{},b}c will be expanded to [a}c,abc].
  // One could argue that this is a bug in Bash, but since the goal of
  // this module is to match Bash's rules, we escape a leading {}
  if (str.substr(0, 2) === '{}') {
    str = '\\{\\}' + str.substr(2);
  }

  return expand(escapeBraces(str), max, maxLength, true).map(unescapeBraces);
}

function identity(e) {
  return e;
}

function embrace(str) {
  return '{' + str + '}';
}
function isPadded(el) {
  return /^-?0\d/.test(el);
}

function lte(i, y) {
  return i <= y;
}
function gte(i, y) {
  return i >= y;
}

// Build `{ acc[a] + pre + values[v] }` for every combination, capping the
// number of results at `max` and the total number of characters at `maxLength`.
// This is the one place output grows, so bounding it here keeps the single
// accumulator - and therefore memory - flat regardless of how many brace groups
// are combined (CVE-2026-14257).
//
// `base[a]` is the length of the part of `acc[a]` that predates the current
// empty-drop baseline (see `expand`). The matching baselines for the results
// are appended to `outBase`, which the caller carries forward alongside them.
function combine(
  acc,
  base,
  pre,
  values,
  max,
  maxLength,
  dropEmpties,
  outBase
) {
  var out = []
  var length = 0
  for (var a = 0; a < acc.length; a++) {
    for (var v = 0; v < values.length; v++) {
      if (out.length >= max) return out
      var expansion = acc[a] + pre + values[v]
      // Bash drops empty results at the top level. Skip them before they count
      // against `max`, so `max` bounds the number of *kept* results. "Empty"
      // means "adds nothing past the baseline", not "empty overall".
      if (dropEmpties && expansion.length === base[a]) continue
      if (length + expansion.length > maxLength) return out
      out.push(expansion)
      outBase.push(base[a])
      length += expansion.length
    }
  }
  return out
}

// The expansion values of a single numeric (`1..5`) or alphabetic (`a..e..2`)
// sequence body.
function expandSequence(
  body,
  isAlphaSequence,
  max,
  maxLength
) {
  var n = body.split(/\.\./)
  var N = []
  // A sequence body always splits into two or three parts, but the compiler
  // can't know that.
  /* c8 ignore start */
  if (n[0] === undefined || n[1] === undefined) {
    return N
  }
  /* c8 ignore stop */
  var x = numeric(n[0])
  var y = numeric(n[1])
  var width = Math.max(n[0].length, n[1].length)
  var incr =
    n.length === 3 && n[2] !== undefined ?
      Math.max(Math.abs(numeric(n[2])), 1)
    : 1
  var test = lte
  var reverse = y < x
  if (reverse) {
    incr *= -1
    test = gte
  }
  var pad = n.some(isPadded)

  var length = 0
  for (var i = x; test(i, y) && N.length < max; i += incr) {
    var c
    if (isAlphaSequence) {
      c = String.fromCharCode(i)
      if (c === '\\') {
        c = ''
      }
    } else {
      c = String(i)
      if (pad) {
        var need = width - c.length
        if (need > 0) {
          var z = new Array(need + 1).join('0')
          if (i < 0) {
            c = '-' + z + c.slice(1)
          } else {
            c = z + c
          }
        }
      }
    }
    if (length + c.length > maxLength) break
    N.push(c)
    length += c.length
  }
  return N
}

function expand(
  str,
  max,
  maxLength,
  isTop
) {
  // Consume the string's top-level brace groups left to right, threading a
  // running set of combined prefixes (`acc`). Expanding the tail iteratively -
  // rather than recursing on `m.post` once per group - keeps the native stack
  // depth constant, so deeply chained input (`'{a,b}'.repeat(3000)`) can no
  // longer overflow the stack, and leaves a single accumulator whose size
  // `maxLength` bounds directly (CVE-2026-14257).
  var acc = ['']

  // Bash drops empty results, but only when the *first* group of the run is a
  // comma set - a sequence like `{a..\}` may legitimately yield ''. The drop
  // is on the final strings, so it is applied to whichever `combine` produces
  // them (the one with no brace set left in the tail).
  //
  // The old implementation recursed on `m.post`, so the drop tested only the
  // expansion of the current call's substring. The `{a},b}` rewrite below turns
  // `isTop` back on part-way through a string, starting a fresh such run, so
  // the drop must ignore whatever `acc` already holds from earlier groups.
  // `accBase[a]` records how much of `acc[a]` predates the current run;
  // `combine` treats an expansion as empty when it adds nothing past that.
  var accBase = [0]
  var dropEmpties = false
  var firstGroup = true
  var nextBase

  for (;;) {
    var m = balanced('{', '}', str);

    // No brace set left: the rest of the string is literal.
    if (!m) {
      return combine(acc, accBase, str, [''], max, maxLength, dropEmpties, [])
    }

    // no need to expand pre, since it is guaranteed to be free of brace-sets
    var pre = m.pre;

    // For compatibility reasons, `${` is not eligible for brace expansion, and
    // on the 1.x line it suppresses expansion of the rest of the string too:
    // the whole remainder is literal. The 2.x and 5.x lines instead keep
    // expanding the tail, which is what bash does, but changing that here would
    // be a breaking change for 1.x consumers. Routed through `combine` so the
    // result is still bounded by `max` and `maxLength`.
    if (/\$$/.test(pre)) {
      return combine(acc, accBase, str, [''], max, maxLength, dropEmpties, [])
    }

    var isNumericSequence = /^-?\d+\.\.-?\d+(?:\.\.-?\d+)?$/.test(m.body);
    var isAlphaSequence = /^[a-zA-Z]\.\.[a-zA-Z](?:\.\.-?\d+)?$/.test(m.body);
    var isSequence = isNumericSequence || isAlphaSequence;
    var isOptions = m.body.indexOf(',') >= 0;
    if (!isSequence && !isOptions) {
      // {a},b}
      if (m.post.match(/,(?!,).*\}/)) {
        str = m.pre + '{' + m.body + escClose + m.post;
        // The rewritten string is expanded as if it were a fresh top-level one,
        // so start a new empty-drop run: anchor the baseline at what `acc`
        // holds now, and let the next expanding group decide whether to drop.
        isTop = true
        firstGroup = true
        dropEmpties = false
        accBase = []
        for (var b = 0; b < acc.length; b++) {
          accBase.push(acc[b].length)
        }
        continue
      }
      // Nothing here expands, so the whole remaining string is literal.
      return combine(
        acc,
        accBase,
        pre + '{' + m.body + '}' + m.post,
        [''],
        max,
        maxLength,
        dropEmpties,
        []
      )
    }

    if (firstGroup) {
      dropEmpties = isTop && !isSequence
      firstGroup = false
    }

    var values;
    if (isSequence) {
      values = expandSequence(m.body, isAlphaSequence, max, maxLength);
    } else {
      var n = parseCommaParts(m.body);
      if (n.length === 1 && n[0] !== undefined) {
        // x{{a,b}}y ==> x{a}y x{b}y
        n = expand(n[0], max, maxLength, false).map(embrace);
        //XXX is this necessary? Can't seem to hit it in tests.
        /* c8 ignore start */
        if (n.length === 1) {
          nextBase = []
          acc = combine(
            acc,
            accBase,
            pre + n[0],
            [''],
            max,
            maxLength,
            dropEmpties && !m.post.length,
            nextBase
          )
          accBase = nextBase
          if (!m.post.length) break
          str = m.post
          continue
        }
        /* c8 ignore stop */
      }

      // Values that `combine` is going to drop as empty produce no result, so
      // they must not count against `max` - otherwise `{a,,b}` with `max: 2`
      // would stop at `['a', '']` and yield one result instead of two. Skipping
      // them outright keeps `values` bounded while leaving `max` a bound on
      // *kept* results. A value is dropped when it adds nothing past the
      // baseline, which is what `combine` tests.
      var dropsEmpties = dropEmpties && !m.post.length && !pre
      for (var d = 0; dropsEmpties && d < acc.length; d++) {
        if (acc[d].length !== accBase[d]) {
          dropsEmpties = false
        }
      }

      values = []
      var valuesLength = 0
      outer: for (var j = 0; j < n.length; j++) {
        var expanded = expand(n[j], max, maxLength, false)
        for (var k = 0; k < expanded.length; k++) {
          var v = expanded[k]
          if (dropsEmpties && !v) continue
          if (values.length >= max || valuesLength + v.length > maxLength) {
            break outer
          }
          values.push(v)
          valuesLength += v.length
        }
      }
    }

    nextBase = []
    acc = combine(
      acc,
      accBase,
      pre,
      values,
      max,
      maxLength,
      dropEmpties && !m.post.length,
      nextBase
    )
    accBase = nextBase
    if (!m.post.length) break
    str = m.post
  }

  return acc
}
