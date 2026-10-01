// Fork (two-factor, integrations PASSKEY-2FA-SPEC I17). Run: yarn test:webauthn
// (node --test src/webauthnCodec.test.mjs). The ONE codec is static/public/static/webauthn.js, the
// file the server-rendered login pages load and the admin SPA loads on demand; Node requires its
// pure conversion functions. No dependency: Node's built-in runner.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const codec = require('../../static/public/static/webauthn.js'); // eslint-disable-line import/extensions

const b64url = (bytes) => Buffer.from(bytes).toString('base64url');
const bytesOf = (buf) => Array.from(new Uint8Array(buf));
const NOPAD = /^[A-Za-z0-9_-]*$/;

test('base64url round-trips every length, url alphabet, no padding', () => {
  for (let n = 0; n <= 70; n += 1) {
    const bytes = Array.from({ length: n }, (_, i) => (i * 37 + n * 11 + 250) % 256);
    const enc = codec.bufToB64url(new Uint8Array(bytes));
    assert.match(enc, NOPAD, `length ${n}: ${enc}`);
    assert.equal(enc, b64url(bytes), `length ${n}: matches Node's base64url`);
    assert.deepEqual(bytesOf(codec.b64urlToBuf(enc)), bytes, `length ${n}: decodes back`);
  }
  // Bytes that need the url alphabet (+ and / in standard base64).
  assert.equal(codec.bufToB64url(new Uint8Array([0xfb, 0xff, 0xbf])), '-_-_');
  // A typed-array view onto a larger buffer encodes only the view.
  const big = new Uint8Array([9, 1, 2, 3, 9]);
  assert.equal(codec.bufToB64url(big.subarray(1, 4)), b64url([1, 2, 3]));
});

// The shape go-webauthn marshals (protocol.CredentialCreation / CredentialAssertion).
const userID = b64url([0x34, 0x32]); // the bytes of "42" -- the decimal user id, never the email
const challenge = b64url(Array.from({ length: 32 }, (_, i) => 255 - i));
const credID = b64url(Array.from({ length: 32 }, (_, i) => (i * 7) % 256));

const creationJSON = {
  publicKey: {
    rp: { name: 'listmonk', id: 'lm.example.test' },
    user: { name: 'boss', displayName: 'Boss', id: userID },
    challenge,
    pubKeyCredParams: [{ type: 'public-key', alg: -7 }, { type: 'public-key', alg: -257 }],
    timeout: 300000,
    excludeCredentials: [{ type: 'public-key', id: credID, transports: ['internal'] }],
    authenticatorSelection: { residentKey: 'preferred', userVerification: 'preferred' },
    attestation: 'none',
  },
};

const requestJSON = {
  publicKey: {
    challenge,
    timeout: 300000,
    rpId: 'lm.example.test',
    allowCredentials: [{ type: 'public-key', id: credID, transports: ['internal', 'hybrid'] }],
    userVerification: 'preferred',
  },
};

test('creation options: binary members decode, everything else is kept, the input is not mutated', () => {
  const before = JSON.stringify(creationJSON);
  const pk = codec.creationOptionsFromJSON(creationJSON);
  assert.equal(JSON.stringify(creationJSON), before);

  assert.ok(pk.challenge instanceof ArrayBuffer);
  assert.ok(pk.user.id instanceof ArrayBuffer);
  assert.ok(pk.excludeCredentials[0].id instanceof ArrayBuffer);
  assert.equal(codec.bufToB64url(pk.challenge), challenge);
  assert.equal(Buffer.from(pk.user.id).toString(), '42');
  assert.equal(codec.bufToB64url(pk.excludeCredentials[0].id), credID);
  assert.deepEqual(pk.excludeCredentials[0].transports, ['internal']);
  assert.deepEqual(pk.rp, creationJSON.publicKey.rp);
  assert.equal(pk.user.name, 'boss');
  assert.equal(pk.attestation, 'none');
  assert.deepEqual(pk.authenticatorSelection, creationJSON.publicKey.authenticatorSelection);

  // Round trip: re-encoding the binary members gives back the JSON options.
  const back = {
    ...pk,
    challenge: codec.bufToB64url(pk.challenge),
    user: { ...pk.user, id: codec.bufToB64url(pk.user.id) },
    excludeCredentials: pk.excludeCredentials.map((d) => ({ ...d, id: codec.bufToB64url(d.id) })),
  };
  assert.deepEqual(back, creationJSON.publicKey);

  // The bare publicKey object is accepted too.
  assert.equal(codec.bufToB64url(codec.creationOptionsFromJSON(creationJSON.publicKey).challenge), challenge);
});

