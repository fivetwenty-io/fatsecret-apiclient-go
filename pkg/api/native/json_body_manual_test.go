package native_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/api/native"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/client/mock"
	"github.com/fivetwenty-io/fatsecret-apiclient-go/pkg/types"
)

// The two native endpoints are the only ones that take a JSON body. Sending
// them the form-urlencoded body every other FatSecret endpoint expects is not
// rejected with a decodable error — the API answers HTTP 200 carrying error
// code 1, "An unknown error occurred: 'please try again later'", which reads
// like a transient upstream fault and is indistinguishable from one. These
// tests pin the content type and the JSON-native value types so that failure
// mode cannot return silently.
//
// These assert the REQUEST side only. The stub replies with an object because
// that is what the generated NLPFoodResponse models; the live endpoint answers
// with an array of detected foods carrying `eaten`/`suggested_serving`, a
// separate known defect in the generated response type that consumers decode
// around. Keeping the stub decodable isolates these tests to the encoding
// change they cover.

// capture records the request body and content type a service method sent.
type capture struct {
	contentType string
	body        []byte
	query       map[string][]string
}

func captureServer(t *testing.T, responseJSON string) (string, *capture) {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		got.contentType = r.Header.Get("Content-Type")
		got.body = body
		got.query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responseJSON))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, got
}

func TestRecognize_SendsJSONBody(t *testing.T) {
	url, got := captureServer(t, `{"food_response":{}}`)
	svc := native.New(mock.NewClient(url))

	imageB64 := "aGVsbG8="
	region := "US"
	includeFoodData := types.APIBool(true)
	if _, err := svc.Recognize(context.Background(), native.RecognizeRequest{
		ImageB64:        &imageB64,
		Region:          &region,
		IncludeFoodData: &includeFoodData,
	}); err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}

	var body map[string]any
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("body is not JSON (%q): %v", got.body, err)
	}
	if body["image_b64"] != imageB64 {
		t.Errorf("image_b64 = %v, want %q", body["image_b64"], imageB64)
	}
	if body["region"] != "US" {
		t.Errorf("region = %v, want US", body["region"])
	}
	// A JSON boolean, not the "1" the form encoder produces: the endpoint does
	// not coerce a stringified flag.
	if body["include_food_data"] != true {
		t.Errorf("include_food_data = %#v, want bool true", body["include_food_data"])
	}

	// format=json must NOT be sent. It is mandatory everywhere else in the API,
	// but this endpoint rejects it with the same opaque code 1 a form body
	// earns, so a well-meant reinstatement would look like an upstream outage.
	if q := got.query["format"]; len(q) != 0 {
		t.Errorf("format query param = %v, want it absent — this endpoint rejects it", q)
	}
	if len(got.query) != 0 {
		t.Errorf("query string = %v, want empty", got.query)
	}
}

func TestRecognize_OmitsUnsetFields(t *testing.T) {
	url, got := captureServer(t, `{"food_response":{}}`)
	svc := native.New(mock.NewClient(url))

	imageB64 := "aGVsbG8="
	if _, err := svc.Recognize(context.Background(), native.RecognizeRequest{ImageB64: &imageB64}); err != nil {
		t.Fatalf("Recognize: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("body is not JSON (%q): %v", got.body, err)
	}
	if len(body) != 1 {
		t.Errorf("body = %v, want only image_b64 (nil fields must be omitted, not sent as null)", body)
	}
}

func TestProcess_SendsJSONBody(t *testing.T) {
	url, got := captureServer(t, `{"food_response":{}}`)
	svc := native.New(mock.NewClient(url))

	input := "1 apple and 2 eggs"
	if _, err := svc.Process(context.Background(), native.ProcessRequest{UserInput: &input}); err != nil {
		t.Fatalf("Process: %v", err)
	}

	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}
	var body map[string]any
	if err := json.Unmarshal(got.body, &body); err != nil {
		t.Fatalf("body is not JSON (%q): %v", got.body, err)
	}
	if body["user_input"] != input {
		t.Errorf("user_input = %v, want %q", body["user_input"], input)
	}
}
