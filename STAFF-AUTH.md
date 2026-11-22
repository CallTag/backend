# Staff access

Staff authenticate with a username, a bcrypt password (cost 12), and RFC 6238 TOTP (SHA-1, six digits, 30 seconds, ±one step). Consumed steps cannot be reused, including concurrent logins. Canonical roles are `MOD` and `ADMIN`; `MODERATOR` is accepted as a setup alias. Device JWT roles never grant staff access.

MongoDB stores staff credentials in `staff_accounts`. Android accounts are stored separately in `users`. Usernames are unique, 3–32 characters, start with a letter or digit, and may include letters, digits, dots, underscores, and hyphens. Storage and lookup trim surrounding whitespace and lowercase the username. Password hashes and TOTP secrets are never included in HTTP responses. Restrict database access and encrypt backups: TOTP secrets must remain recoverable for verification.

Sessions use `staff_sessions`: only SHA-256 token hashes are persisted, alongside the CSRF token and an eight-hour expiry. Mongo TTL removes expired records; requests also check the expiry directly. Logout deletes the server record. Account disablement and current role changes take effect on the next request. Cookies are HttpOnly, SameSite=Strict, Secure in production and on non-loopback development hosts. Only local development on localhost/loopback permits an HTTP cookie.

`POST /api/v1/admin/login` accepts `{username,password,totp_code}` (`totp` is an alias). `GET /api/v1/admin/auth/session` restores `{id,username,role,csrf_token}`. `POST /api/v1/admin/logout` requires the returned `X-CSRF-Token`. The `/auth/login` and `/auth/logout` aliases share these handlers. Every mutating staff request needs that CSRF header, and any supplied Origin must match the request host. Admin responses disable caching and do not opt into credentialed cross-origin access.

The login limit is five attempts per transport peer IP per fifteen minutes, including successful attempts. Forwarded headers are ignored; behind a reverse proxy this conservatively limits the proxy peer. Deploy one backend instance for this in-process limiter; configure a trusted edge limiter for larger deployments.

## Initial account

No default staff accounts are created. The explicit operator command connects only to MongoDB and does not initialize application demo seeds:

```sh
cd backend
go run ./cmd/seedstaff
```

Supply `MONGOSTRING` (or `MONGODB_URI`), optional `DB_NAME`, and credentials through secret environment variables `STAFF_SEED_USERNAME`, `STAFF_SEED_PASSWORD`, `STAFF_SEED_ROLE`, `STAFF_SEED_TOTP_SECRET`. Alternatively pipe those four values, one per line, to stdin. Passwords require 8–72 bytes; TOTP secrets are base32 with at least 160 bits. Do not put credentials in command-line arguments or shell history. Interactive terminal input is rejected to avoid echoing secrets.

Use `go run ./cmd/seedstaff -enroll` to deliberately generate a secret and display its enrollment URI once after account creation. This is the only mode that prints an enrollment secret; handle that output as sensitive. With `-enroll`, stdin only needs username, password and role. Existing accounts are never overwritten.

Production requires MongoDB. Explicit `STAFF_ALLOW_MEMORY=true` permits ephemeral development accounts created through `config.CreateStaff`; there are no predefined credentials and the CLI still requires MongoDB. Development memory state disappears on restart.


## Migrating an existing staff account

In a deployment with email-based staff accounts, assign each one a username during the update. From `backend`, with the same MongoDB URI and database as the backend, run:

```sh
printf '%s\n' 'previous-staff@example.com' 'admin-ridwan' | go run ./cmd/seedstaff -rename-from-email
```

The migration can also read `STAFF_LEGACY_EMAIL` and `STAFF_SEED_USERNAME`. It keeps the account ID, audit history, password, role, and TOTP enrollment. The previous email remains in the document to identify its origin. Use the assigned username from then on. Backend startup creates the unique username index and retires the previous `email_1` index.
