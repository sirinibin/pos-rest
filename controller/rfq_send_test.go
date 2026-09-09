package controller

import (
	"testing"
)

// TestBuildRFQHeaderComponent covers all branches of buildRFQHeaderComponent.
func TestBuildRFQHeaderComponent(t *testing.T) {
	tests := []struct {
		name                string
		mediaType           string
		templateHeaderFormat string
		mediaID             string
		rfqCode             string
		wantEffectiveType   string
		wantFilename        string // empty = no filename key expected
	}{
		{
			name:                "document template + no override → document with filename",
			mediaType:           "",
			templateHeaderFormat: "DOCUMENT",
			mediaID:             "media-123",
			rfqCode:             "RFQ-0017",
			wantEffectiveType:   "document",
			wantFilename:        "RFQ-0017.pdf",
		},
		{
			name:                "image template + no override → image, no filename",
			mediaType:           "",
			templateHeaderFormat: "IMAGE",
			mediaID:             "media-456",
			rfqCode:             "RFQ-0017",
			wantEffectiveType:   "image",
			wantFilename:        "",
		},
		{
			name:                "explicit document mediaType wins over IMAGE template",
			mediaType:           "document",
			templateHeaderFormat: "IMAGE",
			mediaID:             "media-789",
			rfqCode:             "RFQ-0005",
			wantEffectiveType:   "document",
			wantFilename:        "RFQ-0005.pdf",
		},
		{
			name:                "explicit image mediaType wins over DOCUMENT template",
			mediaType:           "image",
			templateHeaderFormat: "DOCUMENT",
			mediaID:             "media-abc",
			rfqCode:             "RFQ-0001",
			wantEffectiveType:   "image",
			wantFilename:        "",
		},
		{
			name:                "empty templateHeaderFormat falls back to image",
			mediaType:           "",
			templateHeaderFormat: "",
			mediaID:             "media-xyz",
			rfqCode:             "RFQ-0099",
			wantEffectiveType:   "image",
			wantFilename:        "",
		},
		{
			name:                "filename uses rfqCode verbatim — no double prefix",
			mediaType:           "",
			templateHeaderFormat: "DOCUMENT",
			mediaID:             "media-001",
			rfqCode:             "RFQ-0001",
			wantEffectiveType:   "document",
			wantFilename:        "RFQ-0001.pdf", // NOT "RFQ-RFQ-0001.pdf"
		},
		{
			name:                "filename uses rfqCode verbatim — high serial",
			mediaType:           "",
			templateHeaderFormat: "DOCUMENT",
			mediaID:             "media-999",
			rfqCode:             "RFQ-9999",
			wantEffectiveType:   "document",
			wantFilename:        "RFQ-9999.pdf",
		},
		{
			name:                "lowercase document mediaType is accepted",
			mediaType:           "document",
			templateHeaderFormat: "DOCUMENT",
			mediaID:             "media-lower",
			rfqCode:             "RFQ-0010",
			wantEffectiveType:   "document",
			wantFilename:        "RFQ-0010.pdf",
		},
		{
			name:                "lowercase image mediaType is accepted",
			mediaType:           "image",
			templateHeaderFormat: "IMAGE",
			mediaID:             "media-img",
			rfqCode:             "RFQ-0011",
			wantEffectiveType:   "image",
			wantFilename:        "",
		},
		{
			name:                "type field is always 'header'",
			mediaType:           "",
			templateHeaderFormat: "DOCUMENT",
			mediaID:             "media-type",
			rfqCode:             "RFQ-0002",
			wantEffectiveType:   "document",
			wantFilename:        "RFQ-0002.pdf",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp := buildRFQHeaderComponent(tc.mediaType, tc.templateHeaderFormat, tc.mediaID, tc.rfqCode)

			// Top-level type must be "header"
			if got := comp["type"]; got != "header" {
				t.Errorf("comp[type] = %q, want \"header\"", got)
			}

			// Extract the single parameter entry
			params, ok := comp["parameters"].([]interface{})
			if !ok || len(params) != 1 {
				t.Fatalf("parameters must be a slice of length 1, got %v", comp["parameters"])
			}
			param, ok := params[0].(map[string]interface{})
			if !ok {
				t.Fatalf("parameter[0] is not a map: %v", params[0])
			}

			// Check "type" in parameter
			if got := param["type"]; got != tc.wantEffectiveType {
				t.Errorf("param[type] = %q, want %q", got, tc.wantEffectiveType)
			}

			// Check the nested media object
			media, ok := param[tc.wantEffectiveType].(map[string]interface{})
			if !ok {
				t.Fatalf("param[%q] is not a map: %v", tc.wantEffectiveType, param[tc.wantEffectiveType])
			}

			// ID must equal mediaID
			if got := media["id"]; got != tc.mediaID {
				t.Errorf("media[id] = %q, want %q", got, tc.mediaID)
			}

			// Filename check
			gotFilename, hasFilename := media["filename"]
			if tc.wantFilename == "" {
				if hasFilename {
					t.Errorf("media[filename] should be absent for image type, got %q", gotFilename)
				}
			} else {
				if !hasFilename {
					t.Errorf("media[filename] missing, want %q", tc.wantFilename)
				} else if gotFilename != tc.wantFilename {
					t.Errorf("media[filename] = %q, want %q", gotFilename, tc.wantFilename)
				}
			}
		})
	}
}
