package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"datascape.dev/platformctl/internal/artifact"
	"datascape.dev/platformctl/internal/binding"
	"datascape.dev/platformctl/internal/canonical"
	"datascape.dev/platformctl/internal/compiler"
	"datascape.dev/platformctl/internal/conformance"
	"datascape.dev/platformctl/internal/diff"
	"datascape.dev/platformctl/internal/docsgen"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/provider"
	"datascape.dev/platformctl/internal/recovery"
	"datascape.dev/platformctl/internal/resource"
	"datascape.dev/platformctl/internal/spec"
	"gopkg.in/yaml.v3"
)

var (
	version        = compiler.DefaultVersion
	sourceCommit   = ""
	compilerDigest = ""
)

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "init":
		return cmdInit(ctx, args[1:])
	case "validate":
		return cmdValidate(ctx, args[1:])
	case "plan":
		return cmdPlan(ctx, args[1:])
	case "generate":
		return cmdGenerate(ctx, args[1:])
	case "diff":
		return cmdDiff(ctx, args[1:])
	case "inspect":
		return cmdInspect(ctx, args[1:])
	case "api-resources":
		return cmdAPIResources(ctx, args[1:])
	case "api-definitions":
		return cmdAPIDefinitions(ctx, args[1:])
	case "providers":
		return cmdProviders(ctx, args[1:])
	case "bindings":
		return cmdBindings(ctx, args[1:])
	case "explain":
		return cmdExplain(ctx, args[1:])
	case "migrate":
		return cmdMigrate(ctx, args[1:])
	case "docs":
		return cmdDocs(ctx, args[1:])
	case "secrets":
		return cmdSecrets(ctx, args[1:])
	case "verify":
		return cmdVerify(ctx, args[1:])
	case "conformance":
		return cmdConformance(ctx, args[1:])
	case "recover":
		return cmdRecover(ctx, args[1:])
	case "version":
		return cmdVersion(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func cmdInit(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	output := flags.String("output", ".", "project directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	files := []artifact.File{
		{Path: "platform.yaml", Mode: 0o644, Content: []byte(initPlatform()), Deterministic: true},
		{Path: "profiles/local.yaml", Mode: 0o644, Content: []byte(initProfile("local", "compose")), Deterministic: true},
		{Path: "profiles/kubernetes.yaml", Mode: 0o644, Content: []byte(initProfile("kubernetes", "kubernetes")), Deterministic: true},
		{Path: "schemas/README.md", Mode: 0o644, Content: []byte("# Schemas\n\nGit is authoritative for schemas. Runtime registries are generated projections.\n"), Deterministic: true},
		{Path: "policies/README.md", Mode: 0o644, Content: []byte("# Policies\n\nDeclare compatibility, retention, lineage, access, verification, and recovery policies here.\n"), Deterministic: true},
		{Path: "docs/index.md", Mode: 0o644, Content: []byte("# Datascape Platform Project\n\nGenerated skeleton for `platformctl`.\n"), Deterministic: true},
		{Path: "examples/README.md", Mode: 0o644, Content: []byte("# Examples\n\nAdd portable platform examples here.\n"), Deterministic: true},
		{Path: ".gitignore", Mode: 0o644, Content: []byte("dist/\n*.tmp\n"), Deterministic: true},
		{Path: "Makefile", Mode: 0o644, Content: []byte("PLATFORMCTL ?= platformctl\n\nvalidate:\n\t$(PLATFORMCTL) validate --platform platform.yaml --profile profiles/local.yaml\n\ngenerate:\n\t$(PLATFORMCTL) generate --platform platform.yaml --profile profiles/local.yaml --output dist/local\n"), Deterministic: true},
	}
	schema, err := compiler.SchemaFile()
	if err != nil {
		return err
	}
	files = append(files, schema)
	return writeNonOverwriting(ctx, *output, artifact.Normalize(files))
}

func cmdValidate(ctx context.Context, args []string) error {
	flags, docs, err := compileInputs(args)
	if err != nil {
		return err
	}
	result := compiler.CompileDocuments(ctx, docs, compileOptions(flags.target))
	printDiagnostics(result.Diagnostics)
	if domain.HasErrors(result.Diagnostics) {
		return errors.New("validation failed")
	}
	fmt.Println("valid")
	return nil
}

func cmdPlan(ctx context.Context, args []string) error {
	flags, docs, err := compileInputs(args)
	if err != nil {
		return err
	}
	result := compiler.CompileDocuments(ctx, docs, compileOptions(flags.target))
	if err := failOnDiagnostics(result.Diagnostics); err != nil {
		return err
	}
	if flags.format == "json" {
		return printCanonical(result.Actions)
	}
	fmt.Print(compiler.FormatActions(result.Actions))
	return nil
}

func cmdGenerate(ctx context.Context, args []string) error {
	flags, docs, err := compileInputs(args)
	if err != nil {
		return err
	}
	result := compiler.CompileDocuments(ctx, docs, compileOptions(flags.target))
	if err := failOnDiagnostics(result.Diagnostics); err != nil {
		return err
	}
	output := flags.output
	if output == "" {
		output = filepath.Join("dist", result.Plan.Target)
	}
	if err := artifact.WriteFiles(ctx, output, result.Files); err != nil {
		return err
	}
	fmt.Printf("generated %d files in %s\n", len(result.Files), output)
	fmt.Printf("bundle digest: %s\n", result.Provenance.BundleDigest)
	return nil
}

func cmdDiff(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("diff", flag.ContinueOnError)
	from := flags.String("from", "", "previous platform spec")
	to := flags.String("to", "", "current platform spec")
	target := flags.String("target", "", "target override")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *from == "" || *to == "" {
		return errors.New("diff requires --from and --to")
	}
	beforeDocs, err := readNamedDocuments([]string{*from})
	if err != nil {
		return err
	}
	afterDocs, err := readNamedDocuments([]string{*to})
	if err != nil {
		return err
	}
	before := compiler.CompileDocuments(ctx, beforeDocs, compileOptions(*target))
	if err := failOnDiagnostics(before.Diagnostics); err != nil {
		return err
	}
	after := compiler.CompileDocuments(ctx, afterDocs, compileOptions(*target))
	if err := failOnDiagnostics(after.Diagnostics); err != nil {
		return err
	}
	return printCanonical(diff.Plans(before.Plan, after.Plan))
}

func cmdInspect(ctx context.Context, args []string) error {
	flags, docs, err := compileInputs(args)
	if err != nil {
		return err
	}
	result := compiler.CompileDocuments(ctx, docs, compileOptions(flags.target))
	if err := failOnDiagnostics(result.Diagnostics); err != nil {
		return err
	}
	if flags.resource == "" {
		return printCanonical(map[string]any{
			"resources":         result.Resources,
			"definitions":       result.Plan.Definitions,
			"providerInstances": result.Plan.ProviderInstances,
			"bindings":          result.Plan.Bindings,
			"graph":             result.Plan.ResourceGraph,
			"plannedResources":  result.Plan.PlannedResources,
			"artifacts":         artifact.BuildManifest(result.Files, result.BundleDigest).Files,
		})
	}
	for _, resource := range result.Resources {
		if strings.Contains(resource.Identity.Display(), flags.resource) || strings.Contains(resource.Identity.CanonicalString(), flags.resource) {
			return printCanonical(resource)
		}
	}
	return fmt.Errorf("resource %q not found", flags.resource)
}

func cmdAPIResources(ctx context.Context, args []string) error {
	docs, target, err := optionalRegistryInputs(args)
	if err != nil {
		return err
	}
	resources, diags := spec.ParseDocuments(ctx, docs)
	printDiagnostics(diags)
	if domain.HasErrors(diags) {
		return errors.New("api-resources failed")
	}
	registry, regDiags := resource.BuildRegistry(resources)
	printDiagnostics(regDiags)
	if domain.HasErrors(regDiags) {
		return errors.New("api-resources failed")
	}
	_ = target
	defs := registry.Definitions()
	out := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		out = append(out, map[string]any{
			"apiVersion": def.APIVersion,
			"kind":       def.Kind,
			"category":   def.Category,
			"extension":  def.Extension,
		})
	}
	return printCanonical(out)
}

