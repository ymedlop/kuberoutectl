# AWS

Use AWS CLI and STS as the source of truth for AWS identity and cluster discovery.

## Typical operations

- `aws sts get-caller-identity`
- profile/config discovery
- EKS cluster discovery through AWS APIs or CLI-backed flows

## Notes

- Identity validation is a key health signal.
- Discovery should focus on effective identity, account context, and EKS clusters.
- Renewal flows depend on auth type and should be provider-specific.
- Keep parsing deterministic and testable.
