// Fork (page state) -- integrations listmonk/PAGE-STATE-SPEC I3/I4/I5 for the Users page.
// Run: node --test src/usersPage.test.mjs.
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { restoreUsersPage, saveUsersPage } from './usersPage.mjs'; // eslint-disable-line import/extensions

test('PAGE-STATE I3: search, sort and page round-trip; never a selection or the form', () => {
  const saved = saveUsersPage({
    query: 'amy', orderBy: 'last_login', order: 'desc', page: 2, checked: [{ id: 1 }], isFormVisible: true,
  });
  assert.deepEqual(restoreUsersPage({ ...saved, checked: [{ id: 1 }] }), {
    query: 'amy', orderBy: 'last_login', order: 'desc', page: 2,
  });
});

test('PAGE-STATE I4: the page reads no route query -- there is no link to override', () => {
  const vue = readFileSync(new URL('./views/Users.vue', import.meta.url), 'utf8');
  assert.doesNotMatch(vue, /\$route\.query/);
  assert.equal(restoreUsersPage(null), null);
});

test('PAGE-STATE I5: unknown sort fields and bad pages default', () => {
  const r = restoreUsersPage({ orderBy: 'password', order: 'up', query: 3 });
  assert.deepEqual([r.orderBy, r.order, r.query], ['id', 'asc', '']);
  [0, -2, 'x', 1.5].forEach((p) => assert.equal(restoreUsersPage({ page: p }).page, 1, String(p)));
});
