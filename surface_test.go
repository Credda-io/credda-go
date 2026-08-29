package credda

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"testing"
)

// The route list this client is held to, taken from the engine rather than
// transcribed.
//
// ---------------------------------------------------------------------------
// WHERE IT COMES FROM
// ---------------------------------------------------------------------------
// route-surface.json is generated in core by scripts/generate-route-surface.ts
// out of apps/api/src/openapi.ts and copied here. IT IS NOT EDITED BY HAND.
//
// parity_test.go named the gap this file closes, and named it correctly: a
// hand-written table lists what this client CALLS, so a route the ENGINE gains
// fails nothing. POST /api/investigations/{id}/cancel shipped in core on
// 2026-08-29 and left credda-js green at 102 tests with no cancelInvestigation;
// this package's table would have been just as quiet. The routes now arrive
// from the engine and only the mapping below -- which Go method serves which
// route -- is ours.
//
// core is private, is not a Go module, and its route table imports
// @credda/shared, @credda/db and @credda/memory, so importing it, reading a
// sibling checkout (passes on a laptop, absent in CI) and fetching a live
// /openapi.json (a network call in a unit suite) were all unavailable. A copy
// with a digest and a ledger is what there is.
//
// Two staleness guards, because a copy that quietly rots is the same defect in
// a new hat:
//
//   - Integrity, here: the digest stamped in the file is recomputed over its
//     own routes below, so a copy hand-edited to make this suite pass fails
//     instead.
//   - Propagation, in core: route-surface.consumers.json records the digest
//     this repository was last given, and core's own suite fails -- in CI,
//     naming credda-go -- when the engine's surface moves past it. Refreshing
//     is: copy the file in, update the ledger there.
//
//go:embed route-surface.json
var routeSurfaceJSON []byte

// surfaceRoute mirrors the generator's SurfaceRoute, in its field order,
// because the digest is taken over the routes as the generator serialised them
// and Go's encoding/json emits struct fields in declaration order.
type surfaceRoute struct {
	Method   string `json:"method"`
	Path     string `json:"path"`
	Statuses []int  `json:"statuses"`
}

type routeSurface struct {
	Generator  string         `json:"generator"`
	Digest     string         `json:"digest"`
	RouteCount int            `json:"routeCount"`
	Routes     []surfaceRoute `json:"routes"`
}

func loadSurface(t *testing.T) routeSurface {
	t.Helper()
	var s routeSurface
	if err := json.Unmarshal(routeSurfaceJSON, &s); err != nil {
		t.Fatalf("route-surface.json does not parse: %v", err)
	}
	return s
}

// clientMethods maps "METHOD /path" from the engine's surface to the method on
// *Client that serves it, or "" for a route this package deliberately does not
// wrap. This is the one hand-written half, and the only half that can be.
var clientMethods = map[string]string{
	"GET /livez": "Livez",

	// The one route with no method, and on purpose. It serves the document
	// that DESCRIBES this API; what a Go caller wants out of that document is
	// the types, and types.go already ships them. A method returning it could
	// only return map[string]any, which is a worse answer than types.go gives.
	"GET /openapi.json": "",

	"GET /api/health":  "GetHealth",
	"GET /api/metrics": "Metrics",

	"GET /api/investigations":               "ListInvestigations",
	"POST /api/investigations":              "CreateInvestigation",
	"GET /api/investigations/{id}":          "GetInvestigation",
	"POST /api/investigations/{id}/cancel":  "CancelInvestigation",
	"GET /api/investigations/{id}/events":   "InvestigationEvents",
	"GET /api/investigations/{id}/evidence": "InvestigationEvidence",
	"GET /api/investigations/{id}/stream":   "StreamInvestigation",

	"GET /api/repositories":                "ListRepositories",
	"GET /api/repositories/{id}":           "GetRepository",
	"GET /api/repositories/{id}/learnings": "RepositoryLearnings",

	"GET /api/resolutions":        "ListResolutions",
	"GET /api/resolutions/latest": "LatestResolution",
	"GET /api/resolutions/{id}":   "GetResolution",

	"GET /api/validations":               "ListValidations",
	"GET /api/validations/{id}":          "GetValidation",
	"GET /api/validations/{id}/checks":   "ValidationChecks",
	"GET /api/validations/{id}/findings": "ValidationFindings",
	"GET /api/validations/{id}/evidence": "ValidationEvidence",
	"GET /api/validations/{id}/events":   "ValidationEvents",
	"GET /api/validations/{id}/stream":   "StreamValidation",

	"GET /api/organization":         "GetOrganization",
	"GET /api/organization/members": "OrganizationMembers",
	"GET /api/organization/keys":    "OrganizationKeys",
}

