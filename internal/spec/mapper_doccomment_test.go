package spec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/antst/go-apispec/internal/metadata"
)

func TestSplitDocComment_SingleSentence(t *testing.T) {
	summary, desc := splitDocComment("GetUser returns a user by ID.")
	assert.Equal(t, "GetUser returns a user by ID.", summary)
	assert.Empty(t, desc, "single sentence should not duplicate as description")
}

func TestSplitDocComment_MultipleSentences(t *testing.T) {
	summary, desc := splitDocComment("GetUser returns a user by ID. It requires authentication and checks permissions.")
	assert.Equal(t, "GetUser returns a user by ID.", summary)
	assert.Equal(t, "GetUser returns a user by ID. It requires authentication and checks permissions.", desc)
}

func TestSplitDocComment_MultiLine(t *testing.T) {
	comment := "GetUser returns a user by ID.\nIt requires authentication."
	summary, desc := splitDocComment(comment)
	assert.Equal(t, "GetUser returns a user by ID.", summary)
	assert.Equal(t, comment, desc)
}

func TestSplitDocComment_Empty(t *testing.T) {
	summary, desc := splitDocComment("")
	assert.Empty(t, summary)
	assert.Empty(t, desc)
}

func TestSplitDocComment_Whitespace(t *testing.T) {
	summary, desc := splitDocComment("  ")
	assert.Empty(t, summary)
	assert.Empty(t, desc)
}

func TestSplitDocComment_NoPeriod(t *testing.T) {
	summary, desc := splitDocComment("Returns a user by ID")
	assert.Equal(t, "Returns a user by ID", summary)
	assert.Empty(t, desc)
}

func TestSplitDocComment_PeriodAtEnd(t *testing.T) {
	summary, desc := splitDocComment("Returns a user by ID.")
	assert.Equal(t, "Returns a user by ID.", summary)
	assert.Empty(t, desc)
}

func TestSplitDocComment_URLInComment(t *testing.T) {
	// URLs contain dots but shouldn't be treated as sentence boundaries
	comment := "See https://example.com/docs for details. More info here."
	summary, desc := splitDocComment(comment)
	assert.Equal(t, "See https://example.com/docs for details.", summary)
	assert.Equal(t, comment, desc)
}

func TestExtractDocComment_Found(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"myapp": {
				Files: map[string]*metadata.File{
					"main.go": {
						Functions: map[string]*metadata.Function{
							"GetUser": {
								Name:     sp.Get("GetUser"),
								Comments: sp.Get("GetUser returns a user by ID. It checks permissions."),
							},
						},
					},
				},
			},
		},
	}

	route := &RouteInfo{
		Function: "myapp.GetUser",
		Metadata: meta,
	}

	summary, desc := extractDocComment(route)
	assert.Equal(t, "GetUser returns a user by ID.", summary)
	assert.Contains(t, desc, "It checks permissions.")
}

func TestExtractDocComment_NotFound(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages:   map[string]*metadata.Package{},
	}

	route := &RouteInfo{
		Function: "myapp.UnknownFunc",
		Metadata: meta,
	}

	summary, desc := extractDocComment(route)
	assert.Empty(t, summary)
	assert.Empty(t, desc)
}

func TestExtractDocComment_NilMetadata(t *testing.T) {
	route := &RouteInfo{
		Function: "myapp.GetUser",
		Metadata: nil,
	}

	summary, desc := extractDocComment(route)
	assert.Empty(t, summary)
	assert.Empty(t, desc)
}

func TestExtractDocComment_EmptyComment(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"myapp": {
				Files: map[string]*metadata.File{
					"main.go": {
						Functions: map[string]*metadata.Function{
							"GetUser": {
								Name:     sp.Get("GetUser"),
								Comments: sp.Get(""), // empty comment
							},
						},
					},
				},
			},
		},
	}

	route := &RouteInfo{
		Function: "myapp.GetUser",
		Metadata: meta,
	}

	summary, desc := extractDocComment(route)
	assert.Empty(t, summary)
	assert.Empty(t, desc)
}

