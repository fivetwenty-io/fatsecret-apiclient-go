package foods

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Drift guard between FatSecret's real wire payloads and the structs this
// package generates for them, running where the structs are made rather than
// in a downstream consumer.
//
// The 2026-08-14 photo outage: food_sub_categories was modelled — as the
// wrong type — and Go's decoder answers a type error by abandoning the whole
// enclosing value, so a single mistyped field discarded every detection in
// the reply. The consumer's copy of this guard would have caught it, but only
// after a version bump; this copy fails the library's own CI on the commit
// that introduces the drift.
//
// Two properties are asserted over captured payloads in testdata/captures/:
//
//   - every key FatSecret sends, at every depth, is one the structs model, and
//   - every captured food object decodes through the production path without
//     a type error.
//
// The key check is RECURSIVE on purpose. DisallowUnknownFields does not
// propagate through a custom UnmarshalJSON — Food has one for its nested
// wrappers — so decoder-side strictness silently stops at the first type that
// implements json.Unmarshaler and can never inspect a Serving. Walking the
// raw JSON against the struct types by reflection is the only check here that
// actually reaches every level.
//
// Captures come from the live probes in the peppi consumer (run on
// infrastructure holding the credentials); they are public catalogue foods.
// Add one whenever a payload class appears that no existing capture covers.

const captureDir = "testdata/captures"

func loadCaptures(t *testing.T) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(captureDir, "*.json"))
	if err != nil {
		t.Fatalf("glob captures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatalf("no captures in %s — the drift guard cannot fail, so it proves nothing. "+
			"Restore the captures or delete this suite deliberately, but do not leave it green and empty.",
			captureDir)
	}
	out := make(map[string][]byte, len(paths))
	for _, p := range paths {
		body, rerr := os.ReadFile(p) //nolint:gosec // test fixture, path from a literal glob
		if rerr != nil {
			t.Fatalf("read %s: %v", p, rerr)
		}
		out[filepath.Base(p)] = body
	}
	return out
}

// capturedFoodObjects pulls every catalogue food object out of a capture,
// whatever envelope it arrived in. It walks the generic decode rather than
// the typed one on purpose: the typed decode is what is under test, so using
// it to find the objects would hide exactly the keys being looked for.
func capturedFoodObjects(t *testing.T, capture []byte) []json.RawMessage {
	t.Helper()

	var generic map[string]json.RawMessage
	if err := json.Unmarshal(capture, &generic); err != nil {
		t.Fatalf("capture is not a JSON object: %v", err)
	}

	// food.get shape: {"food": {...}}
	if food, ok := generic["food"]; ok {
		return []json.RawMessage{food}
	}

	// foods.search v5 shape: {"foods_search":{"results":{"food":[...]}}}
	if rawSearch, ok := generic["foods_search"]; ok {
		return searchFoodObjects(t, rawSearch)
	}

	// image-recognition shape: {"food_response":[{..., "food":{...}}]} — the
	// envelope belongs to the consumer, but each detection nests a catalogue
	// food that decodes into this package's Food.
	rawResponse, ok := generic["food_response"]
	if !ok {
		t.Fatalf("capture has none of `food`, `foods_search`, or `food_response`; keys = %v", sortedKeys(generic))
	}
	var detections []map[string]json.RawMessage
	if err := json.Unmarshal(rawResponse, &detections); err != nil {
		var single map[string]json.RawMessage
		if serr := json.Unmarshal(rawResponse, &single); serr != nil {
			t.Fatalf("food_response is neither an array nor an object: %v / %v", err, serr)
		}
		detections = []map[string]json.RawMessage{single}
	}
	foodObjects := make([]json.RawMessage, 0, len(detections))
	for _, d := range detections {
		if food, has := d["food"]; has {
			foodObjects = append(foodObjects, food)
		}
	}
	return foodObjects
}

