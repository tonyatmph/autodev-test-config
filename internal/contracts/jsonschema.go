package contracts

import (
	"bytes"
	"embed"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schemas/*.schema.json
var schemaFS embed.FS

const (
	ConfigSchema                = "config.schema.json"
	StageSpecSchema             = "stage-spec.schema.json"
	IssueFileSchema             = "issue-file.schema.json"
	IssueSpecSchema             = "issue-spec.schema.json"
	PipelineCatalogSchema       = "pipeline-catalog.schema.json"
	PersistedStateSchema        = "persisted-state.schema.json"
	PipelineIntentSchema        = "pipeline-intent.schema.json"
	PolicyEvaluationSchema      = "policy-evaluation.schema.json"
	PipelineBuildPlanSchema     = "pipeline-build-plan.schema.json"
	PipelineExecutionPlanSchema = "pipeline-execution-plan.schema.json"
	StageContextSchema          = "stage-context.schema.json"
	StageResultSchema           = "stage-result.schema.json"
	StageReportSchema           = "stage-report.schema.json"
	StageStateSchema            = "stage-state.schema.json"
	StageMetadataSchema         = "stage-metadata.schema.json"
)

var (
	compileOnce sync.Once
	compileErr  error
	compiled    map[string]*jsonschema.Schema
)

func Validate(schemaName, documentName string, data []byte) error {
	sch, err := compiledSchema(schemaName)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s is not valid json: %w", documentName, err)
	}
	if err := sch.Validate(inst); err != nil {
		return fmt.Errorf("%s does not satisfy %s: %w", documentName, schemaName, err)
	}
	return nil
}

func compiledSchema(schemaName string) (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		compiled = map[string]*jsonschema.Schema{}
		c := jsonschema.NewCompiler()
		for _, name := range []string{
			ConfigSchema,
			StageSpecSchema,
			IssueFileSchema,
			IssueSpecSchema,
			PipelineCatalogSchema,
			PersistedStateSchema,
			PipelineIntentSchema,
			PolicyEvaluationSchema,
			PipelineBuildPlanSchema,
			PipelineExecutionPlanSchema,
			StageContextSchema,
			StageResultSchema,
			StageReportSchema,
			StageStateSchema,
			StageMetadataSchema,
		} {
			data, err := schemaFS.ReadFile("schemas/" + name)
			if err != nil {
				compileErr = fmt.Errorf("read embedded schema %s: %w", name, err)
				return
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
			if err != nil {
				compileErr = fmt.Errorf("decode embedded schema %s: %w", name, err)
				return
			}
			if err := c.AddResource(name, doc); err != nil {
				compileErr = fmt.Errorf("register embedded schema %s: %w", name, err)
				return
			}
		}
		for _, name := range []string{
			ConfigSchema,
			StageSpecSchema,
			IssueFileSchema,
			IssueSpecSchema,
			PipelineCatalogSchema,
			PersistedStateSchema,
			PipelineIntentSchema,
			PolicyEvaluationSchema,
			PipelineBuildPlanSchema,
			PipelineExecutionPlanSchema,
			StageContextSchema,
			StageResultSchema,
			StageReportSchema,
			StageStateSchema,
			StageMetadataSchema,
		} {
			sch, err := c.Compile(name)
			if err != nil {
				compileErr = fmt.Errorf("compile embedded schema %s: %w", name, err)
				return
			}
			compiled[name] = sch
		}
	})
	if compileErr != nil {
		return nil, compileErr
	}
	sch, ok := compiled[schemaName]
	if !ok {
		return nil, fmt.Errorf("unknown schema %q", schemaName)
	}
	return sch, nil
}
