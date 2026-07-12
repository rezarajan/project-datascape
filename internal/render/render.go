package render

import (
	"context"
	"fmt"

	"datascape.dev/platformctl/internal/artifact"
	"datascape.dev/platformctl/internal/canonical"
	"datascape.dev/platformctl/internal/ir"
)

func FoundationFiles(ctx context.Context, plan ir.PlatformPlan) ([]artifact.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	planJSON, err := canonical.JSON(plan)
	if err != nil {
		return nil, err
	}
	resourcesJSON, err := canonical.JSON(plan.Resources)
	if err != nil {
		return nil, err
	}
	files := []artifact.File{
		text("README.md", fmt.Sprintf("# platformctl bundle\n\nTarget: `%s`\n\nThis bundle contains deterministic compiler metadata, resource definitions, provider selections, binding resolutions, graph state, policies, and overrides. Use the Compose target when provider-owned local runtime services are required.\n", plan.Target)),
		jsonFile("plan.json", planJSON),
		jsonFile("resources.json", resourcesJSON),
		text("manifests/README.md", "# Manifests\n\nTarget renderers place provider-owned manifests here.\n"),
		text("configuration/README.md", "# Configuration\n\nGenerated binding and provider configuration is placed here.\n"),
		text("schemas/README.md", "# Schemas\n\nResourceDefinition schemas and provider schema projections are placed here.\n"),
		text("policies/README.md", "# Policies\n\nPolicy projections are placed here.\n"),
		text("pipelines/README.md", "# Pipelines\n\nPipeline definitions are placed here.\n"),
		text("workflows/README.md", "# Workflows\n\nWorkflow definitions are placed here.\n"),
		text("dashboards/README.md", "# Dashboards\n\nDashboard definitions are placed here.\n"),
		text("verification/README.md", "# Verification\n\nStatic and runtime verification artifacts are placed here.\n"),
		text("recovery/README.md", "# Recovery\n\nRecovery graphs and target recovery artifacts are placed here.\n"),
		text("documentation/README.md", "# Documentation\n\nGenerated documentation is placed here.\n"),
	}
	return files, nil
}

func text(path, content string) artifact.File {
	return artifact.File{Path: path, Mode: 0o644, Content: []byte(content), Deterministic: true}
}

func jsonFile(path string, content []byte) artifact.File {
	return artifact.File{Path: path, Mode: 0o644, Content: append(content, '\n'), Deterministic: true}
}
