'use strict'

const test = require('tape')
const fastURI = require('..')

const MALFORMED_SCHEME_ERROR = 'URI scheme is malformed.'

const malformedSchemes = [
  '%2f%2fevil.example:/pwn',
  '%u002f%u002fevil.example:/pwn',
  '%0d%0aSet-Cookie:%20sid=attacker:/p',
  'foo%3Abar:value',
  'foo%2Fbar:value',
  '1http://example.com/',
  'foo_bar:value',
  'éxample:value',
  'Kttp://example.com/',
  'ſcheme:value'
]

test('parse validates the decoded scheme against RFC 3986', (t) => {
  const validSchemes = [
    ['a:value', 'a'],
    ['HTTP://example.com/', 'http'],
    ['a1+.-:value', 'a1+.-'],
    ['%4Aavascript:alert(1)', 'javascript'],
    ['foo%2Bbar:value', 'foo+bar'],
    ['%u006Aavascript:1', 'javascript'],
    ['ht%74ps://example.com/', 'https']
  ]

  for (const [uri, scheme] of validSchemes) {
    const parsed = fastURI.parse(uri)
    t.equal(parsed.error, undefined, uri)
    t.equal(parsed.scheme, scheme, uri + ' scheme')
  }

  for (const uri of malformedSchemes) {
    const parsed = fastURI.parse(uri)
    t.equal(parsed.error, MALFORMED_SCHEME_ERROR, uri)
  }
  t.end()
})

test('decoded schemes select their scheme handlers', (t) => {
  t.equal(
    fastURI.normalize('ht%74ps://example.com:443'),
    'https://example.com/',
    'HTTP normalization runs after decoding the scheme'
  )

  // 3.x has no mailto scheme handler (added in v4); use the ws handler to
  // verify that a percent-encoded scheme still selects its scheme handler
  const ws = fastURI.parse('we%62socket://example.com/')
  t.equal(ws.scheme, 'websocket', 'ws parsing runs after decoding the scheme')
  t.end()
})

test('normalize preserves schemes that decode to invalid identifiers', (t) => {
  for (const uri of malformedSchemes) {
    t.equal(fastURI.normalize(uri), uri, uri)
  }
  t.end()
})

test('scheme normalization cannot introduce authority or control delimiters', (t) => {
  const authority = '%2f%2fevil.example:/pwn'
  const crlf = '%0d%0aSet-Cookie:%20sid=attacker:/p'

  t.equal(fastURI.parse(authority).host, undefined, 'original input has no authority')
  t.equal(fastURI.normalize(authority), authority, 'normalization does not create an authority')
  t.equal(fastURI.normalize(crlf), crlf, 'normalization does not emit raw CRLF')
  t.equal(fastURI.normalize(crlf).includes('\r\n'), false, 'normalized output contains no raw CRLF')
  t.end()
})

test('equal returns false for malformed decoded schemes', (t) => {
  for (const uri of malformedSchemes) {
    t.equal(fastURI.equal(uri, uri, {}), false, uri)
  }
  t.end()
})

test('resolve rejects malformed decoded schemes in either input', (t) => {
  t.throws(
    () => fastURI.resolve('%2f%2fevil.example:/base', 'child'),
    /URI scheme is malformed\./,
    'malformed base'
  )
  t.throws(
    () => fastURI.resolve('https://allowed.example/app/', '%2f%2fevil.example:/pwn'),
    /URI scheme is malformed\./,
    'malformed relative reference'
  )
  t.end()
})

test('serialize validates decoded component schemes', (t) => {
  t.equal(
    fastURI.serialize({ scheme: 'foo%2Bbar', path: 'value' }),
    'foo+bar:value',
    'valid decoded scheme is serialized'
  )
  t.throws(
    () => fastURI.serialize({ scheme: '//evil.example', path: '/pwn' }),
    /URI scheme is malformed\./,
    'raw invalid scheme'
  )
  t.throws(
    () => fastURI.serialize({ scheme: '%2f%2fevil.example', path: '/pwn' }),
    /URI scheme is malformed\./,
    'encoded invalid scheme'
  )
  t.equal(
    fastURI.equal(
      { scheme: '%2f%2fevil.example', path: '/pwn' },
      { scheme: '%2f%2fevil.example', path: '/pwn' },
      {}
    ),
    false,
    'equality fails closed for malformed component objects'
  )
  t.end()
})
