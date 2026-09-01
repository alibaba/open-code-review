'use strict'

const test = require('tape')
const fastURI = require('..')

test('URN parsing validates the complete scheme-specific input', (t) => {
  const embedded = fastURI.parse('urn:x|foo:bar')
  t.equal(embedded.error, 'URN can not be parsed.', 'does not search for a later nid:nss substring')
  t.equal(embedded.nid, undefined, 'does not adopt the later nid')
  t.equal(embedded.nss, undefined, 'does not adopt the later nss')

  const trailing = fastURI.parse('urn:foo:bar|ignored')
  t.equal(trailing.error, 'URN can not be parsed.', 'rejects trailing input outside the RFC 2141 NSS grammar')
  t.equal(trailing.nid, undefined, 'does not return an nid from a partial match')
  t.equal(trailing.nss, undefined, 'does not return a truncated nss')

  t.end()
})

test('URN parsing preserves the complete RFC 2141 NSS', (t) => {
  const parsed = fastURI.parse('urn:foo:allowed/evil')
  t.equal(parsed.error, undefined, 'accepts slash in an RFC 2141 NSS')
  t.equal(parsed.nid, 'foo', 'parses the nid')
  t.equal(parsed.nss, 'allowed/evil', 'preserves the complete NSS')
  t.equal(fastURI.normalize('urn:foo:allowed/evil'), 'urn:foo:allowed/evil', 'normalization retains slash data')
  t.equal(fastURI.equal('urn:foo:allowed/evil', 'urn:foo:allowed'), false, 'distinct NSS values do not compare equal')

  t.end()
})
