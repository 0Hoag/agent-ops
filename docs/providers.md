# Provider configuration

Configure one or more repositories with `SCM_REPOSITORIES_JSON`. Each entry is
independent: its provider, upstream full name, secret variable, token variable,
and optional API base URL variable are resolved separately.

```dotenv
SCM_REPOSITORIES_JSON=[{"provider":"github","full_name":"acme/app","webhook_secret_env":"ACME_GITHUB_WEBHOOK_SECRET","token_env":"ACME_GITHUB_TOKEN","base_url_env":"ACME_GITHUB_API_BASE_URL"},{"provider":"gitlab","full_name":"acme/platform/app","webhook_secret_env":"ACME_GITLAB_WEBHOOK_SECRET","token_env":"ACME_GITLAB_TOKEN","base_url_env":"ACME_GITLAB_API_BASE_URL"}]

ACME_GITHUB_WEBHOOK_SECRET=
ACME_GITHUB_TOKEN=
ACME_GITHUB_API_BASE_URL=https://api.github.com
ACME_GITLAB_WEBHOOK_SECRET=
ACME_GITLAB_TOKEN=
ACME_GITLAB_API_BASE_URL=https://gitlab.com
```

GitHub webhooks use `X-Hub-Signature-256` and GitLab webhooks use
`X-Gitlab-Token`. The API selects the configured repository by the provider
payload identity (`repository.full_name` for GitHub and
`project.path_with_namespace` for GitLab), so equal issue or merge-request
numbers remain isolated by repository. Sync uses the same entry-specific token
and base URL and is safe to repeat because persistence conflicts are keyed by
`repository_id` plus the upstream entity key.

For self-hosted installations, set the matching base URL variable to the
instance API origin, for example `https://gitlab.example.com` or a GitHub
Enterprise API origin such as `https://github.example.com/api/v3`.
