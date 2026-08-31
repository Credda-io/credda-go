package credda

import (
	"bytes"
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
//     this repository was last given -- both of them, since 2026-08-30, the
//     route digest and the vocabularyDigest -- and core's own suite fails --
//     in CI, naming credda-go -- when the engine's surface moves past either.
//     Refreshing is: copy the file in, update the ledger there.
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
	Generator        string         `json:"generator"`
	Digest           string         `json:"digest"`
	VocabularyDigest string         `json:"vocabularyDigest"`
	RouteCount       int            `json:"routeCount"`
	Routes           []surfaceRoute `json:"routes"`
	// Vocabularies is kept raw. The digest below is taken over the generator's
	// own serialisation, and a Go map does not preserve the key order that
	// serialisation has; decoding into one and re-encoding would compute a
	// digest over a different document.
	Vocabularies json.RawMessage `json:"vocabularies"`
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

// ── the query vocabularies ──────────────────────────────────────────────────
//
// WHY THIS EXISTS. The route digest covers the routes and nothing else, so the
// ten closed sets in types.go were held to the engine by nobody. That is how
// eight members drifted with this suite green on 2026-08-30: InvestigationStates
// withheld all seven patch-path states after ADR 0019 put them back on the
// investigation path, so ?state=READY_FOR_REVIEW -- a 200 on the engine -- was
// a value this package told its reader did not exist, and InvestigationOutcomes
// was missing three more. The copy of route-surface.json this repository held
// carried no `vocabularies` block at all until the same day, so there was
// nothing to check against either.
//
// The check below is a COMPARISON against the artifact, not a second list. A
// test that restated the members in Go source would drift in exactly the way
// types.go did, one level down.

// filterVocabularies maps a declared query filter to the exported slice that
// publishes its closed set. This is the hand-written half and the only half
// that can be: which Go identifier answers which filter. The MEMBERS are never
// written here.
var filterVocabularies = map[string]map[string]*[]string{
	"GET /api/investigations": {
		"state":   &InvestigationStates,
		"outcome": &InvestigationOutcomes,
	},
	"GET /api/investigations/{id}/evidence": {"type": &EvidenceTypes},
	"GET /api/repositories/{id}/learnings":  {"kind": &LearningKinds},
	"GET /api/resolutions":                  {"confidence": &ResolutionConfidenceClasses},
	"GET /api/validations": {
		"state":   &ValidationStates,
		"outcome": &ValidationOutcomes,
	},
	"GET /api/validations/{id}/checks":   {"status": &CheckStatuses},
	"GET /api/validations/{id}/evidence": {"type": &EvidenceTypes},
	"GET /api/validations/{id}/findings": {
		"severity": &FindingSeverities,
		"status":   &FindingStatuses,
	},
}

// untypedFilters are the declared filters this package deliberately publishes
// no slice for, with the reason. A filter that is neither mapped above nor
// named here fails, so the engine gaining one is noticed rather than absorbed.
var untypedFilters = map[string]string{
	// NOT CARRIED BY THIS PACKAGE AT ALL, which is a different fact from the
	// one this entry used to state. InvestigationQuery and ResolutionQuery have
	// Signal -- WHICH signal raised the run -- and no field for hasSignal,
	// which asks WHETHER one did; api.go says so at both structs. There is no
	// slice to publish because the filter is absent, not untyped.
	// @credda/mcp-server does take it as a boolean on both routes. Recorded
	// rather than built: adding the field is a capability decision.
	"hasSignal": "not reachable from this package; InvestigationQuery and ResolutionQuery carry no such field, as api.go states at both",
	// This one IS carried: EventQuery.IncludeDebug is a *bool and values()
	// serialises it. The artifact spells the four accepted encodings and Go
	// has one, so no slice is published for it.
	"includeDebug": "a boolean, sent as EventQuery.IncludeDebug; the artifact spells its four accepted encodings and Go has one",
}

func surfaceVocabularies(t *testing.T, s routeSurface) map[string]map[string][]string {
	t.Helper()
	if len(s.Vocabularies) == 0 {
		t.Fatal("route-surface.json carries no vocabularies block; re-copy it from core")
	}
	var v map[string]map[string][]string
	if err := json.Unmarshal(s.Vocabularies, &v); err != nil {
		t.Fatalf("the vocabularies block does not parse: %v", err)
	}
	return v
}

// TestVocabularyBlockWasNotEditedByHand is the integrity guard for the second
// half of the artifact, mirroring TestRouteSurfaceWasNotEditedByHand. The
// generator digests JSON.stringify of the block, which is the compaction of the
// bytes on disk, so the raw bytes are compacted rather than round-tripped.
func TestVocabularyBlockWasNotEditedByHand(t *testing.T) {
	s := loadSurface(t)
	if len(s.Vocabularies) == 0 {
		t.Fatal("route-surface.json carries no vocabularies block; re-copy it from core")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, s.Vocabularies); err != nil {
		t.Fatalf("compacting the vocabularies block: %v", err)
	}
	sum := sha256.Sum256(compact.Bytes())
	got := "sha256-" + hex.EncodeToString(sum[:])
	if got != s.VocabularyDigest {
		t.Errorf("vocabularyDigest = %s, want %s\nroute-surface.json was modified after generation; re-copy it from core", got, s.VocabularyDigest)
	}
}

// TestEveryDeclaredFilterIsAccountedFor closes both directions: a filter the
// engine gains with no slice fails by name, and a mapping kept for a filter the
// engine dropped fails too.
func TestEveryDeclaredFilterIsAccountedFor(t *testing.T) {
	s := loadSurface(t)
	vocabularies := surfaceVocabularies(t, s)

	known := map[string]bool{}
	for _, key := range surfaceKeys(s) {
		known[key] = true
	}

	declared := 0
	for key, params := range vocabularies {
		if !known[key] {
			t.Errorf("%s carries a vocabulary and is not a route the engine serves", key)
		}
		for param := range params {
			declared++
			if _, ok := filterVocabularies[key][param]; ok {
				continue
			}
			if _, ok := untypedFilters[param]; ok {
				continue
			}
			t.Errorf("the engine declares a vocabulary for %s ?%s and this package neither publishes it nor says why not; add the slice to types.go and map it, or add untypedFilters entry with a reason", key, param)
		}
	}
	if declared <= 10 {
		t.Errorf("read %d filters out of the artifact; too few to have checked anything", declared)
	}

	for key, params := range filterVocabularies {
		for param := range params {
			if _, ok := vocabularies[key][param]; !ok {
				t.Errorf("this package publishes a vocabulary for %s ?%s and the engine no longer declares it", key, param)
			}
		}
	}
}

// TestPublishedVocabulariesAreTheEngines is the assertion the ten slices were
// missing. DeepEqual on the whole slice, not membership: a set that has LOST a
// value the engine can return fails exactly as loudly as one that has gained a
// value the engine cannot, and the order is the engine's own.
func TestPublishedVocabulariesAreTheEngines(t *testing.T) {
	s := loadSurface(t)
	vocabularies := surfaceVocabularies(t, s)

	checked := 0
	for key, params := range filterVocabularies {
		for param, published := range params {
			want, ok := vocabularies[key][param]
			if !ok {
				continue // reported by TestEveryDeclaredFilterIsAccountedFor
			}
			if !reflect.DeepEqual(*published, want) {
				t.Errorf("%s ?%s: this package publishes\n  %v\nand the engine declares\n  %v", key, param, *published, want)
			}
			checked++
		}
	}
	if checked != 11 {
		t.Errorf("compared %d vocabularies, want 11; a mapping was dropped and this test went quiet", checked)
	}
}
