'use strict'

const test = require('tape')
const fastURI = require('..')

test('query percent-encoding is normalized (RFC 3986 §6.2.2)', (t) => {
  // hex digits are uppercased, like the path component already does
  t.equal(fastURI.normalize('x://h/p?a=%2a'), 'x://h/p?a=%2A', 'uppercases hex in query')
  // unreserved characters are decoded
  t.equal(fastURI.normalize('x://h/p?a=%7e%2e'), 'x://h/p?a=~.', 'decodes unreserved in query')
  // reserved characters stay percent-encoded
  t.equal(fastURI.normalize('x://h/p?a=%2F'), 'x://h/p?a=%2F', 'keeps reserved encoded in query')
  // raw characters that are not allowed are percent-encoded
  t.equal(fastURI.normalize('x://h/p?a b'), 'x://h/p?a%20b', 'encodes a raw space in query')
  t.equal(fastURI.normalize('x://h/p?café'), 'x://h/p?caf%C3%A9', 'encodes raw non-ASCII in query')
  // `?` is allowed unencoded in a query
  t.equal(fastURI.normalize('x://h/p?a?b'), 'x://h/p?a?b', 'keeps `?` raw in query')

  t.end()
})

test('fragment percent-encoding is normalized without decoding reserved characters', (t) => {
  // reserved characters must NOT be decoded — `%2F` is not `/`
  t.equal(fastURI.normalize('x://h/p#a%2Fb%2A'), 'x://h/p#a%2Fb%2A', 'keeps reserved encoded in fragment')
  // hex is uppercased and unreserved is decoded
  t.equal(fastURI.normalize('x://h/p#a%2a%7e'), 'x://h/p#a%2A~', 'uppercases hex and decodes unreserved in fragment')
  // raw characters are still encoded
  t.equal(fastURI.normalize('x://h/p#a b'), 'x://h/p#a%20b', 'encodes a raw space in fragment')
  // `?` and `/` are allowed unencoded in a fragment
  t.equal(fastURI.normalize('x://h/p#f?x/y'), 'x://h/p#f?x/y', 'keeps `?` and `/` raw in fragment')

  t.end()
})