func cmdAPIDefinitions(ctx context.Context, args []string) error {
	docs, _, err := optionalRegistryInputs(args)
	if err != nil {
		return err
	}
	resources, diags := spec.ParseDocuments(ctx, docs)
	printDiagnostics(diags)
	if domain.HasErrors(diags) {
		return errors.New("api-definitions failed")
	}
	registry, regDiags := resource.BuildRegistry(resources)
	printDiagnostics(regDiags)
	if domain.HasErrors(regDiags) {
		return errors.New("api-definitions failed")
	}
	return printCanonical(registry.Definitions())
}

func cmdProviders(ctx context.Context, args []string) error {
	docs, target, err := optionalRegistryInputs(args)
	if err != nil {
		return err
	}
	resources, diags := spec.ParseDocuments(ctx, docs)
	printDiagnostics(diags)
	if domain.HasErrors(diags) {
		return errors.New("providers failed")
	}
	registry, regDiags := provider.BuildRegistry(resources, target)
	printDiagnostics(regDiags)
	if domain.HasErrors(regDiags) {
		return errors.New("providers failed")
	}
	return printCanonical(map[string]any{"providers": registry.Descriptors(), "instances": registry.Instances()})
}

func cmdBindings(ctx context.Context, args []string) error {
	docs, _, err := optionalRegistryInputs(args)
	if err != nil {
		return err
	}
	resources, diags := spec.ParseDocuments(ctx, docs)
	printDiagnostics(diags)
	if domain.HasErrors(diags) {
		return errors.New("bindings failed")
	}
	registry, regDiags := binding.BuildRegistry(resources)
	printDiagnostics(regDiags)
	if domain.HasErrors(regDiags) {
		return errors.New("bindings failed")
	}
	return printCanonical(registry.Definitions())
}

