package spec

import (
	"strings"

	"github.com/antst/go-apispec/internal/metadata"
)

// wrapperFieldOverride describes one field of a wrapper-style response struct
// whose concrete payload type was recovered at the call site through the
// assignment + parameter chain. StructFieldName is the Go field name (e.g.
// "Data"); the JSON name is resolved from the wrapper type's tags at
// schema-emission time.
type wrapperFieldOverride struct {
	StructFieldName string
	GoType          string
}

// collectWrapperOverrides specialises a wrapper-response body using primitives
// that are already populated for every function — Function.AssignmentMap,
// Function.ReturnVars, and CallGraphEdge.ParamArgMap.
//
// The pattern we recognise:
//
//	func RespondWithSuccess(w http.ResponseWriter, msg string, data any, code int) {
//	    response := NewEnvelope(msg, data, code)
//	    json.NewEncoder(w).Encode(response)
//	}
//
//	func NewEnvelope(msg string, data any, code int) *Envelope {
//	    return &Envelope{Message: msg, Data: data, Code: code}
//	}
//
// When the response matcher matches Encode and the body arg is the local
// `response`, we specialise the Envelope schema with the caller-site type of
// the `data` argument. The chain is:
//
//	response ── helper.AssignmentMap ──► NewEnvelope(msg, data, code)
//	NewEnvelope.ReturnVars[i]          ── the composite literal &Envelope{...}
//	   field "Data" is bound to constructor param "data"
//	   constructor edge ParamArgMap["data"] ──► the helper-local ident `data`
//	   helper's parent edges (ParamArgMap) ──► caller's actual arg type
//
// Unlike upstream, the constructor's parameter names are read from its
// call-graph edge's ParamArgMap (populated from go/types) rather than from
// Function.Signature, which in this fork retains only parameter *types*, not
// names.
func (r *ResponsePatternMatcherImpl) collectWrapperOverrides(arg *metadata.CallArgument, node TrackerNodeInterface) []wrapperFieldOverride {
	if arg == nil || arg.GetKind() != metadata.KindIdent || node == nil {
		return nil
	}
	edge := node.GetEdge()
	if edge == nil {
		return nil
	}
	meta := metadataFromContextProvider(r.contextProvider)
	if meta == nil {
		return nil
	}

	helper := findFunction(meta, meta.StringPool.GetString(edge.Caller.Pkg), meta.StringPool.GetString(edge.Caller.Name))
	if helper == nil {
		return nil
	}
	assigns := helper.AssignmentMap[arg.GetName()]
	if len(assigns) == 0 {
		return nil
	}
	assign := assigns[len(assigns)-1] // latest wins, like TraceVariableOrigin
	if assign.CalleeFunc == "" || assign.CalleePkg == "" {
		return nil
	}

	ctor := findFunction(meta, assign.CalleePkg, assign.CalleeFunc)
	if ctor == nil {
		return nil
	}
	idx := assign.ReturnIndex
	if idx < 0 || idx >= len(ctor.ReturnVars) {
		return nil
	}

	// The constructor's call edge carries the reliable param-name→arg map
	// (built from go/types) that Function.Signature does not retain here.
	ctorEdge := findConstructorEdge(meta, edge.Caller.BaseID(), assign.CalleePkg, assign.CalleeFunc)
	if ctorEdge == nil {
		return nil
	}

	bindings := fieldParamBindingsFromReturnVar(&ctor.ReturnVars[idx], paramNameSetFromEdge(ctorEdge))
	if len(bindings) == 0 {
		return nil
	}

	out := make([]wrapperFieldOverride, 0, len(bindings))
	for fieldName, ctorParamName := range bindings {
		ctorArg, ok := ctorEdge.ParamArgMap[ctorParamName]
		if !ok {
			continue
		}
		concrete := r.resolveOverrideGoType(&ctorArg, node)
		if concrete == "" {
			continue
		}
		out = append(out, wrapperFieldOverride{
			StructFieldName: fieldName,
			GoType:          concrete,
		})
	}
	return out
}

