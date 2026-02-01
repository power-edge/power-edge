package main

import (
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"gopkg.in/yaml.v3"
)

// Schema represents a JSON Schema with extensions
type Schema struct {
	Schema      string                 `yaml:"$schema"`
	ID          string                 `yaml:"$id"`
	Type        string                 `yaml:"type"`
	Description string                 `yaml:"description"`
	Properties  map[string]Property    `yaml:"properties"`
	Definitions map[string]Property    `yaml:"definitions"`
	Required    []string               `yaml:"required"`
	XGenerate   map[string]interface{} `yaml:",inline"`
}

// Property represents a schema property
type Property struct {
	Type        interface{}            `yaml:"type"`
	Description string                 `yaml:"description"`
	Ref         string                 `yaml:"$ref"`
	Enum        []string               `yaml:"enum"`
	Properties  map[string]Property    `yaml:"properties"`
	Items       *Property              `yaml:"items"`
	Required    []string               `yaml:"required"`
	Default     interface{}            `yaml:"default"`
	Pattern     string                 `yaml:"pattern"`
	Minimum     *int                   `yaml:"minimum"`
	Maximum     *int                   `yaml:"maximum"`
	MinLength   *int                   `yaml:"minLength"`
	MaxLength   *int                   `yaml:"maxLength"`

	// Code generation directives
	XGenerateStruct string                 `yaml:"x-generate-struct"`
	XGenerateField  string                 `yaml:"x-generate-field"`
	XGenerateEnum   string                 `yaml:"x-generate-enum"`
	XGenerateType   string                 `yaml:"x-generate-type"`
	XGenerateMap    string                 `yaml:"x-generate-map"`
	XGenerateConst  bool                   `yaml:"x-generate-const"`
	XRoot           bool                   `yaml:"x-root"`
	XChecker        map[string]interface{} `yaml:"x-checker"`
	XWatcher        map[string]interface{} `yaml:"x-watcher"`
}

// GeneratedType represents a Go type to generate
type GeneratedType struct {
	Name        string
	GoType      string
	IsStruct    bool
	IsEnum      bool
	Fields      []Field
	EnumValues  []string
	Description string
}

// Field represents a struct field
type Field struct {
	Name        string
	GoType      string
	JSONTag     string
	YAMLTag     string
	Description string
	Validations []string
}

func main() {
	schemaDir := flag.String("schema-dir", "./schemas", "Directory containing schema files")
	outputDir := flag.String("output-dir", "./pkg/config", "Output directory for generated code")
	flag.Parse()

	log.Printf("Reading schemas from: %s", *schemaDir)
	log.Printf("Generating code to: %s", *outputDir)

	// Read all schema files
	schemas := make(map[string]*Schema)
	schemaFiles, err := filepath.Glob(filepath.Join(*schemaDir, "*.schema.yaml"))
	if err != nil {
		log.Fatalf("Failed to glob schema files: %v", err)
	}

	for _, schemaFile := range schemaFiles {
		log.Printf("Parsing: %s", filepath.Base(schemaFile))
		data, err := os.ReadFile(schemaFile)
		if err != nil {
			log.Fatalf("Failed to read %s: %v", schemaFile, err)
		}

		var schema Schema
		if err := yaml.Unmarshal(data, &schema); err != nil {
			log.Fatalf("Failed to parse %s: %v", schemaFile, err)
		}

		basename := filepath.Base(schemaFile)
		schemas[basename] = &schema
	}

	// Generate types from schemas
	types := extractTypes(schemas)

	// Generate Go code
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output dir: %v", err)
	}

	if err := generateGoCode(types, filepath.Join(*outputDir, "generated.go")); err != nil {
		log.Fatalf("Failed to generate code: %v", err)
	}

	log.Printf("✅ Successfully generated %d types", len(types))
}