func cmdExplain(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("explain requires a kind or apiVersion/kind")
	}
	docs, _, err := optionalRegistryInputs(args[1:])
	if err != nil {
		return err
	}
	resources, diags := spec.ParseDocuments(ctx, docs)
	printDiagnostics(diags)
	if domain.HasErrors(diags) {
		return errors.New("explain failed")
	}
	registry, regDiags := resource.BuildRegistry(resources)
	printDiagnostics(regDiags)
	if domain.HasErrors(regDiags) {
		return errors.New("explain failed")
	}
	query := args[0]
	for _, def := range registry.Definitions() {
		if def.Kind == query || def.APIVersion+"/"+def.Kind == query {
			return printCanonical(def)
		}
	}
	return fmt.Errorf("kind %q is not registered", query)
}

func cmdMigrate(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	output := flags.String("output", "", "optional migrated YAML output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if len(flags.Args()) == 0 {
		return errors.New("migrate requires one or more manifest paths")
	}
	docs, err := readNamedDocuments(flags.Args())
	if err != nil {
		return err
	}
	resources, diags := spec.ParseDocuments(ctx, docs)
	printDiagnostics(diags)
	if domain.HasErrors(diags) {
		return errors.New("migrate failed")
	}
	migrated := make([]map[string]any, 0, len(resources))
	for _, res := range resources {
		items := migrateResource(res)
		migrated = append(migrated, items...)
	}
	content, err := yaml.Marshal(map[string]any{"items": migrated})
	if err != nil {
		return err
	}
	if *output != "" {
		return writeOne(ctx, *output, content)
	}
	_, err = os.Stdout.Write(content)
	return err
}

func cmdDocs(ctx context.Context, args []string) error {
	if len(args) > 0 && args[0] == "serve" {
		flags := flag.NewFlagSet("docs serve", flag.ContinueOnError)
		directory := flags.String("directory", "dist/docs", "built documentation directory")
		listen := flags.String("listen", "127.0.0.1:8000", "HTTP listen address")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(*directory, "index.html")); err != nil {
			return fmt.Errorf("documentation site is not built: %w", err)
		}
		server := &http.Server{Addr: *listen, Handler: http.FileServer(http.Dir(*directory)), ReadHeaderTimeout: 5 * time.Second}
		fmt.Printf("serving documentation from %s at http://%s\n", *directory, *listen)
		return server.ListenAndServe()
	}
	if len(args) > 0 && args[0] == "build" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("docs", flag.ContinueOnError)
	source := flags.String("source", "docs", "documentation source directory")
	output := flags.String("output", "dist/docs", "documentation output directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	files, err := docsgen.BuildSite(ctx, *source)
	if err != nil {
		return err
	}
	return artifact.WriteFiles(ctx, *output, files)
}

