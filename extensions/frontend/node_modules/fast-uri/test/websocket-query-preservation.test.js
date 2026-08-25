'use strict'

const test = require('tape')
const fastURI = require('..')

test('WebSocket queries preserve additional question marks', (t) => {
  for (const scheme of ['ws', 'wss']) {
    const uri = `${scheme}://example.com/chat?a?b`
    const truncatedURI = `${scheme}://example.com/chat?a`
    const parsed = fastURI.parse(uri)

    t.equal(parsed.resourceName, '/chat?a?b', `${scheme} parse preserves the resource name`)
    t.equal(fastURI.serialize(parsed), uri, `${scheme} parsed components round-trip`)
    t.equal(
      fastURI.serialize({ scheme, host: 'example.com', resourceName: '/chat?a?b' }),
      uri,
      `${scheme} resource name serializes without truncation`
    )
    t.equal(fastURI.normalize(uri), uri, `${scheme} normalization preserves the full query`)
    t.equal(fastURI.equal(uri, truncatedURI), false, `${scheme} equality distinguishes the full query`)
  }

  t.end()
})
