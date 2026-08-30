package dictionary

import (
	"errors"
	"testing"
)

func TestValidateDictionary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   DictionaryInput
		wantErr bool
	}{
		{name: "valid", input: DictionaryInput{Code: "order.status", Name: "Order status", MetadataJSON: `{}`}},
		{name: "invalid code", input: DictionaryInput{Code: "Order Status", Name: "Order status"}, wantErr: true},
		{name: "missing name", input: DictionaryInput{Code: "order.status"}, wantErr: true},
		{name: "invalid metadata", input: DictionaryInput{Code: "order.status", Name: "Order status", MetadataJSON: `{`}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateDictionary(test.input, true)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateDictionary() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestBuildTree(t *testing.T) {
	t.Parallel()
	items := []Item{
		{ID: "root", Code: "root", Name: "Root", SortOrder: 1},
		{ID: "child-b", ParentID: "root", Code: "child.b", Name: "Beta", SortOrder: 2},
		{ID: "child-a", ParentID: "root", Code: "child.a", Name: "Alpha", SortOrder: 1},
	}
	roots, truncated, err := buildTree(items, "full", "", "", 8, 100)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(roots) != 1 || len(roots[0].Children) != 2 || roots[0].Children[0].Item.ID != "child-a" {
		t.Fatalf("unexpected tree: roots=%+v truncated=%v", roots, truncated)
	}
}

func TestBuildTreeSearchIncludesAncestors(t *testing.T) {
	t.Parallel()
	items := []Item{{ID: "root", Code: "region", Name: "Region"}, {ID: "city", ParentID: "root", Code: "shanghai", Name: "Shanghai"}}
	roots, _, err := buildTree(items, "search_with_ancestors", "", "shang", 8, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || len(roots[0].Children) != 1 || roots[0].Children[0].Item.ID != "city" {
		t.Fatalf("search tree did not preserve ancestors: %+v", roots)
	}
}

func TestValidateTreeRejectsCycleAndMissingParent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		items []Item
	}{
		{name: "cycle", items: []Item{{ID: "a", ParentID: "b"}, {ID: "b", ParentID: "a"}}},
		{name: "missing parent", items: []Item{{ID: "a", ParentID: "missing"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validateTree(test.items); err == nil {
				t.Fatal("validateTree() error = nil")
			}
		})
	}
}

func TestTranslatePreservesApplicationError(t *testing.T) {
	t.Parallel()
	original := paginationError()
	if got := translate(original); !errors.Is(got, original) {
		t.Fatalf("translate() = %v, want original %v", got, original)
	}
}

func TestValidateProviderAppliesLimits(t *testing.T) {
	t.Parallel()
	value, err := validateProvider(ProviderInput{ServiceName: "order-service", Target: "order-service:9090", Capabilities: []Capability{{DictionaryCode: "order.status", SupportsSearch: true}}, LeaseSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if value.TimeoutMilliseconds != 3000 || value.Capabilities[0].MaxPageSize != 100 {
		t.Fatalf("defaults not applied: %+v", value)
	}
}

func TestValidateProviderRejectsDuplicateDictionary(t *testing.T) {
	t.Parallel()
	_, err := validateProvider(ProviderInput{ServiceName: "order-service", Target: "order-service:9090", Capabilities: []Capability{{DictionaryCode: "order.status"}, {DictionaryCode: "order.status"}}, LeaseSeconds: 60})
	if err == nil {
		t.Fatal("validateProvider() error = nil")
	}
}

func TestLeaseTokenHash(t *testing.T) {
	t.Parallel()
	token, hashed, err := newLeaseToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || hashed == token || hashToken(token) != hashed {
		t.Fatal("lease token was empty, stored in plaintext, or hashed inconsistently")
	}
}

func paginationError() error {
	_, _, err := pagination(1, 101)
	return err
}
