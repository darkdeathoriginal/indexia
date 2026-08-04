package models_test

import (
	"testing"

	"indexia/models"
)

func TestGetFirstLetter(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Apple", "A"},
		{"apple", "A"},
		{" Banana", "B"},
		{"123 Links", "#"},
		{"!Special", "#"},
		{"", "#"},
		{"Zebra", "Z"},
	}

	for _, tt := range tests {
		result := models.GetFirstLetter(tt.input)
		if result != tt.expected {
			t.Errorf("GetFirstLetter(%q) = %q; want %q", tt.input, result, tt.expected)
		}
	}
}
