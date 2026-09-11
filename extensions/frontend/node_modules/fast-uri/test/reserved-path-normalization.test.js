'use strict'

const test = require('tape')
const fastURI = require('..')

const PATH_RESERVED = "!$&'()*+,;=:@/"

function percentEncode (character, lowerCase) {
  const hex = character.charCodeAt(0).toString(16).padStart(2, '0')
  return '%' + (lowerCase ? hex : hex.toUpperCase())
}

test('normalize preserves literal and escaped reserved path characters', (t) => {
  t.equal(
    fastURI.normalize('http://example.com/a;b'),
    'http://example.com/a;b',
    'literal semicolon remains literal'
  )
  t.equal(
    fastURI.normalize('http://example.com/a%3ab'),
    'http://example.com/a%3Ab',
    'escaped colon remains escaped and its hex is uppercased'
  )

  for (const character of PATH_RESERVED) {
    const literal = `http://example.com/a${character}b`
    const escaped = `http://example.com/a${percentEncode(character, true)}b`
    const normalizedEscape = `http://example.com/a${percentEncode(character, false)}b`

    t.equal(fastURI.normalize(literal), literal, `preserves literal ${character}`)
    t.equal(fastURI.normalize(escaped), normalizedEscape, `preserves escaped ${character}`)
  }

  t.equal(
    fastURI.normalize('http://example.com/a/./café'),
    'http://example.com/a/caf%C3%A9',
    'removes real dot segments and UTF-8 encodes raw non-ASCII'
  )
  t.equal(
    fastURI.normalize('http://example.com/a/%2e%2e/b'),
    'http://example.com/a/%2E%2E/b',
    'preserves escaped dots as path data'
  )
  t.equal(
    fastURI.normalize('http://example.com/Kſ'),
    'http://example.com/%E2%84%AA%C5%BF',
    'UTF-8 encodes Unicode characters that case-fold to ASCII'
  )
  t.end()
})

test('serialize uses the RFC 3986 path character set without opening escapes', (t) => {
  const rawPath = `/a${PATH_RESERVED}b`
  t.equal(
    fastURI.serialize({ scheme: 'http', host: 'example.com', path: rawPath }),
    `http://example.com${rawPath}`,
    'keeps all legal raw path characters'
  )

  const escapedPath = Array.from(PATH_RESERVED, (character) => percentEncode(character, true)).join('')
  const normalizedEscapedPath = Array.from(PATH_RESERVED, (character) => percentEncode(character, false)).join('')
  t.equal(
    fastURI.serialize({ scheme: 'http', host: 'example.com', path: '/' + escapedPath }),
    'http://example.com/' + normalizedEscapedPath,
    'uppercases valid escapes without decoding reserved characters'
  )

  t.equal(
    fastURI.serialize({ scheme: 'http', host: 'example.com', path: '/a?b#c[d]' }),
    'http://example.com/a%3Fb%23c%5Bd%5D',
    'encodes characters that would leave the path component'
  )
  t.equal(
    fastURI.serialize({ path: 'a:b/c:d' }),
    'a%3Ab/c:d',
    'only escapes a colon where path-noscheme requires it'
  )
  t.equal(
    fastURI.serialize({ path: './a:b/c:d' }),
    'a%3Ab/c:d',
    'escapes a first-segment colon exposed by dot-segment removal'
  )
  t.equal(
    fastURI.normalize('./a:b'),
    'a%3Ab',
    'normalization keeps a relative path from becoming a scheme'
  )
  t.equal(
    fastURI.serialize({ path: '/Kſ' }),
    '/%E2%84%AA%C5%BF',
    'UTF-8 encodes Unicode characters that case-fold to ASCII'
  )
  t.end()
})

test('equal distinguishes escaped reserved path data from literal syntax', (t) => {
  for (const character of PATH_RESERVED) {
    const literal = `http://example.com/a${character}b`
    const escaped = `http://example.com/a${percentEncode(character, false)}b`
    t.equal(fastURI.equal(literal, escaped, {}), false, `distinguishes literal and escaped ${character}`)
  }

  t.equal(
    fastURI.equal('./a:b', 'a:b', {}),
    false,
    'does not equate a relative path with an absolute URI after removing dot segments'
  )
  t.end()
})
