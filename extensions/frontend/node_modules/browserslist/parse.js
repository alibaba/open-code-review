var SPACE = /\s/

function flatten(array) {
  if (!Array.isArray(array)) return [array]
  // Iterative flatten: `reduce`+`concat` copies the accumulator on every step,
  // which is O(n²) once a query resolves to many nodes.
  var result = []
  var stack = [array]
  while (stack.length) {
    var item = stack.pop()
    if (Array.isArray(item)) {
      for (var i = item.length - 1; i >= 0; i--) {
        stack.push(item[i])
      }
    } else {
      result.push(item)
    }
  }
  return result
}

function matchQuery(all, query) {
  var node = { query: query }
  if (query.indexOf('not ') === 0) {
    node.not = true
    query = query.slice(4)
  }

  for (var name in all) {
    var type = all[name]
    var match = query.match(type.regexp)
    if (match) {
      node.type = name
      for (var i = 0; i < type.matches.length; i++) {
        node[type.matches[i]] = match[i + 1]
      }
      return node
    }
  }

  node.type = 'unknown'
  return node
}

function pushClause(all, qs, text, compose) {
  var node = matchQuery(all, text.trim())
  node.compose = compose
  qs.push(node)
}

// Splits a query block into clauses on the `\s+and\s+`, `\s+or\s+` and `,\s*`
// delimiters in a single left-to-right pass. The previous implementation grew
// a suffix one character at a time and re-tested anchored `\s+…` regexps at
// every length, which backtracks across whitespace runs and is O(n²) — a
// small padded query could freeze the event loop for tens of seconds.
function parseBlock(all, block, qs) {
  if (block.length === 0) return

  var len = block.length
  var clauseStart = 0
  // Compose for the clause currently being read; the leftmost clause is `or`.
  var compose = 'or'
  var i = 0

  while (i < len) {
    var ch = block[i]

    if (ch === ',') {
      // `,\s*` delimiter. A delimiter at the very start (i === 0) has no left
      // clause — the original never emits one there.
      if (i !== 0) pushClause(all, qs, block.slice(clauseStart, i), compose)
      i++
      while (i < len && SPACE.test(block[i])) i++
      compose = 'or'
      clauseStart = i
      continue
    }

    if (SPACE.test(ch)) {
      // Possible `\s+and\s+` or `\s+or\s+`. Scan the whitespace run once
      // (linear, no backtracking), then check the following keyword.
      var q = i
      while (q < len && SPACE.test(block[q])) q++

      if (
        q + 3 < len &&
        (block[q] === 'a' || block[q] === 'A') &&
        (block[q + 1] === 'n' || block[q + 1] === 'N') &&
        (block[q + 2] === 'd' || block[q + 2] === 'D') &&
        SPACE.test(block[q + 3])
      ) {
        // The leading `\s+` of `\s+and\s+` absorbs whitespace at the block
        // start, so a delimiter at i === 0 has no left clause.
        if (i !== 0) pushClause(all, qs, block.slice(clauseStart, i), compose)
        var afterAnd = q + 3
        while (afterAnd < len && SPACE.test(block[afterAnd])) afterAnd++
        compose = 'and'
        i = afterAnd
        clauseStart = afterAnd
        continue
      } else if (
        q + 2 < len &&
        (block[q] === 'o' || block[q] === 'O') &&
        (block[q + 1] === 'r' || block[q + 1] === 'R') &&
        SPACE.test(block[q + 2])
      ) {
        if (i !== 0) pushClause(all, qs, block.slice(clauseStart, i), compose)
        var afterOr = q + 2
        while (afterOr < len && SPACE.test(block[afterOr])) afterOr++
        compose = 'or'
        i = afterOr
        clauseStart = afterOr
        continue
      } else {
        // Whitespace inside a clause; skip the run and keep reading.
        i = q
        continue
      }
    }

    i++
  }

  pushClause(all, qs, block.slice(clauseStart), compose)
}

module.exports = function parse(all, queries) {
  if (!Array.isArray(queries)) queries = [queries]
  return flatten(
    queries.map(function (block) {
      var qs = []
      parseBlock(all, block, qs)
      return qs
    })
  )
}
