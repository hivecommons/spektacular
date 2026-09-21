---
tags: [repo, config, validation, default]
---

## RepoConfig must be constructed via NewDefaultRepoConfig(), never a bare struct literal

Building a `config.RepoConfig` (`internal/config/repo.go`) directly as a struct
literal — e.g. `config.RepoConfig{Description: "...", Role: "..."}` — looks
harmless when you only care about the descriptive fields, but it silently
fails later: `config.RepoConfigFromYAMLFile`'s `cfg.Validate()` rejects a
`RepoConfig` whose `Knowledge`/`Changelog` sub-configs are left zero-value,
even though those fields have nothing to do with what you were trying to set.
The failure surfaces far from the construction site — as `DescriptiveMetadata`
quietly returning `ok=false` for a repo whose config file exists and looks
fine — which makes it easy to misdiagnose as a path or filesystem problem.

Always start from `config.NewDefaultRepoConfig()` and override only the
fields you need:

```go
cfg := config.NewDefaultRepoConfig()
cfg.Description = "..."
cfg.Role = "..."
```

This applies to any code path that constructs a `RepoConfig` from scratch
(tests, tooling, migrations) rather than reading one back from an existing
YAML file, where `RepoConfigFromYAMLFile` already applies the same defaulting
internally before unmarshalling over it.
