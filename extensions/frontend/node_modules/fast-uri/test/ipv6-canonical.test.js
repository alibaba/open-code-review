'use strict'

const test = require('tape')
const fastURI = require('..')

test('IPv6 hosts normalize to the RFC 5952 canonical form', (t) => {
  const cases = [
    ['http://[0:0:0:0:0:0:0:1]/', 'http://[::1]/'],
    ['http://[0:0:0:0:0:0:0:0]/', 'http://[::]/'],
    ['http://[2001:0db8:0000:0000:0000:0000:0000:0001]/', 'http://[2001:db8::1]/'],
    ['http://[2001:0:0:0:0:0:0:1]/', 'http://[2001::1]/'],
    ['http://[fe80:0:0:0:0:0:0:1]/', 'http://[fe80::1]/'],
    ['http://[1:0:0:0:2:0:0:3]/', 'http://[1::2:0:0:3]/']
  ]

  for (const [uri, normalized] of cases) {
    t.equal(fastURI.normalize(uri), normalized, `${uri} normalizes to ${normalized}`)
  }
  t.end()
})

test('IPv6 equal() matches the same address across compressed and expanded forms', (t) => {
  const pairs = [
    ['http://[::1]/', 'http://[0:0:0:0:0:0:0:1]/'],
    ['http://[::]/', 'http://[0:0:0:0:0:0:0:0]/'],
    ['http://[2001:db8::1]/', 'http://[2001:0db8:0000:0000:0000:0000:0000:0001]/'],
    ['http://[1::2:0:0:3]/', 'http://[1:0:0:0:2:0:0:3]/']
  ]

  for (const [a, b] of pairs) {
    t.equal(fastURI.equal(a, b), true, `${a} equals ${b}`)
  }
  t.end()
})
