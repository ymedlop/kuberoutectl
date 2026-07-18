## Description

<!-- What does this PR do? State the goal clearly. -->

## Design & Assumptions

<!-- 
- State key assumptions explicitly
- If multiple approaches were considered, explain why you chose this one
- For major changes, describe the domain mapping, interfaces, or provider fit
- Link to any design discussion or related issues
-->

## Changes

<!-- List what was actually changed. Every change should trace to the request. -->

- 
- 

## Verification

<!-- Document what was tested and what was NOT. This is mandatory. -->

### ✅ Verified

- [ ] `go test ./...` passes (unit tests: domain, services, providers)
- [ ] `make check` passes (fmt + vet + test)
- [ ] `bash scripts/e2e.sh` passes (4-provider operator flow)
- [ ] New tests added (if applicable)
- [ ] Fixtures or FakeRunner tests updated (if applicable)

### ⚠️ Not Verified (Caveats)

<!-- If something couldn't be tested (e.g., real CLI output, interactive auth), say so here. -->
- 

## Provider Impact (if applicable)

<!-- If this touches provider packages (Azure, AWS, GCP, kubectl), confirm: -->
- [ ] Provider package structure unchanged or documented
- [ ] `parse.go` / `build.go` / `health.go` / `activate.go` / `renew.go` pattern preserved
- [ ] Error handling follows convention (CLI command failure = resilient; parse failure = hard error)

## Related Issues

<!-- Link related issues or PRs -->

Closes #
Related to #

---

**Note:** Minimal, surgical changes. No speculative features. Tradeoffs highlighted.