func TestExtractDocComment_ReceiverMethod(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"handlers": {
				Files: map[string]*metadata.File{
					"user.go": {
						Functions: map[string]*metadata.Function{
							"GetUser": {
								Name:     sp.Get("GetUser"),
								Comments: sp.Get("GetUser fetches a user."),
							},
						},
					},
				},
			},
		},
	}

	route := &RouteInfo{
		Function: "handlers.UserHandler.GetUser",
		Metadata: meta,
	}

	summary, _ := extractDocComment(route)
	assert.Equal(t, "GetUser fetches a user.", summary)
}

func TestExtractDocComment_NilRoute(_ *testing.T) {
	// Should not panic
	extractDocComment(nil)
}

func TestExtractDocComment_PackageScopedLookup(t *testing.T) {
	// Two packages with same function name but different comments
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"users": {
				Files: map[string]*metadata.File{
					"handler.go": {
						Functions: map[string]*metadata.Function{
							"Create": {
								Name:     sp.Get("Create"),
								Comments: sp.Get("Create registers a new user."),
							},
						},
					},
				},
			},
			"items": {
				Files: map[string]*metadata.File{
					"handler.go": {
						Functions: map[string]*metadata.Function{
							"Create": {
								Name:     sp.Get("Create"),
								Comments: sp.Get("Create adds a new item."),
							},
						},
					},
				},
			},
		},
	}

	// Should match the correct package
	route := &RouteInfo{Function: "users.Create", Metadata: meta}
	summary, _ := extractDocComment(route)
	assert.Equal(t, "Create registers a new user.", summary)

	route2 := &RouteInfo{Function: "items.Create", Metadata: meta}
	summary2, _ := extractDocComment(route2)
	assert.Equal(t, "Create adds a new item.", summary2)
}

func TestExtractDocComment_FallbackWhenPackageDoesntMatch(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"myapp": {
				Files: map[string]*metadata.File{
					"main.go": {
						Functions: map[string]*metadata.Function{
							"Serve": {
								Name:     sp.Get("Serve"),
								Comments: sp.Get("Serve starts the server."),
							},
						},
					},
				},
			},
		},
	}

	// Package prefix doesn't match any metadata package — falls back to global search
	route := &RouteInfo{Function: "unknown.Serve", Metadata: meta}
	summary, _ := extractDocComment(route)
	assert.Equal(t, "Serve starts the server.", summary)
}

func TestExtractDocComment_DescriptionOverrideTakesPrecedence(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"myapp": {
				Files: map[string]*metadata.File{
					"main.go": {
						Functions: map[string]*metadata.Function{
							"GetUser": {
								Name:     sp.Get("GetUser"),
								Comments: sp.Get("GetUser returns a user. It checks permissions."),
							},
						},
					},
				},
			},
		},
	}

	route := &RouteInfo{
		Function:    "myapp.GetUser",
		Description: "Custom description from config override.",
		Metadata:    meta,
	}

	// Simulate what the mapper does
	summary, description := extractDocComment(route)
	if route.Summary != "" {
		summary = route.Summary
	}
	if route.Description != "" {
		description = route.Description
	}

	assert.Equal(t, "GetUser returns a user.", summary)
	assert.Equal(t, "Custom description from config override.", description)
}

func TestExtractDocComment_SummaryOverrideTakesPrecedence(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"myapp": {
				Files: map[string]*metadata.File{
					"main.go": {
						Functions: map[string]*metadata.Function{
							"GetUser": {
								Name:     sp.Get("GetUser"),
								Comments: sp.Get("GetUser returns a user."),
							},
						},
					},
				},
			},
		},
	}

	route := &RouteInfo{
		Function: "myapp.GetUser",
		Summary:  "Custom summary from config.",
		Metadata: meta,
	}

	summary, _ := extractDocComment(route)
	if route.Summary != "" {
		summary = route.Summary
	}

	assert.Equal(t, "Custom summary from config.", summary)
}
