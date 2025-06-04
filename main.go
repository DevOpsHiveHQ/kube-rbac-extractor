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

type schemaKindsRBAC struct {
	GroupVersion string   `json:"groupVersion"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Verbs        []string `json:"verbs"`
}

type metadata struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type roleRule struct {
	APIGroups     []string `yaml:"apiGroups"`
	Resources     []string `yaml:"resources"`
	ResourceNames []string `yaml:"resourceNames,omitempty"`
	Verbs         []string `yaml:"verbs"`
}

type roleDefinition struct {
	APIVersion string     `yaml:"apiVersion"`
	Kind       string     `yaml:"kind"`
	Metadata   metadata   `yaml:"metadata"`
	Rules      []roleRule `yaml:"rules"`
}

type roleBindingSubject struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace,omitempty"`
}

type roleBinding struct {
	APIVersion string               `yaml:"apiVersion"`
	Kind       string               `yaml:"kind"`
	Metadata   metadata             `yaml:"metadata"`
	Subjects   []roleBindingSubject `yaml:"subjects"`
	RoleRef    struct {
		APIGroup string `yaml:"apiGroup"`
		Kind     string `yaml:"kind"`
		Name     string `yaml:"name"`
	} `yaml:"roleRef"`
}

func loadSchemaKindsRBAC(base []byte, extraPath string) ([]schemaKindsRBAC, error) {
	var combined []schemaKindsRBAC

	if err := json.Unmarshal(base, &combined); err != nil {
		return nil, err
	}

	if extraPath != "" {
		extraData, err := os.ReadFile(extraPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read extra kinds RBAC schema file: %v", err)
		}
		var extra []schemaKindsRBAC
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

func parseRoleBindingSubjects(input string) ([]roleBindingSubject, error) {
	var subjects []roleBindingSubject
	entries := strings.Split(input, ",")
	for _, entry := range entries {
		roleBindingSubjectParts := strings.Split(entry, ":")
		if len(roleBindingSubjectParts) < 2 {
			return nil, fmt.Errorf("invalid subject format: %s", entry)
		}
		kind := roleBindingSubjectParts[0]
		switch kind {
		case "User", "Group":
			subjects = append(subjects, roleBindingSubject{
				Kind: kind,
				Name: roleBindingSubjectParts[1],
			})
		case "ServiceAccount":
			if len(roleBindingSubjectParts) != 3 {
				return nil, fmt.Errorf("ServiceAccount requires format ServiceAccount:namespace:name")
			}
			subjects = append(subjects, roleBindingSubject{
				Kind:      "ServiceAccount",
				Namespace: roleBindingSubjectParts[1],
				Name:      roleBindingSubjectParts[2],
			})
		default:
			return nil, fmt.Errorf("unknown subject kind: %s", kind)
		}
	}
	return subjects, nil
}

// extractManifestInfo extracts apiGroup, kind, resourceName from a manifest.
func extractManifestInfo(manifest map[string]any) (apiGroup, kind, resourceName string, ok bool) {
	apiVersionRaw, ok := manifest["apiVersion"].(string)
	faildReturn := func() (string, string, string, bool) {
		return "", "", "", false
	}

	if !ok {
		return faildReturn()
	}
	apiGroup = strings.Split(apiVersionRaw, "/")[0]

	kindRaw, ok := manifest["kind"].(string)
	if !ok {
		return faildReturn()
	}

	metadata, ok := manifest["metadata"].(map[string]any)
	if !ok {
		return faildReturn()
	}

	resourceName, _ = metadata["name"].(string)
	return apiGroup, kindRaw, resourceName, true
}

// findKindEntry finds the matching schemaKindsRBAC entry for a given group and kind.
func findKindEntry(sk []schemaKindsRBAC, apiGroup, kind string) (schemaKindsRBAC, bool) {
	for _, entry := range sk {
		if entry.GroupVersion == apiGroup && entry.Kind == kind {
			return entry, true
		}
	}
	return schemaKindsRBAC{}, false
}

// mergeVerbs merges new verbs into the rule's verbs, avoiding duplicates.
func mergeVerbs(rule *roleRule, verbs []string) {
	existingVerbs := make(map[string]bool)
	for _, v := range rule.Verbs {
		existingVerbs[v] = true
	}
	for _, v := range verbs {
		if !existingVerbs[v] {
			rule.Verbs = append(rule.Verbs, v)
		}
	}
}

func parseManifests(input string, sk []schemaKindsRBAC, access string, includeResourceNames bool) []roleRule {
	docs := strings.Split(input, "---")
	rulesMap := map[string]roleRule{}

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var manifest map[string]any
		if unmarshalErr := yaml.Unmarshal([]byte(doc), &manifest); unmarshalErr != nil {
			fmt.Fprintf(os.Stderr, "invalid YAML: %v\n", unmarshalErr)
			continue
		}

		apiGroup, kindRaw, resourceName, ok := extractManifestInfo(manifest)
		if !ok {
			continue
		}

		entry, found := findKindEntry(sk, apiGroup, kindRaw)
		if !found {
			fmt.Fprintf(os.Stderr, "skipping unknown resource: %s %s\n", apiGroup, kindRaw)
			continue
		}

		pluralName := entry.Name
		verbs := getVerbsForAccessType(access, entry.Verbs)

		group := apiGroup
		if group == "v1" {
			group = ""
		}

		key := fmt.Sprintf("%s|%s", group, pluralName)
		if rule, exists := rulesMap[key]; exists {
			mergeVerbs(&rule, verbs)
			rulesMap[key] = rule
		} else {
			rule := roleRule{
				APIGroups: []string{group},
				Resources: []string{pluralName},
				Verbs:     verbs,
			}
			if includeResourceNames && resourceName != "" {
				rule.ResourceNames = []string{resourceName}
			}
			rulesMap[key] = rule
		}
	}

	var rules []roleRule
	for _, r := range rulesMap {
		rules = append(rules, r)
	}
	return rules
}

func outputRoleAndBinding(cluster bool, name, namespace string, rules []roleRule, roleBindingSubjects string, roleKind string) {
	role := roleDefinition{
		APIVersion: "rbac.authorization.k8s.io/v1",
		Kind:       roleKind,
		Metadata: metadata{
			Name: name,
		},
		Rules: rules,
	}

	if !cluster && namespace != "" {
		role.Metadata.Namespace = namespace
	}

	out, err := marshalYAMLWithIndent(role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal role: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))

	if roleBindingSubjects != "" {
		subjects, err := parseRoleBindingSubjects(roleBindingSubjects)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid subjects: %v\n", err)
			os.Exit(1)
		}

		binding := roleBinding{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
			Metadata: metadata{
				Name: name + "-binding",
			},
			Subjects: subjects,
		}
		if cluster {
			binding.Kind = "ClusterRoleBinding"
		}
		if !cluster && namespace != "" {
			binding.Metadata.Namespace = namespace
		}
		binding.RoleRef = struct {
			APIGroup string `yaml:"apiGroup"`
			Kind     string `yaml:"kind"`
			Name     string `yaml:"name"`
		}{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     roleKind,
			Name:     name,
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

func main() {
	cluster := flag.Bool("cluster", false, "Generate ClusterRole instead of Role")
	access := flag.String("access", "read", "Access type: read, write, admin")
	name := flag.String("name", "access", "Metadata name for the Role/ClusterRole")
	namespace := flag.String("namespace", "", "Namespace for Role (ignored for ClusterRole)")
	extraSchemaPath := flag.String("extra-schema", "", "Path to extra kinds RBAC schema JSON file for custom resources")
	includeResourceNames := flag.Bool("resource-names", false, "Include resourceNames from manifest metadata.name in the rules")
	generateRoleBindingSubjects := flag.String("role-binding-subjects", "",
		"Generate RoleBinding/ClusterRoleBinding using comma-separated list of subjects to bind the role to (e.g., User:alice,Group:devs,ServiceAccount:ns:sa)")
	flag.Parse()

	schemaKinds, err := loadSchemaKindsRBAC(schemaKindsJSON, *extraSchemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load kinds RBAC schema data: %v\n", err)
		os.Exit(1)
	}

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read stdin: %v\n", err)
		os.Exit(1)
	}

	rules := parseManifests(string(input), schemaKinds, *access, *includeResourceNames)

	roleKind := "Role"
	if *cluster {
		roleKind = "ClusterRole"
	}

	outputRoleAndBinding(*cluster, *name, *namespace, rules, *generateRoleBindingSubjects, roleKind)
}
