package liteboxd

import "github.com/fslongjin/liteboxd/backend/pkg/model"

// Sandbox types
type Sandbox = model.Sandbox
type SandboxStatus = model.SandboxStatus
type SandboxOverrides = model.SandboxOverrides
type CreateSandboxRequest = model.CreateSandboxRequest
type SandboxListResponse = model.SandboxListResponse
type ExecRequest = model.ExecRequest
type ExecResponse = model.ExecResponse
type LogsResponse = model.LogsResponse

// Template types
type Template = model.Template
type TemplateSpec = model.TemplateSpec
type ResourceSpec = model.ResourceSpec
type FileSpec = model.FileSpec
type ProbeSpec = model.ProbeSpec
type TemplateVersion = model.TemplateVersion
type CreateTemplateRequest = model.CreateTemplateRequest
type UpdateTemplateRequest = model.UpdateTemplateRequest
type RollbackRequest = model.RollbackRequest
type RollbackResponse = model.RollbackResponse
type TemplateListResponse = model.TemplateListResponse
type VersionListResponse = model.VersionListResponse
type TemplateListOptions = model.TemplateListOptions

// Prepull types
type PrepullStatus = model.PrepullStatus
type PrepullResponse = model.PrepullResponse
type PrepullListResponse = model.PrepullListResponse
type CreatePrepullRequest = model.CreatePrepullRequest

// Import/Export types
type ImportStrategy = model.ImportStrategy
type ImportTemplatesRequest = model.ImportTemplatesRequest
type ImportTemplatesResponse = model.ImportTemplatesResponse
type ImportResult = model.ImportResult
type TemplateYAML = model.TemplateYAML
type TemplateYAMLMetadata = model.TemplateYAMLMetadata
type TemplateListYAML = model.TemplateListYAML

// Constants
const (
	SandboxStatusPending     = model.SandboxStatusPending
	SandboxStatusRunning     = model.SandboxStatusRunning
	SandboxStatusSucceeded   = model.SandboxStatusSucceeded
	SandboxStatusFailed      = model.SandboxStatusFailed
	SandboxStatusTerminating = model.SandboxStatusTerminating
	SandboxStatusUnknown     = model.SandboxStatusUnknown

	PrepullStatusPending   = model.PrepullStatusPending
	PrepullStatusPulling   = model.PrepullStatusPulling
	PrepullStatusCompleted = model.PrepullStatusCompleted
	PrepullStatusFailed    = model.PrepullStatusFailed

	ImportStrategyCreateOnly     = model.ImportStrategyCreateOnly
	ImportStrategyUpdateOnly     = model.ImportStrategyUpdateOnly
	ImportStrategyCreateOrUpdate = model.ImportStrategyCreateOrUpdate
)
