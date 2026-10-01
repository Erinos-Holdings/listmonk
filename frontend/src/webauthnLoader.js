// Fork (two-factor, integrations PASSKEY-2FA-SPEC D5). The admin SPA loads the ONE WebAuthn codec
// -- static/public/static/webauthn.js, the file the server-rendered login pages use -- on demand,
// and gets window.listmonkWebAuthn.{create,get}.
let loading = null;

export default function loadWebAuthn() {
  if (window.listmonkWebAuthn) {
    return Promise.resolve(window.listmonkWebAuthn);
  }
  if (!loading) {
    const root = (import.meta.env.VUE_APP_ROOT_URL || '').replace(/\/+$/, '');
    loading = new Promise((resolve, reject) => {
      const s = document.createElement('script');
      s.src = `${root}/public/static/webauthn.js`;
      s.onload = () => resolve(window.listmonkWebAuthn);
      s.onerror = () => {
        loading = null;
        reject(new Error('webauthn.js'));
      };
      document.head.appendChild(s);
    });
  }
  return loading;
}
