'use strict'

const test = require('tape')
const fastURI = require('..')

test('parse preserves reserved path escapes as data', (t) => {
  const components = fastURI.parse('http://example.com/a%2Fb/public/%2e%2e/admin')

  t.equal(components.path, '/a%2Fb/public/%2E%2E/admin')
  t.end()
})

test('normalize preserves percent-encoded path separators and dot segments', (t) => {
  t.equal(
    fastURI.normalize('http://example.com/public/%2e%2e/admin'),
    'http://example.com/public/%2E%2E/admin'
  )

  t.equal(
    fastURI.normalize('http://example.com/a%2Fb'),
    'http://example.com/a%2Fb'
  )

  t.end()
})

test('equal does not treat reserved path escapes as live path syntax', (t) => {
  t.equal(
    fastURI.equal('http://example.com/public/%2e%2e/admin', 'http://example.com/admin', {}),
    false
  )

  t.equal(
    fastURI.equal('http://example.com/a%2Fb', 'http://example.com/a/b', {}),
    false
  )

  t.end()
})

test('serialize preserves literal RFC 3986 reserved path characters', (t) => {
  // GHSA-7mh8-fcmq-x23c: literal reserved path chars must not be rewritten to
  // percent escapes by the escape()-style safe set.
  const cases = [
    'http://example.com/a;b',
    'http://example.com/a=b',
    'http://example.com/a&b',
    'http://example.com/a$b',
    'http://example.com/a:b',
    'http://example.com/a@b'
  ]

  t.plan(cases.length)

  cases.forEach((uri) => {
    t.equal(fastURI.serialize(fastURI.parse(uri)), uri, uri)
  })
  t.end()
})

test('serialize preserves existing reserved path escapes as data', (t) => {
  // GHSA-7mh8-fcmq-x23c: an existing %3A escape must stay %3A instead of being
  // rewritten into a live colon.
  const cases = [
    'http://example.com/a%3Ab',
    'http://example.com/a%3Bb',
    'http://example.com/a%3D%3D'
  ]

  t.plan(cases.length)

  cases.forEach((uri) => {
    t.equal(fastURI.serialize(fastURI.parse(uri)), uri, uri)
  })
  t.end()
})

test('serialize keeps path-noscheme colon escaping', (t) => {
  // Without a scheme a literal colon would be reparsed as a scheme separator,
  // so it must stay percent-escaped while an existing %3A is preserved.
  t.equal(fastURI.serialize({ path: 'foo:bar' }), 'foo%3Abar')
  t.equal(fastURI.serialize({ path: 'a%3Ab' }), 'a%3Ab')
  t.end()
})

test('hostname normalization never decodes an escape more than once', (t) => {
  const encodedLocalhost = 'http://%256c%256f%2563%2561%256c%2568%256f%2573%2574/'
  const encodedLoopback = '//127%252e0%252e0%252e1/private'
  const encodedMetadataAddress = '//169%252E254%252E169%252E254/latest/meta-data/'

  t.equal(fastURI.normalize(encodedLocalhost), encodedLocalhost, 'nested hostname letters remain encoded')
  t.equal(fastURI.normalize(encodedLoopback), encodedLoopback, 'nested IPv4 dots remain encoded')
  t.equal(
    fastURI.resolve('https://safe.example/', encodedLoopback),
    'https://127%252e0%252e0%252e1/private',
    'resolve does not turn nested dots into a loopback address'
  )
  t.equal(
    fastURI.resolve('https://allowed.com/api/v1/', encodedMetadataAddress),
    'https://169%252e254%252e169%252e254/latest/meta-data/',
    'resolve does not turn nested dots into a metadata address'
  )
  t.equal(
    fastURI.normalize('http://allowed.com%255Cevil.com/'),
    'http://allowed.com%255Cevil.com/',
    'normalize does not activate a nested backslash'
  )
  t.equal(
    fastURI.serialize({ scheme: 'http', host: '%256cocalhost', path: '/' }),
    'http://%256cocalhost/',
    'component serialization preserves an encoded percent sign'
  )
  t.equal(
    fastURI.equal(encodedLocalhost, 'http://localhost/', {}),
    false,
    'nested escapes do not compare equal to their twice-decoded target'
  )
  t.end()
})

test('hostname normalization decodes only current unreserved escapes', (t) => {
  t.equal(fastURI.normalize('x://%6cocalhost/'), 'x://localhost/', 'a current unreserved escape is decoded')
  t.equal(fastURI.normalize('x://%256cocalhost/'), 'x://%256cocalhost/', 'an encoded percent is preserved')
  t.equal(fastURI.normalize('x://host%2540evil/'), 'x://host%2540evil/', 'a nested authority delimiter stays inert')
  t.equal(fastURI.normalize('x://%2525/'), 'x://%2525/', 'nested encoded percent signs stay encoded')
  t.end()
})

test('host conversion failures are not treated as comparable URLs', (t) => {
  const malformedHost = 'http://trusted.test%2540evil.test/'

  t.equal(fastURI.normalize(malformedHost), malformedHost, 'normalization preserves the failing input')
  t.equal(fastURI.equal(malformedHost, malformedHost, {}), false, 'equal rejects a failed host conversion')
  t.throws(
    () => fastURI.resolve(malformedHost, 'child', { domainHost: true }),
    /Host's domain name can not be converted to ASCII/,
    'resolve propagates a host conversion failure'
  )
  t.end()
})
