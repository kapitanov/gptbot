package llmprovider

import (
	"reflect"
	"testing"
)

func TestParseModels(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "single", in: "z-ai/glm-5.3", want: []string{"z-ai/glm-5.3"}},
		{name: "list", in: "a, b,c", want: []string{"a", "b", "c"}},
		{name: "spaces and duplicates", in: " a ,, a , b ,", want: []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseModels(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseModels(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFindModel(t *testing.T) {
	models := []string{"z-ai/glm-5.3", "nousresearch/hermes-4-405b"}

	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "exact", input: "z-ai/glm-5.3", want: "z-ai/glm-5.3", ok: true},
		{name: "case insensitive", input: "Z-AI/GLM-5.3", want: "z-ai/glm-5.3", ok: true},
		{name: "surrounding spaces", input: "  nousresearch/hermes-4-405b ", want: "nousresearch/hermes-4-405b", ok: true},
		{name: "unknown", input: "gpt-5", ok: false},
		{name: "empty", input: "", ok: false},
		{name: "spaces only", input: "   ", ok: false},
		{name: "empty list", input: "z-ai/glm-5.3", ok: false, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FindModel(nil, tt.input)
			if tt.name != "empty list" {
				got, ok = FindModel(models, tt.input)
			}
			if got != tt.want || ok != tt.ok {
				t.Fatalf("FindModel(%q) = (%q, %v), want (%q, %v)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNewRequiresModel(t *testing.T) {
	_, err := New(Parameters{Provider: ProviderOpenAI, Model: " , "})
	if err == nil {
		t.Fatal("expected an error for an empty model list")
	}
}
