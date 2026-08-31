package httptransport

import (
	"encoding/json"
	"testing"

	"github.com/lihongjie0209/dictionary-service/internal/dictionary"
)

func TestDictionaryTransportEmitsStructuredJSON(t *testing.T) {
	view := dictionaryView(dictionary.Dictionary{MetadataJSON: `{"color":"blue"}`})
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	metadata, ok := payload["metadata_json"].(map[string]any)
	if !ok || metadata["color"] != "blue" {
		t.Fatalf("metadata_json = %#v, want JSON object", payload["metadata_json"])
	}
}

func TestProviderTransportEmitsCapabilitiesArray(t *testing.T) {
	view := providerView(dictionary.Provider{CapabilitiesJSON: `[{"dictionary_code":"regions"}]`})
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	capabilities, ok := payload["capabilities_json"].([]any)
	if !ok || len(capabilities) != 1 {
		t.Fatalf("capabilities_json = %#v, want JSON array", payload["capabilities_json"])
	}
}
