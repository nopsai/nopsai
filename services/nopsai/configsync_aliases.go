package main

import "nopsai/services/nopsai/internal/configsync"

type configRepositoryBindingFile = configsync.BindingFile
type pipelineRunStructureNode = configsync.PipelineRunStructureNode
type pipelineRunStructureApp = configsync.PipelineRunStructureApp

var (
	normalizeConfigRepositoryBasePathValue         = configsync.NormalizeRepositoryBasePathValue
	configRepoJoinPath                             = configsync.RepoJoinPath
	relativeConfigPath                             = configsync.RelativePath
	normalizeConfigPathForFolder                   = configsync.NormalizePathForFolder
	stripConfigResourcePrefix                      = configsync.StripResourcePrefix
	cleanConfigPathSegments                        = configsync.CleanPathSegments
	configResourceUnderAnyScope                    = configsync.ResourceUnderAnyScope
	configResourceUnderScope                       = configsync.ResourceUnderScope
	canConfigRepositoryWriteOver                   = configsync.CanRepositoryWriteOver
	canConfigRepositoryAdoptUnmanagedResource      = configsync.CanRepositoryAdoptUnmanagedResource
	configRepositoryShadowsCurrent                 = configsync.RepositoryShadowsCurrent
	splitPipelineIdentifier                        = configsync.SplitPipelineIdentifier
	normalizePipelineIdentifierReference           = configsync.NormalizePipelineIdentifierReference
	buildPipelineIdentifier                        = configsync.BuildPipelineIdentifier
	buildPipelineFilePath                          = configsync.BuildPipelineFilePath
	splitStepIdentifier                            = configsync.SplitStepIdentifier
	buildStepIdentifier                            = configsync.BuildStepIdentifier
	parseScopeFilePath                             = configsync.ParseScopeFilePath
	parseConfigRepositoryBindingPath               = configsync.ParseBindingPath
	validateConfigRepositoryBindingFile            = configsync.ValidateBindingFile
	configRepositoryBindingWriteSettings           = configsync.BindingWriteSettings
	parsePipelineRunStructure                      = configsync.ParsePipelineRunStructure
	parseConfigRepositoryGroupPipelineRunStructure = configsync.ParseConfigRepositoryGroupPipelineRunStructure
	normalizePipelineRunStructureForFolder         = configsync.NormalizePipelineRunStructureForFolder
	ensurePipelineRunStructurePath                 = configsync.EnsurePipelineRunStructurePath
	mergePipelineRunStructure                      = configsync.MergePipelineRunStructure
	filterPipelineRunStructureByScopes             = configsync.FilterPipelineRunStructureByScopes
	normalizeStructureName                         = configsync.NormalizeStructureName
	isReservedRootGroupName                        = configsync.IsReservedRootGroupName
)
