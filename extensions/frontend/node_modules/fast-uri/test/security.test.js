'use strict'

const test = require('tape')
const fastURI = require('..')

test('parse marks malformed authority and port inputs as errors', (t) => {
  const malformedCases = [
    {
      input: 'http://[::1]foo',
      expectedError: 'URI path must start with "/" when authority is present.'
    },
    {
      input: 'http://[::1]:80abc/path',
      expectedError: 'URI path must start with "/" when authority is present.'
    },
    {
      input: 'http://example.com:80abc/path',
      expectedError: 'URI path must start with "/" when authority is present.'
    },
    {
      input: 'http://[::1]:65536',
      expectedError: 'URI port is malformed.'
    }
  ]

  t.plan(malformedCases.length)

  malformedCases.forEach(({ input, expectedError }) => {
    t.equal(fastURI.parse(input).error, expectedError, input)
  })
})

test('normalize does not canonicalize malformed URLs into different valid URLs', (t) => {
  const malformedCases = [
    'http://[::1]foo',
    'http://[::1]:80abc/path',
    'http://example.com:80abc/path',
    'http://[::1]:65536'
  ]

  t.plan(malformedCases.length)

  malformedCases.forEach((input) => {
    t.equal(fastURI.normalize(input), input, input)
  })
})

test('equal returns false when either side is malformed', (t) => {
  const malformedPairs = [
    ['http://[::1]foo', 'http://[::1]/foo'],
    ['http://[::1]:80abc/path', 'http://[::1]/abc/path'],
    ['http://example.com:80abc/path', 'http://example.com/abc/path'],
    ['http://[::1]:65536', 'http://[::1]:65536/']
  ]

  t.plan(malformedPairs.length)

  malformedPairs.forEach(([left, right]) => {
    t.equal(fastURI.equal(left, right), false, `${left} != ${right}`)
  })
})

test('normalize preserves encoded authority delimiters in host', (t) => {
  const cases = [
    ['http://trusted.com%40evil.com/', 'http://trusted.com%40evil.com/'],
    ['http://example.com%3A8080/', 'http://example.com%3A8080/'],
    ['http://example.com%2Fevil.com/path', 'http://example.com%2Fevil.com/path'],
    ['http://example.com%23fragment/path', 'http://example.com%23fragment/path'],
    ['http://example.com%3Fq=evil/path', 'http://example.com%3Fq=evil/path'],
    ['http://user%3Apass%40evil.com/', 'http://user%3Apass%40evil.com/'],
    ['http://user@trusted.com%40evil.com/', 'http://user@trusted.com%40evil.com/'],
    ['https://trusted.com%40evil.com/', 'https://trusted.com%40evil.com/'],
    ['ws://trusted.com%40evil.com/chat', 'ws://trusted.com%40evil.com/chat'],
    ['wss://trusted.com%40evil.com/chat', 'wss://trusted.com%40evil.com/chat']
  ]

  t.plan(cases.length)

  cases.forEach(([input, expected]) => {
    t.equal(fastURI.normalize(input), expected, input)
  })
})

test('parse preserves encoded authority delimiters in host', (t) => {
  const cases = [
    ['http://trusted.com%40evil.com/', 'trusted.com%40evil.com'],
    ['http://example.com%3A8080/', 'example.com%3A8080'],
    ['http://user%3Apass%40evil.com/', 'user%3Apass%40evil.com']
  ]

  t.plan(cases.length)

  cases.forEach(([input, expectedHost]) => {
    t.equal(fastURI.parse(input).host, expectedHost, input)
  })
})

test('equal returns false when encoded delimiters differ from live delimiters', (t) => {
  const pairs = [
    ['http://trusted.com%40evil.com/', 'http://trusted.com@evil.com/'],
    ['http://example.com%3A8080/', 'http://example.com:8080/']
  ]

  t.plan(pairs.length)

  pairs.forEach(([left, right]) => {
    t.equal(fastURI.equal(left, right, {}), false, `${left} != ${right}`)
  })
})

