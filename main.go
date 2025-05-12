package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed api_kinds_v1.json
var schemaKindsJSON []byte

type SchemaKindsRBAC struct {
	GroupVersion string   `json:"groupVersion"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Verbs        []string `json:"verbs"`
}

type Metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type RoleRule struct {
	APIGroups []string `yaml:"apiGroups"`
	Resources []string `yaml:"resources"`
	Verbs     []string `yaml:"verbs"`
}

type RoleDefinition struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   Metadata   `yaml:"metadata"`
	Rules      []RoleRule `yaml:"rules"`
}

func loadSchemaKindsRBAC(base []byte, extraPath string) ([]SchemaKindsRBAC, error) {
	var combined []SchemaKindsRBAC

	if err := json.Unmarshal(base, &combined); err != nil {
		return nil, err
	}

	if extraPath != "" {
		extraData, err := os.ReadFile(extraPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read extra kinds RBAC schema file: %v", err)
		}
		var extra []SchemaKindsRBAC
		if err := json.Unmarshal(extraData, &extra); err != nil {
			return nil, fmt.Errorf("failed to parse extra kinds RBAC schema file: %v", err)
		}
		combined = append(combined, extra...)
	}

	return combined, nil
}

func getVerbsForAccessType(accessType string, available []string) []string {
	switch accessType {
	case "read":
		return intersect(available, []string{"get", "list", "watch"})
	case "write":
		return intersect(available, []string{"create", "update", "patch", "delete"})
	case "admin":
		return available
	default:
		return []string{}
	}
}

func intersect(a, b []string) []string {
	set := make(map[string]struct{})
	for _, val := range a {
		set[val] = struct{}{}
	}
	var result []string
	for _, val := range b {
		if _, ok := set[val]; ok {
			result = append(result, val)
		}
	}
	return result
}

func adjustIndentation(yamlData []byte) string {
	lines := strings.Split(string(yamlData), "\n")
	for i, line := range lines {
		lines[i] = strings.ReplaceAll(line, "    ", "  ")
	}
	return strings.Join(lines, "\n")
}

func marshalYAMLWithIndent(v any) ([]byte, error) {
	out, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	return []byte(adjustIndentation(out)), nil
}

func main() {
	cluster := flag.Bool("cluster", false, "Generate ClusterRole instead of Role")
	access := flag.String("access", "read", "Access type: read, write, admin")
	name := flag.String("name", "access", "Metadata name for the Role/ClusterRole")
	namespace := flag.String("namespace", "", "Namespace for Role (ignored for ClusterRole)")
	extraSchemaPath := flag.String("extra-schema", "", "Path to extra kinds RBAC schema JSON file for custom resources")
	flag.Parse()

	sk, err := loadSchemaKindsRBAC(schemaKindsJSON, *extraSchemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load kinds RBAC schema data: %v\n", err)
		os.Exit(1)
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read stdin: %v\n", err)
		os.Exit(1)
	}

	docs := strings.Split(string(input), "---")
	rulesMap := map[string]RoleRule{}

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var manifest map[string]any
		if err := yaml.Unmarshal([]byte(doc), &manifest); err != nil {
			fmt.Fprintf(os.Stderr, "invalid YAML: %v\n", err)
			continue
		}

		apiVersionRaw, ok := manifest["apiVersion"].(string)
		if !ok {
			continue
		}
		apiGroup := strings.Split(apiVersionRaw, "/")[0]

		kindRaw, ok := manifest["kind"].(string)
		if !ok {
			continue
		}

		var pluralName string
		var verbs []string

		for _, entry := range sk {
			if entry.GroupVersion == apiGroup && entry.Kind == kindRaw {
				pluralName = entry.Name
				verbs = getVerbsForAccessType(*access, entry.Verbs)
				break
			}
		}

		if pluralName == "" {
			fmt.Fprintf(os.Stderr, "skipping unknown resource: %s %s\n", apiGroup, kindRaw)
			continue
		}

		group := apiGroup
		if group == "v1" {
			group = ""
		}

		key := fmt.Sprintf("%s|%s", group, pluralName)
		if rule, exists := rulesMap[key]; exists {
			// Merge verbs
			existingVerbs := make(map[string]bool)
			for _, v := range rule.Verbs {
				existingVerbs[v] = true
			}
			for _, v := range verbs {
				if !existingVerbs[v] {
					rule.Verbs = append(rule.Verbs, v)
				}
			}
			rulesMap[key] = rule
		} else {
			rulesMap[key] = RoleRule{
				APIGroups: []string{group},
				Resources: []string{pluralName},
				Verbs:     verbs,
			}
		}
	}

	var rules []RoleRule
	for _, r := range rulesMap {
		rules = append(rules, r)
	}

	roleKind := "Role"
	if *cluster {
		roleKind = "ClusterRole"
	}

	role := RoleDefinition{
		APIVersion: "rbac.authorization.k8s.io/v1",
		Kind:       roleKind,
		Metadata: Metadata{
			Name: *name,
		},
		Rules: rules,
	}

	if !*cluster && *namespace != "" {
		role.Metadata.Namespace = *namespace
	}

	out, err := marshalYAMLWithIndent(role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal role: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(out))
}
