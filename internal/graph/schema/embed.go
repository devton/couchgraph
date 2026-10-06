// Package schema embeds the GraphQL SDL files so they can be loaded at runtime
// by the dynamic engine (internal/engine), while gqlgen keeps reading the same
// files at generation time. These files are the single source of truth.
package schema

import _ "embed"

// Core is the agnostic core SDL (Document, findDocs, queryView, mutations,
// docChanges, Map scalar). It is always loaded by the dynamic engine.
//
//go:embed schema.graphqls
var Core string

// CoreFile is the source name reported in schema errors for Core.
const CoreFile = "core/schema.graphqls"
