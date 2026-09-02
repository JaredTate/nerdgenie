package provider

import "github.com/JaredTate/coeus/internal/contract"

// jsonSchemaTypes are the type words a tool field may use. Anything else is sent
// as a string, because a model that is told the wrong type writes the wrong
// value, and a string is the one shape every tool can read back.
var jsonSchemaTypes = []string{"string", "integer", "number", "boolean", "array", "object"}

// jsonSchema is the object shape both APIs want a tool's inputs described in.
type jsonSchema struct {
	// Type is always "object", because a tool's inputs are named fields.
	Type string `json:"type"`
	// Properties is one entry per input field.
	Properties map[string]schemaField `json:"properties"`
	// Required names the fields the tool cannot run without.
	Required []string `json:"required"`
}

// schemaField is one input of a tool as the schema describes it.
type schemaField struct {
	// Type is the schema's word for the kind of value the field takes.
	Type string `json:"type"`
	// Description says what to put in the field.
	Description string `json:"description,omitempty"`
}

// schemaForFields turns a tool's input fields into the JSON Schema object both
// wire protocols send.
func schemaForFields(fields []contract.ToolField) jsonSchema {
	schema := jsonSchema{Type: "object", Properties: map[string]schemaField{}, Required: []string{}}
	for _, field := range fields {
		schema.Properties[field.Name] = schemaField{
			Type:        schemaTypeFor(field.Type),
			Description: field.Description,
		}
		if field.Required {
			schema.Required = append(schema.Required, field.Name)
		}
	}
	return schema
}

// schemaTypeFor maps a field's plain-words type onto the schema's vocabulary.
func schemaTypeFor(written string) string {
	for _, known := range jsonSchemaTypes {
		if written == known {
			return known
		}
	}
	return "string"
}
