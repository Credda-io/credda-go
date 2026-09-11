module github.com/Credda-io/credda-go

go 1.21

// No `require` block, and that is a promise this repository makes about itself:
// standard library only. TestModuleHasNoThirdPartyDependencies fails the build
// if one appears here.

// v0.1.0 through v0.3.0 are a client for Credda's retired reliability-score
// API. Every route they call is gone. `retract` adds no dependency; it stops
// `go get -u` selecting them and makes `go list -m -versions` show the reason.
// It does NOT remove them from the proxy: code already pinned to them keeps
// building. See the Versioning section of README.md.
//
// v0.1.0 is here because the proxy published it (proxy.golang.org lists it
// alongside v0.1.1-v0.3.0) even though its git tag no longer exists; a retract
// that skipped it would leave one retired version selectable by `go get`.
retract (
	v0.1.0 // Client for the retired reliability-score API; no route it calls exists.
	v0.1.1 // Same.
	v0.2.0 // Same.
	v0.3.0 // Same.
)
