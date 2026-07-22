---
title: "kuberoutectl setup aws-sso"
parent: Command reference
layout: default
description: "Generate ~/.aws/config profiles for every account in an AWS SSO session"
---

## kuberoutectl setup aws-sso

Generate ~/.aws/config profiles for every account in an AWS SSO session

### Synopsis

Enumerate every account you can reach through an AWS IAM Identity Center
(SSO) session and write a `kr-<account>-<role>` profile for each into
~/.aws/config, so `kuberoutectl sync aws` (and plain aws/kubectl) can use
them. Requires an active SSO login first: `aws sso login --sso-session <name>`.
Existing profiles are never modified; only missing ones are appended.

```
kuberoutectl setup aws-sso --sso-session <name> [flags]
```

### Options

```
  -h, --help                 help for aws-sso
      --region string        region set on generated profiles for EKS discovery (default: the session's sso_region)
      --role string          preferred role to select per account (default: AdministratorAccess if present, else first)
      --sso-session string   name of the [sso-session] block in ~/.aws/config
```

### Options inherited from parent commands

```
  -o, --output string   output format: text|json (default "text")
```

### SEE ALSO

* [kuberoutectl setup](kuberoutectl_setup.md)	 - Prepare local provider configuration