// fieldParamBindingsFromReturnVar inspects a constructor's ReturnVars entry
// and, when the return is a composite literal `T{Field: paramIdent, ...}` or
// `&T{Field: paramIdent, ...}`, reports which struct fields are bound directly
// to parameters of the constructor. The mapping is field-name → parameter-name.
// `params` is the set of valid constructor parameter names.
func fieldParamBindingsFromReturnVar(arg *metadata.CallArgument, params map[string]bool) map[string]string {
	if arg == nil {
		return nil
	}
	cl := arg
	// Strip address-of (&T{...}) and parens.
	for cl != nil {
		switch cl.GetKind() {
		case metadata.KindUnary, metadata.KindParen:
			cl = cl.X
		default:
			goto done
		}
	}
done:
	if cl == nil || cl.GetKind() != metadata.KindCompositeLit {
		return nil
	}
	out := map[string]string{}
	for _, elt := range cl.Args {
		if elt == nil || elt.GetKind() != metadata.KindKeyValue {
			continue
		}
		keyArg := elt.X   // composite-literal key (field name)
		valArg := elt.Fun // composite-literal value
		if keyArg == nil || valArg == nil {
			continue
		}
		if keyArg.GetKind() != metadata.KindIdent || valArg.GetKind() != metadata.KindIdent {
			continue
		}
		fieldName := keyArg.GetName()
		paramName := valArg.GetName()
		if fieldName == "" || paramName == "" {
			continue
		}
		if !params[paramName] {
			continue
		}
		out[fieldName] = paramName
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveOverrideGoType produces the Go-type string for one bound field by
// walking the helper-call's parent edges via ParamArgMap when the
// constructor's arg is a helper-parameter passthrough, then falling back to
// the constructor-arg's own declared type. The result is run through
// cleanOverrideType, which drops values that aren't real Go types (function
// names, untyped expressions, interface{}, …) so the override stays a safe
// no-op for those shapes rather than emitting a $ref to a non-existent
// component.
func (r *ResponsePatternMatcherImpl) resolveOverrideGoType(ctorArg *metadata.CallArgument, node TrackerNodeInterface) string {
	if ctorArg == nil {
		return ""
	}
	if ctorArg.GetKind() == metadata.KindIdent {
		if t := cleanOverrideType(r.resolveParamArgType(node, ctorArg.GetName())); t != "" {
			return t
		}
	}
	return cleanOverrideType(ctorArg.GetType())
}

// cleanOverrideType normalises a Go-type string for use by the schema mapper,
// returning "" when the input doesn't describe a concrete user-defined type.
// Rejected shapes:
//
//   - empty / interface{} / any — no information beyond the wrapper's declared
//     field type.
//   - "untyped …" — Go's type-system tag for untyped constants.
//   - bare identifiers with no dot, slash, or container prefix that aren't
//     recognised primitives — almost always a function name that leaked
//     through GetArgumentInfo for a call whose return type wasn't populated.
func cleanOverrideType(t string) string {
	t = strings.TrimPrefix(strings.TrimSpace(t), "&")
	t = strings.TrimPrefix(t, "*")
	if t == "" || t == "interface{}" || t == "any" {
		return ""
	}
	if strings.HasPrefix(t, "untyped ") {
		return ""
	}
	if !strings.ContainsAny(t, "./[") && !metadata.IsPrimitiveType(t) {
		return ""
	}
	return t
}

// specialiseWrapperSchema composes a per-route response schema by taking the
// base wrapper $ref and overlaying an inline object that overrides the
// resolved fields. Result shape:
//
//	allOf:
//	  - $ref: '#/components/schemas/Envelope'
//	  - type: object
//	    properties:
//	      data:
//	        $ref: '#/components/schemas/Order'
//
// If baseSchema isn't a $ref (e.g. the mapper inlined it) or no override
// property survived JSON-name resolution, the original schema is returned
// unchanged.
func specialiseWrapperSchema(baseSchema *Schema, overrides []wrapperFieldOverride, wrapperGoType string, usedTypes map[string]*Schema, meta *metadata.Metadata, cfg *APISpecConfig) *Schema {
	if baseSchema == nil || baseSchema.Ref == "" || len(overrides) == 0 || meta == nil {
		return baseSchema
	}
	wrapperType := lookupWrapperType(meta, wrapperGoType)
	if wrapperType == nil {
		return baseSchema
	}

	properties := map[string]*Schema{}
	for _, override := range overrides {
		// Only specialise fields whose declared wrapper type is genuinely
		// generic (interface{} / any). Fields with a concrete declared type —
		// e.g. `Message string`, `Code int` — already render correctly from
		// the base $ref, and overriding them would mis-render the call-site
		// literal as the field's type.
		if !wrapperFieldIsGeneric(meta, wrapperType, override.StructFieldName) {
			continue
		}
		jsonName := jsonNameForField(meta, wrapperType, override.StructFieldName)
		if jsonName == "" {
			continue
		}
		propSchema, discovered := schemaForType(usedTypes, override.GoType, nil, meta, cfg, nil)
		if propSchema == nil {
			continue
		}
		// mapGoTypeToOpenAPISchema returns the payload's $ref in propSchema but
		// hands its freshly discovered component definitions back in
		// `discovered` — the caller is responsible for registering them, or the
		// `data` $ref we just produced can point at a component nothing ever
		// populates.
		for name, sch := range discovered {
			markUsedType(usedTypes, name, sch)
		}
		properties[jsonName] = propSchema
	}
	if len(properties) == 0 {
		return baseSchema
	}
	return &Schema{
		AllOf: []*Schema{
			baseSchema,
			{Type: "object", Properties: properties},
		},
	}
}

// --- helpers ---------------------------------------------------------

func metadataFromContextProvider(cp ContextProvider) *metadata.Metadata {
	if impl, ok := cp.(*ContextProviderImpl); ok {
		return impl.meta
	}
	return nil
}

// findFunction looks up a function declaration by (pkg, name) across all files
// in the package.
func findFunction(meta *metadata.Metadata, pkg, name string) *metadata.Function {
	if meta == nil {
		return nil
	}
	p, ok := meta.Packages[pkg]
	if !ok {
		return nil
	}
	for _, file := range p.Files {
		if fn, ok := file.Functions[name]; ok {
			return fn
		}
	}
	return nil
}

// findConstructorEdge returns the call-graph edge from the helper (identified
// by helperBaseID) to the constructor (ctorPkg.ctorFunc), whose ParamArgMap
// carries the constructor's parameter names mapped to the helper's arguments.
func findConstructorEdge(meta *metadata.Metadata, helperBaseID, ctorPkg, ctorFunc string) *metadata.CallGraphEdge {
	for _, e := range meta.Callers[helperBaseID] {
		if meta.StringPool.GetString(e.Callee.Name) == ctorFunc &&
			meta.StringPool.GetString(e.Callee.Pkg) == ctorPkg {
			return e
		}
	}
	return nil
}

func paramNameSetFromEdge(edge *metadata.CallGraphEdge) map[string]bool {
	out := make(map[string]bool, len(edge.ParamArgMap))
	for name := range edge.ParamArgMap {
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func lookupWrapperType(meta *metadata.Metadata, goType string) *metadata.Type {
	if meta == nil || goType == "" {
		return nil
	}
	goType = strings.TrimPrefix(goType, "*")
	parts := TypeParts(goType)
	if parts.TypeName == "" {
		return nil
	}
	return typeByName(parts, meta)
}

// wrapperFieldIsGeneric reports whether the declared type of the named struct
// field on wrapperType is `interface{}` or `any` — i.e. the type system
// carries no concrete information and a per-route override is meaningful.
func wrapperFieldIsGeneric(meta *metadata.Metadata, wrapperType *metadata.Type, structFieldName string) bool {
	if wrapperType == nil {
		return false
	}
	for _, field := range wrapperType.Fields {
		if meta.StringPool.GetString(field.Name) != structFieldName {
			continue
		}
		declared := meta.StringPool.GetString(field.Type)
		declared = strings.TrimPrefix(declared, "*")
		return declared == "interface{}" || declared == "any"
	}
	return false
}

func jsonNameForField(meta *metadata.Metadata, wrapperType *metadata.Type, structFieldName string) string {
	if wrapperType == nil {
		return ""
	}
	for _, field := range wrapperType.Fields {
		if meta.StringPool.GetString(field.Name) != structFieldName {
			continue
		}
		tag := meta.StringPool.GetString(field.Tag)
		if name := extractJSONName(tag); name != "" {
			return name
		}
		return structFieldName
	}
	return ""
}
