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
	APIGroups     []string `yaml:"apiGroups"`
	Resources     []string `yaml:"resources"`
	ResourceNames []string `yaml:"resourceNames,omitempty"`
	Verbs         []string `yaml:"verbs"`
}

type RoleDefinition struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   Metadata   `yaml:"metadata"`
	Rules      []RoleRule `yaml:"rules"`
}

type Subject struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type RoleBinding struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Subjects   []Subject `yaml:"subjects"`
	RoleRef    struct {
		APIGroup string `yaml:"apiGroup"`
		Kind     string `yaml:"kind"`
		Name     string `yaml:"name"`
	} `yaml:"roleRef"`
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

func parseRoleBindingSubjects(input string) ([]Subject, error) {
	var subjects []Subject
	entries := strings.Split(input, ",")
	for _, entry := range entries {
		roleBindingSubject := strings.Split(entry, ":")
		if len(roleBindingSubject) < 2 {
			return nil, fmt.Errorf("invalid subject format: %s", entry)
		}
		kind := roleBindingSubject[0]
		switch kind {
		case "User", "Group":
			subjects = append(subjects, Subject{
				Kind: kind,
				Name: roleBindingSubject[1],
			})
		case "ServiceAccount":
			if len(roleBindingSubject) != 3 {
				return nil, fmt.Errorf("ServiceAccount requires format ServiceAccount:namespace:name")
			}
			subjects = append(subjects, Subject{
				Kind:      "ServiceAccount",
				Namespace: roleBindingSubject[1],
				Name:      roleBindingSubject[2],
			})
		default:
			return nil, fmt.Errorf("unknown subject kind: %s", kind)
		}
	}
	return subjects, nil
}

func main() {
	cluster := flag.Bool("cluster", false, "Generate ClusterRole instead of Role")
	access := flag.String("access", "read", "Access type: read, write, admin")
	name := flag.String("name", "access", "Metadata name for the Role/ClusterRole")
	namespace := flag.String("namespace", "", "Namespace for Role (ignored for ClusterRole)")
	extraSchemaPath := flag.String("extra-schema", "", "Path to extra kinds RBAC schema JSON file for custom resources")
	includeResourceNames := flag.Bool("resource-names", false, "Include resourceNames from manifest metadata.name in the rules")
	roleBindingSubjects := flag.String("role-binding-subjects", "", "Comma-separated list of subjects to bind the role to (e.g., User:alice,Group:devs,ServiceAccount:ns:sa)")
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

		metadata, ok := manifest["metadata"].(map[string]any)
		if !ok {
			continue
		}

		var resourceName string
		if nameVal, ok := metadata["name"].(string); ok {
			resourceName = nameVal
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
			rule := RoleRule{
				APIGroups: []string{group},
				Resources: []string{pluralName},
				Verbs:     verbs,
			}
			if *includeResourceNames && resourceName != "" {
				rule.ResourceNames = []string{resourceName}
			}
			rulesMap[key] = rule
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

	if *roleBindingSubjects != "" {
		subjects, err := parseRoleBindingSubjects(*roleBindingSubjects)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid subjects: %v\n", err)
			os.Exit(1)
		}

		binding := RoleBinding{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
			Metadata: Metadata{
				Name: *name + "-binding",
			},
			Subjects: subjects,
		}
		if *cluster {
			binding.Kind = "ClusterRoleBinding"
		}
		if !*cluster && *namespace != "" {
			binding.Metadata.Namespace = *namespace
		}
		binding.RoleRef = struct {
			APIGroup string `yaml:"apiGroup"`
			Kind     string `yaml:"kind"`
			Name     string `yaml:"name"`
		}{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     roleKind,
			Name:     *name,
		}

		bindOut, err := marshalYAMLWithIndent(binding)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal binding: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("---")
		fmt.Println(string(bindOut))
	}
}