func searchFoodObjects(t *testing.T, rawSearch json.RawMessage) []json.RawMessage {
	t.Helper()
	var search map[string]json.RawMessage
	if err := json.Unmarshal(rawSearch, &search); err != nil {
		t.Fatalf("foods_search is not an object: %v", err)
	}
	rawResults, ok := search["results"]
	if !ok {
		return nil // total_results=0 omits `results` entirely
	}
	var results map[string]json.RawMessage
	if err := json.Unmarshal(rawResults, &results); err != nil {
		t.Fatalf("foods_search.results is not an object: %v", err)
	}
	rawFood, ok := results["food"]
	if !ok {
		return nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(rawFood, &list); err != nil {
		return []json.RawMessage{rawFood} // collapsed to a single object
	}
	return list
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var rawMessageType = reflect.TypeOf(json.RawMessage(nil))

// modelledFields maps each JSON key a struct declares to its field type.
func modelledFields(t *testing.T, typ reflect.Type) map[string]reflect.Type {
	t.Helper()
	if typ.Kind() != reflect.Struct {
		t.Fatalf("modelledFields: %s is not a struct", typ)
	}
	fields := make(map[string]reflect.Type, typ.NumField())
	for i := range typ.NumField() {
		tag := typ.Field(i).Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name != "" {
			fields[name] = typ.Field(i).Type
		}
	}
	return fields
}

// assertKeysModelled walks a raw JSON object against a struct type, at every
// depth. Wire keys with no struct field are reported; fields held as
// json.RawMessage are skipped (their shape is deliberately unverified and any
// bytes are legal); FlexSlice fields unwrap FatSecret's singular-key wrapper
// convention before recursing into their element type.
func assertKeysModelled(t *testing.T, path string, raw json.RawMessage, typ reflect.Type) {
	t.Helper()

	var onTheWire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &onTheWire); err != nil {
		t.Errorf("%s: not a JSON object: %v", path, err)
		return
	}
	fields := modelledFields(t, typ)

	for _, key := range sortedKeys(onTheWire) {
		fieldType, ok := fields[key]
		if !ok {
			t.Errorf("%s: FatSecret sends %q and %s does not model it. "+
				"Capture the shape and add it to spec/fatsecret.yaml — an unmodelled key is "+
				"dropped silently today and becomes an outage the moment someone models it wrong.",
				path, key, typ)
			continue
		}
		if fieldType == rawMessageType {
			continue
		}
		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		val := onTheWire[key]
		switch fieldType.Kind() {
		case reflect.Struct:
			if isObject(val) {
				assertKeysModelled(t, path+"."+key, val, fieldType)
			}
		case reflect.Slice:
			elem := fieldType.Elem()
			if elem.Kind() != reflect.Struct {
				continue
			}
			for i, obj := range elementObjects(val) {
				assertKeysModelled(t, path+"."+key+"["+strconv.Itoa(i)+"]", obj, elem)
			}
		default:
			// scalar field, nothing beneath it to check
		}
	}
}

// elementObjects returns the JSON objects inside a list-shaped value in any
// of FatSecret's forms: a singular-key wrapper {"serving": ...}, a bare
// array, or a single collapsed object.
func elementObjects(raw json.RawMessage) []json.RawMessage {
	// Unwrap the singular-key wrapper if present.
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper) == 1 {
		for _, inner := range wrapper {
			raw = inner
		}
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		out := make([]json.RawMessage, 0, len(list))
		for _, el := range list {
			if isObject(el) {
				out = append(out, el)
			}
		}
		return out
	}
	if isObject(raw) {
		return []json.RawMessage{raw}
	}
	return nil
}

func isObject(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return strings.HasPrefix(trimmed, "{")
}

// Every key on every captured food object — at every depth — is one the
// generated structs model.
func TestCapturedFoodObjects_CarryNoUnmodelledKeyAtAnyDepth(t *testing.T) {
	for name, capture := range loadCaptures(t) {
		t.Run(name, func(t *testing.T) {
			objects := capturedFoodObjects(t, capture)
			if len(objects) == 0 {
				t.Skip("capture carries no catalogue food object")
			}
			for i, raw := range objects {
				assertKeysModelled(t, "food["+strconv.Itoa(i)+"]", raw, reflect.TypeOf(Food{}))
			}
		})
	}
}

// Every captured food object decodes through the production path without a
// type error, with real fields reached. A type error here is the outage
// class: Go abandons the enclosing value, so one field would cost every
// detection in the reply.
func TestCapturedFoodObjects_DecodeThroughProductionPath(t *testing.T) {
	for name, capture := range loadCaptures(t) {
		t.Run(name, func(t *testing.T) {
			objects := capturedFoodObjects(t, capture)
			if len(objects) == 0 {
				t.Skip("capture carries no catalogue food object")
			}
			for i, raw := range objects {
				var food Food
				if err := json.Unmarshal(raw, &food); err != nil {
					t.Errorf("food[%d] does not decode: %v", i, err)
					continue
				}
				if food.FoodName == "" {
					t.Errorf("food[%d]: decoded with an empty food_name — the decode is not reaching real fields", i)
				}
				// Where the wire carries servings, the decode must keep them:
				// losing the serving list is the quiet degradation the
				// consumer tolerates in production, which makes silently
				// dropping it here the failure this suite exists to catch.
				var probe map[string]json.RawMessage
				if err := json.Unmarshal(raw, &probe); err == nil {
					if _, hasServings := probe["servings"]; hasServings && len(food.Servings.Items()) == 0 {
						t.Errorf("food[%d]: wire carries servings but the decode kept none", i)
					}
				}
			}
		})
	}
}
