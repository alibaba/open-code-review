'use strict'

/** @type {(value: string) => boolean} */
const isUUID = RegExp.prototype.test.bind(/^[\da-f]{8}-[\da-f]{4}-[\da-f]{4}-[\da-f]{4}-[\da-f]{12}$/iu)

/** @type {(value: string) => boolean} */
const isIPv4 = RegExp.prototype.test.bind(/^(?:(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]\d|\d)\.){3}(?:25[0-5]|2[0-4]\d|1\d{2}|[1-9]\d|\d)$/u)

/** @type {(value: string) => boolean} */
const isHexPair = RegExp.prototype.test.bind(/^[\da-f]{2}$/iu)

/** @type {(value: string) => boolean} */
const isUnreserved = RegExp.prototype.test.bind(/^[\da-z\-._~]$/iu)

/** @type {(value: string) => boolean} */
const isPathCharacter = RegExp.prototype.test.bind(/^[A-Za-z0-9\-._~!$&'()*+,;=:@/]$/u)

/** @type {(value: string) => boolean} */
const isQueryFragmentCharacter = RegExp.prototype.test.bind(/^[A-Za-z0-9\-._~!$&'()*+,;=:@/?]$/u)

/** @type {(value: string) => boolean} */
const isUserinfoCharacter = RegExp.prototype.test.bind(/^[A-Za-z0-9\-._~!$&'()*+,;=:]$/u)

const BYTE_HEX = new Array(256)
{
  const HEX_DIGITS = '0123456789ABCDEF'
  for (let i = 0; i < 256; i++) {
    BYTE_HEX[i] = '%' + HEX_DIGITS[i >> 4] + HEX_DIGITS[i & 0xF]
  }
}
function percentEncodeNonAscii (cp) {
  if (cp < 0x800) {
    return BYTE_HEX[0xC0 | (cp >> 6)] +
           BYTE_HEX[0x80 | (cp & 0x3F)]
  }
  if (cp < 0x10000) {
    return BYTE_HEX[0xE0 | (cp >> 12)] +
           BYTE_HEX[0x80 | ((cp >> 6) & 0x3F)] +
           BYTE_HEX[0x80 | (cp & 0x3F)]
  }
  return BYTE_HEX[0xF0 | (cp >> 18)] +
         BYTE_HEX[0x80 | ((cp >> 12) & 0x3F)] +
         BYTE_HEX[0x80 | ((cp >> 6) & 0x3F)] +
         BYTE_HEX[0x80 | (cp & 0x3F)]
}

/**
 * @param {Array<string>} input
 * @returns {string}
 */
function stringArrayToHexStripped (input) {
  let acc = ''
  let code = 0
  let i = 0

  for (i = 0; i < input.length; i++) {
    code = input[i].charCodeAt(0)
    if (code === 48) {
      continue
    }
    if (!((code >= 48 && code <= 57) || (code >= 65 && code <= 70) || (code >= 97 && code <= 102))) {
      return ''
    }
    acc += input[i]
    break
  }

  for (i += 1; i < input.length; i++) {
    code = input[i].charCodeAt(0)
    if (!((code >= 48 && code <= 57) || (code >= 65 && code <= 70) || (code >= 97 && code <= 102))) {
      return ''
    }
    acc += input[i]
  }
  return acc
}

/** @type {(value: string) => boolean} */
const isHextet = RegExp.prototype.test.bind(/^[\dA-Fa-f]{1,4}$/)

/** @type {(value: string) => boolean} */
const isIPvFuture = RegExp.prototype.test.bind(/^[vV][\dA-Fa-f]+\.[A-Za-z\d\-._~!$&'()*+,;=:]+$/)

/** @type {(value: string) => boolean} */
const isZoneCharacter = RegExp.prototype.test.bind(/^[A-Za-z\d\-._~]$/)

/**
 * @param {string} value
 * @returns {boolean}
 */
const nonSimpleDomain = RegExp.prototype.test.bind(/[^!"$&'()*+,\-.;=_`a-z{}~]/u)

/**
 * @param {string} zone
 * @returns {boolean}
 */
function isZoneIdentifier (zone) {
  if (zone.length === 0) return false

  for (let i = 0; i < zone.length; i++) {
    if (isZoneCharacter(zone[i])) continue
    if (zone[i] === '%' && i + 2 < zone.length && isHexPair(zone.slice(i + 1, i + 3))) {
      i += 2
      continue
    }
    return false
  }

  return true
}

/**
 * Compresses the longest run of zero hextets to "::" per RFC 5952. A run of a
 * single zero hextet is left uncompressed. On ties the leftmost run wins.
 *
 * @param {string[]} hextets
 * @returns {string}
 */
function compressIPv6ZeroRun (hextets) {
  let bestStart = -1
  let bestLength = 0
  let runStart = -1
  let runLength = 0
  for (let i = 0; i < hextets.length; i++) {
    if (hextets[i] === '0') {
      if (runStart === -1) runStart = i
      runLength++
      if (runLength > bestLength) {
        bestLength = runLength
        bestStart = runStart
      }
    } else {
      runStart = -1
      runLength = 0
    }
  }

  if (bestLength < 2) return hextets.join(':')

  const head = hextets.slice(0, bestStart).join(':')
  const tail = hextets.slice(bestStart + bestLength).join(':')
  return head + '::' + tail
}

/**
 * Validates an IPv6 address against the alternatives in RFC 3986 section
 * 3.2.2 and returns the same address with leading hextet zeroes removed.
 * An embedded IPv4 address counts as two hextets and is only valid at the end.
 *
 * @param {string} input
 * @returns {string|undefined}
 */
function normalizeIPv6Address (input) {
  const compression = input.indexOf('::')
  if (compression !== -1 && input.indexOf('::', compression + 1) !== -1) return undefined

  const left = compression === -1 ? input.split(':') : input.slice(0, compression).split(':')
  const right = compression === -1 ? [] : input.slice(compression + 2).split(':')
  if (compression !== -1) {
    if (left.length === 1 && left[0] === '') left.length = 0
    if (right.length === 1 && right[0] === '') right.length = 0
  }

  const parts = left.concat(right)
  let hextetCount = 0
  for (let i = 0; i < parts.length; i++) {
    const part = parts[i]
    if (part === '') return undefined

    if (part.indexOf('.') !== -1) {
      if (i !== parts.length - 1 || (compression !== -1 && right.length === 0) || !isIPv4(part)) return undefined
      hextetCount += 2
      continue
    }

    if (!isHextet(part)) return undefined
    parts[i] = parseInt(part, 16).toString(16)
    hextetCount++
  }

  if (compression === -1) {
    if (hextetCount !== 8) return undefined
    return compressIPv6ZeroRun(parts)
  }
  if (hextetCount >= 8) return undefined

  // expand "::" then re-compress the longest run for a canonical result
  const expanded = parts.slice(0, left.length)
  for (let i = hextetCount; i < 8; i++) expanded.push('0')
  for (let i = left.length; i < parts.length; i++) expanded.push(parts[i])
  return compressIPv6ZeroRun(expanded)
}

/**
 * @typedef {Object} NormalizeIPv6Result
 * @property {string} host - The normalized host.
 * @property {string} [escapedHost] - The escaped host.
 * @property {boolean} isIPV6 - Indicates if the host is an IPv6 address.
 * @property {boolean} [isIPVFuture] - Indicates if the host is an IPvFuture literal.
 * @property {boolean} [error] - Indicates if a bracketed IP literal is malformed.
 */

/**
 * Validates and normalizes a bracketed IP literal. Raw zone separators remain
 * accepted for backwards compatibility, while encoded separators and zone
 * contents follow RFC 6874.
 *
 * @param {string} host
 * @returns {NormalizeIPv6Result}
 */
function normalizeIPv6 (host) {
  const bracketed = host[0] === '[' && host[host.length - 1] === ']'
  const hasBracket = host[0] === '[' || host[host.length - 1] === ']'
  if (hasBracket && !bracketed) return { host, isIPV6: false, error: true }

  let input = bracketed ? host.slice(1, -1) : host
  if (bracketed && isIPvFuture(input)) {
    input = input.toLowerCase()
    return { host: `[${input}]`, escapedHost: input, isIPV6: false, isIPVFuture: true }
  }

  if (findToken(input, ':') < 2) {
    return { host, isIPV6: false, error: bracketed }
  }

  let zoneIdentifier = ''
  const zoneSeparator = input.indexOf('%')
  if (zoneSeparator !== -1) {
    const separatorLength = input.slice(zoneSeparator, zoneSeparator + 3).toLowerCase() === '%25' ? 3 : 1
    zoneIdentifier = input.slice(zoneSeparator + separatorLength)
    if (!isZoneIdentifier(zoneIdentifier)) return { host, isIPV6: false, error: true }
    input = input.slice(0, zoneSeparator)
  }

  const address = normalizeIPv6Address(input)
  if (address === undefined) return { host, isIPV6: false, error: true }

  return {
    host: address + (zoneIdentifier ? '%' + zoneIdentifier : ''),
    escapedHost: address + (zoneIdentifier ? '%25' + zoneIdentifier : ''),
    isIPV6: true
  }
}

/**
 * @param {string} str
 * @param {string} token
 * @returns {number}
 */
function findToken (str, token) {
  let ind = 0
  for (let i = 0; i < str.length; i++) {
    if (str[i] === token) ind++
  }
  return ind
}

/**
 * @param {string} path
 * @returns {string}
 *
 * @see https://datatracker.ietf.org/doc/html/rfc3986#section-5.2.4
 */
function removeDotSegments (path) {
  let input = path
  const output = []
  let nextSlash = -1
  let len = 0

  // eslint-disable-next-line no-cond-assign
  while (len = input.length) {
    if (len === 1) {
      if (input === '.') {
        break
      } else if (input === '/') {
        output.push('/')
        break
      } else {
        output.push(input)
        break
      }
    } else if (len === 2) {
      if (input[0] === '.') {
        if (input[1] === '.') {
          break
        } else if (input[1] === '/') {
          input = input.slice(2)
          continue
        }
      } else if (input[0] === '/') {
        if (input[1] === '.' || input[1] === '/') {
          output.push('/')
          break
        }
      }
    } else if (len === 3) {
      if (input === '/..') {
        if (output.length !== 0) {
          output.pop()
        }
        output.push('/')
        break
      }
    }
    if (input[0] === '.') {
      if (input[1] === '.') {
        if (input[2] === '/') {
          input = input.slice(3)
          continue
        }
      } else if (input[1] === '/') {
        input = input.slice(2)
        continue
      }
    } else if (input[0] === '/') {
      if (input[1] === '.') {
        if (input[2] === '/') {
          input = input.slice(2)
          continue
        } else if (input[2] === '.') {
          if (input[3] === '/') {
            input = input.slice(3)
            if (output.length !== 0) {
              output.pop()
            }
            continue
          }
        }
      }
    }

    // Rule 2E: Move normal path segment to output
    if ((nextSlash = input.indexOf('/', 1)) === -1) {
      output.push(input)
      break
    } else {
      output.push(input.slice(0, nextSlash))
      input = input.slice(nextSlash)
    }
  }

  return output.join('')
}

/**
 * Re-escape RFC 3986 gen-delims that must not appear literally in the host.
 * After the URI regex parses, these characters cannot be literal in the host
 * field, so any that appear after decoding came from percent-encoding and
 * must be restored to prevent authority structure changes.
 *
 * @param {string} host
 * @param {boolean} isIP - true for IPv4/IPv6 hosts (skip colon re-escaping)
 * @returns {string}
 */
const HOST_DELIMS = { '@': '%40', '/': '%2F', '?': '%3F', '#': '%23', ':': '%3A' }
const HOST_DELIM_RE = /[@/?#:]/g
const HOST_DELIM_NO_COLON_RE = /[@/?#]/g

function reescapeHostDelimiters (host, isIP) {
  const re = isIP ? HOST_DELIM_NO_COLON_RE : HOST_DELIM_RE
  re.lastIndex = 0
  return host.replace(re, (ch) => HOST_DELIMS[ch])
}

/**
 * Normalizes percent escapes and optionally decodes only unreserved ASCII bytes.
 * Reserved delimiters such as `%2F` stay escaped; `%2E` is unreserved.
 *
 * @param {string} input
 * @param {boolean} [decodeUnreserved=false]
 * @returns {string}
 */
function normalizePercentEncoding (input, decodeUnreserved = false) {
  if (input.indexOf('%') === -1) {
    return input
  }

  let output = ''

  for (let i = 0; i < input.length; i++) {
    if (input[i] === '%' && i + 2 < input.length) {
      const hex = input.slice(i + 1, i + 3)
      if (isHexPair(hex)) {
        const normalizedHex = hex.toUpperCase()
        const decoded = String.fromCharCode(parseInt(normalizedHex, 16))

        if (decodeUnreserved && isUnreserved(decoded)) {
          output += decoded
        } else {
          output += '%' + normalizedHex
        }

        i += 2
        continue
      }
    }

    output += input[i]
  }

  return output
}

/**
 * Normalizes path data without turning reserved escapes into live path syntax.
 * Valid escapes are uppercased, raw unsafe characters are escaped, and only
 * unreserved bytes that are not `.` are decoded.
 *
 * @param {string} input
 * @returns {string}
 */
function normalizePathEncoding (input) {
  let output = ''

  for (let i = 0; i < input.length; i++) {
    const ch = input[i]
    if (ch === '%' && i + 2 < input.length) {
      const hex = input.slice(i + 1, i + 3)
      if (isHexPair(hex)) {
        const normalizedHex = hex.toUpperCase()
        const decoded = String.fromCharCode(parseInt(normalizedHex, 16))

        if (decoded !== '.' && isUnreserved(decoded)) {
          output += decoded
        } else {
          output += '%' + normalizedHex
        }

        i += 2
        continue
      }
    }

    if (isPathCharacter(ch)) {
      output += ch
    } else {
      const code = input.charCodeAt(i)
      if (code < 0x80) {
        output += isEscapeSafe(code) ? ch : BYTE_HEX[code]
      } else if (code < 0xD800 || code > 0xDFFF) {
        output += percentEncodeNonAscii(code)
      } else if (code <= 0xDBFF && i + 1 < input.length) {
        const low = input.charCodeAt(i + 1)
        if (low >= 0xDC00 && low <= 0xDFFF) {
          output += percentEncodeNonAscii(0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00))
          i++
        } else {
          output += percentEncodeNonAscii(0xFFFD)
        }
      } else {
        output += percentEncodeNonAscii(0xFFFD)
      }
    }
  }

  return output
}

/**
 * Serializes a path without rewriting reserved data. Raw RFC 3986 path
 * characters remain literal, valid escapes are preserved and uppercased, and
 * everything else is UTF-8 percent-encoded. In a path-noscheme, a colon in the
 * first segment must be escaped so the result cannot be parsed as a scheme.
 *
 * @param {string} input
 * @param {boolean} [pathNoScheme=false]
 * @returns {string}
 */
function serializePathEncoding (input, pathNoScheme = false) {
  let output = ''
  let firstSegment = pathNoScheme && input[0] !== '/'

  for (let i = 0; i < input.length; i++) {
    const ch = input[i]
    if (ch === '%' && i + 2 < input.length) {
      const hex = input.slice(i + 1, i + 3)
      if (isHexPair(hex)) {
        output += '%' + hex.toUpperCase()
        i += 2
        continue
      }
    }

    if (ch === '/') {
      firstSegment = false
    }

    if (isPathCharacter(ch) && (ch !== ':' || !firstSegment)) {
      output += ch
    } else {
      const code = input.charCodeAt(i)
      if (code < 0x80) {
        output += BYTE_HEX[code]
      } else if (code < 0xD800 || code > 0xDFFF) {
        output += percentEncodeNonAscii(code)
      } else if (code <= 0xDBFF && i + 1 < input.length) {
        const low = input.charCodeAt(i + 1)
        if (low >= 0xDC00 && low <= 0xDFFF) {
          output += percentEncodeNonAscii(0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00))
          i++
        } else {
          output += percentEncodeNonAscii(0xFFFD)
        }
      } else {
        output += percentEncodeNonAscii(0xFFFD)
      }
    }
  }

  return output
}

/**
 * Percent-encodes a URI component using its RFC 3986 literal character set.
 * Existing valid escapes are preserved and normalized to uppercase hex.
 *
 * @param {string} input
 * @param {(value: string) => boolean} isAllowed
 * @returns {string}
 */
function encodeComponent (input, isAllowed) {
  let output = ''

  for (let i = 0; i < input.length; i++) {
    const ch = input[i]
    if (ch === '%' && i + 2 < input.length) {
      const hex = input.slice(i + 1, i + 3)
      if (isHexPair(hex)) {
        output += '%' + hex.toUpperCase()
        i += 2
        continue
      }
    }

    if (isAllowed(ch)) {
      output += ch
    } else {
      const code = input.charCodeAt(i)
      if (code < 0x80) {
        output += BYTE_HEX[code]
      } else if (code < 0xD800 || code > 0xDFFF) {
        output += percentEncodeNonAscii(code)
      } else if (code <= 0xDBFF && i + 1 < input.length) {
        const low = input.charCodeAt(i + 1)
        if (low >= 0xDC00 && low <= 0xDFFF) {
          output += percentEncodeNonAscii(0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00))
          i++
        } else {
          output += percentEncodeNonAscii(0xFFFD)
        }
      } else {
        output += percentEncodeNonAscii(0xFFFD)
      }
    }
  }

  return output
}

/**
 * Encodes userinfo while preserving its RFC 3986 §3.2.1 literal characters.
 * In particular, authority delimiters such as `@`, `/`, `?`, and `#` are data.
 *
 * @param {string} input
 * @returns {string}
 */
function encodeUserinfo (input) {
  return encodeComponent(input, isUserinfoCharacter)
}

/**
 * Encodes query data using the RFC 3986 §3.4 grammar. A literal `#` must be
 * escaped because it would otherwise begin the fragment component.
 *
 * @param {string} input
 * @returns {string}
 */
function encodeQuery (input) {
  return encodeComponent(input, isQueryFragmentCharacter)
}

/**
 * Encodes fragment data using the RFC 3986 §3.5 grammar.
 *
 * @param {string} input
 * @returns {string}
 */
function encodeFragment (input) {
  return encodeComponent(input, isQueryFragmentCharacter)
}

function isEscapeSafe (cp) {
  return (
    (cp >= 0x30 && cp <= 0x39) ||
    (cp >= 0x41 && cp <= 0x5A) ||
    (cp >= 0x61 && cp <= 0x7A) ||
    cp === 0x2A || cp === 0x2B || cp === 0x2D || cp === 0x2E ||
    cp === 0x2F || cp === 0x40 || cp === 0x5F
  )
}

/**
 * Normalizes the percent-encoding of a query or fragment component.
 *
 * Like `normalizePathEncoding`, but uses the query/fragment character set
 * (which additionally allows `?`) and decodes `.` since it has no dot-segment
 * meaning outside of a path.
 *
 * @param {string} input
 * @returns {string}
 */
function normalizeQueryFragmentEncoding (input) {
  let output = ''

  for (let i = 0; i < input.length; i++) {
    const ch = input[i]
    if (ch === '%' && i + 2 < input.length) {
      const hex = input.slice(i + 1, i + 3)
      if (isHexPair(hex)) {
        const normalizedHex = hex.toUpperCase()
        const decoded = String.fromCharCode(parseInt(normalizedHex, 16))

        if (isUnreserved(decoded)) {
          output += decoded
        } else {
          output += '%' + normalizedHex
        }

        i += 2
        continue
      }
    }

    if (isQueryFragmentCharacter(ch)) {
      output += ch
    } else {
      const code = input.charCodeAt(i)
      if (code < 0x80) {
        output += isEscapeSafe(code) ? ch : BYTE_HEX[code]
      } else if (code < 0xD800 || code > 0xDFFF) {
        output += percentEncodeNonAscii(code)
      } else if (code <= 0xDBFF && i + 1 < input.length) {
        const low = input.charCodeAt(i + 1)
        if (low >= 0xDC00 && low <= 0xDFFF) {
          output += percentEncodeNonAscii(0x10000 + ((code - 0xD800) << 10) + (low - 0xDC00))
          i++
        } else {
          output += percentEncodeNonAscii(0xFFFD)
        }
      } else {
        output += percentEncodeNonAscii(0xFFFD)
      }
    }
  }

  return output
}

/**
 * Escapes a component while preserving existing valid percent escapes.
 *
 * @param {string} input
 * @returns {string}
 */
function escapePreservingEscapes (input) {
  let output = ''

  for (let i = 0; i < input.length; i++) {
    if (input[i] === '%' && i + 2 < input.length) {
      const hex = input.slice(i + 1, i + 3)
      if (isHexPair(hex)) {
        output += '%' + hex.toUpperCase()
        i += 2
        continue
      }
    }

    output += escape(input[i])
  }

  return output
}

/**
 * @param {import('../types/index').URIComponent} component
 * @returns {string|undefined}
 */
function recomposeAuthority (component) {
  const uriTokens = []

  if (component.userinfo !== undefined) {
    uriTokens.push(encodeUserinfo(component.userinfo))
    uriTokens.push('@')
  }

  if (component.host !== undefined) {
    let host = component.host
    if (!isIPv4(host)) {
      let ipV6res = normalizeIPv6(host)
      if (ipV6res.isIPV6 !== true && ipV6res.isIPVFuture !== true) {
        // Decode only unreserved bytes, once. In particular, keep %25 encoded
        // so it cannot introduce a second escape during recomposition.
        host = normalizePercentEncoding(host, true)
        ipV6res = normalizeIPv6(host)
      }
      if (ipV6res.isIPV6 === true || ipV6res.isIPVFuture === true) {
        host = `[${ipV6res.escapedHost}]`
      } else {
        host = reescapeHostDelimiters(host, false)
      }
    }
    uriTokens.push(host)
  }

  if (typeof component.port === 'number' || typeof component.port === 'string') {
    uriTokens.push(':')
    uriTokens.push(String(component.port))
  }

  return uriTokens.length ? uriTokens.join('') : undefined
};

module.exports = {
  nonSimpleDomain,
  recomposeAuthority,
  reescapeHostDelimiters,
  normalizePercentEncoding,
  normalizePathEncoding,
  serializePathEncoding,
  normalizeQueryFragmentEncoding,
  encodeUserinfo,
  encodeQuery,
  encodeFragment,
  escapePreservingEscapes,
  removeDotSegments,
  isIPv4,
  isUUID,
  normalizeIPv6,
  stringArrayToHexStripped
}