func extractTypes(schemas map[string]*Schema) []GeneratedType {
	var types []GeneratedType
	seen := make(map[string]bool) // Track seen type names to avoid duplicates

	for schemaName, schema := range schemas {
		log.Printf("Extracting types from: %s", schemaName)

		// Process definitions
		for defName, defProp := range schema.Definitions {
			if defProp.XGenerateEnum != "" {
				if !seen[defProp.XGenerateEnum] {
					types = append(types, GeneratedType{
						Name:        defProp.XGenerateEnum,
						IsEnum:      true,
						EnumValues:  defProp.Enum,
						Description: defProp.Description,
					})
					seen[defProp.XGenerateEnum] = true
				}
			} else if defProp.XGenerateType != "" {
				if !seen[defProp.XGenerateType] {
					types = append(types, GeneratedType{
						Name:        defProp.XGenerateType,
						GoType:      mapSchemaTypeToGo(defProp),
						Description: defProp.Description,
					})
					seen[defProp.XGenerateType] = true
				}
			} else if defProp.XGenerateStruct != "" {
				// Handle struct definitions
				if !seen[defProp.XGenerateStruct] {
					types = append(types, extractStruct(defProp.XGenerateStruct, defProp.Properties, defProp.Required, defProp.Description))
					seen[defProp.XGenerateStruct] = true
				}
				// Recursively extract nested types
				for _, t := range extractNestedStructs(defProp.Properties, defProp.Required) {
					if !seen[t.Name] {
						types = append(types, t)
						seen[t.Name] = true
					}
				}
			} else if defProp.XGenerateConst {
				// Generate type alias for const definitions (like version)
				typeName := toGoName(defName)
				if !seen[typeName] {
					types = append(types, GeneratedType{
						Name:        typeName,
						GoType:      mapSchemaTypeToGo(defProp),
						Description: defProp.Description,
					})
					seen[typeName] = true
				}
			}
		}

		// Process root schema
		if schema.XGenerate != nil {
			if structName, ok := schema.XGenerate["x-generate-struct"].(string); ok && structName != "" {
				if !seen[structName] {
					types = append(types, extractStruct(structName, schema.Properties, schema.Required, schema.Description))
					seen[structName] = true
				}
			}
		}

		// Process properties for nested structs
		for _, t := range extractNestedStructs(schema.Properties, schema.Required) {
			if !seen[t.Name] {
				types = append(types, t)
				seen[t.Name] = true
			}
		}
	}

	return types
}

func extractStruct(name string, properties map[string]Property, required []string, description string) GeneratedType {
	var fields []Field

	for propName, prop := range properties {
		fieldName := prop.XGenerateField
		if fieldName == "" {
			fieldName = toGoName(propName)
		}

		fields = append(fields, Field{
			Name:        fieldName,
			GoType:      inferGoType(prop),
			JSONTag:     propName,
			YAMLTag:     propName,
			Description: prop.Description,
		})
	}

	return GeneratedType{
		Name:        name,
		IsStruct:    true,
		Fields:      fields,
		Description: description,
	}
}

func extractNestedStructs(properties map[string]Property, required []string) []GeneratedType {
	var types []GeneratedType

	for _, prop := range properties {
		if prop.XGenerateStruct != "" {
			types = append(types, extractStruct(prop.XGenerateStruct, prop.Properties, prop.Required, prop.Description))
			// Recursively extract nested structs
			types = append(types, extractNestedStructs(prop.Properties, prop.Required)...)
		}

		// Handle inline enums without explicit x-generate-enum
		if prop.XGenerateEnum != "" && len(prop.Enum) > 0 {
			types = append(types, GeneratedType{
				Name:        prop.XGenerateEnum,
				IsEnum:      true,
				EnumValues:  prop.Enum,
				Description: prop.Description,
			})
		} else if len(prop.Enum) > 0 && prop.XGenerateField != "" {
			// Auto-generate enum type name from field name
			enumTypeName := prop.XGenerateField + "Enum"
			types = append(types, GeneratedType{
				Name:        enumTypeName,
				IsEnum:      true,
				EnumValues:  prop.Enum,
				Description: prop.Description,
			})
		}

		// Handle arrays of structs
		if prop.Items != nil && prop.Items.XGenerateStruct != "" {
			types = append(types, extractStruct(prop.Items.XGenerateStruct, prop.Items.Properties, prop.Items.Required, prop.Items.Description))
			types = append(types, extractNestedStructs(prop.Items.Properties, prop.Items.Required)...)
		}
	}

	return types
}

