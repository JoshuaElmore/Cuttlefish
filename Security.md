  Cuttlefish — Security & Supply-Chain Review

  Scope: fs_api (Go), fs_indexer/fs_aggregator/fs_common (Rust), fs_ui (React), CI (build-rpm.yml), RPM spec + systemd
  units, dependency manifests, config handling.

  Overall: The auth core is better than average — JWT with explicit HMAC algorithm pinning, constant-time credential
  compare, OIDC state/CSRF check, parameterized SQL, startup validation of the session secret, and decent systemd hardening
  on the API service. The serious gaps are transport security (cleartext everywhere) and supply-chain hygiene in CI
  (unpinned actions, unverified toolchain downloads, unsigned packages, no dependency scanning).

  ---
  Medium

  M1 — npm install in Makefile vs npm ci in CI

  Makefile:26 uses npm install (can mutate package-lock.json and resolve newer versions); CI correctly uses npm ci (workflow
  line 96). Use npm ci in the Makefile too for reproducibility. (Lockfiles are present and committed for Go, Cargo, and npm
  — good.)

  M2 — randomHex ignores the RNG error (OIDC CSRF state)

  auth.go:218-222:
  func randomHex(n int) string {
      b := make([]byte, n)
      rand.Read(b)           // error ignored
      return hex.EncodeToString(b)
  }
  If crypto/rand ever fails, state becomes all-zeros and predictable, weakening the OIDC CSRF protection. Check the error
  and fail the request. Also set Secure on the oidc_state cookie (H2).

  M4 — Connection-string injection via unescaped credentials

  Both config.go:49-52 (Go) and lib.rs:27-30 (Rust) build libpq keyword/value strings with fmt.Sprintf!/format! and no
  escaping. A password containing a space (e.g. ... sslmode=disable) would be parsed as additional connection parameters and
  could downgrade TLS or alter the target.
  Fix: escape values (wrap in single quotes, backslash-escape ' and \) or use a proper connection-config builder / URL with
  percent-encoding.

  M6 — No brute-force protection on local login

  handleLocalLogin does constant-time compare (good, no timing leak) but there's no rate limiting or lockout, so the single
  admin password can be guessed online. Add rate limiting / account lockout, or document fronting with fail2ban / a WAF.

  ---
  Low / Hardening

  - L1 — Unbounded limit/offset in listIdentityStats (handlers.go:275-282): strconv.Atoi results are interpolated with no
  clamping. A huge limit enables a cheap memory-DoS; a negative one causes SQL errors. Cap limit (e.g. ≤1000) and floor
  offset at 0. (Note: I verified the fmt.Sprintf query at lines 284-292 is NOT SQL-injectable — idType is internal, col and
  order are allowlisted, limit/offset are ints. Good.)
  - L2 — Swagger UI is unauthenticated (main.go:66, /swagger/ not behind authMiddleware) — discloses the full API schema.
  Consider gating in production.
  - L3 — World-readable config with secrets. The RPM ships fs_config.toml.example as mode 644 (cuttlefish.spec:70) and
  neither the spec nor README tells operators to chmod 600 / chown cuttlefish the real fs_config.toml (which holds DB
  password, session secret, OIDC client secret). Ship the example 640 root:cuttlefish and add a hardening note. (Good:
  fs_config.toml is correctly git-ignored and is not tracked — only the template is; no secret in git history.)
  - L4 — Local working fs_config.toml contains password = "changeme" and a real session secret. Not committed (fine), but
  rotate before any real deployment and never reuse the example admin password.
  - L5 — Documentation=https://github.com/YourOrg/cuttlefish placeholder left in cuttlefish-api.service:3.
  - L6 — Aggregator hardcodes DELETE ... WHERE path = '/proc/kcore' (fs_aggregator/src/main.rs:183-187), implying the
  indexer walks /proc. Storing pseudo-filesystem metadata is low-risk (only metadata, not contents), but consider excluding
  /proc, /sys, /dev at the indexer (related to M3's walker config) rather than patching one entry afterward.
  - L7 — Static file handler (main.go:74-84): low risk — r.URL.Path is pre-cleaned by net/http and http.FileServer/http.Dir
  sanitize traversal; the ServeFile fallback uses a constant path. No action needed, noted for completeness.

  ---
  What's already done well

  - JWT verification pins the signing method to HMAC (auth.go:75), preventing alg=none/RS↔HS confusion.
  - crypto/subtle.ConstantTimeCompare for credentials (auth.go:138-139).
  - OIDC state cookie + state validation on callback; ID token verified against issuer/audience.
  - Session secret validated at startup (non-empty, not the template default, ≥32 chars — config.go:64-72).
  - All data-plane SQL is parameterized; the one dynamic query is allowlisted.
  - Committed lockfiles for all three ecosystems; CGO_ENABLED=0 -buildvcs=false -ldflags="-s -w" build.
  - Debug request logging is build-tag gated (debug.go/nodebug.go), off by default.
  - API systemd unit is sandboxed (ProtectSystem=strict, NoNewPrivileges, PrivateTmp, ReadWritePaths).

  ---
  Suggested priority order

  1. H1 (Rust TLS) + H2 (HTTPS + Secure cookies) — close the cleartext exposure.
  2. H3 — pin actions to SHAs, verify toolchain checksums, pin swag, sign RPMs, add Dependabot + govulncheck/cargo audit/npm
  audit.
  3. M3 (indexer walker flags + sandbox the root oneshot), M4 (conn-string escaping), M2 (RNG error).
  4. M5/M6/L1–L3 hardening and docs.

  Want me to implement any of these? The quickest high-value wins I can do now are: add Secure cookies + RNG error check
  (H2/M2), fix the indexer walker flags (M3), pin the GitHub Actions to SHAs and add dependabot.yml + a scanning workflow
  (H3). I'd tackle the Rust TLS change (H1) separately since it needs a new dependency and connector wiring.

  (Note: an earlier auto-scheduled wake-up may fire shortly from when the tool channel was failing — it's stale; the review
  is complete and I won't reschedule it.)

✻ Cooked for 9m 1s

✻ Claude resuming /loop wakeup (May 30 12:34am)

● This is the stale wake-up I flagged — it was scheduled while the tool channel was dropping outputs. The review is already
  complete: all of those files (auth.go, config.go, handlers.go, middleware.go, fs_common/src/lib.rs, build-rpm.yml,
  cuttlefish.spec, and the four systemd units) were read and analyzed, and the consolidated findings report was delivered in
  my previous turn.

  No further action needed. Quick pointer back to the headline results:

  - High: H1 Rust NoTls (cleartext DB), H2 plaintext HTTP + non-Secure session cookie, H3 CI supply-chain (unpinned actions,
  unverified Go/Node downloads, swag@latest, unsigned RPMs, no Dependabot/scanning).
  - Medium: M1 npm install vs npm ci, M2 ignored crypto/rand error, M3 root indexer with no sandbox + walker silently
  skipping hidden/gitignored files, M4 conn-string injection via unescaped credentials, M5 no authz beyond authn, M6 no
  login rate-limiting.
  - Low: unbounded limit/offset, unauthenticated Swagger, world-readable config mode, placeholder Documentation= URL,
  /proc/kcore handling.
  - Confirmed good: JWT alg pinning, constant-time compare, OIDC state check, parameterized SQL, secret validation,
  committed lockfiles, API systemd sandboxing, and fs_config.toml correctly git-ignored (not in history).

  I'm not rescheduling. If you'd like, I can start implementing fixes — the fastest high-value set is H2 (Secure cookies) +
  M2 (RNG error) + M3 (walker flags) + H3 (pin actions to SHAs, add dependabot.yml and a scan workflow). Just say which.