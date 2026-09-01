'use strict'

const test = require('tape')
const fastURI = require('..')

const MALFORMED_PERCENT_ERROR = 'URI contains malformed percent-encoding.'

const malformedComponents = [
  ['path', 'x://example.test/a%'],
  ['path with one trailing hex digit', 'x://example.test/a%2'],
  ['path with non-hex digits', 'x://example.test/a%GG'],
  ['host', 'x://exa%mple.test/path'],
  ['userinfo', 'x://us%er@example.test/path'],
  ['query', 'x://example.test/path?q=%G0'],
  ['fragment', 'x://example.test/path#frag%1']
]

test('parse reports malformed percent syntax in generic URI components', (t) => {
  for (const [component, uri] of malformedComponents) {
    t.equal(fastURI.parse(uri).error, MALFORMED_PERCENT_ERROR, component)
  }
  t.end()
})

test('normalize preserves malformed percent input unchanged', (t) => {
  for (const [component, uri] of malformedComponents) {
    t.equal(fastURI.normalize(uri), uri, component)
  }
  t.end()
})

test('equal does not equate malformed input with repaired valid input', (t) => {
  const cases = [
    ['x://example.test/a%', 'x://example.test/a%25'],
    ['x://exa%mple.test/path', 'x://exa%25mple.test/path'],
    ['x://us%er@example.test/path', 'x://us%25er@example.test/path'],
    ['x://example.test/path?q=%', 'x://example.test/path?q=%25'],
    ['x://example.test/path#%', 'x://example.test/path#%25']
  ]

  for (const [malformed, repaired] of cases) {
    t.equal(fastURI.equal(malformed, repaired, {}), false, malformed)
  }
  t.end()
})

test('resolve rejects malformed percent syntax in either input', (t) => {
  t.throws(
    () => fastURI.resolve('x://example.test/base%', 'child'),
    /URI contains malformed percent-encoding\./,
    'malformed base'
  )
  t.throws(
    () => fastURI.resolve('x://example.test/base', 'child%'),
    /URI contains malformed percent-encoding\./,
    'malformed relative reference'
  )
  t.end()
})

test('valid percent octets remain valid without UTF-8 validation', (t) => {
  const uri = '/%ff?q=%80#%fe'
  const parsed = fastURI.parse(uri)

  t.equal(parsed.error, undefined, 'non-UTF-8 octets are valid percent syntax')
  t.equal(parsed.path, '/%FF', 'path octet is preserved and hex is uppercased')
  t.equal(parsed.query, 'q=%80', 'query octet is preserved')
  t.equal(parsed.fragment, '%FE', 'fragment octet is preserved and hex is uppercased')
  t.equal(fastURI.normalize(uri), '/%FF?q=%80#%FE', 'normalization preserves the octets')

  const allGenericComponents = fastURI.parse('x://u%2f@exa%2fmple.test/%2f?q=%2f#%2f')
  t.equal(allGenericComponents.error, undefined, 'valid escapes are accepted in every generic component')

  const ipv6Zone = fastURI.parse('//[2001:db8::7%en0]')
  t.equal(ipv6Zone.error, undefined, 'historically accepted raw IPv6 zone separator is unchanged')
  t.end()
})
