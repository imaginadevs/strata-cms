// Package plugins is the extension seam for closed-source / premium plugins.
//
// The OSS core defines the extension points (here); premium plugins live in a
// SEPARATE PRIVATE Go module and register against them from their init() via
// OnRegister. They are compiled in only with the `premium` build tag:
//
//	go build -tags premium ./cmd/server
//
// A gitignored registration file (see doc.go) blank-imports the private module
// under that tag, triggering its init(). This keeps premium code out of the OSS
// repo while staying updatable as a normal versioned module dependency.
package plugins

import "github.com/imaginacolombia/cms/internal/core/email"

// Deps are the core extension points handed to each registered plugin.
// Add fields here (e.g. a workflow registry) as more seams are opened up.
type Deps struct {
	Email *email.Registry
}

var registrations []func(Deps)

// OnRegister is called by premium plugins (from init()) to hook into the app.
func OnRegister(fn func(Deps)) { registrations = append(registrations, fn) }

// ApplyAll runs every registration against the core deps. Called once at startup.
// With no premium build tag this is a no-op.
func ApplyAll(d Deps) {
	for _, fn := range registrations {
		fn(d)
	}
}
