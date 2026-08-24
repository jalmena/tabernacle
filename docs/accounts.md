# Accounts

Reading a collection needs no account. Changing one does, as soon as the instance
has been claimed.

## Claiming an instance

A fresh instance has no accounts and says so on every page. In that state it
behaves exactly as it did before accounts existed: anybody who can reach the port
can add and edit songs. That is deliberate — it is how a collection that predates
this feature keeps working — but it is not a state to stay in.

Two ways to close it:

- Press **Acceder**, then **Ajustes**, and fill in the claim form. The account you
  create is an administrator and you are signed in as it.
- Or set `TABERNACLE_ADMIN_USER` and `TABERNACLE_ADMIN_PASSWORD` before the first
  start. Both are needed; one without the other is a configuration error.

There is no open registration. An administrator creates everyone else.

## What an account carries

A name, a password, a role, and two preferences that belong to the person rather
than to the browser they happen to be using: the interface language and the
theme. Both follow them from the phone to the laptop, which is why they are in the
accounts file and not in a cookie.

## Signing up

Closed by default: an administrator creates the accounts. **Usuarios → Registro**
opens it, and then the sign-in box offers a way to create one. Accounts made that
way are always members, never administrators — opening the door lets people add
songs, not decide who else may.

## Roles

| | Read | Add and edit songs | Manage accounts and settings |
| --- | --- | --- | --- |
| anybody | ✓ | | |
| `user` | ✓ | ✓ | |
| `admin` | ✓ | ✓ | ✓ |

Read-only mode (`TABERNACLE_READ_ONLY`) governs the **collection**, not the
application. Signing in, editing a profile, choosing a theme and managing accounts
all work in it; songs and imports do not.

## The file

`accounts.json` in the data directory, mode 600. It holds the accounts and the key
that signs sessions.

```json
{ "version": 1, "secret": "…", "users": [
  { "id": "3f9c1a7e5b2d4801", "name": "jose", "role": "admin",
    "hash": "pbkdf2-sha256$600000$…$…", "theme": "", "epoch": 1,
    "created_at": "2026-08-21T10:00:00Z", "last_login_at": "2026-08-21T10:00:00Z" } ] }
```

It is **the one file in the data directory that cannot be regenerated**. The index
is derived from the songs and rebuilds itself; this is not. Back it up.

- `id` never changes. The session cookie carries it, so renaming an account does
  not sign anybody out.
- `name` is the login identity: 2 to 32 characters from `A–Z a–z 0–9 . _ -`,
  unique regardless of case. Accents are deliberately not folded, which keeps
  `José` and `Jose` two different names rather than one confusable one.
- `epoch` is the session generation. It goes up on a password change, on "log out
  everywhere", and on an administrator's reset — which is how any of those reach
  a device you no longer have.
- A file that cannot be understood is a startup failure. It is never read as an
  empty one, because that would silently reopen the instance to anybody.

Editing it by hand is supported. That is what makes the recovery below possible.

## Passwords

PBKDF2-HMAC-SHA256 from the standard library, 600,000 iterations, a 16-byte salt
per password, stored as `pbkdf2-sha256$iterations$salt$key`. The parameters travel
with the hash, so raising the count later is one number and no migration: the next
successful login re-hashes that password in place.

Minimum eight characters and no composition rules. Length is the property that
matters; the rest mostly teaches people to write `Password1!`. Eight is a floor
rather than a recommendation: guessing costs a PBKDF2 derivation at 600,000
iterations and the login handler rations attempts, so the arithmetic that makes
a short password dangerous on a public site does not hold here.

`account.MinPasswordRunes` is the only place the number lives. The sentence the
reader sees takes it as an argument rather than spelling it out in nineteen
languages, where the number and the rule would eventually disagree.

A wrong password and a name that does not exist give the same answer and cost the
same work, so response times do not report which names are real.

## Sessions

A cookie holding a signed token — the account id, the epoch and an expiry — and no
server-side table.

Why no table: the index runs on a single SQL connection that scans already
contend for, so a write per login would compete with them. Without a table there
is no migration, no expiry sweeper, and no session fixation to worry about,
because there is no pre-login identifier to fix. A session also survives deleting
the index, which is a supported thing to do.

What it costs: no list of your devices. The epoch buys back what matters — a
password change or "log out everywhere" invalidates every session at once.

The cookie is `HttpOnly`, `SameSite=Lax`, host-only, and lasts thirty days,
renewing itself once it is more than half spent. It is marked `Secure` only when
`TABERNACLE_BASE_URL` is `https` or the request arrived over TLS: the ordinary
deployment is plain HTTP on a home network, and a `Secure` cookie would never be
sent, producing a login form that appears to do nothing. `X-Forwarded-Proto` is
not consulted, because nothing else here trusts a header a hop can invent.

## What this does and does not defend against

A write has to prove it came from one of our own pages. The primary evidence is
`Sec-Fetch-Site`, which the browser sets and page content cannot: it is a
forbidden header name, so neither a form nor `fetch` can forge it. `same-origin`
is accepted, anything cross-site is refused.

`Origin` corroborates rather than decides, because a browser will not always name
one. Firefox reduces it to `null` on a POST whenever the referrer policy is
restrictive, and every response here carries `Referrer-Policy: no-referrer` — so
`Origin: null` is what an ordinary form submission looks like, not an oddity. When
a browser does name an origin it has to match this exact host **and port**, or a
name in `TABERNACLE_ALLOWED_HOSTS` or `TABERNACLE_BASE_URL`; the port matters
because a browser scopes neither an origin nor a cookie by hostname alone.

For a client that sends neither header — something that is not a browser — the
Host allowlist is the whole of it, which is what makes that path safe: a page on a
public name cannot make a browser claim to be talking to a private one.

`SameSite=Lax` keeps the cookie off a cross-site POST as well. Between them there
is no useful gap for a synchroniser token to close, so there is not one.

What remains:

- A top-level cross-site **GET** navigation does carry the cookie. That is safe
  here only because no `GET` route changes anything, which is a rule with a test.
- Anything able to run script on this exact origin defeats all of it. The
  content security policy allows no inline script anywhere, and a test scans every
  template for one; that is the defence, not this.
- On plain HTTP, a network attacker can read the cookie. The threat model is a
  trusted network: put a VPN or TLS in front of it, as
  [deployment](deployment.md) says.

Login attempts are rate limited, ten per name and per address with one back per
minute, and password hashing is bounded to two at a time. That is less about
guessing than about arithmetic: 600,000 iterations reachable by an anonymous POST
is a lever on a small machine.

## Locked out

Three ways, in order of convenience:

1. Set `TABERNACLE_ADMIN_USER` and `TABERNACLE_ADMIN_PASSWORD` and restart. If
   that account exists, its password is reset and its role restored to
   administrator. This needs shell access to the host, which is the right bar.
2. Edit `accounts.json` by hand.
3. Delete `accounts.json`. The instance returns to unclaimed and can be claimed
   again. No song is touched — that file holds only accounts.

There is always at least one administrator, and an administrator can neither
delete nor demote their own account, so no single mis-click can lock you out.

## Not here, on purpose

No email, and therefore no reset-by-email — a field whose
only effect would be to promise a recovery path that cannot exist. No avatars. No
OIDC or LDAP. No per-song permissions. No audit log. No password expiry.
