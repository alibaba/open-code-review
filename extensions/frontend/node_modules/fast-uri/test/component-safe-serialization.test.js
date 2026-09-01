'use strict'

const test = require('tape')
const fastURI = require('..')

test('userinfo serialization cannot terminate the authority', (t) => {
  const uri = fastURI.serialize({
    scheme: 'http',
    userinfo: 'attacker.example/',
    host: 'trusted.example',
    path: '/'
  })

  t.equal(uri, 'http://attacker.example%2F@trusted.example/', 'slash is encoded as userinfo data')

  const reparsed = fastURI.parse(uri)
  t.equal(reparsed.host, 'trusted.example', 'reparsing retains the supplied host')
  t.equal(reparsed.userinfo, 'attacker.example%2F', 'encoded slash remains in userinfo')
  t.equal(
    fastURI.serialize({ userinfo: 'user@/?:#[]\\', host: 'example.test' }),
    '//user%40%2F%3F:%23%5B%5D%5C@example.test',
    'all userinfo component delimiters are encoded as data'
  )
  t.end()
})

test('query and fragment serialization cannot inject component delimiters', (t) => {
  const uri = fastURI.serialize({
    scheme: 'x',
    path: '/resource',
    query: 'key=value#still-query',
    fragment: 'first#still-fragment'
  })

  t.equal(
    uri,
    'x:/resource?key=value%23still-query#first%23still-fragment',
    'raw hashes are encoded within their supplied components'
  )

  const reparsed = fastURI.parse(uri)
  t.equal(reparsed.query, 'key=value%23still-query', 'hash remains query data')
  t.equal(reparsed.fragment, 'first%23still-fragment', 'hash remains fragment data')
  t.end()
})

test('component serializers preserve each RFC 3986 literal character set', (t) => {
  const unreservedAndSubDelims = "AZaz09-._~!$&'()*+,;="
  const userinfo = unreservedAndSubDelims + ':'
  const queryOrFragment = unreservedAndSubDelims + ':@/?'

  t.equal(
    fastURI.serialize({ userinfo, host: 'example.test' }),
    '//' + userinfo + '@example.test',
    'userinfo literals are preserved'
  )
  t.equal(
    fastURI.serialize({ query: queryOrFragment }),
    '?' + queryOrFragment,
    'query literals are preserved'
  )
  t.equal(
    fastURI.serialize({ fragment: queryOrFragment }),
    '#' + queryOrFragment,
    'fragment literals are preserved'
  )
  t.end()
})

test('component serializers preserve escapes and encode Unicode as UTF-8', (t) => {
  t.equal(
    fastURI.serialize({ userinfo: 'café/%2f%GG', host: 'example.test' }),
    '//caf%C3%A9%2F%2F%25GG@example.test',
    'userinfo preserves valid escapes, uppercases hex, and escapes malformed percent data'
  )
  t.equal(
    fastURI.serialize({ query: '日本/%2f#%GG' }),
    '?%E6%97%A5%E6%9C%AC/%2F%23%25GG',
    'query uses UTF-8 and does not double encode valid escapes'
  )
  t.equal(
    fastURI.serialize({ fragment: '😀/%3f#%GG' }),
    '#%F0%9F%98%80/%3F%23%25GG',
    'fragment uses UTF-8 and does not double encode valid escapes'
  )
  t.equal(
    fastURI.serialize({ query: 'Kſ' }),
    '?%E2%84%AA%C5%BF',
    'Unicode characters that case-fold to ASCII are still UTF-8 encoded'
  )
  t.end()
})

test('parsed URI serialization remains stable', (t) => {
  const input = 'x://user:pass@example.test/a%2Fb?x=a/b?c&y=%23#frag/a?b%23'
  const serialized = fastURI.serialize(fastURI.parse(input))

  t.equal(serialized, input, 'an already normalized parsed URI round-trips unchanged')
  t.equal(
    fastURI.serialize(fastURI.parse(serialized)),
    serialized,
    'repeated parse and serialize cycles are stable'
  )
  t.end()
})
