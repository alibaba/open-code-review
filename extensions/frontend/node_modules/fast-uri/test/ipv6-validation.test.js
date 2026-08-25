'use strict'

const test = require('tape')
const fastURI = require('..')

const HOST_ERROR = 'URI host is malformed.'

const malformedLiterals = [
  '::not-valid',
  'fc00::not-hex',
  'fe80::not-hex',
  '1:2:3',
  '1:2:3:4:5:6:7',
  '1:2:3:4:5:6:7:8:9',
  '1::2::3',
  '1:::2',
  ':::1',
  '12345::',
  '1:2:3:4:5:6:7::8',
  '::ffff:192.0.2.999',
  '::ffff:192.0.2',
  '::ffff:192.168.001.1',
  '::192.0.2.1:1',
  '1:2:3:4:5:192.0.2.1::',
  'v.foo',
  'v1.',
  'v1.foo%25bar',
  'v1.K',
  'fe80::1%25',
  'fe80::1%25eth 0',
  'fe80::1%25eth%ZZ',
  'fe80::1%25K',
  'not-an-ip'
]

test('malformed bracketed IP literals fail without being rewritten', (t) => {
  for (const literal of malformedLiterals) {
    const uri = `http://[${literal}]/private`
    const parsed = fastURI.parse(uri)

    t.equal(parsed.error, HOST_ERROR, `parse rejects ${literal}`)
    t.equal(parsed.host, `[${literal.toLowerCase()}]`, `parse does not truncate ${literal}`)
    t.equal(fastURI.normalize(uri), uri, `normalize preserves ${literal}`)
    t.equal(fastURI.equal(uri, uri), false, `equal rejects ${literal}`)
  }
  t.end()
})

test('resolve throws for malformed bracketed IP literals', (t) => {
  for (const literal of malformedLiterals) {
    const uri = `http://[${literal}]/private`

    t.throws(
      () => fastURI.resolve(uri, 'child'),
      /URI host is malformed\./,
      `rejects malformed base ${literal}`
    )
    t.throws(
      () => fastURI.resolve('http://example.com/', uri),
      /URI host is malformed\./,
      `rejects malformed relative input ${literal}`
    )
  }
  t.end()
})

test('valid IPv6, IPvFuture, embedded IPv4, and zone forms normalize safely', (t) => {
  const cases = [
    ['http://[::]/', 'http://[::]/', '::'],
    ['http://[::1]/', 'http://[::1]/', '::1'],
    ['http://[1::]/', 'http://[1::]/', '1::'],
    ['http://[2001:0DB8::0001]/', 'http://[2001:db8::1]/', '2001:db8::1'],
    ['http://[0:0:0:0:0:0:0:0]/', 'http://[::]/', '::'],
    ['http://[::ffff:192.0.2.1]/', 'http://[::ffff:192.0.2.1]/', '::ffff:192.0.2.1'],
    ['http://[1:2:3:4:5:6:192.0.2.1]/', 'http://[1:2:3:4:5:6:192.0.2.1]/', '1:2:3:4:5:6:192.0.2.1'],
    ['http://[fe80::A%25EN1]/', 'http://[fe80::a%25EN1]/', 'fe80::a%EN1'],
    ['http://[fe80::a%en1]/', 'http://[fe80::a%25en1]/', 'fe80::a%en1'],
    ['http://[fe80::a%25eth%2D0]/', 'http://[fe80::a%25eth%2D0]/', 'fe80::a%eth%2D0'],
    ['http://[v1.example]/', 'http://[v1.example]/', '[v1.example]'],
    ['http://[vF.A:b]/', 'http://[vf.a:b]/', '[vf.a:b]']
  ]

  for (const [uri, normalized, host] of cases) {
    const parsed = fastURI.parse(uri)
    t.equal(parsed.error, undefined, `${uri} parses without error`)
    t.equal(parsed.host, host, `${uri} has the expected host`)
    t.equal(fastURI.normalize(uri), normalized, `${uri} normalizes safely`)
  }
  t.end()
})
