module github.com/Credda-io/credda-go

go 1.21

// No `require` block, and that is a promise this repository makes about itself:
// standard library only. TestModuleHasNoThirdPartyDependencies fails the build
// if one appears here.

// v0.1.1 through v0.3.0 are a client for Credda's retired reliability-score
// API. Every route they call is gone. `retract` adds no dependency; it stops
// `go get -u` selecting them and makes `go list -m -versions` show the reason.
// It does NOT remove them from the proxy: code already pinned to them keeps
// building. See the Versioning section of README.md.
retract (
	v0.1.1 // Client for the retired reliability-score API; no route it calls exists.
	v0.2.0 // Same.
	v0.3.0 // Same.
)