test('request options: binary members decode and round-trip', () => {
  const pk = codec.requestOptionsFromJSON(requestJSON);
  assert.ok(pk.challenge instanceof ArrayBuffer);
  assert.ok(pk.allowCredentials[0].id instanceof ArrayBuffer);
  const back = {
    ...pk,
    challenge: codec.bufToB64url(pk.challenge),
    allowCredentials: pk.allowCredentials.map((d) => ({ ...d, id: codec.bufToB64url(d.id) })),
  };
  assert.deepEqual(back, requestJSON.publicKey);
  // No allowCredentials (never sent by this server, but tolerated).
  const bare = codec.requestOptionsFromJSON({ publicKey: { challenge } });
  assert.equal(bare.allowCredentials, undefined);
});

// A stand-in for the browser's PublicKeyCredential without toJSON (the manual fallback path).
const raw = (n, seed) => new Uint8Array(Array.from({ length: n }, (_, i) => (i * seed + 3) % 256)).buffer;

test('a registration credential encodes to base64url JSON', () => {
  const cred = {
    id: credID,
    rawId: codec.b64urlToBuf(credID),
    type: 'public-key',
    authenticatorAttachment: 'platform',
    response: {
      clientDataJSON: raw(121, 3),
      attestationObject: raw(250, 5),
      getTransports: () => ['internal'],
    },
    getClientExtensionResults: () => ({ credProps: { rk: true } }),
  };
  const out = codec.credentialToJSON(cred);
  assert.deepEqual(out, {
    id: credID,
    rawId: credID,
    type: 'public-key',
    authenticatorAttachment: 'platform',
    response: {
      clientDataJSON: b64url(bytesOf(cred.response.clientDataJSON)),
      attestationObject: b64url(bytesOf(cred.response.attestationObject)),
      transports: ['internal'],
    },
    clientExtensionResults: { credProps: { rk: true } },
  });
  [out.rawId, out.response.clientDataJSON, out.response.attestationObject].forEach((v) => assert.match(v, NOPAD));
  // Decoding the JSON gives back the credential's bytes.
  assert.deepEqual(bytesOf(codec.b64urlToBuf(out.response.attestationObject)), bytesOf(cred.response.attestationObject));
});

test('an assertion credential encodes to base64url JSON (userHandle included)', () => {
  const cred = {
    id: credID,
    rawId: codec.b64urlToBuf(credID),
    type: 'public-key',
    response: {
      clientDataJSON: raw(110, 7),
      authenticatorData: raw(37, 11),
      signature: raw(71, 13),
      userHandle: new Uint8Array([0x34, 0x32]).buffer,
    },
    getClientExtensionResults: () => ({}),
  };
  const out = codec.credentialToJSON(cred);
  assert.equal(out.rawId, credID);
  assert.equal(out.response.userHandle, userID);
  assert.equal(out.response.signature, b64url(bytesOf(cred.response.signature)));
  assert.equal(out.response.authenticatorData, b64url(bytesOf(cred.response.authenticatorData)));
  assert.equal(out.response.attestationObject, undefined);
  assert.deepEqual(out.clientExtensionResults, {});
  assert.equal(out.authenticatorAttachment, undefined);
});

test('Node gets the pure functions and no page wiring', () => {
  ['create', 'get', 'supported', 'b64urlToBuf', 'bufToB64url', 'creationOptionsFromJSON', 'requestOptionsFromJSON', 'credentialToJSON']
    .forEach((fn) => assert.equal(typeof codec[fn], 'function', fn));
  assert.equal(codec.supported(), false);
  assert.equal(global.listmonkWebAuthn, undefined);
});