// The routes this package deliberately does not wrap. Listing them here rather
// than counting them means removing a wrapper is a deliberate edit in two
// places, not a quiet subtraction in one.
var deliberatelyUnwrapped = []string{"GET /openapi.json"}

func surfaceKeys(s routeSurface) []string {
	keys := make([]string, 0, len(s.Routes))
	for _, r := range s.Routes {
		keys = append(keys, fmt.Sprintf("%s %s", r.Method, r.Path))
	}
	return keys
}

// TestRouteSurfaceWasNotEditedByHand recomputes the digest the generator
// stamped. A copy edited to make the test below pass fails here first.
func TestRouteSurfaceWasNotEditedByHand(t *testing.T) {
	s := loadSurface(t)
	encoded, err := json.Marshal(s.Routes)
	if err != nil {
		t.Fatalf("re-encoding the routes: %v", err)
	}
	sum := sha256.Sum256(encoded)
	got := "sha256-" + hex.EncodeToString(sum[:])
	if got != s.Digest {
		t.Errorf("digest = %s, want %s\nroute-surface.json was modified after generation; re-copy it from core", got, s.Digest)
	}
	if s.RouteCount != len(s.Routes) {
		t.Errorf("routeCount = %d, len(routes) = %d", s.RouteCount, len(s.Routes))
	}
	if s.Generator != "scripts/generate-route-surface.ts" {
		t.Errorf("generator = %q, want scripts/generate-route-surface.ts", s.Generator)
	}
}

// TestEveryEngineRouteHasAClientMethod is the assertion parity_test.go could
// not make: it fails when the ENGINE gains a route, not only when this client
// changes one.
func TestEveryEngineRouteHasAClientMethod(t *testing.T) {
	s := loadSurface(t)
	keys := surfaceKeys(s)

	for _, key := range keys {
		if _, ok := clientMethods[key]; !ok {
			t.Errorf("the engine serves %s and this client has no method for it, and no reason not to", key)
		}
	}

	known := map[string]bool{}
	for _, key := range keys {
		known[key] = true
	}
	for key := range clientMethods {
		if !known[key] {
			t.Errorf("%s is mapped here and the engine no longer serves it", key)
		}
	}

	var unwrapped []string
	for _, key := range keys {
		if clientMethods[key] == "" {
			unwrapped = append(unwrapped, key)
		}
	}
	sort.Strings(unwrapped)
	want := append([]string(nil), deliberatelyUnwrapped...)
	sort.Strings(want)
	if !reflect.DeepEqual(unwrapped, want) {
		t.Errorf("deliberately unwrapped = %v, want %v; an unwrapped route needs a stated reason", unwrapped, want)
	}
}

// TestEveryMappedMethodExists closes the other direction: a mapping is a claim
// about this package, and a renamed or deleted method must fail by name.
func TestEveryMappedMethodExists(t *testing.T) {
	s := loadSurface(t)
	typ := reflect.TypeOf(&Client{})

	seen := map[string]string{}
	for _, key := range surfaceKeys(s) {
		name := clientMethods[key]
		if name == "" {
			continue
		}
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("%s is mapped to (*Client).%s, which this package does not have", key, name)
		}
		if prev, dup := seen[name]; dup {
			t.Errorf("(*Client).%s is mapped to both %s and %s; one method may not serve two routes", name, prev, key)
		}
		seen[name] = key
	}
}