test('resolve preserves encoded authority delimiters', (t) => {
  const result = fastURI.resolve('http://base.com/', '//trusted.com%40evil.com/path')
  const parsed = fastURI.parse(result)

  t.plan(1)
  t.notEqual(parsed.host, 'evil.com', '//trusted.com%40evil.com/path')
})

test('serialize escapes authority delimiters in host field', (t) => {
  const result = fastURI.serialize({ scheme: 'http', host: 'trusted.com@evil.com', path: '/' })
  const parsed = fastURI.parse(result)

  t.plan(1)
  t.notEqual(parsed.host, 'evil.com', 'host: trusted.com@evil.com')
})

test('normalize does not double-decode %2540 into a live @', (t) => {
  const input = 'http://trusted.com%2540evil.com/'
  const result = fastURI.normalize(input)
  const parsed = fastURI.parse(result)

  t.plan(2)
  t.equal(result, input, 'the encoded percent sign is preserved')
  t.notEqual(parsed.host, 'trusted.com@evil.com', input)
})

test('parse canonicalises IDN / Unicode hosts to their ASCII form', (t) => {
  const cases = [
    {
      input: 'http://127。0。0。1/',
      expectedHost: '127.0.0.1',
      description: 'full-width ideographic stops as octet separators'
    },
    {
      input: 'http://ｅxample.com/',
      expectedHost: 'example.com',
      description: 'fullwidth e as first letter'
    },
    {
      input: 'http://納豆.example.org/',
      expectedHost: 'xn--99zt52a.example.org',
      description: 'CJK label requiring punycode'
    }
  ]

  t.plan(cases.length * 2)

  cases.forEach(({ input, expectedHost, description }) => {
    const parsed = fastURI.parse(input)
    t.notOk(parsed.error, `parse should not set error: ${description}`)
    t.equal(parsed.host, expectedHost, `host canonicalised to ASCII: ${description}`)
  })
})

test('resolve canonicalises the host using the final resolved scheme', (t) => {
  const cases = [
    {
      base: 'http://trusted.example/base',
      relative: '//127\u30020\u30020\u30021/private',
      expected: 'http://127.0.0.1/private',
      expectedHost: '127.0.0.1',
      description: 'scheme-relative loopback host'
    },
    {
      base: 'https://ex\u00ADample.com/base',
      relative: 'child',
      expected: 'https://example.com/child',
      expectedHost: 'example.com',
      description: 'host inherited from the base'
    },
    {
      base: 'http://trusted.example/base',
      relative: 'http://ex\u200Bample.com/',
      expected: 'http://example.com/',
      expectedHost: 'example.com',
      description: 'absolute relative reference'
    }
  ]

  t.plan(cases.length * 2)

  cases.forEach(({ base, relative, expected, expectedHost, description }) => {
    const resolved = fastURI.resolve(base, relative)
    t.equal(resolved, expected, description)
    t.equal(fastURI.parse(resolved).host, expectedHost, `${description} reparses consistently`)
  })
})

test('resolve applies domain canonicalisation only when the effective scheme opts in', (t) => {
  const host = 'ex\u00ADample.com'

  t.plan(2)
  t.equal(
    fastURI.resolve('uri://trusted.example/', `//${host}/`),
    `uri://${host}/`,
    'an unsupported scheme preserves the host'
  )
  t.equal(
    fastURI.resolve('http://trusted.example/', `//${host}/`, { unicodeSupport: true }),
    `http://${host}/`,
    'unicodeSupport preserves the Unicode host'
  )
})