func cmdSecrets(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "init" {
		return errors.New("secrets requires the init subcommand")
	}
	flags := flag.NewFlagSet("secrets init", flag.ContinueOnError)
	bundle := flags.String("bundle", "", "generated bundle directory")
	development := flags.Bool("development", false, "allow generation of local development secret values")
	force := flags.Bool("force", false, "replace an existing .env file")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if !*development {
		return errors.New("automatic secret generation is restricted to --development")
	}
	if *bundle == "" {
		return errors.New("--bundle is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	examplePath, outputPath := filepath.Join(*bundle, ".env.example"), filepath.Join(*bundle, ".env")
	if info, err := os.Lstat(outputPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to write development secrets through symlink %s", outputPath)
		}
		if !*force {
			return fmt.Errorf("%s already exists; use --force to replace it", outputPath)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	content, err := os.ReadFile(examplePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	for index, line := range lines {
		key, _, ok := strings.Cut(line, "=")
		if !ok || key == "" {
			continue
		}
		value := "datascape"
		if !strings.Contains(strings.ToLower(key), "username") && !strings.HasSuffix(strings.ToLower(key), "user") {
			secret := make([]byte, 24)
			if _, err := rand.Read(secret); err != nil {
				return err
			}
			value = base64.RawURLEncoding.EncodeToString(secret)
		}
		lines[index] = key + "=" + value
	}
	if err := os.WriteFile(outputPath, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		return err
	}
	fmt.Printf("created development secrets at %s\n", outputPath)
	return nil
}

func cmdVerify(ctx context.Context, args []string) error {
	verifyFlags := flag.NewFlagSet("verify", flag.ContinueOnError)
	bundle := verifyFlags.String("bundle", "", "generated bundle directory")
	runtime := verifyFlags.Bool("runtime", false, "run runtime-oriented bundle checks")
	if err := verifyFlags.Parse(args); err != nil {
		return err
	}
	if *bundle != "" {
		return verifyBundle(ctx, *bundle, *runtime)
	}
	flags, docs, err := compileInputs(args)
	if err != nil {
		return err
	}
	result := compiler.CompileDocuments(ctx, docs, compileOptions(flags.target))
	printDiagnostics(result.Diagnostics)
	if domain.HasErrors(result.Diagnostics) {
		return errors.New("verification failed")
	}
	fmt.Printf("static verification passed\nbundle digest: %s\n", result.Provenance.BundleDigest)
	return nil
}

func verifyBundle(ctx context.Context, bundle string, runtime bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	checkPath := filepath.Join(bundle, "verification", "checks.json")
	content, err := os.ReadFile(checkPath)
	if err != nil {
		return err
	}
	var checks []map[string]any
	if err := json.Unmarshal(content, &checks); err != nil {
		return err
	}
	results := make([]map[string]any, 0, len(checks))
	failed := false
	runtimeMessage := ""
	var runtimeErr error
	if runtime {
		runtimeMessage, runtimeErr = verifyComposeRuntime(ctx, bundle)
	}
	for _, check := range checks {
		id, _ := check["id"].(string)
		description, _ := check["message"].(string)
		status := "pass"
		message := description
		remediation := ""
		switch id {
		case "COMPOSE-001":
			if _, err := os.Stat(filepath.Join(bundle, "compose.yaml")); err != nil {
				status = "fail"
				message = "compose.yaml is missing"
				remediation = "regenerate the bundle"
			}
			if runtime && runtimeErr != nil {
				status = "fail"
				message = runtimeErr.Error()
				remediation = "install Docker Compose, start the generated bundle, and inspect unhealthy services"
			}
		case "BINDING-001", "SOURCE-001", "CDC-001", "EVENTSTREAM-001", "ARCHIVE-001", "LINEAGE-001", "RECOVERY-001":
			message = description
			if runtime {
				if runtimeErr != nil {
					status = "fail"
					message = runtimeErr.Error()
					remediation = "start the generated Compose bundle and resolve unhealthy services"
				} else {
					message = runtimeMessage + "; " + description
				}
			}
		default:
			if !runtime {
				status = "not-run"
				message = "runtime check requires --runtime"
				remediation = "start the bundle and rerun with --runtime"
			} else if runtimeErr != nil {
				status = "fail"
				message = runtimeErr.Error()
				remediation = "start the generated Compose bundle and rerun runtime verification"
			} else {
				message = runtimeMessage + "; " + description
			}
		}
		if status == "fail" {
			failed = true
		}
		results = append(results, map[string]any{
			"id":          id,
			"status":      status,
			"message":     message,
			"evidenceRef": filepath.ToSlash(filepath.Join("execution-evidence", id+".json")),
			"duration":    "0s",
			"remediation": remediation,
		})
	}
	if err := printCanonical(results); err != nil {
		return err
	}
	if failed {
		return errors.New("bundle verification failed")
	}
	return nil
}

func verifyComposeRuntime(ctx context.Context, bundle string) (string, error) {
	if _, err := exec.LookPath("docker"); err != nil {
		return "", fmt.Errorf("Docker CLI is required for runtime verification: %w", err)
	}
	args := []string{"compose", "--env-file", ".env", "-f", "compose.yaml", "ps", "--all", "--format", "json"}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = bundle
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("Compose runtime inspection failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	services, err := parseComposePS(output)
	if err != nil {
		return "", fmt.Errorf("could not parse Compose runtime state: %w", err)
	}
	if len(services) == 0 {
		return "", errors.New("Compose runtime has no running services")
	}
	return summarizeComposePS(services)
}

type composePS struct {
	Name     string `json:"Name"`
	Service  string `json:"Service"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
}

func parseComposePS(output []byte) ([]composePS, error) {
	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return nil, nil
	}
	var services []composePS
	if err := json.Unmarshal([]byte(trimmed), &services); err == nil {
		return services, nil
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	for scanner.Scan() {
		var service composePS
		if err := json.Unmarshal(scanner.Bytes(), &service); err != nil {
			return nil, err
		}
		services = append(services, service)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return services, nil
}

func summarizeComposePS(services []composePS) (string, error) {
	running, completed := 0, 0
	for _, service := range services {
		name := service.Service
		if name == "" {
			name = service.Name
		}
		switch strings.ToLower(service.State) {
		case "running":
			if health := strings.ToLower(service.Health); health != "" && health != "healthy" {
				return "", fmt.Errorf("Compose service %s is running with health %s", name, service.Health)
			}
			running++
		case "exited":
			if service.ExitCode != 0 {
				return "", fmt.Errorf("Compose service %s exited with code %d", name, service.ExitCode)
			}
			completed++
		default:
			return "", fmt.Errorf("Compose service %s is %s", name, service.State)
		}
	}
	return fmt.Sprintf("Compose reports %d running services and %d successful completion jobs", running, completed), nil
}

func composeImagesPinned(path string) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "image: ") {
			continue
		}
		image := strings.TrimSpace(strings.TrimPrefix(trimmed, "image: "))
		if !strings.Contains(image, "@sha256:") {
			return false, nil
		}
	}
	return true, nil
}

func cmdConformance(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("conformance", flag.ContinueOnError)
	output := flags.String("output", "", "optional output file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	content, err := canonical.JSON(conformance.FoundationSuites())
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if *output == "" {
		_, err = os.Stdout.Write(content)
		return err
	}
	return writeOne(ctx, *output, content)
}

func cmdRecover(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("recover requires subcommand: plan or generate")
	}
	switch args[0] {
	case "plan":
		content, err := canonical.JSON(recovery.FoundationPlan())
		if err != nil {
			return err
		}
		fmt.Println(string(content))
		return nil
	case "generate":
		flags := flag.NewFlagSet("recover generate", flag.ContinueOnError)
		output := flags.String("output", "dist/recovery", "recovery output directory")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		content, err := canonical.JSON(recovery.FoundationPlan())
		if err != nil {
			return err
		}
		files := []artifact.File{{Path: "recovery-plan.json", Mode: 0o644, Content: append(content, '\n'), Deterministic: true}}
		return artifact.WriteFiles(ctx, *output, files)
	default:
		return fmt.Errorf("unknown recover subcommand %q", args[0])
	}
}

func cmdVersion(args []string) error {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	jsonOut := flags.Bool("json", false, "print JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	record := map[string]string{
		"version":        version,
		"sourceCommit":   sourceCommit,
		"compilerDigest": compilerDigest,
	}
	if *jsonOut {
		return printCanonical(record)
	}
	fmt.Printf("platformctl %s\n", version)
	if sourceCommit != "" {
		fmt.Printf("source commit: %s\n", sourceCommit)
	}
	return nil
}

type inputFlags struct {
	platform  string
	profile   string
	catalogue string
	output    string
	target    string
	format    string
	resource  string
	args      []string
}

func compileInputs(args []string) (inputFlags, []spec.NamedDocument, error) {
	flags := flag.NewFlagSet("platformctl", flag.ContinueOnError)
	parsed := inputFlags{}
	flags.StringVar(&parsed.platform, "platform", "", "platform specification file")
	flags.StringVar(&parsed.profile, "profile", "", "runtime profile file")
	flags.StringVar(&parsed.catalogue, "catalogue", "", "component catalogue file")
	flags.StringVar(&parsed.output, "output", "", "output directory")
	flags.StringVar(&parsed.target, "target", "", "target override")
	flags.StringVar(&parsed.format, "format", "text", "output format: text or json")
	flags.StringVar(&parsed.resource, "resource", "", "resource identity substring")
	if err := flags.Parse(args); err != nil {
		return parsed, nil, err
	}
	paths := make([]string, 0)
	if parsed.platform != "" {
		paths = append(paths, parsed.platform)
	}
	if parsed.profile != "" {
		paths = append(paths, parsed.profile)
	}
	if parsed.catalogue != "" {
		paths = append(paths, parsed.catalogue)
	}
	parsed.args = flags.Args()
	paths = append(paths, parsed.args...)
	if len(paths) == 0 {
		if _, err := os.Stat("platform.yaml"); err == nil {
			paths = append(paths, "platform.yaml")
		}
		if _, err := os.Stat("profiles/local.yaml"); err == nil {
			paths = append(paths, "profiles/local.yaml")
		}
	}
	if len(paths) == 0 {
		return parsed, nil, errors.New("no input files supplied; use --platform or run platformctl init")
	}
	docs, err := readNamedDocuments(paths)
	if err != nil {
		return parsed, nil, err
	}
	return parsed, docs, nil
}

func readNamedDocuments(paths []string) ([]spec.NamedDocument, error) {
	docs := make([]spec.NamedDocument, 0, len(paths))
	seen := map[string]struct{}{}
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		docs = append(docs, spec.NamedDocument{Name: filepath.ToSlash(path), Content: content})
	}
	return docs, nil
}

func optionalRegistryInputs(args []string) ([]spec.NamedDocument, string, error) {
	flags := flag.NewFlagSet("registry", flag.ContinueOnError)
	target := flags.String("target", "compose", "target used for provider instance resolution")
	if err := flags.Parse(args); err != nil {
		return nil, "", err
	}
	if len(flags.Args()) == 0 {
		return nil, *target, nil
	}
	docs, err := readNamedDocuments(flags.Args())
	if err != nil {
		return nil, "", err
	}
	return docs, *target, nil
}

func migrateResource(res spec.Resource) []map[string]any {
	body := map[string]any{}
	_ = json.Unmarshal(res.Spec, &body)
	base := func(apiVersion, kind string, specBody map[string]any) map[string]any {
		return resourceMap(apiVersion, kind, res.Metadata.Name, res.Metadata.Namespace, specBody)
	}
	baseNamed := func(apiVersion, kind, name string, specBody map[string]any) map[string]any {
		return resourceMap(apiVersion, kind, name, res.Metadata.Namespace, specBody)
	}
	baseConnection := func() map[string]any {
		specBody := map[string]any{"ownership": "external", "engine": "postgres"}
		for _, key := range []string{"host", "port", "database", "ssl"} {
			if value, ok := body[key]; ok {
				specBody[key] = value
			}
		}
		if specBody["database"] == nil {
			specBody["database"] = res.Metadata.Name
		}
		return baseNamed("connections.datascape.dev/v1alpha1", "DatabaseConnection", res.Metadata.Name+"-db", specBody)
	}
	baseSource := func(specBody map[string]any) map[string]any {
		if specBody["connectionRef"] == nil {
			specBody["connectionRef"] = "DatabaseConnection/" + res.Metadata.Name + "-db"
		}
		return map[string]any{
			"apiVersion": "sources.datascape.dev/v1alpha1",
			"kind":       "RelationalSource",
			"metadata": map[string]any{
				"name":      res.Metadata.Name,
				"namespace": res.Metadata.Namespace,
			},
			"spec": specBody,
		}
	}
	clean := func(keys ...string) map[string]any {
		out := map[string]any{}
		skip := map[string]bool{}
		for _, key := range keys {
			skip[key] = true
		}
		for key, value := range body {
			if !skip[key] {
				out[key] = value
			}
		}
		return out
	}
	switch res.Kind {
	case "DataPlatform":
		specBody := clean("rawArchiveRef", "lineagePolicyRef", "verificationPolicyRef", "recoveryPlanRef")
		if specBody["type"] == nil {
			specBody["type"] = "compose"
		}
		return []map[string]any{base(spec.APIVersionV1Alpha1, "Target", specBody)}
	case "PostgresSource":
		specBody := clean("streamRef", "rawArchiveRef", "lineagePolicyRef", "verificationPolicyRef", "host", "port", "database", "username", "password", "credentialsRef")
		out := []map[string]any{baseConnection(), baseSource(specBody)}
		if streamRef, _ := body["streamRef"].(string); streamRef != "" {
			out = append(out, bindingResource(res, res.Metadata.Name+"-change-stream", "datascape.dev/source.change-stream", "RelationalSource/"+res.Metadata.Name, migrateRef(streamRef)))
		}
		if archiveRef, _ := body["rawArchiveRef"].(string); archiveRef != "" {
			source := migrateRef(stringValue(body["streamRef"]))
			if source != "" {
				out = append(out, bindingResource(res, res.Metadata.Name+"-archive", "datascape.dev/store.object", source, migrateRef(archiveRef)))
			}
		}
		if lineageRef, _ := body["lineagePolicyRef"].(string); lineageRef != "" {
			out = append(out, bindingResource(res, res.Metadata.Name+"-lineage", "datascape.dev/lineage.admit", "RelationalSource/"+res.Metadata.Name, migrateRef(lineageRef)))
		}
		return out
	case "EventSource":
		specBody := clean("streamRef", "rawArchiveRef", "lineagePolicyRef")
		out := []map[string]any{base("sources.datascape.dev/v1alpha1", "EventProducer", specBody)}
		if streamRef, _ := body["streamRef"].(string); streamRef != "" {
			out = append(out, bindingResource(res, res.Metadata.Name+"-publish", "datascape.dev/stream.publish", "EventProducer/"+res.Metadata.Name, migrateRef(streamRef)))
		}
		if archiveRef, _ := body["rawArchiveRef"].(string); archiveRef != "" {
			source := migrateRef(stringValue(body["streamRef"]))
			if source != "" {
				out = append(out, bindingResource(res, res.Metadata.Name+"-archive", "datascape.dev/store.object", source, migrateRef(archiveRef)))
			}
		}
		return out
	case "EventStream":
		return []map[string]any{base("streams.datascape.dev/v1alpha1", "EventStream", clean())}
	case "EventContract":
		return []map[string]any{base("contracts.datascape.dev/v1alpha1", "EventContract", clean())}
	case "RawArchive", "ObjectStore":
		return []map[string]any{base("stores.datascape.dev/v1alpha1", "ObjectStore", clean())}
	case "LineagePolicy", "LineageGateway":
		return []map[string]any{base("lineage.datascape.dev/v1alpha1", "LineageSink", clean())}
	case "AuditStore":
		return []map[string]any{base("audit.datascape.dev/v1alpha1", "AuditStore", clean())}
	case "Binding":
		item := bindingResource(res, res.Metadata.Name, capabilityDefault(body, ""), migrateRef(stringValue(body["sourceRef"])), migrateRef(stringValue(body["targetRef"])))
		copyBindingOptions(item, body)
		return []map[string]any{item}
	case "SourceBinding", "ProducerBinding":
		return []map[string]any{bindingResource(res, res.Metadata.Name, capabilityDefault(body, "datascape.dev/stream.publish"), migrateRef(firstString(body, "sourceRef", "producerRef")), migrateRef(firstString(body, "targetRef", "streamRef")))}
	case "ArchiveBinding":
		return []map[string]any{bindingResource(res, res.Metadata.Name, "datascape.dev/store.object", migrateRef(firstString(body, "sourceRef", "streamRef")), migrateRef(firstString(body, "targetRef", "archiveRef")))}
	case "LineageBinding":
		return []map[string]any{bindingResource(res, res.Metadata.Name, "datascape.dev/lineage.admit", migrateRef(stringValue(body["sourceRef"])), migrateRef(firstString(body, "targetRef", "backendRef", "lineagePolicyRef")))}
	case "AuditBinding":
		return []map[string]any{bindingResource(res, res.Metadata.Name, "datascape.dev/audit.record", migrateRef(stringValue(body["sourceRef"])), migrateRef(firstString(body, "targetRef", "auditStoreRef")))}
	case "ComponentCatalogue":
		return nil
	default:
		return []map[string]any{base(res.APIVersion, res.Kind, clean())}
	}
}

func resourceMap(apiVersion, kind, name, namespace string, specBody map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": specBody,
	}
}

func bindingResource(owner spec.Resource, name, capability, sourceRef, targetRef string) map[string]any {
	switch capability {
	case "datascape.dev/source.change-stream":
		return typedBindingResource(owner, name, "CDCBinding", map[string]any{"sourceRef": sourceRef, "streamRef": targetRef})
	case "datascape.dev/stream.publish":
		return typedBindingResource(owner, name, "StreamPublishBinding", map[string]any{"sourceRef": sourceRef, "streamRef": targetRef})
	case "datascape.dev/store.object":
		return typedBindingResource(owner, name, "StreamArchiveBinding", map[string]any{"streamRef": sourceRef, "objectStoreRef": targetRef})
	case "datascape.dev/lineage.admit":
		return typedBindingResource(owner, name, "LineageBinding", map[string]any{"sourceRef": sourceRef, "sinkRef": targetRef})
	case "datascape.dev/audit.record":
		return typedBindingResource(owner, name, "AuditBinding", map[string]any{"sourceRef": sourceRef, "auditStoreRef": targetRef})
	default:
		specBody := map[string]any{}
		if capability != "" {
			specBody["capability"] = capability
		}
		if sourceRef != "" {
			specBody["sourceRef"] = sourceRef
		}
		if targetRef != "" {
			specBody["targetRef"] = targetRef
		}
		return resourceMap(spec.APIVersionV1Alpha1, "Binding", name, owner.Metadata.Namespace, specBody)
	}
}

func typedBindingResource(owner spec.Resource, name, kind string, specBody map[string]any) map[string]any {
	for key, value := range specBody {
		if value == "" {
			delete(specBody, key)
		}
	}
	return resourceMap("bindings.datascape.dev/v1alpha1", kind, name, owner.Metadata.Namespace, specBody)
}

func copyBindingOptions(item map[string]any, body map[string]any) {
	specBody, _ := item["spec"].(map[string]any)
	if specBody == nil {
		return
	}
	for _, key := range []string{"providerInstanceRef", "mode", "ownership", "state", "snapshot", "tables", "format", "retention"} {
		if value, ok := body[key]; ok {
			specBody[key] = value
		}
	}
}

func migrateRef(ref string) string {
	parts := strings.Split(ref, "/")
	if len(parts) == 0 {
		return ref
	}
	kind := parts[0]
	name := ""
	if len(parts) == 2 {
		name = parts[1]
	}
	if len(parts) == 3 {
		name = parts[2]
	}
	if name == "" {
		return ref
	}
	switch kind {
	case "PostgresSource":
		return "RelationalSource/" + name
	case "EventSource":
		return "EventProducer/" + name
	case "RawArchive":
		return "ObjectStore/" + name
	case "LineagePolicy", "LineageGateway":
		return "LineageSink/" + name
	default:
		return kind + "/" + name
	}
}

func capabilityDefault(body map[string]any, fallback string) string {
	if capability := stringValue(body["capability"]); capability != "" {
		return capability
	}
	mode := stringValue(body["mode"])
	switch mode {
	case "cdc":
		return "datascape.dev/source.change-stream"
	case "archive":
		return "datascape.dev/store.object"
	default:
		return fallback
	}
}

func firstString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(body[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func compileOptions(target string) compiler.Options {
	return compiler.Options{
		Target:          target,
		CompilerVersion: version,
		CompilerDigest:  compilerDigest,
		SourceCommit:    sourceCommit,
		SourceDateEpoch: os.Getenv("SOURCE_DATE_EPOCH"),
	}
}

func failOnDiagnostics(diags []domain.Diagnostic) error {
	printDiagnostics(diags)
	if domain.HasErrors(diags) {
		return errors.New("command failed")
	}
	return nil
}

func printDiagnostics(diags []domain.Diagnostic) {
	for _, diag := range diags {
		fmt.Fprintf(os.Stderr, "%s %s", diag.Severity, diag.Code)
		if diag.Resource != "" {
			fmt.Fprintf(os.Stderr, " %s", diag.Resource)
		}
		if diag.FieldPath != "" {
			fmt.Fprintf(os.Stderr, " %s", diag.FieldPath)
		}
		fmt.Fprintf(os.Stderr, ": %s", diag.Message)
		if diag.Remediation != "" {
			fmt.Fprintf(os.Stderr, " remediation: %s", diag.Remediation)
		}
		if diag.Location.File != "" {
			fmt.Fprintf(os.Stderr, " source: %s", diag.Location.File)
		}
		fmt.Fprintln(os.Stderr)
	}
}

func printCanonical(value any) error {
	content, err := canonical.JSON(value)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(content, '\n'))
	return err
}

func writeNonOverwriting(ctx context.Context, root string, files []artifact.File) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if _, err := os.Stat(target); err == nil {
			continue
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, file.Content, os.FileMode(file.Mode)); err != nil {
			return err
		}
	}
	return nil
}

func writeOne(ctx context.Context, path string, content []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func usage() {
	fmt.Println(`platformctl - deterministic data-platform compiler

Commands:
  init
  validate
  plan
  generate
  diff
  inspect
  api-resources
  api-definitions
  providers
  bindings
  explain <kind>
  migrate
  docs
  secrets init
  verify
  conformance
  recover plan
  recover generate
  version`)
}

func initPlatform() string {
	return `apiVersion: platform.datascape.dev/v1alpha1
kind: Target
metadata:
  name: local
  namespace: education
spec:
  type: compose
---
apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata:
  name: student-records-db-credentials
  namespace: education
spec:
  backend: env
  keys:
    - username
    - password
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata:
  name: student-records-db
  namespace: education
spec:
  engine: postgres
  host: relational-source
  port: 5432
  database: attendance
  credentialsRef: SecretReference/student-records-db-credentials
---
apiVersion: sources.datascape.dev/v1alpha1
kind: RelationalSource
metadata:
  name: student-records
  namespace: education
spec:
  connectionRef: DatabaseConnection/student-records-db
  tables:
    - student_attendance
---
apiVersion: streams.datascape.dev/v1alpha1
kind: EventStream
metadata:
  name: attendance-changes
  namespace: education
spec:
  eventClass: change
---
apiVersion: stores.datascape.dev/v1alpha1
kind: ObjectStore
metadata:
  name: evidence-store
  namespace: education
spec:
  bucket: attendance-evidence
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata:
  name: student-records-change-stream
  namespace: education
spec:
  sourceRef: RelationalSource/student-records
  streamRef: EventStream/attendance-changes
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamArchiveBinding
metadata:
  name: attendance-changes-object-store
  namespace: education
spec:
  streamRef: EventStream/attendance-changes
  objectStoreRef: ObjectStore/evidence-store
`
}

func initProfile(name, target string) string {
	return fmt.Sprintf(`{
  "apiVersion": "platform.datascape.dev/v1alpha1",
  "kind": "RuntimeProfile",
  "metadata": {
    "name": %q,
    "namespace": "education"
  },
  "spec": {
    "target": %q,
    "availability": {
      "class": "local",
      "failureDomains": 1,
      "minimumReplicas": 1
    },
    "development": {
      "enabled": true,
      "allowUnpinnedImages": true
    }
  }
}
`, name, target)
}
