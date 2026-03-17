# CLAUDE.md — PetalFlow Repository Instructions

## Documentation Policy

After any CRUD operation to the repository — adding a file, modifying a schema, implementing a feature, updating an API, changing a CLI interface, adding a node type, modifying a data model, or removing a capability — you must create or update a documentation file in the `/docs/changes/` directory.

This directory is the source of truth for an automated documentation pipeline. Another LLM will read these files to generate and keep the external project docs current. Write with that reader in mind: be precise, include concrete examples, and make every assumption explicit.

---

### Filename Convention

```
/docs/changes/YYYY-MM-DD_v{version}_{feature-slug}.md
```

| Segment | Rule |
|---|---|
| `YYYY-MM-DD` | The current date (UTC). |
| `v{version}` | Semantic version from `go.mod` or the nearest `VERSION` file. If no version is declared, use `v0.0.0-dev`. |
| `{feature-slug}` | A kebab-case description of the feature or change. Be specific — prefer `agent-task-compiler` over `compiler`, and `mcp-overlay-schema` over `overlay`. |

**Examples:**

```
2026-03-17_v1.0.0_agent-task-compiler.md
2026-03-17_v1.0.0_mcp-adapter-stdio-transport.md
2026-03-18_v1.0.0_tool-registry-cli-commands.md
2026-03-20_v1.1.0_cortex-auth-jwt.md
```

If a session produces multiple related changes that belong to the same logical feature (e.g., adding both a Go package and its CLI commands for the same capability), consolidate them into one file. If changes are genuinely independent features, create one file per feature.

Do not overwrite an existing doc for a different feature just because the date matches. Create a new file with a distinct feature slug.

---

### Document Structure

Every change doc must follow this structure. Do not skip sections — if a section doesn't apply, write "N/A" with one sentence explaining why.

```markdown
---
date: YYYY-MM-DD
version: v{semver}
feature: {human-readable feature name}
product: {petalflow | cortex | iris | petaltrace | petalcron | shared}
change_type: {feature | bugfix | breaking | deprecation | refactor | schema | api | cli | docs}
affected_components: [{list of packages, files, or subsystems changed}]
related_frds: [{list of FRD filenames or section references, if applicable}]
---

## Summary

<!-- 2–4 sentences. What was added, changed, or removed, and why. This is the
     one paragraph a developer reads to decide if they need to care. -->

## Motivation

<!-- Why this change exists. What problem it solves or what capability it
     unlocks. Reference the FRD or design doc if one drove this work. -->

## What Changed

### New Additions
<!-- List new files, packages, types, functions, endpoints, CLI commands,
     schemas, or config fields. For each, include its signature or shape
     and a one-sentence purpose. -->

### Modifications
<!-- Existing things that changed behavior, signature, or shape. For each,
     show the before and after if the diff is non-trivial. -->

### Removals
<!-- Anything deleted or made internal that was previously public. -->

## Technical Specification

<!-- The meaty section. Include all of the following that apply: -->

### Data Schemas / Types
<!-- JSON, YAML, or Go struct definitions for any new or changed data shapes.
     Include field-level comments explaining purpose and constraints. -->

### API Endpoints
<!-- For daemon API changes: method, path, request body shape, response body
     shape, and error codes. -->

### CLI Interface
<!-- For CLI changes: the full command signatures with flags, expected output
     format, and example invocations with realistic inputs and outputs. -->

### Go Package API
<!-- For SDK-level changes: exported types, interfaces, and functions.
     Include the function signature and a sentence on usage contract. -->

### Configuration
<!-- New or changed config fields in petalflow.yaml, environment variables,
     or MCP overlay schema. Include type, required/optional, default, and
     where PetalFlow reads it from. -->

## Usage Examples

<!-- At least one end-to-end example per major addition. Examples should be
     copy-paste ready. Prefer realistic inputs over toy ones. Show both the
     happy path and at least one error case where relevant. -->

### Example: {descriptive title}

```{language}
{code}
```

## Integration Notes

<!-- How this change connects to the rest of the Petal Labs ecosystem.
     Specifically: does this change affect PetalFlow ↔ Cortex, Iris, or
     PetalTrace integration? Does it change the Agent/Task compiler's
     behavior, the graph IR, or MCP adapter semantics? Note any downstream
     components that need to be updated or tested as a result. -->

## Breaking Changes & Migration

<!-- Anything that breaks existing workflows, configs, CLI scripts, or SDK
     usage. For each breaking change: what breaks, who is affected, and
     what they need to do to migrate. If nothing breaks, write "None." -->

## Deferred / Out of Scope

<!-- Capabilities that were explicitly left out of this change and why.
     Reference the v1.1 or post-v1 deferral list if applicable. This helps
     the documentation pipeline avoid documenting things that don't exist yet. -->

## Testing Notes

<!-- What was tested, how, and what edge cases were validated. If tests were
     added, name the test files or test functions. This is not a test plan —
     it's a record of what actually ran. -->
```

---

### Rules

1. **Write the doc before marking work complete.** The documentation file is part of the deliverable, not an afterthought.

2. **Be concrete.** Vague summaries like "updated the tool system" are useless to the downstream LLM. Name the specific types, endpoints, flags, and schemas that changed.

3. **Preserve intent, not just mechanics.** The `Motivation` and `Integration Notes` sections exist so the external docs can explain *why* something works the way it does, not just *what* it does.

4. **Exact schemas and signatures are required.** If you added a new Go struct, paste the full struct definition. If you added a CLI command, paste the full help output. The downstream doc generator cannot infer what you don't write.

5. **Do not duplicate FRD content verbatim.** If an FRD already captures a design decision, reference it by name (`petalflow-tools-contract-frd.md § 4.3`) instead of copying paragraphs. The change doc records *what was implemented*, not what was designed.

6. **One doc per logical feature, not per file edit.** If implementing the MCP adapter required changes to 12 files, that is one doc.

7. **The `affected_components` front-matter field must be exhaustive.** List every Go package path, file, or subsystem touched. This is used by the pipeline to build a change index.

8. **Iterative working**. If a doc with today's date and the same feature slug already exists, append a ## Revision: {timestamp} section rather than creating a new file.