test('resolve throws when the final scheme cannot canonicalise the host', (t) => {
  const invalidHost = '\u200D.example'
  const cases = [
    ['http://trusted.example/', `//${invalidHost}/`],
    [`https://${invalidHost}/base`, 'child'],
    ['http://trusted.example/', `http://${invalidHost}/`],
    ['http://trusted.example/', `//${invalidHost}%2Etest/`]
  ]

  t.plan(cases.length)

  cases.forEach(([base, relative]) => {
    t.throws(
      () => fastURI.resolve(base, relative),
      /Host's domain name can not be converted to ASCII/,
      `${base} + ${relative}`
    )
  })
})

test('parse rejects a literal backslash in the authority as malformed (RFC 3986)', (t) => {
  // Regression for the host-confusion bypass: a literal "\" is invalid RFC 3986
  // syntax and must be flagged malformed, not silently rewritten. Otherwise "\"
  // acts as a host delimiter here while Node's native URL parses a different
  // host, defeating a host-based SSRF/redirect/origin allowlist.
  const cases = [
    'http://evil.com\\@allowed.com',
    'https://169.254.169.254\\@trusted.example.com',
    'http://127.0.0.1\\@public.example.com',
    'https://attacker.com\\@api.internal',
    'http://a\\@b',
    'ws://evil.com\\@allowed.com/chat',
    'wss://evil.com\\@allowed.com/chat',
    'http://evil.com\\%40allowed.com',
    '//evil.com\\@allowed.com'
  ]

  t.plan(cases.length)

  cases.forEach((input) => {
    t.equal(
      fastURI.parse(input).error,
      'URI authority must not contain a literal backslash.',
      input
    )
  })
})

test('normalize does not canonicalize a literal-backslash URI into a different valid URL', (t) => {
  const cases = [
    'http://evil.com\\@allowed.com',
    'https://attacker.com\\@api.internal'
  ]

  t.plan(cases.length)

  cases.forEach((input) => {
    t.equal(fastURI.normalize(input), input, input)
  })
})

test('parse leaves percent-encoded %5C untouched as encoded data (not rejected)', (t) => {
  // Only the literal "\" byte is rejected; %5C stays valid encoded data and
  // does not diverge from the native URL parser, so it must not be flagged.
  const input = 'http://evil.com%5C@allowed.com'
  const parsed = fastURI.parse(input)

  t.plan(2)
  t.notOk(parsed.error, '%5C is valid encoded data, not malformed')
  t.equal(parsed.host, new URL(input).hostname, '%5C host matches native URL (no divergence)')
})

test('parse does not reject a literal backslash in the query or fragment', (t) => {
  // The rejection is scoped to the authority/path (the host-confusion surface);
  // a backslash after "?"/"#" is normalized as encoded data as before.
  const parsed = fastURI.parse('http://host.example.com/?x=\\y#z\\w')

  t.plan(2)
  t.notOk(parsed.error, 'backslash in query/fragment does not mark the URI malformed')
  t.equal(parsed.host, 'host.example.com', 'host parsed normally')
})

test('parse rejects a malformed authority introducer (\\\\, /\\, \\/) in place of //', (t) => {
  // Regression: "\\", "/\\", "\\/" after the scheme colon are not valid authority
  // introducers. Node's URL treats "\\" as interchangeable with "/" on special
  // schemes, so "http:\\\\evil.com/path" would be parsed as host "evil.com" by
  // Node, but fast-uri must reject it as malformed to prevent SSRF/redirect bypass.
  const cases = [
    'http:\\\\evil.com/path',
    'http:/\\evil.com/path',
    'http:\\/evil.com/path',
    'ws:\\\\evil.com/chat',
    'wss:\\\\evil.com/chat',
    'ftp:\\\\evil.com/',
    '\\\\evil.com/path'
  ]

  t.plan(cases.length)

  cases.forEach((input) => {
    t.equal(
      fastURI.parse(input).error,
      'URI authority must not contain a literal backslash.',
      input
    )
  })
})

test('normalize does not canonicalize a malformed-authority-introducer URI', (t) => {
  const cases = [
    'http:\\\\evil.com/path',
    'http:/\\evil.com/path'
  ]

  t.plan(cases.length)

  cases.forEach((input) => {
    t.equal(fastURI.normalize(input), input, input)
  })
})