func inferGoType(prop Property) string {
	// Check for explicit type override
	if prop.XGenerateType != "" {
		return prop.XGenerateType
	}

	if prop.XGenerateStruct != "" {
		return prop.XGenerateStruct
	}

	if prop.XGenerateEnum != "" {
		return prop.XGenerateEnum
	}

	if prop.XGenerateMap != "" {
		return prop.XGenerateMap
	}

	// Handle $ref
	if prop.Ref != "" {
		parts := strings.Split(prop.Ref, "/")
		refName := parts[len(parts)-1]
		return toGoName(refName)
	}

	return mapSchemaTypeToGo(prop)
}

func mapSchemaTypeToGo(prop Property) string {
	typeStr := ""
	if s, ok := prop.Type.(string); ok {
		typeStr = s
	}

	switch typeStr {
	case "string":
		if len(prop.Enum) > 0 {
			// This should have x-generate-enum
			return "string"
		}
		return "string"
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "array":
		if prop.Items != nil {
			itemType := inferGoType(*prop.Items)
			return "[]" + itemType
		}
		return "[]interface{}"
	case "object":
		if prop.Properties != nil && len(prop.Properties) > 0 {
			// Should have x-generate-struct
			return "map[string]interface{}"
		}
		return "map[string]interface{}"
	default:
		return "interface{}"
	}
}

func toGoName(name string) string {
	// Convert snake_case or kebab-case to PascalCase
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-'
	})

	for i, part := range parts {
		parts[i] = toPascalCase(part)
	}

	return strings.Join(parts, "")
}

// toPascalCase converts a string to PascalCase (proper Go exported name)
func toPascalCase(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func generateGoCode(types []GeneratedType, outputFile string) error {
	tmpl := template.Must(template.New("go").Funcs(template.FuncMap{
		"quote":   func(s string) string { return fmt.Sprintf("%q", s) },
		"goIdent": toGoName,
		"formatDoc": formatDocComment,
	}).Parse(goTemplate))

	// Generate to buffer first
	var buf strings.Builder
	if err := tmpl.Execute(&buf, map[string]interface{}{
		"Types": types,
	}); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	// Format with gofmt
	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		// If formatting fails, write unformatted code for debugging
		log.Printf("Warning: gofmt failed: %v", err)
		formatted = []byte(buf.String())
	}

	// Write to file
	if err := os.WriteFile(outputFile, formatted, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// formatDocComment formats a description as a proper Go doc comment
func formatDocComment(typeName, description string) string {
	if description == "" {
		return fmt.Sprintf("// %s represents a generated type.", typeName)
	}
	// Ensure doc comment starts with type name (Go convention)
	if !strings.HasPrefix(description, typeName) {
		return fmt.Sprintf("// %s %s", typeName, description)
	}
	return fmt.Sprintf("// %s", description)
}

const goTemplate = `// Code generated by schema generator. DO NOT EDIT.

// Package config provides generated configuration types from JSON schemas.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

{{range .Types}}
{{if .IsEnum}}
{{formatDoc .Name .Description}}
type {{.Name}} string

const (
{{$typeName := .Name}}{{range $i, $val := .EnumValues}}	{{$typeName}}{{$val | goIdent}} {{$typeName}} = {{$val | quote}}
{{end}})

{{else if .IsStruct}}
{{formatDoc .Name .Description}}
type {{.Name}} struct {
{{range .Fields}}	{{.Name}} {{.GoType}} ` + "`json:\"{{.JSONTag}}\" yaml:\"{{.YAMLTag}}\"`" + ` // {{.Description}}
{{end}}}

{{else}}
{{formatDoc .Name .Description}}
type {{.Name}} {{.GoType}}

{{end}}
{{end}}

// LoadStateConfig loads state configuration from YAML file
func LoadStateConfig(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var config State
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &config, nil
}

// LoadWatcherConfig loads watcher configuration from YAML file
func LoadWatcherConfig(path string) (*WatcherConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var config WatcherConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &config, nil
}
`
