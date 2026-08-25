'use strict'

const test = require('tape')
const fastURI = require('..')

const malformedURNs = [
  'urn:',
  'URN:',
  'urn:foo',
  'urn::foo',
  'urn:foo:',
  'urn:%66oo:bar'
]

test('parse reports malformed ordinary URNs', (t) => {
  for (const uri of malformedURNs) {
    t.match(fastURI.parse(uri).error, /^URN can not be parsed\.?$/, uri)
  }
  t.end()
})

test('normalize preserves malformed ordinary URNs without throwing', (t) => {
  for (const uri of malformedURNs) {
    t.doesNotThrow(() => fastURI.normalize(uri), `${uri} does not throw`)
    t.equal(fastURI.normalize(uri), uri, `${uri} is preserved`)
  }
  t.equal(
    fastURI.normalize('urn:foo', { reference: 'relative' }),
    'urn:foo',
    'an earlier parse error does not hide the missing URN nid'
  )
  t.end()
})

test('equal returns false for malformed ordinary URNs', (t) => {
  for (const uri of malformedURNs) {
    t.equal(fastURI.equal(uri, uri, {}), false, `${uri} is not equal to itself as malformed input`)
    t.equal(fastURI.equal(uri, 'urn:foo:bar', {}), false, `${uri} is not equal to a valid URN`)
  }
  t.end()
})

test('resolve handles malformed ordinary URNs without throwing', (t) => {
  for (const uri of malformedURNs) {
    t.doesNotThrow(() => fastURI.resolve('uri://base/', uri), `${uri} does not throw as a relative reference`)
    t.doesNotThrow(() => fastURI.resolve(uri, ''), `${uri} does not throw as a base URI`)
  }
  // resolve preserves the malformed scheme-specific input rather than surfacing
  // an uncaught 'URN without nid cannot be serialized' error (matches upstream uri-js)
  t.equal(fastURI.resolve('uri://base/', 'urn:'), 'urn:', 'malformed relative URN is preserved')
  t.equal(fastURI.resolve('URN:', ''), 'urn:', 'scheme case is normalized')
  t.equal(fastURI.resolve('uri://base/', 'urn:%66oo:bar'), 'urn:foo:bar', 'percent-encoding is decoded')
  t.end()
})

test('valid URNs retain their existing behavior', (t) => {
  t.equal(fastURI.normalize('URN:FOO:a123,456'), 'urn:foo:a123,456')
  t.equal(fastURI.equal('urn:foo:a123,456', 'URN:FOO:a123,456', {}), true)
  t.equal(fastURI.resolve('uri://base/', 'urn:'), 'urn:', 'default resolve behavior is unchanged')
  t.end()
})
