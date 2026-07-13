---
name: cloud-adapters
description: Use when implementing or reviewing provider integrations, external CLI execution, binary resolution, and provider-specific discovery or renewal logic for kuberoutectl.
---

# Cloud Adapters

Use this skill when working on cloud provider integrations in `kuberoutectl`.

## Goals

- Keep provider logic isolated behind interfaces.
- Treat external CLIs as adapters, not as the core product.
- Preserve provider-agnostic services and models.
- Keep Azure and AWS behavior explicit and testable.

## Workflow

1. Identify the provider and capability being implemented.
2. Locate the provider-specific adapter boundary.
3. Add or update the smallest discovery or renewal function.
4. Keep external command execution structured and isolated.
5. Add tests for parsing and state mapping.
6. Verify the core domain model still stays provider-agnostic.

## When to use

Use this skill for:
- Azure discovery and renewal logic,
- AWS discovery and renewal logic,
- binary resolution,
- command execution wrappers,
- provider capability modeling,
- external CLI integration.

## References

- `references/azure.md`
- `references/aws.md`
- `references/cli-resolution.md`
- `references/interfaces.md`
