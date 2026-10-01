/*
 * Fork (two-factor, integrations PASSKEY-2FA-SPEC D5) -- the ONE WebAuthn codec, shared by the
 * server-rendered challenge and enrolment pages and the admin SPA (which loads this file on
 * demand). It exposes window.listmonkWebAuthn.{create,get}: options JSON in (the server's
 * {publicKey: {...}} as go-webauthn marshals it), credential JSON out (base64url, no padding).
 * The native PublicKeyCredential.parseCreationOptionsFromJSON / parseRequestOptionsFromJSON /
 * toJSON are used where the browser has them, with a manual fallback. The pure conversion
 * functions are exported for Node (frontend/src/webauthnCodec.test.mjs). The page wiring below
 * replaces any inline script: a [data-webauthn="get|create"] block names its begin/finish URLs,
 * token and next in data attributes.
 */
(function (root) {
  'use strict';

  // base64url (no padding) -> ArrayBuffer.
  function b64urlToBuf(s) {
    var b64 = String(s).replace(/-/g, '+').replace(/_/g, '/');
    while (b64.length % 4) {
      b64 += '=';
    }
    var bin = root.atob(b64);
    var out = new Uint8Array(bin.length);
    for (var i = 0; i < bin.length; i += 1) {
      out[i] = bin.charCodeAt(i);
    }
    return out.buffer;
  }

  // ArrayBuffer / typed array -> base64url (no padding).
  function bufToB64url(buf) {
    var bytes = buf instanceof ArrayBuffer ? new Uint8Array(buf) : new Uint8Array(buf.buffer, buf.byteOffset, buf.byteLength);
    var bin = '';
    for (var i = 0; i < bytes.length; i += 1) {
      bin += String.fromCharCode(bytes[i]);
    }
    return root.btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  }

  function publicKeyOf(json) {
    return json && json.publicKey ? json.publicKey : json;
  }

  function descriptors(list) {
    return (list || []).map(function (d) {
      var out = { type: d.type, id: b64urlToBuf(d.id) };
      if (d.transports) {
        out.transports = d.transports;
      }
      return out;
    });
  }

  // Manual PublicKeyCredentialCreationOptionsJSON -> PublicKeyCredentialCreationOptions.
  function creationOptionsFromJSON(json) {
    var pk = Object.assign({}, publicKeyOf(json));
    pk.challenge = b64urlToBuf(pk.challenge);
    pk.user = Object.assign({}, pk.user, { id: b64urlToBuf(pk.user.id) });
    if (pk.excludeCredentials) {
      pk.excludeCredentials = descriptors(pk.excludeCredentials);
    }
    return pk;
  }

  // Manual PublicKeyCredentialRequestOptionsJSON -> PublicKeyCredentialRequestOptions.
  function requestOptionsFromJSON(json) {
    var pk = Object.assign({}, publicKeyOf(json));
    pk.challenge = b64urlToBuf(pk.challenge);
    if (pk.allowCredentials) {
      pk.allowCredentials = descriptors(pk.allowCredentials);
    }
    return pk;
  }

  // Manual PublicKeyCredential -> JSON (registration or assertion response).
  function credentialToJSON(cred) {
    var r = cred.response;
    var resp = { clientDataJSON: bufToB64url(r.clientDataJSON) };
    if (r.attestationObject) {
      resp.attestationObject = bufToB64url(r.attestationObject);
      if (typeof r.getTransports === 'function') {
        resp.transports = r.getTransports();
      }
    }
    if (r.authenticatorData) {
      resp.authenticatorData = bufToB64url(r.authenticatorData);
    }
    if (r.signature) {
      resp.signature = bufToB64url(r.signature);
    }
    if (r.userHandle) {
      resp.userHandle = bufToB64url(r.userHandle);
    }
    var out = {
      id: cred.id,
      rawId: bufToB64url(cred.rawId),
      type: cred.type,
      response: resp,
      clientExtensionResults: typeof cred.getClientExtensionResults === 'function' ? cred.getClientExtensionResults() : {},
    };
    if (cred.authenticatorAttachment) {
      out.authenticatorAttachment = cred.authenticatorAttachment;
    }
    return out;
  }

  function nativePKC() {
    return root.PublicKeyCredential;
  }

  function supported() {
    return !!(nativePKC() && root.navigator && root.navigator.credentials);
  }

  function serialize(cred) {
    if (cred && typeof cred.toJSON === 'function') {
      try {
        return cred.toJSON();
      } catch (e) {
        // Fall through to the manual codec.
      }
    }
    return credentialToJSON(cred);
  }

  // create(optionsJSON) -> credential JSON (registration).
  function create(optionsJSON) {
    var PKC = nativePKC();
    var publicKey = PKC && typeof PKC.parseCreationOptionsFromJSON === 'function'
      ? PKC.parseCreationOptionsFromJSON(publicKeyOf(optionsJSON))
      : creationOptionsFromJSON(optionsJSON);
    return root.navigator.credentials.create({ publicKey: publicKey }).then(serialize);
  }

  // get(optionsJSON) -> credential JSON (assertion).
  function get(optionsJSON) {
    var PKC = nativePKC();
    var publicKey = PKC && typeof PKC.parseRequestOptionsFromJSON === 'function'
      ? PKC.parseRequestOptionsFromJSON(publicKeyOf(optionsJSON))
      : requestOptionsFromJSON(optionsJSON);
    return root.navigator.credentials.get({ publicKey: publicKey }).then(serialize);
  }

  var api = {
    create: create,
    get: get,
    supported: supported,
    b64urlToBuf: b64urlToBuf,
    bufToB64url: bufToB64url,
    creationOptionsFromJSON: creationOptionsFromJSON,
    requestOptionsFromJSON: requestOptionsFromJSON,
    credentialToJSON: credentialToJSON,
  };

  if (typeof module === 'object' && module.exports) {
    module.exports = api;
  }
  if (typeof root.document === 'undefined') {
    return;
  }
  root.listmonkWebAuthn = api;

  // Page wiring for the server-rendered challenge and enrolment pages.
  function post(url, body) {
    return root.fetch(url, {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (r.status === 401) {
          // A missing, expired or used-up token: back to the login page.
          root.location.href = '/admin/login';
        }
        if (!r.ok) {
          throw new Error(j.message || r.statusText);
        }
        return j.data;
      });
    });
  }

  function wire(el) {
    var btn = el.querySelector('[data-webauthn-start]');
    var errEl = el.querySelector('[data-webauthn-error]');
    var nameEl = el.querySelector('[data-webauthn-name]');
    var isCreate = el.getAttribute('data-webauthn') === 'create';
    if (!btn) {
      return;
    }

    function fail(msg) {
      btn.disabled = false;
      if (errEl) {
        errEl.textContent = msg;
        errEl.hidden = false;
      }
    }

    btn.addEventListener('click', function () {
      if (errEl) {
        errEl.hidden = true;
      }
      if (!supported()) {
        fail(el.getAttribute('data-unsupported') || 'Passkeys are not supported.');
        return;
      }
      var body = { token: el.getAttribute('data-token'), next: el.getAttribute('data-next') };
      if (isCreate) {
        body.name = nameEl ? nameEl.value.trim() : '';
        if (!body.name) {
          if (nameEl) {
            nameEl.focus();
          }
          return;
        }
      }
      btn.disabled = true;
      post(el.getAttribute('data-begin'), body)
        .then(function (opts) { return isCreate ? create(opts) : get(opts); })
        .then(function (cred) {
          body.credential = cred;
          return post(el.getAttribute('data-finish'), body);
        })
        .then(function (out) { root.location.href = out.redirect; })
        .catch(function (e) { fail(e && e.message ? e.message : String(e)); });
    });
  }

  function init() {
    var els = root.document.querySelectorAll('[data-webauthn]');
    for (var i = 0; i < els.length; i += 1) {
      wire(els[i]);
    }
  }

  if (root.document.readyState === 'loading') {
    root.document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
}(typeof window !== 'undefined' ? window : globalThis));