test('equal returns false for malformed-authority-introducer URIs', (t) => {
  const pairs = [
    ['http:\\\\evil.com/path', 'http://evil.com/path'],
    ['http:/\\evil.com/path', 'http://evil.com/path']
  ]

  t.plan(pairs.length)

  pairs.forEach(([left, right]) => {
    t.equal(fastURI.equal(left, right), false, `${left} != ${right}`)
  })
})

test('resolve throws on malformed authority introducer', (t) => {
  // resolve() returns a plain string with no error field, so the only safe
  // behavior is to throw when either component has a malformed authority.
  const pairs = [
    ['https://allowed.com/', '\\\\evil.com/path'],
    ['\\\\evil.com/path', 'https://allowed.com/'],
    ['https://allowed.com/', 'http:/\\evil.com/path'],
    ['https://allowed.com/', 'http:\\/evil.com/path']
  ]

  t.plan(pairs.length)

  pairs.forEach(([base, rel]) => {
    t.throws(
      () => fastURI.resolve(base, rel),
      /URI authority must not contain a literal backslash/,
      `${base} + ${rel}`
    )
  })
})

test('parse rejects a whitespace-split authority introducer (TAB, LF, CR)', (t) => {
  // The WHATWG URL parser removes TAB (U+0009), LF (U+000A) and CR (U+000D) from
  // the input before parsing, so a stripped character wedged into the introducer
  // ("/<TAB>\\", "/<TAB>/", or a leading "<TAB>//") reaches an authority in Node
  // while fast-uri would otherwise fold it into the path. These must be rejected
  // like the adjacent "\\", "/\\", "\\/" forms.
  const cases = [
    { input: '/\t\\evil.com/path', expectedError: 'URI authority must not contain a literal backslash.' },
    { input: '/\t/evil.com/path', expectedError: 'URI authority introducer must not contain whitespace.' },
    { input: '/\n\\evil.com/path', expectedError: 'URI authority must not contain a literal backslash.' },
    { input: '/\r\\evil.com/path', expectedError: 'URI authority must not contain a literal backslash.' },
    { input: '\t//evil.com/path', expectedError: 'URI authority introducer must not contain whitespace.' },
    { input: '\t/\\evil.com/path', expectedError: 'URI authority must not contain a literal backslash.' },
    { input: 'https:/\t/evil.com/path', expectedError: 'URI authority introducer must not contain whitespace.' }
  ]

  t.plan(cases.length)

  cases.forEach(({ input, expectedError }) => {
    t.equal(fastURI.parse(input).error, expectedError, JSON.stringify(input))
  })
})

test('resolve throws on a whitespace-split authority introducer', (t) => {
  const pairs = [
    ['https://allowed.com/', '/\t\\evil.com/path'],
    ['https://allowed.com/', '/\t/evil.com/path'],
    ['https://allowed.com/', '/\n\\evil.com/path'],
    ['/\t/evil.com/path', 'https://allowed.com/']
  ]

  t.plan(pairs.length)

  pairs.forEach(([base, rel]) => {
    t.throws(
      () => fastURI.resolve(base, rel),
      /URI authority (must not contain a literal backslash|introducer must not contain whitespace)/,
      `${JSON.stringify(base)} + ${JSON.stringify(rel)}`
    )
  })
})

test('parse does not reject valid authority introducer patterns', (t) => {
  // No false positives: "//" introducer and scheme-less "//" must be valid.
  const cases = [
    'http://good.com/',
    'https://good.com/',
    'ws://good.com/chat',
    'wss://good.com/chat',
    'ftp://good.com/',
    '//good.com/path',
    '/absolute/path',
    'relative/path'
  ]

  t.plan(cases.length)

  cases.forEach((input) => {
    const parsed = fastURI.parse(input)
    t.notOk(parsed.error, input)
  })
})
