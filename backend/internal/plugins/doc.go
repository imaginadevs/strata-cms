package plugins

// Shipping premium / closed-source plugins without touching the OSS repo
// ---------------------------------------------------------------------------
//
// WHEN does any private code get pulled?  BUILD TIME ONLY — on your build machine
// or CI, when you compile the premium edition. Never during a plain OSS install,
// and never at runtime (end users run a finished binary). So:
//
//   - `go build ./cmd/server`                → OSS edition. No premium files, no
//                                              private dependency, no auth. Can't fail.
//   - `go build -tags premium ./cmd/server`  → premium edition. Needs the private
//                                              plugin code checked out (below).
//
// WHERE does premium code live?  In a SEPARATE PRIVATE repo, checked out into
// `backend/internal/premium/` as a git submodule. It must live *inside* this Go
// module (not as a standalone module) because Go forbids importing another
// module's internal/ packages — and plugins need internal/core/email, etc.
// `backend/internal/premium/` and `backend/cmd/server/premium.go` are gitignored,
// so the OSS repo never contains them.
//
// SETUP (once):
//
//   # in the OSS checkout, add your private repo as a submodule at the ignored path
//   git submodule add git@github.com:imaginacolombia/cms-premium.git backend/internal/premium
//
// BUILD AUTH: only the submodule checkout needs credentials, and only on the
// builder. Use SSH (a deploy key in CI) or a PAT:
//
//   git config --global url."git@github.com:".insteadOf "https://github.com/"
//   git submodule update --init                 # fetch/refresh premium code
//   go build -tags premium ./cmd/server         # compile the premium binary
//
// UPDATES: `git submodule update --remote` pulls the latest premium code; pin a
// tag/commit for reproducible CI.
//
// HOW REGISTRATION WORKS: a premium plugin registers from init():
//
//   func init() {
//       plugins.OnRegister(func(d plugins.Deps) {
//           d.Email.Register("premium.example", email.Template{ /* ... */ })
//       })
//   }
//
// `cmd/server/premium.go` (premium tag) blank-imports internal/premium so those
// init()s run, and the server calls plugins.ApplyAll(Deps{...}) once at startup
// (see router.go). Extend Deps as new seams are exposed (workflow registry, etc.).
